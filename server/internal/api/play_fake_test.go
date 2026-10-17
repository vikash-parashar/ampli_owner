package api_test

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/vikash-parashar/ampli_owner/server/internal/play"
)

// fakePlay answers token and purchase calls like Google Play. Tokens starting with "good-" are paid,
// "test-" are license-tester purchases, anything else is rejected.
// playDown makes "flaky-" tokens fail with 503 until a test sets it false.
var playDown atomic.Bool

func fakePlay(t *testing.T) *play.Client {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("POST /token", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{"access_token": "play-token", "expires_in": 3600})
	})
	mux.HandleFunc("GET /applications/{pkg}/purchases/products/{sku}/tokens/{tok}", func(w http.ResponseWriter, r *http.Request) {
		tok := r.PathValue("tok")
		switch {
		case strings.HasPrefix(tok, "good-"):
			json.NewEncoder(w).Encode(map[string]any{"purchaseState": 0, "orderId": "GPA." + tok, "regionCode": "IN"})
		case strings.HasPrefix(tok, "flaky-") && playDown.Load():
			w.WriteHeader(503)
			w.Write([]byte(`{"error":{"message":"Backend Error"}}`))
		case strings.HasPrefix(tok, "flaky-"):
			json.NewEncoder(w).Encode(map[string]any{"purchaseState": 0, "orderId": "GPA." + tok, "regionCode": "IN"})
		case strings.HasPrefix(tok, "pend-"):
			json.NewEncoder(w).Encode(map[string]any{"purchaseState": 2, "orderId": "GPA." + tok})
		case strings.HasPrefix(tok, "promo-"):
			json.NewEncoder(w).Encode(map[string]any{"purchaseState": 0, "orderId": "GPA." + tok, "regionCode": "IN", "purchaseType": 1})
		case strings.HasPrefix(tok, "test-"):
			json.NewEncoder(w).Encode(map[string]any{"purchaseState": 0, "orderId": "GPA." + tok, "regionCode": "IN", "purchaseType": 0})
		default:
			w.WriteHeader(400)
			w.Write([]byte(`{"error":{"message":"Invalid purchase token"}}`))
		}
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	key, _ := rsa.GenerateKey(rand.Reader, 2048)
	der, _ := x509.MarshalPKCS8PrivateKey(key)
	sa, _ := json.Marshal(map[string]string{"client_email": "owner@test.iam.gserviceaccount.com",
		"private_key": string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der})), "token_uri": srv.URL + "/token"})
	c, err := play.New(string(sa))
	if err != nil {
		t.Fatal(err)
	}
	c.Base = srv.URL
	return c
}
