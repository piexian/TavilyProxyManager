package mcpserver

import (
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

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"tavily-proxy/server/internal/db"
	"tavily-proxy/server/internal/services"
)

func TestTavilyUsage_ReturnsAggregatedStatsWithoutUpstreamCall(t *testing.T) {
	t.Parallel()

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	ctx := context.Background()

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
	if err := master.LoadOrCreate(ctx); err != nil {
		t.Fatalf("master key init: %v", err)
	}

	keys := services.NewKeyService(database, logger)
	keyA, err := keys.Create(ctx, "tvly-pool-a", "a", 1000)
	if err != nil {
		t.Fatalf("create key a: %v", err)
	}
	keyB, err := keys.Create(ctx, "tvly-pool-b", "b", 500)
	if err != nil {
		t.Fatalf("create key b: %v", err)
	}
	if err := keys.SetUsage(ctx, keyA.ID, 250, nil); err != nil {
		t.Fatalf("set usage for key a: %v", err)
	}
	if err := keys.SetUsage(ctx, keyB.ID, 100, nil); err != nil {
		t.Fatalf("set usage for key b: %v", err)
	}

	stats := services.NewStatsService(database)
	expected, err := stats.Get(ctx)
	if err != nil {
		t.Fatalf("stats get: %v", err)
	}

	var upstreamCalls int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&upstreamCalls, 1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"key":{"usage":0,"limit":0}}`))
	}))
	t.Cleanup(upstream.Close)

	proxy := services.NewTavilyProxy(upstream.URL, 3*time.Second, keys, nil, nil, logger)
	handler := NewHandler(Dependencies{
		MasterKey:  master,
		Proxy:      proxy,
		Stats:      stats,
		Stateless:  true,
		SessionTTL: time.Minute,
	})
	server := httptest.NewServer(handler.Streamable)
	t.Cleanup(server.Close)

	session := connectMCPClient(t, server.URL, master.Get())
	defer session.Close()

	callCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	res, err := session.CallTool(callCtx, &mcp.CallToolParams{Name: "tavily-usage"})
	if err != nil {
		t.Fatalf("call tool: %v", err)
	}
	if res.IsError {
		t.Fatalf("expected success, got error result: %+v", res)
	}

	payload := mustStructuredMap(t, res.StructuredContent)
	key := mustStructuredMap(t, payload["key"])
	usage := asInt64(t, key["usage"])
	limit := asInt64(t, key["limit"])

	if usage != expected.TotalUsed {
		t.Fatalf("unexpected usage: got %d want %d", usage, expected.TotalUsed)
	}
	if limit != expected.TotalQuota {
		t.Fatalf("unexpected limit: got %d want %d", limit, expected.TotalQuota)
	}
	textPayload := mustStructuredMap(t, mustTextJSON(t, res))
	textKey := mustStructuredMap(t, textPayload["key"])
	if asInt64(t, textKey["usage"]) != usage || asInt64(t, textKey["limit"]) != limit {
		t.Fatalf("content text does not match structured content")
	}

	if got := atomic.LoadInt32(&upstreamCalls); got != 0 {
		t.Fatalf("unexpected upstream calls for tavily-usage: %d", got)
	}
}

func TestTavilyUsage_ReturnsErrorWhenStatsUnavailable(t *testing.T) {
	t.Parallel()

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	ctx := context.Background()

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
	if err := master.LoadOrCreate(ctx); err != nil {
		t.Fatalf("master key init: %v", err)
	}

	handler := NewHandler(Dependencies{
		MasterKey:  master,
		Stateless:  true,
		SessionTTL: time.Minute,
	})
	server := httptest.NewServer(handler.Streamable)
	t.Cleanup(server.Close)

	session := connectMCPClient(t, server.URL, master.Get())
	defer session.Close()

	callCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	res, err := session.CallTool(callCtx, &mcp.CallToolParams{Name: "tavily-usage"})
	if err != nil {
		t.Fatalf("call tool: %v", err)
	}
	if !res.IsError {
		t.Fatalf("expected error result when stats unavailable")
	}

	payload := mustStructuredMap(t, res.StructuredContent)
	if payload["error"] != "stats service unavailable" {
		t.Fatalf("unexpected error payload: %+v", payload)
	}
}

func TestMCPHandler_RejectsUnauthorizedRequest(t *testing.T) {
	t.Parallel()

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	ctx := context.Background()

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
	if err := master.LoadOrCreate(ctx); err != nil {
		t.Fatalf("master key init: %v", err)
	}

	handler := NewHandler(Dependencies{
		MasterKey:  master,
		Stateless:  true,
		SessionTTL: time.Minute,
	})

	req := httptest.NewRequest(http.MethodPost, "/mcp", nil)
	w := httptest.NewRecorder()
	handler.Streamable.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("unexpected status: got %d want %d", w.Code, http.StatusUnauthorized)
	}
}

type authRoundTripper struct {
	base  http.RoundTripper
	token string
}

func (t *authRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	transport := t.base
	if transport == nil {
		transport = http.DefaultTransport
	}
	clone := req.Clone(req.Context())
	clone.Header = req.Header.Clone()
	clone.Header.Set("Authorization", "Bearer "+t.token)
	return transport.RoundTrip(clone)
}

func connectMCPClient(t *testing.T, endpoint, token string) *mcp.ClientSession {
	t.Helper()

	connectCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	client := mcp.NewClient(&mcp.Implementation{
		Name:    "test-client",
		Version: "0.0.1",
	}, nil)
	session, err := client.Connect(connectCtx, &mcp.StreamableClientTransport{
		Endpoint:   endpoint,
		HTTPClient: &http.Client{Transport: &authRoundTripper{token: token}},
		MaxRetries: -1,
	}, nil)
	if err != nil {
		t.Fatalf("connect mcp client: %v", err)
	}
	return session
}

func mustTextJSON(t *testing.T, result *mcp.CallToolResult) map[string]any {
	t.Helper()

	if len(result.Content) == 0 {
		t.Fatalf("missing content")
	}
	text, ok := result.Content[0].(*mcp.TextContent)
	if !ok {
		t.Fatalf("unexpected content type: %T", result.Content[0])
	}

	var out map[string]any
	if err := json.Unmarshal([]byte(text.Text), &out); err != nil {
		t.Fatalf("text content is not json: %v (text=%q)", err, text.Text)
	}
	return out
}

func mustStructuredMap(t *testing.T, v any) map[string]any {
	t.Helper()
	out, ok := v.(map[string]any)
	if !ok {
		t.Fatalf("unexpected structured type: %T", v)
	}
	return out
}

func asInt64(t *testing.T, v any) int64 {
	t.Helper()
	switch x := v.(type) {
	case int:
		return int64(x)
	case int32:
		return int64(x)
	case int64:
		return x
	case float64:
		return int64(x)
	case json.Number:
		n, err := x.Int64()
		if err != nil {
			t.Fatalf("invalid json number %q: %v", x, err)
		}
		return n
	default:
		t.Fatalf("unexpected numeric type: %T", v)
		return 0
	}
}
