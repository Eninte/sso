// Package metrics Prometheus 指标服务
// 提供服务监控指标
package metrics

import (
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/prometheus/client_golang/prometheus/testutil"
)

// ============================================================================
// 指标类型（保留以兼容外部调用方）
// ============================================================================

// MetricType 指标类型
type MetricType string

const (
	Counter   MetricType = "counter"   // 计数器
	Gauge     MetricType = "gauge"     // 仪表盘
	Histogram MetricType = "histogram" // 直方图
)

// ============================================================================
// 指标服务
// ============================================================================

// Service Prometheus 指标服务
type Service struct {
	registry   *prometheus.Registry
	mu         sync.RWMutex
	counters   map[string]prometheus.Counter
	gauges     map[string]prometheus.Gauge
	histograms map[string]prometheus.Histogram
}

// NewService 创建指标服务
func NewService() *Service {
	registry := prometheus.NewRegistry()
	// 自动采集 Go runtime + process 指标（官方库自带能力）
	registry.MustRegister(
		collectors.NewGoCollector(),
		collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}),
	)

	svc := &Service{
		registry:   registry,
		counters:   make(map[string]prometheus.Counter),
		gauges:     make(map[string]prometheus.Gauge),
		histograms: make(map[string]prometheus.Histogram),
	}

	svc.registerDefaults()

	return svc
}

// registerDefaults 注册默认指标
func (s *Service) registerDefaults() {
	// HTTP 请求相关
	s.Register("http_requests_total", "HTTP 请求总数", Counter)
	s.Register("http_request_duration_seconds", "HTTP 请求耗时", Histogram)
	s.Register("http_requests_in_flight", "当前处理中的 HTTP 请求数", Gauge)

	// 认证相关
	s.Register("auth_login_total", "登录请求总数", Counter)
	s.Register("auth_login_failed_total", "登录失败总数", Counter)
	s.Register("auth_register_total", "注册请求总数", Counter)
	s.Register("auth_token_refresh_total", "Token 刷新总数", Counter)
	s.Register("auth_token_revoke_total", "Token 撤销总数", Counter)
	s.Register("auth_account_locked_total", "账户锁定总数", Counter)

	// OAuth 相关
	s.Register("oauth_authorize_total", "授权请求总数", Counter)
	s.Register("oauth_token_exchange_total", "Token 交换总数", Counter)
	s.Register("oauth_code_invalid_total", "无效授权码总数", Counter)

	// 安全相关
	s.Register("security_rate_limit_total", "限流触发总数", Counter)
	s.Register("security_ratelimit_error_total", "限流器错误总数（fail-open 放行）", Counter)
	s.Register("security_invalid_token_total", "无效 Token 总数", Counter)
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
func (s *Service) Register(name, help string, metricType MetricType) {
	s.mu.Lock()
	defer s.mu.Unlock()

	switch metricType {
	case Counter:
		c := prometheus.NewCounter(prometheus.CounterOpts{
			Name: name,
			Help: help,
		})
		s.registry.MustRegister(c)
		s.counters[name] = c
	case Gauge:
		g := prometheus.NewGauge(prometheus.GaugeOpts{
			Name: name,
			Help: help,
		})
		s.registry.MustRegister(g)
		s.gauges[name] = g
	case Histogram:
		h := prometheus.NewHistogram(prometheus.HistogramOpts{
			Name:    name,
			Help:    help,
			Buckets: prometheus.DefBuckets,
		})
		s.registry.MustRegister(h)
		s.histograms[name] = h
	}
}

// ============================================================================
// 指标操作
// ============================================================================

// Increment 增加计数器
func (s *Service) Increment(name string) {
	s.IncrementBy(name, 1)
}

// IncrementBy 增加指定值
// 注：prometheus.Counter 不允许负值，负值调用会被忽略
func (s *Service) IncrementBy(name string, value float64) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if c, ok := s.counters[name]; ok && value >= 0 {
		c.Add(value)
	}
}

// Set 设置仪表盘值
func (s *Service) Set(name string, value float64) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if g, ok := s.gauges[name]; ok {
		g.Set(value)
	}
}

// Observe 记录直方图观测值
func (s *Service) Observe(name string, value float64) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if h, ok := s.histograms[name]; ok {
		h.Observe(value)
	}
}

// Get 获取指标当前值
//   - Counter: 返回累计值
//   - Gauge: 返回当前值
//   - Histogram: 返回观测次数（_count）
func (s *Service) Get(name string) float64 {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if c, ok := s.counters[name]; ok {
		return testutil.ToFloat64(c)
	}
	if g, ok := s.gauges[name]; ok {
		return testutil.ToFloat64(g)
	}
	if _, ok := s.histograms[name]; ok {
		// Histogram 没有标量"当前值"，返回观测次数（_count 行）
		return s.histogramCount(name)
	}
	return 0
}

// histogramCount 从 Prometheus 文本输出中提取指定 histogram 的 _count 值
func (s *Service) histogramCount(name string) float64 {
	output := s.ToPrometheusFormat()
	prefix := name + "_count "
	for _, line := range strings.Split(output, "\n") {
		if strings.HasPrefix(line, prefix) {
			parts := strings.Fields(line)
			if len(parts) >= 2 {
				if v, err := strconv.ParseFloat(parts[1], 64); err == nil {
					return v
				}
			}
		}
	}
	return 0
}

// ============================================================================
// Prometheus 格式输出
// ============================================================================

// ToPrometheusFormat 输出 Prometheus 文本格式
// 仅供测试与兼容旧调用使用；生产端点请使用 HTTPHandler
func (s *Service) ToPrometheusFormat() string {
	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	rec := httptest.NewRecorder()
	s.HTTPHandler().ServeHTTP(rec, req)
	return rec.Body.String()
}

// HTTPHandler 返回标准的 Prometheus /metrics HTTP handler
func (s *Service) HTTPHandler() http.Handler {
	return promhttp.HandlerFor(s.registry, promhttp.HandlerOpts{
		EnableOpenMetrics: true,
	})
}

// Registry 返回底层 prometheus.Registry，用于自定义采集器注册
func (s *Service) Registry() *prometheus.Registry {
	return s.registry
}

// ============================================================================
// HTTP 请求指标中间件
// ============================================================================

// HTTPMiddleware HTTP 请求指标中间件
func (s *Service) HTTPMiddleware(next http.Handler) http.Handler {
	// 在中间件装配时加 RLock 读取指标引用，避免与并发 Register 的数据竞争。
	// 实际使用中 Register 仅在 NewService 初始化期间调用，此处加锁仅为防御性编程。
	s.mu.RLock()
	inFlight := s.gauges["http_requests_in_flight"]
	reqTotal := s.counters["http_requests_total"]
	reqDuration := s.histograms["http_request_duration_seconds"]
	s.mu.RUnlock()

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()

		// 增加正在处理的请求数
		if inFlight != nil {
			inFlight.Inc()
			defer inFlight.Dec()
		}

		// 包装 ResponseWriter 以捕获状态码
		wrapped := &responseWriter{ResponseWriter: w, statusCode: http.StatusOK}

		// 处理请求
		next.ServeHTTP(wrapped, r)

		// 记录指标
		if reqTotal != nil {
			reqTotal.Inc()
		}
		if reqDuration != nil {
			reqDuration.Observe(time.Since(start).Seconds())
		}
	})
}

// responseWriter 包装 http.ResponseWriter
type responseWriter struct {
	http.ResponseWriter
	statusCode int
}

func (rw *responseWriter) WriteHeader(code int) {
	rw.statusCode = code
	rw.ResponseWriter.WriteHeader(code)
}
