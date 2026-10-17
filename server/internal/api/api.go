// Package api is the HTTP layer: owner endpoints (bearer token) and public endpoints the product apps call.
package api

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/vikash-parashar/ampli_owner/server/internal/admob"
	"github.com/vikash-parashar/ampli_owner/server/internal/ads"
	"github.com/vikash-parashar/ampli_owner/server/internal/play"
	"github.com/vikash-parashar/ampli_owner/server/internal/secret"
	"github.com/vikash-parashar/ampli_owner/server/internal/store"
)

// AuthFailuresPerDay is how many wrong owner tokens one network may send per day.
const AuthFailuresPerDay = 20

// ReportsPerDay is how many reports one install may file in 24 hours.
const ReportsPerDay = 5

type Server struct {
	Store      *store.Store
	OwnerToken string
	Now        func() time.Time

	// Ads manager and Play reviews; all optional.
	Box       *secret.Box             // encrypts ad tokens (TOKEN_KEY)
	PublicURL string                  // where the platforms send the browser back, e.g. https://abc.lambda-url.ap-south-1.on.aws
	Platforms map[string]ads.Platform // meta, tiktok, google
	Play      *play.Client

	AdMob          *admob.Client // ad earnings (reuses the Google OAuth client)
	PlayBucket     string        // Play Console bulk reports bucket, pubsite_prod_rev_...
	OwnerTokenNext string        // accepted alongside OwnerToken while rotating it
}

func (s *Server) Handler() http.Handler {
	if s.Now == nil {
		s.Now = time.Now
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) { writeJSON(w, 200, map[string]string{"status": "ok"}) })

	// Public: called by Ampli MP3 Player and Ampli Car Hub (see in-app-hook-spec.md).
	mux.HandleFunc("POST /v1/public/reports", s.createReport)
	mux.HandleFunc("GET /v1/public/reports", s.myReports)
	mux.HandleFunc("GET /v1/public/features", s.publicFeatures)
	mux.HandleFunc("POST /v1/public/features/{id}/vote", s.vote)
	mux.HandleFunc("POST /v1/public/installs", s.publicInstall)
	mux.HandleFunc("POST /v1/public/purchases", s.publicPurchase)
	mux.HandleFunc("POST /v1/public/events", s.publicEvents)
	mux.HandleFunc("POST /v1/public/promo/claim", s.publicClaimPromo)
	mux.HandleFunc("GET /oauth/{platform}/callback", s.oauthCallback)

	// Owner only.
	owner := http.NewServeMux()
	owner.HandleFunc("GET /v1/overview", s.overview)
	owner.HandleFunc("GET /v1/tickets", s.listTickets)
	owner.HandleFunc("POST /v1/tickets", s.createTicket)
	owner.HandleFunc("GET /v1/tickets/{id}", s.getTicket)
	owner.HandleFunc("PATCH /v1/tickets/{id}", s.patchTicket)
	owner.HandleFunc("POST /v1/tickets/{id}/messages", s.ownerReply)
	owner.HandleFunc("GET /v1/checklist", s.checklist)
	owner.HandleFunc("PUT /v1/checklist", s.importChecklist)
	owner.HandleFunc("PATCH /v1/checklist/{id}", s.patchChecklist)
	owner.HandleFunc("GET /v1/todos", s.todos)
	owner.HandleFunc("POST /v1/todos", s.createTodo)
	owner.HandleFunc("PATCH /v1/todos/{id}", s.patchTodo)
	owner.HandleFunc("DELETE /v1/todos/{id}", s.deleteTodo)
	owner.HandleFunc("GET /v1/features", s.features)
	owner.HandleFunc("POST /v1/features", s.createFeature)
	owner.HandleFunc("PATCH /v1/features/{id}", s.patchFeature)
	owner.HandleFunc("GET /v1/stats", s.stats)
	owner.HandleFunc("PUT /v1/stats", s.putStats)
	owner.HandleFunc("GET /v1/dashboard", s.dashboard)
	owner.HandleFunc("GET /v1/ads/platforms", s.adPlatforms)
	owner.HandleFunc("POST /v1/ads/connect/{platform}", s.connectAds)
	owner.HandleFunc("DELETE /v1/ads/accounts/{id}", s.deleteAdAccount)
	owner.HandleFunc("GET /v1/ads/campaigns", s.adCampaigns)
	owner.HandleFunc("POST /v1/ads/campaigns", s.createAdCampaign)
	owner.HandleFunc("POST /v1/ads/campaigns/{id}/{action}", s.campaignAction)
	owner.HandleFunc("GET /v1/ads/campaigns/{id}/log", s.campaignLog)
	owner.HandleFunc("DELETE /v1/ads/campaigns/{id}", s.deleteCampaign)
	owner.HandleFunc("GET /v1/alerts", s.alerts)
	owner.HandleFunc("GET /v1/audit", s.auditLog)
	owner.HandleFunc("GET /v1/integrations", s.integrations)
	owner.HandleFunc("POST /v1/integrations/admob/connect", s.connectAdMob)
	owner.HandleFunc("DELETE /v1/integrations/admob", s.disconnectAdMob)
	owner.HandleFunc("POST /v1/integrations/sync", s.syncIntegrations)
	owner.HandleFunc("GET /v1/skus", s.skus)
	owner.HandleFunc("PUT /v1/skus", s.saveSKU)
	owner.HandleFunc("DELETE /v1/installs/{install}", s.eraseInstall)
	owner.HandleFunc("GET /v1/links", s.links)
	owner.HandleFunc("POST /v1/links", s.createLink)
	owner.HandleFunc("DELETE /v1/links/{id}", s.deleteLink)
	owner.HandleFunc("GET /v1/promo", s.promoReport)
	owner.HandleFunc("POST /v1/promo/codes", s.importPromoCodes)
	owner.HandleFunc("POST /v1/ads/sync", s.syncAds)
	owner.HandleFunc("GET /v1/reviews", s.reviews)
	owner.HandleFunc("POST /v1/reviews", s.addReview)
	owner.HandleFunc("POST /v1/reviews/{id}/reply", s.replyReview)
	owner.HandleFunc("POST /v1/reviews/sync", s.syncReviews)
	owner.HandleFunc("GET /v1/guides", s.guides)
	owner.HandleFunc("PUT /v1/guides/{id}", s.saveGuide)
	mux.Handle("/v1/", s.requireOwner(owner))

	return logRequests(methodOverride(mux))
}

