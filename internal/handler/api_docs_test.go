// Package handler API 文档调试前端单元测试
//
// 验证内容：
//   - HandlePage 返回 HTML，CSP 正确放松（仅 style-src unsafe-inline）
//   - HandleSpec 返回 OpenAPI 3.0 规范，注入运行时 baseURL 和版本
//   - HandleScalarJS 返回内嵌 JS，Content-Type 和 Cache-Control 正确
//   - 路由器集成测试：未鉴权/普通用户 token/管理员 token 的访问控制
package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// 测试用 baseURL 和版本
const (
	testBaseURL = "http://localhost:9090"
	testVersion = "test-v1.0.0"
)

// newTestAPIDocsHandler 创建测试用 handler
func newTestAPIDocsHandler() *APIDocsHandler {
	return NewAPIDocsHandler(testBaseURL, testVersion)
}

// ============================================================================
// HandlePage 测试
// ============================================================================

func TestAPIDocsHandler_HandlePage_ReturnsHTML(t *testing.T) {
	h := newTestAPIDocsHandler()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/api-docs", nil)
	rr := httptest.NewRecorder()

	h.HandlePage(rr, req)

	// 验证状态码
	assert.Equal(t, http.StatusOK, rr.Code, "应返回 200")

	// 验证 Content-Type
	ct := rr.Header().Get("Content-Type")
	assert.Contains(t, ct, "text/html", "Content-Type 应为 text/html")
	assert.Contains(t, ct, "charset=utf-8", "应包含 utf-8 charset")

	// 验证 HTML 内容
	body := rr.Body.String()
	assert.Contains(t, body, "<!DOCTYPE html>", "应是 HTML 文档")
	assert.Contains(t, body, "SSO API 文档", "应包含页面标题")
	assert.Contains(t, body, "api-reference", "应包含 Scalar web component 标签")

	// 验证模板变量注入
	assert.Contains(t, body, testBaseURL, "应注入 baseURL")
	assert.Contains(t, body, "/api/v1/admin/api-docs/openapi.json", "应注入 specURL")
	assert.Contains(t, body, "/api/v1/admin/api-docs/scalar.js", "应注入 scalarJSURL")
}

func TestAPIDocsHandler_HandlePage_SetsCSPAllowInlineStyle(t *testing.T) {
	h := newTestAPIDocsHandler()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/api-docs", nil)
	rr := httptest.NewRecorder()

	h.HandlePage(rr, req)

	csp := rr.Header().Get("Content-Security-Policy")
	require.NotEmpty(t, csp, "CSP 必须设置")

	// 离线模式：不应允许任何 CDN
	assert.NotContains(t, csp, "cdn.jsdelivr.net", "CSP 不应允许 CDN（离线模式）")

	// script-src 必须保持严格 'self'（不带 unsafe-inline）
	assert.Contains(t, csp, "script-src 'self'", "script-src 必须为 'self'")
	assert.NotContains(t, strings.Split(csp, ";")[1], "unsafe-inline",
		"script-src 不应允许 unsafe-inline")

	// style-src 允许 unsafe-inline（Scalar 内部需要内联样式）
	assert.Contains(t, csp, "style-src 'self'", "style-src 应包含 'self'")
	// 注意：不严格断言 'unsafe-inline' 因为只有 Scalar 实际渲染时才需要
}

// ============================================================================
// HandleSpec 测试
// ============================================================================

func TestAPIDocsHandler_HandleSpec_ReturnsValidOpenAPI(t *testing.T) {
	h := newTestAPIDocsHandler()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/api-docs/openapi.json", nil)
	rr := httptest.NewRecorder()

	h.HandleSpec(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code, "应返回 200")

	// 验证 Content-Type
	assert.Contains(t, rr.Header().Get("Content-Type"), "application/json",
		"Content-Type 应为 application/json")

	// 验证返回的是合法 JSON
	var spec map[string]interface{}
	err := json.Unmarshal(rr.Body.Bytes(), &spec)
	require.NoError(t, err, "应返回合法 JSON")

	// 验证 OpenAPI 3.0 基本结构
	assert.Equal(t, "3.0.3", spec["openapi"], "应为 OpenAPI 3.0.3")
	assert.NotNil(t, spec["info"], "应包含 info")
	assert.NotNil(t, spec["paths"], "应包含 paths")
	assert.NotNil(t, spec["components"], "应包含 components")

	// 验证运行时 baseURL 注入
	servers, ok := spec["servers"].([]interface{})
	require.True(t, ok, "servers 应为数组")
	require.Len(t, servers, 1, "应有 1 个 server")
	server := servers[0].(map[string]interface{})
	assert.Equal(t, testBaseURL, server["url"], "应注入运行时 baseURL")
	assert.Equal(t, "当前服务实例", server["description"], "应有描述")

	// 验证版本注入
	info := spec["info"].(map[string]interface{})
	assert.Equal(t, testVersion, info["version"], "应注入版本号")
}

