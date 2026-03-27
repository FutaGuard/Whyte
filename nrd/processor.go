package nrd

import (
	"context"
	"fmt"
	"sync"
	"time"

	"gorm.io/gorm"
)

// Processor 處理 NRD 資料的主要邏輯
type Processor struct {
	db         *gorm.DB
	downloader *Downloader
	config     *Config
}

// NewProcessor 建立新的處理器
func NewProcessor(db *gorm.DB, config *Config) *Processor {
	return &Processor{
		db:         db,
		downloader: NewDownloader(config),
		config:     config,
	}
}

// ProcessDate 處理指定日期的 NRD 資料
func (p *Processor) ProcessDate(ctx context.Context, date time.Time) error {
	fmt.Printf("Processing NRD data for %s\n", date.Format("2006-01-02"))

	allDomains := make(map[string]bool)
	var mu sync.Mutex

	// Phase 1: whoisds.com
	fmt.Println("\n=== Downloading Phase 1 (whoisds.com) ===")
	if domains, err := p.downloader.DownloadPhase1(ctx, date); err == nil {
		mu.Lock()
		for _, d := range domains {
			allDomains[d] = true
		}
		mu.Unlock()
		fmt.Printf("  ✓ Phase 1: %d domains\n", len(domains))
	} else {
		fmt.Printf("  ✗ Phase 1 failed: %v\n", err)
	}

	// Phase 2: GitHub shreshta-labs (固定檔名)
	fmt.Println("\n=== Downloading Phase 2 (GitHub shreshta-labs) ===")
	phase2Files := []string{"nrd-1w.csv", "nrd-1m.csv"}
	for _, filename := range phase2Files {
		fmt.Printf("[Phase2/%s] Starting download...\n", filename)
		if domains, err := p.downloader.DownloadPhase2(ctx, filename); err == nil {
			mu.Lock()
			for _, d := range domains {
				allDomains[d] = true
			}
			mu.Unlock()
			fmt.Printf("  ✓ Phase 2 (%s): %d domains\n", filename, len(domains))
		} else {
			fmt.Printf("  ✗ Phase 2 (%s) failed: %v\n", filename, err)
		}
	}

	// Phase 3: whoisfreaks.com
	fmt.Println("\n=== Downloading Phase 3 (whoisfreaks.com) ===")
	if domains, err := p.downloader.DownloadPhase3(ctx); err == nil {
		mu.Lock()
		for _, d := range domains {
			allDomains[d] = true
		}
		mu.Unlock()
		fmt.Printf("  ✓ Phase 3: %d domains\n", len(domains))
	} else {
		fmt.Printf("  ✗ Phase 3 failed: %v\n", err)
	}

	// Phase 4: hagezi dns-blocklists (嘗試多個檔案)
	fmt.Println("\n=== Downloading Phase 4 (hagezi dns-blocklists) ===")
	phase4Files := []string{"nrd7", "nrd14-8", "nrd21-15", "nrd28-22", "nrd35-29"}
	for _, filename := range phase4Files {
		fmt.Printf("[Phase4/%s] Starting download...\n", filename)
		if domains, err := p.downloader.DownloadPhase4(ctx, filename); err == nil {
			mu.Lock()
			for _, d := range domains {
				allDomains[d] = true
			}
			mu.Unlock()
			fmt.Printf("[Phase4/%s] ✓ Downloaded %d domains\n", filename, len(domains))
			fmt.Printf("  ✓ Phase 4 (%s): %d domains\n", filename, len(domains))
		} else {
			fmt.Printf("  ✗ Phase 4 (%s) failed: %v\n", filename, err)
		}
	}

	// 轉換為 Domain 結構並寫入資料庫
	fmt.Printf("\nTotal unique domains: %d\n", len(allDomains))

	if len(allDomains) == 0 {
		return fmt.Errorf("no domains downloaded")
	}

	domains := make([]Domain, 0, len(allDomains))
	now := time.Now()
	for domainName := range allDomains {
		domains = append(domains, Domain{
			DomainName: domainName,
			TLD:        ExtractTLD(domainName),
			UpdatedAt:  now,
		})
	}

	fmt.Println("Inserting domains into database...")
	inserted, err := BatchInsertDomains(p.db, domains, 5000)
	if err != nil {
		return fmt.Errorf("failed to insert domains: %w", err)
	}

	fmt.Printf("✓ Inserted %d new domains (skipped %d duplicates)\n", inserted, len(domains)-int(inserted))
	return nil
}

// ProcessDateRange 處理日期範圍內的 NRD 資料
func (p *Processor) ProcessDateRange(ctx context.Context, startDate, endDate time.Time) error {
	currentDate := startDate
	for currentDate.Before(endDate) || currentDate.Equal(endDate) {
		if err := p.ProcessDate(ctx, currentDate); err != nil {
			fmt.Printf("Error processing %s: %v\n", currentDate.Format("2006-01-02"), err)
		}
		currentDate = currentDate.AddDate(0, 0, 1)
	}
	return nil
}

// CleanupOldData 清理超過一年的資料
func (p *Processor) CleanupOldData() error {
	fmt.Println("Cleaning up domains older than 1 year...")
	if err := CleanupOldDomains(p.db); err != nil {
		return err
	}
	fmt.Println("✓ Cleanup completed")
	return nil
}
