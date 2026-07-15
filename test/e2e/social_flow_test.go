//go:build e2e

// Package e2e 社交登录端到端测试（骨架）
// TODO: 完整社交登录 E2E 待后续实现（需 mock OAuth 提供商或使用测试账号）
package e2e

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ============================================================================
// 提供商列表端点
// ============================================================================

// TestSocialLogin_ProvidersEndpoint 验证 /auth/providers 返回支持的提供商列表
func TestSocialLogin_ProvidersEndpoint(t *testing.T) {
	resp, body, err := doRequest("GET", "/auth/providers", nil, "")
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode,
		"期望200，实际%d，body=%s", resp.StatusCode, string(body))

	// providers 端点返回 JSON 数组 [{name, label, icon}, ...]
	var providers []map[string]string
	err = json.Unmarshal(body, &providers)
	require.NoError(t, err, "响应应可解析为提供商数组")
	assert.GreaterOrEqual(t, len(providers), 2, "应至少返回2个提供商（google, github）")

	// 收集所有提供商名称
	names := make(map[string]bool, len(providers))
	for _, p := range providers {
		if name, ok := p["name"]; ok {
			names[name] = true
		}
	}
	assert.True(t, names["google"], "应包含 google 提供商")
	assert.True(t, names["github"], "应包含 github 提供商")
}

// ============================================================================
// 登录重定向端点（骨架）
// ============================================================================

// TestSocialLogin_LoginRedirect 验证 /auth/{provider} 端点存在
// 完整重定向流程待后续实现（需配置真实 OAuth 提供商凭据）
func TestSocialLogin_LoginRedirect(t *testing.T) {
	resp, _, err := doRequest("GET", "/auth/github", nil, "")
	require.NoError(t, err)
	// 端点应存在（非 404）；实际响应可能是 307 重定向（已配置凭据）
	// 或 400/500（未配置凭据），但不应是 404
	assert.NotEqual(t, http.StatusNotFound, resp.StatusCode,
		"/auth/github 端点应存在，不应返回404")
}

// ============================================================================
// 回调端点（骨架）
// ============================================================================

// TestSocialLogin_CallbackEndpoint 验证 /auth/{provider}/callback 端点存在
// 完整回调流程待后续实现（需 mock OAuth 提供商返回 code）
func TestSocialLogin_CallbackEndpoint(t *testing.T) {
	// 使用无效的 code/state，验证端点存在（非 404）
	// 实际响应应为 400（code/state 无效）或 500（OAuth 配置缺失）
	resp, _, err := doRequest("GET", "/auth/github/callback?code=test&state=test", nil, "")
	require.NoError(t, err)
	assert.NotEqual(t, http.StatusNotFound, resp.StatusCode,
		"/auth/github/callback 端点应存在，不应返回404")
}
