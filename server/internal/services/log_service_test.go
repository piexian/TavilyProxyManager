package services

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"path/filepath"
	"testing"

	"tavily-proxy/server/internal/db"
	"tavily-proxy/server/internal/models"
)

// 空库时 StatusCodeCounts 必须序列化为 []，而不是 null：前端对其直接 .map，null 会让日志页报错。
func TestStatusCodeCounts_EmptySerializesAsArray(t *testing.T) {
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
	logs := NewLogService(database, logger)

	got, err := logs.StatusCodeCounts(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if body, _ := json.Marshal(got); string(body) != "[]" {
		t.Fatalf("empty status counts marshal as %s, want []", body)
	}

	for _, code := range []int{200, 200, 429} {
		if err := logs.Create(context.Background(), &models.RequestLog{RequestID: "r", Endpoint: "/search", StatusCode: code}); err != nil {
			t.Fatal(err)
		}
	}
	got, err = logs.StatusCodeCounts(context.Background())
	if err != nil || len(got) != 2 || got[0].StatusCode != 200 || got[0].Count != 2 || got[1].StatusCode != 429 {
		t.Fatalf("counts: %+v err=%v", got, err)
	}
}
