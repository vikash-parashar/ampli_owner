package store

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
)

// ---- auto-stop rules and the campaign log ----

func (s *Store) SetAutoStop(ctx context.Context, id int64, a AutoStop) error {
	tag, err := s.DB.Exec(ctx, `UPDATE campaigns SET auto_stop=$2, auto_min_roas=$3, auto_min_days=$4, auto_min_spend=$5, auto_strike_at=NULL WHERE id=$1`,
		id, a.Enabled, a.MinROAS, a.MinDays, a.MinSpendPaise)
	if err == nil && tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return err
}

type LogEntry struct {
	ID         int64     `json:"id"`
	CampaignID int64     `json:"campaign_id"`
	Actor      string    `json:"actor"`
	Action     string    `json:"action"`
	Detail     string    `json:"detail"`
	CreatedAt  time.Time `json:"created_at"`
}

func (s *Store) LogCampaign(ctx context.Context, id int64, actor, action, detail string) error {
	_, err := s.DB.Exec(ctx, `INSERT INTO campaign_log (campaign_id, actor, action, detail) VALUES ($1,$2,$3,$4)`, id, actor, action, detail)
	return err
}

func (s *Store) CampaignLog(ctx context.Context, id int64) ([]LogEntry, error) {
	rows, err := s.DB.Query(ctx, `SELECT id, campaign_id, actor, action, detail, created_at FROM campaign_log
		WHERE campaign_id=$1 ORDER BY created_at DESC, id DESC LIMIT 100`, id)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(r pgx.CollectableRow) (LogEntry, error) {
		var e LogEntry
		return e, r.Scan(&e.ID, &e.CampaignID, &e.Actor, &e.Action, &e.Detail, &e.CreatedAt)
	})
}

// ---- integrations (AdMob) ----

type Integration struct {
	Kind       string     `json:"kind"`
	Account    string     `json:"account"`
	LastSyncAt *time.Time `json:"last_sync_at"`
	LastError  string     `json:"last_error"`
}

func (s *Store) SaveIntegration(ctx context.Context, kind, account string, t SealedToken) error {
	_, err := s.DB.Exec(ctx, `INSERT INTO integrations (kind, account, token_enc, refresh_enc, expires_at) VALUES ($1,$2,$3,$4,$5)
		ON CONFLICT (kind) DO UPDATE SET account=EXCLUDED.account, token_enc=EXCLUDED.token_enc,
		refresh_enc=COALESCE(EXCLUDED.refresh_enc, integrations.refresh_enc), expires_at=EXCLUDED.expires_at, last_error=''`,
		kind, account, t.Access, t.Refresh, t.Expires)
	return err
}

// IntegrationToken returns ErrNotFound when the integration was never connected.
func (s *Store) IntegrationToken(ctx context.Context, kind string) (Integration, SealedToken, error) {
	var in Integration
	var t SealedToken
	err := s.DB.QueryRow(ctx, `SELECT kind, account, last_sync_at, last_error, token_enc, refresh_enc, expires_at FROM integrations WHERE kind=$1`, kind).
		Scan(&in.Kind, &in.Account, &in.LastSyncAt, &in.LastError, &t.Access, &t.Refresh, &t.Expires)
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && t.Access == nil) {
		return in, t, ErrNotFound
	}
	return in, t, err
}

func (s *Store) UpdateIntegrationToken(ctx context.Context, kind string, t SealedToken) error {
	_, err := s.DB.Exec(ctx, `UPDATE integrations SET token_enc=$2, expires_at=$3 WHERE kind=$1`, kind, t.Access, t.Expires)
	return err
}

// MarkIntegration records a sync result (creating the row for Play, which has no token).
func (s *Store) MarkIntegration(ctx context.Context, kind, lastErr string, synced bool) error {
	_, err := s.DB.Exec(ctx, `INSERT INTO integrations (kind, last_error, last_sync_at)
		VALUES ($1, $2, CASE WHEN $3 THEN now() END)
		ON CONFLICT (kind) DO UPDATE SET last_error=EXCLUDED.last_error,
		last_sync_at = CASE WHEN $3 THEN now() ELSE integrations.last_sync_at END`, kind, lastErr, synced)
	return err
}

// Integrations returns every integration row by kind; Connected is true when a token is stored.
func (s *Store) Integrations(ctx context.Context) (map[string]IntegrationState, error) {
	rows, err := s.DB.Query(ctx, `SELECT kind, account, last_sync_at, last_error, token_enc IS NOT NULL FROM integrations`)
	if err != nil {
		return nil, err
	}
	list, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (IntegrationState, error) {
		var in IntegrationState
		return in, r.Scan(&in.Kind, &in.Account, &in.LastSyncAt, &in.LastError, &in.Connected)
	})
	out := map[string]IntegrationState{}
	for _, in := range list {
		out[in.Kind] = in
	}
	return out, err
}

type IntegrationState struct {
	Integration
	Connected bool `json:"connected"`
}

