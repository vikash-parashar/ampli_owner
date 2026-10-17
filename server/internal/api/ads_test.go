package api_test

import (
	"context"
	"encoding/base64"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/vikash-parashar/ampli_owner/server/internal/ads"
	"github.com/vikash-parashar/ampli_owner/server/internal/api"
	"github.com/vikash-parashar/ampli_owner/server/internal/secret"
	"github.com/vikash-parashar/ampli_owner/server/internal/store"
)

// fakeMeta stands in for a real ad platform.
type fakeMeta struct {
	mu      sync.Mutex
	created []ads.Spec
	active  map[string]bool
	budget  map[string]int64
	days    []ads.DayInsight
	token   string
}

func (f *fakeMeta) ID() string       { return "meta" }
func (f *fakeMeta) Name() string     { return "Meta" }
func (f *fakeMeta) Configured() bool { return true }
func (f *fakeMeta) AuthURL(state, redirect string) string {
	return "https://facebook.test/dialog?" + url.Values{"state": {state}, "redirect_uri": {redirect}}.Encode()
}
func (f *fakeMeta) Exchange(_ context.Context, code, _ string) (ads.Token, []ads.Account, error) {
	if code != "good-code" {
		return ads.Token{}, nil, fmt.Errorf("bad code")
	}
	return ads.Token{Access: "secret-access-token"}, []ads.Account{{ExternalID: "act_42", Name: "Ampli Studio", Currency: "INR"}}, nil
}
func (f *fakeMeta) Fresh(_ context.Context, t ads.Token) (ads.Token, error) { return t, nil }
func (f *fakeMeta) Insights(_ context.Context, t ads.Token, _ ads.Account, _, _ string) ([]ads.DayInsight, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.token = t.Access
	return f.days, nil
}
func (f *fakeMeta) Create(_ context.Context, t ads.Token, _ ads.Account, s ads.Spec) (ads.Ref, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.token = t.Access
	f.created = append(f.created, s)
	return ads.Ref{CampaignID: fmt.Sprint(1000 + len(f.created)), SetID: "set-1"}, nil
}
func (f *fakeMeta) SetActive(_ context.Context, _ ads.Token, _ ads.Account, r ads.Ref, on bool) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.active[r.CampaignID] = on
	return nil
}
func (f *fakeMeta) SetDailyBudget(_ context.Context, _ ads.Token, _ ads.Account, r ads.Ref, p int64) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.budget[r.SetID] = p
	return nil
}

// clockAhead moves the test server's clock forward (nanoseconds).
var clockAhead atomic.Int64

