package services

import (
	"context"
	"io"
	"log/slog"
	"path/filepath"
	"testing"

	"tavily-proxy/server/internal/db"
	"tavily-proxy/server/internal/models"
)

func TestSettingsServicePingDoesNotWriteHealthSetting(t *testing.T) {
	t.Parallel()

	database, err := db.Open(filepath.Join(t.TempDir(), "app.db"))
	if err != nil {
		t.Fatalf("db open: %v", err)
	}
	sqlDB, err := database.DB()
	if err != nil {
		t.Fatalf("db handle: %v", err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })

	settings := NewSettingsService(database)
	if err := settings.Ping(context.Background()); err != nil {
		t.Fatalf("ping: %v", err)
	}

	var count int64
	if err := database.Model(&models.Setting{}).Where("key = ?", "health_check_at").Count(&count).Error; err != nil {
		t.Fatalf("count health setting: %v", err)
	}
	if count != 0 {
		t.Fatalf("health_check_at rows = %d, want 0", count)
	}
}

func TestKeyServiceIncrementUsedAllowsZeroCredits(t *testing.T) {
	t.Parallel()

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
	keys := NewKeyService(database, slog.New(slog.NewTextHandler(io.Discard, nil)))
	key, err := keys.Create(ctx, "tvly-zero", "zero", 1000)
	if err != nil {
		t.Fatalf("create key: %v", err)
	}

	if err := keys.IncrementUsed(ctx, key.ID, 0); err != nil {
		t.Fatalf("increment zero credits: %v", err)
	}
	got, err := keys.Get(ctx, key.ID)
	if err != nil {
		t.Fatalf("get key: %v", err)
	}
	if got.UsedQuota != 0 {
		t.Fatalf("used quota = %d, want 0", got.UsedQuota)
	}
	if got.LastUsedAt == nil {
		t.Fatal("last_used_at was not updated")
	}
}
