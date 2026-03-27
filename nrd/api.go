package nrd

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/gorilla/mux"
	"gorm.io/gorm"
)

// APIServer HTTP API 伺服器
type APIServer struct {
	db     *gorm.DB
	router *mux.Router
}

// NewAPIServer 建立新的 API 伺服器
func NewAPIServer(db *gorm.DB) *APIServer {
	server := &APIServer{
		db:     db,
		router: mux.NewRouter(),
	}
	server.setupRoutes()
	return server
}

// setupRoutes 設定路由
func (s *APIServer) setupRoutes() {
	s.router.HandleFunc("/api/nrd/{period}", s.handleGetNRD).Methods("GET")
	s.router.HandleFunc("/api/stats/{period}", s.handleGetStats).Methods("GET")
	s.router.HandleFunc("/health", s.handleHealth).Methods("GET")
}

// Start 啟動 API 伺服器
func (s *APIServer) Start(addr string) error {
	fmt.Printf("Starting API server on %s\n", addr)
	return http.ListenAndServe(addr, s.router)
}

// handleGetNRD 處理 NRD 資料下載請求
// 支援的 period: 01d, 07d, 01m, 02m, 03m, 04m, 05m, 06m, 07m, 08m, 09m, 10m, 11m, 01y
func (s *APIServer) handleGetNRD(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	period := vars["period"]

	startDate, endDate, err := parsePeriod(period)
	if err != nil {
		http.Error(w, fmt.Sprintf("Invalid period: %v", err), http.StatusBadRequest)
		return
	}

	domains, err := GetDomainsByDateRange(s.db, startDate, endDate)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to query domains: %v", err), http.StatusInternalServerError)
		return
	}

	// 設定下載檔案的 header
	filename := fmt.Sprintf("nrd-%s-%s.txt", period, time.Now().Format("20060102"))
	w.Header().Set("Content-Type", "text/plain")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%s", filename))

	// 輸出域名列表
	for _, domain := range domains {
		fmt.Fprintln(w, domain.DomainName)
	}
}

// handleGetStats 處理統計資料請求
func (s *APIServer) handleGetStats(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	period := vars["period"]

	startDate, endDate, err := parsePeriod(period)
	if err != nil {
		http.Error(w, fmt.Sprintf("Invalid period: %v", err), http.StatusBadRequest)
		return
	}

	count, err := GetDomainCount(s.db, startDate, endDate)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to get count: %v", err), http.StatusInternalServerError)
		return
	}

	stats := map[string]interface{}{
		"period":     period,
		"start_date": startDate.Format("2006-01-02"),
		"end_date":   endDate.Format("2006-01-02"),
		"count":      count,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(stats)
}

// handleHealth 健康檢查
func (s *APIServer) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

// parsePeriod 解析時間週期參數
func parsePeriod(period string) (time.Time, time.Time, error) {
	now := time.Now()
	endDate := now

	var startDate time.Time

	switch period {
	case "01d":
		startDate = now.AddDate(0, 0, -1)
	case "07d":
		startDate = now.AddDate(0, 0, -7)
	case "01m":
		startDate = now.AddDate(0, -1, 0)
	case "02m":
		startDate = now.AddDate(0, -2, 0)
	case "03m":
		startDate = now.AddDate(0, -3, 0)
	case "04m":
		startDate = now.AddDate(0, -4, 0)
	case "05m":
		startDate = now.AddDate(0, -5, 0)
	case "06m":
		startDate = now.AddDate(0, -6, 0)
	case "07m":
		startDate = now.AddDate(0, -7, 0)
	case "08m":
		startDate = now.AddDate(0, -8, 0)
	case "09m":
		startDate = now.AddDate(0, -9, 0)
	case "10m":
		startDate = now.AddDate(0, -10, 0)
	case "11m":
		startDate = now.AddDate(0, -11, 0)
	case "01y":
		startDate = now.AddDate(-1, 0, 0)
	default:
		return time.Time{}, time.Time{}, fmt.Errorf("unsupported period: %s", period)
	}

	// 將時間設定為當天的開始
	startDate = time.Date(startDate.Year(), startDate.Month(), startDate.Day(), 0, 0, 0, 0, startDate.Location())
	endDate = time.Date(endDate.Year(), endDate.Month(), endDate.Day(), 23, 59, 59, 999999999, endDate.Location())

	return startDate, endDate, nil
}
