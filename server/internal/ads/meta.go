package ads

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// Meta runs Facebook + Instagram app-install ads through the Marketing API.
//
//	META_APP_ID, META_APP_SECRET  developer app from developers.facebook.com (Marketing API product)
//	META_PAGE_ID                  Facebook Page the ads run as
//	META_PROMOTED_APPS            mp3=<meta app id>,carhub=...,naukri=...  (each Android app registered in Meta)
type Meta struct {
	AppID, AppSecret, PageID string
	Promoted                 map[string]string
	Graph                    string // https://graph.facebook.com/v26.0
	Dialog                   string // https://www.facebook.com/v26.0/dialog/oauth
	h                        httpc
}

func NewMeta(appID, secret, pageID, promoted, version string) *Meta {
	if version == "" {
		version = "v26.0"
	}
	return &Meta{AppID: appID, AppSecret: secret, PageID: pageID, Promoted: productMap(promoted),
		Graph: "https://graph.facebook.com/" + version, Dialog: "https://www.facebook.com/" + version + "/dialog/oauth", h: newHTTP()}
}

func (m *Meta) ID() string       { return "meta" }
func (m *Meta) Name() string     { return "Meta" }
func (m *Meta) Configured() bool { return m.AppID != "" && m.AppSecret != "" }

func (m *Meta) AuthURL(state, redirect string) string {
	q := url.Values{"client_id": {m.AppID}, "redirect_uri": {redirect}, "state": {state},
		"scope": {"ads_management,ads_read,business_management,pages_show_list"}}
	return m.Dialog + "?" + q.Encode()
}

// proof adds appsecret_proof next to an access token, so a leaked token is useless without the app secret
// (turn on "Require App Secret" in the Meta app's advanced settings).
func (m *Meta) proof(q url.Values) {
	if tok := q.Get("access_token"); tok != "" && q.Get("appsecret_proof") == "" && m.AppSecret != "" {
		h := hmac.New(sha256.New, []byte(m.AppSecret))
		h.Write([]byte(tok))
		q.Set("appsecret_proof", hex.EncodeToString(h.Sum(nil)))
	}
}

func (m *Meta) get(ctx context.Context, path string, q url.Values, out any) error {
	u := path
	if strings.HasPrefix(path, "https://") { // a paging link Meta returned
		if pu, err := url.Parse(path); err == nil {
			pq := pu.Query()
			m.proof(pq)
			pu.RawQuery = pq.Encode()
			u = pu.String()
		}
	} else {
		u = m.Graph + "/" + strings.TrimPrefix(path, "/")
		if q != nil {
			m.proof(q)
			u += "?" + q.Encode()
		}
	}
	return m.h.do(ctx, "Meta", "GET", u, nil, nil, out)
}

func (m *Meta) post(ctx context.Context, t Token, path string, form url.Values, out any) error {
	form.Set("access_token", t.Access)
	m.proof(form)
	return m.h.do(ctx, "Meta", "POST", m.Graph+"/"+strings.TrimPrefix(path, "/"), nil, form, out)
}

func (m *Meta) Exchange(ctx context.Context, code, redirect string) (Token, []Account, error) {
	var short struct {
		AccessToken string `json:"access_token"`
	}
	err := m.get(ctx, "oauth/access_token", url.Values{"client_id": {m.AppID}, "client_secret": {m.AppSecret},
		"redirect_uri": {redirect}, "code": {code}}, &short)
	if err != nil {
		return Token{}, nil, err
	}
	// Swap for a long-lived (about 60 day) token.
	var long struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   int64  `json:"expires_in"`
	}
	err = m.get(ctx, "oauth/access_token", url.Values{"grant_type": {"fb_exchange_token"}, "client_id": {m.AppID},
		"client_secret": {m.AppSecret}, "fb_exchange_token": {short.AccessToken}}, &long)
	if err != nil {
		return Token{}, nil, err
	}
	t := Token{Access: long.AccessToken}
	if long.ExpiresIn > 0 {
		t.Expires = time.Now().Add(time.Duration(long.ExpiresIn) * time.Second)
	}
	var accts struct {
		Data []struct {
			ID       string `json:"id"` // act_123
			Name     string `json:"name"`
			Currency string `json:"currency"`
		} `json:"data"`
	}
	if err := m.get(ctx, "me/adaccounts", url.Values{"fields": {"name,currency"}, "limit": {"100"}, "access_token": {t.Access}}, &accts); err != nil {
		return Token{}, nil, err
	}
	out := make([]Account, 0, len(accts.Data))
	for _, a := range accts.Data {
		out = append(out, Account{ExternalID: a.ID, Name: a.Name, Currency: a.Currency})
	}
	return t, out, nil
}

// Fresh: Meta long-lived tokens cannot be refreshed server-side; the owner reconnects when one expires.
func (m *Meta) Fresh(_ context.Context, t Token) (Token, error) {
	if !t.Expires.IsZero() && time.Now().After(t.Expires) {
		return t, ErrExpired
	}
	return t, nil
}

// ErrExpired means the owner must reconnect the account.
var ErrExpired = errors.New("access expired, connect the account again")

