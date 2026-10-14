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

	"github.com/vikash-parashar/ampli_owner/server/internal/store"
)

// ReportsPerDay is how many reports one install may file in 24 hours.
const ReportsPerDay = 5

type Server struct {
	Store      *store.Store
	OwnerToken string
	Now        func() time.Time
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
	owner.HandleFunc("GET /v1/campaigns", s.campaigns)
	owner.HandleFunc("POST /v1/campaigns", s.saveCampaign)
	owner.HandleFunc("PUT /v1/campaigns/{id}", s.saveCampaign)
	owner.HandleFunc("GET /v1/stats", s.stats)
	owner.HandleFunc("PUT /v1/stats", s.putStats)
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
		if s.OwnerToken == "" || !ok || subtle.ConstantTimeCompare([]byte(got), []byte(s.OwnerToken)) != 1 {
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

// productID accepts the app-facing names ("mp3", "carhub").
func productID(p string) string {
	switch p {
	case "mp3", "carhub":
		return p
	}
	return ""
}
