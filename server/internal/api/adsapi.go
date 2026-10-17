package api

import (
	"context"
	"errors"
	"fmt"
	"html"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/vikash-parashar/ampli_owner/server/internal/ads"
	"github.com/vikash-parashar/ampli_owner/server/internal/store"
)

var platformOrder = []string{"meta", "tiktok", "google"}

var platformSub = map[string]string{"meta": "Facebook + Instagram", "tiktok": "TikTok", "google": "YouTube + Play + Search"}

type platformView struct {
	ID         string            `json:"id"`
	Name       string            `json:"name"`
	Sub        string            `json:"sub"`
	Configured bool              `json:"configured"`
	MinBudget  int64             `json:"min_daily_budget_paise"`
	Accounts   []store.AdAccount `json:"accounts"`
}

func (s *Server) platform(id string) ads.Platform {
	if s.Platforms == nil {
		return nil
	}
	return s.Platforms[id]
}

func (s *Server) adPlatforms(w http.ResponseWriter, r *http.Request) {
	accts, err := s.Store.AdAccounts(r.Context())
	if err != nil {
		fail(w, r, err)
		return
	}
	out := make([]platformView, 0, len(platformOrder))
	for _, id := range platformOrder {
		v := platformView{ID: id, Name: map[string]string{"meta": "Meta", "tiktok": "TikTok", "google": "Google Ads"}[id],
			Sub: platformSub[id], MinBudget: ads.MinDailyBudgetPaise[id], Accounts: []store.AdAccount{}}
		if p := s.platform(id); p != nil {
			v.Configured = p.Configured() && s.Box != nil && s.PublicURL != ""
		}
		for _, a := range accts {
			if a.Platform == id {
				v.Accounts = append(v.Accounts, a)
			}
		}
		out = append(out, v)
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) redirectURL(platform string) string {
	return strings.TrimRight(s.PublicURL, "/") + "/oauth/" + platform + "/callback"
}

func (s *Server) connectAds(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("platform")
	p := s.platform(id)
	switch {
	case p == nil:
		writeErr(w, http.StatusNotFound, "unknown platform")
		return
	case s.Box == nil || s.PublicURL == "":
		writeErr(w, http.StatusConflict, "set TOKEN_KEY and PUBLIC_URL on the server first")
		return
	case !p.Configured():
		writeErr(w, http.StatusConflict, p.Name()+" developer app keys are not set on the server yet")
		return
	}
	state := s.Box.State(id, s.Now(), 15*time.Minute)
	writeJSON(w, http.StatusOK, map[string]string{"auth_url": p.AuthURL(state, s.redirectURL(id))})
}

// oauthCallback is where the platform sends the browser after the owner signs in.
func (s *Server) oauthCallback(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("platform")
	q := r.URL.Query()
	page := func(code int, title, msg string) { oauthPage(w, code, title, msg) }
	if id == "admob" {
		s.admobCallback(r, page)
		return
	}
	p := s.platform(id)
	if p == nil || s.Box == nil {
		page(http.StatusNotFound, "Unknown platform", "")
		return
	}
	if got, err := s.Box.CheckState(q.Get("state"), s.Now()); err != nil || got != id || !s.stateOnce(r) {
		page(http.StatusBadRequest, "Link expired", "Start again from Ampli Owner > Ads > Connect.")
		return
	}
	if e := q.Get("error"); e != "" {
		page(http.StatusBadRequest, "Not connected", firstOf(q.Get("error_description"), q.Get("error_reason"), e))
		return
	}
	code := firstOf(q.Get("code"), q.Get("auth_code"))
	if code == "" {
		page(http.StatusBadRequest, "Not connected", "The sign-in did not return a code.")
		return
	}
	tok, accounts, err := p.Exchange(r.Context(), code, s.redirectURL(id))
	if err != nil {
		slog.Error("oauth exchange", "platform", id, "err", err)
		page(http.StatusBadGateway, "Not connected", err.Error())
		return
	}
	if len(accounts) == 0 {
		page(http.StatusOK, p.Name()+" connected", "But this login has no ad accounts yet. Create one in "+p.Name()+" and connect again.")
		return
	}
	for _, a := range accounts {
		if _, err := s.Store.SaveAdAccount(r.Context(), id, a, s.seal(tok)); err != nil {
			fail(w, r, err)
			return
		}
	}
	page(http.StatusOK, p.Name()+" connected", fmt.Sprintf("%d ad account(s) added. Go back to Ampli Owner.", len(accounts)))
}

func firstOf(v ...string) string {
	for _, s := range v {
		if s != "" {
			return s
		}
	}
	return ""
}

func (s *Server) seal(t ads.Token) store.SealedToken {
	st := store.SealedToken{Access: s.Box.Seal(t.Access)}
	if t.Refresh != "" {
		st.Refresh = s.Box.Seal(t.Refresh)
	}
	if !t.Expires.IsZero() {
		e := t.Expires
		st.Expires = &e
	}
	return st
}

// account loads an ad account with a usable token, refreshing and saving it when needed.
func (s *Server) account(ctx context.Context, id int64) (store.AdAccount, ads.Platform, ads.Account, ads.Token, error) {
	a, sealed, err := s.Store.AdAccountToken(ctx, id)
	if err != nil {
		return a, nil, ads.Account{}, ads.Token{}, err
	}
	p := s.platform(a.Platform)
	if p == nil || s.Box == nil {
		return a, nil, ads.Account{}, ads.Token{}, errors.New(a.Platform + " is not set up on the server")
	}
	var t ads.Token
	if t.Access, err = s.Box.Open(sealed.Access); err != nil {
		return a, nil, ads.Account{}, ads.Token{}, errors.New("stored token unreadable (TOKEN_KEY changed?), connect again")
	}
	if sealed.Refresh != nil {
		t.Refresh, _ = s.Box.Open(sealed.Refresh)
	}
	if sealed.Expires != nil {
		t.Expires = *sealed.Expires
	}
	fresh, err := p.Fresh(ctx, t)
	if err != nil {
		_ = s.Store.MarkAdAccount(ctx, id, "expired", err.Error(), false)
		return a, nil, ads.Account{}, ads.Token{}, err
	}
	if fresh.Access != t.Access {
		if err := s.Store.UpdateAdAccountToken(ctx, id, s.seal(fresh)); err != nil {
			return a, nil, ads.Account{}, ads.Token{}, err
		}
	}
	return a, p, ads.Account{ExternalID: a.ExternalID, Name: a.Name, Currency: a.Currency}, fresh, nil
}

func (s *Server) deleteAdAccount(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	if err := s.Store.DeleteAdAccount(r.Context(), id); err != nil {
		fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) adCampaigns(w http.ResponseWriter, r *http.Request) {
	p := r.URL.Query().Get("product")
	if p == "all" {
		p = ""
	}
	if p != "" && productID(p) == "" {
		writeErr(w, http.StatusBadRequest, "product must be all, mp3, carhub or naukri")
		return
	}
	cs, err := s.Store.Campaigns(r.Context(), p)
	if err != nil {
		fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"summary": summarize(cs), "campaigns": cs})
}

type newCampaignReq struct {
	Product          string       `json:"product"`
	AdAccountID      int64        `json:"ad_account_id"`
	Name             string       `json:"name"`
	Countries        []string     `json:"countries"`
	DailyBudgetPaise int64        `json:"daily_budget_paise"`
	Days             int          `json:"days"`
	Creative         ads.Creative `json:"creative"`
	Launch           bool         `json:"launch"`
}

func (q newCampaignReq) validate() string {
	switch {
	case productID(q.Product) == "":
		return "product must be mp3, carhub or naukri"
	case q.AdAccountID <= 0:
		return "pick an ad account"
	case len(q.Countries) == 0 || len(q.Countries) > 10:
		return "pick 1 to 10 countries"
	case q.DailyBudgetPaise < 10000 || q.DailyBudgetPaise > 10000000:
		return "daily budget must be ₹100 to ₹1,00,000"
	case q.Days < 0 || q.Days > 90:
		return "days must be 0 (no end) to 90"
	case !oneOf(q.Creative.Kind, "video", "image"):
		return "creative kind must be video or image"
	case !strings.HasPrefix(q.Creative.URL, "https://") || len(q.Creative.URL) > 2000:
		return "creative url must be an https link"
	case q.Creative.ThumbnailURL != "" && !strings.HasPrefix(q.Creative.ThumbnailURL, "https://"):
		return "thumbnail url must be an https link"
	case !lenBetween(q.Name, 0, 120), !lenBetween(q.Creative.Headline, 0, 120), !lenBetween(q.Creative.Text, 0, 500):
		return "name, headline or text is too long"
	}
	for _, c := range q.Countries {
		if _, ok := ads.Countries[c]; !ok {
			return "unknown country " + c
		}
	}
	return ""
}

func (s *Server) createAdCampaign(w http.ResponseWriter, r *http.Request) {
	var q newCampaignReq
	if !decode(w, r, &q) {
		return
	}
	if msg := q.validate(); msg != "" {
		writeErr(w, http.StatusBadRequest, msg)
		return
	}
	ctx := r.Context()
	acct, p, a, tok, err := s.account(ctx, q.AdAccountID)
	if errors.Is(err, store.ErrNotFound) {
		writeErr(w, http.StatusBadRequest, "ad account not found")
		return
	}
	if err != nil {
		writeErr(w, http.StatusConflict, err.Error())
		return
	}
	if min := ads.MinDailyBudgetPaise[acct.Platform]; q.DailyBudgetPaise < min {
		writeErr(w, http.StatusBadRequest, fmt.Sprintf("%s needs at least %s a day", p.Name(), rupees(min)))
		return
	}
	prods, err := s.Store.Products(ctx)
	if err != nil {
		fail(w, r, err)
		return
	}
	var pkg, pname string
	for _, pr := range prods {
		if pr.ID == q.Product {
			pkg, pname = pr.PackageID, pr.Name
		}
	}
	if q.Name == "" {
		q.Name = fmt.Sprintf("%s · %s · %s", pname, p.Name(), strings.Join(q.Countries, "+"))
	}
	var ends *string
	if q.Days > 0 {
		e := s.today().AddDate(0, 0, q.Days).Format(time.DateOnly)
		ends = &e
	}
	id, err := s.Store.InsertCampaign(ctx, store.NewCampaign{Platform: acct.Platform, ProductID: q.Product, AdAccountID: acct.ID,
		Name: q.Name, Countries: q.Countries, DailyBudgetPaise: q.DailyBudgetPaise, CreativeURL: q.Creative.URL, EndsOn: ends})
	if err != nil {
		fail(w, r, err)
		return
	}
	ref, err := p.Create(ctx, tok, a, ads.Spec{Product: q.Product, Name: q.Name, Countries: q.Countries, DailyBudgetPaise: q.DailyBudgetPaise,
		Days: q.Days, PackageID: pkg, Creative: q.Creative, Launch: q.Launch, Currency: a.Currency, Now: s.Now()})
	status, lastErr := "paused", ""
	if q.Launch {
		status = "in_review"
	}
	if err != nil {
		slog.Error("create campaign", "platform", acct.Platform, "err", err)
		status, lastErr = "error", err.Error()
		if ref.CampaignID != "" && q.Launch {
			// The campaign exists but a later step failed; keep it paused so nothing spends without an ad.
			_ = p.SetActive(ctx, tok, a, ref, false)
		}
	}
	if err := s.Store.SetCampaignRemote(ctx, id, ref, status, lastErr); err != nil {
		fail(w, r, err)
		return
	}
	detail := fmt.Sprintf("%s a day, %s", rupees(q.DailyBudgetPaise), strings.Join(q.Countries, ", "))
	if lastErr != "" {
		detail += "; failed: " + lastErr
	}
	_ = s.Store.LogCampaign(ctx, id, "owner", map[bool]string{true: "created and launched", false: "created paused"}[q.Launch], detail)
	c, err := s.Store.Campaign(ctx, id)
	if err != nil {
		fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, c)
}

// campaignAction runs pause, resume or budget on the platform, then records it.
func (s *Server) campaignAction(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	action := r.PathValue("action")
	if action == "autostop" {
		s.setAutoStop(w, r, id)
		return
	}
	var body struct {
		DailyBudgetPaise int64 `json:"daily_budget_paise"`
	}
	if action == "budget" {
		if !decode(w, r, &body) {
			return
		}
		if body.DailyBudgetPaise < 10000 || body.DailyBudgetPaise > 10000000 {
			writeErr(w, http.StatusBadRequest, "daily budget must be ₹100 to ₹1,00,000")
			return
		}
	} else if !oneOf(action, "pause", "resume") {
		writeErr(w, http.StatusNotFound, "unknown action")
		return
	}
	ctx := r.Context()
	c, err := s.Store.Campaign(ctx, id)
	if err != nil {
		fail(w, r, err)
		return
	}
	if min := ads.MinDailyBudgetPaise[c.Platform]; action == "budget" && body.DailyBudgetPaise < min {
		writeErr(w, http.StatusBadRequest, fmt.Sprintf("%s needs at least %s a day", c.Platform, rupees(min)))
		return
	}
	remote := c.AdAccountID != nil && c.ExternalID != ""
	var callErr error
	if remote {
		_, p, a, tok, err := s.account(ctx, *c.AdAccountID)
		if err != nil {
			writeErr(w, http.StatusConflict, err.Error())
			return
		}
		ref := ads.Ref{CampaignID: c.ExternalID, SetID: c.ExternalSetID}
		switch action {
		case "pause":
			callErr = p.SetActive(ctx, tok, a, ref, false)
		case "resume":
			callErr = p.SetActive(ctx, tok, a, ref, true)
		case "budget":
			callErr = p.SetDailyBudget(ctx, tok, a, ref, body.DailyBudgetPaise)
		}
	}
	if callErr != nil {
		_ = s.Store.SetCampaignStatus(ctx, id, c.Status, callErr.Error())
		writeErr(w, http.StatusBadGateway, callErr.Error())
		return
	}
	switch action {
	case "pause":
		err = s.Store.SetCampaignStatus(ctx, id, "paused", "")
	case "resume":
		err = s.Store.SetCampaignStatus(ctx, id, "live", "")
		if err == nil && c.AutoStop.Enabled {
			err = s.Store.HoldAutoStop(ctx, id, s.Now().Add(ResumeHold))
		}
	case "budget":
		err = s.Store.SetCampaignBudget(ctx, id, body.DailyBudgetPaise)
	}
	if err != nil {
		fail(w, r, err)
		return
	}
	switch action {
	case "budget":
		_ = s.Store.LogCampaign(ctx, id, "owner", "budget", fmt.Sprintf("%s → %s a day", rupees(c.DailyBudgetPaise), rupees(body.DailyBudgetPaise)))
	default:
		_ = s.Store.LogCampaign(ctx, id, "owner", map[string]string{"pause": "paused", "resume": "resumed"}[action], "")
	}
	if c, err = s.Store.Campaign(ctx, id); err != nil {
		fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, c)
}

func (s *Server) setAutoStop(w http.ResponseWriter, r *http.Request, id int64) {
	var a store.AutoStop
	if !decode(w, r, &a) {
		return
	}
	if a.MinROAS < 0 || a.MinROAS > 5 || a.MinDays < 3 || a.MinDays > 60 || a.MinSpendPaise < 0 || a.MinSpendPaise > 100000000 {
		writeErr(w, http.StatusBadRequest, "min_roas 0 to 5, min_days 3 to 60, min_spend_paise up to ₹10,00,000")
		return
	}
	ctx := r.Context()
	if err := s.Store.SetAutoStop(ctx, id, a); err != nil {
		fail(w, r, err)
		return
	}
	detail := "off"
	if a.Enabled {
		detail = fmt.Sprintf("pause when ROAS < %.2fx after %d days and %s spent", a.MinROAS, a.MinDays, rupees(a.MinSpendPaise))
	}
	_ = s.Store.LogCampaign(ctx, id, "owner", "auto-stop", detail)
	c, err := s.Store.Campaign(ctx, id)
	if err != nil {
		fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, c)
}

func (s *Server) campaignLog(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	log, err := s.Store.CampaignLog(r.Context(), id)
	if err != nil {
		fail(w, r, err)
		return
	}
	if log == nil {
		log = []store.LogEntry{}
	}
	writeJSON(w, http.StatusOK, log)
}

// deleteCampaign pauses the campaign on the platform first so nothing keeps spending, then forgets it.
func (s *Server) deleteCampaign(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	ctx := r.Context()
	c, err := s.Store.Campaign(ctx, id)
	if err != nil {
		fail(w, r, err)
		return
	}
	// Always pause on the platform (it is idempotent): our status may say paused while the platform is live.
	if c.AdAccountID != nil && c.ExternalID != "" {
		_, p, a, tok, err := s.account(ctx, *c.AdAccountID)
		if err == nil {
			err = p.SetActive(ctx, tok, a, ads.Ref{CampaignID: c.ExternalID, SetID: c.ExternalSetID}, false)
		}
		if err != nil {
			writeErr(w, http.StatusBadGateway, "could not pause it on "+c.Platform+" first: "+err.Error())
			return
		}
	}
	if err := s.Store.DeleteCampaign(ctx, id); err != nil {
		fail(w, r, err)
		return
	}
	_ = s.Store.LogCampaign(ctx, id, "owner", "deleted", "paused on the platform and hidden; numbers and log are kept")
	w.WriteHeader(http.StatusNoContent)
}

// rupees formats paise as ₹1,234 (Indian grouping is not needed for these short labels).
func rupees(paise int64) string {
	r := paise / 100
	s := fmt.Sprintf("%d", r)
	for i := len(s) - 3; i > 0; i -= 3 {
		s = s[:i] + "," + s[i:]
	}
	return "₹" + s
}

// SyncAds pulls the last 30 days of numbers from every connected account. Accounts synced in the
// last 10 minutes are skipped unless force is set.
func (s *Server) SyncAds(ctx context.Context, force bool) (int, []string) {
	accts, err := s.Store.AdAccounts(ctx)
	if err != nil {
		return 0, []string{err.Error()}
	}
	today := s.today()
	since, until := today.AddDate(0, 0, -29).Format(time.DateOnly), today.Format(time.DateOnly)
	synced, problems := 0, []string{}
	for _, acct := range accts {
		if !force && acct.LastSyncAt != nil && s.Now().Sub(*acct.LastSyncAt) < 10*time.Minute {
			continue
		}
		err := func() error {
			_, p, a, tok, err := s.account(ctx, acct.ID)
			if err != nil {
				return err
			}
			ids, err := s.Store.CampaignIDs(ctx, acct.ID)
			if err != nil {
				return err
			}
			if len(ids) == 0 {
				return nil
			}
			rows, err := p.Insights(ctx, tok, a, since, until)
			if err != nil {
				return err
			}
			byCampaign := map[int64][]ads.DayInsight{}
			for _, d := range rows {
				if cid, ok := ids[d.CampaignID]; ok {
					byCampaign[cid] = append(byCampaign[cid], d)
				}
			}
			for cid, days := range byCampaign {
				if err := s.Store.SaveCampaignDays(ctx, cid, days); err != nil {
					return err
				}
			}
			s.applyAutoStop(ctx, acct.ID, p, a, tok)
			return nil
		}()
		if err != nil {
			problems = append(problems, acct.Platform+" "+acct.Name+": "+err.Error())
			status := "connected"
			if errors.Is(err, ads.ErrExpired) {
				status = "expired"
			}
			_ = s.Store.MarkAdAccount(ctx, acct.ID, status, err.Error(), false)
			continue
		}
		_ = s.Store.MarkAdAccount(ctx, acct.ID, "connected", "", true)
		synced++
	}
	return synced, problems
}

// AutoStopConfirm is how long a campaign must stay below its auto-stop limit (two syncs this far apart) before
// the rule pauses it; ResumeHold is how long the rule waits after the owner resumes a campaign by hand.
const (
	AutoStopConfirm = 24 * time.Hour
	ResumeHold      = 72 * time.Hour
)

// applyAutoStop pauses live campaigns whose auto-stop rule is met, on the platform first. It never acts on
// revenue it cannot see (no Play service account, or no installs matched to the campaign), waits after a
// manual resume, and needs two below-limit syncs at least AutoStopConfirm apart.
func (s *Server) applyAutoStop(ctx context.Context, accountID int64, p ads.Platform, a ads.Account, tok ads.Token) {
	if s.Play == nil {
		return // purchases are not verified, so revenue (and ROAS) would read as 0
	}
	cs, err := s.Store.Campaigns(ctx, "")
	if err != nil {
		return
	}
	now := s.Now()
	for _, c := range cs {
		r := c.AutoStop
		if c.AdAccountID == nil || *c.AdAccountID != accountID || c.ExternalID == "" || c.Status != "live" || !r.Enabled {
			continue
		}
		if r.HoldUntil != nil && now.Before(*r.HoldUntil) {
			continue
		}
		below := c.Attribution != "none" && c.Days >= r.MinDays && c.SpendPaise >= r.MinSpendPaise && c.ROAS < r.MinROAS
		if !below {
			if r.PendingSince != nil {
				_ = s.Store.SetAutoStrike(ctx, c.ID, nil)
				_ = s.Store.LogCampaign(ctx, c.ID, "rule", "recovered", fmt.Sprintf("ROAS %.2fx, no longer below %.2fx", c.ROAS, r.MinROAS))
			}
			continue
		}
		detail := fmt.Sprintf("ROAS %.2fx after %d days and %s spent (rule: below %.2fx)", c.ROAS, c.Days, rupees(c.SpendPaise), r.MinROAS)
		if r.PendingSince == nil {
			_ = s.Store.SetAutoStrike(ctx, c.ID, &now)
			_ = s.Store.LogCampaign(ctx, c.ID, "rule", "will pause", detail+"; pauses at the next sync after 24 hours unless it improves")
			continue
		}
		if now.Sub(*r.PendingSince) < AutoStopConfirm {
			continue
		}
		if err := p.SetActive(ctx, tok, a, ads.Ref{CampaignID: c.ExternalID, SetID: c.ExternalSetID}, false); err != nil {
			_ = s.Store.SetCampaignStatus(ctx, c.ID, c.Status, "auto-stop could not pause: "+err.Error())
			_ = s.Store.LogCampaign(ctx, c.ID, "rule", "pause failed", err.Error())
			continue
		}
		_ = s.Store.SetCampaignStatus(ctx, c.ID, "paused", "")
		_ = s.Store.SetAutoStrike(ctx, c.ID, nil)
		_ = s.Store.LogCampaign(ctx, c.ID, "rule", "paused", detail)
		slog.Info("auto-stop paused campaign", "campaign", c.ID, "detail", detail)
	}
}

func (s *Server) syncAds(w http.ResponseWriter, r *http.Request) {
	n, problems := s.SyncAds(r.Context(), r.URL.Query().Get("force") == "1")
	writeJSON(w, http.StatusOK, map[string]any{"synced": n, "errors": problems})
}

func oauthPage(w http.ResponseWriter, code int, title, msg string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(code)
	fmt.Fprintf(w, `<!doctype html><meta name="viewport" content="width=device-width,initial-scale=1"><title>Ampli Owner</title>
<body style="font-family:system-ui;background:#F4F2FB;color:#17132B;display:grid;place-items:center;min-height:90vh;margin:0;padding:16px;text-align:center">
<div><div style="font-size:44px">%s</div><h2>%s</h2><p style="color:#6B6585">%s</p></div></body>`,
		map[bool]string{true: "✅", false: "⚠️"}[code == 200], html.EscapeString(title), html.EscapeString(msg))
}
