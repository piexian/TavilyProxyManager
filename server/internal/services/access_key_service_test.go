package services

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"

	"gorm.io/gorm"

	"tavily-proxy/server/internal/db"
)

func newAccessKeyTestServices(t *testing.T) (*AccessKeyService, *KeyService) {
	t.Helper()
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
	return NewAccessKeyService(database, logger), NewKeyService(database, logger)
}

// alias 唯一性按「去空格 + Unicode 大小写折叠」判定；SQLite 的 LOWER 只处理 ASCII，
// 非 ASCII 字母的大小写变体不能漏判。
func TestAccessKeyService_AliasUniqueness(t *testing.T) {
	t.Parallel()
	access, keys := newAccessKeyTestServices(t)
	ctx := context.Background()

	for _, alias := range []string{"Alice", "ÉCOLE"} {
		if _, err := access.Create(ctx, alias); err != nil {
			t.Fatalf("create %q: %v", alias, err)
		}
	}
	for _, alias := range []string{" alice ", "ALICE", "école"} {
		if _, err := access.Create(ctx, alias); !errors.Is(err, ErrAliasTaken) {
			t.Fatalf("create %q: got %v want ErrAliasTaken", alias, err)
		}
	}

	// 无主的历史捐赠同样占用 alias（含非 ASCII 大小写变体）。
	if _, _, err := keys.DonateOwnedKey(ctx, "tvly-legacy-key-000000", "ÜBER", 0, 0, nil); err != nil {
		t.Fatal(err)
	}
	if taken, err := keys.UnownedDonationAliasExists(ctx, " über "); err != nil || !taken {
		t.Fatalf("unowned alias: taken=%v err=%v", taken, err)
	}
	if taken, err := keys.UnownedDonationAliasExists(ctx, "other"); err != nil || taken {
		t.Fatalf("unrelated alias: taken=%v err=%v", taken, err)
	}
}

// 并发创建同名访问密钥：「检查 + 写入」在锁内完成，只会有一个成功。
func TestAccessKeyService_ConcurrentCreateSameAliasOnlyOneWins(t *testing.T) {
	t.Parallel()
	access, _ := newAccessKeyTestServices(t)
	ctx := context.Background()

	var wg sync.WaitGroup
	var ok, taken atomic.Int32
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			switch _, err := access.Create(ctx, "race"); {
			case err == nil:
				ok.Add(1)
			case errors.Is(err, ErrAliasTaken):
				taken.Add(1)
			default:
				t.Errorf("unexpected error: %v", err)
			}
		}()
	}
	wg.Wait()
	if ok.Load() != 1 || taken.Load() != 7 {
		t.Fatalf("ok=%d taken=%d, want 1/7", ok.Load(), taken.Load())
	}
}

// Update 会去空格保存；改成他人 alias 被拒；不改 alias 时不受历史重名影响。
func TestAccessKeyService_UpdateAlias(t *testing.T) {
	t.Parallel()
	access, _ := newAccessKeyTestServices(t)
	ctx := context.Background()
	a, _ := access.Create(ctx, "Alpha")
	if _, err := access.Create(ctx, "Beta"); err != nil {
		t.Fatal(err)
	}

	alias := "  Gamma "
	got, err := access.Update(ctx, a.ID, AccessKeyUpdate{Alias: &alias})
	if err != nil || got.Alias != "Gamma" {
		t.Fatalf("trimmed update: alias=%q err=%v", got.Alias, err)
	}
	taken := "beta"
	if _, err := access.Update(ctx, a.ID, AccessKeyUpdate{Alias: &taken}); !errors.Is(err, ErrAliasTaken) {
		t.Fatalf("conflicting update: got %v want ErrAliasTaken", err)
	}
	same := "gamma"
	off := false
	if got, err := access.Update(ctx, a.ID, AccessKeyUpdate{Alias: &same, IsActive: &off}); err != nil || got.IsActive {
		t.Fatalf("case-only change of own alias: %+v err=%v", got, err)
	}
}

// ResetKey 对不存在的 ID 报错；成功时新 token 立即可用、旧 token 立即失效，且不被后续 Update 覆盖。
func TestAccessKeyService_ResetKey(t *testing.T) {
	t.Parallel()
	access, _ := newAccessKeyTestServices(t)
	ctx := context.Background()

	if _, err := access.ResetKey(ctx, 9999); !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("unknown id: got %v want ErrRecordNotFound", err)
	}

	ak, err := access.Create(ctx, "Reset")
	if err != nil {
		t.Fatal(err)
	}
	token, err := access.ResetKey(ctx, ak.ID)
	if err != nil || token == ak.Key {
		t.Fatalf("reset: token=%q err=%v", token, err)
	}
	disabled := true
	if _, err := access.Update(ctx, ak.ID, AccessKeyUpdate{LoginDisabled: &disabled}); err != nil {
		t.Fatal(err)
	}
	if _, ok := access.Authenticate(ak.Key); ok {
		t.Fatal("old token still valid")
	}
	if got, ok := access.Authenticate(token); !ok || got.ID != ak.ID {
		t.Fatalf("new token must survive a later Update: ok=%v", ok)
	}
}
