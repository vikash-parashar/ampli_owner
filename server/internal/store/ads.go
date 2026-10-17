package store

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/vikash-parashar/ampli_owner/server/internal/ads"
)

// ---- ad accounts ----

type AdAccount struct {
	ID         int64      `json:"id"`
	Platform   string     `json:"platform"`
	ExternalID string     `json:"external_id"`
	Name       string     `json:"name"`
	Currency   string     `json:"currency"`
	Status     string     `json:"status"`
	LastSyncAt *time.Time `json:"last_sync_at"`
	LastError  string     `json:"last_error"`
	CreatedAt  time.Time  `json:"created_at"`
}

// SealedToken is an encrypted access/refresh token pair.
type SealedToken struct {
	Access  []byte
	Refresh []byte
	Expires *time.Time
}

const accountCols = `id, platform, external_id, name, currency, status, last_sync_at, last_error, created_at`

func scanAccount(r pgx.Row, extra ...any) (AdAccount, error) {
	var a AdAccount
	err := r.Scan(append([]any{&a.ID, &a.Platform, &a.ExternalID, &a.Name, &a.Currency, &a.Status, &a.LastSyncAt, &a.LastError, &a.CreatedAt}, extra...)...)
	if errors.Is(err, pgx.ErrNoRows) {
		return a, ErrNotFound
	}
	return a, err
}

// SaveAdAccount adds or reconnects an account.
func (s *Store) SaveAdAccount(ctx context.Context, platform string, a ads.Account, t SealedToken) (AdAccount, error) {
	return scanAccount(s.DB.QueryRow(ctx, `INSERT INTO ad_accounts (platform, external_id, name, currency, token_enc, refresh_enc, expires_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7)
		ON CONFLICT (platform, external_id) DO UPDATE SET name=EXCLUDED.name, currency=EXCLUDED.currency, token_enc=EXCLUDED.token_enc,
			refresh_enc=COALESCE(EXCLUDED.refresh_enc, ad_accounts.refresh_enc), expires_at=EXCLUDED.expires_at, status='connected', last_error=''
		RETURNING `+accountCols, platform, a.ExternalID, a.Name, a.Currency, t.Access, t.Refresh, t.Expires))
}

func (s *Store) AdAccounts(ctx context.Context) ([]AdAccount, error) {
	rows, err := s.DB.Query(ctx, `SELECT `+accountCols+` FROM ad_accounts ORDER BY platform, name`)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(r pgx.CollectableRow) (AdAccount, error) { return scanAccount(r) })
}

func (s *Store) AdAccountToken(ctx context.Context, id int64) (AdAccount, SealedToken, error) {
	var t SealedToken
	a, err := scanAccount(s.DB.QueryRow(ctx, `SELECT `+accountCols+`, token_enc, refresh_enc, expires_at FROM ad_accounts WHERE id=$1`, id),
		&t.Access, &t.Refresh, &t.Expires)
	return a, t, err
}

func (s *Store) UpdateAdAccountToken(ctx context.Context, id int64, t SealedToken) error {
	_, err := s.DB.Exec(ctx, `UPDATE ad_accounts SET token_enc=$2, expires_at=$3 WHERE id=$1`, id, t.Access, t.Expires)
	return err
}

// MarkAdAccount records the outcome of a sync or call. synced also stamps last_sync_at.
func (s *Store) MarkAdAccount(ctx context.Context, id int64, status, lastErr string, synced bool) error {
	_, err := s.DB.Exec(ctx, `UPDATE ad_accounts SET status=$2, last_error=$3,
		last_sync_at = CASE WHEN $4 THEN now() ELSE last_sync_at END WHERE id=$1`, id, status, lastErr, synced)
	return err
}

func (s *Store) DeleteAdAccount(ctx context.Context, id int64) error {
	tag, err := s.DB.Exec(ctx, `DELETE FROM ad_accounts WHERE id=$1`, id)
	if err == nil && tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return err
}

// ---- campaigns ----

