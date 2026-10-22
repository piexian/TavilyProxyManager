package services

import (
	"context"
	"crypto/subtle"
	"errors"
	"log/slog"
	"strings"
	"sync"
	"time"

	"tavily-proxy/server/internal/models"

	"gorm.io/gorm"
)

// ErrAliasTaken 表示别名已被其它访问密钥占用（去空格、忽略大小写）。
var ErrAliasTaken = errors.New("alias taken")

type AccessKeyService struct {
	db     *gorm.DB
	logger *slog.Logger

	// aliasMu 串行化别名唯一性的「检查 + 写入」，保证进程内 alias 全局唯一。
	aliasMu sync.Mutex

	// touchWG 跟踪 Authenticate 触发的后台 last_used_at 回写，供 Wait 等待。
	touchWG sync.WaitGroup

	mu    sync.RWMutex
	cache []models.AccessKey
}

func NewAccessKeyService(db *gorm.DB, logger *slog.Logger) *AccessKeyService {
	return &AccessKeyService{db: db, logger: logger}
}

func (s *AccessKeyService) LoadCache(ctx context.Context) error {
	var keys []models.AccessKey
	if err := s.db.WithContext(ctx).Where("is_active = ?", true).Find(&keys).Error; err != nil {
		return err
	}
	s.mu.Lock()
	s.cache = keys
	s.mu.Unlock()
	return nil
}

func (s *AccessKeyService) refreshCache(ctx context.Context) error {
	if err := s.LoadCache(ctx); err != nil {
		s.logger.Error("failed to refresh access key cache", "err", err)
		return err
	}
	return nil
}

