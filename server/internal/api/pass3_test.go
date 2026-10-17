package api_test

import (
	"archive/zip"
	"bytes"
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
		"GPA.other,Charge,com.vikash.ampli,ampli_pro,IN,INR,199.00\n"))
	zw.Close()
	playObjects["earnings/earnings_"+month.Format("200601")+"_1234-1.zip"] = zb.Bytes()
	t.Cleanup(func() { clear(playObjects) })

	srv, _ := newServerWith(t, func(s *api.Server) { s.Play = fakePlay(t); s.PlayBucket = "pubsite_prod_rev_1" })
	call(t, srv, "POST", "/v1/public/purchases", false, map[string]any{"product": "mp3", "install_id": "44444444-4444-4444-8444-444444444444",
		"kind": "pro", "sku": "ampli_pro", "purchase_token": "good-usa-token-1"}, nil)
	var res api.SyncResult
	call(t, srv, "POST", "/v1/integrations/sync", true, nil, &res)
	if res.ExactNet != 1 {
		t.Fatalf("sync = %+v", res)
	}
	var d struct {
		Revenue30d []int64 `json:"revenue_30d"`
	}
	call(t, srv, "GET", "/v1/dashboard?product=mp3", true, nil, &d)
	if d.Revenue30d[len(d.Revenue30d)-1] != 21250 {
		t.Fatalf("revenue today = %v, want 21250 paise from the earnings report", d.Revenue30d[len(d.Revenue30d)-1])
	}
	call(t, srv, "POST", "/v1/integrations/sync", true, nil, &res)
	if res.ExactNet != 0 {
		t.Fatalf("second sync rewrote %d rows", res.ExactNet)
	}
}
