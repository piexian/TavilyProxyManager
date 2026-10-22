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
