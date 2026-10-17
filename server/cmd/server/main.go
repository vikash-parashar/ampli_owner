// Command server runs the Ampli Owner API: as a plain HTTP server locally or in Docker,
// and as an AWS Lambda (Function URL or API Gateway) when AWS_LAMBDA_FUNCTION_NAME is set.
//
//	DATABASE_URL  Postgres connection string (required)
//	OWNER_TOKEN   bearer token for the owner endpoints (required)
//	PORT          listen port when not on Lambda (default 8080)
//
// Ads manager and Play reviews (all optional; see README "Connect ad accounts"):
//
//	TOKEN_KEY, PUBLIC_URL
//	META_APP_ID, META_APP_SECRET, META_PAGE_ID, META_PROMOTED_APPS, META_GRAPH_VERSION
//	TIKTOK_APP_ID, TIKTOK_APP_SECRET, TIKTOK_PROMOTED_APPS
//	GOOGLE_ADS_CLIENT_ID, GOOGLE_ADS_CLIENT_SECRET, GOOGLE_ADS_DEVELOPER_TOKEN, GOOGLE_ADS_LOGIN_CUSTOMER_ID, GOOGLE_ADS_API_VERSION
//	PLAY_SERVICE_ACCOUNT_JSON, PLAY_GCS_BUCKET, FX_RATES
//	ADMOB_APPS ("mp3=ca-app-pub-...~123,naukri=...", uses the Google Ads OAuth client)
//	OWNER_TOKEN_NEXT  a second accepted owner token while rotating OWNER_TOKEN
//	SSM_PREFIX        read secrets from SSM SecureString parameters <prefix><NAME> (see ssm.go)
//
// `server seed <dir>` loads checklist.json, guides.json and features.json from dir and exits.
// `server sync` pulls ad results, AdMob and Google Play once and exits (for a cron or scheduled task).
// On Lambda, an EventBridge Scheduler event with the payload {"task":"sync"} does the same.
package main

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/akrylysov/algnhsa"
	"github.com/aws/aws-lambda-go/lambda"

	"github.com/vikash-parashar/ampli_owner/server/internal/admob"
	"github.com/vikash-parashar/ampli_owner/server/internal/ads"
	"github.com/vikash-parashar/ampli_owner/server/internal/api"
	"github.com/vikash-parashar/ampli_owner/server/internal/play"
	"github.com/vikash-parashar/ampli_owner/server/internal/secret"
	"github.com/vikash-parashar/ampli_owner/server/internal/store"
)

func main() {
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, nil)))
	if err := run(); err != nil {
		slog.Error("fatal", "err", err)
		os.Exit(1)
	}
}

func run() error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := loadSSM(ctx); err != nil {
		return err
	}
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		return errMissing("DATABASE_URL")
	}
	onLambda := os.Getenv("AWS_LAMBDA_FUNCTION_NAME") != ""
	if onLambda && !strings.Contains(dbURL, "pool_max_conns") {
		// Each Lambda instance serves one request at a time; keep Postgres connections low.
		dbURL += map[bool]string{true: "&", false: "?"}[strings.Contains(dbURL, "?")] + "pool_max_conns=2"
	}
	st, err := store.Open(ctx, dbURL)
	if err != nil {
		return err
	}
	defer st.Close()
	if err := st.Migrate(ctx); err != nil {
		return err
	}

	if len(os.Args) == 3 && os.Args[1] == "seed" {
		return seed(ctx, st, os.Args[2])
	}

	srv, err := newServer(st)
	if err != nil {
		return err
	}
	if len(os.Args) == 2 && os.Args[1] == "sync" {
		syncAll(ctx, srv)
		return nil
	}
	if len(srv.OwnerToken) < 24 || (srv.OwnerTokenNext != "" && len(srv.OwnerTokenNext) < 24) {
		return errMissing("OWNER_TOKEN (24+ characters; OWNER_TOKEN_NEXT too when set)")
	}
	h := srv.Handler()

	if onLambda {
		lambda.StartWithOptions(scheduled{http: algnhsa.New(h, nil), srv: srv})
		return nil
	}

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	hs := &http.Server{Addr: ":" + port, Handler: h, ReadHeaderTimeout: 10 * time.Second}
	go func() {
		<-ctx.Done()
		shutdown, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = hs.Shutdown(shutdown)
	}()
	slog.Info("listening", "port", port)
	if err := hs.ListenAndServe(); err != http.ErrServerClosed {
		return err
	}
	return nil
}

