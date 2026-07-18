package app

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/example/sso/internal/store/mock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ============================================================================
// healthHandler 单元测试
// 目标端点：GET /healthz
// 期望行为：始终返回 200，响应体为 JSON {"status":"ok","service":"sso","timestamp":"..."}
// ============================================================================

func TestHealthHandler_ReturnsOK(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rr := httptest.NewRecorder()

	healthHandler(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code, "应返回 200")
	assert.Contains(t, rr.Header().Get("Content-Type"), "application/json",
		"Content-Type 应为 application/json")
}

func TestHealthHandler_ResponseIsValidJSON(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rr := httptest.NewRecorder()

	healthHandler(rr, req)

	var body map[string]string
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &body),
		"响应体应为合法 JSON")
}

func TestHealthHandler_ResponseContainsRequiredFields(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rr := httptest.NewRecorder()

	healthHandler(rr, req)

	var body map[string]string
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &body))

	assert.Equal(t, "ok", body["status"], "status 应为 ok")
	assert.Equal(t, "sso", body["service"], "service 应为 sso")
	assert.NotEmpty(t, body["timestamp"], "timestamp 不应为空")
}

func TestHealthHandler_TimestampIsRFC3339(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rr := httptest.NewRecorder()

	healthHandler(rr, req)

	var body map[string]string
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &body))

	// timestamp 应为 RFC3339 格式（可被 time.Parse 解析）
	_, err := time.Parse(time.RFC3339, body["timestamp"])
	assert.NoError(t, err, "timestamp 应为 RFC3339 格式")
}

// ============================================================================
// readyzHandler 单元测试
// 目标端点：GET /readyz
// 期望行为：
//   - store.Ping 成功时返回 200，响应体 {"status":"ready","service":"sso"}
//   - store.Ping 失败时返回 503，响应体 {"status":"unready","service":"sso"}
//   - 失败时不泄漏内部错误详情（驱动名、地址、凭据等）
// ============================================================================

func TestReadyzHandler_StoreReady_ReturnsOK(t *testing.T) {
	mockStore := &mock.Store{PingErr: nil} // store.Ping 返回 nil → 就绪
	handler := readyzHandler(mockStore)

	req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code, "store 就绪时应返回 200")
	assert.Contains(t, rr.Header().Get("Content-Type"), "application/json",
		"Content-Type 应为 application/json")

	var body map[string]string
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &body))
	assert.Equal(t, "ready", body["status"], "status 应为 ready")
	assert.Equal(t, "sso", body["service"], "service 应为 sso")
}

func TestReadyzHandler_StoreUnready_Returns503(t *testing.T) {
	// store.Ping 返回 error → 未就绪
	mockStore := &mock.Store{PingErr: errors.New("database connection lost")}
	handler := readyzHandler(mockStore)

	req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusServiceUnavailable, rr.Code,
		"store 未就绪时应返回 503")
	assert.Contains(t, rr.Header().Get("Content-Type"), "application/json",
		"Content-Type 应为 application/json")

	var body map[string]string
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &body))
	assert.Equal(t, "unready", body["status"], "status 应为 unready")
	assert.Equal(t, "sso", body["service"], "service 应为 sso")
}

func TestReadyzHandler_ResponseDoesNotLeakInternalError(t *testing.T) {
	// store.Ping 失败时，响应体不应包含内部错误详情（安全要求）
	mockStore := &mock.Store{PingErr: errors.New("pgx: connection refused 127.0.0.1:5432 password=secret123")}
	handler := readyzHandler(mockStore)

	req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	bodyText := rr.Body.String()
	assert.NotContains(t, bodyText, "pgx", "不应泄漏驱动名")
	assert.NotContains(t, bodyText, "127.0.0.1:5432", "不应泄漏数据库地址")
	assert.NotContains(t, bodyText, "password", "不应泄漏凭据")
	assert.NotContains(t, bodyText, "secret123", "不应泄漏密码")
}
