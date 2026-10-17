package ads

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"
)

// Google runs App campaigns (Play, YouTube, Search, Display) through the Google Ads API.
//
//	GOOGLE_ADS_CLIENT_ID, GOOGLE_ADS_CLIENT_SECRET  OAuth client (Web application) from Google Cloud
//	GOOGLE_ADS_DEVELOPER_TOKEN                      from the Google Ads manager account's API Center
//	GOOGLE_ADS_LOGIN_CUSTOMER_ID                    optional manager account id (digits only)
//	GOOGLE_ADS_API_VERSION                          optional, default v21
type Google struct {
	ClientID, Secret, DevToken, LoginCustomer string
	Base                                      string // https://googleads.googleapis.com/v21
	AuthBase, TokenURL                        string
	h                                         httpc
}

func NewGoogle(clientID, secret, devToken, loginCustomer, version string) *Google {
	if version == "" {
		version = "v21"
	}
	return &Google{ClientID: clientID, Secret: secret, DevToken: devToken, LoginCustomer: strings.ReplaceAll(loginCustomer, "-", ""),
		Base: "https://googleads.googleapis.com/" + version, AuthBase: "https://accounts.google.com/o/oauth2/v2/auth",
		TokenURL: "https://oauth2.googleapis.com/token", h: newHTTP()}
}

func (g *Google) ID() string       { return "google" }
func (g *Google) Name() string     { return "Google Ads" }
func (g *Google) Configured() bool { return g.ClientID != "" && g.Secret != "" && g.DevToken != "" }

func (g *Google) AuthURL(state, redirect string) string {
	return g.AuthBase + "?" + url.Values{"client_id": {g.ClientID}, "redirect_uri": {redirect}, "response_type": {"code"},
		"scope": {"https://www.googleapis.com/auth/adwords"}, "access_type": {"offline"}, "prompt": {"consent"}, "state": {state}}.Encode()
}

type gToken struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    int64  `json:"expires_in"`
}

func (g *Google) token(ctx context.Context, form url.Values) (Token, error) {
	form.Set("client_id", g.ClientID)
	form.Set("client_secret", g.Secret)
	var tk gToken
	if err := g.h.do(ctx, "Google", "POST", g.TokenURL, nil, form, &tk); err != nil {
		return Token{}, err
	}
	return Token{Access: tk.AccessToken, Refresh: tk.RefreshToken, Expires: time.Now().Add(time.Duration(tk.ExpiresIn) * time.Second)}, nil
}

func (g *Google) hdr(t Token) map[string]string {
	h := map[string]string{"Authorization": "Bearer " + t.Access, "developer-token": g.DevToken}
	if g.LoginCustomer != "" {
		h["login-customer-id"] = g.LoginCustomer
	}
	return h
}

type gRow map[string]map[string]any

func (g *Google) search(ctx context.Context, t Token, customer, query string) ([]gRow, error) {
	var out []gRow
	body := map[string]any{"query": query}
	for page := 0; page < 50; page++ {
		var res struct {
			Results       []gRow `json:"results"`
			NextPageToken string `json:"nextPageToken"`
		}
		if err := g.h.do(ctx, "Google Ads", "POST", g.Base+"/customers/"+customer+"/googleAds:search", g.hdr(t), body, &res); err != nil {
			return nil, err
		}
		out = append(out, res.Results...)
		if res.NextPageToken == "" {
			break
		}
		body["pageToken"] = res.NextPageToken
	}
	return out, nil
}

