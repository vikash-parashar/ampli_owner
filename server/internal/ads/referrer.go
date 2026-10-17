package ads

import (
	"net/url"
	"strings"
)

// Referrer is the parsed Play Install Referrer.
type Referrer struct {
	Source, Medium, Campaign, Content string
	Platform                          string // meta, tiktok, google or "" for organic
}

// ParseReferrer reads a raw Play Install Referrer string such as
// "utm_source=apps.facebook.com&utm_campaign=fb4a&utm_content=..." or "gclid=...".
func ParseReferrer(raw string) Referrer {
	q, _ := url.ParseQuery(strings.TrimSpace(raw))
	r := Referrer{Source: clip(q.Get("utm_source"), 120), Medium: clip(q.Get("utm_medium"), 120),
		Campaign: clip(q.Get("utm_campaign"), 120), Content: clip(q.Get("utm_content"), 500)}
	src := strings.ToLower(r.Source)
	switch {
	case strings.HasPrefix(r.Campaign, "ampli-c"), strings.HasPrefix(r.Campaign, "ampli-l"):
		// Our own links (ad campaign or tracked channel link): matched by utm_campaign, not by platform, so an
		// unpaid Instagram or WhatsApp link is never counted as a paid Meta install.
	case strings.Contains(src, "facebook") || strings.Contains(src, "instagram") || src == "meta" || src == "fb":
		r.Platform = "meta"
	case strings.Contains(src, "tiktok") || strings.Contains(src, "bytedance"):
		r.Platform = "tiktok"
	case q.Get("gclid") != "" || ((src == "google" || src == "google-play" || src == "youtube") && r.Medium != "organic"):
		r.Platform = "google"
	}
	return r
}