func syncAll(ctx context.Context, srv *api.Server) map[string]any {
	n, problems := srv.SyncAds(ctx, true)
	res := srv.SyncIntegrations(ctx)
	out := map[string]any{"ad_accounts": n, "admob_days": res.AdMobDays, "vitals": res.Vitals, "reviews": res.Reviews,
		"play_days": res.PlayDays, "refunds": res.Refunds, "errors": append(problems, res.Errors...)}
	slog.Info("synced", "result", out)
	return out
}

// scheduled routes {"task":"sync"} events to the sync and everything else (Function URL requests) to HTTP.
type scheduled struct {
	http lambda.Handler
	srv  *api.Server
}

func (h scheduled) Invoke(ctx context.Context, payload []byte) ([]byte, error) {
	var ev struct {
		Task string `json:"task"`
	}
	if json.Unmarshal(payload, &ev) == nil && ev.Task == "sync" {
		return json.Marshal(syncAll(ctx, h.srv))
	}
	return h.http.Invoke(ctx, payload)
}

// newServer reads the optional ads and Play settings from the environment.
func newServer(st *store.Store) (*api.Server, error) {
	env := os.Getenv
	s := &api.Server{Store: st, OwnerToken: env("OWNER_TOKEN"), OwnerTokenNext: env("OWNER_TOKEN_NEXT"), PublicURL: env("PUBLIC_URL"),
		PlayBucket: env("PLAY_GCS_BUCKET"), AdMob: admob.New(env("GOOGLE_ADS_CLIENT_ID"), env("GOOGLE_ADS_CLIENT_SECRET"), env("ADMOB_APPS")), Platforms: map[string]ads.Platform{
			"meta":   ads.NewMeta(env("META_APP_ID"), env("META_APP_SECRET"), env("META_PAGE_ID"), env("META_PROMOTED_APPS"), env("META_GRAPH_VERSION")),
			"tiktok": ads.NewTikTok(env("TIKTOK_APP_ID"), env("TIKTOK_APP_SECRET"), env("TIKTOK_PROMOTED_APPS")),
			"google": ads.NewGoogle(env("GOOGLE_ADS_CLIENT_ID"), env("GOOGLE_ADS_CLIENT_SECRET"), env("GOOGLE_ADS_DEVELOPER_TOKEN"),
				env("GOOGLE_ADS_LOGIN_CUSTOMER_ID"), env("GOOGLE_ADS_API_VERSION")),
		}}
	if k := env("TOKEN_KEY"); k != "" {
		box, err := secret.New(k)
		if err != nil {
			return nil, err
		}
		s.Box = box
	}
	if j := env("PLAY_SERVICE_ACCOUNT_JSON"); j != "" {
		pc, err := play.New(j)
		if err != nil {
			return nil, err
		}
		s.Play = pc
	}
	return s, nil
}

// seed imports the checklist and guides (upsert, so ticks are kept in sync with the file) and
// the built-vs-left feature list (only into an empty table, so later edits are not overwritten).
func seed(ctx context.Context, st *store.Store, dir string) error {
	var items []store.ChecklistItem
	if err := readJSON(filepath.Join(dir, "checklist.json"), &items); err != nil {
		return err
	}
	if err := st.UpsertChecklist(ctx, items, true); err != nil {
		return err
	}
	var guides []store.Guide
	if err := readJSON(filepath.Join(dir, "guides.json"), &guides); err != nil {
		return err
	}
	for _, g := range guides {
		if err := st.SaveGuide(ctx, g); err != nil {
			return err
		}
	}
	var features []store.Feature
	if err := readJSON(filepath.Join(dir, "features.json"), &features); err != nil {
		return err
	}
	added, err := st.SeedFeatures(ctx, features)
	if err != nil {
		return err
	}
	slog.Info("seeded", "checklist", len(items), "guides", len(guides), "features", added)
	return nil
}

func readJSON(path string, v any) error {
	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return json.Unmarshal(b, v)
}

type errMissing string

func (e errMissing) Error() string { return "missing env " + string(e) }
