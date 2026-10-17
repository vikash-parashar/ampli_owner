package play

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/csv"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
	"unicode/utf16"
)

// Purchase is Google's record of a one-time product purchase.
type Purchase struct {
	State      int // 0 purchased, 1 cancelled, 2 pending
	OrderID    string
	RegionCode string
	Time       time.Time
	Test       bool // not money: license tester, promo code or rewarded
	Promo      bool // redeemed with a Play promo code (purchaseType 1)
	Quantity   int64
}

// ProductPurchase checks a purchase token with Google Play (purchases.products.get).
func (c *Client) ProductPurchase(ctx context.Context, pkg, sku, token string) (Purchase, error) {
	tok, err := c.accessToken(ctx)
	if err != nil {
		return Purchase{}, err
	}
	var res struct {
		State      int    `json:"purchaseState"`
		OrderID    string `json:"orderId"`
		RegionCode string `json:"regionCode"`
		TimeMillis string `json:"purchaseTimeMillis"`
		Type       *int   `json:"purchaseType"`
		Quantity   int64  `json:"quantity"`
	}
	u := fmt.Sprintf("%s/applications/%s/purchases/products/%s/tokens/%s", c.Base, url.PathEscape(pkg), url.PathEscape(sku), url.PathEscape(token))
	if err := c.do(ctx, "GET", u, tok, nil, &res); err != nil {
		return Purchase{}, err
	}
	p := Purchase{State: res.State, OrderID: res.OrderID, RegionCode: res.RegionCode, Test: res.Type != nil, Promo: res.Type != nil && *res.Type == 1, Quantity: max(res.Quantity, 1)}
	if ms, err := strconv.ParseInt(res.TimeMillis, 10, 64); err == nil {
		p.Time = time.UnixMilli(ms)
	}
	return p, nil
}

// Voided is a refunded, charged-back or revoked purchase.
type Voided struct {
	OrderID string
	At      time.Time
}

// VoidedPurchases lists purchases voided since `since` (Google keeps 30 days).
func (c *Client) VoidedPurchases(ctx context.Context, pkg string, since time.Time) ([]Voided, error) {
	tok, err := c.accessToken(ctx)
	if err != nil {
		return nil, err
	}
	var out []Voided
	next := ""
	for page := 0; page < 20; page++ {
		q := url.Values{"startTime": {strconv.FormatInt(since.UnixMilli(), 10)}, "maxResults": {"1000"}}
		if next != "" {
			q.Set("token", next)
		}
		var res struct {
			List []struct {
				OrderID string `json:"orderId"`
				Millis  string `json:"voidedTimeMillis"`
			} `json:"voidedPurchases"`
			Page struct {
				Next string `json:"nextPageToken"`
			} `json:"tokenPagination"`
		}
		if err := c.do(ctx, "GET", c.Base+"/applications/"+url.PathEscape(pkg)+"/purchases/voidedpurchases?"+q.Encode(), tok, nil, &res); err != nil {
			return nil, err
		}
		for _, v := range res.List {
			ms, _ := strconv.ParseInt(v.Millis, 10, 64)
			out = append(out, Voided{OrderID: v.OrderID, At: time.UnixMilli(ms)})
		}
		if res.Page.Next == "" {
			break
		}
		next = res.Page.Next
	}
	return out, nil
}

// VitalsDay is one day of 28-day user-weighted crash and ANR rates.
type VitalsDay struct {
	Day   time.Time
	Crash *float64
	ANR   *float64
}

func gdate(t time.Time) map[string]any {
	return map[string]any{"year": t.Year(), "month": int(t.Month()), "day": t.Day(), "timeZone": map[string]string{"id": "America/Los_Angeles"}}
}

