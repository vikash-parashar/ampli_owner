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
4. Seed once from your laptop: `DATABASE_URL=... go run ./cmd/server seed seed`.
5. Put the Function URL and token in the app's Settings.

Secrets live in Lambda env / AWS Secrets Manager and GitHub secrets, never in the repo.

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

**Google Play reviews.** Google Cloud service account with a JSON key; in Play Console > Users and permissions,
invite its email with "View app information" and "Reply to reviews". Env: `PLAY_SERVICE_ACCOUNT_JSON` (the file
contents). Play only returns the last week of reviews, so sync at least weekly.

**Sync.** The app syncs when you open Ads (at most every 10 minutes per account). For a schedule, run
`server sync` daily (EventBridge Scheduler running the same image as a task, or any cron).

### How results are attributed
Each product app sends `POST /v1/public/installs` once with the Play Install Referrer, and
`POST /v1/public/purchases` after each purchase. An install goes to a campaign when its `utm_campaign` is that
campaign's `ampli-cN` link, otherwise to the newest campaign on the same platform for the same app. Earned money =
purchases by attributed installs. Verdict: under 5 days of spend = too early; ROAS 1.5x or more = scale;
0.8x to 1.5x = watch; under 0.8x = stop. Spend, clicks and platform-counted installs come from the sync.

## API
Full shapes: [docs/api-v2.md](docs/api-v2.md).
Owner (bearer token): `/v1/overview`, `/v1/dashboard`, `/v1/tickets`, `/v1/tickets/{id}/messages`, `/v1/checklist`,
`/v1/todos`, `/v1/features`, `/v1/stats`, `/v1/guides`, `/v1/ads/platforms`, `/v1/ads/connect/{platform}`,
`/v1/ads/accounts/{id}`, `/v1/ads/campaigns`, `/v1/ads/campaigns/{id}/pause|resume|budget`, `/v1/ads/sync`,
`/v1/reviews`, `/v1/reviews/{id}/reply`, `/v1/reviews/sync`.
Public (product apps): `POST /v1/public/reports`, `GET /v1/public/reports?install_id=`, `GET /v1/public/features?product=`,
`POST /v1/public/features/{id}/vote` (5 reports per install per day), `POST /v1/public/installs`,
`POST /v1/public/purchases`, `POST /v1/public/events` (30 calls per install per day).

## Later
Play installs and AdMob earnings still go in by hand (Dashboard > Add day's numbers) until Play Console and AdMob
report exports are wired in.
