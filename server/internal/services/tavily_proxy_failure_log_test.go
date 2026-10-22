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

// 请求已发出但结果未知（上游超时 / 客户端断连）时，可能已被上游计费，
// 日志与统计仍必须留下 502 记录，不能凭空消失。
func TestTavilyProxy_UnknownResultStillRecordsFailure(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		timeout time.Duration
		cancel  bool // 在上游处理期间取消请求 ctx（模拟客户端断连）
	}{
		{name: "upstream_timeout", timeout: 20 * time.Millisecond},
		{name: "client_disconnect", timeout: 5 * time.Second, cancel: true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
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

			keys := NewKeyService(database, logger)
			logs := NewLogService(database, logger)
			stats := NewStatsService(database)
			if _, err := keys.Create(context.Background(), "tvly-test-a", "a", 1000); err != nil {
				t.Fatalf("create key: %v", err)
			}

			ctx, cancel := context.WithCancel(context.Background())
			t.Cleanup(cancel)
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if tc.cancel {
					cancel()
				}
				time.Sleep(150 * time.Millisecond)
				_, _ = io.WriteString(w, `{"results":[],"usage":{"credits":1}}`)
			}))
			t.Cleanup(upstream.Close)

			proxy := NewTavilyProxy(upstream.URL, tc.timeout, keys, logs, stats, logger)
			_, err = proxy.Do(ctx, ProxyRequest{Method: http.MethodPost, Path: "/search", Body: []byte(`{"query":"x"}`), ContentType: "application/json"})
			if err == nil {
				t.Fatal("expected unknown-result error")
			}

			var logCount, statCount int64
			if err := database.Model(&models.RequestLog{}).Where("status_code = ?", http.StatusBadGateway).Count(&logCount).Error; err != nil {
				t.Fatalf("count logs: %v", err)
			}
			if err := database.Model(&models.RequestStat{}).Count(&statCount).Error; err != nil {
				t.Fatalf("count stats: %v", err)
			}
			if logCount != 1 {
				t.Fatalf("502 request logs: got %d want 1", logCount)
			}
			if statCount == 0 {
				t.Fatal("request was not counted in stats")
			}
		})
	}
}

// 最终以 429/403 返回给客户端的请求，以及缓存命中的请求，都要在日志里保留发起者身份，
// 否则门户的「我发起的请求」与按访问密钥过滤的日志会系统性漏数。
func TestTavilyProxy_LogsKeepAccessKeyIdentity(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name   string
		status int
	}{
		{name: "rate_limited", status: http.StatusTooManyRequests},
		{name: "forbidden", status: http.StatusForbidden},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			logger := slog.New(slog.NewTextHandler(io.Discard, nil))
			database, err := db.Open(filepath.Join(t.TempDir(), "app.db"))
			if err != nil {
				t.Fatalf("db open: %v", err)
			}
			sqlDB, _ := database.DB()
			t.Cleanup(func() { _ = sqlDB.Close() })

			keys := NewKeyService(database, logger)
			logs := NewLogService(database, logger)
			if _, err := keys.Create(context.Background(), "tvly-test-a", "a", 1000); err != nil {
				t.Fatalf("create key: %v", err)
			}
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tc.status)
				_, _ = io.WriteString(w, `{"detail":{"error":"nope"}}`)
			}))
			t.Cleanup(upstream.Close)

			proxy := NewTavilyProxy(upstream.URL, time.Second, keys, logs, nil, logger)
			resp, err := proxy.Do(context.Background(), ProxyRequest{
				Method: http.MethodPost, Path: "/search", Body: []byte(`{"query":"x"}`), ContentType: "application/json",
				AccessKeyID: 7, AccessKeyAlias: "donor",
			})
			if err != nil || resp.StatusCode != tc.status {
				t.Fatalf("resp=%d err=%v, want %d", resp.StatusCode, err, tc.status)
			}

			var row models.RequestLog
			if err := database.Where("status_code = ?", tc.status).First(&row).Error; err != nil {
				t.Fatalf("log row: %v", err)
			}
			if row.AccessKeyID != 7 || row.AccessKeyAlias != "donor" {
				t.Fatalf("access key identity lost: id=%d alias=%q", row.AccessKeyID, row.AccessKeyAlias)
			}
			// 429/403 都只换 Key 重试，不应把 Key 标记为额度耗尽。
			if candidates, err := keys.Candidates(context.Background()); err != nil || len(candidates) != 1 {
				t.Fatalf("key must stay available: candidates=%d err=%v", len(candidates), err)
			}
		})
	}
}

func TestTavilyProxy_CacheHitLogKeepsAccessKeyIdentity(t *testing.T) {
	t.Parallel()

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	database, err := db.Open(filepath.Join(t.TempDir(), "app.db"))
	if err != nil {
		t.Fatalf("db open: %v", err)
	}
	sqlDB, _ := database.DB()
	t.Cleanup(func() { _ = sqlDB.Close() })

	ctx := context.Background()
	settings := NewSettingsService(database)
	if err := settings.SetBool(ctx, SettingCacheEnabled, true); err != nil {
		t.Fatalf("enable cache: %v", err)
	}
	keys := NewKeyService(database, logger)
	logs := NewLogService(database, logger)
	if _, err := keys.Create(ctx, "tvly-test-a", "a", 1000); err != nil {
		t.Fatalf("create key: %v", err)
	}
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"results":[],"usage":{"credits":1}}`)
	}))
	t.Cleanup(upstream.Close)

	proxy := NewTavilyProxy(upstream.URL, time.Second, keys, logs, nil, logger).
		WithSettings(settings).
		WithCache(NewCacheService(database, logger))
	req := ProxyRequest{
		Method: http.MethodPost, Path: "/search", Body: []byte(`{"query":"cached"}`), ContentType: "application/json",
		AccessKeyID: 9, AccessKeyAlias: "donor",
	}
	for i := 0; i < 2; i++ { // 第 1 次回源并写缓存，第 2 次命中缓存
		if _, err := proxy.Do(ctx, req); err != nil {
			t.Fatalf("request %d: %v", i, err)
		}
	}

	var row models.RequestLog
	if err := database.Where("cache_hit = ?", true).First(&row).Error; err != nil {
		t.Fatalf("cache-hit log row: %v", err)
	}
	if row.AccessKeyID != 9 || row.AccessKeyAlias != "donor" {
		t.Fatalf("access key identity lost on cache hit: id=%d alias=%q", row.AccessKeyID, row.AccessKeyAlias)
	}
}
