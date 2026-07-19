// Package metrics_test Prometheus 指标服务单元测试
package metrics_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/example/sso/internal/metrics"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ============================================================================
// NewService 测试
// ============================================================================

func TestNewService_CreatesWithDefaults(t *testing.T) {
	svc := metrics.NewService()
	require.NotNil(t, svc)

	// 验证默认指标已注册（初始值为 0）
	assert.Equal(t, float64(0), svc.Get("http_requests_total"))
	assert.Equal(t, float64(0), svc.Get("auth_login_total"))
	assert.Equal(t, float64(0), svc.Get("security_rate_limit_total"))
}

// ============================================================================
// Register 测试
// ============================================================================

func TestRegister_CustomMetric(t *testing.T) {
	svc := metrics.NewService()

	svc.Register("custom_metric", "自定义指标", metrics.Counter)
	assert.Equal(t, float64(0), svc.Get("custom_metric"))

	svc.Increment("custom_metric")
	assert.Equal(t, float64(1), svc.Get("custom_metric"))
}

// ============================================================================
// Increment 测试
// ============================================================================

func TestIncrement(t *testing.T) {
	svc := metrics.NewService()

	svc.Increment("http_requests_total")
	assert.Equal(t, float64(1), svc.Get("http_requests_total"))

	svc.Increment("http_requests_total")
	assert.Equal(t, float64(2), svc.Get("http_requests_total"))
}

func TestIncrement_NonExistentMetric(t *testing.T) {
	svc := metrics.NewService()

	// 不存在的指标不会 panic
	svc.Increment("nonexistent_metric")
	assert.Equal(t, float64(0), svc.Get("nonexistent_metric"))
}

func TestIncrementBy(t *testing.T) {
	svc := metrics.NewService()

	svc.IncrementBy("auth_login_total", 5)
	assert.Equal(t, float64(5), svc.Get("auth_login_total"))

	// prometheus.Counter 不允许负值，负调用被忽略
	svc.IncrementBy("auth_login_total", -2)
	assert.Equal(t, float64(5), svc.Get("auth_login_total"))
}

// ============================================================================
// Set / Get 测试
// ============================================================================

func TestSet(t *testing.T) {
	svc := metrics.NewService()

	svc.Set("db_connections_active", 10)
	assert.Equal(t, float64(10), svc.Get("db_connections_active"))

	svc.Set("db_connections_active", 0)
	assert.Equal(t, float64(0), svc.Get("db_connections_active"))
}

func TestGet_NonExistentMetric(t *testing.T) {
	svc := metrics.NewService()

	assert.Equal(t, float64(0), svc.Get("nonexistent_metric"))
}

// ============================================================================
// ToPrometheusFormat 测试
// ============================================================================

func TestToPrometheusFormat(t *testing.T) {
	svc := metrics.NewService()
	svc.Increment("http_requests_total")
	svc.Set("db_connections_active", 5)

	output := svc.ToPrometheusFormat()

	// 验证 HELP 和 TYPE 注释，以及指标值
	assert.Contains(t, output, "# HELP http_requests_total HTTP 请求总数")
	assert.Contains(t, output, "# TYPE http_requests_total counter")
	assert.Contains(t, output, "http_requests_total 1")

	assert.Contains(t, output, "# TYPE db_connections_active gauge")
	assert.Contains(t, output, "db_connections_active 5")
}

func TestToPrometheusFormat_EmptyMetrics(t *testing.T) {
	svc := metrics.NewService()
	// 使用一个新注册的指标
	svc.Register("empty_test", "test", metrics.Counter)

	output := svc.ToPrometheusFormat()
	assert.Contains(t, output, "empty_test 0")
}

func TestHistogramObserve_PrometheusBuckets(t *testing.T) {
	svc := metrics.NewService()

	svc.Observe("http_request_duration_seconds", 0.007)
	svc.Observe("http_request_duration_seconds", 0.2)
	svc.Observe("http_request_duration_seconds", 12)

	output := svc.ToPrometheusFormat()

	assert.Contains(t, output, "# TYPE http_request_duration_seconds histogram")
	// prometheus.DefBuckets: 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10, +Inf
	assert.Contains(t, output, `http_request_duration_seconds_bucket{le="0.005"} 0`)
	assert.Contains(t, output, `http_request_duration_seconds_bucket{le="0.01"} 1`)
	assert.Contains(t, output, `http_request_duration_seconds_bucket{le="0.25"} 2`)
	assert.Contains(t, output, `http_request_duration_seconds_bucket{le="+Inf"} 3`)
	assert.Contains(t, output, "http_request_duration_seconds_count 3")
}

