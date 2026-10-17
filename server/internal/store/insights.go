package store

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

// ---- reviews ----

type Review struct {
	ID         int64      `json:"id"`
	ProductID  string     `json:"product"`
	ExternalID *string    `json:"-"`
	Source     string     `json:"source"`
	Author     string     `json:"author"`
	Rating     int16      `json:"rating"`
	Body       string     `json:"body"`
	Sentiment  string     `json:"sentiment"`
	Reply      string     `json:"reply"`
	RepliedAt  *time.Time `json:"replied_at"`
	AppVersion string     `json:"app_version"`
	CreatedAt  time.Time  `json:"created_at"`
}

const reviewCols = `id, product_id, external_id, source, author, rating, body, sentiment, reply, replied_at, app_version, created_at`

func scanReview(r pgx.Row) (Review, error) {
	var v Review
	err := r.Scan(&v.ID, &v.ProductID, &v.ExternalID, &v.Source, &v.Author, &v.Rating, &v.Body, &v.Sentiment, &v.Reply,
		&v.RepliedAt, &v.AppVersion, &v.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return v, ErrNotFound
	}
	return v, err
}

// SaveReview inserts a review; a Play review with a known external id is updated in place.
func (s *Store) SaveReview(ctx context.Context, v Review) (Review, error) {
	if v.CreatedAt.IsZero() {
		v.CreatedAt = time.Now()
	}
	return scanReview(s.DB.QueryRow(ctx, `INSERT INTO reviews (product_id, external_id, source, author, rating, body, reply, app_version, created_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)
		ON CONFLICT (product_id, external_id) DO UPDATE SET rating=EXCLUDED.rating, body=EXCLUDED.body, author=EXCLUDED.author,
			reply = CASE WHEN EXCLUDED.reply <> '' THEN EXCLUDED.reply ELSE reviews.reply END
		RETURNING `+reviewCols, v.ProductID, v.ExternalID, v.Source, v.Author, v.Rating, v.Body, v.Reply, v.AppVersion, v.CreatedAt))
}

func (s *Store) Review(ctx context.Context, id int64) (Review, error) {
	return scanReview(s.DB.QueryRow(ctx, `SELECT `+reviewCols+` FROM reviews WHERE id=$1`, id))
}

func (s *Store) Reviews(ctx context.Context, product string, limit int) ([]Review, error) {
	rows, err := s.DB.Query(ctx, `SELECT `+reviewCols+` FROM reviews WHERE ($1 = '' OR product_id = $1)
		ORDER BY created_at DESC, id DESC LIMIT $2`, product, limit)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(r pgx.CollectableRow) (Review, error) { return scanReview(r) })
}

func (s *Store) SetReviewReply(ctx context.Context, id int64, text string) (Review, error) {
	return scanReview(s.DB.QueryRow(ctx, `UPDATE reviews SET reply=$2, replied_at=now() WHERE id=$1 RETURNING `+reviewCols, id, text))
}

// Stars returns review counts for 5,4,3,2,1 stars.
func (s *Store) Stars(ctx context.Context, product string) ([5]int64, error) {
	var st [5]int64
	rows, err := s.DB.Query(ctx, `SELECT rating, count(*) FROM reviews WHERE ($1 = '' OR product_id = $1) GROUP BY rating`, product)
	if err != nil {
		return st, err
	}
	defer rows.Close()
	for rows.Next() {
		var r int16
		var n int64
		if err := rows.Scan(&r, &n); err != nil {
			return st, err
		}
		if r >= 1 && r <= 5 {
			st[5-r] = n
		}
	}
	return st, rows.Err()
}

// Sentiment turns star counts into positive (4-5), mixed (3) and negative (1-2) percentages.
func Sentiment(st [5]int64) map[string]float64 {
	total := st[0] + st[1] + st[2] + st[3] + st[4]
	out := map[string]float64{"positive": 0, "mixed": 0, "negative": 0}
	if total == 0 {
		return out
	}
	pct := func(n int64) float64 { return float64(int(float64(n)*1000/float64(total)+0.5)) / 10 }
	out["positive"], out["mixed"], out["negative"] = pct(st[0]+st[1]), pct(st[2]), pct(st[3]+st[4])
	return out
}

// Theme is a topic people mention in reviews.
type Theme struct {
	Label string `json:"label"`
	Count int    `json:"count"`
}

