package api

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"regexp"
	"strings"

	"github.com/vikash-parashar/ampli_owner/server/internal/ads"
	"github.com/vikash-parashar/ampli_owner/server/internal/store"
)

const (
	// PublicHitsPerDay caps install, purchase, event and vote calls per install per day.
	PublicHitsPerDay = 30
	// IPHitsPerDay caps all public calls from one /24 (IPv4) or /64 (IPv6) network per day.
	IPHitsPerDay = 300
)

var uuidV4 = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)

func validInstall(id string) bool { return uuidV4.MatchString(strings.ToLower(id)) }

// clientNet returns the caller's network prefix. Behind a Lambda Function URL algnhsa puts the
// source IP in RemoteAddr; X-Forwarded-For is only a fallback.
func clientNet(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	if host == "" || host == "127.0.0.1" || host == "::1" {
		if xf := r.Header.Get("X-Forwarded-For"); xf != "" {
			host = strings.TrimSpace(strings.Split(xf, ",")[0])
		}
	}
	ip := net.ParseIP(host)
	switch {
	case ip == nil:
		return "unknown"
	case ip.To4() != nil:
		return ip.Mask(net.CIDRMask(24, 32)).String()
	default:
		return ip.Mask(net.CIDRMask(64, 128)).String()
	}
}

func (s *Server) hashKey(purpose, v string) string {
	if s.Box != nil {
		return s.Box.HashKey(purpose, v)
	}
	sum := sha256.Sum256([]byte("ampli-owner:" + purpose + ":" + v))
	return base64.RawURLEncoding.EncodeToString(sum[:18])
}

// guard checks the install id format and the per-network limit, then (when perInstall) the per-install limit.
func (s *Server) guard(w http.ResponseWriter, r *http.Request, installID string, perInstall bool) bool {
	if !validInstall(installID) {
		writeErr(w, http.StatusBadRequest, "install_id must be a random UUID v4")
		return false
	}
	ctx := r.Context()
	ok, err := s.Store.IPHit(ctx, s.hashKey("ip", clientNet(r)), s.today(), IPHitsPerDay)
	if err != nil {
		fail(w, r, err)
		return false
	}
	if ok && perInstall {
		ok, err = s.Store.Hit(ctx, installID, s.today(), PublicHitsPerDay)
		if err != nil {
			fail(w, r, err)
			return false
		}
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
	if productID(in.Product) == "" || len(in.Referrer) > 2000 || len(in.AppVersion) > 64 || len(in.Country) > 8 {
		writeErr(w, http.StatusBadRequest, "product and install_id are required; referrer up to 2000 characters")
		return
	}
	if !s.guard(w, r, in.InstallID, true) {
		return
	}
	created, err := s.Store.SaveInstall(r.Context(), store.Install{ProductID: in.Product, InstallID: strings.ToLower(in.InstallID),
		AppVersion: in.AppVersion, Country: strings.ToUpper(in.Country), RawReferrer: in.Referrer, Ref: ads.ParseReferrer(in.Referrer)})
	if err != nil {
		fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"recorded": created})
}

// netOf is what reaches the bank from a price paid in India: minus 18% GST, then minus Google's 15% fee.
func netOf(gross int64) int64 { return gross * 100 / 118 * 85 / 100 }

