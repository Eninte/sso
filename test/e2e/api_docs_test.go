//go:build e2e

// Package e2e API 文档调试前端端到端测试
//
// 验证内容：
//   - 3 个新增路由的 HTTP 行为（HTML / OpenAPI JSON / Scalar JS）
//   - 鉴权链路：匿名拒绝、非管理员拒绝、管理员通过
//   - 运行时 baseURL 注入到 OpenAPI servers
//   - Scalar JS 是非空的离线内嵌资源
//
// 前置条件：服务已启动、限流已禁用、E2E_ADMIN_EMAIL/PASSWORD 已设置
package e2e

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ============================================================================
// 鉴权链路测试
// ============================================================================

// TestAPIDocs_AnonymousAccessDenied 匿名访问应被 AuthMiddleware 拦截
func TestAPIDocs_AnonymousAccessDenied(t *testing.T) {
	paths := []string{
		"/api/v1/admin/api-docs",
		"/api/v1/admin/api-docs/openapi.json",
		"/api/v1/admin/api-docs/scalar.js",
	}
	for _, p := range paths {
		t.Run(p, func(t *testing.T) {
			resp, _, err := doRequest("GET", p, nil, "")
			require.NoError(t, err, "请求不应失败")
			assert.True(t, resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden,
				"匿名访问 %s 应被拒绝（401/403），实际 %d", p, resp.StatusCode)
		})
	}
}

// TestAPIDocs_NonAdminAccessDenied 非管理员 token 应被 RequireAdmin 拦截
func TestAPIDocs_NonAdminAccessDenied(t *testing.T) {
	// 注册并登录一个普通用户
	// registerUser 已内置从 DB 查 user_id 的兜底（helpers.go:510-525）
	user, err := registerUser("api-docs-user@example.com", "TestPassword123!")
	require.NoError(t, err, "注册普通用户失败")

	userID, _ := user["user_id"].(string)
	require.NotEmpty(t, userID, "注册响应应包含 user_id（由 registerUser 兜底注入）")
	require.NoError(t, verifyEmail(userID), "验证邮箱失败")

	tokens, err := loginUser("api-docs-user@example.com", "TestPassword123!")
	require.NoError(t, err, "普通用户登录失败")

	paths := []string{
		"/api/v1/admin/api-docs",
		"/api/v1/admin/api-docs/openapi.json",
		"/api/v1/admin/api-docs/scalar.js",
	}
	for _, p := range paths {
		t.Run(p, func(t *testing.T) {
			resp, _, err := doRequest("GET", p, nil, tokens.AccessToken)
			require.NoError(t, err, "请求不应失败")
			assert.Equal(t, http.StatusForbidden, resp.StatusCode,
				"非管理员访问 %s 应返回 403，实际 %d", p, resp.StatusCode)
		})
	}
}

// ============================================================================
// 管理员访问测试
// ============================================================================

// TestAPIDocs_AdminCanAccessHTML 管理员可访问 Scalar HTML 页面
func TestAPIDocs_AdminCanAccessHTML(t *testing.T) {
	adminTokens, err := loginAdmin()
	require.NoError(t, err, "管理员登录失败")

	resp, body, err := doRequest("GET", "/api/v1/admin/api-docs", nil, adminTokens.AccessToken)
	require.NoError(t, err, "请求失败")

	require.Equal(t, http.StatusOK, resp.StatusCode, "管理员访问 HTML 应返回 200")

	// 验证 Content-Type
	assert.Contains(t, resp.Header.Get("Content-Type"), "text/html",
		"Content-Type 应为 text/html")

	// 验证 CSP 离线策略
	csp := resp.Header.Get("Content-Security-Policy")
	assert.NotEmpty(t, csp, "CSP 必须设置")
	assert.NotContains(t, csp, "cdn.jsdelivr.net", "离线模式不应允许 CDN")
	assert.Contains(t, csp, "script-src 'self'", "script-src 应为 'self'")

	// 验证 HTML 内容
	bodyStr := string(body)
	assert.Contains(t, bodyStr, "<!DOCTYPE html>", "应为 HTML 文档")
	assert.Contains(t, bodyStr, "api-reference", "应包含 Scalar web component")
	assert.Contains(t, bodyStr, "/api/v1/admin/api-docs/openapi.json", "应注入 specURL")
	assert.Contains(t, bodyStr, "/api/v1/admin/api-docs/scalar.js", "应注入 scalarJSURL")
}

