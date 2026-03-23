// Package metrics Prometheus指标服务
// 提供服务监控指标
package metrics

import (
	"net/http"
	"strconv"
	"sync"
	"time"
)

// ============================================================================
// 指标类型
// ============================================================================

// MetricType 指标类型
type MetricType string

const (
	Counter   MetricType = "counter"   // 计数器
	Gauge     MetricType = "gauge"     // 仪表盘
	Histogram MetricType = "histogram" // 直方图
)

// ============================================================================
// 指标结构
// ============================================================================

// Metric 指标
type Metric struct {
	Name      string            // 指标名称
	Help      string            // 指标说明
	Type      MetricType        // 指标类型
	Value     float64           // 当前值
	Labels    map[string]string // 标签
	Timestamp time.Time         // 时间戳
}

// ============================================================================
// 指标服务
// ============================================================================

// MetricsService Prometheus指标服务
type MetricsService struct {
	mu      sync.RWMutex
	metrics map[string]*Metric
}

// NewMetricsService 创建指标服务
func NewMetricsService() *MetricsService {
	svc := &MetricsService{
		metrics: make(map[string]*Metric),
	}

	// 注册默认指标
	svc.registerDefaults()

	return svc
}

// registerDefaults 注册默认指标
func (s *MetricsService) registerDefaults() {
	// HTTP请求相关
	s.Register("http_requests_total", "HTTP请求总数", Counter)
	s.Register("http_request_duration_seconds", "HTTP请求耗时", Histogram)
	s.Register("http_requests_in_flight", "当前处理中的HTTP请求数", Gauge)

	// 认证相关
	s.Register("auth_login_total", "登录请求总数", Counter)
	s.Register("auth_login_failed_total", "登录失败总数", Counter)
	s.Register("auth_register_total", "注册请求总数", Counter)
	s.Register("auth_token_refresh_total", "Token刷新总数", Counter)
	s.Register("auth_token_revoke_total", "Token撤销总数", Counter)
	s.Register("auth_account_locked_total", "账户锁定总数", Counter)

	// OAuth相关
	s.Register("oauth_authorize_total", "授权请求总数", Counter)
	s.Register("oauth_token_exchange_total", "Token交换总数", Counter)
	s.Register("oauth_code_invalid_total", "无效授权码总数", Counter)

	// 安全相关
	s.Register("security_rate_limit_total", "限流触发总数", Counter)
	s.Register("security_invalid_token_total", "无效Token总数", Counter)
	s.Register("security_password_mismatch_total", "密码不匹配总数", Counter)

	// 系统相关
	s.Register("db_connections_active", "活跃数据库连接数", Gauge)
	s.Register("db_connections_idle", "空闲数据库连接数", Gauge)
	s.Register("cache_hits_total", "缓存命中总数", Counter)
	s.Register("cache_misses_total", "缓存未命中总数", Counter)
}

// ============================================================================
// 指标注册
// ============================================================================

// Register 注册指标
func (s *MetricsService) Register(name, help string, metricType MetricType) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.metrics[name] = &Metric{
		Name:      name,
		Help:      help,
		Type:      metricType,
		Value:     0,
		Labels:    make(map[string]string),
		Timestamp: time.Now(),
	}
}

// ============================================================================
// 指标操作
// ============================================================================

// Increment 增加计数器
func (s *MetricsService) Increment(name string) {
	s.IncrementBy(name, 1)
}

// IncrementBy 增加指定值
func (s *MetricsService) IncrementBy(name string, value float64) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if m, ok := s.metrics[name]; ok {
		m.Value += value
		m.Timestamp = time.Now()
	}
}

// Set 设置仪表盘值
func (s *MetricsService) Set(name string, value float64) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if m, ok := s.metrics[name]; ok {
		m.Value = value
		m.Timestamp = time.Now()
	}
}

// Get 获取指标值
func (s *MetricsService) Get(name string) float64 {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if m, ok := s.metrics[name]; ok {
		return m.Value
	}
	return 0
}

// ============================================================================
// Prometheus格式输出
// ============================================================================

// ToPrometheusFormat 输出Prometheus格式
func (s *MetricsService) ToPrometheusFormat() string {
	s.mu.RLock()
	defer s.mu.RUnlock()

	output := ""
	for _, m := range s.metrics {
		// 输出HELP注释
		output += "# HELP " + m.Name + " " + m.Help + "\n"

		// 输出TYPE注释
		output += "# TYPE " + m.Name + " " + string(m.Type) + "\n"

		// 输出指标值
		output += m.Name + " " + formatFloat(m.Value) + "\n"
	}

	return output
}

// formatFloat 格式化浮点数
func formatFloat(f float64) string {
	return strconv.FormatFloat(f, 'f', -1, 64)
}

// ============================================================================
// HTTP请求指标中间件
// ============================================================================

// HTTPMiddleware HTTP请求指标中间件
func (s *MetricsService) HTTPMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()

		// 增加正在处理的请求数
		s.Increment("http_requests_in_flight")
		defer s.IncrementBy("http_requests_in_flight", -1)

		// 包装ResponseWriter以捕获状态码
		wrapped := &responseWriter{ResponseWriter: w, statusCode: 200}

		// 处理请求
		next.ServeHTTP(wrapped, r)

		// 记录指标
		s.Increment("http_requests_total")
		duration := time.Since(start).Seconds()
		s.Set("http_request_duration_seconds", duration)
	})
}

// responseWriter 包装http.ResponseWriter
type responseWriter struct {
	http.ResponseWriter
	statusCode int
}

func (rw *responseWriter) WriteHeader(code int) {
	rw.statusCode = code
	rw.ResponseWriter.WriteHeader(code)
}
