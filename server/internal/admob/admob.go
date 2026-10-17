// Package admob reads daily AdMob earnings per app (AdMob API v1, OAuth with the admob.readonly scope).
// It reuses the Google OAuth client from the Google Ads setup (GOOGLE_ADS_CLIENT_ID / _SECRET).
package admob

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

type Client struct {
	ClientID, Secret string
	Apps             map[string]string // AdMob app id → product id
	AuthBase         string
	TokenURL         string
	Base             string // https://admob.googleapis.com/v1
	h                *http.Client
}

// New parses ADMOB_APPS ("mp3=ca-app-pub-...~123,naukri=...").
func New(clientID, secret, apps string) *Client {
	m := map[string]string{}
	for _, kv := range strings.Split(apps, ",") {
		if k, v, ok := strings.Cut(strings.TrimSpace(kv), "="); ok && v != "" {
			m[v] = k
		}
	}
	return &Client{ClientID: clientID, Secret: secret, Apps: m, AuthBase: "https://accounts.google.com/o/oauth2/v2/auth",
		TokenURL: "https://oauth2.googleapis.com/token", Base: "https://admob.googleapis.com/v1", h: &http.Client{Timeout: 30 * time.Second}}
}

func (c *Client) Configured() bool { return c.ClientID != "" && c.Secret != "" }

func (c *Client) AuthURL(state, redirect string) string {
	return c.AuthBase + "?" + url.Values{"client_id": {c.ClientID}, "redirect_uri": {redirect}, "response_type": {"code"},
		"scope": {"https://www.googleapis.com/auth/admob.readonly"}, "access_type": {"offline"}, "prompt": {"consent"}, "state": {state}}.Encode()
}

type Token struct {
	Access, Refresh string
	Expires         time.Time
}

func (c *Client) do(ctx context.Context, method, u, bearer string, body any, out any) ([]byte, error) {
	var rd io.Reader
	ctype := ""
	switch b := body.(type) {
	case nil:
	case url.Values:
		rd, ctype = strings.NewReader(b.Encode()), "application/x-www-form-urlencoded"
	default:
		buf, _ := json.Marshal(b)
		rd, ctype = bytes.NewReader(buf), "application/json"
	}
	req, err := http.NewRequestWithContext(ctx, method, u, rd)
	if err != nil {
		return nil, err
	}
	if ctype != "" {
		req.Header.Set("Content-Type", ctype)
	}
	if bearer != "" {
		req.Header.Set("Authorization", "Bearer "+bearer)
	}
	resp, err := c.h.Do(req)
	if err != nil {
		return nil, fmt.Errorf("AdMob: %w", err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 16<<20))
	if resp.StatusCode >= 300 {
		var e struct {
			Error any    `json:"error"`
			Desc  string `json:"error_description"`
		}
		_ = json.Unmarshal(raw, &e)
		msg := e.Desc
		if m, ok := e.Error.(map[string]any); ok {
			msg, _ = m["message"].(string)
		} else if s, ok := e.Error.(string); ok && msg == "" {
			msg = s
		}
		return nil, fmt.Errorf("AdMob: %s (HTTP %d)", msg, resp.StatusCode)
	}
	if out != nil {
		return raw, json.Unmarshal(raw, out)
	}
	return raw, nil
}

func (c *Client) token(ctx context.Context, form url.Values) (Token, error) {
	form.Set("client_id", c.ClientID)
	form.Set("client_secret", c.Secret)
	var t struct {
		Access  string `json:"access_token"`
		Refresh string `json:"refresh_token"`
		Expires int64  `json:"expires_in"`
	}
	if _, err := c.do(ctx, "POST", c.TokenURL, "", form, &t); err != nil {
		return Token{}, err
	}
	return Token{t.Access, t.Refresh, time.Now().Add(time.Duration(t.Expires) * time.Second)}, nil
}

// Exchange trades the sign-in code for tokens and returns the publisher account ("accounts/pub-…").
func (c *Client) Exchange(ctx context.Context, code, redirect string) (Token, string, error) {
	t, err := c.token(ctx, url.Values{"grant_type": {"authorization_code"}, "code": {code}, "redirect_uri": {redirect}})
	if err != nil {
		return t, "", err
	}
	var res struct {
		Account []struct {
			Name string `json:"name"`
		} `json:"account"`
	}
	if _, err := c.do(ctx, "GET", c.Base+"/accounts", t.Access, nil, &res); err != nil {
		return t, "", err
	}
	if len(res.Account) == 0 {
		return t, "", fmt.Errorf("AdMob: this Google login has no AdMob account")
	}
	return t, res.Account[0].Name, nil
}

// Fresh refreshes the access token when it is about to expire.
func (c *Client) Fresh(ctx context.Context, t Token) (Token, error) {
	if time.Until(t.Expires) > 2*time.Minute {
		return t, nil
	}
	if t.Refresh == "" {
		return t, fmt.Errorf("AdMob: access expired, connect again")
	}
	nt, err := c.token(ctx, url.Values{"grant_type": {"refresh_token"}, "refresh_token": {t.Refresh}})
	nt.Refresh = t.Refresh
	return nt, err
}

// Earning is one product's AdMob earnings for one day, in paise.
type Earning struct {
	Product string
	Day     time.Time
	Paise   int64
}

func gdate(t time.Time) map[string]int {
	return map[string]int{"year": t.Year(), "month": int(t.Month()), "day": t.Day()}
}

// Earnings returns estimated earnings per day and product (apps not in ADMOB_APPS are skipped).
func (c *Client) Earnings(ctx context.Context, t Token, account string, from, to time.Time) ([]Earning, error) {
	body := map[string]any{"reportSpec": map[string]any{
		"dateRange":            map[string]any{"startDate": gdate(from), "endDate": gdate(to)},
		"dimensions":           []string{"DATE", "APP"},
		"metrics":              []string{"ESTIMATED_EARNINGS"},
		"localizationSettings": map[string]string{"currencyCode": "INR"},
	}}
	raw, err := c.do(ctx, "POST", c.Base+"/"+account+"/networkReport:generate", t.Access, body, nil)
	if err != nil {
		return nil, err
	}
	var parts []struct {
		Row *struct {
			Dims map[string]struct {
				Value string `json:"value"`
			} `json:"dimensionValues"`
			Metrics map[string]struct {
				Micros string `json:"microsValue"`
			} `json:"metricValues"`
		} `json:"row"`
	}
	if err := json.Unmarshal(raw, &parts); err != nil {
		return nil, fmt.Errorf("AdMob: bad report: %w", err)
	}
	sum := map[string]*Earning{}
	for _, p := range parts {
		if p.Row == nil {
			continue
		}
		prod := c.Apps[p.Row.Dims["APP"].Value]
		day, err := time.Parse("20060102", p.Row.Dims["DATE"].Value)
		if prod == "" || err != nil {
			continue
		}
		micros, _ := strconv.ParseInt(p.Row.Metrics["ESTIMATED_EARNINGS"].Micros, 10, 64)
		k := prod + day.Format("20060102")
		if sum[k] == nil {
			sum[k] = &Earning{Product: prod, Day: day}
		}
		sum[k].Paise += micros / 10000
	}
	out := make([]Earning, 0, len(sum))
	for _, e := range sum {
		out = append(out, *e)
	}
	return out, nil
}
