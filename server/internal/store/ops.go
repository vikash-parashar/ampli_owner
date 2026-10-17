package store

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
)

// ---- auto-stop rules and the campaign log ----

func (s *Store) SetAutoStop(ctx context.Context, id int64, a AutoStop) error {
	tag, err := s.DB.Exec(ctx, `UPDATE campaigns SET auto_stop=$2, auto_min_roas=$3, auto_min_days=$4, auto_min_spend=$5 WHERE id=$1`,
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
