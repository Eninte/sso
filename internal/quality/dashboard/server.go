// Package dashboard 质量仪表盘HTTP服务
package dashboard

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"github.com/example/sso/internal/store"
)

// ============================================================================
// 仪表盘服务
// ============================================================================

// Server 仪表盘HTTP服务
type Server struct {
	store  store.QualityMetricsStore
	logger *slog.Logger
}

// NewServer 创建仪表盘服务
func NewServer(store store.QualityMetricsStore, logger *slog.Logger) *Server {
	return &Server{
		store:  store,
		logger: logger,
	}
}

// ============================================================================
// HTTP处理函数
// ============================================================================

// HandleMetricsAPI 处理指标API请求
// GET /quality/api/metrics
func (s *Server) HandleMetricsAPI(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// 获取30天趋势数据
	to := time.Now()
	from := to.AddDate(0, 0, -30)
	metrics, err := s.store.GetMetricsRange(ctx, from, to)
	if err != nil {
		s.logger.Error("获取指标数据失败", "error", err)
		writeJSONError(w, http.StatusInternalServerError, "获取指标数据失败")
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"metrics": metrics,
		"count":   len(metrics),
	})
}

// HandleWeeklyReportAPI 处理周报API请求
// GET /quality/api/report/weekly
func (s *Server) HandleWeeklyReportAPI(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	comparison, err := s.store.GetWeeklyComparison(ctx)
	if err != nil {
		s.logger.Error("获取周对比数据失败", "error", err)
		writeJSONError(w, http.StatusInternalServerError, "获取周报数据失败")
		return
	}

	report := GenerateWeeklySummary(comparison)
	writeJSON(w, http.StatusOK, report)
}

// ============================================================================
// 辅助函数
// ============================================================================

func writeJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(data)
}

func writeJSONError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}