func (s *Store) DeleteIntegration(ctx context.Context, kind string) error {
	_, err := s.DB.Exec(ctx, `UPDATE integrations SET token_enc=NULL, refresh_enc=NULL, expires_at=NULL, account='' WHERE kind=$1`, kind)
	return err
}

// SetAdRevenue writes one day's AdMob earnings without touching the hand-entered columns.
func (s *Store) SetAdRevenue(ctx context.Context, product string, day time.Time, paise int64) error {
	_, err := s.DB.Exec(ctx, `INSERT INTO daily_stats (product_id, day, ad_revenue_paise) VALUES ($1,$2,$3)
		ON CONFLICT (product_id, day) DO UPDATE SET ad_revenue_paise=EXCLUDED.ad_revenue_paise`, product, day, paise)
	return err
}

// ---- vitals ----

type Vitals struct {
	ProductID string   `json:"product"`
	Day       string   `json:"day"`
	CrashRate *float64 `json:"crash_rate"`
	ANRRate   *float64 `json:"anr_rate"`
}

func (s *Store) SaveVitals(ctx context.Context, product string, day time.Time, crash, anr *float64) error {
	_, err := s.DB.Exec(ctx, `INSERT INTO vitals (product_id, day, crash_rate, anr_rate) VALUES ($1,$2,$3,$4)
		ON CONFLICT (product_id, day) DO UPDATE SET crash_rate=COALESCE(EXCLUDED.crash_rate, vitals.crash_rate),
		anr_rate=COALESCE(EXCLUDED.anr_rate, vitals.anr_rate)`, product, day, crash, anr)
	return err
}

// LatestVitals returns each product's most recent vitals row.
func (s *Store) LatestVitals(ctx context.Context) ([]Vitals, error) {
	rows, err := s.DB.Query(ctx, `SELECT DISTINCT ON (product_id) product_id, to_char(day,'YYYY-MM-DD'), crash_rate, anr_rate
		FROM vitals ORDER BY product_id, day DESC`)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(r pgx.CollectableRow) (Vitals, error) {
		var v Vitals
		return v, r.Scan(&v.ProductID, &v.Day, &v.CrashRate, &v.ANRRate)
	})
}

// ---- alert inputs ----

type ReviewPulse struct {
	ProductID    string
	BadUnreplied int64    // 1-2 star reviews in the last 7 days with no reply
	AvgWeek      *float64 // average stars, last 7 days
	AvgBefore    *float64 // average stars, the 30 days before that
}

func (s *Store) ReviewPulses(ctx context.Context, now time.Time) ([]ReviewPulse, error) {
	rows, err := s.DB.Query(ctx, `SELECT p.id,
		(SELECT count(*) FROM reviews r WHERE r.product_id=p.id AND r.rating <= 2 AND r.reply='' AND r.created_at > $1::timestamptz - interval '7 days'),
		(SELECT avg(rating)::float8 FROM reviews r WHERE r.product_id=p.id AND r.created_at > $1::timestamptz - interval '7 days'),
		(SELECT avg(rating)::float8 FROM reviews r WHERE r.product_id=p.id AND r.created_at <= $1::timestamptz - interval '7 days'
			AND r.created_at > $1::timestamptz - interval '37 days')
		FROM products p ORDER BY p.id`, now)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(r pgx.CollectableRow) (ReviewPulse, error) {
		var p ReviewPulse
		return p, r.Scan(&p.ProductID, &p.BadUnreplied, &p.AvgWeek, &p.AvgBefore)
	})
}

func (s *Store) WaitingOnOwner(ctx context.Context) (int64, error) {
	var n int64
	err := s.DB.QueryRow(ctx, `SELECT count(*) FROM tickets WHERE status IN ('new','waiting_on_owner')`).Scan(&n)
	return n, err
}

// SetPlayStats writes one day from the Play Console reports without touching the money columns.
func (s *Store) SetPlayStats(ctx context.Context, product string, day time.Time, installs, uninstalls, active int64, rating *float64) error {
	_, err := s.DB.Exec(ctx, `INSERT INTO daily_stats (product_id, day, installs, uninstalls, active_users, rating) VALUES ($1,$2,$3,$4,$5,$6)
		ON CONFLICT (product_id, day) DO UPDATE SET installs=EXCLUDED.installs, uninstalls=EXCLUDED.uninstalls,
		active_users=EXCLUDED.active_users, rating=COALESCE(EXCLUDED.rating, daily_stats.rating)`,
		product, day, installs, uninstalls, active, rating)
	return err
}

// ---- owner audit log ----

type AuditEntry struct {
	ID        int64     `json:"id"`
	Action    string    `json:"action"`
	Detail    string    `json:"detail"`
	CreatedAt time.Time `json:"created_at"`
}

func (s *Store) Audit(ctx context.Context, action, detail string) error {
	_, err := s.DB.Exec(ctx, `INSERT INTO audit_log (action, detail) VALUES ($1,$2)`, action, detail)
	return err
}