var themeWords = []struct {
	label string
	words []string
}{
	{"Too many ads", []string{"ads", "advert", "ad after", "विज्ञापन"}},
	{"Price", []string{"price", "expensive", "cheaper", "costly", "₹", "paisa", "mehnga", "mahanga"}},
	{"Crashes or bugs", []string{"crash", "bug", "not working", "band ho", "hang", "freeze", "stuck", "error"}},
	{"Battery", []string{"battery", "drain"}},
	{"Voice", []string{"voice", "awaaz", "aawaz", "bunty", "simran", "hey ampli"}},
	{"Haryanvi", []string{"haryanvi", "haryana"}},
	{"Record player", []string{"record", "turntable", "vinyl"}},
	{"Sound and EQ", []string{"equalizer", "eq", "bass", "sound quality", "loud"}},
	{"No subscription", []string{"no monthly", "one time", "one-time", "lifetime", "no subscription", "pay once"}},
	{"Job alerts", []string{"alert", "notification", "bharti", "vacancy", "naukri", "job"}},
	{"Login or OTP", []string{"otp", "login", "sign in", "signin"}},
	{"Lyrics", []string{"lyrics"}},
	{"Design", []string{"beautiful", "design", "ui", "look", "sundar"}},
	{"Navigation", []string{"navigation", "route", "map", "rasta"}},
	{"Easy to use", []string{"easy", "simple", "smooth", "aasan"}},
}

// Themes counts topics separately for praise (4-5 stars) and complaints (1-3 stars) over recent reviews.
func Themes(rs []Review) (praise, complaints []Theme) {
	pc, cc := map[string]int{}, map[string]int{}
	for _, r := range rs {
		body := " " + strings.ToLower(r.Body) + " "
		for _, t := range themeWords {
			for _, w := range t.words {
				if hasWord(body, w) {
					if r.Rating >= 4 {
						pc[t.label]++
					} else {
						cc[t.label]++
					}
					break
				}
			}
		}
	}
	return topThemes(pc), topThemes(cc)
}

func hasWord(body, w string) bool {
	if len(w) > 3 {
		return strings.Contains(body, w)
	}
	// Short words ("ads", "eq", "ui") must stand alone.
	for _, sep := range []string{" ", ".", ",", "!", "?"} {
		for _, end := range []string{" ", ".", ",", "!", "?"} {
			if strings.Contains(body, sep+w+end) {
				return true
			}
		}
	}
	return false
}

func topThemes(m map[string]int) []Theme {
	out := make([]Theme, 0, len(m))
	for k, v := range m {
		out = append(out, Theme{k, v})
	}
	for i := 1; i < len(out); i++ {
		for j := i; j > 0 && (out[j].Count > out[j-1].Count || (out[j].Count == out[j-1].Count && out[j].Label < out[j-1].Label)); j-- {
			out[j], out[j-1] = out[j-1], out[j]
		}
	}
	if len(out) > 6 {
		out = out[:6]
	}
	return out
}

// ---- dashboard ----

// DayRow is one day of a product's money and installs, merging hand-entered daily_stats
// with purchases and installs the app reported (whichever is larger, so nothing counts twice).
type DayRow struct {
	Day        time.Time
	Pro        int64
	Ad         int64
	Tips       int64
	Other      int64
	ProSales   int64
	Installs   int64
	AdInstalls int64
}

func (d DayRow) Revenue() int64 { return d.Pro + d.Ad + d.Tips + d.Other }

