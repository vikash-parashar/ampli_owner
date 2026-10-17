# Ampli Owner API v2: dashboards, ads manager, reviews, attribution

Money is always in paise (integer). Percentages are floats 0-100. Dates are ISO-8601.
Owner endpoints need `Authorization: Bearer <OWNER_TOKEN>`. Android sends PATCH as POST with
`X-HTTP-Method-Override: PATCH`. Product ids: `mp3`, `carhub`, `naukri` (`all` where noted).

## Dashboard

`GET /v1/dashboard?product=all|mp3|carhub|naukri`

```json
{
  "product": "mp3",
  "business_paise": 18430000,          // all-time revenue: pro + ads + tips + other
  "business_change_pct": 22.0,         // last 30 days vs the 30 before
  "downloads": 12480,                  // all-time Play installs
  "downloads_week": 1120,              // last 7 days
  "rating": 4.6,                       // null when unknown
  "reviews": 842,
  "active_users": 5310,                // latest day with data
  "active_change_pct": 12.0,
  "pro_sales": 612,
  "pro_revenue_paise": 0, "ad_revenue_paise": 0, "tips_paise": 0, "other_revenue_paise": 0,
  "revenue_30d": [0, 0, "... 30 values, oldest first"],
  "weekly_downloads": [{"organic": 310, "ads": 220}, "... 4 weeks, oldest first"],
  "stars": [612, 131, 46, 21, 32],     // counts for 5,4,3,2,1 stars
  "sentiment": {"positive": 81.0, "mixed": 11.0, "negative": 8.0},
  "ads": {"spend_paise": 0, "revenue_paise": 0, "installs": 0, "roas": 0, "cpi_paise": 0, "profit_paise": 0},
  "widgets": [
    {"id": "funnel", "title": "Free to Pro funnel", "kind": "funnel|bars|split",
     "rows": [{"label": "Installed", "value": 12480, "pct": 100.0}]}
  ],
  "products": [ {"id": "mp3", "name": "Ampli MP3 Player", "business_paise": 0, "downloads": 0,
                 "rating": 4.6, "change_pct": 22.0} ]   // only for product=all
}
```

Widgets per product (built from `app_events` counters the apps send):
- mp3: `funnel` (installed, played_10, pro_feature_opened, pro_trial, pro bought), `features` bars (feature_eq, feature_record_player, feature_sleep_timer, feature_ringtone, feature_wifi_transfer)
- carhub: `voice` split (voice_bunty vs voice_simran), `commands` bars (cmd_navigate, cmd_music, cmd_call, cmd_fastag, cmd_sos)
- naukri: `alerts` bars (alert_sent, marked_applied), `states` bars (state_hr, state_up, state_rj, state_dl, state_other)
- all: `split` money by product

## Ads manager

`GET /v1/ads/platforms`
```json
[{"id": "meta", "name": "Meta", "sub": "Facebook + Instagram", "configured": true,
  "accounts": [{"id": 1, "platform": "meta", "external_id": "act_123", "name": "Ampli Studio",
                "currency": "INR", "status": "connected", "last_sync_at": "...", "last_error": ""}]}]
```
`configured` is false until the server has that platform's developer app keys.

`POST /v1/ads/connect/{platform}` → `{"auth_url": "https://..."}`. The app opens it in the browser;
the platform redirects to the server (`/oauth/{platform}/callback`), which stores the token
encrypted and shows "Connected, go back to Ampli Owner".

`DELETE /v1/ads/accounts/{id}` disconnects.

`GET /v1/ads/campaigns?product=all|mp3|...`
```json
{"summary": {"spend_paise": 0, "revenue_paise": 0, "installs": 0, "roas": 0, "cpi_paise": 0, "profit_paise": 0},
 "campaigns": [{"id": 1, "platform": "meta", "product": "mp3", "name": "...", "status": "live",
   "countries": ["IN"], "daily_budget_paise": 100000, "spend_paise": 0, "impressions": 0, "clicks": 0,
   "installs": 0, "revenue_paise": 0, "roas": 0, "cpi_paise": 0, "days": 12,
   "verdict": {"code": "scale|watch|stop|early|new", "label": "Scale up", "reason": "..."},
   "ad_account_id": 1, "utm_campaign": "ampli-c1", "last_error": ""}]}
```
status: draft, in_review, live, paused, ended, error.
Verdict: under 5 days of data = early; ROAS ≥ 1.5 = scale; 0.8-1.5 = watch; < 0.8 = stop.

`POST /v1/ads/campaigns`
```json
{"product": "mp3", "ad_account_id": 1, "name": "MP3 Reels USA", "countries": ["US"],
 "daily_budget_paise": 100000, "days": 7,
 "creative": {"kind": "video|image", "url": "https://...", "thumbnail_url": "", "headline": "", "text": ""},
 "launch": true}
```
→ the campaign. `launch: false` creates it paused.

`POST /v1/ads/campaigns/{id}/pause`, `/resume`, `/budget` (`{"daily_budget_paise": 120000}`) → the campaign.
`POST /v1/ads/sync` → `{"synced": 3}` (pulls spend, clicks, installs from every connected account).

## Reviews

`GET /v1/reviews?product=all|mp3|...`
```json
{"summary": {"total": 842, "positive": 81.0, "mixed": 11.0, "negative": 8.0, "stars": [612,131,46,21,32]},
 "themes": {"praise": [{"label": "Record player", "count": 212}], "complaints": [{"label": "Too many ads", "count": 64}]},
 "reviews": [{"id": 1, "product": "mp3", "author": "Rohit S.", "rating": 5, "body": "...",
              "sentiment": "positive", "reply": "", "source": "play", "created_at": "..."}]}
```
`POST /v1/reviews` (manual add: product, author, rating, body), `POST /v1/reviews/{id}/reply` (`{"text": "..."}`,
also posts to Google Play when the Play service account is set), `POST /v1/reviews/sync`.

## Public (called by the product apps, no token)

- `POST /v1/public/installs` `{"product","install_id","referrer","app_version","country"}` once on first run.
  `referrer` is the raw Play Install Referrer string (`utm_source=meta&utm_campaign=ampli-c1...`).
- `POST /v1/public/purchases` `{"product","install_id","kind":"pro|tip|subscription|other","sku","amount_paise","currency","order_id"}`
  after every completed purchase. `order_id` is unique, so retries are safe.
- `POST /v1/public/events` `{"product","install_id","events":["feature_eq","cmd_navigate"]}` at most once per
  app start, counters only, no personal data. Names match `^[a-z0-9_]{1,40}$`, up to 20 per call.
