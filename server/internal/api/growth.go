package api

import (
	"errors"
	"fmt"
	"math"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/vikash-parashar/ampli_owner/server/internal/store"
)

// channels a tracked link can be made for, with the utm_medium each one gets and the name shown in the app.
var linkChannels = map[string]struct{ Medium, Label string }{
	"play_hindi": {"listing", "Hindi Play listing"},
	"website":    {"web", "ampliapps.in"},
	"whatsapp":   {"chat", "WhatsApp"},
	"telegram":   {"chat", "Telegram"},
	"instagram":  {"social", "Instagram (unpaid)"},
	"youtube":    {"video", "YouTube"},
	"crosspromo": {"app", "More from Ampli"},
	"other":      {"link", "Other link"},
}

// labels for the dashboard's "where installs come from" widget, in display order after sorting by count.
var channelLabels = map[string]string{
	"play": "Play search and browse", "ads_meta": "Ads: Facebook + Instagram", "ads_tiktok": "Ads: TikTok",
	"ads_google": "Ads: Google", "ads_link": "Ads (our link)", "unknown": "Other or unknown",
}

func channelLabel(k string) string {
	if c, ok := linkChannels[k]; ok {
		return c.Label
	}
	if l, ok := channelLabels[k]; ok {
		return l
	}
	return k
}

// playURL is the store link with the install referrer filled in; Play hands the referrer to the app on first open.
func playURL(pkg string, l store.Link) string {
	ref := url.Values{}
	ref.Set("utm_source", l.Channel)
	ref.Set("utm_medium", linkChannels[l.Channel].Medium)
	ref.Set("utm_campaign", l.UTMCampaign)
	if l.FromProduct != "" {
		ref.Set("utm_content", l.FromProduct)
	}
	q := url.Values{}
	q.Set("id", pkg)
	if l.Listing != "" {
		q.Set("listing", l.Listing) // Play custom store listing, e.g. the Hindi one
	}
	if l.Channel == "play_hindi" {
		q.Set("hl", "hi")
	}
	q.Set("referrer", ref.Encode())
	return "https://play.google.com/store/apps/details?" + q.Encode()
}

type linkView struct {
	store.Link
	ChannelName string `json:"channel_name"`
	URL         string `json:"url"`
}

var listingName = regexp.MustCompile(`^[a-z0-9_-]{1,40}$`)

func (s *Server) links(w http.ResponseWriter, r *http.Request) {
	p := r.URL.Query().Get("product")
	if p != "" && productID(p) == "" {
		writeErr(w, http.StatusBadRequest, "product must be mp3, carhub or naukri")
		return
	}
	ls, err := s.Store.Links(r.Context(), p, s.today().AddDate(0, 0, -29))
	if err != nil {
		fail(w, r, err)
		return
	}
	out := make([]linkView, 0, len(ls))
	for _, l := range ls {
		out = append(out, linkView{Link: l, ChannelName: channelLabel(l.Channel), URL: playURL(l.PackageID, l)})
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) createLink(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Product     string `json:"product"`
		Channel     string `json:"channel"`
		Label       string `json:"label"`
		Listing     string `json:"listing"`
		FromProduct string `json:"from_product"`
	}
	if !decode(w, r, &in) {
		return
	}
	in.Label, in.Listing = strings.TrimSpace(in.Label), strings.ToLower(strings.TrimSpace(in.Listing))
	if _, ok := linkChannels[in.Channel]; productID(in.Product) == "" || !ok {
		writeErr(w, http.StatusBadRequest, "product (mp3, carhub, naukri) and channel (play_hindi, website, whatsapp, telegram, instagram, youtube, crosspromo, other) are required")
		return
	}
	if len([]rune(in.Label)) > 60 || (in.Listing != "" && !listingName.MatchString(in.Listing)) {
		writeErr(w, http.StatusBadRequest, "label up to 60 characters; listing is the custom store listing's URL name (a-z, 0-9, - or _)")
		return
	}
	if in.FromProduct != "" && (productID(in.FromProduct) == "" || in.FromProduct == in.Product || in.Channel != "crosspromo") {
		writeErr(w, http.StatusBadRequest, "from_product is only for cross-promo links and must be another app")
		return
	}
	if in.Channel == "play_hindi" && in.Listing == "" {
		in.Listing = "hindi"
	}
	pkg, err := s.packageOf(r.Context(), in.Product)
	if err != nil {
		fail(w, r, err)
		return
	}
	l, err := s.Store.CreateLink(r.Context(), store.Link{ProductID: in.Product, PackageID: pkg, Channel: in.Channel,
		Label: in.Label, Listing: in.Listing, FromProduct: in.FromProduct})
	if err != nil {
		fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, linkView{Link: l, ChannelName: channelLabel(l.Channel), URL: playURL(pkg, l)})
}

