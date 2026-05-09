package services

import (
	"context"
	"log/slog"
	"time"

	"tavily-proxy/server/internal/models"

	"gorm.io/gorm"
)

type LogService struct {
	db     *gorm.DB
	logger *slog.Logger
}

func NewLogService(db *gorm.DB, logger *slog.Logger) *LogService {
	return &LogService{db: db, logger: logger}
}

func (s *LogService) Create(ctx context.Context, entry *models.RequestLog) error {
	return s.db.WithContext(ctx).Create(entry).Error
}

type PaginatedLogs struct {
	Items []models.RequestLog `json:"items"`
	Total int64               `json:"total"`
	Page  int                 `json:"page"`
	Size  int                 `json:"page_size"`
}

type StatusCodeCount struct {
	StatusCode int   `json:"status_code"`
	Count      int64 `json:"count"`
}

func (s *LogService) List(ctx context.Context, page, size int, statusCode *int, accessKeyID *uint) (PaginatedLogs, error) {
	if page < 1 {
		page = 1
	}
	if size <= 0 || size > 200 {
		size = 20
	}
	offset := (page - 1) * size

	base := s.db.WithContext(ctx).Model(&models.RequestLog{})
	if statusCode != nil {
		base = base.Where("status_code = ?", *statusCode)
	}
	if accessKeyID != nil {
		base = base.Where("access_key_id = ?", *accessKeyID)
	}

	var total int64
	if err := base.Count(&total).Error; err != nil {
		return PaginatedLogs{}, err
	}

	var logs []models.RequestLog
	query := s.db.WithContext(ctx).
		Model(&models.RequestLog{}).
		Order("id desc").
		Limit(size).
		Offset(offset)
	if statusCode != nil {
		query = query.Where("status_code = ?", *statusCode)
	}
	if accessKeyID != nil {
		query = query.Where("access_key_id = ?", *accessKeyID)
	}
	if err := query.Find(&logs).Error; err != nil {
		return PaginatedLogs{}, err
	}

	return PaginatedLogs{Items: logs, Total: total, Page: page, Size: size}, nil
}

func (s *LogService) StatusCodeCounts(ctx context.Context) ([]StatusCodeCount, error) {
	var out []StatusCodeCount
	if err := s.db.WithContext(ctx).
		Model(&models.RequestLog{}).
		Select("status_code, COUNT(*) as count").
		Group("status_code").
		Order("status_code asc").
		Scan(&out).Error; err != nil {
		return nil, err
	}
	return out, nil
}

func (s *LogService) DeleteOlderThan(ctx context.Context, before time.Time) (int64, error) {
	result := s.db.WithContext(ctx).Where("created_at < ?", before).Delete(&models.RequestLog{})
	return result.RowsAffected, result.Error
}

func (s *LogService) DeleteAll(ctx context.Context) (int64, error) {
	result := s.db.WithContext(ctx).Where("1 = 1").Delete(&models.RequestLog{})
	return result.RowsAffected, result.Error
}