func (g *Google) Exchange(ctx context.Context, code, redirect string) (Token, []Account, error) {
	t, err := g.token(ctx, url.Values{"grant_type": {"authorization_code"}, "code": {code}, "redirect_uri": {redirect}})
	if err != nil {
		return Token{}, nil, err
	}
	var list struct {
		ResourceNames []string `json:"resourceNames"`
	}
	if err := g.h.do(ctx, "Google Ads", "GET", g.Base+"/customers:listAccessibleCustomers", g.hdr(t), nil, &list); err != nil {
		return Token{}, nil, err
	}
	var out []Account
	for _, rn := range list.ResourceNames {
		id := strings.TrimPrefix(rn, "customers/")
		rows, err := g.search(ctx, t, id, "SELECT customer.descriptive_name, customer.currency_code, customer.manager FROM customer")
		if err != nil || len(rows) == 0 {
			out = append(out, Account{ExternalID: id, Name: id, Currency: "INR"})
			continue
		}
		c := rows[0]["customer"]
		if m, _ := c["manager"].(bool); m {
			continue // manager accounts cannot run ads themselves
		}
		name, _ := c["descriptiveName"].(string)
		cur, _ := c["currencyCode"].(string)
		out = append(out, Account{ExternalID: id, Name: firstNonEmpty(name, id), Currency: firstNonEmpty(cur, "INR")})
	}
	return t, out, nil
}

func (g *Google) Fresh(ctx context.Context, t Token) (Token, error) {
	if time.Until(t.Expires) > 2*time.Minute {
		return t, nil
	}
	if t.Refresh == "" {
		return t, ErrExpired
	}
	nt, err := g.token(ctx, url.Values{"grant_type": {"refresh_token"}, "refresh_token": {t.Refresh}})
	if err != nil {
		return t, err
	}
	nt.Refresh = t.Refresh
	return nt, nil
}

func str(v any) string { s, _ := v.(string); return s }

func num(v any) float64 {
	switch x := v.(type) {
	case float64:
		return x
	case string:
		return parseF(x)
	}
	return 0
}

func (g *Google) Insights(ctx context.Context, t Token, a Account, since, until string) ([]DayInsight, error) {
	rows, err := g.search(ctx, t, a.ExternalID, fmt.Sprintf(`SELECT campaign.id, segments.date, metrics.cost_micros,
		metrics.impressions, metrics.clicks, metrics.conversions FROM campaign WHERE segments.date BETWEEN '%s' AND '%s'`, since, until))
	if err != nil {
		return nil, err
	}
	out := make([]DayInsight, 0, len(rows))
	for _, r := range rows {
		m := r["metrics"]
		out = append(out, DayInsight{CampaignID: str(r["campaign"]["id"]), Day: str(r["segments"]["date"]),
			SpendPaise: ToPaise(num(m["costMicros"])/1e6, a.Currency), Impressions: int64(num(m["impressions"])),
			Clicks: int64(num(m["clicks"])), Installs: int64(num(m["conversions"]) + 0.5)})
	}
	return out, nil
}

func (g *Google) mutate(ctx context.Context, t Token, customer, resource string, ops []map[string]any) ([]string, error) {
	var res struct {
		Results []struct {
			ResourceName string `json:"resourceName"`
		} `json:"results"`
	}
	if err := g.h.do(ctx, "Google Ads", "POST", g.Base+"/customers/"+customer+"/"+resource+":mutate", g.hdr(t),
		map[string]any{"operations": ops}, &res); err != nil {
		return nil, err
	}
	names := make([]string, 0, len(res.Results))
	for _, r := range res.Results {
		names = append(names, r.ResourceName)
	}
	if len(names) == 0 {
		return nil, errors.New("Google Ads: mutate returned nothing")
	}
	return names, nil
}

func micros(paise int64, cur string) string { return fmt.Sprint(int64(FromPaise(paise, cur) * 1e6)) }

