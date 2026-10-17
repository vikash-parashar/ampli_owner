package ads

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"
)

// TikTok runs app-install ads through the TikTok API for Business (v1.3).
//
//	TIKTOK_APP_ID, TIKTOK_APP_SECRET  developer app from business-api.tiktok.com
//	TIKTOK_PROMOTED_APPS              mp3=<tiktok app id>,...  (each Android app added in TikTok Events Manager)
type TikTok struct {
	AppID, Secret string
	Promoted      map[string]string
	Base          string // https://business-api.tiktok.com/open_api/v1.3
	Portal        string // https://business-api.tiktok.com/portal/auth
	h             httpc
}

func NewTikTok(appID, secret, promoted string) *TikTok {
	return &TikTok{AppID: appID, Secret: secret, Promoted: productMap(promoted),
		Base: "https://business-api.tiktok.com/open_api/v1.3", Portal: "https://business-api.tiktok.com/portal/auth", h: newHTTP()}
}

func (k *TikTok) ID() string       { return "tiktok" }
func (k *TikTok) Name() string     { return "TikTok" }
func (k *TikTok) Configured() bool { return k.AppID != "" && k.Secret != "" }

func (k *TikTok) AuthURL(state, redirect string) string {
	return k.Portal + "?" + url.Values{"app_id": {k.AppID}, "state": {state}, "redirect_uri": {redirect}}.Encode()
}

// call wraps TikTok's {code, message, data} envelope.
func (k *TikTok) call(ctx context.Context, t Token, method, path string, q url.Values, body any, data any) error {
	u := k.Base + "/" + strings.TrimPrefix(path, "/")
	if q != nil {
		u += "?" + q.Encode()
	}
	hdr := map[string]string{}
	if t.Access != "" {
		hdr["Access-Token"] = t.Access
	}
	var env struct {
		Code    int             `json:"code"`
		Message string          `json:"message"`
		Data    json.RawMessage `json:"data"`
	}
	if err := k.h.do(ctx, "TikTok", method, u, hdr, body, &env); err != nil {
		return err
	}
	if env.Code != 0 {
		return &APIError{"TikTok", 200, env.Message}
	}
	if data != nil && len(env.Data) > 0 {
		return json.Unmarshal(env.Data, data)
	}
	return nil
}

func (k *TikTok) Exchange(ctx context.Context, code, _ string) (Token, []Account, error) {
	var d struct {
		AccessToken string `json:"access_token"`
	}
	if err := k.call(ctx, Token{}, "POST", "oauth2/access_token/", nil,
		map[string]string{"app_id": k.AppID, "secret": k.Secret, "auth_code": code}, &d); err != nil {
		return Token{}, nil, err
	}
	t := Token{Access: d.AccessToken}
	var list struct {
		List []struct {
			ID   string `json:"advertiser_id"`
			Name string `json:"advertiser_name"`
		} `json:"list"`
	}
	if err := k.call(ctx, t, "GET", "oauth2/advertiser/get/", url.Values{"app_id": {k.AppID}, "secret": {k.Secret}}, nil, &list); err != nil {
		return Token{}, nil, err
	}
	out := make([]Account, 0, len(list.List))
	ids := make([]string, 0, len(list.List))
	for _, a := range list.List {
		out = append(out, Account{ExternalID: a.ID, Name: a.Name, Currency: "USD"})
		ids = append(ids, a.ID)
	}
	if len(ids) > 0 {
		idsJSON, _ := json.Marshal(ids)
		var info struct {
			List []struct {
				ID       string `json:"advertiser_id"`
				Currency string `json:"currency"`
			} `json:"list"`
		}
		if k.call(ctx, t, "GET", "advertiser/info/", url.Values{"advertiser_ids": {string(idsJSON)}, "fields": {`["advertiser_id","currency"]`}}, nil, &info) == nil {
			for _, in := range info.List {
				for i := range out {
					if out[i].ExternalID == in.ID && in.Currency != "" {
						out[i].Currency = in.Currency
					}
				}
			}
		}
	}
	return t, out, nil
}

// Fresh: TikTok advertiser tokens do not expire unless the owner revokes them.
func (k *TikTok) Fresh(_ context.Context, t Token) (Token, error) { return t, nil }

func (k *TikTok) Insights(ctx context.Context, t Token, a Account, since, until string) ([]DayInsight, error) {
	var out []DayInsight
	for page := 1; page <= 50; page++ {
		q := url.Values{"advertiser_id": {a.ExternalID}, "report_type": {"BASIC"}, "data_level": {"AUCTION_CAMPAIGN"},
			"dimensions": {`["campaign_id","stat_time_day"]`}, "metrics": {`["spend","impressions","clicks","app_install"]`},
			"start_date": {since}, "end_date": {until}, "page": {fmt.Sprint(page)}, "page_size": {"1000"}}
		var d struct {
			List []struct {
				Dim map[string]string `json:"dimensions"`
				Met map[string]string `json:"metrics"`
			} `json:"list"`
			PageInfo struct {
				TotalPage int `json:"total_page"`
			} `json:"page_info"`
		}
		if err := k.call(ctx, t, "GET", "report/integrated/get/", q, nil, &d); err != nil {
			return nil, err
		}
		for _, r := range d.List {
			day := r.Dim["stat_time_day"]
			if len(day) > 10 {
				day = day[:10]
			}
			out = append(out, DayInsight{CampaignID: r.Dim["campaign_id"], Day: day, SpendPaise: ToPaise(parseF(r.Met["spend"]), a.Currency),
				Impressions: parseI(r.Met["impressions"]), Clicks: parseI(r.Met["clicks"]), Installs: parseI(r.Met["app_install"])})
		}
		if page >= d.PageInfo.TotalPage {
			break
		}
	}
	return out, nil
}

