package httpserver

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"tavily-proxy/server/internal/db"
	"tavily-proxy/server/internal/services"
)

func TestProxy_LegacyBodyAPIKey_TavilyKey_IsUnauthorized(t *testing.T) {
	t.Parallel()

	gin.SetMode(gin.TestMode)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	const directKey = "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"
	const poolKey = "tvly-pool-1234567890abcdef"
	var upstreamCalls int32

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&upstreamCalls, 1)
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(upstream.Close)

	database, err := db.Open(filepath.Join(t.TempDir(), "app.db"))
	if err != nil {
		t.Fatalf("db open: %v", err)
	}
	sqlDB, err := database.DB()
	if err != nil {
		t.Fatalf("db handle: %v", err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })

	master := services.NewMasterKeyService(database, logger)
	if err := master.LoadOrCreate(context.Background()); err != nil {
		t.Fatalf("master key init: %v", err)
	}

	ctx := context.Background()
	keys := services.NewKeyService(database, logger)
	if _, err := keys.Create(ctx, poolKey, "pool", 1000); err != nil {
		t.Fatalf("create key: %v", err)
	}
	proxy := services.NewTavilyProxy(upstream.URL, 5*time.Second, keys, nil, nil, logger)

	router := NewRouter(Dependencies{
		MasterKeyService: master,
		TavilyProxy:      proxy,
	})

	payload := []byte(`{"query":"today is 2026-01-26 \r\n test query","max_results":5,"api_key":"` + directKey + `"}`)

	req := httptest.NewRequest(http.MethodPost, "/search", bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("unexpected status: got %d want %d (body=%q)", w.Code, http.StatusUnauthorized, w.Body.String())
	}
	if got := atomic.LoadInt32(&upstreamCalls); got != 0 {
		t.Fatalf("upstream should not be called, got %d calls", got)
	}
}