// Vitals queries the Play Developer Reporting API for the last days of crash and ANR rates.
func (c *Client) Vitals(ctx context.Context, pkg string, from, to time.Time) ([]VitalsDay, error) {
	tok, err := c.accessToken(ctx)
	if err != nil {
		return nil, err
	}
	byDay := map[string]*VitalsDay{}
	for _, m := range []struct{ set, metric string }{
		{"crashRateMetricSet", "userPerceivedCrashRate28dUserWeighted"},
		{"anrRateMetricSet", "userPerceivedAnrRate28dUserWeighted"},
	} {
		body := map[string]any{"timelineSpec": map[string]any{"aggregationPeriod": "DAILY", "startTime": gdate(from), "endTime": gdate(to)},
			"metrics": []string{m.metric}, "pageSize": 100}
		var res struct {
			Rows []struct {
				Start struct {
					Year, Month, Day int
				} `json:"startTime"`
				Metrics []struct {
					Metric  string `json:"metric"`
					Decimal struct {
						Value string `json:"value"`
					} `json:"decimalValue"`
				} `json:"metrics"`
			} `json:"rows"`
		}
		if err := c.do(ctx, "POST", c.Report+"/apps/"+url.PathEscape(pkg)+"/"+m.set+":query", tok, body, &res); err != nil {
			return nil, err
		}
		for _, r := range res.Rows {
			d := time.Date(r.Start.Year, time.Month(r.Start.Month), r.Start.Day, 0, 0, 0, 0, time.UTC)
			k := d.Format(time.DateOnly)
			if byDay[k] == nil {
				byDay[k] = &VitalsDay{Day: d}
			}
			for _, mv := range r.Metrics {
				f, err := strconv.ParseFloat(mv.Decimal.Value, 64)
				if err != nil {
					continue
				}
				if m.set == "crashRateMetricSet" {
					byDay[k].Crash = &f
				} else {
					byDay[k].ANR = &f
				}
			}
		}
	}
	out := make([]VitalsDay, 0, len(byDay))
	for _, v := range byDay {
		out = append(out, *v)
	}
	return out, nil
}

// StatsDay is one day from the Play Console bulk reports (installs overview + ratings overview).
type StatsDay struct {
	Day         time.Time
	Installs    int64
	Uninstalls  int64
	Active      int64
	Rating      *float64 // total average rating that day
	HasInstalls bool     // false when the installs report lacked every known column (don't overwrite with zeros)
}

// BulkStats reads installs and ratings overview CSVs for one month from the Play Console bucket.
func (c *Client) BulkStats(ctx context.Context, bucket, pkg string, month time.Time) ([]StatsDay, error) {
	ym := month.Format("200601")
	days := map[string]*StatsDay{}
	get := func(k string) *StatsDay {
		if days[k] == nil {
			d, _ := time.Parse(time.DateOnly, k)
			days[k] = &StatsDay{Day: d}
		}
		return days[k]
	}
	inst, err := c.objectCSV(ctx, bucket, fmt.Sprintf("stats/installs/installs_%s_%s_overview.csv", pkg, ym))
	if err != nil {
		return nil, err
	}
	for _, row := range inst {
		k := row["Date"]
		if k == "" {
			continue
		}
		installKeys := []string{"Daily User Installs", "Daily Device Installs", "Install events"}
		if !hasAny(row, installKeys...) {
			continue
		}
		d := get(k)
		d.HasInstalls = true
		d.Installs = firstInt(row, installKeys...)
		d.Uninstalls = firstInt(row, "Daily User Uninstalls", "Daily Device Uninstalls", "Uninstall events")
		d.Active = firstInt(row, "Active Device Installs", "Active Devices") // not "Total User Installs" (cumulative)
	}
	rat, err := c.objectCSV(ctx, bucket, fmt.Sprintf("stats/ratings/ratings_%s_%s_overview.csv", pkg, ym))
	if err == nil {
		for _, row := range rat {
			if k := row["Date"]; k != "" {
				if f, err := strconv.ParseFloat(strings.TrimSpace(row["Total Average Rating"]), 64); err == nil && f > 0 {
					get(k).Rating = &f
				}
			}
		}
	}
	out := make([]StatsDay, 0, len(days))
	for _, d := range days {
		if !d.Day.IsZero() {
			out = append(out, *d)
		}
	}
	return out, nil
}