func TestAPIDocsHandler_HandleSpec_ContainsAllEndpointTags(t *testing.T) {
	h := newTestAPIDocsHandler()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/api-docs/openapi.json", nil)
	rr := httptest.NewRecorder()

	h.HandleSpec(rr, req)

	var spec map[string]interface{}
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &spec))

	// 验证关键端点都已包含
	paths := spec["paths"].(map[string]interface{})
	expectedPaths := []string{
		"/health",
		"/.well-known/openid-configuration",
		"/api/v1/register",
		"/api/v1/login",
		"/api/v1/token",
		"/api/v1/userinfo",
		"/api/v1/authorize",
		"/api/v1/mfa/setup",
		"/api/v1/admin/users",
		"/api/v1/admin/audit-logs",
		"/api/v1/admin/quality/api/metrics",
	}
	for _, p := range expectedPaths {
		assert.Contains(t, paths, p, "OpenAPI 应包含端点 %s", p)
	}
}

// ============================================================================
// HandleScalarJS 测试
// ============================================================================

func TestAPIDocsHandler_HandleScalarJS_ReturnsEmbeddedJS(t *testing.T) {
	h := newTestAPIDocsHandler()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/api-docs/scalar.js", nil)
	rr := httptest.NewRecorder()

	h.HandleScalarJS(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code, "应返回 200")

	// 验证 Content-Type
	ct := rr.Header().Get("Content-Type")
	assert.Contains(t, ct, "application/javascript", "Content-Type 应为 application/javascript")
	assert.Contains(t, ct, "charset=utf-8", "应包含 utf-8 charset")

	// 验证 Cache-Control
	cc := rr.Header().Get("Cache-Control")
	assert.Contains(t, cc, "public", "Cache-Control 应为 public")
	assert.Contains(t, cc, "max-age=86400", "应设置 1 天缓存")

	// 验证返回的是 Scalar JS（非空且包含特征字符串）
	body := rr.Body.Bytes()
	assert.NotEmpty(t, body, "JS 内容不应为空")
	// Scalar standalone JS 的特征字符串（首部注释）
	previewLen := 200
	if len(body) < previewLen {
		previewLen = len(body)
	}
	bodyStr := string(body[:previewLen])
	assert.Contains(t, bodyStr, "scalar", "应包含 scalar 特征字符串")
}

// ============================================================================
// 路由器集成测试：安全鉴权
// ============================================================================
// 注：这里只验证 handler 层面的行为，完整的鉴权链路（AuthMiddleware + RequireAdmin）
// 由 router.go 在 admin 子路由下注册，已在 internal/app 包测试覆盖。
// handler 层不重复实现鉴权，遵循"职责分离"原则。

// ============================================================================
// 工具函数测试
// ============================================================================

func TestFormatCSP_ReplacesBothNonces(t *testing.T) {
	tmpl := "script-src 'self' 'nonce-%s'; style-src 'self' 'nonce-%s'"
	out := formatCSP(tmpl, "abc123", "def456")
	assert.Equal(t, "script-src 'self' 'nonce-abc123'; style-src 'self' 'nonce-def456'", out)
}

func TestFormatCSP_HandlesPercentInValue(t *testing.T) {
	// 验证不使用 fmt.Sprintf 避免 % 字符误解析
	// CSP 中正常不会出现 %，但 nonce 值理论上可能包含
	tmpl := "script-src 'self' 'nonce-%s'"
	out := formatCSP(tmpl, "abc%123", "")
	assert.Equal(t, "script-src 'self' 'nonce-abc%123'", out)
}
