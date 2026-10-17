# Ampli Owner (ampli_owner)

The owner's console for Ampli Tech: one place for sales, downloads, revenue, user feedback,
support tickets, bugs, feature requests, ad campaigns, the launch checklist, to-dos and the roadmap
for every Ampli product (Ampli MP3 Player, Ampli Car Hub, Ampli Naukri), plus an ads manager that runs
Meta (Facebook + Instagram), TikTok and Google Ads (YouTube, Play, Search) campaigns from the app.

| Folder | What |
| --- | --- |
| `android/` | Owner app, Kotlin + Jetpack Compose. Works on the phone alone, or against the server. |
| `server/`  | Go API + Postgres. Runs in Docker locally and on AWS Lambda (Function URL). |

## Install the app
Every push to `android/` builds `AmpliOwner-N.apk` and publishes it as a GitHub release (`build-N`).
Open the release on the phone and install. Builds share one signing key, so each installs over the last.

Without a server the app keeps everything on the phone (checklist, to-dos, tickets you log, stats you type).
Add the server address and owner token in Settings to switch to the server.

## Run the server locally
```sh
cd server
docker compose up --build                 # API on :8080, Postgres on :5432
docker compose run --rm api seed /seed    # load the launch checklist, guides and feature list once
curl -H "Authorization: Bearer local-dev-owner-token-change-me" localhost:8080/v1/overview
```
Tests need a throwaway Postgres: `make test` (uses `TEST_DATABASE_URL`).

## Deploy to AWS Lambda
1. Postgres: Neon free tier or RDS. Keep the URL secret.
2. `make lambda` builds `build/ampli-owner-lambda.zip` (arm64 `bootstrap`; CI uploads it as an artifact too).
3. Create a Lambda: runtime `provided.al2023`, arch arm64, handler `bootstrap`; env `DATABASE_URL`,
   `OWNER_TOKEN` (24+ random characters); enable a Function URL (auth NONE; the app sends the token).
   Set **reserved concurrency to 5**: the public endpoints are open to the internet, and this caps both the
   bill and the Postgres connections (the server uses at most 2 connections per instance on Lambda).
4. Seed once from your laptop: `DATABASE_URL=... go run ./cmd/server seed seed`.
5. Put the Function URL and token in the app's Settings. The app keeps it encrypted with an Android Keystore key
   and asks for your fingerprint before showing money screens.
6. Daily sync: EventBridge Scheduler > target this Lambda > payload `{"task":"sync"}` (once a day, 06:00 IST).
   It pulls ad results (and runs auto-stop rules), AdMob earnings, Play vitals, reviews, refunds and reports.

**Secrets in SSM, not plain Lambda env.** Create SecureString parameters `/ampli-owner/DATABASE_URL`,
`/ampli-owner/OWNER_TOKEN`, `/ampli-owner/TOKEN_KEY`, `/ampli-owner/PLAY_SERVICE_ACCOUNT_JSON`, and the ad secrets
(`META_APP_SECRET`, `TIKTOK_APP_SECRET`, `GOOGLE_ADS_CLIENT_SECRET`, `GOOGLE_ADS_DEVELOPER_TOKEN`; full list in
`cmd/server/ssm.go`). Add the *AWS Parameters and Secrets Lambda Extension* layer, set the env `SSM_PREFIX=/ampli-owner/`,
and give the Lambda role `ssm:GetParameter` on `arn:aws:ssm:ap-south-1:<account>:parameter/ampli-owner/*` plus
`kms:Decrypt` on the key. Only non-secret settings (PUBLIC_URL, ids, ADMOB_APPS, PLAY_GCS_BUCKET) stay in plain env.
Never put secrets in the repo or chat.

**Rotating the owner token.** Set `OWNER_TOKEN_NEXT` to a new value, update the app's Settings, then move the new
value into `OWNER_TOKEN` and clear `OWNER_TOKEN_NEXT`. Both work in between, so nothing breaks.

**Release APK signing.** CI builds the non-debuggable release APK only when these GitHub secrets exist (until then it
builds the test-key debug APK, and the release build never uses the test key): `OWNER_KEYSTORE_BASE64`
(`base64 -w0 owner.jks`), `OWNER_KEYSTORE_PASSWORD`, `OWNER_KEY_ALIAS`, `OWNER_KEY_PASSWORD`. Create the
keystore once with `keytool -genkeypair -v -keystore owner.jks -alias owner -keyalg RSA -keysize 4096 -validity 10000`
and keep a copy offline: losing it means uninstalling to update.

## Connect ad accounts
The app never holds an ad token: you sign in once in the browser, the platform sends the browser back to the
server, and the server stores the token encrypted (AES-GCM) in Postgres. Set on the Lambda:

| Env | What |
| --- | --- |
| `TOKEN_KEY` | `openssl rand -base64 32`. Changing it means connecting every account again. |
| `PUBLIC_URL` | The Function URL, e.g. `https://xyz.lambda-url.ap-south-1.on.aws`. |

**Meta (Facebook + Instagram).** developers.facebook.com > Create app > type Business > add *Marketing API* and
*Facebook Login for Business*. Valid OAuth redirect URI: `PUBLIC_URL/oauth/meta/callback`. Development mode is
enough for your own ad accounts. Register each Android app under Events Manager > Apps (package name) and note
its app id. Env: `META_APP_ID`, `META_APP_SECRET`, `META_PAGE_ID` (the Page ads run as),
`META_PROMOTED_APPS=mp3=<id>,carhub=<id>,naukri=<id>`.

