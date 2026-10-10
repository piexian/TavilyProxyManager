package httpserver

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/gin-gonic/gin"

	"tavily-proxy/server/internal/db"
	"tavily-proxy/server/internal/models"
	"tavily-proxy/server/internal/services"
)

func TestHandleListLogs_StatusCodeFilter(t *testing.T) {
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

	logs := services.NewLogService(database, logger)
	ctx := context.Background()

	if err := logs.Create(ctx, &models.RequestLog{RequestID: "a", Endpoint: "/search", StatusCode: 200, ClientIP: "127.0.0.1"}); err != nil {
		t.Fatalf("create log: %v", err)
	}
	if err := logs.Create(ctx, &models.RequestLog{RequestID: "b", Endpoint: "/search", StatusCode: 429, ClientIP: "127.0.0.1"}); err != nil {
		t.Fatalf("create log: %v", err)
	}
	if err := logs.Create(ctx, &models.RequestLog{RequestID: "c", Endpoint: "/usage", StatusCode: 200, ClientIP: "127.0.0.1"}); err != nil {
		t.Fatalf("create log: %v", err)
	}

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/logs?page=1&page_size=20&status_code=200", nil)

	handleListLogs(c, logs)

	if w.Code != http.StatusOK {
		t.Fatalf("unexpected status: got %d want %d", w.Code, http.StatusOK)
	}

	var out services.PaginatedLogs
	if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
		t.Fatalf("unmarshal response: %v (body=%q)", err, w.Body.String())
	}
	if out.Total != 2 {
		t.Fatalf("unexpected total: got %d want %d", out.Total, 2)
	}
	if len(out.Items) != 2 {
		t.Fatalf("unexpected items length: got %d want %d", len(out.Items), 2)
	}
	for _, item := range out.Items {
		if item.StatusCode != 200 {
			t.Fatalf("unexpected status_code in item: got %d want %d", item.StatusCode, 200)
		}
	}
}

func TestHandleListLogs_InvalidStatusCode(t *testing.T) {
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

	logs := services.NewLogService(database, logger)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/logs?status_code=abc", nil)

	handleListLogs(c, logs)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("unexpected status: got %d want %d", w.Code, http.StatusBadRequest)
	}

	var out map[string]string
	if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
		t.Fatalf("unmarshal response: %v (body=%q)", err, w.Body.String())
	}
	if out["error"] != "invalid_status_code" {
		t.Fatalf("unexpected error: got %q want %q", out["error"], "invalid_status_code")
	}
}

func TestHandleLogStatusCodes_ReturnsCounts(t *testing.T) {
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

	logs := services.NewLogService(database, logger)
	ctx := context.Background()

	if err := logs.Create(ctx, &models.RequestLog{RequestID: "a", Endpoint: "/search", StatusCode: 200, ClientIP: "127.0.0.1"}); err != nil {
		t.Fatalf("create log: %v", err)
	}
	if err := logs.Create(ctx, &models.RequestLog{RequestID: "b", Endpoint: "/search", StatusCode: 429, ClientIP: "127.0.0.1"}); err != nil {
		t.Fatalf("create log: %v", err)
	}
	if err := logs.Create(ctx, &models.RequestLog{RequestID: "c", Endpoint: "/usage", StatusCode: 200, ClientIP: "127.0.0.1"}); err != nil {
		t.Fatalf("create log: %v", err)
	}

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/logs/status-codes", nil)

	handleLogStatusCodes(c, logs)

	if w.Code != http.StatusOK {
		t.Fatalf("unexpected status: got %d want %d", w.Code, http.StatusOK)
	}

	var out []services.StatusCodeCount
	if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
		t.Fatalf("unmarshal response: %v (body=%q)", err, w.Body.String())
	}
	if len(out) != 2 {
		t.Fatalf("unexpected result length: got %d want %d", len(out), 2)
	}
	if out[0].StatusCode != 200 || out[0].Count != 2 {
		t.Fatalf("unexpected first item: %+v", out[0])
	}
	if out[1].StatusCode != 429 || out[1].Count != 1 {
		t.Fatalf("unexpected second item: %+v", out[1])
	}
}
