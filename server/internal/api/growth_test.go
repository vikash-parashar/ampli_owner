package api_test

import (
	"context"
	"fmt"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/vikash-parashar/ampli_owner/server/internal/api"
	"github.com/vikash-parashar/ampli_owner/server/internal/store"
)

type linkOut struct {
	store.Link
	ChannelName string `json:"channel_name"`
	URL         string `json:"url"`
}

func TestTrackedLinksAndChannels(t *testing.T) {
	srv, _ := newServerWith(t, func(s *api.Server) { s.Play = fakePlay(t) })
	var wa, hi linkOut
	if code := call(t, srv, "POST", "/v1/links", true, map[string]string{"product": "mp3", "channel": "whatsapp", "label": "Family groups"}, &wa); code != 201 {
		t.Fatalf("create: %d", code)
	}
	call(t, srv, "POST", "/v1/links", true, map[string]string{"product": "mp3", "channel": "play_hindi"}, &hi)
	u, _ := url.Parse(hi.URL)
	ref, _ := url.ParseQuery(u.Query().Get("referrer"))
	if u.Query().Get("id") != "com.vikash.ampli" || u.Query().Get("listing") != "hindi" || ref.Get("utm_campaign") != "ampli-l2" || ref.Get("utm_source") != "play_hindi" {
		t.Fatalf("hindi url = %s", hi.URL)
	}
	for _, bad := range []map[string]string{{"product": "mp3", "channel": "sms"}, {"product": "mp3", "channel": "whatsapp", "from_product": "carhub"},
		{"product": "mp3", "channel": "crosspromo", "from_product": "mp3"}} {
		if code := call(t, srv, "POST", "/v1/links", true, bad, nil); code != 400 {
			t.Fatalf("bad link %v: %d", bad, code)
		}
	}

	// An unpaid Instagram link must not count as a paid Meta install.
	insta := linkOut{}
	call(t, srv, "POST", "/v1/links", true, map[string]string{"product": "mp3", "channel": "instagram"}, &insta)
	instaRef := strings.SplitN(insta.URL, "referrer=", 2)[1]
	instaRef, _ = url.QueryUnescape(instaRef)
	refs := []string{waRef(wa.URL), waRef(wa.URL), instaRef, "utm_source=crosspromo&utm_medium=app&utm_content=carhub",
		"utm_source=google-play&utm_medium=organic", "utm_source=apps.facebook.com&utm_campaign=fb4a", "utm_campaign=ampli-lzz"}
	for i, ref := range refs {
		in := map[string]string{"product": "mp3", "install_id": fmt.Sprintf("00000000-0000-4000-8000-%012d", i), "referrer": ref}
		if code := call(t, srv, "POST", "/v1/public/installs", false, in, nil); code != 200 {
			t.Fatalf("install %d: %d", i, code)
		}
	}
	p := map[string]any{"product": "mp3", "install_id": "00000000-0000-4000-8000-000000000000", "kind": "pro", "sku": "ampli_pro", "purchase_token": "good-wa-token-0"}
	call(t, srv, "POST", "/v1/public/purchases", false, p, nil)

	var ls []linkOut
	call(t, srv, "GET", "/v1/links?product=mp3", true, nil, &ls)
	if len(ls) != 3 || ls[0].ID != wa.ID || ls[0].Installs30 != 2 || ls[0].ProSales != 1 || ls[0].NetPaise != 14334 || ls[0].ChannelName != "WhatsApp" {
		t.Fatalf("links = %+v", ls)
	}

	var d struct{ Widgets []api.Widget }
	call(t, srv, "GET", "/v1/dashboard?product=mp3", true, nil, &d)
	ch := d.Widgets[len(d.Widgets)-1]
	got := map[string]int64{}
	for _, r := range ch.Rows {
		got[r.Label] = r.Value
	}
	want := map[string]int64{"WhatsApp": 2, "Instagram (unpaid)": 1, "More from Ampli": 1, "Play search and browse": 1, "Ads: Facebook + Instagram": 1, "Other or unknown": 1}
	if ch.ID != "channels" || fmt.Sprint(got) != fmt.Sprint(want) || ch.Rows[0].Label != "WhatsApp" {
		t.Fatalf("channels = %+v", ch)
	}

	if code := call(t, srv, "DELETE", fmt.Sprintf("/v1/links/%d", wa.ID), true, nil, nil); code != 204 {
		t.Fatalf("delete: %d", code)
	}
	call(t, srv, "GET", "/v1/links", true, nil, &ls)
	if len(ls) != 2 {
		t.Fatalf("after delete %d links", len(ls))
	}
}

