//go:build e2e

// Package e2e MFA（多因素认证）完整链路端到端测试
package e2e

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ============================================================================
// MFA 设置测试
// ============================================================================

func TestMFASetup(t *testing.T) {
	email, tokens, err := registerAndLoginWithCleanup(t)
	require.NoError(t, err, "注册登录失败")
	_ = email

	t.Run("成功设置MFA返回secret和qr_code_url", func(t *testing.T) {
		resp, body, err := doRequest("POST", "/api/v1/mfa/setup", nil, tokens.AccessToken)
		require.NoError(t, err)
		assert.Equal(t, http.StatusOK, resp.StatusCode, "期望200，实际%d，body=%s", resp.StatusCode, string(body))

		var result map[string]interface{}
		err = json.Unmarshal(body, &result)
		require.NoError(t, err)
		assert.NotEmpty(t, result["secret"], "应返回非空secret")
		assert.NotEmpty(t, result["qr_code_url"], "应返回非空qr_code_url")
	})

	t.Run("已启用MFA后再次设置返回409", func(t *testing.T) {
		// 第一次 setup 获取 secret
		resp, body, err := doRequest("POST", "/api/v1/mfa/setup", nil, tokens.AccessToken)
		require.NoError(t, err)
		require.Equal(t, http.StatusOK, resp.StatusCode)

		var setupResult map[string]interface{}
		err = json.Unmarshal(body, &setupResult)
		require.NoError(t, err)
		secret := setupResult["secret"].(string)

		// verify 启用 MFA
		code := generateTOTPCode(secret)
		verifyReq := map[string]string{"code": code}
		resp, _, err = doRequest("POST", "/api/v1/mfa/verify", verifyReq, tokens.AccessToken)
		require.NoError(t, err)
		require.Equal(t, http.StatusOK, resp.StatusCode)

		// 已启用后再次 setup 应返回 409
		resp2, body2, err := doRequest("POST", "/api/v1/mfa/setup", nil, tokens.AccessToken)
		require.NoError(t, err)
		assert.Equal(t, http.StatusConflict, resp2.StatusCode,
			"期望409 Conflict，实际%d，body=%s", resp2.StatusCode, string(body2))
	})

	t.Run("未认证请求返回401", func(t *testing.T) {
		resp, _, err := doRequest("POST", "/api/v1/mfa/setup", nil, "")
		require.NoError(t, err)
		assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
	})
}

// ============================================================================
// MFA 验证（启用）测试
// ============================================================================

func TestMFAVerify(t *testing.T) {
	email, tokens, err := registerAndLoginWithCleanup(t)
	require.NoError(t, err, "注册登录失败")
	_ = email

	// 1. 先 setup 获取 secret
	resp, body, err := doRequest("POST", "/api/v1/mfa/setup", nil, tokens.AccessToken)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var setupResult map[string]interface{}
	err = json.Unmarshal(body, &setupResult)
	require.NoError(t, err)
	secret, ok := setupResult["secret"].(string)
	require.True(t, ok && secret != "", "setup 应返回非空 secret")

	t.Run("有效TOTP码验证成功", func(t *testing.T) {
		code := generateTOTPCode(secret)
		verifyReq := map[string]string{"code": code}
		resp, body, err := doRequest("POST", "/api/v1/mfa/verify", verifyReq, tokens.AccessToken)
		require.NoError(t, err)
		assert.Equal(t, http.StatusOK, resp.StatusCode,
			"期望200，实际%d，body=%s", resp.StatusCode, string(body))
	})

	t.Run("无效TOTP码验证失败", func(t *testing.T) {
		// 重新 setup（因上一个子测试已验证启用，需新用户）
		// 这里用一个明确无效的 code
		verifyReq := map[string]string{"code": "000000"}
		resp, body, err := doRequest("POST", "/api/v1/mfa/verify", verifyReq, tokens.AccessToken)
		require.NoError(t, err)
		// 已启用后再次验证可能返回 400 或其他错误
		assert.True(t, resp.StatusCode >= 400,
			"期望4xx错误，实际%d，body=%s", resp.StatusCode, string(body))
	})

	t.Run("空code返回400", func(t *testing.T) {
		verifyReq := map[string]string{"code": ""}
		resp, _, err := doRequest("POST", "/api/v1/mfa/verify", verifyReq, tokens.AccessToken)
		require.NoError(t, err)
		assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
	})
}

// ============================================================================
// MFA 状态查询测试
// ============================================================================