// methodOverride lets clients without PATCH support (Android's HttpURLConnection) send
// POST with X-HTTP-Method-Override: PATCH.
func methodOverride(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && strings.EqualFold(r.Header.Get("X-HTTP-Method-Override"), "PATCH") {
			r.Method = http.MethodPatch
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) requireOwner(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
		match := func(want string) bool {
			return want != "" && subtle.ConstantTimeCompare([]byte(got), []byte(want)) == 1
		}
		if !ok || !(match(s.OwnerToken) || match(s.OwnerTokenNext)) {
			// Throttle guessing: 20 wrong tokens per network per day, then 429 for everyone on it.
			if allowed, err := s.Store.IPHit(r.Context(), s.hashKey("auth", clientNet(r)), s.today(), AuthFailuresPerDay); err == nil && !allowed {
				writeErr(w, http.StatusTooManyRequests, "too many wrong tokens today")
				return
			}
			writeErr(w, http.StatusUnauthorized, "owner token required")
			return
		}
		next.ServeHTTP(w, r)
	})
}

func logRequests(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		next.ServeHTTP(w, r)
		slog.Info("request", "method", r.Method, "path", r.URL.Path, "ms", time.Since(start).Milliseconds())
	})
}

// ---- helpers ----

type apiError struct {
	Error string `json:"error"`
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, code int, msg string) { writeJSON(w, code, apiError{msg}) }

func decode(w http.ResponseWriter, r *http.Request, v any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return false
	}
	return true
}

func pathID(w http.ResponseWriter, r *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(strings.TrimPrefix(r.PathValue("id"), "T-"), 10, 64)
	if err != nil || id <= 0 {
		writeErr(w, http.StatusBadRequest, "bad id")
		return 0, false
	}
	return id, true
}

// fail maps store errors to HTTP responses.
func fail(w http.ResponseWriter, r *http.Request, err error) {
	if errors.Is(err, store.ErrNotFound) {
		writeErr(w, http.StatusNotFound, "not found")
		return
	}
	if errors.Is(err, context.Canceled) {
		return
	}
	slog.Error("handler", "path", r.URL.Path, "err", err)
	writeErr(w, http.StatusInternalServerError, "internal error")
}

func oneOf(v string, allowed ...string) bool {
	for _, a := range allowed {
		if v == a {
			return true
		}
	}
	return false
}

func lenBetween(s string, min, max int) bool {
	n := utf8.RuneCountInString(strings.TrimSpace(s))
	return n >= min && n <= max
}

// productID accepts the app-facing names ("mp3", "carhub", "naukri").
func productID(p string) string {
	switch p {
	case "mp3", "carhub", "naukri":
		return p
	}
	return ""
}
