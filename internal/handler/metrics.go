// Package handler 指标处理器
// 提供 Prometheus 指标端点
package handler

import (
	"net/http"

	"github.com/example/sso/internal/metrics"
)

// ============================================================================
// MetricsHandler 指标处理器
// ============================================================================

// MetricsHandler 指标处理器
// 认证由外部 BasicAuth 中间件处理
type MetricsHandler struct {
	handler http.Handler
}

// NewMetricsHandler 创建指标处理器
// 在构造时缓存官方 promhttp.Handler，避免每次请求重新创建
func NewMetricsHandler(metrics *metrics.Service) *MetricsHandler {
	return &MetricsHandler{
		handler: metrics.HTTPHandler(),
	}
}

// HandleMetrics 处理指标请求
// GET /metrics
// 认证由 BasicAuth 中间件处理
func (h *MetricsHandler) HandleMetrics(w http.ResponseWriter, r *http.Request) {
	// 直接委托给官方 promhttp.Handler，由其设置正确的 Content-Type 与状态码
	h.handler.ServeHTTP(w, r)
}
