package api

import (
	"net/http"
	"regexp"
	"strings"

	"github.com/vikash-parashar/ampli_owner/server/internal/ads"
	"github.com/vikash-parashar/ampli_owner/server/internal/store"
)

// PublicHitsPerDay caps install, purchase and event calls per install per day.
const PublicHitsPerDay = 30

func (s *Server) allow(w http.ResponseWriter, r *http.Request, installID string) bool {
	ok, err := s.Store.Hit(r.Context(), installID, s.today(), PublicHitsPerDay)
	if err != nil {
		fail(w, r, err)
		return false
	}
	if !ok {
		writeErr(w, http.StatusTooManyRequests, "daily limit reached")
	}
	return ok
}

func (s *Server) publicInstall(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Product    string `json:"product"`
		InstallID  string `json:"install_id"`
		Referrer   string `json:"referrer"`
		AppVersion string `json:"app_version"`
		Country    string `json:"country"`
	}
	if !decode(w, r, &in) {
		return
	}
	if productID(in.Product) == "" || !lenBetween(in.InstallID, 8, 64) || len(in.Referrer) > 2000 || len(in.AppVersion) > 64 || len(in.Country) > 8 {
		writeErr(w, http.StatusBadRequest, "product and install_id are required; referrer up to 2000 characters")
		return
	}
	if !s.allow(w, r, in.InstallID) {
		return
	}
	created, err := s.Store.SaveInstall(r.Context(), store.Install{ProductID: in.Product, InstallID: in.InstallID,
		AppVersion: in.AppVersion, Country: strings.ToUpper(in.Country), RawReferrer: in.Referrer, Ref: ads.ParseReferrer(in.Referrer)})
	if err != nil {
		fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"recorded": created})
}

func (s *Server) publicPurchase(w http.ResponseWriter, r *http.Request) {
	var p store.Purchase
	if !decode(w, r, &p) {
		return
	}
	if p.Currency == "" {
		p.Currency = "INR"
	}
	if productID(p.ProductID) == "" || !lenBetween(p.InstallID, 8, 64) || !oneOf(p.Kind, "pro", "tip", "subscription", "other") ||
		!lenBetween(p.OrderID, 4, 128) || len(p.SKU) > 100 || p.AmountPaise < 0 || p.AmountPaise > 10000000 || len(p.Currency) != 3 {
		writeErr(w, http.StatusBadRequest, "product, install_id, kind (pro|tip|subscription|other), order_id and amount_paise are required")
		return
	}
	if !s.allow(w, r, p.InstallID) {
		return
	}
	created, err := s.Store.SavePurchase(r.Context(), p)
	if err != nil {
		fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"recorded": created})
}

var eventName = regexp.MustCompile(`^[a-z0-9_]{1,40}$`)

func (s *Server) publicEvents(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Product   string   `json:"product"`
		InstallID string   `json:"install_id"`
		Events    []string `json:"events"`
	}
	if !decode(w, r, &in) {
		return
	}
	if productID(in.Product) == "" || !lenBetween(in.InstallID, 8, 64) || len(in.Events) == 0 || len(in.Events) > 20 {
		writeErr(w, http.StatusBadRequest, "product, install_id and 1 to 20 events are required")
		return
	}
	seen := map[string]bool{}
	names := make([]string, 0, len(in.Events))
	for _, e := range in.Events {
		if !eventName.MatchString(e) {
			writeErr(w, http.StatusBadRequest, "event names are lowercase letters, digits and _ (up to 40)")
			return
		}
		if !seen[e] {
			seen[e] = true
			names = append(names, e)
		}
	}
	if !s.allow(w, r, in.InstallID) {
		return
	}
	if err := s.Store.AddEvents(r.Context(), in.Product, s.today(), names); err != nil {
		fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]int{"recorded": len(names)})
}
