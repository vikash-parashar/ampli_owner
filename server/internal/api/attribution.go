package api

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"os"
	"regexp"
	"strings"

	"github.com/vikash-parashar/ampli_owner/server/internal/ads"
	"github.com/vikash-parashar/ampli_owner/server/internal/play"
	"github.com/vikash-parashar/ampli_owner/server/internal/store"
)

const (
	// PublicHitsPerDay caps install, purchase, event and vote calls per install per day.
	PublicHitsPerDay = 30
	// IPHitsPerDay caps public writes from one /24 (IPv4) or /48 (IPv6) network per day. Indian mobile
	// carriers put many real users behind one IPv4 /24 (CGNAT), so it is generous; GETs are not counted.
	IPHitsPerDay = 1000
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
	// Behind your own reverse proxy (Docker), set TRUST_PROXY=1: the right-most X-Forwarded-For hop is the one
	// that proxy added, so the client can't spoof it.
	if os.Getenv("TRUST_PROXY") == "1" {
		if xf := r.Header.Get("X-Forwarded-For"); xf != "" {
			hops := strings.Split(xf, ",")
			host = strings.TrimSpace(hops[len(hops)-1])
		}
	}
	ip := net.ParseIP(host)
	switch {
	case ip == nil:
		return "unknown"
	case ip.To4() != nil:
		return ip.Mask(net.CIDRMask(24, 32)).String()
	default:
		return ip.Mask(net.CIDRMask(48, 128)).String()
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
	ok := true
	if r.Method != http.MethodGet {
		var err error
		if ok, err = s.Store.IPHit(ctx, s.hashKey("ip", clientNet(r)), s.today(), IPHitsPerDay); err != nil {
			fail(w, r, err)
			return false
		}
	}
	if ok && perInstall {
		var err error
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

// netOf is what reaches the bank: for buyers in India minus 18% GST, then minus Google's 15% fee for everyone.
// Outside India the INR list price is an estimate (Play converts to local prices and taxes).
func netOf(gross int64, region string) int64 {
	if region == "" || region == "IN" {
		gross = gross * 100 / 118
	}
	return gross * 85 / 100
}

func (s *Server) publicPurchase(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Product       string `json:"product"`
		InstallID     string `json:"install_id"`
		SKU           string `json:"sku"`
		ProductID     string `json:"product_id"` // Play's name for the sku; accepted as an alias
		PurchaseToken string `json:"purchase_token"`
		Kind          string `json:"kind"`
	}
	if !decode(w, r, &in) {
		return
	}
	if in.SKU == "" {
		in.SKU = in.ProductID
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
	v := s.verifyPurchase(ctx, in.Product, in.SKU, in.PurchaseToken, &p)
	if v.status != 0 && !v.keep {
		writeErr(w, v.status, v.msg)
		return
	}
	created, err := s.Store.SavePurchase(ctx, p)
	if err != nil {
		fail(w, r, err)
		return
	}
	if v.status != 0 {
		// Saved unverified so nothing is lost; the app retries and the retry upgrades the row.
		w.Header().Set("Retry-After", "3600")
		writeErr(w, v.status, v.msg)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"recorded": created, "verified": p.Verified})
}

// verdict on a purchase token. status 0 = store it (verified, or a tester purchase that is not money).
// Otherwise status is the HTTP answer: 422 = Google refused it (the app drops it), 409 = pending (retry),
// 503 = Google unreachable or not set up (retry; keep = store unverified meanwhile).
type purchaseCheck struct {
	status int
	msg    string
	keep   bool
}

func (s *Server) verifyPurchase(ctx context.Context, product, sku, token string, p *store.Purchase) purchaseCheck {
	p.OrderID = "unverified:" + p.TokenHash
	if s.Play == nil {
		return purchaseCheck{http.StatusServiceUnavailable, "purchase saved; it is checked once Google Play is connected, retry later", true}
	}
	pkg, err := s.packageOf(ctx, product)
	if err != nil {
		return purchaseCheck{status: http.StatusInternalServerError, msg: "internal error"}
	}
	gp, err := s.Play.ProductPurchase(ctx, pkg, sku, token)
	var he *play.HTTPError
	if errors.As(err, &he) && (he.Status == http.StatusBadRequest || he.Status == http.StatusGone) {
		// 400 = the token is not a real purchase, 410 = it expired: the app can drop it.
		slog.Warn("purchase refused by Google", "product", product, "sku", sku, "status", he.Status)
		return purchaseCheck{status: http.StatusUnprocessableEntity, msg: "Google Play did not confirm this purchase"}
	}
	if errors.As(err, &he) && he.Status != http.StatusTooManyRequests && he.Status < 500 {
		// 401/403 = our service account lacks "View financial data"; 404 = wrong package or sku on our side.
		// Our setup is wrong, not the purchase: keep it unverified and let the app retry (an alert says so).
		slog.Error("purchase check blocked by our Play setup", "product", product, "sku", sku, "status", he.Status, "err", err)
		return purchaseCheck{http.StatusServiceUnavailable, "purchase saved; the server can't check it with Google Play yet, retry later", true}
	}
	if err != nil {
		slog.Warn("purchase check failed, will retry", "product", product, "sku", sku, "err", err)
		return purchaseCheck{http.StatusServiceUnavailable, "Google Play is not answering; retry later", true}
	}
	switch gp.State {
	case 0:
	case 2:
		return purchaseCheck{status: http.StatusConflict, msg: "payment pending; retry when it completes"}
	default:
		return purchaseCheck{status: http.StatusUnprocessableEntity, msg: "purchase was cancelled"}
	}
	p.OrderID, p.Region = gp.OrderID, gp.RegionCode
	if gp.Test {
		p.Region = "TEST" // license testers and promo codes are not money
		if gp.Promo {
			p.Region = "PROMO" // counted as a cross-promo redemption, not revenue
		}
		return purchaseCheck{}
	}
	p.AmountPaise *= gp.Quantity
	p.Verified, p.NetPaise = true, netOf(p.AmountPaise, gp.RegionCode)
	return purchaseCheck{}
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