// ============================================================================
// HTTPMiddleware 测试
// ============================================================================

func TestHTTPMiddleware(t *testing.T) {
	svc := metrics.NewService()

	handler := svc.HTTPMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/test", nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, float64(1), svc.Get("http_requests_total"))
	// Histogram 返回观测次数（_count）
	assert.Equal(t, float64(1), svc.Get("http_request_duration_seconds"))
	// in_flight 应该在请求完成后减少回 0
	assert.Equal(t, float64(0), svc.Get("http_requests_in_flight"))
}

func TestHTTPMiddleware_MultipleRequests(t *testing.T) {
	svc := metrics.NewService()

	handler := svc.HTTPMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	for i := 0; i < 10; i++ {
		req := httptest.NewRequest("GET", "/test", nil)
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)
	}

	assert.Equal(t, float64(10), svc.Get("http_requests_total"))
}

func TestHTTPMiddleware_RecordsStatusCode(t *testing.T) {
	svc := metrics.NewService()

	handler := svc.HTTPMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))

	req := httptest.NewRequest("GET", "/notfound", nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
	assert.Equal(t, float64(1), svc.Get("http_requests_total"))
}

// ============================================================================
// 并发安全测试
// ============================================================================

func TestService_ConcurrentAccess(t *testing.T) {
	svc := metrics.NewService()

	done := make(chan bool, 100)
	for i := 0; i < 100; i++ {
		go func() {
			svc.Increment("http_requests_total")
			_ = svc.Get("http_requests_total")
			svc.Set("db_connections_active", 5)
			done <- true
		}()
	}

	for i := 0; i < 100; i++ {
		<-done
	}

	assert.Equal(t, float64(100), svc.Get("http_requests_total"))
}

// ============================================================================
// 所有默认指标验证
// ============================================================================

func TestDefaultMetrics_AllRegistered(t *testing.T) {
	svc := metrics.NewService()

	defaults := []string{
		"http_requests_total",
		"http_request_duration_seconds",
		"http_requests_in_flight",
		"auth_login_total",
		"auth_login_failed_total",
		"auth_register_total",
		"auth_token_refresh_total",
		"auth_token_revoke_total",
		"auth_account_locked_total",
		"oauth_authorize_total",
		"oauth_token_exchange_total",
		"oauth_code_invalid_total",
		"security_rate_limit_total",
		"security_invalid_token_total",
		"security_password_mismatch_total",
		"db_connections_active",
		"db_connections_idle",
		"cache_hits_total",
		"cache_misses_total",
	}

	for _, name := range defaults {
		assert.Equal(t, float64(0), svc.Get(name), "metric %s should be registered", name)
	}
}

// ============================================================================
// ToPrometheusFormat 格式验证
// ============================================================================

func TestToPrometheusFormat_ContainsAllMetrics(t *testing.T) {
	svc := metrics.NewService()

	output := svc.ToPrometheusFormat()

	// 19 个业务指标 + Go runtime + process 指标
	// 至少应该包含 19 个业务指标的 HELP/TYPE
	assert.True(t, strings.Count(output, "# HELP") >= 19, "expected at least 19 HELP lines")
	assert.True(t, strings.Count(output, "# TYPE") >= 19, "expected at least 19 TYPE lines")
}

// ============================================================================
// 新增：HTTPHandler / Registry 暴露测试
// ============================================================================

func TestHTTPHandler_ReturnsValidHandler(t *testing.T) {
	svc := metrics.NewService()
	svc.Increment("http_requests_total")

	h := svc.HTTPHandler()
	require.NotNil(t, h)

	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	// Content-Type 由 promhttp 设置（text/plain 或 application/openmetrics-text）
	ct := rec.Header().Get("Content-Type")
	assert.True(t, strings.HasPrefix(ct, "text/plain") || strings.HasPrefix(ct, "application/openmetrics-text"))
	assert.Contains(t, rec.Body.String(), "http_requests_total 1")
}

func TestRegistry_ReturnsUnderlyingRegistry(t *testing.T) {
	svc := metrics.NewService()
	reg := svc.Registry()
	require.NotNil(t, reg)
}

// ============================================================================
// 新增：Go runtime + process 指标验证
// ============================================================================

func TestRuntimeMetrics_Registered(t *testing.T) {
	svc := metrics.NewService()
	output := svc.ToPrometheusFormat()

	// 官方库自动采集 Go runtime 指标
	assert.Contains(t, output, "# TYPE go_goroutines gauge")
	assert.Contains(t, output, "# TYPE go_memstats_alloc_bytes gauge")
	// 官方库自动采集 process 指标
	assert.Contains(t, output, "# TYPE process_resident_memory_bytes gauge")
}
