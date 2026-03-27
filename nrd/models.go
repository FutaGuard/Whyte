package nrd

import (
	"time"
)

// Domain 儲存 NRD 域名資料
type Domain struct {
	ID         uint      `gorm:"primarykey"`
	DomainName string    `gorm:"uniqueIndex;size:255;not null" json:"domain_name"`
	TLD        string    `gorm:"index:idx_tld;size:63;not null" json:"tld"`
	UpdatedAt  time.Time `gorm:"index:idx_updated_at;not null" json:"updated_at"`
}

// TableName 指定表名
func (Domain) TableName() string {
	return "nrd_domains"
}

// DomainStats 統計資訊
type DomainStats struct {
	TLD   string `json:"tld"`
	Count int64  `json:"count"`
	Date  string `json:"date"`
}
