package services

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"path/filepath"
	"testing"
	"time"

	"tavily-proxy/server/internal/db"
)

func TestTavilyProxy_UsageReturnsPoolSummaryWithoutUpstreamCall(t *testing.T) {
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

	first, err := keys.Create(ctx, "tvly-test-a", "a", 1000)
	if err != nil {
		t.Fatalf("create first key: %v", err)
	}
	if err := keys.SetUsage(ctx, first.ID, 150, nil); err != nil {
		t.Fatalf("set first key usage: %v", err)
	}

	second, err := keys.Create(ctx, "tvly-test-b", "b", 600)
	if err != nil {
		t.Fatalf("create second key: %v", err)
	}
	if err := keys.SetUsage(ctx, second.ID, 50, nil); err != nil {
		t.Fatalf("set second key usage: %v", err)
	}
	if err := keys.MarkInvalid(ctx, second.ID); err != nil {
		t.Fatalf("mark second key invalid: %v", err)
	}

	third, err := keys.Create(ctx, "tvly-test-c", "c", 400)
	if err != nil {
		t.Fatalf("create third key: %v", err)
	}
	if err := keys.SetUsage(ctx, third.ID, 100, nil); err != nil {
		t.Fatalf("set third key usage: %v", err)
	}
	if err := keys.MarkInactive(ctx, third.ID); err != nil {
		t.Fatalf("mark third key inactive: %v", err)
	}

	proxy := NewTavilyProxy("http://127.0.0.1:1", 200*time.Millisecond, keys, nil, nil, logger)
	resp, err := proxy.Do(ctx, ProxyRequest{Method: http.MethodGet, Path: "/usage", ClientIP: "127.0.0.1"})
	if err != nil {
		t.Fatalf("proxy request: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("unexpected status: got %d want %d", resp.StatusCode, http.StatusOK)
	}
	if got := resp.Headers.Get("Content-Type"); got != "application/json; charset=utf-8" {
		t.Fatalf("unexpected content type: got %q want %q", got, "application/json; charset=utf-8")
	}

	// Official /usage response shape: key.usage/limit + account.plan_usage/plan_limit
	var out struct {
		Key struct {
			Usage int64 `json:"usage"`
			Limit int64 `json:"limit"`
		} `json:"key"`
		Account struct {
			PlanUsage int64 `json:"plan_usage"`
			PlanLimit int64 `json:"plan_limit"`
		} `json:"account"`
	}
	if err := json.Unmarshal(resp.Body, &out); err != nil {
		t.Fatalf("unmarshal response: %v (body=%q)", err, string(resp.Body))
	}

	if out.Key.Usage != 150 || out.Key.Limit != 1000 {
		t.Fatalf("unexpected key summary: %+v", out.Key)
	}
	if out.Account.PlanUsage != 150 || out.Account.PlanLimit != 1000 {
		t.Fatalf("unexpected account summary: %+v", out.Account)
	}
}