func TestAdsManagerFlow(t *testing.T) {
	fake := &fakeMeta{active: map[string]bool{}, budget: map[string]int64{}}
	box, _ := secret.New(base64.StdEncoding.EncodeToString([]byte("0123456789abcdef0123456789abcdef")))
	srv, st := newServerWith(t, func(s *api.Server) {
		s.Box, s.PublicURL = box, "https://owner.test"
		s.Platforms = map[string]ads.Platform{"meta": fake}
		s.Play = fakePlay(t)
		s.Now = func() time.Time { return time.Now().Add(time.Duration(clockAhead.Load())) }
	})
	clockAhead.Store(0)

	// Connect: the app gets a sign-in link, the platform calls back with a code.
	var conn struct {
		AuthURL string `json:"auth_url"`
	}
	if code := call(t, srv, "POST", "/v1/ads/connect/meta", true, nil, &conn); code != 200 {
		t.Fatalf("connect: %d", code)
	}
	u, _ := url.Parse(conn.AuthURL)
	if u.Query().Get("redirect_uri") != "https://owner.test/oauth/meta/callback" {
		t.Fatalf("redirect = %q", u.Query().Get("redirect_uri"))
	}
	if code := call(t, srv, "GET", "/oauth/meta/callback?code=good-code&state=forged.1.x", false, nil, nil); code != 400 {
		t.Fatalf("forged state: %d", code)
	}
	res, err := http.Get(srv.URL + "/oauth/meta/callback?code=good-code&state=" + url.QueryEscape(u.Query().Get("state")))
	if err != nil || res.StatusCode != 200 {
		t.Fatalf("callback: %v %d", err, res.StatusCode)
	}
	res.Body.Close()
	var platforms []struct {
		ID         string            `json:"id"`
		Configured bool              `json:"configured"`
		Accounts   []store.AdAccount `json:"accounts"`
	}
	call(t, srv, "GET", "/v1/ads/platforms", true, nil, &platforms)
	if len(platforms) != 3 || !platforms[0].Configured || len(platforms[0].Accounts) != 1 || platforms[1].Configured {
		t.Fatalf("platforms = %+v", platforms)
	}
	acct := platforms[0].Accounts[0]
	// The token is stored encrypted, never in plain text.
	var raw []byte
	if err := st.DB.QueryRow(context.Background(), `SELECT token_enc FROM ad_accounts WHERE id=$1`, acct.ID).Scan(&raw); err != nil ||
		strings.Contains(string(raw), "secret-access-token") {
		t.Fatalf("token stored in plain text or missing: %v", err)
	}

	// Launch a campaign for MP3 in the USA.
	newC := map[string]any{"product": "mp3", "ad_account_id": acct.ID, "countries": []string{"US"}, "daily_budget_paise": 100000,
		"days": 7, "creative": map[string]string{"kind": "video", "url": "https://cdn.test/mp3.mp4", "text": "Pay once, play forever"}, "launch": true}
	var c store.Campaign
	if code := call(t, srv, "POST", "/v1/ads/campaigns", true, newC, &c); code != 201 {
		t.Fatalf("create: %d", code)
	}
	if c.Status != "in_review" || c.UTMCampaign != "ampli-c1" || c.Verdict.Code != "new" || len(fake.created) != 1 ||
		fake.created[0].PackageID != "com.vikash.ampli" || !fake.created[0].Launch || fake.token != "secret-access-token" {
		t.Fatalf("campaign = %+v, spec = %+v", c, fake.created)
	}
	bad := map[string]any{"product": "mp3", "ad_account_id": acct.ID, "countries": []string{"XX"}, "daily_budget_paise": 100000,
		"creative": map[string]string{"kind": "video", "url": "https://cdn.test/a.mp4"}}
	if code := call(t, srv, "POST", "/v1/ads/campaigns", true, bad, nil); code != 400 {
		t.Fatalf("bad country: %d", code)
	}

	// Sync pulls 6 days of spend: ₹1,000 a day.
	today := time.Now().In(time.FixedZone("IST", 19800))
	for i := 0; i < 6; i++ {
		fake.days = append(fake.days, ads.DayInsight{CampaignID: "1001", Day: today.AddDate(0, 0, -i).Format(time.DateOnly),
			SpendPaise: 100000, Impressions: 5000, Clicks: 100, Installs: 20})
	}
	var sync struct {
		Synced int `json:"synced"`
	}
	call(t, srv, "POST", "/v1/ads/sync", true, nil, &sync)
	if sync.Synced != 1 {
		t.Fatalf("synced = %d", sync.Synced)
	}

	// Installs from the ad, and one purchase each from two of them: ₹199 × 2 = 0.07x ROAS.
	for i, ref := range []string{"utm_source=apps.facebook.com&utm_campaign=fb4a", "utm_source=ampli&utm_campaign=ampli-c1", "utm_source=google-play&utm_medium=organic"} {
		in := map[string]string{"product": "mp3", "install_id": fmt.Sprintf("00000000-0000-4000-8000-%012d", i), "referrer": ref}
		if code := call(t, srv, "POST", "/v1/public/installs", false, in, nil); code != 200 {
			t.Fatalf("install: %d", code)
		}
	}
	for i := 0; i < 2; i++ {
		p := map[string]any{"product": "mp3", "install_id": fmt.Sprintf("00000000-0000-4000-8000-%012d", i), "kind": "pro",
			"sku": "ampli_pro", "purchase_token": fmt.Sprintf("good-token-%d", i)}
		var res struct{ Recorded, Verified bool }
		code := call(t, srv, "POST", "/v1/public/purchases", false, p, &res)
		if !res.Recorded || !res.Verified {
			t.Fatalf("purchase %d: %d %+v", i, code, res)
		}
		call(t, srv, "POST", "/v1/public/purchases", false, p, &res) // retry is ignored
		if res.Recorded {
			t.Fatalf("retry recorded twice")
		}
	}
	// A forged token is refused; a license tester's purchase is stored but is not money.
	forged := map[string]any{"product": "mp3", "install_id": "00000000-0000-4000-8000-000000000002", "sku": "ampli_pro", "purchase_token": "forged-token-1"}
	if code := call(t, srv, "POST", "/v1/public/purchases", false, forged, nil); code != 422 {
		t.Fatalf("forged purchase: %d", code)
	}
	forged["purchase_token"] = "pend-token-1"
	if code := call(t, srv, "POST", "/v1/public/purchases", false, forged, nil); code != 409 {
		t.Fatalf("pending purchase: %d", code)
	}
	// Google down: saved unverified with 503 (retry); the retry after Google is back upgrades it.
	playDown.Store(true)
	forged["purchase_token"] = "flaky-token-1"
	if code := call(t, srv, "POST", "/v1/public/purchases", false, forged, nil); code != 503 {
		t.Fatalf("google down: %d", code)
	}
	playDown.Store(false)
	var up struct{ Recorded, Verified bool }
	if code := call(t, srv, "POST", "/v1/public/purchases", false, forged, &up); code != 200 || !up.Recorded || !up.Verified {
		t.Fatalf("retry after outage: %d %+v", code, up)
	}
	forged["purchase_token"] = "test-token-1"
	if code := call(t, srv, "POST", "/v1/public/purchases", false, forged, nil); code != 200 {
		t.Fatalf("tester purchase: %d", code)
	}
	delete(forged, "sku")
	forged["product_id"], forged["purchase_token"] = "ampli_tip_49", "test-token-2" // Play's field name works too
	if code := call(t, srv, "POST", "/v1/public/purchases", false, forged, nil); code != 200 {
		t.Fatalf("product_id alias: %d", code)
	}
	forged["product_id"] = "made_up"
	if code := call(t, srv, "POST", "/v1/public/purchases", false, forged, nil); code != 400 {
		t.Fatalf("unknown sku: %d", code)
	}
	if code := call(t, srv, "POST", "/v1/public/installs", false, map[string]string{"product": "mp3", "install_id": "not-a-uuid"}, nil); code != 400 {
		t.Fatalf("bad install id: %d", code)
	}
	if code := call(t, srv, "POST", "/v1/public/events", false, map[string]any{"product": "mp3",
		"install_id": "00000000-0000-4000-8000-000000000001", "events": []string{"made_up_event"}}, nil); code != 400 {
		t.Fatalf("unknown event: %d", code)
	}
	const net = 2 * 14334 // ₹199 minus 18% GST minus Google's 15%, twice (the organic install's purchase is not the ad's)
	var list struct {
		Summary   api.AdsSummary   `json:"summary"`
		Campaigns []store.Campaign `json:"campaigns"`
	}
	call(t, srv, "GET", "/v1/ads/campaigns?product=mp3", true, nil, &list)
	got := list.Campaigns[0]
	if got.SpendPaise != 600000 || got.Installs != 120 || got.AttributedInstalls != 2 || got.RevenuePaise != net ||
		got.Days != 6 || got.Status != "live" || got.Verdict.Code != "stop" || list.Summary.ProfitPaise != net-600000 {
		t.Fatalf("after sync = %+v summary %+v", got, list.Summary)
	}

	// Pause and budget go to the platform.
	call(t, srv, "POST", "/v1/ads/campaigns/1/pause", true, nil, &c)
	if c.Status != "paused" || fake.active["1001"] {
		t.Fatalf("pause: %+v", c)
	}
	call(t, srv, "POST", "/v1/ads/campaigns/1/budget", true, map[string]int64{"daily_budget_paise": 120000}, &c)
	if c.DailyBudgetPaise != 120000 || fake.budget["set-1"] != 120000 {
		t.Fatalf("budget: %+v", c)
	}

	// The MP3 dashboard counts the purchases and ad installs.
	var d api.Dashboard
	call(t, srv, "GET", "/v1/dashboard?product=mp3", true, nil, &d)
	if d.BusinessPaise != net+14334 || d.ProSales != 3 || d.Downloads != 3 || d.WeeklyDownloads[3].Ads != 2 || d.WeeklyDownloads[3].Organic != 1 ||
		d.Revenue30d[29] != net+14334 || d.Ads.SpendPaise != 600000 || len(d.Widgets) != 3 {
		t.Fatalf("dashboard = %+v", d)
	}
	if got.Attribution != "estimate" {
		t.Fatalf("attribution = %q, want estimate (one install matched only by platform and time)", got.Attribution)
	}

	// A live losing campaign raises a high alert.
	call(t, srv, "POST", "/v1/ads/campaigns/1/resume", true, nil, &c)
	var alerts []api.Alert
	call(t, srv, "GET", "/v1/alerts", true, nil, &alerts)
	if len(alerts) == 0 || alerts[0].ID != "campaign-stop-1" || alerts[0].Severity != "high" {
		t.Fatalf("alerts = %+v", alerts)
	}

	// Auto-stop pauses it on the platform at the next sync and logs why.
	rule := map[string]any{"enabled": true, "min_roas": 0.5, "min_days": 5, "min_spend_paise": 200000}
	if code := call(t, srv, "POST", "/v1/ads/campaigns/1/autostop", true, rule, &c); code != 200 || !c.AutoStop.Enabled || c.AutoStop.MinDays != 5 {
		t.Fatalf("autostop: %d %+v", code, c.AutoStop)
	}
	if code := call(t, srv, "POST", "/v1/ads/campaigns/1/autostop", true, map[string]any{"enabled": true, "min_days": 1}, nil); code != 400 {
		t.Fatalf("autostop min_days 1: %d", code)
	}
	if !fake.active["1001"] {
		t.Fatal("resume did not reach the platform")
	}
	// First below-limit sync only warns; the one 24 hours later pauses.
	call(t, srv, "POST", "/v1/ads/sync?force=1", true, nil, nil)
	call(t, srv, "GET", "/v1/ads/campaigns?product=mp3", true, nil, &list)
	if list.Campaigns[0].Status != "live" || list.Campaigns[0].AutoStop.PendingSince == nil || !fake.active["1001"] {
		t.Fatalf("first strike: %+v", list.Campaigns[0])
	}
	call(t, srv, "GET", "/v1/alerts", true, nil, &alerts)
	if alerts[0].ID != "campaign-autostop-soon-1" {
		t.Fatalf("alerts before pause = %+v", alerts)
	}
	clockAhead.Store(int64(25 * time.Hour))
	call(t, srv, "POST", "/v1/ads/sync?force=1", true, nil, nil)
	call(t, srv, "GET", "/v1/ads/campaigns?product=mp3", true, nil, &list)
	if list.Campaigns[0].Status != "paused" || fake.active["1001"] {
		t.Fatalf("auto-stop did not pause: %+v", list.Campaigns[0])
	}
	var log []store.LogEntry
	call(t, srv, "GET", "/v1/ads/campaigns/1/log", true, nil, &log)
	if len(log) < 6 || log[0].Actor != "rule" || log[0].Action != "paused" || log[1].Action != "will pause" {
		t.Fatalf("log = %+v", log)
	}
	call(t, srv, "GET", "/v1/alerts", true, nil, &alerts)
	found := false
	for _, a := range alerts {
		found = found || a.ID == "campaign-autostopped-1"
	}
	if !found {
		t.Fatalf("no paused alert: %+v", alerts)
	}

	// A manual resume is honoured: the rule waits instead of pausing it again at the next syncs.
	call(t, srv, "POST", "/v1/ads/campaigns/1/resume", true, nil, &c)
	for _, ahead := range []time.Duration{26 * time.Hour, 51 * time.Hour} {
		clockAhead.Store(int64(ahead))
		call(t, srv, "POST", "/v1/ads/sync?force=1", true, nil, nil)
	}
	call(t, srv, "GET", "/v1/ads/campaigns?product=mp3", true, nil, &list)
	if list.Campaigns[0].Status != "live" || !fake.active["1001"] {
		t.Fatalf("rule overrode a manual resume: %+v", list.Campaigns[0])
	}

	// Delete forgets it.
	if code := call(t, srv, "DELETE", "/v1/ads/campaigns/1", true, nil, nil); code != 204 {
		t.Fatalf("delete: %d", code)
	}
	call(t, srv, "GET", "/v1/ads/campaigns?product=mp3", true, nil, &list)
	if len(list.Campaigns) != 0 {
		t.Fatalf("deleted campaign still listed: %+v", list.Campaigns)
	}
	if code := call(t, srv, "GET", "/v1/ads/campaigns/1/log", true, nil, &log); code != 200 || log[0].Action != "deleted" {
		t.Fatalf("log after delete should be kept: %d %+v", code, log)
	}
}

