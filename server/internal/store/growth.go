package store

import (
	"context"
	"errors"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"
)

// ---- tracked links ----

// Link is a Play link for one app and channel. Its installs carry utm_campaign "ampli-l<ID>".
type Link struct {
	ID          int64     `json:"id"`
	ProductID   string    `json:"product"`
	PackageID   string    `json:"-"`
	Channel     string    `json:"channel"`
	Label       string    `json:"label"`
	Listing     string    `json:"listing"`
	FromProduct string    `json:"from_product"`
	UTMCampaign string    `json:"utm_campaign"`
	CreatedAt   time.Time `json:"created_at"`
	Installs    int64     `json:"installs"`
	Installs30  int64     `json:"installs_30d"`
	ProSales    int64     `json:"pro_sales"`
	NetPaise    int64     `json:"net_paise"`
}

// LinkCampaign is the utm_campaign of a tracked link.
func LinkCampaign(id int64) string { return "ampli-l" + strconv.FormatInt(id, 10) }

func (s *Store) CreateLink(ctx context.Context, l Link) (Link, error) {
	err := s.DB.QueryRow(ctx, `INSERT INTO tracked_links (product_id, channel, label, listing, from_product)
		VALUES ($1,$2,$3,$4,$5) RETURNING id, created_at`, l.ProductID, l.Channel, l.Label, l.Listing, l.FromProduct).
		Scan(&l.ID, &l.CreatedAt)
	l.UTMCampaign = LinkCampaign(l.ID)
	return l, err
}

// Links lists live links with their installs and the verified money those installs brought.
func (s *Store) Links(ctx context.Context, product string, since time.Time) ([]Link, error) {
	rows, err := s.DB.Query(ctx, `
		WITH inst AS (
		  SELECT i.product_id, i.install_id, i.created_at,
		    CASE WHEN i.utm_campaign ~ '^ampli-l[0-9]{1,18}$' THEN substr(i.utm_campaign, 8)::bigint END AS link_id
		  FROM installs i WHERE i.utm_campaign LIKE 'ampli-l%'),
		per AS (
		  SELECT link_id, count(*) n, count(*) FILTER (WHERE created_at >= $2) n30 FROM inst GROUP BY link_id),
		money AS (
		  SELECT inst.link_id, count(*) FILTER (WHERE p.kind='pro') pro, COALESCE(sum(p.net_paise),0) net
		  FROM inst JOIN purchases p ON p.product_id=inst.product_id AND p.install_id=inst.install_id
		  WHERE p.verified AND p.refunded_at IS NULL GROUP BY inst.link_id)
		SELECT l.id, l.product_id, pr.package_id, l.channel, l.label, l.listing, l.from_product, l.created_at,
		  COALESCE(per.n,0), COALESCE(per.n30,0), COALESCE(money.pro,0), COALESCE(money.net,0)
		FROM tracked_links l JOIN products pr ON pr.id=l.product_id
		LEFT JOIN per ON per.link_id=l.id LEFT JOIN money ON money.link_id=l.id
		WHERE l.deleted_at IS NULL AND ($1='' OR l.product_id=$1)
		ORDER BY COALESCE(per.n30,0) DESC, l.id DESC`, product, since)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(r pgx.CollectableRow) (Link, error) {
		var l Link
		err := r.Scan(&l.ID, &l.ProductID, &l.PackageID, &l.Channel, &l.Label, &l.Listing, &l.FromProduct, &l.CreatedAt,
			&l.Installs, &l.Installs30, &l.ProSales, &l.NetPaise)
		l.UTMCampaign = LinkCampaign(l.ID)
		return l, err
	})
}

// DeleteLink hides a link; installs that already came through it keep counting under its channel.
func (s *Store) DeleteLink(ctx context.Context, id int64) error {
	tag, err := s.DB.Exec(ctx, `UPDATE tracked_links SET deleted_at=now() WHERE id=$1 AND deleted_at IS NULL`, id)
	if err == nil && tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return err
}

// ChannelInstalls counts one app's installs since a time by where they came from: a tracked link's channel,
// a known channel name in utm_source (apps' own cross-promo links), an ad platform or our ad link, Play's own
// organic traffic, or other.
func (s *Store) ChannelInstalls(ctx context.Context, product string, since time.Time) (map[string]int64, error) {
	rows, err := s.DB.Query(ctx, `
		SELECT CASE
		  WHEN l.channel IS NOT NULL THEN l.channel
		  WHEN i.utm_source IN ('play_hindi','website','whatsapp','telegram','instagram','youtube','crosspromo') THEN i.utm_source
		  WHEN i.ad_platform <> '' THEN 'ads_' || i.ad_platform
		  WHEN i.utm_campaign LIKE 'ampli-c%' THEN 'ads_link'
		  WHEN i.utm_source IN ('google-play','google_play') AND i.utm_medium IN ('organic','') THEN 'play'
		  ELSE 'unknown' END AS ch, count(*)
		FROM installs i
		LEFT JOIN tracked_links l ON l.id = CASE WHEN i.utm_campaign ~ '^ampli-l[0-9]{1,18}$' THEN substr(i.utm_campaign, 8)::bigint END
		WHERE i.product_id=$1 AND i.created_at >= $2 GROUP BY 1`, product, since)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]int64{}
	for rows.Next() {
		var k string
		var n int64
		if err := rows.Scan(&k, &n); err != nil {
			return nil, err
		}
		out[k] = n
	}
	return out, rows.Err()
}

