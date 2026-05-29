package db

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestOpenUsesPlainPathAndAppliesPragmas(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "app.db")
	database, err := Open(path)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	sqlDB, err := database.DB()
	if err != nil {
		t.Fatalf("db handle: %v", err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })

	if _, err := os.Stat(path); err != nil {
		t.Fatalf("expected database at plain path: %v", err)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read temp dir: %v", err)
	}
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), "app.db?") {
			t.Fatalf("unexpected DSN-derived database file created: %s", entry.Name())
		}
	}

	var busyTimeout int
	if err := database.Raw("PRAGMA busy_timeout").Scan(&busyTimeout).Error; err != nil {
		t.Fatalf("read busy_timeout: %v", err)
	}
	if busyTimeout != 5000 {
		t.Fatalf("busy_timeout = %d, want 5000", busyTimeout)
	}

	var foreignKeys int
	if err := database.Raw("PRAGMA foreign_keys").Scan(&foreignKeys).Error; err != nil {
		t.Fatalf("read foreign_keys: %v", err)
	}
	if foreignKeys != 1 {
		t.Fatalf("foreign_keys = %d, want 1", foreignKeys)
	}
}