func TestSKUsEraseAndTokenRotation(t *testing.T) {
	srv, _ := newServerWith(t, func(s *api.Server) { s.OwnerTokenNext = "next-owner-token-0123456789abcdef" })
	var ks []store.SKU
	call(t, srv, "GET", "/v1/skus", true, nil, &ks)
	if len(ks) < 10 {
		t.Fatalf("seeded skus = %d", len(ks))
	}
	if code := call(t, srv, "PUT", "/v1/skus", true, map[string]any{"product": "carhub", "sku": "ampli_carhub_pro", "kind": "pro", "price_paise": 49900}, nil); code != 200 {
		t.Fatalf("save sku: %d", code)
	}
	id := "12345678-1234-4234-8234-123456789abc"
	call(t, srv, "POST", "/v1/public/reports", false, report("bug", id), nil)
	var erased struct {
		Rows int64 `json:"erased_rows"`
	}
	if code := call(t, srv, "DELETE", "/v1/installs/"+id, true, nil, &erased); code != 200 || erased.Rows == 0 {
		t.Fatalf("erase: %d %+v", code, erased)
	}
	var mine []any
	call(t, srv, "GET", "/v1/public/reports?install_id="+id, false, nil, &mine)
	if len(mine) != 0 {
		t.Fatalf("erased install still has %d tickets", len(mine))
	}
	if code := callH(t, srv, "GET", "/v1/alerts", false, map[string]string{"Authorization": "Bearer next-owner-token-0123456789abcdef"}, nil, nil); code != 200 {
		t.Fatalf("next token: %d", code)
	}
	for i := 0; i < api.AuthFailuresPerDay; i++ {
		if code := callH(t, srv, "GET", "/v1/alerts", false, map[string]string{"Authorization": "Bearer wrong"}, nil, nil); code != 401 {
			t.Fatalf("wrong token %d: %d", i, code)
		}
	}
	if code := callH(t, srv, "GET", "/v1/alerts", false, map[string]string{"Authorization": "Bearer wrong"}, nil, nil); code != 429 {
		t.Fatalf("token guessing not throttled: %d", code)
	}
	var audit []store.AuditEntry
	call(t, srv, "GET", "/v1/audit", true, nil, &audit)
	if len(audit) != 2 || audit[0].Action != "erase install" || audit[1].Action != "sku" {
		t.Fatalf("audit = %+v", audit)
	}
}