type Campaign struct {
	ID                 int64       `json:"id"`
	Platform           string      `json:"platform"`
	ProductID          string      `json:"product"`
	Name               string      `json:"name"`
	Status             string      `json:"status"`
	Countries          []string    `json:"countries"`
	DailyBudgetPaise   int64       `json:"daily_budget_paise"`
	SpendPaise         int64       `json:"spend_paise"`
	Impressions        int64       `json:"impressions"`
	Clicks             int64       `json:"clicks"`
	Installs           int64       `json:"installs"`
	AttributedInstalls int64       `json:"attributed_installs"`
	RevenuePaise       int64       `json:"revenue_paise"`
	ROAS               float64     `json:"roas"`
	CPIPaise           int64       `json:"cpi_paise"`
	Days               int         `json:"days"`
	Verdict            ads.Verdict `json:"verdict"`
	AdAccountID        *int64      `json:"ad_account_id"`
	UTMCampaign        string      `json:"utm_campaign"`
	CreativeURL        string      `json:"creative_url"`
	LastError          string      `json:"last_error"`
	AutoStop           AutoStop    `json:"auto_stop"`
	Attribution        string      `json:"attribution"` // link, estimate or none
	CreatedAt          time.Time   `json:"created_at"`
	ExternalID         string      `json:"-"`
	ExternalSetID      string      `json:"-"`
	manualRevenue      int64
}

// AutoStop pauses a live campaign once ROAS stays below MinROAS after MinDays of spend and MinSpendPaise.
type AutoStop struct {
	Enabled       bool    `json:"enabled"`
	MinROAS       float64 `json:"min_roas"`
	MinDays       int     `json:"min_days"`
	MinSpendPaise int64   `json:"min_spend_paise"`
	// PendingSince is when a sync first found the campaign below the limit; the next sync 24 h later pauses it.
	PendingSince *time.Time `json:"pending_since"`
	// HoldUntil: after the owner resumes by hand, the rule waits until then.
	HoldUntil *time.Time `json:"hold_until"`
}

// attribution assigns each ad install to one campaign: by our own utm_campaign link first,
// otherwise to the newest campaign on the same platform for the same app that started before the install.
const attribution = `
WITH att AS (
  SELECT i.product_id, i.install_id, l.id AS link_id, COALESCE(l.id,
    (SELECT c.id FROM campaigns c WHERE i.ad_platform <> '' AND c.platform = i.ad_platform AND c.product_id = i.product_id
       AND c.created_at <= i.created_at AND c.status <> 'draft' ORDER BY c.created_at DESC, c.id DESC LIMIT 1)) AS campaign_id
  FROM installs i LEFT JOIN campaigns l ON i.utm_campaign <> '' AND l.utm_campaign = i.utm_campaign
  WHERE i.ad_platform <> '' OR i.utm_campaign LIKE 'ampli-c%'
), per AS (
  SELECT att.campaign_id, count(*) AS installs, count(att.link_id) AS by_link, COALESCE(sum(p.amt), 0)::bigint AS revenue
  FROM att LEFT JOIN (SELECT product_id, install_id, sum(net_paise) AS amt FROM purchases
    WHERE verified AND refunded_at IS NULL GROUP BY 1, 2) p
    ON p.product_id = att.product_id AND p.install_id = att.install_id
  WHERE att.campaign_id IS NOT NULL GROUP BY 1
)`

const campaignSelect = attribution + `
SELECT c.id, c.platform, COALESCE(c.product_id, ''), c.name, c.status, c.countries, c.daily_budget_paise, c.spend_paise,
  c.impressions, c.clicks, c.installs, c.revenue_paise, c.ad_account_id, COALESCE(c.utm_campaign, ''), c.creative_url, c.last_error,
  c.created_at, c.external_id, c.external_set_id, c.auto_stop, c.auto_min_roas, c.auto_min_days, c.auto_min_spend, c.auto_strike_at, c.auto_hold_until,
  (SELECT count(*) FROM campaign_days d WHERE d.campaign_id = c.id AND d.spend_paise > 0)::int,
  COALESCE(per.installs, 0), COALESCE(per.by_link, 0), COALESCE(per.revenue, 0)
FROM campaigns c LEFT JOIN per ON per.campaign_id = c.id`