func (s *Store) DayRows(ctx context.Context, product string) ([]DayRow, error) {
	rows, err := s.DB.Query(ctx, `
WITH ds AS (
  SELECT day, sum(pro_revenue_paise) pro, sum(ad_revenue_paise) ad, sum(tips_paise) tips, sum(other_revenue_paise) other,
         sum(pro_sales) sales, sum(installs) inst
  FROM daily_stats WHERE product_id = $1 GROUP BY day
), pu AS (
  SELECT (created_at AT TIME ZONE 'Asia/Kolkata')::date AS day,
         COALESCE(sum(amount_paise) FILTER (WHERE kind IN ('pro','subscription')), 0) pro,
         COALESCE(sum(amount_paise) FILTER (WHERE kind = 'tip'), 0) tips,
         COALESCE(sum(amount_paise) FILTER (WHERE kind = 'other'), 0) other,
         count(*) FILTER (WHERE kind = 'pro') sales
  FROM purchases WHERE product_id = $1 GROUP BY 1
), ins AS (
  SELECT (created_at AT TIME ZONE 'Asia/Kolkata')::date AS day, count(*) n,
         count(*) FILTER (WHERE ad_platform <> '' OR utm_campaign LIKE 'ampli-c%') ads
  FROM installs WHERE product_id = $1 GROUP BY 1
), d AS (SELECT day FROM ds UNION SELECT day FROM pu UNION SELECT day FROM ins)
SELECT d.day,
  GREATEST(COALESCE(ds.pro, 0), COALESCE(pu.pro, 0))::bigint,
  COALESCE(ds.ad, 0)::bigint,
  GREATEST(COALESCE(ds.tips, 0), COALESCE(pu.tips, 0))::bigint,
  GREATEST(COALESCE(ds.other, 0), COALESCE(pu.other, 0))::bigint,
  GREATEST(COALESCE(ds.sales, 0), COALESCE(pu.sales, 0))::bigint,
  GREATEST(COALESCE(ds.inst, 0), COALESCE(ins.n, 0))::bigint,
  COALESCE(ins.ads, 0)::bigint
FROM d LEFT JOIN ds USING (day) LEFT JOIN pu USING (day) LEFT JOIN ins USING (day) ORDER BY d.day`, product)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(r pgx.CollectableRow) (DayRow, error) {
		var d DayRow
		return d, r.Scan(&d.Day, &d.Pro, &d.Ad, &d.Tips, &d.Other, &d.ProSales, &d.Installs, &d.AdInstalls)
	})
}

// ActiveUsers returns the latest active-user count and the one about 30 days before it.
func (s *Store) ActiveUsers(ctx context.Context, product string, today time.Time) (now, before int64, err error) {
	err = s.DB.QueryRow(ctx, `SELECT
		COALESCE((SELECT active_users FROM daily_stats WHERE product_id=$1 AND active_users > 0 AND day <= $2 ORDER BY day DESC LIMIT 1), 0),
		COALESCE((SELECT active_users FROM daily_stats WHERE product_id=$1 AND active_users > 0 AND day <= $2::date - 30 ORDER BY day DESC LIMIT 1), 0)`,
		product, today).Scan(&now, &before)
	return
}

// Rating returns the latest Play rating typed in daily_stats, else the review average, and the review count.
func (s *Store) Rating(ctx context.Context, product string) (*float64, int64, error) {
	var rating *float64
	var reviews, typed int64
	err := s.DB.QueryRow(ctx, `SELECT
		COALESCE((SELECT rating::float8 FROM daily_stats WHERE product_id=$1 AND rating IS NOT NULL ORDER BY day DESC LIMIT 1),
		         (SELECT round(avg(rating), 1)::float8 FROM reviews WHERE product_id=$1)),
		(SELECT count(*) FROM reviews WHERE product_id=$1),
		COALESCE((SELECT reviews FROM daily_stats WHERE product_id=$1 AND reviews > 0 ORDER BY day DESC LIMIT 1), 0)`,
		product).Scan(&rating, &reviews, &typed)
	if typed > reviews {
		reviews = typed
	}
	return rating, reviews, err
}

// EventCounts sums app event counters over the last 30 days.
func (s *Store) EventCounts(ctx context.Context, product string, today time.Time) (map[string]int64, error) {
	rows, err := s.DB.Query(ctx, `SELECT name, sum(count)::bigint FROM app_events WHERE product_id=$1 AND day > $2::date - 30
		GROUP BY name`, product, today)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	m := map[string]int64{}
	for rows.Next() {
		var n string
		var c int64
		if err := rows.Scan(&n, &c); err != nil {
			return nil, err
		}
		m[n] = c
	}
	return m, rows.Err()
}

// Product is one row of the products table.
type Product struct {
	ID, Name, PackageID string
}

func (s *Store) Products(ctx context.Context) ([]Product, error) {
	rows, err := s.DB.Query(ctx, `SELECT id, name, package_id FROM products ORDER BY CASE id WHEN 'mp3' THEN 1 WHEN 'carhub' THEN 2 ELSE 3 END`)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(r pgx.CollectableRow) (Product, error) {
		var p Product
		return p, r.Scan(&p.ID, &p.Name, &p.PackageID)
	})
}
