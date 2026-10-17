package admob

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestEarnings(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`[{"header":{}},
		 {"row":{"dimensionValues":{"DATE":{"value":"20261016"},"APP":{"value":"ca-app-pub-1~1"}},"metricValues":{"ESTIMATED_EARNINGS":{"microsValue":"125500000"}}}},
		 {"row":{"dimensionValues":{"DATE":{"value":"20261016"},"APP":{"value":"ca-app-pub-1~9"}},"metricValues":{"ESTIMATED_EARNINGS":{"microsValue":"1000000"}}}},
		 {"footer":{"matchingRowCount":"2"}}]`))
	}))
	defer ts.Close()
	c := New("id", "secret", "mp3=ca-app-pub-1~1")
	c.Base = ts.URL
	got, err := c.Earnings(context.Background(), Token{Access: "x"}, "accounts/pub-1", time.Now(), time.Now())
	if err != nil || len(got) != 1 || got[0].Product != "mp3" || got[0].Paise != 12550 {
		t.Fatalf("earnings = %+v, %v", got, err)
	}
}