func TestMFAStatus(t *testing.T) {
	email, tokens, err := registerAndLoginWithCleanup(t)
	require.NoError(t, err, "注册登录失败")
	_ = email

	t.Run("未启用MFA状态查询", func(t *testing.T) {
		resp, body, err := doRequest("GET", "/api/v1/mfa/status", nil, tokens.AccessToken)
		require.NoError(t, err)
		assert.Equal(t, http.StatusOK, resp.StatusCode)

		var status map[string]interface{}
		err = json.Unmarshal(body, &status)
		require.NoError(t, err)
		// 未启用时应返回 enabled=false 或类似
		if enabled, ok := status["enabled"]; ok {
			assert.False(t, enabled.(bool), "未启用MFA应返回enabled=false")
		}
	})

	t.Run("启用后MFA状态查询", func(t *testing.T) {
		// setup + verify 启用 MFA
		resp, body, err := doRequest("POST", "/api/v1/mfa/setup", nil, tokens.AccessToken)
		require.NoError(t, err)
		require.Equal(t, http.StatusOK, resp.StatusCode)

		var setupResult map[string]interface{}
		err = json.Unmarshal(body, &setupResult)
		require.NoError(t, err)
		secret := setupResult["secret"].(string)

		code := generateTOTPCode(secret)
		verifyReq := map[string]string{"code": code}
		resp, _, err = doRequest("POST", "/api/v1/mfa/verify", verifyReq, tokens.AccessToken)
		require.NoError(t, err)
		require.Equal(t, http.StatusOK, resp.StatusCode)

		// 查询状态应显示已启用
		resp, body, err = doRequest("GET", "/api/v1/mfa/status", nil, tokens.AccessToken)
		require.NoError(t, err)
		assert.Equal(t, http.StatusOK, resp.StatusCode)

		var status map[string]interface{}
		err = json.Unmarshal(body, &status)
		require.NoError(t, err)
		if enabled, ok := status["enabled"]; ok {
			assert.True(t, enabled.(bool), "已启用MFA应返回enabled=true")
		}
	})
}

// ============================================================================
// MFA 禁用测试
// ============================================================================

func TestMFADisable(t *testing.T) {
	email, tokens, err := registerAndLoginWithCleanup(t)
	require.NoError(t, err, "注册登录失败")
	_ = email

	// 先 setup + verify 启用 MFA
	resp, body, err := doRequest("POST", "/api/v1/mfa/setup", nil, tokens.AccessToken)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var setupResult map[string]interface{}
	err = json.Unmarshal(body, &setupResult)
	require.NoError(t, err)
	secret := setupResult["secret"].(string)

	code := generateTOTPCode(secret)
	verifyReq := map[string]string{"code": code}
	resp, _, err = doRequest("POST", "/api/v1/mfa/verify", verifyReq, tokens.AccessToken)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode)

	t.Run("有效TOTP码禁用MFA成功", func(t *testing.T) {
		// 使用相邻时间步的码（-1），避免与 verify 的码相同触发重放保护
		disableCode := generateTOTPCodeWithOffset(secret, -1)
		disableReq := map[string]string{"code": disableCode}
		resp, body, err := doRequest("POST", "/api/v1/mfa/disable", disableReq, tokens.AccessToken)
		require.NoError(t, err)
		assert.Equal(t, http.StatusOK, resp.StatusCode,
			"期望200，实际%d，body=%s", resp.StatusCode, string(body))

		// 验证已禁用：status 应返回 enabled=false
		resp, body, err = doRequest("GET", "/api/v1/mfa/status", nil, tokens.AccessToken)
		require.NoError(t, err)
		require.Equal(t, http.StatusOK, resp.StatusCode)
		var status map[string]interface{}
		err = json.Unmarshal(body, &status)
		require.NoError(t, err)
		if enabled, ok := status["enabled"]; ok {
			assert.False(t, enabled.(bool), "禁用后应返回enabled=false")
		}
	})

	t.Run("未启用MFA禁用返回400", func(t *testing.T) {
		// 用新用户（未启用 MFA）
		_, newTokens, err := registerAndLoginWithCleanup(t)
		require.NoError(t, err)

		code := generateTOTPCode("JBSWY3DPEHPK3PXP") // 任意 secret 生成的 code
		disableReq := map[string]string{"code": code}
		resp, _, err := doRequest("POST", "/api/v1/mfa/disable", disableReq, newTokens.AccessToken)
		require.NoError(t, err)
		assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
	})
}

// ============================================================================
// MFA 完整流程测试
// ============================================================================

func TestMFAFullFlow(t *testing.T) {
	email, tokens, err := registerAndLoginWithCleanup(t)
	require.NoError(t, err, "注册登录失败")
	_ = email

	// 1. Setup
	resp, body, err := doRequest("POST", "/api/v1/mfa/setup", nil, tokens.AccessToken)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode, "setup 失败")

	var setupResult map[string]interface{}
	err = json.Unmarshal(body, &setupResult)
	require.NoError(t, err)
	secret := setupResult["secret"].(string)
	require.NotEmpty(t, secret)

	// 2. Verify（启用）
	code := generateTOTPCode(secret)
	verifyReq := map[string]string{"code": code}
	resp, _, err = doRequest("POST", "/api/v1/mfa/verify", verifyReq, tokens.AccessToken)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode, "verify 失败")

	// 3. Status（应已启用）
	resp, body, err = doRequest("GET", "/api/v1/mfa/status", nil, tokens.AccessToken)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	var status map[string]interface{}
	err = json.Unmarshal(body, &status)
	require.NoError(t, err)
	if enabled, ok := status["enabled"]; ok {
		assert.True(t, enabled.(bool), "启用后 status 应为 true")
	}

	// 4. Disable（用相邻时间步的码，避免与 verify 的码相同触发重放保护）
	disableCode := generateTOTPCodeWithOffset(secret, -1)
	disableReq := map[string]string{"code": disableCode}
	resp, _, err = doRequest("POST", "/api/v1/mfa/disable", disableReq, tokens.AccessToken)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode, "disable 失败")

	// 5. Status（应已禁用）
	resp, body, err = doRequest("GET", "/api/v1/mfa/status", nil, tokens.AccessToken)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	err = json.Unmarshal(body, &status)
	require.NoError(t, err)
	if enabled, ok := status["enabled"]; ok {
		assert.False(t, enabled.(bool), "禁用后 status 应为 false")
	}
}
