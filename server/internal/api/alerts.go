package api

import (
	"fmt"
	"net/http"
	"sort"
)

// Play's bad-behaviour thresholds: above these the app is shown less in the store.
const (
	PlayCrashLimit = 0.0109
	PlayANRLimit   = 0.0047
)

type Alert struct {
	ID       string `json:"id"`
	Severity string `json:"severity"` // high, medium, low
	Product  string `json:"product"`  // "" when it is not about one app
	Title    string `json:"title"`
	Detail   string `json:"detail"`
	Action   string `json:"action"` // open_ads, open_reviews, open_inbox, open_settings or ""
}

var severityRank = map[string]int{"high": 0, "medium": 1, "low": 2}

func (s *Server) alerts(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	out := []Alert{}
	add := func(a Alert) { out = append(out, a) }

	vitals, err := s.Store.LatestVitals(ctx)
	if err != nil {
		fail(w, r, err)
		return
	}
	for _, v := range vitals {
		check := func(kind string, rate *float64, limit float64) {
			if rate == nil || *rate < limit*0.8 {
				return
			}
			sev, verb := "medium", "close to"
			if *rate >= limit {
				sev, verb = "high", "over"
			}
			add(Alert{ID: "vitals-" + kind + "-" + v.ProductID, Severity: sev, Product: v.ProductID,
				Title:  fmt.Sprintf("%s rate %.2f%%, %s Play's limit %.2f%%", map[string]string{"crash": "Crash", "anr": "ANR"}[kind], *rate*100, verb, limit*100),
				Detail: "Above this limit Google Play shows the app to fewer people. Check the latest crashes and fix the top one first."})
		}
		check("crash", v.CrashRate, PlayCrashLimit)
		check("anr", v.ANRRate, PlayANRLimit)
	}

	pulses, err := s.Store.ReviewPulses(ctx, s.Now())
	if err != nil {
		fail(w, r, err)
		return
	}
	for _, p := range pulses {
		if p.BadUnreplied > 0 {
			add(Alert{ID: "reviews-bad-" + p.ProductID, Severity: "medium", Product: p.ProductID, Action: "open_reviews",
				Title:  fmt.Sprintf("%d new 1-2★ review%s without a reply", p.BadUnreplied, plural(p.BadUnreplied)),
				Detail: "A quick, kind reply often turns a 1★ into a 4★. Reply from the Reviews tab."})
		}
		if p.AvgWeek != nil && p.AvgBefore != nil && *p.AvgBefore-*p.AvgWeek >= 0.3 {
			add(Alert{ID: "rating-drop-" + p.ProductID, Severity: "medium", Product: p.ProductID, Action: "open_reviews",
				Title:  fmt.Sprintf("Rating fell to %.1f★ this week (was %.1f★)", *p.AvgWeek, *p.AvgBefore),
				Detail: "Read this week's reviews for what changed, often the latest update."})
		}
	}

	cs, err := s.Store.Campaigns(ctx, "")
	if err != nil {
		fail(w, r, err)
		return
	}
	for _, c := range cs {
		switch {
		case c.Status == "error":
			add(Alert{ID: fmt.Sprintf("campaign-error-%d", c.ID), Severity: "high", Product: c.ProductID, Action: "open_ads",
				Title: c.Name + " has a problem", Detail: firstOf(c.LastError, "Open it in Ads to see what the platform said.")})
		case c.Status == "live" && c.Verdict.Code == "stop":
			add(Alert{ID: fmt.Sprintf("campaign-stop-%d", c.ID), Severity: "high", Product: c.ProductID, Action: "open_ads",
				Title:  fmt.Sprintf("%s is losing money (%.2fx back)", c.Name, c.ROAS),
				Detail: fmt.Sprintf("%s spent, %s back. Pause it or turn on auto-stop.", rupees(c.SpendPaise), rupees(c.RevenuePaise))})
		case c.Status == "live" && c.LastError != "":
			add(Alert{ID: fmt.Sprintf("campaign-warn-%d", c.ID), Severity: "medium", Product: c.ProductID, Action: "open_ads",
				Title: c.Name + " needs a look", Detail: c.LastError})
		}
	}

	accts, err := s.Store.AdAccounts(ctx)
	if err != nil {
		fail(w, r, err)
		return
	}
	for _, a := range accts {
		if a.Status == "expired" {
			add(Alert{ID: fmt.Sprintf("account-expired-%d", a.ID), Severity: "high", Action: "open_ads",
				Title: a.Name + " sign-in expired", Detail: "Connect it again in Ads so results keep syncing and auto-stop keeps working."})
		}
	}
	ints, err := s.Store.Integrations(ctx)
	if err != nil {
		fail(w, r, err)
		return
	}
	if am := ints["admob"]; am.Connected && am.LastError != "" {
		add(Alert{ID: "admob-error", Severity: "medium", Action: "open_settings", Title: "AdMob earnings are not syncing", Detail: am.LastError})
	}
	if pl := ints["play"]; pl.LastError != "" {
		add(Alert{ID: "play-error", Severity: "low", Action: "open_settings", Title: "Google Play sync had a problem", Detail: pl.LastError})
	}

	waiting, err := s.Store.WaitingOnOwner(ctx)
	if err != nil {
		fail(w, r, err)
		return
	}
	if waiting > 0 {
		add(Alert{ID: "tickets-waiting", Severity: "low", Action: "open_inbox",
			Title: fmt.Sprintf("%d ticket%s waiting for your reply", waiting, plural(waiting)), Detail: "Users see your reply in the app's Help screen."})
	}

	sort.SliceStable(out, func(i, j int) bool { return severityRank[out[i].Severity] < severityRank[out[j].Severity] })
	writeJSON(w, http.StatusOK, out)
}

func plural(n int64) string {
	if n == 1 {
		return ""
	}
	return "s"
}
