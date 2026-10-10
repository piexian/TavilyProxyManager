package services

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync/atomic"
	"tavily-proxy/server/internal/db"
	"testing"
	"time"
)

func TestAuditPaidFeature403DoesNotExhaustSearchKeys(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	database, err := db.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, _ := database.DB()
	defer sqlDB.Close()
	keys := NewKeyService(database, logger)
	for _, key := range []string{"tvly-test-a", "tvly-test-b"} {
		if _, err := keys.Create(context.Background(), key, key, 1000); err != nil {
			t.Fatal(err)
		}
	}
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(403)
		io.WriteString(w, `{"detail":{"error":"Access denied. Usage logs are only available to paying customers."}}`)
	}))
	defer upstream.Close()
	proxy := NewTavilyProxy(upstream.URL, time.Second, keys, nil, nil, logger)
	_, _ = proxy.Do(context.Background(), ProxyRequest{Method: "POST", Path: "/logs", Body: []byte(`{"limit":1}`), ContentType: "application/json"})
	candidates, err := keys.Candidates(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(candidates) != 2 {
		t.Fatalf("a feature-only 403 exhausted otherwise valid keys: remaining=%d, want 2", len(candidates))
	}
}

func TestAuditTimedOutResearchIsNotSubmittedToEveryKey(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	database, err := db.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, _ := database.DB()
	defer sqlDB.Close()
	keys := NewKeyService(database, logger)
	for _, key := range []string{"tvly-test-a", "tvly-test-b"} {
		if _, err := keys.Create(context.Background(), key, key, 1000); err != nil {
			t.Fatal(err)
		}
	}
	var requests atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		time.Sleep(80 * time.Millisecond)
		io.WriteString(w, `{"results":[],"usage":{"credits":1}}`)
	}))
	defer upstream.Close()
	proxy := NewTavilyProxy(upstream.URL, 20*time.Millisecond, keys, nil, nil, logger)
	_, err = proxy.Do(context.Background(), ProxyRequest{Method: "POST", Path: "/research", Body: []byte(`{"input":"test"}`), ContentType: "application/json"})
	if err == nil {
		t.Fatal("expected unknown-result error")
	}
	if requests.Load() != 1 {
		t.Fatalf("an accepted but slow crawl was submitted %d times, want 1", requests.Load())
	}
}

func TestMapAndCrawlTimeoutBudget(t *testing.T) {
	proxy := &TavilyProxy{client: &http.Client{Timeout: 30 * time.Second}}
	for _, path := range []string{"/map", "/crawl"} {
		if got := proxy.requestTimeout(ProxyRequest{Path: path, Body: []byte(`{}`)}); got != 160*time.Second {
			t.Fatalf("default %s timeout: %s", path, got)
		}
		if got := proxy.requestTimeout(ProxyRequest{Path: path, Body: []byte(`{"timeout":60}`)}); got != 70*time.Second {
			t.Fatalf("explicit %s timeout: %s", path, got)
		}
	}
	if got := proxy.requestTimeout(ProxyRequest{Path: "/search"}); got != 30*time.Second {
		t.Fatalf("search budget changed: %s", got)
	}
}
