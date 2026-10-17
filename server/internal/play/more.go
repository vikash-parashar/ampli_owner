package play

import (
	"bytes"
	"context"
	"encoding/csv"
	"fmt"
	"io"
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
	return parseCSV(raw)
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
