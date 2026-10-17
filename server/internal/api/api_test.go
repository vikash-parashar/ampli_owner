package api_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/vikash-parashar/ampli_owner/server/internal/api"
	"github.com/vikash-parashar/ampli_owner/server/internal/store"
)

const token = "test-owner-token-0123456789abcdef"

// newServer needs TEST_DATABASE_URL pointing at a throwaway database; every table is wiped.
func newServer(t *testing.T) *httptest.Server {
	t.Helper()
	srv, _ := newServerWith(t, nil)
	return srv
}

// newServerWith lets a test add ad platforms, a token box or a fixed clock.
func newServerWith(t *testing.T, configure func(*api.Server)) (*httptest.Server, *store.Store) {
	t.Helper()
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL not set")
	}
	ctx := context.Background()
	st, err := store.Open(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(st.Close)
	if err := st.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := st.DB.Exec(ctx, `TRUNCATE tickets, ticket_messages, feature_votes, features, checklist_items, todos,
		builds, daily_stats, campaigns, guides, ad_accounts, campaign_days, installs, purchases, reviews, app_events,
		public_hits, ip_hits, campaign_log, vitals, integrations, audit_log RESTART IDENTITY CASCADE`); err != nil {
		t.Fatal(err)
	}
	as := &api.Server{Store: st, OwnerToken: token}
	if configure != nil {
		configure(as)
	}
	srv := httptest.NewServer(as.Handler())
	t.Cleanup(srv.Close)
	return srv, st
}

func call(t *testing.T, srv *httptest.Server, method, path string, auth bool, body any, out any) int {
	return callH(t, srv, method, path, auth, nil, body, out)
}

