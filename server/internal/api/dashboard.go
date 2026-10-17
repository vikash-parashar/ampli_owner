package api

import (
	"context"
	"math"
	"net/http"
	"time"

	"github.com/vikash-parashar/ampli_owner/server/internal/store"
)

type WeekDownloads struct {
	Organic int64 `json:"organic"`
	Ads     int64 `json:"ads"`
}

type WidgetRow struct {
	Label string  `json:"label"`
	Value int64   `json:"value"`
	Pct   float64 `json:"pct"`
}

type Widget struct {
	ID    string      `json:"id"`
	Title string      `json:"title"`
	Kind  string      `json:"kind"` // funnel, bars, split
	Rows  []WidgetRow `json:"rows"`
}

type AdsSummary struct {
	SpendPaise   int64   `json:"spend_paise"`
	RevenuePaise int64   `json:"revenue_paise"`
	Installs     int64   `json:"installs"`
	ROAS         float64 `json:"roas"`
	CPIPaise     int64   `json:"cpi_paise"`
	ProfitPaise  int64   `json:"profit_paise"`
}

type DashProduct struct {
	ID            string   `json:"id"`
	Name          string   `json:"name"`
	BusinessPaise int64    `json:"business_paise"`
	Downloads     int64    `json:"downloads"`
	Rating        *float64 `json:"rating"`
	ChangePct     float64  `json:"change_pct"`
}

type Dashboard struct {
	Product           string             `json:"product"`
	BusinessPaise     int64              `json:"business_paise"`
	BusinessChangePct float64            `json:"business_change_pct"`
	Downloads         int64              `json:"downloads"`
	DownloadsWeek     int64              `json:"downloads_week"`
	Rating            *float64           `json:"rating"`
	Reviews           int64              `json:"reviews"`
	ActiveUsers       int64              `json:"active_users"`
	ActiveChangePct   float64            `json:"active_change_pct"`
	ProSales          int64              `json:"pro_sales"`
	ProRevenuePaise   int64              `json:"pro_revenue_paise"`
	AdRevenuePaise    int64              `json:"ad_revenue_paise"`
	TipsPaise         int64              `json:"tips_paise"`
	OtherPaise        int64              `json:"other_revenue_paise"`
	Revenue30d        []int64            `json:"revenue_30d"`
	WeeklyDownloads   []WeekDownloads    `json:"weekly_downloads"`
	Stars             [5]int64           `json:"stars"`
	Sentiment         map[string]float64 `json:"sentiment"`
	Ads               AdsSummary         `json:"ads"`
	Widgets           []Widget           `json:"widgets"`
	Products          []DashProduct      `json:"products,omitempty"`
	Vitals            *store.Vitals      `json:"vitals"` // latest Play crash and ANR rates; null for all or when unknown

	activeBefore int64
	last30       int64
	prev30       int64
}

var ist = time.FixedZone("IST", 5*3600+1800)

func (s *Server) today() time.Time {
	t := s.Now().In(ist)
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)
}

func changePct(now, before int64) float64 {
	switch {
	case before == 0 && now == 0:
		return 0
	case before == 0:
		return 100
	}
	return math.Round(float64(now-before)/float64(before)*1000) / 10
}

func summarize(cs []store.Campaign) AdsSummary {
	var a AdsSummary
	for _, c := range cs {
		if c.Status == "draft" {
			continue
		}
		a.SpendPaise += c.SpendPaise
		a.RevenuePaise += c.RevenuePaise
		a.Installs += c.Installs
	}
	if a.SpendPaise > 0 {
		a.ROAS = math.Round(float64(a.RevenuePaise)/float64(a.SpendPaise)*100) / 100
	}
	if a.Installs > 0 {
		a.CPIPaise = a.SpendPaise / a.Installs
	}
	a.ProfitPaise = a.RevenuePaise - a.SpendPaise
	return a
}