// ---- cross-promo codes ----

// ImportPromoCodes adds Play promo codes one app hands to another app's Pro buyers; duplicates are skipped.
func (s *Store) ImportPromoCodes(ctx context.Context, source, target, sku string, codes []string) (int64, error) {
	tag, err := s.DB.Exec(ctx, `INSERT INTO promo_codes (source_product, target_product, sku, code)
		SELECT $1, $2, $3, c FROM unnest($4::text[]) c ON CONFLICT (code) DO NOTHING`, source, target, sku, codes)
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}

type PromoCode struct {
	Code   string `json:"code"`
	Target string `json:"target_product"`
	SKU    string `json:"sku"`
}

// ErrNoCodes means every imported code is already given out.
var ErrNoCodes = errors.New("no promo codes left")

// ClaimPromo gives an install one code, the same one on every call.
func (s *Store) ClaimPromo(ctx context.Context, source, installID string, at time.Time) (PromoCode, error) {
	var c PromoCode
	err := s.DB.QueryRow(ctx, `SELECT code, target_product, sku FROM promo_codes WHERE source_product=$1 AND claimed_install=$2
		ORDER BY id LIMIT 1`, source, installID).Scan(&c.Code, &c.Target, &c.SKU)
	if err == nil {
		return c, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return c, err
	}
	err = s.DB.QueryRow(ctx, `UPDATE promo_codes SET claimed_install=$2, claimed_at=$3 WHERE id = (
		  SELECT id FROM promo_codes WHERE source_product=$1 AND claimed_at IS NULL ORDER BY id LIMIT 1 FOR UPDATE SKIP LOCKED)
		RETURNING code, target_product, sku`, source, installID, at).Scan(&c.Code, &c.Target, &c.SKU)
	if errors.Is(err, pgx.ErrNoRows) {
		return c, ErrNoCodes
	}
	return c, err
}

// PromoReport is one cross-promo pair: who could claim, who did, and how many redeemed in the target app.
type PromoReport struct {
	Source    string `json:"source_product"`
	Target    string `json:"target_product"`
	SKU       string `json:"sku"`
	Imported  int64  `json:"imported"`
	Available int64  `json:"available"`
	Eligible  int64  `json:"eligible"` // verified, unrefunded Pro buyers of the source app
	Claimed   int64  `json:"claimed"`
	Redeemed  int64  `json:"redeemed"`        // promo-code purchases of the sku in the target app since the first claim
	Refunded  int64  `json:"refunded_claims"` // claimers whose source-app Pro was refunded afterwards (they kept the code)
}

// ProBoughtAt is when this install's first confirmed, unrefunded Pro purchase was made (ErrNotFound if none).
func (s *Store) ProBoughtAt(ctx context.Context, product, installID string) (time.Time, error) {
	var at *time.Time
	err := s.DB.QueryRow(ctx, `SELECT min(created_at) FROM purchases WHERE product_id=$1 AND install_id=$2 AND kind='pro'
		AND verified AND refunded_at IS NULL`, product, installID).Scan(&at)
	if err == nil && at == nil {
		return time.Time{}, ErrNotFound
	}
	if err != nil {
		return time.Time{}, err
	}
	return *at, nil
}

func (s *Store) PromoReports(ctx context.Context) ([]PromoReport, error) {
	rows, err := s.DB.Query(ctx, `
		SELECT c.source_product, c.target_product, c.sku, count(*), count(*) FILTER (WHERE c.claimed_at IS NULL),
		  (SELECT count(DISTINCT install_id) FROM purchases p WHERE p.product_id=c.source_product AND p.kind='pro'
		     AND p.verified AND p.refunded_at IS NULL),
		  count(*) FILTER (WHERE c.claimed_at IS NOT NULL),
		  (SELECT count(*) FROM purchases p WHERE p.product_id=c.target_product AND p.sku=c.sku AND p.region='PROMO'
		     AND p.created_at >= COALESCE(min(c.claimed_at), 'infinity')),
		  count(*) FILTER (WHERE c.claimed_install IS NOT NULL
		    AND EXISTS (SELECT 1 FROM purchases p WHERE p.product_id=c.source_product AND p.install_id=c.claimed_install
		      AND p.kind='pro' AND p.refunded_at IS NOT NULL)
		    AND NOT EXISTS (SELECT 1 FROM purchases p WHERE p.product_id=c.source_product AND p.install_id=c.claimed_install
		      AND p.kind='pro' AND p.verified AND p.refunded_at IS NULL))
		FROM promo_codes c GROUP BY 1,2,3 ORDER BY 1,2,3`)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(r pgx.CollectableRow) (PromoReport, error) {
		var p PromoReport
		return p, r.Scan(&p.Source, &p.Target, &p.SKU, &p.Imported, &p.Available, &p.Eligible, &p.Claimed, &p.Redeemed, &p.Refunded)
	})
}
