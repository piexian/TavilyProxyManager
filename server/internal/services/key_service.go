package services

import (
	"context"
	"errors"
	"log/slog"
	"math/rand"
	"sort"
	"strings"
	"sync/atomic"
	"time"

	"tavily-proxy/server/internal/models"

	"gorm.io/gorm"
)

const (
	StrategyBestRemaining  = "best_remaining"
	StrategyRoundRobin     = "round_robin"
	StrategySequentialFill = "sequential_fill"
)

type KeyService struct {
	db           *gorm.DB
	logger       *slog.Logger
	settings     *SettingsService
	rrCounter    atomic.Uint64
	seqLastKeyID atomic.Uint64
}

type PoolQuotaSummary struct {
	TotalQuota     int64 `json:"total_quota"`
	TotalUsed      int64 `json:"total_used"`
	TotalRemaining int64 `json:"total_remaining"`
	ActiveKeyCount int64 `json:"active_key_count"`
}

func NewKeyService(db *gorm.DB, logger *slog.Logger) *KeyService {
	return &KeyService{db: db, logger: logger}
}

func (s *KeyService) WithSettings(settings *SettingsService) *KeyService {
	s.settings = settings
	return s
}

func (s *KeyService) getStrategy(ctx context.Context) string {
	if s.settings == nil {
		return StrategyBestRemaining
	}
	v, ok, err := s.settings.Get(ctx, SettingKeyPoolStrategy)
	if err != nil || !ok {
		return StrategyBestRemaining
	}
	switch v {
	case StrategyRoundRobin, StrategySequentialFill:
		return v
	default:
		return StrategyBestRemaining
	}
}

func (s *KeyService) List(ctx context.Context) ([]models.APIKey, error) {
	var keys []models.APIKey
	if err := s.db.WithContext(ctx).Order("id desc").Find(&keys).Error; err != nil {
		return nil, err
	}
	return keys, nil
}

func (s *KeyService) Create(ctx context.Context, key, alias string, totalQuota int) (*models.APIKey, error) {
	if totalQuota <= 0 {
		totalQuota = 1000
	}

	// Upsert: if key already exists, update alias (when provided) and return.
	var existing models.APIKey
	if err := s.db.WithContext(ctx).Where("`key` = ?", key).First(&existing).Error; err == nil {
		if alias != "" && alias != "Default" {
			existing.Alias = alias
			if err := s.db.WithContext(ctx).Save(&existing).Error; err != nil {
				return nil, err
			}
		}
		return &existing, nil
	}

	record := models.APIKey{
		Key:        key,
		Alias:      alias,
		TotalQuota: totalQuota,
		UsedQuota:  0,
		IsActive:   true,
		IsInvalid:  false,
	}
	if err := s.db.WithContext(ctx).Create(&record).Error; err != nil {
		return nil, err
	}
	return &record, nil
}

// CreateOrUpdate inserts a new key or updates the alias of an existing one.
// Returns the record and a boolean indicating whether it was newly created (true) or updated (false).
func (s *KeyService) CreateOrUpdate(ctx context.Context, key, alias string, totalQuota int) (*models.APIKey, bool, error) {
	if totalQuota <= 0 {
		totalQuota = 1000
	}

	var existing models.APIKey
	err := s.db.WithContext(ctx).Where("`key` = ?", key).First(&existing).Error
	if err == nil {
		// Key exists — update alias if provided and different.
		if alias != "" && alias != "Default" && existing.Alias != alias {
			existing.Alias = alias
			if err := s.db.WithContext(ctx).Save(&existing).Error; err != nil {
				return nil, false, err
			}
		}
		return &existing, false, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, false, err
	}

	// Key does not exist — create.
	record := models.APIKey{
		Key:        key,
		Alias:      alias,
		TotalQuota: totalQuota,
		IsActive:   true,
	}
	if err := s.db.WithContext(ctx).Create(&record).Error; err != nil {
		return nil, false, err
	}
	return &record, true, nil
}