func waRef(u string) string {
	parsed, _ := url.Parse(u)
	return parsed.Query().Get("referrer")
}

func TestCrossPromoCodes(t *testing.T) {
	now := time.Now()
	srv, st := newServerWith(t, func(s *api.Server) { s.Play = fakePlay(t); s.Now = func() time.Time { return now } })
	buyer, other := "11111111-1111-4111-8111-111111111111", "22222222-2222-4222-8222-222222222222"
	claim := func(id string, out any) int {
		return call(t, srv, "POST", "/v1/public/promo/claim", false, map[string]string{"product": "carhub", "install_id": id}, out)
	}
	if code := claim(buyer, nil); code != 403 {
		t.Fatalf("claim without Pro: %d", code)
	}
	call(t, srv, "POST", "/v1/public/purchases", false, map[string]any{"product": "carhub", "install_id": buyer, "kind": "pro",
		"sku": "ampli_drive_pro", "purchase_token": "good-car-1"}, nil)
	var early struct {
		AvailableAt time.Time `json:"available_at"`
	}
	if code := claim(buyer, &early); code != 409 || early.AvailableAt.Before(now.Add(47*time.Hour)) {
		t.Fatalf("claim inside the refund window: %d %+v", code, early)
	}
	if _, err := st.DB.Exec(context.Background(), `UPDATE purchases SET created_at = now() - interval '49 hours'`); err != nil {
		t.Fatal(err)
	}
	if code := claim(buyer, nil); code != 410 {
		t.Fatalf("claim with no codes: %d", code)
	}
	var imp struct{ Added, Skipped int64 }
	body := map[string]any{"source_product": "carhub", "target_product": "mp3", "sku": "ampli_pro", "codes": []string{"abcd1234efgh", "ABCD1234EFGH", "ZZZZ9999YYYY"}}
	if code := call(t, srv, "POST", "/v1/promo/codes", true, body, &imp); code != 200 || imp.Added != 2 || imp.Skipped != 1 {
		t.Fatalf("import: %d %+v", code, imp)
	}
	var c1, c2 struct {
		Code      string `json:"code"`
		RedeemURL string `json:"redeem_url"`
	}
	claim(buyer, &c1)
	claim(buyer, &c2)
	if c1.Code != "ABCD1234EFGH" || c2.Code != c1.Code || !strings.Contains(c1.RedeemURL, "code=ABCD1234EFGH") {
		t.Fatalf("claims = %+v %+v", c1, c2)
	}
	if code := claim(other, nil); code != 403 {
		t.Fatalf("non-buyer claim: %d", code)
	}
	// The MP3 app reports the redeemed promo like any purchase; it is counted, not paid.
	var res struct{ Recorded, Verified bool }
	call(t, srv, "POST", "/v1/public/purchases", false, map[string]any{"product": "mp3", "install_id": other, "kind": "pro",
		"sku": "ampli_pro", "purchase_token": "promo-redeem-1"}, &res)
	if !res.Recorded || res.Verified {
		t.Fatalf("promo purchase = %+v", res)
	}
	var rep []store.PromoReport
	call(t, srv, "GET", "/v1/promo", true, nil, &rep)
	if len(rep) != 1 || rep[0].Imported != 2 || rep[0].Available != 1 || rep[0].Eligible != 1 || rep[0].Claimed != 1 || rep[0].Redeemed != 1 {
		t.Fatalf("report = %+v", rep)
	}
	var alerts []api.Alert
	call(t, srv, "GET", "/v1/alerts", true, nil, &alerts)
	found := false
	for _, a := range alerts {
		found = found || a.ID == "promo-low-carhub-mp3"
	}
	if found {
		t.Fatalf("low-codes alert with every buyer served: %+v", alerts)
	}
	if err := st.MarkRefunded(context.Background(), "GPA.good-car-1", now); err != nil {
		t.Fatal(err)
	}
	call(t, srv, "GET", "/v1/promo", true, nil, &rep)
	if rep[0].Refunded != 1 || rep[0].Eligible != 0 {
		t.Fatalf("refunded claimer not reported: %+v", rep)
	}
	var erased struct {
		Rows int64 `json:"erased_rows"`
	}
	call(t, srv, "DELETE", "/v1/installs/"+buyer, true, nil, &erased)
	call(t, srv, "GET", "/v1/promo", true, nil, &rep)
	if rep[0].Available != 1 || rep[0].Claimed != 1 {
		t.Fatalf("erase gave the code out again: %+v", rep)
	}
}