// ErrNoObject means the report for that month does not exist yet.
var ErrNoObject = fmt.Errorf("Google Play: report not found")

func (c *Client) objectCSV(ctx context.Context, bucket, object string) ([]map[string]string, error) {
	raw, err := c.object(ctx, bucket, object)
	if err != nil {
		return nil, err
	}
	return parseCSV(raw)
}

// object downloads one file from the Play Console reports bucket.
func (c *Client) object(ctx context.Context, bucket, object string) ([]byte, error) {
	tok, err := c.accessToken(ctx)
	if err != nil {
		return nil, err
	}
	u := c.Storage + "/storage/v1/b/" + url.PathEscape(bucket) + "/o/" + url.PathEscape(object) + "?alt=media"
	req, err := http.NewRequestWithContext(ctx, "GET", u, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+tok)
	resp, err := c.h.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return nil, ErrNoObject
	}
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 32<<20))
	if resp.StatusCode >= 300 {
		return nil, fmt.Errorf("Google Play reports: HTTP %d for %s", resp.StatusCode, object)
	}
	return raw, nil
}

// list names the bucket's objects under a prefix.
func (c *Client) list(ctx context.Context, bucket, prefix string) ([]string, error) {
	tok, err := c.accessToken(ctx)
	if err != nil {
		return nil, err
	}
	var names []string
	page := ""
	for i := 0; i < 20; i++ {
		q := url.Values{"prefix": {prefix}}
		if page != "" {
			q.Set("pageToken", page)
		}
		var res struct {
			Items []struct {
				Name string `json:"name"`
			} `json:"items"`
			Next string `json:"nextPageToken"`
		}
		if err := c.do(ctx, "GET", c.Storage+"/storage/v1/b/"+url.PathEscape(bucket)+"/o?"+q.Encode(), tok, nil, &res); err != nil {
			return nil, err
		}
		for _, it := range res.Items {
			names = append(names, it.Name)
		}
		if page = res.Next; page == "" {
			break
		}
	}
	return names, nil
}

// EarningOrder adds up one order's rows in Google's earnings reports, in INR (Merchant Currency).
type EarningOrder struct {
	Gross     float64 // "Charge" and "Charge refund" rows: what the buyer paid, net of refunds
	Fee       float64 // "Google fee" rows (negative)
	Tax       float64 // "Tax" rows Google withheld (negative)
	Other     float64
	Country   string
	HasCharge bool // the original charge is in these reports (a refund-only order can't be priced from them)
	HasTax    bool
}

// NetPaise is what the order leaves the developer with. For Indian buyers Google does not withhold GST (the
// developer pays it), so with no Tax row the GST share (18%) is taken out of the gross here.
// TODO(owner): confirm against one real earnings_YYYYMM file that IN orders have no Tax row.
func (e EarningOrder) NetPaise() int64 {
	gross := e.Gross
	if e.Country == "IN" && !e.HasTax {
		gross = gross * 100 / 118
	}
	return int64(math.Round((gross + e.Fee + e.Tax + e.Other) * 100))
}

// MergeEarnings adds month b into a, so an order charged in one month and partly refunded in the next adds up.
func MergeEarnings(a, b map[string]EarningOrder) map[string]EarningOrder {
	if a == nil {
		a = map[string]EarningOrder{}
	}
	for o, e := range b {
		m := a[o]
		m.Gross += e.Gross
		m.Fee += e.Fee
		m.Tax += e.Tax
		m.Other += e.Other
		m.HasCharge = m.HasCharge || e.HasCharge
		m.HasTax = m.HasTax || e.HasTax
		if m.Country == "" {
			m.Country = e.Country
		}
		a[o] = m
	}
	return a
}

