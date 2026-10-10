package httpserver

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"tavily-proxy/server/internal/db"
	"tavily-proxy/server/internal/services"
)

func TestHandleExportKeys_ExcludesInvalidKeys(t *testing.T) {
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

	keys := services.NewKeyService(database, logger)
	ctx := context.Background()

	active, err := keys.Create(ctx, "tvly-active", "active", 1000)
	if err != nil {
		t.Fatalf("create active: %v", err)
	}

	exhausted, err := keys.Create(ctx, "tvly-exhausted", "exhausted", 1000)
	if err != nil {
		t.Fatalf("create exhausted: %v", err)
	}
	if err := keys.MarkExhausted(ctx, exhausted.ID); err != nil {
		t.Fatalf("mark exhausted: %v", err)
	}

	invalid, err := keys.Create(ctx, "tvly-invalid", "invalid", 1000)
	if err != nil {
		t.Fatalf("create invalid: %v", err)
	}
	if err := keys.MarkInvalid(ctx, invalid.ID); err != nil {
		t.Fatalf("mark invalid: %v", err)
	}

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/keys/export", nil)

	handleExportKeys(c, keys)

	if w.Code != http.StatusOK {
		t.Fatalf("unexpected status: got %d want %d", w.Code, http.StatusOK)
	}

	if got := w.Header().Get("Content-Disposition"); !strings.Contains(got, "attachment") {
		t.Fatalf("unexpected content-disposition: %q", got)
	}

	if got := w.Header().Get("X-Exported-Count"); got != "2" {
		t.Fatalf("unexpected X-Exported-Count: got %q want %q", got, "2")
	}

	body := strings.TrimSpace(w.Body.String())
	lines := []string{}
	if body != "" {
		lines = strings.Split(body, "\n")
	}
	if len(lines) != 2 {
		t.Fatalf("unexpected line count: got %d want %d (body=%q)", len(lines), 2, body)
	}

	// Keys are listed by id desc, so invalid is filtered out and we expect exhausted then active.
	if lines[0] != exhausted.Key {
		t.Fatalf("unexpected first key: got %q want %q", lines[0], exhausted.Key)
	}
	if lines[1] != active.Key {
		t.Fatalf("unexpected second key: got %q want %q", lines[1], active.Key)
	}
}