func (s *Server) publicPurchase(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Product       string `json:"product"`
		InstallID     string `json:"install_id"`
		SKU           string `json:"sku"`
		PurchaseToken string `json:"purchase_token"`
		Kind          string `json:"kind"`
	}
	if !decode(w, r, &in) {
		return
	}
	if productID(in.Product) == "" || !lenBetween(in.SKU, 1, 100) || !lenBetween(in.PurchaseToken, 10, 4000) {
		writeErr(w, http.StatusBadRequest, "product, install_id, sku and purchase_token are required")
		return
	}
	if !s.guard(w, r, in.InstallID, true) {
		return
	}
	ctx := r.Context()
	sku, err := s.Store.SKU(ctx, in.Product, in.SKU)
	if errors.Is(err, store.ErrNotFound) {
		writeErr(w, http.StatusBadRequest, "unknown sku")
		return
	}
	if err != nil {
		fail(w, r, err)
		return
	}
	p := store.Purchase{ProductID: in.Product, InstallID: strings.ToLower(in.InstallID), Kind: sku.Kind, SKU: in.SKU,
		AmountPaise: sku.PricePaise, Currency: "INR", TokenHash: s.hashKey("purchase", in.PurchaseToken)}
	verified, status, msg := s.verifyPurchase(ctx, in.Product, in.SKU, in.PurchaseToken, &p)
	if !verified && status != 0 {
		writeErr(w, status, msg)
		return
	}
	created, err := s.Store.SavePurchase(ctx, p)
	if err != nil {
		fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"recorded": created, "verified": p.Verified})
}

// verifyPurchase asks Google Play about the token. Without a Play service account the purchase is stored
// unverified (kept out of revenue) so nothing is lost; a token Google rejects is refused.
func (s *Server) verifyPurchase(ctx context.Context, product, sku, token string, p *store.Purchase) (bool, int, string) {
	if s.Play == nil {
		p.OrderID = "unverified:" + p.TokenHash
		return false, 0, ""
	}
	pkg, err := s.packageOf(ctx, product)
	if err != nil {
		return false, http.StatusInternalServerError, "internal error"
	}
	gp, err := s.Play.ProductPurchase(ctx, pkg, sku, token)
	if err != nil {
		slog.Warn("purchase check failed", "product", product, "sku", sku, "err", err)
		return false, http.StatusBadRequest, "Google Play did not confirm this purchase"
	}
	if gp.State != 0 {
		return false, http.StatusBadRequest, "purchase is not completed"
	}
	p.OrderID, p.Region = gp.OrderID, gp.RegionCode
	if gp.Test {
		p.Region = "TEST" // license testers and promo codes are not money
		return false, 0, ""
	}
	p.Verified, p.NetPaise = true, netOf(p.AmountPaise)
	return true, 0, ""
}

// eventNames lists the counters each app may send.
var eventNames = map[string]map[string]bool{
	"mp3": set("played_10", "pro_feature_opened", "pro_trial", "feature_eq", "feature_record_player", "feature_sleep_timer",
		"feature_ringtone", "feature_wifi_transfer"),
	"carhub": set("voice_bunty", "voice_simran", "cmd_navigate", "cmd_music", "cmd_call", "cmd_fastag", "cmd_sos"),
	"naukri": set("alert_sent", "marked_applied", "state_hr", "state_up", "state_rj", "state_dl", "state_other"),
}

func set(v ...string) map[string]bool {
	m := map[string]bool{}
	for _, s := range v {
		m[s] = true
	}
	return m
}

func (s *Server) publicEvents(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Product   string   `json:"product"`
		InstallID string   `json:"install_id"`
		Events    []string `json:"events"`
	}
	if !decode(w, r, &in) {
		return
	}
	if productID(in.Product) == "" || len(in.Events) == 0 || len(in.Events) > 20 {
		writeErr(w, http.StatusBadRequest, "product, install_id and 1 to 20 events are required")
		return
	}
	seen := map[string]bool{}
	names := make([]string, 0, len(in.Events))
	for _, e := range in.Events {
		if !eventNames[in.Product][e] {
			writeErr(w, http.StatusBadRequest, "unknown event "+e)
			return
		}
		if !seen[e] {
			seen[e] = true
			names = append(names, e)
		}
	}
	if !s.guard(w, r, in.InstallID, true) {
		return
	}
	if err := s.Store.AddEvents(r.Context(), in.Product, s.today(), names); err != nil {
		fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]int{"recorded": len(names)})
}