func scanCampaign(r pgx.Row) (Campaign, error) {
	var c Campaign
	var attrRev, byLink int64
	err := r.Scan(&c.ID, &c.Platform, &c.ProductID, &c.Name, &c.Status, &c.Countries, &c.DailyBudgetPaise, &c.SpendPaise,
		&c.Impressions, &c.Clicks, &c.Installs, &c.manualRevenue, &c.AdAccountID, &c.UTMCampaign, &c.CreativeURL, &c.LastError,
		&c.CreatedAt, &c.ExternalID, &c.ExternalSetID, &c.AutoStop.Enabled, &c.AutoStop.MinROAS, &c.AutoStop.MinDays,
		&c.AutoStop.MinSpendPaise, &c.AutoStop.PendingSince, &c.AutoStop.HoldUntil, &c.Days, &c.AttributedInstalls, &byLink, &attrRev)
	if errors.Is(err, pgx.ErrNoRows) {
		return c, ErrNotFound
	}
	if err != nil {
		return c, err
	}
	c.RevenuePaise = attrRev + c.manualRevenue
	switch {
	case c.AttributedInstalls == 0:
		c.Attribution = "none"
	case byLink == c.AttributedInstalls:
		c.Attribution = "link"
	default:
		c.Attribution = "estimate" // some installs matched by platform and time only
	}
	if c.Installs == 0 {
		c.Installs = c.AttributedInstalls
	}
	if c.SpendPaise > 0 {
		c.ROAS = float64(c.RevenuePaise) / float64(c.SpendPaise)
	}
	if c.Installs > 0 {
		c.CPIPaise = c.SpendPaise / c.Installs
	}
	if c.Countries == nil {
		c.Countries = []string{}
	}
	c.Verdict = ads.Judge(c.Status, c.SpendPaise, c.RevenuePaise, c.Days)
	if c.Attribution == "none" && c.SpendPaise > 0 && oneOfCode(c.Verdict.Code, "scale", "watch", "stop") {
		c.Verdict = ads.Verdict{Code: "unknown", Label: "Can't judge yet",
			Reason: "No installs are matched to this ad yet, so its earnings can't be measured. Check Google Play is connected."}
	}
	return c, nil
}

// Campaigns lists campaigns for one product, or all when product is "".
func (s *Store) Campaigns(ctx context.Context, product string) ([]Campaign, error) {
	rows, err := s.DB.Query(ctx, campaignSelect+` WHERE c.deleted_at IS NULL AND ($1 = '' OR c.product_id = $1) ORDER BY c.created_at DESC, c.id DESC`, product)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(r pgx.CollectableRow) (Campaign, error) { return scanCampaign(r) })
}

func (s *Store) Campaign(ctx context.Context, id int64) (Campaign, error) {
	return scanCampaign(s.DB.QueryRow(ctx, campaignSelect+` WHERE c.id = $1 AND c.deleted_at IS NULL`, id))
}

// NewCampaign is the row written before the platform call.
type NewCampaign struct {
	Platform         string
	ProductID        string
	AdAccountID      int64
	Name             string
	Countries        []string
	DailyBudgetPaise int64
	CreativeURL      string
	EndsOn           *string
}

func (s *Store) InsertCampaign(ctx context.Context, n NewCampaign) (int64, error) {
	var id int64
	err := pgx.BeginFunc(ctx, s.DB, func(tx pgx.Tx) error {
		if err := tx.QueryRow(ctx, `INSERT INTO campaigns (platform, product_id, ad_account_id, name, status, countries, daily_budget_paise,
			budget_paise, creative_url, starts_on, ends_on) VALUES ($1,$2,$3,$4,'draft',$5,$6,$6,$7,current_date,$8::date) RETURNING id`,
			n.Platform, n.ProductID, n.AdAccountID, n.Name, n.Countries, n.DailyBudgetPaise, n.CreativeURL, n.EndsOn).Scan(&id); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `UPDATE campaigns SET utm_campaign = 'ampli-c' || id WHERE id = $1`, id)
		return err
	})
	return id, err
}

// SetCampaignRemote stores the platform ids and the resulting status.
func (s *Store) SetCampaignRemote(ctx context.Context, id int64, ref ads.Ref, status, lastErr string) error {
	_, err := s.DB.Exec(ctx, `UPDATE campaigns SET external_id=$2, external_set_id=$3, status=$4, last_error=$5 WHERE id=$1`,
		id, ref.CampaignID, ref.SetID, status, lastErr)
	return err
}