func (s *Server) deleteLink(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	if err := s.Store.DeleteLink(r.Context(), id); err != nil {
		fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// channelWidget is the product dashboard's "Where installs come from (30 days)".
func channelWidget(counts map[string]int64) Widget {
	rows := make([]WidgetRow, 0, len(counts))
	var total int64
	for k, n := range counts {
		rows = append(rows, WidgetRow{Label: channelLabel(k), Value: n})
		total += n
	}
	sort.SliceStable(rows, func(i, j int) bool {
		if rows[i].Value != rows[j].Value {
			return rows[i].Value > rows[j].Value
		}
		return rows[i].Label < rows[j].Label
	})
	for i := range rows {
		if total > 0 {
			rows[i].Pct = math.Round(float64(rows[i].Value)/float64(total)*1000) / 10
		}
	}
	return Widget{"channels", "Where installs come from (30 days)", "bars", rows}
}

// ---- cross-promo codes ----

// PromoHold is how long after a Pro purchase its cross-promo code unlocks: Play's self-service refund window.
const PromoHold = 48 * time.Hour

var promoCode = regexp.MustCompile(`^[A-Z0-9]{6,40}$`)

func (s *Server) importPromoCodes(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Source string   `json:"source_product"`
		Target string   `json:"target_product"`
		SKU    string   `json:"sku"`
		Codes  []string `json:"codes"`
	}
	if !decode(w, r, &in) {
		return
	}
	if productID(in.Source) == "" || productID(in.Target) == "" || in.Source == in.Target || !lenBetween(in.SKU, 1, 100) ||
		len(in.Codes) == 0 || len(in.Codes) > 2000 {
		writeErr(w, http.StatusBadRequest, "source_product, target_product (different apps), sku and 1 to 2000 codes are required")
		return
	}
	codes := make([]string, 0, len(in.Codes))
	for _, c := range in.Codes {
		c = strings.ToUpper(strings.TrimSpace(c))
		if c == "" {
			continue
		}
		if !promoCode.MatchString(c) {
			writeErr(w, http.StatusBadRequest, "not a Play promo code: "+c)
			return
		}
		codes = append(codes, c)
	}
	if _, err := s.Store.SKU(r.Context(), in.Target, in.SKU); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeErr(w, http.StatusBadRequest, "unknown sku for "+in.Target)
			return
		}
		fail(w, r, err)
		return
	}
	n, err := s.Store.ImportPromoCodes(r.Context(), in.Source, in.Target, in.SKU, codes)
	if err != nil {
		fail(w, r, err)
		return
	}
	s.audit(r, "promo import", fmt.Sprintf("%d codes %s -> %s %s", n, in.Source, in.Target, in.SKU))
	writeJSON(w, http.StatusOK, map[string]int64{"added": n, "skipped": int64(len(codes)) - n})
}

func (s *Server) promoReport(w http.ResponseWriter, r *http.Request) {
	rs, err := s.Store.PromoReports(r.Context())
	if err != nil {
		fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, rs)
}

// publicClaimPromo gives a verified Pro buyer of one app a promo code for another app (hook spec v1.3).
func (s *Server) publicClaimPromo(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Product   string `json:"product"`
		InstallID string `json:"install_id"`
	}
	if !decode(w, r, &in) {
		return
	}
	if productID(in.Product) == "" {
		writeErr(w, http.StatusBadRequest, "product and install_id are required")
		return
	}
	if !s.guard(w, r, in.InstallID, true) {
		return
	}
	ctx, install := r.Context(), strings.ToLower(in.InstallID)
	bought, err := s.Store.ProBoughtAt(ctx, in.Product, install)
	if errors.Is(err, store.ErrNotFound) {
		writeErr(w, http.StatusForbidden, "only for Pro buyers whose purchase Google confirmed")
		return
	}
	if err != nil {
		fail(w, r, err)
		return
	}
	if ready := bought.Add(PromoHold); s.Now().Before(ready) {
		// Play lets buyers refund themselves for 48 hours; a code handed out sooner is kept after a refund.
		writeJSON(w, http.StatusConflict, map[string]string{"error": "the gift unlocks 48 hours after buying Pro",
			"available_at": ready.UTC().Format(time.RFC3339)})
		return
	}
	c, err := s.Store.ClaimPromo(ctx, in.Product, install, s.Now())
	if errors.Is(err, store.ErrNoCodes) {
		writeErr(w, http.StatusGone, "no codes left; hide the offer and try again in a day")
		return
	}
	if err != nil {
		fail(w, r, err)
		return
	}
	pkg, err := s.packageOf(ctx, c.Target)
	if err != nil {
		fail(w, r, err)
		return
	}
	storeURL := playURL(pkg, store.Link{Channel: "crosspromo", FromProduct: in.Product, UTMCampaign: "ampli-promo"})
	writeJSON(w, http.StatusOK, map[string]string{"code": c.Code, "target_product": c.Target, "sku": c.SKU,
		"redeem_url": "https://play.google.com/redeem?code=" + url.QueryEscape(c.Code), "store_url": storeURL})
}
