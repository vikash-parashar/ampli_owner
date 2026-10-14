// Command server runs the Ampli Owner API: as a plain HTTP server locally or in Docker,
// and as an AWS Lambda (Function URL or API Gateway) when AWS_LAMBDA_FUNCTION_NAME is set.
//
//	DATABASE_URL  Postgres connection string (required)
//	OWNER_TOKEN   bearer token for the owner endpoints (required)
//	PORT          listen port when not on Lambda (default 8080)
//
// `server seed <dir>` loads checklist.json, guides.json and features.json from dir and exits.
package main

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/akrylysov/algnhsa"

	"github.com/vikash-parashar/ampli_owner/server/internal/api"
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

	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		return errMissing("DATABASE_URL")
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

	token := os.Getenv("OWNER_TOKEN")
	if len(token) < 24 {
		return errMissing("OWNER_TOKEN (24+ characters)")
	}
	h := (&api.Server{Store: st, OwnerToken: token}).Handler()

	if os.Getenv("AWS_LAMBDA_FUNCTION_NAME") != "" {
		algnhsa.ListenAndServe(h, nil)
		return nil
	}

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	srv := &http.Server{Addr: ":" + port, Handler: h, ReadHeaderTimeout: 10 * time.Second}
	go func() {
		<-ctx.Done()
		shutdown, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutdown)
	}()
	slog.Info("listening", "port", port)
	if err := srv.ListenAndServe(); err != http.ErrServerClosed {
		return err
	}
	return nil
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
