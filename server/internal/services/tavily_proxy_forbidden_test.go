package services

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"tavily-proxy/server/internal/db"

	"tavily-proxy/server/internal/models"
)

func TestTavilyProxy_ForbiddenHandled(t *testing.T) {
	t.Parallel()

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	database, err := db.Open(filepath.Join(t.TempDir(), "app.db"))
	if err != nil {
		t.Fatalf("db open: %v", err)
	}
	sqlDB, err := database.DB()
	if err != nil {
		t.Fatalf("db handle: %v", err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })

	ctx := context.Background()
	keys := NewKeyService(database, logger)

	// Create two keys: forbidden key has higher quota so it's always tried first
	// (Candidates sorts by remaining quota descending; same quota = random shuffle)
	forbidden, err := keys.Create(ctx, "tvly-403", "forbidden", 2000)
	if err != nil {
		t.Fatalf("create forbidden key: %v", err)
	}
	ok, err := keys.Create(ctx, "tvly-ok", "ok", 1000)
	if err != nil {
		t.Fatalf("create ok key: %v", err)
	}

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Header.Get("Authorization") {
		case "Bearer tvly-403":
			w.WriteHeader(http.StatusForbidden)
			_, _ = w.Write([]byte(`{"error":"forbidden","request_id":"req-403"}`))
		case "Bearer tvly-ok":
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"ok":true,"request_id":"req-ok"}`))
		default:
			w.WriteHeader(http.StatusUnauthorized)
		}
	}))
	t.Cleanup(upstream.Close)

	proxy := NewTavilyProxy(upstream.URL, 5*time.Second, keys, nil, nil, logger)

	resp, err := proxy.Do(ctx, ProxyRequest{
		Method:      http.MethodPost,
		Path:        "/search",
		Body:        []byte(`{"query":"test"}`),
		ContentType: "application/json",
		ClientIP:    "127.0.0.1",
	})
	if err != nil {
		t.Fatalf("proxy.Do: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d (body=%q)", resp.StatusCode, http.StatusOK, string(resp.Body))
	}

	// The forbidden key should have been marked exhausted
	var forbiddenKey models.APIKey
	if err := database.First(&forbiddenKey, forbidden.ID).Error; err != nil {
		t.Fatalf("find forbidden key: %v", err)
	}
	if forbiddenKey.UsedQuota != forbiddenKey.TotalQuota {
		t.Errorf("forbidden key used_quota = %d, want %d (should be marked exhausted)", forbiddenKey.UsedQuota, forbiddenKey.TotalQuota)
	}

	// The ok key should have been incremented by 1 (basic search)
	var okKey models.APIKey
	if err := database.First(&okKey, ok.ID).Error; err != nil {
		t.Fatalf("find ok key: %v", err)
	}
	if okKey.UsedQuota != 1 {
		t.Errorf("ok key used_quota = %d, want 1", okKey.UsedQuota)
	}
}
