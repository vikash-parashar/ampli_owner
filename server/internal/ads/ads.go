// Package ads talks to the ad platforms (Meta, TikTok, Google Ads) so the owner can connect accounts,
// launch and steer campaigns and read results without opening each platform's own portal.
package ads

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

// Token is what an OAuth exchange returns.
type Token struct {
	Access  string
	Refresh string
	Expires time.Time // zero when the token does not expire
}

// Account is one ad account the signed-in user can run ads from.
type Account struct {
	ExternalID string
	Name       string
	Currency   string
}

// DayInsight is one campaign's numbers for one day, already converted to paise.
type DayInsight struct {
	CampaignID  string
	Day         string // YYYY-MM-DD
	SpendPaise  int64
	Impressions int64
	Clicks      int64
	Installs    int64
}

// Ref points at a campaign on a platform. SetID is the ad set (Meta), ad group (TikTok)
// or campaign budget resource (Google) that holds the daily budget.
type Ref struct {
	CampaignID string
	SetID      string
}

type Creative struct {
	Kind         string `json:"kind"` // video or image
	URL          string `json:"url"`
	ThumbnailURL string `json:"thumbnail_url"`
	Headline     string `json:"headline"`
	Text         string `json:"text"`
}

// Spec is everything needed to create an app-install campaign.
type Spec struct {
	Product          string
	Name             string
	Countries        []string // ISO codes from Countries
	DailyBudgetPaise int64
	Days             int
	PackageID        string
	Creative         Creative
	Launch           bool // false creates it paused
	Currency         string
	Now              time.Time
}

// StoreURL is the Play listing the ads send people to.
func (s Spec) StoreURL() string {
	return "https://play.google.com/store/apps/details?id=" + url.QueryEscape(s.PackageID)
}

// Platform is one ad network.
type Platform interface {
	ID() string
	Name() string
	Configured() bool
	AuthURL(state, redirect string) string
	Exchange(ctx context.Context, code, redirect string) (Token, []Account, error)
	// Fresh returns a usable token, refreshing it when it is about to expire.
	Fresh(ctx context.Context, t Token) (Token, error)
	Insights(ctx context.Context, t Token, a Account, since, until string) ([]DayInsight, error)
	Create(ctx context.Context, t Token, a Account, s Spec) (Ref, error)
	SetActive(ctx context.Context, t Token, a Account, r Ref, active bool) error
	SetDailyBudget(ctx context.Context, t Token, a Account, r Ref, paise int64) error
}

// Country is a market the wizard offers, with each platform's location id and a rough cost per install.
type Country struct {
	Code      string
	Name      string
	GoogleGeo int
	TikTokLoc int
	CPIPaise  int64
}

var Countries = map[string]Country{
	"IN": {"IN", "India", 2356, 1269750, 600},
	"US": {"US", "USA", 2840, 6252001, 4000},
	"GB": {"GB", "UK", 2826, 2635167, 3500},
	"AE": {"AE", "UAE", 2784, 290557, 2200},
	"CA": {"CA", "Canada", 2124, 6251999, 3800},
}

// ---- verdict ----

type Verdict struct {
	Code   string `json:"code"` // scale, watch, stop, early, new
	Label  string `json:"label"`
	Reason string `json:"reason"`
}

// Judge says whether to keep spending: ROAS 1.5x or more scales, 0.8x-1.5x watch, under 0.8x stop.
func Judge(status string, spendPaise, revenuePaise int64, days int) Verdict {
	switch {
	case status == "draft" || status == "in_review":
		return Verdict{"new", "In review", "Waiting for the platform to approve the ads."}
	case status == "error":
		return Verdict{"stop", "Needs fixing", "The platform rejected a change. Open the campaign to see why."}
	case spendPaise == 0:
		return Verdict{"early", "No spend yet", "Nothing has been spent yet."}
	case days < 5:
		return Verdict{"early", "Too early", fmt.Sprintf("Only %d days of data. Give it 5 days before deciding.", days)}
	}
	roas := float64(revenuePaise) / float64(spendPaise)
	switch {
	case roas >= 1.5:
		return Verdict{"scale", "Scale up", fmt.Sprintf("Every ₹1 spent brought back ₹%.2f. Raise the budget 20%%.", roas)}
	case roas >= 0.8:
		return Verdict{"watch", "Watch", fmt.Sprintf("ROAS %.2fx is near break-even. Try a new creative before adding money.", roas)}
	default:
		return Verdict{"stop", "Stop", fmt.Sprintf("Only ₹%.2f back per ₹1. Pause this and move the budget to a winner.", roas)}
	}
}