func (s *Store) SetCampaignStatus(ctx context.Context, id int64, status, lastErr string) error {
	_, err := s.DB.Exec(ctx, `UPDATE campaigns SET status=$2, last_error=$3 WHERE id=$1`, id, status, lastErr)
	return err
}

func (s *Store) SetCampaignBudget(ctx context.Context, id int64, paise int64) error {
	_, err := s.DB.Exec(ctx, `UPDATE campaigns SET daily_budget_paise=$2, budget_paise=$2, last_error='' WHERE id=$1`, id, paise)
	return err
}

// SetAutoStrike records (or clears, with nil) the first below-limit sync.
func (s *Store) SetAutoStrike(ctx context.Context, id int64, at *time.Time) error {
	_, err := s.DB.Exec(ctx, `UPDATE campaigns SET auto_strike_at=$2 WHERE id=$1`, id, at)
	return err
}

// HoldAutoStop clears any strike and keeps the rule off until `until` (after a manual resume).
func (s *Store) HoldAutoStop(ctx context.Context, id int64, until time.Time) error {
	_, err := s.DB.Exec(ctx, `UPDATE campaigns SET auto_strike_at=NULL, auto_hold_until=$2 WHERE id=$1`, id, until)
	return err
}

// RecentRulePauses lists campaigns the auto-stop rule paused since `since`.
func (s *Store) RecentRulePauses(ctx context.Context, since time.Time) ([]LogEntry, error) {
	rows, err := s.DB.Query(ctx, `SELECT l.id, l.campaign_id, l.actor, l.action, c.name || ': ' || l.detail, l.created_at
		FROM campaign_log l JOIN campaigns c ON c.id = l.campaign_id
		WHERE l.actor='rule' AND l.action='paused' AND l.created_at >= $1 AND c.status='paused' ORDER BY l.created_at DESC`, since)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(r pgx.CollectableRow) (LogEntry, error) {
		var e LogEntry
		return e, r.Scan(&e.ID, &e.CampaignID, &e.Actor, &e.Action, &e.Detail, &e.CreatedAt)
	})
}

// DeleteCampaign hides a campaign; its numbers and log stay for the record.
func (s *Store) DeleteCampaign(ctx context.Context, id int64) error {
	tag, err := s.DB.Exec(ctx, `UPDATE campaigns SET deleted_at=now() WHERE id=$1 AND deleted_at IS NULL`, id)
	if err == nil && tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return err
}

