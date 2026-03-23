// Package handler 指标处理器
// 提供Prometheus指标端点
package handler

import (
	"net/http"

	"github.com/your-org/sso/internal/metrics"
)

// ============================================================================
// MetricsHandler 指标处理器
// ============================================================================

// MetricsHandler 指标处理器
type MetricsHandler struct {
	metrics *metrics.MetricsService
}

// NewMetricsHandler 创建指标处理器
func NewMetricsHandler(metrics *metrics.MetricsService) *MetricsHandler {
	return &MetricsHandler{metrics: metrics}
}

// HandleMetrics 处理指标请求
// GET /metrics
func (h *MetricsHandler) HandleMetrics(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain; version=0.0.4")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(h.metrics.ToPrometheusFormat()))
}