// ---- money ----

// fx holds rupees per unit of other currencies; FX_RATES="USD=84,EUR=91" overrides.
var fx = map[string]float64{"INR": 1, "USD": 84, "EUR": 91, "GBP": 108, "AED": 22.9, "CAD": 61}

func init() {
	for _, kv := range strings.Split(os.Getenv("FX_RATES"), ",") {
		k, v, ok := strings.Cut(strings.TrimSpace(kv), "=")
		if f, err := strconv.ParseFloat(v, 64); ok && err == nil && f > 0 {
			fx[strings.ToUpper(k)] = f
		}
	}
}

func rate(cur string) float64 {
	if r, ok := fx[strings.ToUpper(cur)]; ok {
		return r
	}
	return 1
}

// ToPaise converts an amount in the account currency's major unit (e.g. "12.34" USD) to paise.
func ToPaise(major float64, cur string) int64 { return int64(math.Round(major * rate(cur) * 100)) }

// FromPaise converts paise to the account currency's major unit.
func FromPaise(paise int64, cur string) float64 { return float64(paise) / 100 / rate(cur) }

func parseF(s string) float64 { f, _ := strconv.ParseFloat(s, 64); return f }
func parseI(s string) int64   { f, _ := strconv.ParseFloat(s, 64); return int64(math.Round(f)) }

// ---- http ----

type httpc struct{ c *http.Client }

func newHTTP() httpc { return httpc{&http.Client{Timeout: 30 * time.Second}} }

// APIError carries the platform's own message so the owner sees why a call failed.
type APIError struct {
	Platform string
	Status   int
	Msg      string
}

func (e *APIError) Error() string {
	return fmt.Sprintf("%s: %s (HTTP %d)", e.Platform, e.Msg, e.Status)
}

// do sends a request and decodes the JSON reply into out. body may be nil, url.Values (form) or any (JSON).
func (h httpc) do(ctx context.Context, platform, method, u string, hdr map[string]string, body any, out any) error {
	var rd io.Reader
	ctype := ""
	switch b := body.(type) {
	case nil:
	case url.Values:
		rd, ctype = strings.NewReader(b.Encode()), "application/x-www-form-urlencoded"
	default:
		buf, err := json.Marshal(b)
		if err != nil {
			return err
		}
		rd, ctype = bytes.NewReader(buf), "application/json"
	}
	req, err := http.NewRequestWithContext(ctx, method, u, rd)
	if err != nil {
		return err
	}
	if ctype != "" {
		req.Header.Set("Content-Type", ctype)
	}
	for k, v := range hdr {
		req.Header.Set(k, v)
	}
	resp, err := h.c.Do(req)
	if err != nil {
		return fmt.Errorf("%s: %w", platform, err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return err
	}
	if resp.StatusCode >= 300 {
		return &APIError{platform, resp.StatusCode, errorText(raw)}
	}
	if out != nil {
		if err := json.Unmarshal(raw, out); err != nil {
			return fmt.Errorf("%s: bad reply: %w", platform, err)
		}
	}
	return nil
}

// errorText pulls the human message out of the usual error shapes.
func errorText(raw []byte) string {
	var e struct {
		Error any    `json:"error"`
		Msg   string `json:"message"`
	}
	if json.Unmarshal(raw, &e) == nil {
		switch v := e.Error.(type) {
		case map[string]any:
			if m, ok := v["message"].(string); ok {
				if d, ok := v["error_user_msg"].(string); ok && d != "" {
					return m + ": " + d
				}
				return m
			}
		case string:
			return v
		}
		if e.Msg != "" {
			return e.Msg
		}
	}
	s := string(raw)
	if len(s) > 300 {
		s = s[:300]
	}
	return s
}

// productMap parses "mp3=123,carhub=456".
func productMap(s string) map[string]string {
	m := map[string]string{}
	for _, kv := range strings.Split(s, ",") {
		if k, v, ok := strings.Cut(strings.TrimSpace(kv), "="); ok && v != "" {
			m[k] = v
		}
	}
	return m
}

func endTime(s Spec) time.Time {
	if s.Days <= 0 {
		return time.Time{}
	}
	return s.Now.Add(time.Duration(s.Days) * 24 * time.Hour)
}
