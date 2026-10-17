package ads

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestJudge(t *testing.T) {
	cases := []struct {
		status     string
		spend, rev int64
		days       int
		want       string
	}{
		{"in_review", 0, 0, 0, "new"},
		{"live", 100, 0, 3, "early"},
		{"live", 1000, 1600, 7, "scale"},
		{"live", 1000, 900, 7, "watch"},
		{"live", 1000, 100, 7, "stop"},
	}
	for _, c := range cases {
		if got := Judge(c.status, c.spend, c.rev, c.days); got.Code != c.want {
			t.Errorf("Judge(%v) = %s, want %s", c, got.Code, c.want)
		}
	}
}

func TestParseReferrer(t *testing.T) {
	for raw, want := range map[string]string{
		"utm_source=apps.facebook.com&utm_campaign=fb4a&utm_content=x": "meta",
		"utm_source=apps.instagram.com":                                "meta",
		"utm_source=tiktok&utm_medium=paid":                            "tiktok",
		"gclid=abc123":                                                 "google",
		"utm_source=google-play&utm_medium=organic":                    "",
		"utm_source=meta&utm_campaign=ampli-c7":                        "",
		"utm_source=instagram&utm_medium=social&utm_campaign=ampli-l3": "",
	} {
		if got := ParseReferrer(raw).Platform; got != want {
			t.Errorf("%q: got %q want %q", raw, got, want)
		}
	}
}

func TestMoney(t *testing.T) {
	if ToPaise(12.34, "INR") != 1234 || ToPaise(1, "USD") != 8400 || minor(8400, "USD") != "100" || minor(1234, "INR") != "1234" {
		t.Fatal("currency conversion")
	}
}

func TestMetaInsights(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/act_1/insights" || r.URL.Query().Get("level") != "campaign" {
			http.Error(w, `{"error":{"message":"wrong call"}}`, 400)
			return
		}
		w.Write([]byte(`{"data":[{"campaign_id":"9","date_start":"2026-10-16","spend":"250.50","impressions":"1000","clicks":"40",
			"actions":[{"action_type":"link_click","value":"40"},{"action_type":"mobile_app_install","value":"12"}]}]}`))
	}))
	defer ts.Close()
	m := NewMeta("app", "secret", "page", "mp3=1", "")
	m.Graph = ts.URL
	got, err := m.Insights(context.Background(), Token{Access: "t"}, Account{ExternalID: "act_1", Currency: "INR"}, "2026-10-01", "2026-10-16")
	if err != nil || len(got) != 1 || got[0].SpendPaise != 25050 || got[0].Installs != 12 || got[0].Clicks != 40 {
		t.Fatalf("insights = %+v, %v", got, err)
	}
	// Errors carry the platform's own message.
	_, err = m.Insights(context.Background(), Token{Access: "t"}, Account{ExternalID: "act_2"}, "a", "b")
	if err == nil || err.Error() != "Meta: wrong call (HTTP 400)" {
		t.Fatalf("err = %v", err)
	}
}