func (s *Server) productDashboard(ctx context.Context, p string, today time.Time) (Dashboard, error) {
	d := Dashboard{Product: p, Revenue30d: make([]int64, 30), WeeklyDownloads: make([]WeekDownloads, 4), Widgets: []Widget{}}
	rows, err := s.Store.DayRows(ctx, p)
	if err != nil {
		return d, err
	}
	var installs30 int64
	for _, r := range rows {
		age := int(today.Sub(r.Day).Hours() / 24) // 0 = today
		rev := r.Revenue()
		d.BusinessPaise += rev
		d.Downloads += r.Installs
		d.ProSales += r.ProSales
		d.ProRevenuePaise += r.Pro
		d.AdRevenuePaise += r.Ad
		d.TipsPaise += r.Tips
		d.OtherPaise += r.Other
		switch {
		case age < 0:
		case age < 30:
			d.last30 += rev
			d.Revenue30d[29-age] = rev
			installs30 += r.Installs
		case age < 60:
			d.prev30 += rev
		}
		if age >= 0 && age < 7 {
			d.DownloadsWeek += r.Installs
		}
		if age >= 0 && age < 28 {
			w := &d.WeeklyDownloads[3-age/7]
			w.Ads += r.AdInstalls
			w.Organic += max(r.Installs-r.AdInstalls, 0)
		}
	}
	d.BusinessChangePct = changePct(d.last30, d.prev30)
	if d.ActiveUsers, d.activeBefore, err = s.Store.ActiveUsers(ctx, p, today); err != nil {
		return d, err
	}
	d.ActiveChangePct = changePct(d.ActiveUsers, d.activeBefore)
	if d.Rating, d.Reviews, err = s.Store.Rating(ctx, p); err != nil {
		return d, err
	}
	if d.Stars, err = s.Store.Stars(ctx, p); err != nil {
		return d, err
	}
	d.Sentiment = store.Sentiment(d.Stars)
	cs, err := s.Store.Campaigns(ctx, p)
	if err != nil {
		return d, err
	}
	d.Ads = summarize(cs)
	ev, err := s.Store.EventCounts(ctx, p, today)
	if err != nil {
		return d, err
	}
	var proSales30 int64
	for _, r := range rows {
		if age := int(today.Sub(r.Day).Hours() / 24); age >= 0 && age < 30 {
			proSales30 += r.ProSales
		}
	}
	d.Widgets = productWidgets(p, ev, installs30, proSales30)
	return d, nil
}

// rowsOf builds widget rows; pct is relative to base, or to the biggest value when base is 0.
func rowsOf(ev map[string]int64, base int64, items ...[2]string) []WidgetRow {
	out := make([]WidgetRow, 0, len(items))
	top := base
	for _, it := range items {
		if v := ev[it[0]]; base == 0 && v > top {
			top = v
		}
	}
	for _, it := range items {
		v := ev[it[0]]
		pct := 0.0
		if top > 0 {
			pct = math.Round(float64(v)/float64(top)*1000) / 10
		}
		out = append(out, WidgetRow{Label: it[1], Value: v, Pct: pct})
	}
	return out
}

func productWidgets(p string, ev map[string]int64, installs30, proSales30 int64) []Widget {
	switch p {
	case "mp3":
		ev["_installed"], ev["_pro"] = installs30, proSales30
		return []Widget{
			{"funnel", "Free to Pro funnel (30 days)", "funnel", rowsOf(ev, installs30, [2]string{"_installed", "Installed"},
				[2]string{"played_10", "Played 10+ songs"}, [2]string{"pro_feature_opened", "Opened a Pro feature"},
				[2]string{"pro_trial", "Tried Pro for 24 h"}, [2]string{"_pro", "Bought Pro ₹199"})},
			{"features", "Most used features", "bars", rowsOf(ev, 0, [2]string{"feature_eq", "Equalizer + bass boost"},
				[2]string{"feature_record_player", "Record player screen"}, [2]string{"feature_sleep_timer", "Sleep timer"},
				[2]string{"feature_ringtone", "Ringtone maker"}, [2]string{"feature_wifi_transfer", "Wi-Fi transfer"})},
		}
	case "carhub":
		voices := rowsOf(ev, 0, [2]string{"voice_bunty", "Bunty (male)"}, [2]string{"voice_simran", "Simran (female)"})
		sharePct(voices)
		return []Widget{
			{"voice", "Voice the drivers picked", "split", voices},
			{"commands", "Top voice commands", "bars", rowsOf(ev, 0, [2]string{"cmd_navigate", "Navigate to…"},
				[2]string{"cmd_music", "Play music"}, [2]string{"cmd_call", "Call…"}, [2]string{"cmd_fastag", "FASTag balance"},
				[2]string{"cmd_sos", "SOS"})},
		}
	case "naukri":
		return []Widget{
			{"alerts", "Alerts (30 days)", "bars", rowsOf(ev, 0, [2]string{"alert_sent", "Job alerts sent"},
				[2]string{"marked_applied", "Marked applied"})},
			{"states", "Users by state", "bars", rowsOf(ev, 0, [2]string{"state_hr", "Haryana"}, [2]string{"state_up", "Uttar Pradesh"},
				[2]string{"state_rj", "Rajasthan"}, [2]string{"state_dl", "Delhi"}, [2]string{"state_other", "Other"})},
		}
	}
	return []Widget{}
}