func TestDashboardAllAndReviews(t *testing.T) {
	srv := newServer(t)
	day := time.Now().In(time.FixedZone("IST", 19800)).AddDate(0, 0, -1).Format(time.DateOnly)
	stats := []map[string]any{
		{"product": "mp3", "day": day, "installs": 100, "active_users": 900, "rating": 4.6, "pro_sales": 3, "pro_revenue_paise": 59700, "ad_revenue_paise": 10000},
		{"product": "naukri", "day": day, "installs": 400, "active_users": 3000, "ad_revenue_paise": 25000, "tips_paise": 4900},
	}
	if code := call(t, srv, "PUT", "/v1/stats", true, stats, nil); code != 200 {
		t.Fatalf("stats: %d", code)
	}
	reviews := []map[string]any{
		{"product": "mp3", "author": "Rohit", "rating": 5, "body": "Record player screen is beautiful, no monthly fee"},
		{"product": "mp3", "author": "Ankit", "rating": 2, "body": "Too many ads after every song"},
		{"product": "naukri", "author": "Pooja", "rating": 5, "body": "Haryana police bharti alert came first"},
		{"product": "naukri", "author": "Kiran", "rating": 3, "body": "Login OTP did not come"},
	}
	for _, rv := range reviews {
		if code := call(t, srv, "POST", "/v1/reviews", true, rv, nil); code != 201 {
			t.Fatalf("review: %d", code)
		}
	}
	call(t, srv, "POST", "/v1/public/events", false, map[string]any{"product": "carhub", "install_id": "ca000000-0000-4000-8000-000000000001",
		"events": []string{"voice_bunty", "cmd_navigate", "voice_bunty"}}, nil)
	call(t, srv, "POST", "/v1/public/events", false, map[string]any{"product": "carhub", "install_id": "ca000000-0000-4000-8000-000000000002",
		"events": []string{"voice_simran"}}, nil)

	var all api.Dashboard
	call(t, srv, "GET", "/v1/dashboard?product=all", true, nil, &all)
	if all.BusinessPaise != 59700+10000+25000+4900 || all.Downloads != 500 || all.ActiveUsers != 3900 || len(all.Products) != 3 ||
		all.Stars != [5]int64{2, 0, 1, 1, 0} || all.Sentiment["positive"] != 50 || all.Widgets[0].ID != "money" {
		t.Fatalf("all = %+v", all)
	}
	var car api.Dashboard
	call(t, srv, "GET", "/v1/dashboard?product=carhub", true, nil, &car)
	if car.Widgets[0].ID != "voice" || car.Widgets[0].Rows[0].Value != 1 || car.Widgets[0].Rows[0].Pct != 50 {
		t.Fatalf("carhub widgets = %+v", car.Widgets)
	}

	var rv struct {
		Summary struct {
			Total    int64   `json:"total"`
			Positive float64 `json:"positive"`
		} `json:"summary"`
		Themes struct {
			Praise     []store.Theme `json:"praise"`
			Complaints []store.Theme `json:"complaints"`
		} `json:"themes"`
		Reviews []store.Review `json:"reviews"`
	}
	call(t, srv, "GET", "/v1/reviews?product=mp3", true, nil, &rv)
	if rv.Summary.Total != 2 || rv.Summary.Positive != 50 || len(rv.Reviews) != 2 || len(rv.Themes.Complaints) == 0 ||
		rv.Themes.Complaints[0].Label != "Too many ads" {
		t.Fatalf("reviews = %+v", rv)
	}
	var replied store.Review
	if code := call(t, srv, "POST", fmt.Sprintf("/v1/reviews/%d/reply", rv.Reviews[0].ID), true, map[string]string{"text": "Thanks!"}, &replied); code != 200 || replied.Reply != "Thanks!" {
		t.Fatalf("reply: %d %+v", code, replied)
	}
}
