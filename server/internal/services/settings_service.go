package services

import (
	"context"
	"strconv"
	"strings"
	"sync"
	"time"

	"tavily-proxy/server/internal/models"

	"gorm.io/gorm"
)

const settingsCacheTTL = 30 * time.Second

type cachedSetting struct {
	value  string
	exists bool
	expiry time.Time
}

type SettingsService struct {
	db    *gorm.DB
	mu    sync.RWMutex
	cache map[string]cachedSetting
}

func NewSettingsService(db *gorm.DB) *SettingsService {
	return &SettingsService{db: db, cache: make(map[string]cachedSetting)}
}

func (s *SettingsService) Get(ctx context.Context, key string) (string, bool, error) {
	// 配置极少变动但每个代理请求都要读，加 30s 内存缓存避免反复打 DB
	s.mu.RLock()
	if c, ok := s.cache[key]; ok && time.Now().Before(c.expiry) {
		s.mu.RUnlock()
		return c.value, c.exists, nil
	}
	s.mu.RUnlock()

	var setting models.Setting
	tx := s.db.WithContext(ctx).Where("key = ?", key).Limit(1).Find(&setting)
	if tx.Error != nil {
		return "", false, tx.Error
	}
	exists := tx.RowsAffected > 0

	s.mu.Lock()
	s.cache[key] = cachedSetting{value: setting.Value, exists: exists, expiry: time.Now().Add(settingsCacheTTL)}
	s.mu.Unlock()

	return setting.Value, exists, nil
}

func (s *SettingsService) Set(ctx context.Context, key, value string) error {
	if err := s.db.WithContext(ctx).Save(&models.Setting{Key: key, Value: value}).Error; err != nil {
		return err
	}
	s.mu.Lock()
	s.cache[key] = cachedSetting{value: value, exists: true, expiry: time.Now().Add(settingsCacheTTL)}
	s.mu.Unlock()
	return nil
}

func (s *SettingsService) GetBool(ctx context.Context, key string, def bool) (bool, error) {
	v, ok, err := s.Get(ctx, key)
	if err != nil || !ok {
		return def, err
	}
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "1", "true", "yes", "y", "on":
		return true, nil
	case "0", "false", "no", "n", "off":
		return false, nil
	default:
		return def, nil
	}
}

func (s *SettingsService) SetBool(ctx context.Context, key string, value bool) error {
	if value {
		return s.Set(ctx, key, "true")
	}
	return s.Set(ctx, key, "false")
}

func (s *SettingsService) GetInt(ctx context.Context, key string, def int) (int, error) {
	v, ok, err := s.Get(ctx, key)
	if err != nil || !ok {
		return def, err
	}
	i, err := strconv.Atoi(strings.TrimSpace(v))
	if err != nil {
		return def, nil
	}
	return i, nil
}

func (s *SettingsService) SetInt(ctx context.Context, key string, value int) error {
	return s.Set(ctx, key, strconv.Itoa(value))
}

func (s *SettingsService) GetTime(ctx context.Context, key string) (*time.Time, error) {
	v, ok, err := s.Get(ctx, key)
	if err != nil || !ok {
		return nil, err
	}
	t, err := time.Parse(time.RFC3339, strings.TrimSpace(v))
	if err != nil {
		return nil, nil
	}
	return &t, nil
}

func (s *SettingsService) SetTime(ctx context.Context, key string, value time.Time) error {
	return s.Set(ctx, key, value.UTC().Format(time.RFC3339))
}

// Ping 探测 DB 读写是否正常。写一行专用记录，可发现只读/被移走等只在写时暴露的故障。
func (s *SettingsService) Ping(ctx context.Context) error {
	return s.db.WithContext(ctx).Save(&models.Setting{Key: "health_check_at", Value: time.Now().UTC().Format(time.RFC3339)}).Error
}