func (s *Store) AuditLog(ctx context.Context) ([]AuditEntry, error) {
	rows, err := s.DB.Query(ctx, `SELECT id, action, detail, created_at FROM audit_log ORDER BY id DESC LIMIT 200`)
	if err != nil {
		return nil, err
	}
	out, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (AuditEntry, error) {
		var e AuditEntry
		return e, r.Scan(&e.ID, &e.Action, &e.Detail, &e.CreatedAt)
	})
	if out == nil {
		out = []AuditEntry{}
	}
	return out, err
}

// ExpiringAccounts lists connected ad accounts whose token expires before `before`.
func (s *Store) ExpiringAccounts(ctx context.Context, before time.Time) ([]AdAccount, error) {
	rows, err := s.DB.Query(ctx, `SELECT `+accountCols+` FROM ad_accounts WHERE status='connected' AND expires_at IS NOT NULL AND expires_at < $1`, before)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(r pgx.CollectableRow) (AdAccount, error) { return scanAccount(r) })
}

// UseState records an OAuth state as used and reports false when it was used before (a replayed callback).
func (s *Store) UseState(ctx context.Context, stateHash string) (bool, error) {
	tag, err := s.DB.Exec(ctx, `INSERT INTO oauth_states_used (state_hash) VALUES ($1) ON CONFLICT DO NOTHING`, stateHash)
	return err == nil && tag.RowsAffected() == 1, err
}

// Unchecked is what the alert says about purchases still waiting for Google's confirmation.
type Unchecked struct {
	Count   int64
	Reasons []string // distinct verify errors, most common first
}

// UncheckedPurchases looks at purchases saved between since and before that Google has not confirmed yet.
// Tester, promo-code and refused rows are not waiting for anything.
func (s *Store) UncheckedPurchases(ctx context.Context, since, before time.Time) (Unchecked, error) {
	var u Unchecked
	rows, err := s.DB.Query(ctx, `SELECT verify_error, count(*) FROM purchases WHERE NOT verified
		AND region NOT IN ('TEST','PROMO','REFUSED') AND refunded_at IS NULL AND created_at >= $1 AND created_at < $2
		GROUP BY 1 ORDER BY 2 DESC`, since, before)
	if err != nil {
		return u, err
	}
	defer rows.Close()
	for rows.Next() {
		var why string
		var n int64
		if err := rows.Scan(&why, &n); err != nil {
			return u, err
		}
		u.Count += n
		if why != "" {
			u.Reasons = append(u.Reasons, why)
		}
	}
	return u, rows.Err()
}

// Pending is an unconfirmed purchase the sync can re-check with Google.
type Pending struct {
	ID int64
	Purchase
}

func (s *Store) PendingPurchases(ctx context.Context, since time.Time, limit int) ([]Pending, error) {
	rows, err := s.DB.Query(ctx, `SELECT id, product_id, install_id, kind, sku, amount_paise, token_hash, token_enc FROM purchases
		WHERE NOT verified AND token_enc IS NOT NULL AND refunded_at IS NULL AND created_at >= $1 ORDER BY id LIMIT $2`, since, limit)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(r pgx.CollectableRow) (Pending, error) {
		var p Pending
		return p, r.Scan(&p.ID, &p.ProductID, &p.InstallID, &p.Kind, &p.SKU, &p.AmountPaise, &p.TokenHash, &p.TokenEnc)
	})
}

// ResolvePending writes Google's answer for a re-checked purchase. A confirmed, tester, promo or refused one
// drops its token; one still waiting keeps it and records why.
func (s *Store) ResolvePending(ctx context.Context, id int64, p Purchase, done bool) error {
	if !done {
		_, err := s.DB.Exec(ctx, `UPDATE purchases SET verify_error=$2 WHERE id=$1`, id, p.VerifyError)
		return err
	}
	_, err := s.DB.Exec(ctx, `UPDATE purchases SET order_id=$2, region=$3, amount_paise=$4, net_paise=$5, verified=$6,
		token_enc=NULL, verify_error='' WHERE id=$1 AND NOT verified`, id, p.OrderID, p.Region, p.AmountPaise, p.NetPaise, p.Verified)
	return err
}

// SetExactNet replaces estimated net amounts with Google's earnings-report figures, per order id.
func (s *Store) SetExactNet(ctx context.Context, net map[string]int64) (int64, error) {
	if len(net) == 0 {
		return 0, nil
	}
	orders, amounts := make([]string, 0, len(net)), make([]int64, 0, len(net))
	for o, v := range net {
		orders, amounts = append(orders, o), append(amounts, max(v, 0))
	}
	tag, err := s.DB.Exec(ctx, `UPDATE purchases p SET net_paise=v.net, net_exact=true
		FROM unnest($1::text[], $2::bigint[]) AS v(o, net)
		WHERE p.order_id=v.o AND p.verified AND (NOT p.net_exact OR p.net_paise <> v.net)`, orders, amounts)
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}