// CampaignIDs maps platform campaign ids to ours for one ad account.
func (s *Store) CampaignIDs(ctx context.Context, accountID int64) (map[string]int64, error) {
	rows, err := s.DB.Query(ctx, `SELECT external_id, id FROM campaigns WHERE ad_account_id=$1 AND external_id <> ''`, accountID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	m := map[string]int64{}
	for rows.Next() {
		var ext string
		var id int64
		if err := rows.Scan(&ext, &id); err != nil {
			return nil, err
		}
		m[ext] = id
	}
	return m, rows.Err()
}

// SaveCampaignDays upserts daily numbers and refreshes the campaign totals.
func (s *Store) SaveCampaignDays(ctx context.Context, campaignID int64, days []ads.DayInsight) error {
	return pgx.BeginFunc(ctx, s.DB, func(tx pgx.Tx) error {
		for _, d := range days {
			if _, err := tx.Exec(ctx, `INSERT INTO campaign_days (campaign_id, day, spend_paise, impressions, clicks, installs)
				VALUES ($1,$2::date,$3,$4,$5,$6) ON CONFLICT (campaign_id, day) DO UPDATE SET spend_paise=EXCLUDED.spend_paise,
				impressions=EXCLUDED.impressions, clicks=EXCLUDED.clicks, installs=EXCLUDED.installs`,
				campaignID, d.Day, d.SpendPaise, d.Impressions, d.Clicks, d.Installs); err != nil {
				return err
			}
		}
		_, err := tx.Exec(ctx, `UPDATE campaigns c SET spend_paise=t.spend, impressions=t.imp, clicks=t.clk, installs=t.inst,
			synced_at=now(), status = CASE WHEN c.status='in_review' AND t.spend > 0 THEN 'live' ELSE c.status END
			FROM (SELECT COALESCE(sum(spend_paise),0) spend, COALESCE(sum(impressions),0) imp, COALESCE(sum(clicks),0) clk,
				COALESCE(sum(installs),0) inst FROM campaign_days WHERE campaign_id=$1) t WHERE c.id=$1`, campaignID)
		return err
	})
}

// ---- attribution and app signals (public endpoints) ----

type Install struct {
	ProductID, InstallID, AppVersion, Country, RawReferrer string
	Ref                                                    ads.Referrer
}

func (s *Store) SaveInstall(ctx context.Context, in Install) (bool, error) {
	tag, err := s.DB.Exec(ctx, `INSERT INTO installs (product_id, install_id, utm_source, utm_medium, utm_campaign, utm_content,
		ad_platform, referrer, app_version, country) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10) ON CONFLICT DO NOTHING`,
		in.ProductID, in.InstallID, in.Ref.Source, in.Ref.Medium, in.Ref.Campaign, in.Ref.Content, in.Ref.Platform,
		in.RawReferrer, in.AppVersion, in.Country)
	return err == nil && tag.RowsAffected() == 1, err
}

// Purchase is a purchase Google Play confirmed. AmountPaise is the list price, NetPaise what reaches the bank.
type Purchase struct {
	ProductID   string
	InstallID   string
	Kind        string
	SKU         string
	AmountPaise int64
	NetPaise    int64
	Currency    string
	OrderID     string
	TokenHash   string
	Region      string
	Verified    bool
}

// SavePurchase stores a purchase once per token. A row saved unverified (Google was down or not set up) is
// upgraded when the app retries and Google confirms it; a verified row is never changed. It reports whether
// the call inserted or upgraded a row.
func (s *Store) SavePurchase(ctx context.Context, p Purchase) (bool, error) {
	rows, err := s.DB.Query(ctx, `INSERT INTO purchases (product_id, install_id, kind, sku, amount_paise, net_paise, currency, order_id,
		token_hash, region, verified) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)
		ON CONFLICT (token_hash) WHERE token_hash IS NOT NULL DO UPDATE SET order_id=EXCLUDED.order_id, region=EXCLUDED.region,
			net_paise=EXCLUDED.net_paise, verified=EXCLUDED.verified
		WHERE NOT purchases.verified AND EXCLUDED.verified
		RETURNING id`,
		p.ProductID, p.InstallID, p.Kind, p.SKU, p.AmountPaise, p.NetPaise, p.Currency, p.OrderID, p.TokenHash, p.Region, p.Verified)
	if err != nil {
		return false, err
	}
	defer rows.Close()
	changed := rows.Next()
	return changed, rows.Err()
}

// HasPro reports whether Google confirmed a Pro purchase for this install.
func (s *Store) HasPro(ctx context.Context, product, installID string) (bool, error) {
	var ok bool
	err := s.DB.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM purchases WHERE product_id=$1 AND install_id=$2 AND kind='pro'
		AND verified AND refunded_at IS NULL)`, product, installID).Scan(&ok)
	return ok, err
}

// MarkRefunded flags voided orders so they drop out of revenue.
func (s *Store) MarkRefunded(ctx context.Context, orderID string, at time.Time) error {
	_, err := s.DB.Exec(ctx, `UPDATE purchases SET refunded_at=$2 WHERE order_id=$1 AND refunded_at IS NULL`, orderID, at)
	return err
}

type SKU struct {
	ProductID  string `json:"product"`
	SKU        string `json:"sku"`
	Kind       string `json:"kind"`
	PricePaise int64  `json:"price_paise"`
}

func (s *Store) SKU(ctx context.Context, product, sku string) (SKU, error) {
	k := SKU{ProductID: product, SKU: sku}
	err := s.DB.QueryRow(ctx, `SELECT kind, price_paise FROM skus WHERE product_id=$1 AND sku=$2`, product, sku).Scan(&k.Kind, &k.PricePaise)
	if errors.Is(err, pgx.ErrNoRows) {
		return k, ErrNotFound
	}
	return k, err
}

func (s *Store) SKUs(ctx context.Context) ([]SKU, error) {
	rows, err := s.DB.Query(ctx, `SELECT product_id, sku, kind, price_paise FROM skus ORDER BY product_id, sku`)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(r pgx.CollectableRow) (SKU, error) {
		var k SKU
		return k, r.Scan(&k.ProductID, &k.SKU, &k.Kind, &k.PricePaise)
	})
}

func (s *Store) SaveSKU(ctx context.Context, k SKU) error {
	_, err := s.DB.Exec(ctx, `INSERT INTO skus (product_id, sku, kind, price_paise) VALUES ($1,$2,$3,$4)
		ON CONFLICT (product_id, sku) DO UPDATE SET kind=EXCLUDED.kind, price_paise=EXCLUDED.price_paise`, k.ProductID, k.SKU, k.Kind, k.PricePaise)
	return err
}

func (s *Store) AddEvents(ctx context.Context, product string, day time.Time, names []string) error {
	return pgx.BeginFunc(ctx, s.DB, func(tx pgx.Tx) error {
		for _, n := range names {
			if _, err := tx.Exec(ctx, `INSERT INTO app_events (product_id, day, name, count) VALUES ($1,$2,$3,1)
				ON CONFLICT (product_id, day, name) DO UPDATE SET count = app_events.count + 1`, product, day, n); err != nil {
				return err
			}
		}
		return nil
	})
}

// IPHit counts one public call from a network prefix today and reports whether it is within limit.
func (s *Store) IPHit(ctx context.Context, key string, day time.Time, limit int) (bool, error) {
	var n int
	err := s.DB.QueryRow(ctx, `INSERT INTO ip_hits (ip_key, day, hits) VALUES ($1,$2,1)
		ON CONFLICT (ip_key, day) DO UPDATE SET hits = ip_hits.hits + 1 RETURNING hits`, key, day).Scan(&n)
	return n <= limit, err
}

// Cleanup drops rate-limit rows older than two days and raw referrers older than 90 days.
func (s *Store) Cleanup(ctx context.Context, today time.Time) error {
	if _, err := s.DB.Exec(ctx, `DELETE FROM public_hits WHERE day < $1::date - 2`, today); err != nil {
		return err
	}
	if _, err := s.DB.Exec(ctx, `DELETE FROM ip_hits WHERE day < $1::date - 2`, today); err != nil {
		return err
	}
	if _, err := s.DB.Exec(ctx, `DELETE FROM oauth_states_used WHERE used_at < $1::date - 2`, today); err != nil {
		return err
	}
	_, err := s.DB.Exec(ctx, `UPDATE installs SET referrer='', utm_content='' WHERE referrer <> '' AND created_at < $1::date - 90`, today)
	return err
}

// EraseInstall removes what one install sent (a user's deletion request). Purchases keep their money
// for the accounts but lose the link to the install.
func (s *Store) EraseInstall(ctx context.Context, installID string) (int64, error) {
	var n int64
	err := pgx.BeginFunc(ctx, s.DB, func(tx pgx.Tx) error {
		for _, q := range []string{
			`DELETE FROM installs WHERE install_id=$1`,
			`UPDATE tickets t SET votes = votes - 1 FROM feature_votes v WHERE v.ticket_id = t.id AND v.install_id=$1`,
			`DELETE FROM feature_votes WHERE install_id=$1`,
			`DELETE FROM public_hits WHERE install_id=$1`,
			`DELETE FROM tickets WHERE install_id=$1`,
			`UPDATE purchases SET install_id='erased' WHERE install_id=$1`,
			`UPDATE promo_codes SET claimed_install=NULL WHERE claimed_install=$1`, // claimed_at stays: the code is not given out again
		} {
			tag, err := tx.Exec(ctx, q, installID)
			if err != nil {
				return err
			}
			n += tag.RowsAffected()
		}
		return nil
	})
	return n, err
}

// Hit counts one public call for an install today and reports whether it is within limit.
func (s *Store) Hit(ctx context.Context, installID string, day time.Time, limit int) (bool, error) {
	var n int
	err := s.DB.QueryRow(ctx, `INSERT INTO public_hits (install_id, day, hits) VALUES ($1,$2,1)
		ON CONFLICT (install_id, day) DO UPDATE SET hits = public_hits.hits + 1 RETURNING hits`, installID, day).Scan(&n)
	return n <= limit, err
}

func oneOfCode(v string, opts ...string) bool {
	for _, o := range opts {
		if v == o {
			return true
		}
	}
	return false
}
