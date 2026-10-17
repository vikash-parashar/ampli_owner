package api

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/vikash-parashar/ampli_owner/server/internal/admob"
	"github.com/vikash-parashar/ampli_owner/server/internal/play"
	"github.com/vikash-parashar/ampli_owner/server/internal/store"
)

type integrationView struct {
	Configured bool       `json:"configured"`
	Connected  bool       `json:"connected"`
	Account    string     `json:"account"`
	LastSyncAt *time.Time `json:"last_sync_at"`
	LastError  string     `json:"last_error"`
}

func (s *Server) admobReady() bool {
	return s.AdMob != nil && s.AdMob.Configured() && s.Box != nil && s.PublicURL != ""
}

func (s *Server) integrations(w http.ResponseWriter, r *http.Request) {
	rows, err := s.Store.Integrations(r.Context())
	if err != nil {
		fail(w, r, err)
		return
	}
	am, pl := rows["admob"], rows["play"]
	writeJSON(w, http.StatusOK, map[string]integrationView{
		"admob": {Configured: s.admobReady(), Connected: am.Connected, Account: am.Account, LastSyncAt: am.LastSyncAt, LastError: am.LastError},
		"play":  {Configured: s.Play != nil, Connected: s.Play != nil, LastSyncAt: pl.LastSyncAt, LastError: pl.LastError},
	})
}

func (s *Server) connectAdMob(w http.ResponseWriter, r *http.Request) {
	if !s.admobReady() {
		writeErr(w, http.StatusConflict, "set GOOGLE_ADS_CLIENT_ID/SECRET, ADMOB_APPS, TOKEN_KEY and PUBLIC_URL on the server first")
		return
	}
	state := s.Box.State("admob", s.Now(), 15*time.Minute)
	writeJSON(w, http.StatusOK, map[string]string{"auth_url": s.AdMob.AuthURL(state, s.redirectURL("admob"))})
}

func (s *Server) disconnectAdMob(w http.ResponseWriter, r *http.Request) {
	if err := s.Store.DeleteIntegration(r.Context(), "admob"); err != nil {
		fail(w, r, err)
		return
	}
	s.audit(r, "admob disconnect", "")
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) admobCallback(r *http.Request, page func(int, string, string)) {
	q := r.URL.Query()
	if !s.admobReady() {
		page(http.StatusNotFound, "AdMob is not set up", "")
		return
	}
	if got, err := s.Box.CheckState(q.Get("state"), s.Now()); err != nil || got != "admob" || !s.stateOnce(r) {
		page(http.StatusBadRequest, "Link expired", "Start again from Ampli Owner > Settings > AdMob.")
		return
	}
	if e := q.Get("error"); e != "" || q.Get("code") == "" {
		page(http.StatusBadRequest, "Not connected", firstOf(q.Get("error_description"), e, "The sign-in did not return a code."))
		return
	}
	tok, account, err := s.AdMob.Exchange(r.Context(), q.Get("code"), s.redirectURL("admob"))
	if err != nil {
		slog.Error("admob exchange", "err", err)
		page(http.StatusBadGateway, "Not connected", err.Error())
		return
	}
	sealed := store.SealedToken{Access: s.Box.Seal(tok.Access)}
	if tok.Refresh != "" {
		sealed.Refresh = s.Box.Seal(tok.Refresh)
	}
	if !tok.Expires.IsZero() {
		e := tok.Expires
		sealed.Expires = &e
	}
	if err := s.Store.SaveIntegration(r.Context(), "admob", account, sealed); err != nil {
		page(http.StatusInternalServerError, "Not connected", "Could not save the AdMob login.")
		return
	}
	_ = s.Store.Audit(r.Context(), "admob connect", account)
	page(http.StatusOK, "AdMob connected", "Ad earnings will show on the dashboards after the next sync. Go back to Ampli Owner.")
}

// SyncResult is what one integrations sync did.
type SyncResult struct {
	AdMobDays int      `json:"admob_days"`
	Vitals    int      `json:"vitals"`
	Reviews   int      `json:"reviews"`
	PlayDays  int      `json:"play_days"`
	Refunds   int      `json:"refunds"`
	ExactNet  int64    `json:"exact_net"` // purchases whose net came from Google's earnings report
	Errors    []string `json:"errors"`
}

