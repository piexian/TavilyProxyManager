package services

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"tavily-proxy/server/internal/db"
)

func TestQuotaSyncService_SyncOne_TooManyRequestsDoesNotMarkExhausted(t *testing.T) {
	t.Parallel()

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/usage" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = w.Write([]byte(`{"error":"rate_limit","message":"Too many requests"}`))
	}))
	t.Cleanup(upstream.Close)

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

	keys := NewKeyService(database, logger)
	proxy := NewTavilyProxy(upstream.URL, 5*time.Second, keys, nil, nil, logger)
	sync := NewQuotaSyncService(keys, proxy, logger)

	ctx := context.Background()
	created, err := keys.Create(ctx, "tvly-test", "test", 1000)
	if err != nil {
		t.Fatalf("create key: %v", err)
	}
	if err := keys.SetUsage(ctx, created.ID, 7, nil); err != nil {
		t.Fatalf("set usage: %v", err)
	}

	_, err = sync.SyncOne(ctx, created.ID)
	if err == nil {
		t.Fatalf("expected error, got nil")
	}

	got, err := keys.Get(ctx, created.ID)
	if err != nil {
		t.Fatalf("get key: %v", err)
	}
	if got.UsedQuota != 7 {
		t.Fatalf("used_quota changed unexpectedly: got %d want %d", got.UsedQuota, 7)
	}
	if got.UsedQuota == got.TotalQuota {
		t.Fatalf("key marked exhausted unexpectedly: used_quota=%d total_quota=%d", got.UsedQuota, got.TotalQuota)
	}
	if !got.IsActive {
		t.Fatalf("key marked inactive unexpectedly")
	}
}

func TestQuotaSyncService_SyncOne_ExhaustedStatusMarksExhausted(t *testing.T) {
	t.Parallel()

	const exhaustedStatus = 433

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/usage" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(exhaustedStatus)
		_, _ = w.Write([]byte(`{"error":"quota_exhausted"}`))
	}))
	t.Cleanup(upstream.Close)

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

	keys := NewKeyService(database, logger)
	proxy := NewTavilyProxy(upstream.URL, 5*time.Second, keys, nil, nil, logger)
	sync := NewQuotaSyncService(keys, proxy, logger)

	ctx := context.Background()
	created, err := keys.Create(ctx, "tvly-test", "test", 1000)
	if err != nil {
		t.Fatalf("create key: %v", err)
	}
	if err := keys.SetUsage(ctx, created.ID, 7, nil); err != nil {
		t.Fatalf("set usage: %v", err)
	}

	_, err = sync.SyncOne(ctx, created.ID)
	if err == nil {
		t.Fatalf("expected error, got nil")
	}

	got, err := keys.Get(ctx, created.ID)
	if err != nil {
		t.Fatalf("get key: %v", err)
	}
	if got.UsedQuota != got.TotalQuota {
		t.Fatalf("key not marked exhausted: used_quota=%d total_quota=%d", got.UsedQuota, got.TotalQuota)
	}
}

