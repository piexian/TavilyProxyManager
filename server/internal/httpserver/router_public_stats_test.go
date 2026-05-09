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
	"tavily-proxy/server/internal/services"
)

func TestHandlePublicPoolStats_ReturnsActivePoolSummary(t *testing.T) {
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

	keys := services.NewKeyService(database, logger)
	ctx := context.Background()

	first, err := keys.Create(ctx, "tvly-public-a", "a", 1000)
	if err != nil {
		t.Fatalf("create first key: %v", err)
	}
	if err := keys.SetUsage(ctx, first.ID, 300, nil); err != nil {
		t.Fatalf("set first key usage: %v", err)
	}

	second, err := keys.Create(ctx, "tvly-public-b", "b", 600)
	if err != nil {
		t.Fatalf("create second key: %v", err)
	}
	if err := keys.SetUsage(ctx, second.ID, 150, nil); err != nil {
		t.Fatalf("set second key usage: %v", err)
	}
	if err := keys.MarkInvalid(ctx, second.ID); err != nil {
		t.Fatalf("mark second key invalid: %v", err)
	}

	third, err := keys.Create(ctx, "tvly-public-c", "c", 400)
	if err != nil {
		t.Fatalf("create third key: %v", err)
	}
	if err := keys.SetUsage(ctx, third.ID, 20, nil); err != nil {
		t.Fatalf("set third key usage: %v", err)
	}
	if err := keys.MarkInactive(ctx, third.ID); err != nil {
		t.Fatalf("mark third key inactive: %v", err)
	}

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/public/stats", nil)

	handlePublicPoolStats(c, keys)

	if w.Code != http.StatusOK {
		t.Fatalf("unexpected status: got %d want %d", w.Code, http.StatusOK)
	}

	var out services.PoolQuotaSummary
	if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
		t.Fatalf("unmarshal response: %v (body=%q)", err, w.Body.String())
	}
	if out.TotalQuota != 1000 || out.TotalUsed != 300 || out.TotalRemaining != 700 || out.ActiveKeyCount != 1 {
		t.Fatalf("unexpected summary: %+v", out)
	}
}
