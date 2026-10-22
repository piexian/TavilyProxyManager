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

// CountByAccessKey 一次查询统计某访问密钥发起的请求数：日志内总数，以及 since 起的数量（用于"今日"）。
// 数据来自 request_logs，受日志保留期与「请求日志」开关影响。
func (s *LogService) CountByAccessKey(ctx context.Context, accessKeyID uint, since time.Time) (total, sinceCount int64, err error) {
	var row struct {
		Total  int64 `gorm:"column:total"`
		Recent int64 `gorm:"column:recent"`
	}
	err = s.db.WithContext(ctx).Model(&models.RequestLog{}).
		Select("COUNT(*) AS total, COALESCE(SUM(CASE WHEN created_at >= ? THEN 1 ELSE 0 END), 0) AS recent", since).
		Where("access_key_id = ?", accessKeyID).
		Scan(&row).Error
	return row.Total, row.Recent, err
}
