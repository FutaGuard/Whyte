package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/lanrat/czds"
	"gopkg.in/yaml.v3"
)

type Config struct {
	ICANN struct {
		Username string `yaml:"username"`
		Password string `yaml:"password"`
	} `yaml:"icann"`
}

func main() {
	configData, err := os.ReadFile("config.yaml")
	if err != nil {
		log.Fatalf("Failed to read config: %v", err)
	}

	var config Config
	if err := yaml.Unmarshal(configData, &config); err != nil {
		log.Fatalf("Failed to parse config: %v", err)
	}

	client := czds.NewClient(config.ICANN.Username, config.ICANN.Password)

	ctx := context.Background()

	fmt.Println("Authenticating...")
	if err := client.AuthenticateWithContext(ctx); err != nil {
		log.Fatalf("Failed to authenticate: %v", err)
	}
	fmt.Println("✓ Authentication successful")

	fmt.Println("\nFetching zone file links...")
	links, err := client.GetLinksWithContext(ctx)
	if err != nil {
		log.Fatalf("Failed to get zone links: %v", err)
	}
	fmt.Printf("✓ Found %d available zone files\n", len(links))

	outputDir := "zones"
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		log.Fatalf("Failed to create output directory: %v", err)
	}

	parallel := 5
	semaphore := make(chan struct{}, parallel)
	var wg sync.WaitGroup
	var mu sync.Mutex

	successCount := 0
	failCount := 0
	startTime := time.Now()

	fmt.Printf("\nDownloading zone files with %d parallel workers...\n\n", parallel)

	for i, link := range links {
		wg.Add(1)
		semaphore <- struct{}{}

		go func(idx int, url string) {
			defer wg.Done()
			defer func() { <-semaphore }()

			tld := extractTLDFromURL(url)
			filename := filepath.Join(outputDir, tld+".zone")

			err := client.DownloadZoneWithContext(ctx, url, filename)
			if err != nil {
				mu.Lock()
				failCount++
				fmt.Printf("[%d/%d] ✗ %s - Download failed: %v\n", idx+1, len(links), tld, err)
				mu.Unlock()
				return
			}

			stat, _ := os.Stat(filename)
			mu.Lock()
			successCount++
			fmt.Printf("[%d/%d] ✓ %s - %.2f MB\n", idx+1, len(links), tld, float64(stat.Size())/1024/1024)
			mu.Unlock()
		}(i, link)
	}

	wg.Wait()
	duration := time.Since(startTime)

	fmt.Printf("\n%s\n", "============================================================")
	fmt.Printf("Download Summary:\n")
	fmt.Printf("  Total:    %d zone files\n", len(links))
	fmt.Printf("  Success:  %d\n", successCount)
	fmt.Printf("  Failed:   %d\n", failCount)
	fmt.Printf("  Duration: %s\n", duration.Round(time.Second))
	fmt.Printf("  Output:   %s/\n", outputDir)
	fmt.Printf("%s\n", "============================================================")
}

func extractTLDFromURL(url string) string {
	lastSlash := -1
	for i := len(url) - 1; i >= 0; i-- {
		if url[i] == '/' {
			lastSlash = i
			break
		}
	}

	if lastSlash == -1 {
		return "unknown"
	}

	filename := url[lastSlash+1:]

	if len(filename) > 5 && filename[len(filename)-5:] == ".zone" {
		return filename[:len(filename)-5]
	}

	return filename
}
