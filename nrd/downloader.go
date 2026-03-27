package nrd

import (
	"archive/zip"
	"bufio"
	"compress/gzip"
	"context"
	"encoding/base64"
	"encoding/csv"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

// Downloader 處理各種來源的 NRD 下載
type Downloader struct {
	config *Config
	client *http.Client
}

// NewDownloader 建立新的下載器
func NewDownloader(config *Config) *Downloader {
	return &Downloader{
		config: config,
		client: &http.Client{
			Timeout: 30 * time.Minute,
		},
	}
}

// DownloadPhase1 下載 Phase1 來源 (whoisds.com)
// URL 格式: base64(yyyy-mm-dd.zip)
func (d *Downloader) DownloadPhase1(ctx context.Context, date time.Time) ([]string, error) {
	dateStr := date.Format("2006-01-02")
	args := base64.StdEncoding.EncodeToString([]byte(dateStr + ".zip"))
	url := strings.Replace(d.config.NRDList.Phase1.URL, "{args}", args, 1)

	fmt.Printf("[Phase1] Downloading from: %s\n", url)
	domains, err := d.downloadAndExtractZip(ctx, url)
	if err != nil {
		return nil, fmt.Errorf("phase1 download failed from URL [%s]: %w", url, err)
	}

	return domains, nil
}

// DownloadPhase2 下載 Phase2 來源 (GitHub shreshta-labs)
// 固定檔名: nrd-1w.csv 和 nrd-1m.csv
func (d *Downloader) DownloadPhase2(ctx context.Context, filename string) ([]string, error) {
	url := d.config.NRDList.Phase2.URL + filename

	fmt.Printf("[Phase2] Downloading from: %s\n", url)
	domains, err := d.downloadTextFile(ctx, url)
	if err != nil {
		return nil, fmt.Errorf("phase2 download failed from URL [%s]: %w", url, err)
	}

	return domains, nil
}

// DownloadPhase3 下載 Phase3 來源 (whoisfreaks.com)
// 格式: CSV.GZ，需要先解壓 gzip
func (d *Downloader) DownloadPhase3(ctx context.Context) ([]string, error) {
	url := d.config.NRDList.Phase3.URL

	fmt.Printf("[Phase3] Downloading from: %s\n", url)
	domains, err := d.downloadAndExtractGzipCSV(ctx, url)
	if err != nil {
		return nil, fmt.Errorf("phase3 download failed from URL [%s]: %w", url, err)
	}

	return domains, nil
}

// DownloadPhase4 下載 Phase4 來源 (hagezi dns-blocklists)
// 支援: nrd7.txt, nrd14-8.txt, nrd21-15.txt, nrd28-22.txt, nrd35-29.txt
func (d *Downloader) DownloadPhase4(ctx context.Context, filename string) ([]string, error) {
	// 添加 .txt 後綴
	if !strings.HasSuffix(filename, ".txt") {
		filename = filename + ".txt"
	}
	url := strings.Replace(d.config.NRDList.Phase4.URL, "{args}", filename, 1)

	fmt.Printf("[Phase4] Downloading from: %s\n", url)
	domains, err := d.downloadTextFile(ctx, url)
	if err != nil {
		return nil, fmt.Errorf("phase4 download failed from URL [%s]: %w", url, err)
	}

	return domains, nil
}

// downloadAndExtractZip 下載並解壓 ZIP 檔案
func (d *Downloader) downloadAndExtractZip(ctx context.Context, url string) ([]string, error) {
	tmpFile, err := os.CreateTemp("", "nrd-*.zip")
	if err != nil {
		return nil, err
	}
	defer os.Remove(tmpFile.Name())
	defer tmpFile.Close()

	if err := d.downloadFile(ctx, url, tmpFile); err != nil {
		return nil, err
	}

	tmpFile.Close()

	r, err := zip.OpenReader(tmpFile.Name())
	if err != nil {
		return nil, err
	}
	defer r.Close()

	var allDomains []string
	for _, f := range r.File {
		if f.FileInfo().IsDir() {
			continue
		}

		rc, err := f.Open()
		if err != nil {
			continue
		}

		domains, err := d.parseDomainList(rc)
		rc.Close()
		if err != nil {
			continue
		}

		allDomains = append(allDomains, domains...)
	}

	return allDomains, nil
}

// downloadAndExtractGzipCSV 下載並解壓 CSV.GZ 檔案（Phase3 專用）
// CSV 格式：第二欄是域名
func (d *Downloader) downloadAndExtractGzipCSV(ctx context.Context, url string) ([]string, error) {
	tmpFile, err := os.CreateTemp("", "nrd-*.csv.gz")
	if err != nil {
		return nil, err
	}
	defer os.Remove(tmpFile.Name())
	defer tmpFile.Close()

	if err := d.downloadFile(ctx, url, tmpFile); err != nil {
		return nil, err
	}

	tmpFile.Seek(0, 0)

	gzr, err := gzip.NewReader(tmpFile)
	if err != nil {
		return nil, err
	}
	defer gzr.Close()

	csvReader := csv.NewReader(gzr)
	var domains []string

	// 跳過第一行（標題行）
	_, err = csvReader.Read()
	if err != nil && err != io.EOF {
		return nil, err
	}

	for {
		record, err := csvReader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			continue
		}

		// 第二欄（索引 1）是域名
		if len(record) > 1 {
			domain := strings.TrimSpace(record[1])
			if domain != "" && !strings.HasPrefix(domain, "#") {
				domains = append(domains, domain)
			}
		}
	}

	return domains, nil
}

