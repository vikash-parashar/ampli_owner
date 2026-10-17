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
- `POST /v1/ads/campaigns/{id}/autostop` with the auto_stop object → campaign. When enabled, a sync that finds ROAS < min_roas
  after min_days days and min_spend_paise spent sets `auto_stop.pending_since` (alert "will pause"); a sync 24 h or more later
  that still finds it below pauses it on the platform. Never runs without the Play service account or when attribution is
  "none". A manual resume sets `auto_stop.hold_until` (72 h) during which the rule does nothing.
- `GET /v1/ads/campaigns/{id}/log` → `[{"id","campaign_id","actor":"owner|rule|sync","action","detail","created_at"}]`.
- `DELETE /v1/ads/campaigns/{id}` → 204 (pauses it on the platform first).
- `POST /v1/ads/sync?force=1` → `{"synced": 1, "errors": ["meta Ampli Studio: ..."]}`; show the first error when present.
- Money everywhere is **net** (after Google's 15% fee and 18% GST) from **verified** purchases only.

## Integrations (AdMob, Play)
- `GET /v1/integrations` → `{"admob": {"configured": true, "connected": false, "account": "", "last_sync_at": null, "last_error": ""},
  "play": {"configured": true, "last_sync_at": null, "last_error": ""}}`
- `POST /v1/integrations/admob/connect` → `{"auth_url": "..."}` (open in the browser, like ad accounts).
- `POST /v1/integrations/sync` → `{"admob_days": 30, "vitals": 3, "reviews": 12, "play_days": 0, "errors": []}`
  (AdMob earnings → ad revenue, Play vitals, Play reviews, Play installs/ratings reports). Also returns `"refunds"`.
- `DELETE /v1/integrations/admob` → 204 (forget the AdMob login).

## SKUs and data deletion
- `GET /v1/skus` → `[{"product","sku","kind","price_paise"}]`; `PUT /v1/skus` with one of those → it (add or change a price).
- `DELETE /v1/installs/{install_id}` → `{"erased_rows": 3}`.

## Audit and safety
- `GET /v1/audit` → `[{"id","action","detail","created_at"}]` (erase, SKU prices, AdMob connect/disconnect).
- 20 wrong owner tokens from one network in a day → 429 for the rest of the day.
- Deleted campaigns are hidden, not erased; their numbers and log stay. Verdict code `unknown` ("Can't judge yet")
  when no installs are matched to the campaign.

## Owner token rotation
The server accepts `OWNER_TOKEN` and, when set, `OWNER_TOKEN_NEXT`.

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
Answers (v1.2): 200 `{"recorded","verified"}`; 400 bad body or unknown sku and 422 Google refused or cancelled (drop);
409 payment pending, 429 limit, 503 Google down or Play not connected (saved unverified, retry; the retry upgrades it).
`product_id` is accepted instead of `sku`.
`install_id` must be a UUID v4 on every public endpoint. Unknown event names are rejected.

## v2.2 growth: tracked links, install channels, cross-promo codes

**Tracked links** (owner): `GET /v1/links?product=`, `POST /v1/links {product, channel, label?, listing?, from_product?}`,
`DELETE /v1/links/{id}` (hides it; past installs keep their channel). Channels: `play_hindi` (adds `listing=hindi` and
`hl=hi`), `website`, `whatsapp`, `telegram`, `instagram` (unpaid, never counted as Meta ads), `youtube`, `crosspromo`
(`from_product` = the app the link sits in), `other`. Each link answers its Play `url` with the install referrer
`utm_source=<channel>&utm_medium=…&utm_campaign=ampli-l<id>` filled in, plus `installs`, `installs_30d`, `pro_sales`
and `net_paise` from verified purchases of those installs.

**Dashboard**: every product dashboard ends with a `channels` bars widget, "Where installs come from (30 days)":
tracked-link channels, apps' own "More from Ampli" links (`utm_source=crosspromo`), ad platforms, our ad links,
Play search and browse, and other.

**Cross-promo codes**: `POST /v1/promo/codes {source_product, target_product, sku, codes[]}` imports Play promo codes
(duplicates skipped, audited); `GET /v1/promo` reports per pair `imported`, `available`, `eligible` (verified Pro buyers
of the source app), `claimed` and `redeemed` (promo purchases of the sku in the target app since the first claim).
Public `POST /v1/public/promo/claim {product, install_id}` hands one code per verified Pro buyer (403 otherwise, 410 when
none are left). Purchases Play marks `purchaseType = 1` are stored with region `PROMO`: counted, never revenue.
An alert fires when 5 or fewer codes are left and buyers are still waiting. Hook spec v1.3 has the app side.

## Review pass 3 (2026-10-17)
- Purchases: Google 400/410 → **422** (drop). Google 401/403/404 (our service account or package is wrong) → **503**,
  saved unverified, `Retry-After: 3600`, plus a high alert `purchases-unchecked` once any is 6 h old.
- `POST /v1/integrations/sync` answers `exact_net`: purchases whose net came from the monthly earnings report
  (`purchases.net_exact`).
- OAuth states carry a random nonce and are single use (`oauth_states_used`); a replayed callback gets "Link expired".
- Meta calls send `appsecret_proof`. HTTP server read/write/idle timeouts. Go 1.27.2, modules updated.

- Cross-promo claim answers **409** `{error, available_at}` until 48 h after the Pro purchase (Play's self-refund
  window); `GET /v1/promo` adds `refunded_claims`.
- Campaigns add `spend_7d_paise`, `revenue_7d_paise`, `roas_7d`, `judged_on` and `judge_roas`. With 14+ days of spend
  the verdict, stop alerts and auto-stop use the last 7 days (`judged_on = last_7_days`), otherwise lifetime totals.

## Review pass 4 (2026-10-17)
- Earnings: rows are typed (Charge, Charge refund, Google fee, Tax). For Indian buyers with no Tax row the 18% GST
  share is taken out of the gross (to be confirmed on one real `earnings_YYYYMM` file). The last two months are merged
  before writing, and only orders whose original Charge is in them are updated, so a partial refund next month nets out.
- Unconfirmed purchases keep their Play token sealed with `TOKEN_KEY` (`purchases.token_enc`) and the reason
  (`verify_error`). The daily sync re-checks up to 200 of them (`rechecked` in the sync answer) and wipes the token once
  Google answers; Cleanup wipes tokens older than 30 days. Refused tokens become region `REFUSED`. The alert names the
  reason (service account, or "sku X not found") and the 30-day limit.
