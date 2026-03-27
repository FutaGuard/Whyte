package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/tdc/whyte/nrd"
	"gorm.io/gorm"
)

func main() {
	configPath := flag.String("config", "config.yaml", "Path to config file")
	mode := flag.String("mode", "server", "Mode: server, download, cleanup")
	date := flag.String("date", "", "Date for download mode (YYYY-MM-DD)")
	days := flag.Int("days", 1, "Number of days to download (for download mode)")
	port := flag.String("port", "8080", "API server port")
	flag.Parse()

	// 載入配置
	config, err := nrd.LoadConfig(*configPath)
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	// 初始化資料庫
	db, err := nrd.InitDB(config.Database)
	if err != nil {
		log.Fatalf("Failed to initialize database: %v", err)
	}

	processor := nrd.NewProcessor(db, config)

	switch *mode {
	case "server":
		runServer(db, *port, config)
	case "download":
		runDownload(processor, *date, *days)
	case "cleanup":
		runCleanup(processor)
	default:
		log.Fatalf("Unknown mode: %s", *mode)
	}
}

func runServer(db *gorm.DB, port string, config *nrd.Config) {
	server := nrd.NewAPIServer(db)
	processor := nrd.NewProcessor(db, config)

	// 啟動定時下載任務（每 6 小時執行一次）
	go func() {
		ticker := time.NewTicker(6 * time.Hour)
		defer ticker.Stop()

		log.Println("NRD download scheduler started (runs every 6 hours)")

		// 立即執行一次下載
		go func() {
			log.Println("Starting initial NRD download...")
			ctx := context.Background()
			yesterday := time.Now().AddDate(0, 0, -1)
			if err := processor.ProcessDate(ctx, yesterday); err != nil {
				log.Printf("Initial download error: %v", err)
			} else {
				log.Println("Initial download completed")
			}
		}()

		// 定時執行下載
		for range ticker.C {
			log.Println("Starting scheduled NRD download...")
			ctx := context.Background()
			yesterday := time.Now().AddDate(0, 0, -1)
			if err := processor.ProcessDate(ctx, yesterday); err != nil {
				log.Printf("Scheduled download error: %v", err)
			} else {
				log.Println("Scheduled download completed")
			}
		}
	}()

	// 啟動定時清理任務（每 24 小時執行一次）
	go func() {
		ticker := time.NewTicker(24 * time.Hour)
		defer ticker.Stop()

		log.Println("Cleanup scheduler started (runs every 24 hours)")

		for range ticker.C {
			log.Println("Starting cleanup of old data...")
			if err := processor.CleanupOldData(); err != nil {
				log.Printf("Cleanup error: %v", err)
			} else {
				log.Println("Cleanup completed")
			}
		}
	}()

	// 處理優雅關閉
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	go func() {
		<-sigChan
		fmt.Println("\nShutting down server...")
		os.Exit(0)
	}()

	addr := ":" + port
	log.Printf("Starting NRD API server on %s", addr)
	if err := server.Start(addr); err != nil {
		log.Fatalf("Server error: %v", err)
	}
}

func runDownload(processor *nrd.Processor, dateStr string, days int) {
	ctx := context.Background()

	var startDate time.Time
	var err error

	if dateStr == "" {
		// 預設下載昨天的資料
		startDate = time.Now().AddDate(0, 0, -1)
	} else {
		startDate, err = time.Parse("2006-01-02", dateStr)
		if err != nil {
			log.Fatalf("Invalid date format: %v", err)
		}
	}

	endDate := startDate.AddDate(0, 0, days-1)

	fmt.Printf("Downloading NRD data from %s to %s\n",
		startDate.Format("2006-01-02"),
		endDate.Format("2006-01-02"))

	if err := processor.ProcessDateRange(ctx, startDate, endDate); err != nil {
		log.Fatalf("Download failed: %v", err)
	}

	fmt.Println("Download completed successfully!")
}

func runCleanup(processor *nrd.Processor) {
	fmt.Println("Running cleanup...")
	if err := processor.CleanupOldData(); err != nil {
		log.Fatalf("Cleanup failed: %v", err)
	}
	fmt.Println("Cleanup completed successfully!")
}
