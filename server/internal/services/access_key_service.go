package services

import (
	"context"
	"crypto/subtle"
	"errors"
	"log/slog"
	"sync"
	"time"

	"tavily-proxy/server/internal/models"

	"gorm.io/gorm"
)

type AccessKeyService struct {
	db     *gorm.DB
	logger *slog.Logger

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

func (s *AccessKeyService) refreshCache(ctx context.Context) {
	var keys []models.AccessKey
	if err := s.db.WithContext(ctx).Where("is_active = ?", true).Find(&keys).Error; err != nil {
		s.logger.Error("failed to refresh access key cache", "err", err)
		return
	}
	s.mu.Lock()
	s.cache = keys
	s.mu.Unlock()
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
			go s.touchLastUsed(ak.ID)
			return &ak, true
		}
	}
	return nil, false
}

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
	token, err := generateSecret(32)
	if err != nil {
		return nil, err
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
	Alias    *string `json:"alias"`
	IsActive *bool   `json:"is_active"`
}

func (s *AccessKeyService) Update(ctx context.Context, id uint, upd AccessKeyUpdate) (*models.AccessKey, error) {
	var key models.AccessKey
	if err := s.db.WithContext(ctx).First(&key, id).Error; err != nil {
		return nil, err
	}
	if upd.Alias != nil {
		key.Alias = *upd.Alias
	}
	if upd.IsActive != nil {
		key.IsActive = *upd.IsActive
	}
	if err := s.db.WithContext(ctx).Save(&key).Error; err != nil {
		return nil, err
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

// FindByAlias returns the first access key matching the given alias, or nil if none exists.
func (s *AccessKeyService) FindByAlias(ctx context.Context, alias string) (*models.AccessKey, error) {
	var key models.AccessKey
	if err := s.db.WithContext(ctx).Where("alias = ?", alias).First(&key).Error; err != nil {
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
