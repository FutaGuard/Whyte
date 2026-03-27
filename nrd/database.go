package nrd

import (
	"fmt"
	"strings"
	"time"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

type DatabaseConfig struct {
	Host     string
	Port     int
	User     string
	Password string
	DBName   string
	SSLMode  string
}

// InitDB 初始化資料庫連接
func InitDB(config DatabaseConfig) (*gorm.DB, error) {
	dsn := fmt.Sprintf("host=%s user=%s password=%s dbname=%s port=%d sslmode=%s",
		config.Host, config.User, config.Password, config.DBName, config.Port, config.SSLMode)

	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Info),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to connect to database: %w", err)
	}

	if err := db.AutoMigrate(&Domain{}); err != nil {
		return nil, fmt.Errorf("failed to migrate database: %w", err)
	}

	return db, nil
}

// CleanupOldDomains 刪除超過 366 天的域名記錄
func CleanupOldDomains(db *gorm.DB) error {
	cutoffDate := time.Now().AddDate(0, 0, -366)
	result := db.Where("updated_at < ?", cutoffDate).Delete(&Domain{})
	if result.Error != nil {
		return fmt.Errorf("failed to cleanup old domains: %w", result.Error)
	}

	// 記錄刪除的數量
	if result.RowsAffected > 0 {
		fmt.Printf("Deleted %d domains older than 366 days\n", result.RowsAffected)
	}

	return nil
}

// BatchInsertDomains 批量插入域名，使用批量 UPSERT
// 使用單一 SQL 語句批量插入，讓資料庫端處理去重
func BatchInsertDomains(db *gorm.DB, domains []Domain, batchSize int) (int64, error) {
	if len(domains) == 0 {
		return 0, nil
	}

	var totalAffected int64
	for i := 0; i < len(domains); i += batchSize {
		end := i + batchSize
		if end > len(domains) {
			end = len(domains)
		}

		batch := domains[i:end]

		// 構建批量插入的 SQL
		// INSERT INTO nrd_domains (domain_name, tld, updated_at) VALUES
		// ('domain1', 'com', '2026-03-27'), ('domain2', 'net', '2026-03-27'), ...
		// ON CONFLICT (domain_name) DO UPDATE SET updated_at = EXCLUDED.updated_at, tld = EXCLUDED.tld

		if len(batch) == 0 {
			continue
		}

		// 構建 VALUES 子句
		valueStrings := make([]string, 0, len(batch))
		valueArgs := make([]interface{}, 0, len(batch)*3)

		for _, domain := range batch {
			valueStrings = append(valueStrings, "(?, ?, ?)")
			valueArgs = append(valueArgs, domain.DomainName, domain.TLD, domain.UpdatedAt)
		}

		// 組合完整的 SQL 語句
		query := fmt.Sprintf(`
			INSERT INTO nrd_domains (domain_name, tld, updated_at) 
			VALUES %s 
			ON CONFLICT (domain_name) 
			DO UPDATE SET updated_at = EXCLUDED.updated_at, tld = EXCLUDED.tld
		`, strings.Join(valueStrings, ", "))

		result := db.Exec(query, valueArgs...)
		if result.Error != nil {
			return totalAffected, fmt.Errorf("failed to batch insert %d domains: %w", len(batch), result.Error)
		}

		totalAffected += result.RowsAffected
	}

	return totalAffected, nil
}

// GetDomainsByDateRange 根據日期範圍查詢域名（使用 updated_at）
func GetDomainsByDateRange(db *gorm.DB, startDate, endDate time.Time) ([]Domain, error) {
	var domains []Domain
	result := db.Where("updated_at >= ? AND updated_at <= ?", startDate, endDate).
		Order("updated_at DESC").
		Find(&domains)

	if result.Error != nil {
		return nil, fmt.Errorf("failed to query domains: %w", result.Error)
	}

	return domains, nil
}

// GetDomainCount 獲取指定日期範圍內的域名數量
func GetDomainCount(db *gorm.DB, startDate, endDate time.Time) (int64, error) {
	var count int64
	result := db.Model(&Domain{}).
		Where("updated_at >= ? AND updated_at <= ?", startDate, endDate).
		Count(&count)

	if result.Error != nil {
		return 0, fmt.Errorf("failed to count domains: %w", result.Error)
	}

	return count, nil
}

// OptimizeDatabase 執行資料庫優化
func OptimizeDatabase(db *gorm.DB) error {
	// VACUUM ANALYZE 可以回收空間並更新統計資訊
	if err := db.Exec("VACUUM ANALYZE nrd_domains").Error; err != nil {
		return fmt.Errorf("failed to vacuum: %w", err)
	}

	// 重建索引以提升查詢效能
	if err := db.Exec("REINDEX TABLE nrd_domains").Error; err != nil {
		return fmt.Errorf("failed to reindex: %w", err)
	}

	return nil
}