func TestQuotaSyncService_SyncOne_UnauthorizedMarksInvalid(t *testing.T) {
	t.Parallel()

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/usage" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":"unauthorized","message":"API key banned"}`))
	}))
	t.Cleanup(upstream.Close)

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

	keys := NewKeyService(database, logger)
	proxy := NewTavilyProxy(upstream.URL, 5*time.Second, keys, nil, nil, logger)
	sync := NewQuotaSyncService(keys, proxy, logger)

	ctx := context.Background()
	created, err := keys.Create(ctx, "tvly-test", "test", 1000)
	if err != nil {
		t.Fatalf("create key: %v", err)
	}
	if err := keys.SetUsage(ctx, created.ID, 7, nil); err != nil {
		t.Fatalf("set usage: %v", err)
	}

	_, err = sync.SyncOne(ctx, created.ID)
	if err == nil {
		t.Fatalf("expected error, got nil")
	}

	got, err := keys.Get(ctx, created.ID)
	if err != nil {
		t.Fatalf("get key: %v", err)
	}
	if got.UsedQuota != 7 {
		t.Fatalf("used_quota changed unexpectedly: got %d want %d", got.UsedQuota, 7)
	}
	if got.IsActive {
		t.Fatalf("key not marked inactive")
	}
	if !got.IsInvalid {
		t.Fatalf("key not marked invalid")
	}
}

func TestQuotaSyncService_SyncOne_PrefersOfficialUsageOverLocal(t *testing.T) {
	t.Parallel()

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/usage" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		// 官方返回 12，本地已记到 900：同步后应以官方为准
		_, _ = w.Write([]byte(`{"key":{"usage":12,"limit":1000},"account":{"plan_usage":12,"plan_limit":1000}}`))
	}))
	t.Cleanup(upstream.Close)

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

	keys := NewKeyService(database, logger)
	proxy := NewTavilyProxy(upstream.URL, 5*time.Second, keys, nil, nil, logger)
	sync := NewQuotaSyncService(keys, proxy, logger)

	ctx := context.Background()
	created, err := keys.Create(ctx, "tvly-test", "test", 1000)
	if err != nil {
		t.Fatalf("create key: %v", err)
	}
	if err := keys.SetUsage(ctx, created.ID, 900, nil); err != nil {
		t.Fatalf("set usage: %v", err)
	}

	item, err := sync.SyncOne(ctx, created.ID)
	if err != nil {
		t.Fatalf("sync: %v", err)
	}
	if item.Status != "ok" {
		t.Fatalf("status = %q, want ok (err=%q)", item.Status, item.Error)
	}
	if item.UsedQuota != 12 {
		t.Fatalf("item.UsedQuota = %d, want 12", item.UsedQuota)
	}

	got, err := keys.Get(ctx, created.ID)
	if err != nil {
		t.Fatalf("get key: %v", err)
	}
	if got.UsedQuota != 12 {
		t.Fatalf("used_quota = %d, want 12 (official should overwrite local)", got.UsedQuota)
	}
	if got.TotalQuota != 1000 {
		t.Fatalf("total_quota = %d, want 1000", got.TotalQuota)
	}
}

func TestQuotaSyncService_SyncOne_UpstreamErrorKeepsLocalUsage(t *testing.T) {
	t.Parallel()

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/usage" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"error":"upstream_error"}`))
	}))
	t.Cleanup(upstream.Close)

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

	keys := NewKeyService(database, logger)
	proxy := NewTavilyProxy(upstream.URL, 5*time.Second, keys, nil, nil, logger)
	sync := NewQuotaSyncService(keys, proxy, logger)

	ctx := context.Background()
	created, err := keys.Create(ctx, "tvly-test", "test", 1000)
	if err != nil {
		t.Fatalf("create key: %v", err)
	}
	if err := keys.SetUsage(ctx, created.ID, 42, nil); err != nil {
		t.Fatalf("set usage: %v", err)
	}

	_, err = sync.SyncOne(ctx, created.ID)
	if err == nil {
		t.Fatalf("expected error, got nil")
	}

	got, err := keys.Get(ctx, created.ID)
	if err != nil {
		t.Fatalf("get key: %v", err)
	}
	if got.UsedQuota != 42 {
		t.Fatalf("used_quota = %d, want 42 (local kept on upstream error)", got.UsedQuota)
	}
}


func TestQuotaSyncService_SyncAllWithConcurrency_LimitsConcurrency(t *testing.T) {
	const concurrency = 2
	const keyCount = 10

	var inFlight int64
	var maxInFlight int64

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/usage" {
			w.WriteHeader(http.StatusNotFound)
			return
		}

		cur := atomic.AddInt64(&inFlight, 1)
		for {
			prev := atomic.LoadInt64(&maxInFlight)
			if cur <= prev || atomic.CompareAndSwapInt64(&maxInFlight, prev, cur) {
				break
			}
		}

		time.Sleep(80 * time.Millisecond)

		atomic.AddInt64(&inFlight, -1)

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"key":{"usage":0,"limit":1000}}`))
	}))
	t.Cleanup(upstream.Close)

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

	keys := NewKeyService(database, logger)
	proxy := NewTavilyProxy(upstream.URL, 5*time.Second, keys, nil, nil, logger)
	sync := NewQuotaSyncService(keys, proxy, logger)

	ctx := context.Background()
	for i := 0; i < keyCount; i++ {
		if _, err := keys.Create(ctx, fmt.Sprintf("tvly-test-%d", i), "test", 1000); err != nil {
			t.Fatalf("create key %d: %v", i, err)
		}
	}

	if _, err := sync.SyncAllWithConcurrency(ctx, concurrency); err != nil {
		t.Fatalf("sync all: %v", err)
	}

	if got := atomic.LoadInt64(&maxInFlight); got > concurrency {
		t.Fatalf("max concurrent /usage requests exceeded: got %d want <= %d", got, concurrency)
	}
}

func TestQuotaSyncService_SyncAllWithConcurrencyAndInterval_RespectsInterval(t *testing.T) {
	const keyCount = 6
	const interval = 50 * time.Millisecond

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/usage" {
			w.WriteHeader(http.StatusNotFound)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"key":{"usage":0,"limit":1000}}`))
	}))
	t.Cleanup(upstream.Close)

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

	keys := NewKeyService(database, logger)
	proxy := NewTavilyProxy(upstream.URL, 5*time.Second, keys, nil, nil, logger)
	sync := NewQuotaSyncService(keys, proxy, logger)

	ctx := context.Background()
	for i := 0; i < keyCount; i++ {
		if _, err := keys.Create(ctx, fmt.Sprintf("tvly-test-interval-%d", i), "test", 1000); err != nil {
			t.Fatalf("create key %d: %v", i, err)
		}
	}

	start := time.Now()
	if _, err := sync.SyncAllWithConcurrencyAndInterval(ctx, keyCount, interval); err != nil {
		t.Fatalf("sync all: %v", err)
	}

	minElapsed := time.Duration(keyCount-1) * interval
	if elapsed := time.Since(start); elapsed < minElapsed {
		t.Fatalf("sync finished too fast: elapsed=%s want >= %s", elapsed, minElapsed)
	}
}