// SyncIntegrations pulls AdMob earnings and everything from Google Play, then drops old rate-limit rows.
func (s *Server) SyncIntegrations(ctx context.Context) SyncResult {
	res := SyncResult{Errors: []string{}}
	today := s.today()
	add := func(prefix string, err error) { res.Errors = append(res.Errors, prefix+": "+err.Error()) }

	if s.admobReady() {
		n, err := s.syncAdMob(ctx, today)
		res.AdMobDays = n
		if err != nil && !errors.Is(err, store.ErrNotFound) {
			add("AdMob", err)
		}
		if !errors.Is(err, store.ErrNotFound) {
			_ = s.Store.MarkIntegration(ctx, "admob", errText(err), err == nil)
		}
	}

	if s.Play != nil {
		before := len(res.Errors)
		prods, err := s.Store.Products(ctx)
		if err != nil {
			add("products", err)
		}
		for _, p := range prods {
			days, err := s.Play.Vitals(ctx, p.PackageID, today.AddDate(0, 0, -7), today.AddDate(0, 0, -1))
			if err != nil {
				add(p.Name+" vitals", err)
			}
			for _, d := range days {
				if err := s.Store.SaveVitals(ctx, p.ID, d.Day, d.Crash, d.ANR); err == nil {
					res.Vitals++
				}
			}
			voided, err := s.Play.VoidedPurchases(ctx, p.PackageID, today.AddDate(0, 0, -30))
			if err != nil {
				add(p.Name+" refunds", err)
			}
			for _, v := range voided {
				if err := s.Store.MarkRefunded(ctx, v.OrderID, v.At); err == nil {
					res.Refunds++
				}
			}
			if s.PlayBucket == "" {
				continue
			}
			thisMonth := time.Date(today.Year(), today.Month(), 1, 0, 0, 0, 0, time.UTC)
			for _, month := range []time.Time{thisMonth.AddDate(0, -1, 0), thisMonth} { // from the 1st, so the 31st has a previous month
				rows, err := s.Play.BulkStats(ctx, s.PlayBucket, p.PackageID, month)
				if errors.Is(err, play.ErrNoObject) {
					continue // Play writes the month's report a day or two into the month
				}
				if err != nil {
					add(fmt.Sprintf("%s Play report %s", p.Name, month.Format("Jan")), err)
					continue
				}
				for _, d := range rows {
					if !d.HasInstalls {
						continue
					}
					if err := s.Store.SetPlayStats(ctx, p.ID, d.Day, d.Installs, d.Uninstalls, d.Active, d.Rating); err == nil {
						res.PlayDays++
					}
				}
			}
		}
		if s.PlayBucket != "" {
			// Last month's earnings report gives each order's real net, including buyers paying in other currencies.
			thisMonth := time.Date(today.Year(), today.Month(), 1, 0, 0, 0, 0, time.UTC)
			for _, month := range []time.Time{thisMonth.AddDate(0, -2, 0), thisMonth.AddDate(0, -1, 0)} {
				net, err := s.Play.Earnings(ctx, s.PlayBucket, month)
				if errors.Is(err, play.ErrNoObject) {
					continue
				}
				if err != nil {
					add("Play earnings "+month.Format("Jan"), err)
					continue
				}
				n, err := s.Store.SetExactNet(ctx, net)
				if err != nil {
					add("earnings", err)
				}
				res.ExactNet += n
			}
		}
		n, problems := s.SyncReviews(ctx)
		res.Reviews = n
		res.Errors = append(res.Errors, problems...)
		var lastErr error
		if len(res.Errors) > before {
			lastErr = errors.New(res.Errors[before])
		}
		_ = s.Store.MarkIntegration(ctx, "play", errText(lastErr), lastErr == nil)
	}

	if err := s.Store.Cleanup(ctx, today); err != nil {
		add("cleanup", err)
	}
	return res
}