// TestAPIDocs_AdminCanAccessOpenAPISpec 管理员可访问 OpenAPI 规范
func TestAPIDocs_AdminCanAccessOpenAPISpec(t *testing.T) {
	adminTokens, err := loginAdmin()
	require.NoError(t, err, "管理员登录失败")

	resp, body, err := doRequest("GET", "/api/v1/admin/api-docs/openapi.json", nil, adminTokens.AccessToken)
	require.NoError(t, err, "请求失败")

	require.Equal(t, http.StatusOK, resp.StatusCode, "管理员访问 OpenAPI 应返回 200")
	assert.Contains(t, resp.Header.Get("Content-Type"), "application/json",
		"Content-Type 应为 application/json")

	// 验证是合法的 OpenAPI 3.0 规范
	var spec map[string]interface{}
	require.NoError(t, json.Unmarshal(body, &spec), "应返回合法 JSON")
	assert.Equal(t, "3.0.3", spec["openapi"], "应为 OpenAPI 3.0.3")
	assert.NotNil(t, spec["paths"], "应包含 paths")
	assert.NotNil(t, spec["components"], "应包含 components")

	// 验证运行时 baseURL 注入到 servers
	servers, ok := spec["servers"].([]interface{})
	require.True(t, ok, "servers 应为数组")
	require.Len(t, servers, 1, "应有 1 个 server")
	server := servers[0].(map[string]interface{})
	assert.NotEmpty(t, server["url"], "应注入非空 baseURL")
	assert.True(t, strings.HasPrefix(server["url"].(string), "http"),
		"baseURL 应为 HTTP(S) URL，实际: %v", server["url"])

	// 验证关键端点存在
	paths := spec["paths"].(map[string]interface{})
	criticalPaths := []string{
		"/health",
		"/.well-known/openid-configuration",
		"/api/v1/login",
		"/api/v1/token",
		"/api/v1/userinfo",
		"/api/v1/admin/users",
	}
	for _, p := range criticalPaths {
		assert.Contains(t, paths, p, "OpenAPI 应包含端点 %s", p)
	}
}

// TestAPIDocs_AdminCanAccessScalarJS 管理员可访问 Scalar JS（离线内嵌）
func TestAPIDocs_AdminCanAccessScalarJS(t *testing.T) {
	adminTokens, err := loginAdmin()
	require.NoError(t, err, "管理员登录失败")

	resp, body, err := doRequest("GET", "/api/v1/admin/api-docs/scalar.js", nil, adminTokens.AccessToken)
	require.NoError(t, err, "请求失败")

	require.Equal(t, http.StatusOK, resp.StatusCode, "管理员访问 Scalar JS 应返回 200")

	// 验证 Content-Type
	ct := resp.Header.Get("Content-Type")
	assert.Contains(t, ct, "application/javascript", "Content-Type 应为 application/javascript")

	// 验证 Cache-Control
	cc := resp.Header.Get("Cache-Control")
	assert.Contains(t, cc, "max-age=86400", "应设置 1 天缓存")

	// 验证是 Scalar JS（非空且包含特征字符串）
	require.NotEmpty(t, body, "JS 内容不应为空")

	// 首部应包含 Scalar 标识
	previewLen := 500
	if len(body) < previewLen {
		previewLen = len(body)
	}
	preview := string(body[:previewLen])
	assert.True(t,
		strings.Contains(strings.ToLower(preview), "scalar"),
		"JS 首部应包含 scalar 标识")
}
