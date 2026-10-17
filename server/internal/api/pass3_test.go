package api_test

import (
	"encoding/base64"

	"archive/zip"
	"bytes"
	"context"
	"github.com/vikash-parashar/ampli_owner/server/internal/secret"
	"testing"
	"time"

	"github.com/vikash-parashar/ampli_owner/server/internal/api"
)

// A service account without access must not make the apps drop real sales, and the owner hears about it.
func TestPlayAccessErrorKeepsPurchase(t *testing.T) {
	now := time.Now()
	srv, _ := newServerWith(t, func(s *api.Server) {
		s.Play = fakePlay(t)
		s.Now = func() time.Time { return now }
	})
	p := map[string]any{"product": "mp3", "install_id": "33333333-3333-4333-8333-333333333333", "kind": "pro",
		"sku": "ampli_pro", "purchase_token": "auth-blocked-token-1"}
	if code := call(t, srv, "POST", "/v1/public/purchases", false, p, nil); code != 503 {
		t.Fatalf("403 from Google answered %d, want 503 (retry)", code)
	}
	if code := call(t, srv, "POST", "/v1/public/purchases", false, map[string]any{"product": "mp3",
		"install_id": "33333333-3333-4333-8333-333333333333", "kind": "pro", "sku": "ampli_pro", "purchase_token": "bogus-token-123"}, nil); code != 422 {
		t.Fatalf("400 from Google answered %d, want 422", code)
	}
	now = now.Add(7 * time.Hour)
	var alerts []api.Alert
	call(t, srv, "GET", "/v1/alerts", true, nil, &alerts)
	found := false
	for _, a := range alerts {
		found = found || (a.ID == "purchases-unchecked" && a.Severity == "high")
	}
	if !found {
		t.Fatalf("no unchecked-purchases alert: %+v", alerts)
	}
}

func TestEarningsReportSetsExactNet(t *testing.T) {
	month := time.Now().UTC().AddDate(0, -1, 0)
	var zb bytes.Buffer
	zw := zip.NewWriter(&zb)
	f, _ := zw.Create("PlayApps_" + month.Format("200601") + ".csv")
	f.Write([]byte("Description,Transaction Type,Product id,Sku Id,Buyer Country,Merchant Currency,Amount (Merchant Currency)\n" +
		"GPA.good-usa-token-1,Charge,com.vikash.ampli,ampli_pro,US,INR,250.00\n" +
		"GPA.good-usa-token-1,Google fee,com.vikash.ampli,ampli_pro,US,INR,-37.50\n" +
		"GPA.good-india-token-1,Charge,com.vikash.ampli,ampli_pro,IN,INR,199.00\n" +
		"GPA.good-india-token-1,Google fee,com.vikash.ampli,ampli_pro,IN,INR,-25.31\n" +
		"GPA.other,Charge,com.vikash.ampli,ampli_pro,IN,INR,199.00\n"))
	zw.Close()
	playObjects["earnings/earnings_"+month.Format("200601")+"_1234-1.zip"] = zb.Bytes()
	// The month before holds nothing for these orders; a refund-only order there must not be priced.
	prev := month.AddDate(0, -1, 0)
	playObjects["earnings/earnings_"+prev.Format("200601")+"_1234-1.csv"] = []byte("Description,Transaction Type,Buyer Country,Merchant Currency,Amount (Merchant Currency)\n" +
		"GPA.refund-only,Charge refund,IN,INR,-199.00\n")
	t.Cleanup(func() { clear(playObjects) })

	srv, _ := newServerWith(t, func(s *api.Server) { s.Play = fakePlay(t); s.PlayBucket = "pubsite_prod_rev_1" })
	call(t, srv, "POST", "/v1/public/purchases", false, map[string]any{"product": "mp3", "install_id": "44444444-4444-4444-8444-444444444444",
		"kind": "pro", "sku": "ampli_pro", "purchase_token": "good-usa-token-1"}, nil)
	call(t, srv, "POST", "/v1/public/purchases", false, map[string]any{"product": "mp3", "install_id": "66666666-6666-4666-8666-666666666666",
		"kind": "pro", "sku": "ampli_pro", "purchase_token": "good-india-token-1"}, nil)
	var res api.SyncResult
	call(t, srv, "POST", "/v1/integrations/sync", true, nil, &res)
	if res.ExactNet != 2 {
		t.Fatalf("sync = %+v", res)
	}
	var d struct {
		Revenue30d []int64 `json:"revenue_30d"`
	}
	call(t, srv, "GET", "/v1/dashboard?product=mp3", true, nil, &d)
	if d.Revenue30d[len(d.Revenue30d)-1] != 21250+14333 {
		t.Fatalf("revenue today = %v, want 35583 paise from the earnings report (IN without GST)", d.Revenue30d[len(d.Revenue30d)-1])
	}
	call(t, srv, "POST", "/v1/integrations/sync", true, nil, &res)
	if res.ExactNet != 0 {
		t.Fatalf("second sync rewrote %d rows", res.ExactNet)
	}
}