func (s *Server) syncAdMob(ctx context.Context, today time.Time) (int, error) {
	in, sealed, err := s.Store.IntegrationToken(ctx, "admob")
	if err != nil {
		return 0, err
	}
	var t admob.Token
	if t.Access, err = s.Box.Open(sealed.Access); err != nil {
		return 0, errors.New("stored AdMob login unreadable (TOKEN_KEY changed?), connect again")
	}
	if sealed.Refresh != nil {
		t.Refresh, _ = s.Box.Open(sealed.Refresh)
	}
	if sealed.Expires != nil {
		t.Expires = *sealed.Expires
	}
	fresh, err := s.AdMob.Fresh(ctx, t)
	if err != nil {
		return 0, err
	}
	if fresh.Access != t.Access {
		up := store.SealedToken{Access: s.Box.Seal(fresh.Access)}
		if !fresh.Expires.IsZero() {
			e := fresh.Expires
			up.Expires = &e
		}
		if err := s.Store.UpdateIntegrationToken(ctx, "admob", up); err != nil {
			return 0, err
		}
	}
	earnings, err := s.AdMob.Earnings(ctx, fresh, in.Account, today.AddDate(0, 0, -29), today)
	if err != nil {
		return 0, err
	}
	for _, e := range earnings {
		if err := s.Store.SetAdRevenue(ctx, e.Product, e.Day, e.Paise); err != nil {
			return 0, err
		}
	}
	return len(earnings), nil
}

// stateOnce marks the callback's OAuth state as used; a second callback with the same state is refused.
func (s *Server) stateOnce(r *http.Request) bool {
	ok, err := s.Store.UseState(r.Context(), s.hashKey("oauth-state", r.URL.Query().Get("state")))
	if err != nil {
		slog.Error("oauth state", "err", err)
	}
	return ok
}

func errText(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

func (s *Server) syncIntegrations(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, s.SyncIntegrations(r.Context()))
}

// ---- SKUs (prices the server trusts for purchases) ----

func (s *Server) skus(w http.ResponseWriter, r *http.Request) {
	ks, err := s.Store.SKUs(r.Context())
	if err != nil {
		fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, ks)
}

func (s *Server) saveSKU(w http.ResponseWriter, r *http.Request) {
	var k store.SKU
	if !decode(w, r, &k) {
		return
	}
	if productID(k.ProductID) == "" || !lenBetween(k.SKU, 1, 100) || !oneOf(k.Kind, "pro", "tip", "subscription", "other") ||
		k.PricePaise < 0 || k.PricePaise > 10000000 {
		writeErr(w, http.StatusBadRequest, "product, sku, kind (pro, tip, subscription, other) and price_paise up to ₹1,00,000 are required")
		return
	}
	if err := s.Store.SaveSKU(r.Context(), k); err != nil {
		fail(w, r, err)
		return
	}
	s.audit(r, "sku", fmt.Sprintf("%s %s %s = %s", k.ProductID, k.SKU, k.Kind, rupees(k.PricePaise)))
	writeJSON(w, http.StatusOK, k)
}

// eraseInstall handles a user's data deletion request (they read their install id from the app's Settings).
func (s *Server) eraseInstall(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("install")
	if !validInstall(id) {
		writeErr(w, http.StatusBadRequest, "install id must be a UUID v4")
		return
	}
	n, err := s.Store.EraseInstall(r.Context(), strings.ToLower(id))
	if err != nil {
		fail(w, r, err)
		return
	}
	s.audit(r, "erase install", fmt.Sprintf("install %s…, %d rows", s.hashKey("audit", strings.ToLower(id))[:10], n))
	writeJSON(w, http.StatusOK, map[string]int64{"erased_rows": n})
}

// audit records an owner action that changes money or data; a failure to write it is logged, not fatal.
func (s *Server) audit(r *http.Request, action, detail string) {
	if err := s.Store.Audit(r.Context(), action, detail); err != nil {
		slog.Error("audit", "action", action, "err", err)
	}
}

func (s *Server) auditLog(w http.ResponseWriter, r *http.Request) {
	es, err := s.Store.AuditLog(r.Context())
	if err != nil {
		fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, es)
}
