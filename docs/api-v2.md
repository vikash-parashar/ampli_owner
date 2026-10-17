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

---

# v2.1 additions (2026-10-17, after review)

## Alerts
`GET /v1/alerts` → list, most severe first:
```json
[{"id": "vitals-crash-mp3", "severity": "high|medium|low", "product": "mp3", "title": "Crash rate 1.3% (Play limit 1.09%)",
  "detail": "Play shows the app less above this. Check the latest crashes.", "action": "open_ads|open_reviews|open_inbox|open_settings|"}]
```
Covers: vitals near or over Play's bad-behaviour limits, new unreplied 1-2★ reviews, rating falling week on week,
live campaigns with a Stop verdict, campaigns in error, expired ad accounts or AdMob, tickets waiting on the owner.

## Ads changes
- `GET /v1/ads/platforms`: each platform also has `"min_daily_budget_paise"` (meta 10000, tiktok 170000, google 100000).
- Campaign JSON adds `"auto_stop": {"enabled": false, "min_roas": 0.5, "min_days": 7, "min_spend_paise": 200000}` and
  `"attribution": "link|estimate|none"` (estimate = installs matched by platform and time, not by our link).
- `POST /v1/ads/campaigns/{id}/autostop` with the auto_stop object → campaign. When enabled, every sync pauses the
  campaign on the platform once ROAS < min_roas after min_days days of spend and at least min_spend_paise spent.
- `GET /v1/ads/campaigns/{id}/log` → `[{"id","campaign_id","actor":"owner|rule|sync","action","detail","created_at"}]`.
- `DELETE /v1/ads/campaigns/{id}` → 204 (pauses it on the platform first).
- `POST /v1/ads/sync?force=1` → `{"synced": 1, "errors": ["meta Ampli Studio: ..."]}`; show the first error when present.
- Money everywhere is **net** (after Google's 15% fee and 18% GST) from **verified** purchases only.

## Integrations (AdMob, Play)
- `GET /v1/integrations` → `{"admob": {"configured": true, "connected": false, "account": "", "last_sync_at": null, "last_error": ""},
  "play": {"configured": true, "last_sync_at": null, "last_error": ""}}`
- `POST /v1/integrations/admob/connect` → `{"auth_url": "..."}` (open in the browser, like ad accounts).
- `POST /v1/integrations/sync` → `{"admob_days": 30, "vitals": 3, "reviews": 12, "play_days": 0, "errors": []}`
  (AdMob earnings → ad revenue, Play vitals, Play reviews, Play installs/ratings reports).

## Dashboard
Adds `"vitals": {"day": "2026-10-15", "crash_rate": 0.0042, "anr_rate": 0.0011}` (null when unknown; Play's limits are
0.0109 and 0.0047) and `"active_users"` is the latest day's count (label it "Active users", not 30 days).

## Public purchases (v1.1: verified with Google Play)
`POST /v1/public/purchases`
```json
{"product": "mp3", "install_id": "uuid v4", "sku": "ampli_pro", "purchase_token": "from Play Billing",
 "kind": "pro|tip|subscription|other"}
```
The server checks the token with Google Play, takes the price from its own SKU table, and stores net rupees.
`install_id` must be a UUID v4 on every public endpoint. Unknown event names are rejected.