// sharePct rewrites pct as each row's share of the total.
func sharePct(rows []WidgetRow) {
	var t int64
	for _, r := range rows {
		t += r.Value
	}
	for i := range rows {
		rows[i].Pct = 0
		if t > 0 {
			rows[i].Pct = math.Round(float64(rows[i].Value)/float64(t)*1000) / 10
		}
	}
}

func (s *Server) dashboard(w http.ResponseWriter, r *http.Request) {
	ctx, today := r.Context(), s.today()
	p := r.URL.Query().Get("product")
	if p == "" {
		p = "all"
	}
	if p != "all" {
		if productID(p) == "" {
			writeErr(w, http.StatusBadRequest, "product must be all, mp3, carhub or naukri")
			return
		}
		d, err := s.productDashboard(ctx, p, today)
		if err != nil {
			fail(w, r, err)
			return
		}
		vs, err := s.Store.LatestVitals(ctx)
		if err != nil {
			fail(w, r, err)
			return
		}
		for i := range vs {
			if vs[i].ProductID == p {
				d.Vitals = &vs[i]
			}
		}
		writeJSON(w, http.StatusOK, d)
		return
	}
	prods, err := s.Store.Products(ctx)
	if err != nil {
		fail(w, r, err)
		return
	}
	all := Dashboard{Product: "all", Revenue30d: make([]int64, 30), WeeklyDownloads: make([]WeekDownloads, 4), Products: []DashProduct{}}
	var ratingSum float64
	var ratingN int64
	var last30, prev30 int64
	money := Widget{"money", "Where the money comes from", "split", []WidgetRow{}}
	for _, pr := range prods {
		d, err := s.productDashboard(ctx, pr.ID, today)
		if err != nil {
			fail(w, r, err)
			return
		}
		all.BusinessPaise += d.BusinessPaise
		all.Downloads += d.Downloads
		all.DownloadsWeek += d.DownloadsWeek
		all.Reviews += d.Reviews
		all.ActiveUsers += d.ActiveUsers
		all.activeBefore += d.activeBefore
		all.ProSales += d.ProSales
		all.ProRevenuePaise += d.ProRevenuePaise
		all.AdRevenuePaise += d.AdRevenuePaise
		all.TipsPaise += d.TipsPaise
		all.OtherPaise += d.OtherPaise
		last30 += d.last30
		prev30 += d.prev30
		for i := range all.Revenue30d {
			all.Revenue30d[i] += d.Revenue30d[i]
		}
		for i := range all.WeeklyDownloads {
			all.WeeklyDownloads[i].Ads += d.WeeklyDownloads[i].Ads
			all.WeeklyDownloads[i].Organic += d.WeeklyDownloads[i].Organic
		}
		for i := range all.Stars {
			all.Stars[i] += d.Stars[i]
		}
		if d.Rating != nil {
			n := max(d.Reviews, 1)
			ratingSum += *d.Rating * float64(n)
			ratingN += n
		}
		all.Products = append(all.Products, DashProduct{pr.ID, pr.Name, d.BusinessPaise, d.Downloads, d.Rating, d.BusinessChangePct})
		money.Rows = append(money.Rows, WidgetRow{Label: pr.Name, Value: d.BusinessPaise})
	}
	if ratingN > 0 {
		v := math.Round(ratingSum/float64(ratingN)*10) / 10
		all.Rating = &v
	}
	all.BusinessChangePct = changePct(last30, prev30)
	all.ActiveChangePct = changePct(all.ActiveUsers, all.activeBefore)
	all.Sentiment = store.Sentiment(all.Stars)
	cs, err := s.Store.Campaigns(ctx, "")
	if err != nil {
		fail(w, r, err)
		return
	}
	all.Ads = summarize(cs)
	sharePct(money.Rows)
	all.Widgets = []Widget{money}
	writeJSON(w, http.StatusOK, all)
}
