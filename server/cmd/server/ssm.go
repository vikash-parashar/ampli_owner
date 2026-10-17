package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"time"
)

// secretEnv lists the settings that may live in SSM Parameter Store as SecureString instead of plain Lambda env.
var secretEnv = []string{
	"DATABASE_URL", "OWNER_TOKEN", "OWNER_TOKEN_NEXT", "TOKEN_KEY", "PLAY_SERVICE_ACCOUNT_JSON",
	"META_APP_SECRET", "TIKTOK_APP_SECRET", "GOOGLE_ADS_CLIENT_SECRET", "GOOGLE_ADS_DEVELOPER_TOKEN",
}

// loadSSM fills unset secrets from SSM through the "AWS Parameters and Secrets Lambda Extension" layer when
// SSM_PREFIX is set (e.g. /ampli-owner/). Parameter names are the prefix plus the env name, e.g.
// /ampli-owner/OWNER_TOKEN. Missing parameters are skipped; the plain env value wins when both exist.
func loadSSM(ctx context.Context) error {
	prefix := os.Getenv("SSM_PREFIX")
	if prefix == "" {
		return nil
	}
	port := os.Getenv("PARAMETERS_SECRETS_EXTENSION_HTTP_PORT")
	if port == "" {
		port = "2773"
	}
	h := &http.Client{Timeout: 5 * time.Second}
	for _, name := range secretEnv {
		if os.Getenv(name) != "" {
			continue
		}
		u := fmt.Sprintf("http://localhost:%s/systemsmanager/parameters/get?withDecryption=true&name=%s", port, url.QueryEscape(prefix+name))
		req, _ := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
		req.Header.Set("X-Aws-Parameters-Secrets-Token", os.Getenv("AWS_SESSION_TOKEN"))
		resp, err := h.Do(req)
		if err != nil {
			return fmt.Errorf("SSM extension not reachable (add the AWS Parameters and Secrets Lambda Extension layer): %w", err)
		}
		raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
		resp.Body.Close()
		if resp.StatusCode == http.StatusBadRequest || resp.StatusCode == http.StatusNotFound {
			continue // parameter not created
		}
		if resp.StatusCode != http.StatusOK {
			return fmt.Errorf("SSM %s: HTTP %d", prefix+name, resp.StatusCode)
		}
		var out struct {
			Parameter struct {
				Value string `json:"Value"`
			} `json:"Parameter"`
		}
		if err := json.Unmarshal(raw, &out); err != nil {
			return fmt.Errorf("SSM %s: %w", prefix+name, err)
		}
		os.Setenv(name, out.Parameter.Value)
	}
	return nil
}