// DonateKey creates a new key with is_active=false and is_donated=true.
// Does not modify existing keys. Returns the record (nil if duplicate).
func (s *KeyService) DonateKey(ctx context.Context, key, alias string) (*models.APIKey, bool, error) {
	key = strings.TrimSpace(key)
	alias = strings.TrimSpace(alias)
	if alias == "" {
		alias = "Donation"
	}

	var existing models.APIKey
	err := s.db.WithContext(ctx).Where("`key` = ?", key).First(&existing).Error
	if err == nil {
		return &existing, false, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, false, err
	}

	record := models.APIKey{
		Key:        key,
		Alias:      alias,
		TotalQuota: 1000,
		IsActive:   false,
		IsDonated:  true,
	}
	if err := s.db.WithContext(ctx).Create(&record).Error; err != nil {
		var check models.APIKey
		if s.db.WithContext(ctx).Where("`key` = ?", key).First(&check).Error == nil {
			return &check, false, nil
		}
		return nil, false, err
	}
	return &record, true, nil
}

// ActivateKey sets is_active=true for the given key ID.
func (s *KeyService) ActivateKey(ctx context.Context, id uint) error {
	return s.db.WithContext(ctx).Model(&models.APIKey{}).Where("id = ?", id).Update("is_active", true).Error
}

// QueryByKey finds a key record by its raw key value.
func (s *KeyService) QueryByKey(ctx context.Context, key string) (*models.APIKey, error) {
	key = strings.TrimSpace(key)
	if key == "" {
		return nil, nil
	}

	var record models.APIKey
	err := s.db.WithContext(ctx).Where("`key` = ?", key).First(&record).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &record, nil
}

func (s *KeyService) Get(ctx context.Context, id uint) (*models.APIKey, error) {
	var key models.APIKey
	if err := s.db.WithContext(ctx).First(&key, id).Error; err != nil {
		return nil, err
	}
	return &key, nil
}

type KeyUpdate struct {
	Alias      *string `json:"alias"`
	TotalQuota *int    `json:"total_quota"`
	UsedQuota  *int    `json:"used_quota"`
	IsActive   *bool   `json:"is_active"`
	ResetQuota bool    `json:"reset_quota"`
	SyncUsage  bool    `json:"sync_usage"`
}

func (s *KeyService) Update(ctx context.Context, id uint, upd KeyUpdate) (*models.APIKey, error) {
	var key models.APIKey
	if err := s.db.WithContext(ctx).First(&key, id).Error; err != nil {
		return nil, err
	}
	if upd.Alias != nil {
		key.Alias = *upd.Alias
	}
	if upd.TotalQuota != nil && *upd.TotalQuota > 0 {
		key.TotalQuota = *upd.TotalQuota
	}
	if upd.UsedQuota != nil && *upd.UsedQuota >= 0 {
		key.UsedQuota = *upd.UsedQuota
	}
	if upd.IsActive != nil {
		if key.IsInvalid && *upd.IsActive {
			key.IsActive = false
		} else {
			key.IsActive = *upd.IsActive
		}
	}
	if upd.ResetQuota {
		key.UsedQuota = 0
	}

	if err := s.db.WithContext(ctx).Save(&key).Error; err != nil {
		return nil, err
	}
	return &key, nil
}

func (s *KeyService) Delete(ctx context.Context, id uint) error {
	return s.db.WithContext(ctx).Delete(&models.APIKey{}, id).Error
}

func (s *KeyService) MarkInactive(ctx context.Context, id uint) error {
	return s.db.WithContext(ctx).Model(&models.APIKey{}).Where("id = ?", id).Update("is_active", false).Error
}

func (s *KeyService) MarkInvalid(ctx context.Context, id uint) error {
	return s.db.WithContext(ctx).Model(&models.APIKey{}).Where("id = ?", id).Updates(map[string]any{
		"is_active":  false,
		"is_invalid": true,
	}).Error
}

func (s *KeyService) MarkExhausted(ctx context.Context, id uint) error {
	return s.db.WithContext(ctx).Model(&models.APIKey{}).
		Where("id = ?", id).
		Update("used_quota", gorm.Expr("total_quota")).Error
}

func (s *KeyService) IncrementUsed(ctx context.Context, id uint, credits int) error {
	if credits < 1 {
		credits = 1
	}
	now := time.Now()
	return s.db.WithContext(ctx).Model(&models.APIKey{}).Where("id = ?", id).Updates(map[string]any{
		"used_quota":   gorm.Expr("CASE WHEN used_quota + ? > total_quota THEN total_quota ELSE used_quota + ? END", credits, credits),
		"last_used_at": &now,
	}).Error
}

func (s *KeyService) ResetAllUsage(ctx context.Context) error {
	return s.db.WithContext(ctx).Model(&models.APIKey{}).Update("used_quota", 0).Error
}