func (m *Meta) Insights(ctx context.Context, t Token, a Account, since, until string) ([]DayInsight, error) {
	tr, _ := json.Marshal(map[string]string{"since": since, "until": until})
	q := url.Values{"level": {"campaign"}, "fields": {"campaign_id,spend,impressions,clicks,actions"}, "time_increment": {"1"},
		"time_range": {string(tr)}, "limit": {"500"}, "access_token": {t.Access}}
	var out []DayInsight
	next := ""
	for page := 0; page < 50; page++ {
		var res struct {
			Data []struct {
				CampaignID  string `json:"campaign_id"`
				DateStart   string `json:"date_start"`
				Spend       string `json:"spend"`
				Impressions string `json:"impressions"`
				Clicks      string `json:"clicks"`
				Actions     []struct {
					Type  string `json:"action_type"`
					Value string `json:"value"`
				} `json:"actions"`
			} `json:"data"`
			Paging struct {
				Next string `json:"next"`
			} `json:"paging"`
		}
		var err error
		if next == "" {
			err = m.get(ctx, a.ExternalID+"/insights", q, &res)
		} else {
			err = m.get(ctx, next, nil, &res)
		}
		if err != nil {
			return nil, err
		}
		for _, d := range res.Data {
			di := DayInsight{CampaignID: d.CampaignID, Day: d.DateStart, SpendPaise: ToPaise(parseF(d.Spend), a.Currency),
				Impressions: parseI(d.Impressions), Clicks: parseI(d.Clicks)}
			for _, ac := range d.Actions {
				if ac.Type == "mobile_app_install" || ac.Type == "app_install" {
					di.Installs += parseI(ac.Value)
				}
			}
			out = append(out, di)
		}
		if res.Paging.Next == "" {
			break
		}
		next = res.Paging.Next
	}
	return out, nil
}

// minor converts paise to the account currency's minor unit (paise for INR, cents for USD).
func minor(paise int64, cur string) string {
	return strconv.FormatInt(int64(FromPaise(paise, cur)*100+0.5), 10)
}

func (m *Meta) Create(ctx context.Context, t Token, a Account, s Spec) (Ref, error) {
	var ref Ref
	appID := m.Promoted[s.Product]
	if appID == "" {
		return ref, fmt.Errorf("Meta: register %s in Meta (Events Manager > Apps) and add it to META_PROMOTED_APPS", s.Product)
	}
	if m.PageID == "" {
		return ref, errors.New("Meta: set META_PAGE_ID to the Facebook Page the ads run as")
	}
	status := "PAUSED"
	if s.Launch {
		status = "ACTIVE"
	}
	var id struct {
		ID string `json:"id"`
	}
	err := m.post(ctx, t, a.ExternalID+"/campaigns", url.Values{"name": {s.Name}, "objective": {"OUTCOME_APP_PROMOTION"},
		"status": {status}, "special_ad_categories": {"[]"}, "buying_type": {"AUCTION"},
		"is_adset_budget_sharing_enabled": {"false"}}, &id)
	if err != nil {
		return ref, err
	}
	ref.CampaignID = id.ID

	promoted, _ := json.Marshal(map[string]string{"application_id": appID, "object_store_url": s.StoreURL()})
	targeting, _ := json.Marshal(map[string]any{"geo_locations": map[string]any{"countries": s.Countries},
		"user_os": []string{"Android"}, "publisher_platforms": []string{"facebook", "instagram"}})
	set := url.Values{"name": {s.Name + " · ad set"}, "campaign_id": {ref.CampaignID}, "daily_budget": {minor(s.DailyBudgetPaise, a.Currency)},
		"billing_event": {"IMPRESSIONS"}, "optimization_goal": {"APP_INSTALLS"}, "bid_strategy": {"LOWEST_COST_WITHOUT_CAP"},
		"promoted_object": {string(promoted)}, "targeting": {string(targeting)}, "status": {status}}
	if end := endTime(s); !end.IsZero() {
		set.Set("end_time", end.UTC().Format(time.RFC3339))
	}
	if err := m.post(ctx, t, a.ExternalID+"/adsets", set, &id); err != nil {
		return ref, err
	}
	ref.SetID = id.ID

	cta := map[string]any{"type": "INSTALL_MOBILE_APP", "value": map[string]string{"link": s.StoreURL()}}
	var story map[string]any
	if s.Creative.Kind == "video" {
		if err := m.post(ctx, t, a.ExternalID+"/advideos", url.Values{"file_url": {s.Creative.URL}}, &id); err != nil {
			return ref, err
		}
		vd := map[string]any{"video_id": id.ID, "message": s.Creative.Text, "title": s.Creative.Headline, "call_to_action": cta}
		if s.Creative.ThumbnailURL != "" {
			vd["image_url"] = s.Creative.ThumbnailURL
		}
		story = map[string]any{"page_id": m.PageID, "video_data": vd}
	} else {
		story = map[string]any{"page_id": m.PageID, "link_data": map[string]any{"link": s.StoreURL(), "picture": s.Creative.URL,
			"message": s.Creative.Text, "name": s.Creative.Headline, "call_to_action": cta}}
	}
	storyJSON, _ := json.Marshal(story)
	if err := m.post(ctx, t, a.ExternalID+"/adcreatives", url.Values{"name": {s.Name + " · creative"}, "object_story_spec": {string(storyJSON)}}, &id); err != nil {
		return ref, err
	}
	creative, _ := json.Marshal(map[string]string{"creative_id": id.ID})
	err = m.post(ctx, t, a.ExternalID+"/ads", url.Values{"name": {s.Name + " · ad"}, "adset_id": {ref.SetID},
		"creative": {string(creative)}, "status": {status}}, &id)
	return ref, err
}

func (m *Meta) SetActive(ctx context.Context, t Token, _ Account, r Ref, active bool) error {
	status := "PAUSED"
	if active {
		status = "ACTIVE"
	}
	return m.post(ctx, t, r.CampaignID, url.Values{"status": {status}}, nil)
}

func (m *Meta) SetDailyBudget(ctx context.Context, t Token, a Account, r Ref, paise int64) error {
	if r.SetID == "" {
		return errors.New("Meta: this campaign has no ad set to change")
	}
	return m.post(ctx, t, r.SetID, url.Values{"daily_budget": {minor(paise, a.Currency)}}, nil)
}