func (g *Google) Create(ctx context.Context, t Token, a Account, s Spec) (Ref, error) {
	var ref Ref
	status := "PAUSED"
	if s.Launch {
		status = "ENABLED"
	}
	budget, err := g.mutate(ctx, t, a.ExternalID, "campaignBudgets", []map[string]any{{"create": map[string]any{
		"name": s.Name + " budget " + s.Now.Format("150405"), "amountMicros": micros(s.DailyBudgetPaise, a.Currency),
		"deliveryMethod": "STANDARD", "explicitlyShared": false}}})
	if err != nil {
		return ref, err
	}
	ref.SetID = budget[0]
	// Target cost per install: the cheapest market's typical CPI.
	var cpi int64
	for _, c := range s.Countries {
		if v := Countries[c].CPIPaise; cpi == 0 || v < cpi {
			cpi = v
		}
	}
	camp := map[string]any{"name": s.Name, "status": status, "advertisingChannelType": "MULTI_CHANNEL",
		"advertisingChannelSubType": "APP_CAMPAIGN", "campaignBudget": ref.SetID,
		"appCampaignSetting": map[string]any{"appId": s.PackageID, "appStore": "GOOGLE_APP_STORE",
			"biddingStrategyGoalType": "OPTIMIZE_INSTALLS_TARGET_INSTALL_COST"},
		"targetCpa":                      map[string]any{"targetCpaMicros": micros(cpi, a.Currency)},
		"containsEuPoliticalAdvertising": "DOES_NOT_CONTAIN_EU_POLITICAL_ADVERTISING"}
	if end := endTime(s); !end.IsZero() {
		camp["endDate"] = end.Format(time.DateOnly)
	}
	cs, err := g.mutate(ctx, t, a.ExternalID, "campaigns", []map[string]any{{"create": camp}})
	if err != nil {
		return ref, err
	}
	rn := cs[0]
	ref.CampaignID = rn[strings.LastIndex(rn, "/")+1:]

	var crit []map[string]any
	for _, c := range s.Countries {
		crit = append(crit, map[string]any{"create": map[string]any{"campaign": rn,
			"location": map[string]any{"geoTargetConstant": fmt.Sprintf("geoTargetConstants/%d", Countries[c].GoogleGeo)}}})
	}
	if _, err := g.mutate(ctx, t, a.ExternalID, "campaignCriteria", crit); err != nil {
		return ref, err
	}
	ag, err := g.mutate(ctx, t, a.ExternalID, "adGroups", []map[string]any{{"create": map[string]any{
		"name": s.Name + " · group", "campaign": rn, "status": "ENABLED"}}})
	if err != nil {
		return ref, err
	}
	// Text-only app ad: Google builds images and video from the Play listing.
	_, err = g.mutate(ctx, t, a.ExternalID, "adGroupAds", []map[string]any{{"create": map[string]any{
		"adGroup": ag[0], "status": "ENABLED", "ad": map[string]any{"appAd": map[string]any{
			"headlines":    []map[string]string{{"text": clip(firstNonEmpty(s.Creative.Headline, s.Name), 30)}},
			"descriptions": []map[string]string{{"text": clip(firstNonEmpty(s.Creative.Text, s.Creative.Headline, s.Name), 90)}},
		}}}}})
	return ref, err
}

func clip(s string, n int) string {
	r := []rune(s)
	if len(r) > n {
		return string(r[:n])
	}
	return s
}

func (g *Google) SetActive(ctx context.Context, t Token, a Account, r Ref, active bool) error {
	status := "PAUSED"
	if active {
		status = "ENABLED"
	}
	_, err := g.mutate(ctx, t, a.ExternalID, "campaigns", []map[string]any{{"updateMask": "status",
		"update": map[string]any{"resourceName": "customers/" + a.ExternalID + "/campaigns/" + r.CampaignID, "status": status}}})
	return err
}

func (g *Google) SetDailyBudget(ctx context.Context, t Token, a Account, r Ref, paise int64) error {
	if r.SetID == "" {
		return errors.New("Google Ads: this campaign has no budget to change")
	}
	_, err := g.mutate(ctx, t, a.ExternalID, "campaignBudgets", []map[string]any{{"updateMask": "amount_micros",
		"update": map[string]any{"resourceName": r.SetID, "amountMicros": micros(paise, a.Currency)}}})
	return err
}