// Earnings reads a month's earnings report (written early the next month), per order id. ErrNoObject = not out yet.
func (c *Client) Earnings(ctx context.Context, bucket string, month time.Time) (map[string]EarningOrder, error) {
	names, err := c.list(ctx, bucket, "earnings/earnings_"+month.Format("200601"))
	if err != nil {
		return nil, err
	}
	if len(names) == 0 {
		return nil, ErrNoObject
	}
	out := map[string]EarningOrder{}
	for _, n := range names {
		raw, err := c.object(ctx, bucket, n)
		if err != nil {
			return nil, err
		}
		var files [][]byte
		if strings.HasSuffix(n, ".zip") {
			zr, err := zip.NewReader(bytes.NewReader(raw), int64(len(raw)))
			if err != nil {
				return nil, fmt.Errorf("Google Play earnings %s: %w", n, err)
			}
			for _, f := range zr.File {
				rc, err := f.Open()
				if err != nil {
					return nil, err
				}
				b, err := io.ReadAll(io.LimitReader(rc, 64<<20))
				rc.Close()
				if err != nil {
					return nil, err
				}
				files = append(files, b)
			}
		} else {
			files = append(files, raw)
		}
		for _, f := range files {
			rows, err := parseCSV(f)
			if err != nil {
				return nil, fmt.Errorf("Google Play earnings %s: %w", n, err)
			}
			for _, r := range rows {
				order := r["Description"]
				if !strings.HasPrefix(order, "GPA.") || r["Merchant Currency"] != "INR" {
					continue
				}
				v, err := strconv.ParseFloat(strings.ReplaceAll(r["Amount (Merchant Currency)"], ",", ""), 64)
				if err != nil {
					continue
				}
				e := out[order]
				switch t := strings.ToLower(r["Transaction Type"]); {
				case t == "charge":
					e.Gross += v
					e.HasCharge = true
				case strings.HasPrefix(t, "charge"): // Charge refund, chargeback
					e.Gross += v
				case strings.HasPrefix(t, "google fee"):
					e.Fee += v
				case strings.HasPrefix(t, "tax"):
					e.Tax += v
					e.HasTax = true
				default:
					e.Other += v
				}
				if c := r["Buyer Country"]; c != "" {
					e.Country = c
				}
				out[order] = e
			}
		}
	}
	return out, nil
}

// parseCSV reads a Play report: UTF-16 (with BOM) or UTF-8, first row is the header.
func parseCSV(raw []byte) ([]map[string]string, error) {
	text := decodeUTF16(raw)
	r := csv.NewReader(strings.NewReader(text))
	r.FieldsPerRecord = -1
	rows, err := r.ReadAll()
	if err != nil || len(rows) == 0 {
		return nil, err
	}
	head := rows[0]
	out := make([]map[string]string, 0, len(rows)-1)
	for _, row := range rows[1:] {
		m := map[string]string{}
		for i, h := range head {
			if i < len(row) {
				m[strings.TrimSpace(h)] = strings.TrimSpace(row[i])
			}
		}
		out = append(out, m)
	}
	return out, nil
}

func decodeUTF16(b []byte) string {
	switch {
	case bytes.HasPrefix(b, []byte{0xFF, 0xFE}):
		return utf16String(b[2:], false)
	case bytes.HasPrefix(b, []byte{0xFE, 0xFF}):
		return utf16String(b[2:], true)
	case bytes.HasPrefix(b, []byte{0xEF, 0xBB, 0xBF}):
		return string(b[3:])
	}
	return string(b)
}

func utf16String(b []byte, bigEndian bool) string {
	u := make([]uint16, 0, len(b)/2)
	for i := 0; i+1 < len(b); i += 2 {
		if bigEndian {
			u = append(u, uint16(b[i])<<8|uint16(b[i+1]))
		} else {
			u = append(u, uint16(b[i+1])<<8|uint16(b[i]))
		}
	}
	return string(utf16.Decode(u))
}

func firstInt(row map[string]string, keys ...string) int64 {
	for _, k := range keys {
		if v, ok := row[k]; ok && v != "" {
			n, _ := strconv.ParseInt(strings.ReplaceAll(v, ",", ""), 10, 64)
			return n
		}
	}
	return 0
}

func hasAny(row map[string]string, keys ...string) bool {
	for _, k := range keys {
		if _, ok := row[k]; ok {
			return true
		}
	}
	return false
}
