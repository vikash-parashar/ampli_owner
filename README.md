# Ampli Owner

The owner's console for Ampli Tech: one place for sales, downloads, revenue, user feedback,
support tickets, bugs, feature requests, ad campaigns, the launch checklist, to-dos and the roadmap
for every Ampli product (Ampli MP3 Player, Ampli Car Hub).

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

## API
Owner (bearer token): `/v1/overview`, `/v1/tickets`, `/v1/tickets/{id}/messages`, `/v1/checklist`, `/v1/todos`,
`/v1/features`, `/v1/campaigns`, `/v1/stats`, `/v1/guides`.
Public (product apps): `POST /v1/public/reports`, `GET /v1/public/reports?install_id=`, `GET /v1/public/features?product=`,
`POST /v1/public/features/{id}/vote`. 5 reports per install per day.

## Later
A daily Lambda pulls Play Console (installs, ratings, reviews, sales) and AdMob earnings once those accounts exist,
writing to `/v1/stats` and turning Play reviews into tickets.