func TestProxy_LegacyBodyAPIKey_MasterKey_StripsFieldAndUsesPoolKey(t *testing.T) {
	t.Parallel()

	gin.SetMode(gin.TestMode)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	const poolKey = "tvly-pool-1234567890abcdef"

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer "+poolKey {
			t.Fatalf("unexpected Authorization header: got %q want %q", got, "Bearer "+poolKey)
		}
		body, _ := io.ReadAll(r.Body)
		var m map[string]any
		if err := json.Unmarshal(body, &m); err != nil {
			t.Fatalf("upstream body json: %v (body=%q)", err, string(body))
		}
		if _, ok := m["api_key"]; ok {
			t.Fatalf("api_key should be stripped from upstream body (body=%q)", string(body))
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"request_id":"test","results":[]}`))
	}))
	t.Cleanup(upstream.Close)

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
	master := services.NewMasterKeyService(database, logger)
	if err := master.LoadOrCreate(ctx); err != nil {
		t.Fatalf("master key init: %v", err)
	}

	keys := services.NewKeyService(database, logger)
	if _, err := keys.Create(ctx, poolKey, "pool", 1000); err != nil {
		t.Fatalf("create key: %v", err)
	}

	proxy := services.NewTavilyProxy(upstream.URL, 5*time.Second, keys, nil, nil, logger)

	router := NewRouter(Dependencies{
		MasterKeyService: master,
		TavilyProxy:      proxy,
	})

	payload := []byte(`{"query":"hello","max_results":5,"api_key":"` + master.Get() + `"}`)

	req := httptest.NewRequest(http.MethodPost, "/search", bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("unexpected status: got %d want %d (body=%q)", w.Code, http.StatusOK, w.Body.String())
	}
}

func TestProxy_LegacyQueryAPIKey_MasterKey_ReturnsPoolUsageSummary(t *testing.T) {
	t.Parallel()

	gin.SetMode(gin.TestMode)
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

	master := services.NewMasterKeyService(database, logger)
	ctx := context.Background()
	if err := master.LoadOrCreate(ctx); err != nil {
		t.Fatalf("master key init: %v", err)
	}

	keys := services.NewKeyService(database, logger)
	first, err := keys.Create(ctx, "tvly-pool-a", "pool-a", 1000)
	if err != nil {
		t.Fatalf("create first key: %v", err)
	}
	if err := keys.SetUsage(ctx, first.ID, 250, nil); err != nil {
		t.Fatalf("set first key usage: %v", err)
	}

	second, err := keys.Create(ctx, "tvly-pool-b", "pool-b", 500)
	if err != nil {
		t.Fatalf("create second key: %v", err)
	}
	if err := keys.SetUsage(ctx, second.ID, 100, nil); err != nil {
		t.Fatalf("set second key usage: %v", err)
	}

	invalid, err := keys.Create(ctx, "tvly-invalid", "invalid", 999)
	if err != nil {
		t.Fatalf("create invalid key: %v", err)
	}
	if err := keys.MarkInvalid(ctx, invalid.ID); err != nil {
		t.Fatalf("mark invalid key: %v", err)
	}

	proxy := services.NewTavilyProxy("http://127.0.0.1:1", 200*time.Millisecond, keys, nil, nil, logger)

	router := NewRouter(Dependencies{
		MasterKeyService: master,
		TavilyProxy:      proxy,
	})

	req := httptest.NewRequest(http.MethodGet, "/usage?api_key="+master.Get()+"&foo=bar", nil)
	req.Header.Set("Accept", "*/*")

	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("unexpected status: got %d want %d (body=%q)", w.Code, http.StatusOK, w.Body.String())
	}

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
	if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
		t.Fatalf("unmarshal response: %v (body=%q)", err, w.Body.String())
	}

	if out.Key.Usage != 350 || out.Key.Limit != 1500 {
		t.Fatalf("unexpected key summary: %+v", out.Key)
	}
	if out.Account.PlanUsage != 350 || out.Account.PlanLimit != 1500 {
		t.Fatalf("unexpected account summary: %+v", out.Account)
	}
}

func TestProxy_AccessKeyUsage_ReturnsPoolUsageSummary(t *testing.T) {
	t.Parallel()

	gin.SetMode(gin.TestMode)
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
	master := services.NewMasterKeyService(database, logger)
	if err := master.LoadOrCreate(ctx); err != nil {
		t.Fatalf("master key init: %v", err)
	}

	keys := services.NewKeyService(database, logger)
	first, err := keys.Create(ctx, "tvly-pool-1", "pool-1", 1000)
	if err != nil {
		t.Fatalf("create first key: %v", err)
	}
	if err := keys.SetUsage(ctx, first.ID, 200, nil); err != nil {
		t.Fatalf("set first key usage: %v", err)
	}

	second, err := keys.Create(ctx, "tvly-pool-2", "pool-2", 500)
	if err != nil {
		t.Fatalf("create second key: %v", err)
	}
	if err := keys.SetUsage(ctx, second.ID, 500, nil); err != nil {
		t.Fatalf("set second key usage: %v", err)
	}

	disabled, err := keys.Create(ctx, "tvly-disabled", "disabled", 800)
	if err != nil {
		t.Fatalf("create disabled key: %v", err)
	}
	if err := keys.MarkInactive(ctx, disabled.ID); err != nil {
		t.Fatalf("mark disabled key inactive: %v", err)
	}

	accessKeys := services.NewAccessKeyService(database, logger)
	accessKey, err := accessKeys.Create(ctx, "client")
	if err != nil {
		t.Fatalf("create access key: %v", err)
	}

	proxy := services.NewTavilyProxy("http://127.0.0.1:1", 200*time.Millisecond, keys, nil, nil, logger)
	router := NewRouter(Dependencies{
		MasterKeyService: master,
		AccessKeyService: accessKeys,
		TavilyProxy:      proxy,
	})

	req := httptest.NewRequest(http.MethodGet, "/usage", nil)
	req.Header.Set("Authorization", "Bearer "+accessKey.Key)
	req.Header.Set("Accept", "application/json")

	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("unexpected status: got %d want %d (body=%q)", w.Code, http.StatusOK, w.Body.String())
	}

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
	if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
		t.Fatalf("unmarshal response: %v (body=%q)", err, w.Body.String())
	}
	if out.Key.Usage != 700 || out.Key.Limit != 1500 {
		t.Fatalf("unexpected key summary: %+v", out.Key)
	}
	if out.Account.PlanUsage != 700 || out.Account.PlanLimit != 1500 {
		t.Fatalf("unexpected account summary: %+v", out.Account)
	}
}