func (s *AccessKeyService) Authenticate(token string) (*models.AccessKey, bool) {
	if token == "" {
		return nil, false
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	for i := range s.cache {
		if subtle.ConstantTimeCompare([]byte(s.cache[i].Key), []byte(token)) == 1 {
			ak := s.cache[i]
			s.touchWG.Add(1)
			go func() {
				defer s.touchWG.Done()
				s.touchLastUsed(ak.ID)
			}()
			return &ak, true
		}
	}
	return nil, false
}

// Wait 等待所有后台 last_used_at 回写结束。测试须在关闭数据库前调用，
// 否则回写协程可能在临时库被清理后仍在写入，造成偶发失败。
func (s *AccessKeyService) Wait() { s.touchWG.Wait() }

func (s *AccessKeyService) touchLastUsed(id uint) {
	now := time.Now()
	s.db.Model(&models.AccessKey{}).Where("id = ?", id).Update("last_used_at", &now)
}

func (s *AccessKeyService) List(ctx context.Context) ([]models.AccessKey, error) {
	var keys []models.AccessKey
	if err := s.db.WithContext(ctx).Order("id desc").Find(&keys).Error; err != nil {
		return nil, err
	}
	return keys, nil
}

func (s *AccessKeyService) Create(ctx context.Context, alias string) (*models.AccessKey, error) {
	alias = strings.TrimSpace(alias)
	token, err := generateSecret(32)
	if err != nil {
		return nil, err
	}

	s.aliasMu.Lock()
	defer s.aliasMu.Unlock()
	if taken, err := s.AliasExists(ctx, alias, 0); err != nil {
		return nil, err
	} else if taken {
		return nil, ErrAliasTaken
	}

	record := models.AccessKey{
		Key:      token,
		Alias:    alias,
		IsActive: true,
	}
	if err := s.db.WithContext(ctx).Create(&record).Error; err != nil {
		return nil, err
	}
	s.refreshCache(ctx)
	return &record, nil
}

type AccessKeyUpdate struct {
	Alias         *string `json:"alias"`
	IsActive      *bool   `json:"is_active"`
	LoginDisabled *bool   `json:"login_disabled"`
}

func (s *AccessKeyService) Update(ctx context.Context, id uint, upd AccessKeyUpdate) (*models.AccessKey, error) {
	s.aliasMu.Lock()
	defer s.aliasMu.Unlock()

	var key models.AccessKey
	if err := s.db.WithContext(ctx).First(&key, id).Error; err != nil {
		return nil, err
	}

	// 只回写发生变化的字段，不用 Save 整行覆盖：否则会撤销并发的 ResetKey 刚轮换的 token。
	updates := map[string]any{}
	if upd.Alias != nil {
		alias := strings.TrimSpace(*upd.Alias)
		// 仅在 alias 相对当前值变化时才查重：编辑弹窗每次都会带上原 alias，
		// 否则历史重名的访问密钥连启停/门户登录开关都改不了。
		if alias != strings.TrimSpace(key.Alias) {
			if taken, err := s.AliasExists(ctx, alias, id); err != nil {
				return nil, err
			} else if taken {
				return nil, ErrAliasTaken
			}
		}
		if alias != key.Alias {
			updates["alias"] = alias
		}
	}
	if upd.IsActive != nil {
		updates["is_active"] = *upd.IsActive
	}
	if upd.LoginDisabled != nil {
		updates["login_disabled"] = *upd.LoginDisabled
	}
	if len(updates) > 0 {
		if err := s.db.WithContext(ctx).Model(&models.AccessKey{}).Where("id = ?", id).Updates(updates).Error; err != nil {
			return nil, err
		}
		if err := s.db.WithContext(ctx).First(&key, id).Error; err != nil {
			return nil, err
		}
	}
	s.refreshCache(ctx)
	return &key, nil
}

func (s *AccessKeyService) Delete(ctx context.Context, id uint) error {
	if err := s.db.WithContext(ctx).Delete(&models.AccessKey{}, id).Error; err != nil {
		return err
	}
	s.refreshCache(ctx)
	return nil
}

func (s *AccessKeyService) FindByID(ctx context.Context, id uint) (*models.AccessKey, error) {
	var key models.AccessKey
	if err := s.db.WithContext(ctx).First(&key, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &key, nil
}

func (s *AccessKeyService) GetRawKey(ctx context.Context, id uint) (string, error) {
	var key models.AccessKey
	if err := s.db.WithContext(ctx).First(&key, id).Error; err != nil {
		return "", err
	}
	return key.Key, nil
}

// AliasExists 判断别名是否已被占用（去空格、忽略大小写）；excludeID 用于更新时排除自身（传 0 表示不排除）。
// 在 Go 侧用 EqualFold 比较：SQLite 的 LOWER 只处理 ASCII，非 ASCII 字母的大小写变体会漏判。
func (s *AccessKeyService) AliasExists(ctx context.Context, alias string, excludeID uint) (bool, error) {
	alias = strings.TrimSpace(alias)
	if alias == "" {
		return false, nil
	}
	var aliases []string
	if err := s.db.WithContext(ctx).Model(&models.AccessKey{}).
		Where("id <> ?", excludeID).
		Pluck("alias", &aliases).Error; err != nil {
		return false, err
	}
	for _, existing := range aliases {
		if strings.EqualFold(strings.TrimSpace(existing), alias) {
			return true, nil
		}
	}
	return false, nil
}

// ResetKey 轮换访问密钥的 token：ID/Alias/IsActive 及名下 Key 的归属(access_key_id)全部不变，
// 仅替换 Key 原文并刷新缓存，使旧 token 立即失效、新 token 立即生效。
func (s *AccessKeyService) ResetKey(ctx context.Context, id uint) (string, error) {
	token, err := generateSecret(32)
	if err != nil {
		return "", err
	}
	result := s.db.WithContext(ctx).Model(&models.AccessKey{}).Where("id = ?", id).Update("key", token)
	if result.Error != nil {
		return "", result.Error
	}
	if result.RowsAffected == 0 {
		return "", gorm.ErrRecordNotFound
	}
	// 缓存刷新失败时不能把新 token 交给调用方：它只显示一次，而缓存里仍是旧 token，会立刻 401。
	if err := s.refreshCache(ctx); err != nil {
		return "", err
	}
	return token, nil
}