func callH(t *testing.T, srv *httptest.Server, method, path string, auth bool, hdr map[string]string, body any, out any) int {
	t.Helper()
	var buf bytes.Buffer
	if body != nil {
		if err := json.NewEncoder(&buf).Encode(body); err != nil {
			t.Fatal(err)
		}
	}
	req, _ := http.NewRequest(method, srv.URL+path, &buf)
	if auth {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	for k, v := range hdr {
		req.Header.Set(k, v)
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if out != nil {
		if err := json.NewDecoder(res.Body).Decode(out); err != nil {
			t.Fatalf("%s %s: decode: %v", method, path, err)
		}
	}
	return res.StatusCode
}

func report(kind, install string) map[string]any {
	return map[string]any{"product": "mp3", "kind": kind, "title": "Crossfade cuts the last second",
		"body": "Lost audio at 6s fade", "install_id": install, "app_version": "1.0 (36)", "device": "OnePlus 13s",
		"os": "Android 15 (SDK 35)", "locale": "en-IN", "pro": kind == "support"}
}

func TestOwnerEndpointsNeedToken(t *testing.T) {
	srv := newServer(t)
	if code := call(t, srv, "GET", "/v1/overview", false, nil, nil); code != 401 {
		t.Fatalf("no token: got %d, want 401", code)
	}
	if code := call(t, srv, "GET", "/healthz", false, nil, nil); code != 200 {
		t.Fatalf("healthz: got %d", code)
	}
}

func TestReportFlow(t *testing.T) {
	srv := newServer(t)
	install := "0a0a0a0a-aaaa-4aaa-8aaa-aaaaaaaaaaaa"

	var created struct {
		ID  int64  `json:"id"`
		Ref string `json:"ref"`
	}
	if code := call(t, srv, "POST", "/v1/public/reports", false, report("support", install), &created); code != 201 {
		t.Fatalf("create: %d", code)
	}
	if created.Ref != "T-0001" {
		t.Fatalf("ref = %q", created.Ref)
	}

	bad := report("bug", install)
	bad["title"] = "x"
	if code := call(t, srv, "POST", "/v1/public/reports", false, bad, nil); code != 400 {
		t.Fatalf("short title: got %d, want 400", code)
	}

	// A Pro claim without a verified Pro purchase is normal priority.
	var detail struct {
		Ticket   store.Ticket    `json:"ticket"`
		Messages []store.Message `json:"messages"`
	}
	call(t, srv, "GET", "/v1/tickets/T-0001", true, nil, &detail)
	if detail.Ticket.Priority != 2 || detail.Ticket.Source != "app" || detail.Ticket.Device != "OnePlus 13s" {
		t.Fatalf("ticket = %+v", detail.Ticket)
	}

	// Owner reply flips status and shows in the user's "My requests".
	if code := call(t, srv, "POST", fmt.Sprintf("/v1/tickets/%d/messages", created.ID), true, map[string]string{"body": "Fixed, restart the app"}, nil); code != 201 {
		t.Fatalf("reply: %d", code)
	}
	var mine []struct {
		Ref     string          `json:"ref"`
		Status  string          `json:"status"`
		Replies []store.Message `json:"replies"`
	}
	call(t, srv, "GET", "/v1/public/reports?install_id="+install, false, nil, &mine)
	if len(mine) != 1 || mine[0].Status != "waiting_on_user" || len(mine[0].Replies) != 1 {
		t.Fatalf("mine = %+v", mine)
	}

	// Another install sees nothing.
	var other []any
	call(t, srv, "GET", "/v1/public/reports?install_id=0b0b0b0b-bbbb-4bbb-8bbb-bbbbbbbbbbbb", false, nil, &other)
	if len(other) != 0 {
		t.Fatalf("other install saw %d tickets", len(other))
	}
}

func TestRateLimit(t *testing.T) {
	srv := newServer(t)
	for i := 0; i < api.ReportsPerDay; i++ {
		if code := call(t, srv, "POST", "/v1/public/reports", false, report("bug", "0c0c0c0c-cccc-4ccc-8ccc-cccccccccccc"), nil); code != 201 {
			t.Fatalf("report %d: %d", i, code)
		}
	}
	if code := call(t, srv, "POST", "/v1/public/reports", false, report("bug", "0c0c0c0c-cccc-4ccc-8ccc-cccccccccccc"), nil); code != 429 {
		t.Fatalf("over limit: got %d, want 429", code)
	}
}

func TestVotesOncePerInstall(t *testing.T) {
	srv := newServer(t)
	var created struct{ ID int64 }
	call(t, srv, "POST", "/v1/public/reports", false, report("feature", "0d0d0d0d-dddd-4ddd-8ddd-dddddddddddd"), &created)
	var v struct{ Votes int }
	path := fmt.Sprintf("/v1/public/features/%d/vote", created.ID)
	call(t, srv, "POST", path, false, map[string]string{"install_id": "11111111-1111-4111-8111-111111111111"}, &v)
	call(t, srv, "POST", path, false, map[string]string{"install_id": "11111111-1111-4111-8111-111111111111"}, &v)
	call(t, srv, "POST", path, false, map[string]string{"install_id": "22222222-2222-4222-8222-222222222222"}, &v)
	if v.Votes != 2 {
		t.Fatalf("votes = %d, want 2", v.Votes)
	}
	// Users only see requests the owner accepted.
	var feats []struct{ Votes int }
	call(t, srv, "GET", "/v1/public/features?product=mp3", false, nil, &feats)
	if len(feats) != 0 {
		t.Fatalf("unmoderated feature shown: %+v", feats)
	}
	call(t, srv, "PATCH", fmt.Sprintf("/v1/tickets/%d", created.ID), true, map[string]string{"status": "planned"}, nil)
	call(t, srv, "GET", "/v1/public/features?product=mp3", false, nil, &feats)
	if len(feats) != 1 || feats[0].Votes != 2 {
		t.Fatalf("features = %+v", feats)
	}
	// Voting on a bug is refused.
	var bug struct{ ID int64 }
	call(t, srv, "POST", "/v1/public/reports", false, report("bug", "0d0d0d0d-dddd-4ddd-8ddd-dddddddddddd"), &bug)
	if code := call(t, srv, "POST", fmt.Sprintf("/v1/public/features/%d/vote", bug.ID), false, map[string]string{"install_id": "11111111-1111-4111-8111-111111111111"}, nil); code != 404 {
		t.Fatalf("vote on bug: %d", code)
	}
}

func TestChecklistTodosStatsOverview(t *testing.T) {
	srv := newServer(t)
	items := []map[string]any{
		{"id": "play-account", "phase": "Play Console & listing", "title": "Create Play developer account", "products": []string{"All"}, "status": "todo"},
		{"id": "ci-test-apks", "phase": "Build & test", "title": "Test APKs via GitHub Actions", "products": []string{"All"}, "status": "done"},
	}
	if code := call(t, srv, "PUT", "/v1/checklist", true, items, nil); code != 200 {
		t.Fatalf("import: %d", code)
	}
	// The Android client sends PATCH as POST + X-HTTP-Method-Override.
	override := map[string]string{"X-HTTP-Method-Override": "PATCH"}
	if code := callH(t, srv, "POST", "/v1/checklist/play-account", true, override, map[string]string{"status": "in-progress"}, nil); code != 200 {
		t.Fatalf("patch: %d", code)
	}
	due := time.Now().Format(time.DateOnly)
	if code := call(t, srv, "POST", "/v1/todos", true, map[string]any{"title": "Add AUDD_API_TOKEN secret", "due": due, "product": "mp3"}, nil); code != 201 {
		t.Fatalf("todo: %d", code)
	}
	day := time.Now().AddDate(0, 0, -1).Format(time.DateOnly)
	stats := []map[string]any{{"product": "mp3", "day": day, "installs": 120, "pro_sales": 4, "pro_revenue_paise": 67660}}
	if code := call(t, srv, "PUT", "/v1/stats", true, stats, nil); code != 200 {
		t.Fatalf("stats: %d", code)
	}
	call(t, srv, "POST", "/v1/public/reports", false, report("bug", "0e0e0e0e-eeee-4eee-8eee-eeeeeeeeeeee"), nil)

	var o store.Overview
	call(t, srv, "GET", "/v1/overview", true, nil, &o)
	if o.ChecklistTotal != 2 || o.ChecklistDone != 1 || o.ChecklistDoing != 1 || o.NextChecklist == nil || o.NextChecklist.ID != "play-account" {
		t.Fatalf("checklist overview = %+v", o)
	}
	if len(o.OpenTodos) != 1 || o.OpenTickets != 1 || o.WaitingOnOwner != 1 || o.Last7Days.Installs != 120 || o.Last7Days.ProRevenuePaise != 67660 {
		t.Fatalf("overview = %+v", o)
	}
}