**TikTok.** business-api.tiktok.com > My apps > Create (needs approval, usually a few days). Advertiser redirect
URL: `PUBLIC_URL/oauth/tiktok/callback`, scopes for ads management and reporting. Add each Android app in
TikTok Events Manager. Env: `TIKTOK_APP_ID`, `TIKTOK_APP_SECRET`, `TIKTOK_PROMOTED_APPS=mp3=<id>,...`.

**Google Ads (YouTube, Play, Search).** Create a Google Ads manager account, apply for a developer token in
API Center (Basic access). In Google Cloud create an OAuth client of type *Web application* with redirect
`PUBLIC_URL/oauth/google/callback` and enable the Google Ads API. Env: `GOOGLE_ADS_CLIENT_ID`,
`GOOGLE_ADS_CLIENT_SECRET`, `GOOGLE_ADS_DEVELOPER_TOKEN`, optional `GOOGLE_ADS_LOGIN_CUSTOMER_ID` (manager id).

Use INR ad accounts where possible. Spend in other currencies is converted with `FX_RATES` (default `USD=84,...`).

For Meta, a **System User** token (Business settings > Users > System users, role Admin, assign the ad account
and app) does not expire every 60 days like a personal login. For Google, publish the OAuth consent screen
("In production"); in Testing mode Google refresh tokens die after 7 days.

**Google Play (purchases, reviews, vitals, reports).** Google Cloud project > enable *Google Play Android Developer
API* and *Google Play Developer Reporting API* > service account with a JSON key. In Play Console > Users and
permissions, invite its email with only "View app information and download bulk reports", "View financial data"
and "Reply to reviews" (not "Manage orders": the server never refunds or changes orders). Env: `PLAY_SERVICE_ACCOUNT_JSON` (the file contents) and
`PLAY_GCS_BUCKET` (Play Console > Download reports > copy the Cloud Storage URI, the `pubsite_prod_...` part).
With the service account set, every purchase is checked with Google before it counts as money.

**Prices.** The server prices purchases from its own SKU table (`GET/PUT /v1/skus`), seeded with ampli_pro ₹199,
ampli_drive_pro ₹499 and the ₹49/99/199 tips. Make it match Play Console when you create the products.
Revenue everywhere is net: minus 18% GST, then minus Google's 15%.

**AdMob.** Uses the same Google OAuth client: add `PUBLIC_URL/oauth/admob/callback` as a redirect URI and enable
the AdMob API. Env: `ADMOB_APPS=mp3=ca-app-pub-...~...,naukri=...`. Then Settings > AdMob > Connect in the app.

**Deleting a user's data.** A user who asks gets their install id from their app's Settings; `DELETE
/v1/installs/{id}` removes their reports, votes and install record (purchases keep the money, not the link).

**Sync.** The app syncs when you open Ads (at most every 10 minutes per account). The daily schedule is the
`{"task":"sync"}` event above; outside Lambda run `server sync` from any cron.

### How results are attributed
Each product app sends `POST /v1/public/installs` once with the Play Install Referrer, and
`POST /v1/public/purchases` after each purchase. An install goes to a campaign when its `utm_campaign` is that
campaign's `ampli-cN` link, otherwise to the newest campaign on the same platform for the same app; the app labels
the second kind "estimate". Earned money = verified, unrefunded net purchases by attributed installs. Verdict: under 5 days of spend = too early; ROAS 1.5x or more = scale;
0.8x to 1.5x = watch; under 0.8x = stop. Spend, clicks and platform-counted installs come from the sync.
Auto-stop (per campaign, off by default) pauses a live campaign on the platform once ROAS stays under its limit
after the minimum days and spend; every owner and rule action is in the campaign's log.

## API
Full shapes: [docs/api-v2.md](docs/api-v2.md).
Owner (bearer token): `/v1/overview`, `/v1/dashboard`, `/v1/tickets`, `/v1/tickets/{id}/messages`, `/v1/checklist`,
`/v1/todos`, `/v1/features`, `/v1/stats`, `/v1/guides`, `/v1/ads/platforms`, `/v1/ads/connect/{platform}`,
`/v1/ads/accounts/{id}`, `/v1/ads/campaigns`, `/v1/ads/campaigns/{id}/pause|resume|budget|autostop|log`, `/v1/ads/sync`,
`/v1/reviews`, `/v1/reviews/{id}/reply`, `/v1/reviews/sync`, `/v1/alerts`, `/v1/integrations`,
`/v1/integrations/admob/connect`, `/v1/integrations/sync`, `/v1/skus`, `DELETE /v1/installs/{id}`.
Public (product apps): `POST /v1/public/reports`, `GET /v1/public/reports?install_id=`, `GET /v1/public/features?product=`,
`POST /v1/public/features/{id}/vote` (5 reports per install per day), `POST /v1/public/installs`,
`POST /v1/public/purchases`, `POST /v1/public/events` (30 calls per install and 300 per network per day; install ids
must be random UUID v4; event names come from a fixed list per app).
