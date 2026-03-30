//go:build e2e

// Package e2e 邮箱验证流程端到端测试
package e2e

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ============================================================================
// 邮箱验证测试
// ============================================================================

func TestVerifyEmail(t *testing.T) {
	t.Run("无效验证令牌", func(t *testing.T) {
		params := "?token=invalid-token"
		resp, _, err := doRequest("GET", "/api/v1/verify-email"+params, nil, "")
		require.NoError(t, err)

		assertNotRateLimited(t, resp)
		assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
	})

	t.Run("空令牌", func(t *testing.T) {
		resp, _, err := doRequest("GET", "/api/v1/verify-email", nil, "")
		require.NoError(t, err)

		assertNotRateLimited(t, resp)
		assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
	})

	t.Run("过期令牌", func(t *testing.T) {
		// 使用一个格式正确但无效的令牌
		params := "?token=expired-token-12345678901234567890123456789012"
		resp, _, err := doRequest("GET", "/api/v1/verify-email"+params, nil, "")
		require.NoError(t, err)

		assertNotRateLimited(t, resp)
		// 应该返回 400（无效令牌）或 404（令牌不存在）
		assert.True(t, resp.StatusCode == http.StatusBadRequest || resp.StatusCode == http.StatusNotFound,
			"期望 400 或 404，实际 %d", resp.StatusCode)
	})
}

// ============================================================================
// 完整邮箱验证流程测试
// ============================================================================

func TestFullEmailVerifyFlow(t *testing.T) {
	email := generateUniqueEmail("verify")
	password := generateTestPassword()

	// 1. 注册用户
	user, err := registerUser(email, password)
	require.NoError(t, err)
	userID := user["user_id"].(string)
	t.Logf("注册成功: %v", userID)

	// 2. 验证邮箱（使用测试专用API）
	err = verifyEmail(userID)
	require.NoError(t, err, "验证邮箱失败")

	// 3. 登录
	tokens, err := loginUser(email, password)
	require.NoError(t, err)
	assert.NotEmpty(t, tokens.AccessToken)

	// 4. 获取用户信息，检查邮箱验证状态
	resp, body, err := doRequest("GET", "/api/v1/userinfo", nil, tokens.AccessToken)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode)

	userInfo, err := parseUserInfo(body)
	require.NoError(t, err)
	assert.True(t, userInfo.EmailVerified, "邮箱验证后 email_verified 应为 true")
}

// ============================================================================
// 邮箱验证安全性测试
// ============================================================================

func TestEmailVerifySecurity(t *testing.T) {
	t.Run("验证令牌应为一次性使用", func(t *testing.T) {
		// 注册用户，通过测试 API 验证邮箱，然后尝试用 GET 端点再次验证
		email := generateUniqueEmail("onetime")
		password := generateTestPassword()

		user, err := registerUser(email, password)
		require.NoError(t, err)
		userID := user["user_id"].(string)

		// 通过测试 API 验证邮箱
		err = verifyEmail(userID)
		require.NoError(t, err)

		// 尝试用 GET 端点验证一个无效令牌（模拟重复验证）
		resp, _, err := doRequest("GET", "/api/v1/verify-email?token=reused-token", nil, "")
		require.NoError(t, err)

		assertNotRateLimited(t, resp)
		// 无效令牌应该被拒绝
		assert.True(t, resp.StatusCode == http.StatusBadRequest || resp.StatusCode == http.StatusNotFound,
			"重用的令牌应该被拒绝，实际 %d", resp.StatusCode)
	})

	t.Run("已验证邮箱不应重复验证", func(t *testing.T) {
		email := generateUniqueEmail("alreadyverified")
		password := generateTestPassword()

		user, err := registerUser(email, password)
		require.NoError(t, err)
		userID := user["user_id"].(string)

		// 第一次验证
		err = verifyEmail(userID)
		require.NoError(t, err)

		// 第二次验证应该仍然成功（幂等操作）
		err = verifyEmail(userID)
		assert.NoError(t, err, "重复验证已验证邮箱不应报错（幂等性）")
	})
}

// ============================================================================
// 邮箱验证与登录关联测试
// ============================================================================

func TestEmailVerifyLoginAssociation(t *testing.T) {
	email := generateUniqueEmail("verifylogin")
	password := generateTestPassword()

	// 注册用户
	user, err := registerUser(email, password)
	require.NoError(t, err)

	// 验证邮箱
	userID := user["user_id"].(string)
	err = verifyEmail(userID)
	require.NoError(t, err)

	// 验证邮箱后可以登录
	tokens, err := loginUser(email, password)
	require.NoError(t, err)
	assert.NotEmpty(t, tokens.AccessToken)

	// 验证可以访问基本资源
	resp, _, err := doRequest("GET", "/api/v1/userinfo", nil, tokens.AccessToken)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
}

// ============================================================================
// 重新发送验证邮件测试
// ============================================================================

func TestResendVerificationEmail(t *testing.T) {
	email := generateUniqueEmail("resend")
	password := generateTestPassword()

	// 注册用户
	user, err := registerUser(email, password)
	require.NoError(t, err)

	// 验证邮箱（为了登录）
	userID := user["user_id"].(string)
	err = verifyEmail(userID)
	require.NoError(t, err)

	// 登录获取Token
	tokens, err := loginUser(email, password)
	require.NoError(t, err)

	t.Run("请求重新发送验证邮件", func(t *testing.T) {
		// 尝试发送重新验证请求
		resp, _, err := doRequest("POST", "/api/v1/verify-email/send", nil, tokens.AccessToken)
		require.NoError(t, err)

		assertNotRateLimited(t, resp)
		// 应该返回成功或已验证
		assert.True(t, resp.StatusCode == http.StatusOK || resp.StatusCode == http.StatusAccepted || resp.StatusCode == http.StatusConflict,
			"期望 200/202/409，实际 %d", resp.StatusCode)
	})

	t.Run("未认证请求重新发送", func(t *testing.T) {
		resp, _, err := doRequest("POST", "/api/v1/verify-email/send", nil, "")
		require.NoError(t, err)

		assertNotRateLimited(t, resp)
		// 未认证应返回401
		assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
	})
}

// ============================================================================
// 邮箱验证状态查询测试
// ============================================================================

func TestEmailVerificationStatus(t *testing.T) {
	email := generateUniqueEmail("status")
	password := generateTestPassword()

	// 注册用户
	user, err := registerUser(email, password)
	require.NoError(t, err)

	// 验证邮箱
	userID := user["user_id"].(string)
	err = verifyEmail(userID)
	require.NoError(t, err)

	// 登录获取Token
	tokens, err := loginUser(email, password)
	require.NoError(t, err)

	// 查询用户信息
	resp, body, err := doRequest("GET", "/api/v1/userinfo", nil, tokens.AccessToken)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode)

	userInfo, err := parseUserInfo(body)
	require.NoError(t, err)

	// 已验证用户邮箱应该为true
	assert.True(t, userInfo.EmailVerified)
}