func (s *KeyService) SetUsage(ctx context.Context, id uint, used int, total *int) error {
	updates := map[string]any{
		"used_quota": used,
	}
	if total != nil && *total > 0 {
		updates["total_quota"] = *total
		if used > *total {
			updates["used_quota"] = *total
		}
	}
	return s.db.WithContext(ctx).Model(&models.APIKey{}).Where("id = ?", id).Updates(updates).Error
}

func (s *KeyService) Candidates(ctx context.Context) ([]models.APIKey, error) {
	var keys []models.APIKey
	if err := s.db.WithContext(ctx).
		Where("is_active = ? AND is_invalid = ? AND used_quota < total_quota", true, false).
		Find(&keys).Error; err != nil {
		return nil, err
	}
	if len(keys) == 0 {
		return nil, nil
	}

	switch s.getStrategy(ctx) {
	case StrategyRoundRobin:
		return s.candidatesRoundRobin(keys), nil
	case StrategySequentialFill:
		return s.candidatesSequentialFill(keys), nil
	default:
		return s.candidatesBestRemaining(keys), nil
	}
}

func (s *KeyService) candidatesBestRemaining(keys []models.APIKey) []models.APIKey {
	type scored struct {
		key       models.APIKey
		remaining int
	}
	scoredKeys := make([]scored, 0, len(keys))
	for _, k := range keys {
		scoredKeys = append(scoredKeys, scored{key: k, remaining: k.TotalQuota - k.UsedQuota})
	}

	sort.Slice(scoredKeys, func(i, j int) bool {
		return scoredKeys[i].remaining > scoredKeys[j].remaining
	})

	// Shuffle ties for fairness.
	rng := rand.New(rand.NewSource(time.Now().UnixNano()))
	out := make([]models.APIKey, 0, len(scoredKeys))
	for i := 0; i < len(scoredKeys); {
		j := i + 1
		for j < len(scoredKeys) && scoredKeys[j].remaining == scoredKeys[i].remaining {
			j++
		}
		group := scoredKeys[i:j]
		rng.Shuffle(len(group), func(a, b int) { group[a], group[b] = group[b], group[a] })
		for _, item := range group {
			out = append(out, item.key)
		}
		i = j
	}
	return out
}

func (s *KeyService) candidatesRoundRobin(keys []models.APIKey) []models.APIKey {
	n := s.rrCounter.Add(1)
	start := int(n) % len(keys)
	out := make([]models.APIKey, len(keys))
	for i := 0; i < len(keys); i++ {
		out[i] = keys[(start+i)%len(keys)]
	}
	return out
}

func (s *KeyService) candidatesSequentialFill(keys []models.APIKey) []models.APIKey {
	sort.Slice(keys, func(i, j int) bool { return keys[i].ID < keys[j].ID })

	lastID := uint(s.seqLastKeyID.Load())
	targetIdx := 0
	for i, k := range keys {
		if k.ID == lastID {
			targetIdx = i
			break
		}
	}

	out := make([]models.APIKey, 0, len(keys))
	out = append(out, keys[targetIdx])
	for i := targetIdx + 1; i < len(keys); i++ {
		out = append(out, keys[i])
	}
	for i := 0; i < targetIdx; i++ {
		out = append(out, keys[i])
	}

	s.seqLastKeyID.Store(uint64(out[0].ID))
	return out
}

func (s *KeyService) PoolQuotaSummary(ctx context.Context) (PoolQuotaSummary, error) {
	var keys []models.APIKey
	if err := s.db.WithContext(ctx).
		Where("is_active = ? AND is_invalid = ?", true, false).
		Find(&keys).Error; err != nil {
		return PoolQuotaSummary{}, err
	}

	var summary PoolQuotaSummary
	summary.ActiveKeyCount = int64(len(keys))
	for _, key := range keys {
		total := key.TotalQuota
		if total < 0 {
			total = 0
		}

		used := key.UsedQuota
		if used < 0 {
			used = 0
		}
		if used > total {
			used = total
		}

		summary.TotalQuota += int64(total)
		summary.TotalUsed += int64(used)
		summary.TotalRemaining += int64(total - used)
	}

	return summary, nil
}

func (s *KeyService) FindByID(ctx context.Context, id uint) (*models.APIKey, error) {
	var key models.APIKey
	if err := s.db.WithContext(ctx).First(&key, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &key, nil
}

func (s *KeyService) DeleteInvalid(ctx context.Context) (int64, error) {
	result := s.db.WithContext(ctx).Where("is_invalid = ?", true).Delete(&models.APIKey{})
	return result.RowsAffected, result.Error
}