// downloadTextFile 下載純文字檔案
func (d *Downloader) downloadTextFile(ctx context.Context, url string) ([]string, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}

	resp, err := d.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	return d.parseDomainList(resp.Body)
}

// downloadFile 下載檔案到指定位置
func (d *Downloader) downloadFile(ctx context.Context, url string, dest *os.File) error {
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return err
	}

	resp, err := d.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	_, err = io.Copy(dest, resp.Body)
	return err
}

// parseDomainList 解析域名列表
func (d *Downloader) parseDomainList(r io.Reader) ([]string, error) {
	var domains []string
	scanner := bufio.NewScanner(r)

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		// 跳過空行和註解
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, ";") {
			continue
		}

		// 清理域名
		domain := cleanDomain(line)
		if domain != "" {
			domains = append(domains, domain)
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, err
	}

	return domains, nil
}

// cleanDomain 清理域名字串
func cleanDomain(s string) string {
	// 移除前後空白
	s = strings.TrimSpace(s)

	// 轉小寫
	s = strings.ToLower(s)

	// 移除可能的協議前綴
	s = strings.TrimPrefix(s, "http://")
	s = strings.TrimPrefix(s, "https://")
	s = strings.TrimPrefix(s, "www.")

	// 移除路徑部分
	if idx := strings.Index(s, "/"); idx != -1 {
		s = s[:idx]
	}

	// 移除埠號
	if idx := strings.Index(s, ":"); idx != -1 {
		s = s[:idx]
	}

	// 只保留域名部分（可能包含子域名）
	s = strings.TrimSpace(s)

	// 基本驗證
	if !isValidDomain(s) {
		return ""
	}

	return s
}

// isValidDomain 簡單驗證域名格式
func isValidDomain(s string) bool {
	if len(s) == 0 || len(s) > 253 {
		return false
	}

	// 必須包含至少一個點
	if !strings.Contains(s, ".") {
		return false
	}

	// 不能以點開始或結束
	if strings.HasPrefix(s, ".") || strings.HasSuffix(s, ".") {
		return false
	}

	return true
}

// ExtractTLD 從域名中提取 TLD
func ExtractTLD(domain string) string {
	parts := strings.Split(domain, ".")
	if len(parts) < 2 {
		return ""
	}
	return parts[len(parts)-1]
}