func (k *TikTok) Create(ctx context.Context, t Token, a Account, s Spec) (Ref, error) {
	var ref Ref
	appID := k.Promoted[s.Product]
	if appID == "" {
		return ref, fmt.Errorf("TikTok: add %s in TikTok Events Manager and put its app id in TIKTOK_PROMOTED_APPS", s.Product)
	}
	if s.Creative.Kind != "video" {
		return ref, errors.New("TikTok ads need a video creative")
	}
	status := "DISABLE"
	if s.Launch {
		status = "ENABLE"
	}
	var c struct {
		ID string `json:"campaign_id"`
	}
	if err := k.call(ctx, t, "POST", "campaign/create/", nil, map[string]any{"advertiser_id": a.ExternalID, "campaign_name": s.Name,
		"objective_type": "APP_PROMOTION", "app_promotion_type": "APP_INSTALL", "budget_mode": "BUDGET_MODE_INFINITE",
		"operation_status": status}, &c); err != nil {
		return ref, err
	}
	ref.CampaignID = c.ID

	locs := make([]string, 0, len(s.Countries))
	for _, cc := range s.Countries {
		locs = append(locs, fmt.Sprint(Countries[cc].TikTokLoc))
	}
	group := map[string]any{"advertiser_id": a.ExternalID, "campaign_id": ref.CampaignID, "adgroup_name": s.Name + " · group",
		"promotion_type": "APP_ANDROID", "app_id": appID, "placement_type": "PLACEMENT_TYPE_NORMAL", "placements": []string{"PLACEMENT_TIKTOK"},
		"location_ids": locs, "operating_systems": []string{"ANDROID"}, "budget_mode": "BUDGET_MODE_DAY",
		"budget": FromPaise(s.DailyBudgetPaise, a.Currency), "optimization_goal": "INSTALL", "billing_event": "OCPM",
		"bid_type": "BID_TYPE_NO_BID", "pacing": "PACING_MODE_SMOOTH", "operation_status": status,
		"schedule_type": "SCHEDULE_FROM_NOW", "schedule_start_time": s.Now.UTC().Format(time.DateTime)}
	if end := endTime(s); !end.IsZero() {
		group["schedule_type"] = "SCHEDULE_START_END"
		group["schedule_end_time"] = end.UTC().Format(time.DateTime)
	}
	var g struct {
		ID string `json:"adgroup_id"`
	}
	if err := k.call(ctx, t, "POST", "adgroup/create/", nil, group, &g); err != nil {
		return ref, err
	}
	ref.SetID = g.ID

	var vids []struct {
		ID string `json:"video_id"`
	}
	if err := k.call(ctx, t, "POST", "file/video/ad/upload/", nil, map[string]string{"advertiser_id": a.ExternalID,
		"upload_type": "UPLOAD_BY_URL", "video_url": s.Creative.URL}, &vids); err != nil {
		return ref, err
	}
	if len(vids) == 0 {
		return ref, errors.New("TikTok: video upload returned no id")
	}
	creative := map[string]any{"ad_name": s.Name + " · ad", "ad_format": "SINGLE_VIDEO", "video_id": vids[0].ID,
		"ad_text": firstNonEmpty(s.Creative.Text, s.Creative.Headline, s.Name), "call_to_action": "DOWNLOAD_NOW"}
	if s.Creative.ThumbnailURL != "" {
		var img struct {
			ID string `json:"image_id"`
		}
		if err := k.call(ctx, t, "POST", "file/image/ad/upload/", nil, map[string]string{"advertiser_id": a.ExternalID,
			"upload_type": "UPLOAD_BY_URL", "image_url": s.Creative.ThumbnailURL}, &img); err == nil && img.ID != "" {
			creative["image_ids"] = []string{img.ID}
		}
	}
	var ident struct {
		ID string `json:"identity_id"`
	}
	if err := k.call(ctx, t, "POST", "identity/create/", nil, map[string]string{"advertiser_id": a.ExternalID, "display_name": "Ampli"}, &ident); err != nil {
		return ref, err
	}
	creative["identity_type"], creative["identity_id"] = "CUSTOMIZED_USER", ident.ID
	return ref, k.call(ctx, t, "POST", "ad/create/", nil, map[string]any{"advertiser_id": a.ExternalID, "adgroup_id": ref.SetID,
		"creatives": []any{creative}}, nil)
}

func (k *TikTok) SetActive(ctx context.Context, t Token, a Account, r Ref, active bool) error {
	status := "DISABLE"
	if active {
		status = "ENABLE"
	}
	return k.call(ctx, t, "POST", "campaign/status/update/", nil, map[string]any{"advertiser_id": a.ExternalID,
		"campaign_ids": []string{r.CampaignID}, "operation_status": status}, nil)
}

func (k *TikTok) SetDailyBudget(ctx context.Context, t Token, a Account, r Ref, paise int64) error {
	if r.SetID == "" {
		return errors.New("TikTok: this campaign has no ad group to change")
	}
	return k.call(ctx, t, "POST", "adgroup/budget/update/", nil, map[string]any{"advertiser_id": a.ExternalID,
		"budget": []map[string]any{{"adgroup_id": r.SetID, "budget": FromPaise(paise, a.Currency)}}}, nil)
}

func firstNonEmpty(v ...string) string {
	for _, s := range v {
		if strings.TrimSpace(s) != "" {
			return s
		}
	}
	return ""
}
