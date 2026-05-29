package db

import (
	"fmt"
	"os"
	"path/filepath"

	"tavily-proxy/server/internal/models"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func Open(path string) (*gorm.DB, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}

	// WAL + busy_timeout 避免读写互斥和瞬时锁失败；synchronous=NORMAL 配合 WAL 更快；
	// _txlock=immediate 在 BEGIN 时即取写锁，避免事务中途升级锁失败
	dsn := path + "?" +
		"_pragma=busy_timeout(5000)" +
		"&_pragma=journal_mode(WAL)" +
		"&_pragma=synchronous(NORMAL)" +
		"&_pragma=foreign_keys(1)" +
		"&_txlock=immediate"

	database, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		return nil, err
	}

	// 启动时做完整性检查，DB 损坏或被移走时直接报错而非静默写失败
	if err := integrityCheck(database); err != nil {
		return nil, err
	}

	// SQLite 单写多读，限制连接数以控制锁竞争
	if sqlDB, err := database.DB(); err == nil {
		sqlDB.SetMaxOpenConns(8)
	}

	if err := database.AutoMigrate(&models.APIKey{}, &models.AccessKey{}, &models.RequestLog{}, &models.RequestStat{}, &models.Setting{}, &models.SearchCache{}); err != nil {
		return nil, err
	}
	return database, nil
}

func integrityCheck(database *gorm.DB) error {
	var result string
	if err := database.Raw("PRAGMA quick_check").Scan(&result).Error; err != nil {
		return fmt.Errorf("integrity check failed to run: %w", err)
	}
	if result != "ok" {
		return fmt.Errorf("database integrity check failed: %s", result)
	}
	return nil
}