// A campaign that paid back early but has stopped earning is judged on its last 7 days once it has 14+ days.
func TestVerdictUsesLastSevenDays(t *testing.T) {
	srv, st := newServerWith(t, nil)
	ctx := context.Background()
	var id int64
	if err := st.DB.QueryRow(ctx, `INSERT INTO campaigns (platform, product_id, name, status, daily_budget_paise, revenue_paise, utm_campaign)
		VALUES ('google', 'mp3', 'Old winner', 'live', 10000, 400000, 'ampli-c900') RETURNING id`).Scan(&id); err != nil {
		t.Fatal(err)
	}
	if _, err := st.DB.Exec(ctx, `INSERT INTO campaign_days (campaign_id, day, spend_paise)
		SELECT $1, current_date - g, 10000 FROM generate_series(0, 19) g`, id); err != nil {
		t.Fatal(err)
	}
	st.DB.Exec(ctx, `UPDATE campaigns SET spend_paise = 200000 WHERE id = $1`, id)
	call(t, srv, "POST", "/v1/public/installs", false, map[string]string{"product": "mp3",
		"install_id": "55555555-5555-4555-8555-555555555555", "referrer": "utm_source=google&utm_campaign=ampli-c900"}, nil)
	var res struct {
		Campaigns []struct {
			ROAS     float64 `json:"roas"`
			ROAS7    float64 `json:"roas_7d"`
			JudgedOn string  `json:"judged_on"`
			Verdict  struct{ Code string }
		}
	}
	call(t, srv, "GET", "/v1/ads/campaigns?product=mp3", true, nil, &res)
	if len(res.Campaigns) != 1 {
		t.Fatalf("campaigns = %+v", res)
	}
	c := res.Campaigns[0]
	if c.ROAS != 2 || c.ROAS7 != 0 || c.JudgedOn != "last_7_days" || c.Verdict.Code != "stop" {
		t.Fatalf("campaign = %+v", c)
	}
}

// A purchase Google couldn't confirm is re-checked by the daily sync, without waiting for the app to retry.
func TestSyncRechecksUnconfirmedPurchases(t *testing.T) {
	box, _ := secret.New(base64.StdEncoding.EncodeToString([]byte("0123456789abcdef0123456789abcdef")))
	srv, st := newServerWith(t, func(s *api.Server) { s.Play = fakePlay(t); s.Box = box })
	playDown.Store(true)
	t.Cleanup(func() { playDown.Store(false) })
	p := map[string]any{"product": "mp3", "install_id": "77777777-7777-4777-8777-777777777777", "kind": "pro",
		"sku": "ampli_pro", "purchase_token": "flaky-recheck-token-1"}
	if code := call(t, srv, "POST", "/v1/public/purchases", false, p, nil); code != 503 {
		t.Fatalf("google down: %d", code)
	}
	var sealed int
	st.DB.QueryRow(context.Background(), `SELECT count(*) FROM purchases WHERE token_enc IS NOT NULL AND verify_error <> ''`).Scan(&sealed)
	if sealed != 1 {
		t.Fatalf("unconfirmed purchase kept no sealed token (%d)", sealed)
	}
	playDown.Store(false)
	var res api.SyncResult
	call(t, srv, "POST", "/v1/integrations/sync", true, nil, &res)
	if res.Rechecked != 1 {
		t.Fatalf("sync = %+v", res)
	}
	var verified bool
	var left []byte
	st.DB.QueryRow(context.Background(), `SELECT verified, token_enc FROM purchases`).Scan(&verified, &left)
	if !verified || left != nil {
		t.Fatalf("after recheck verified=%v token kept=%v", verified, left != nil)
	}
}
