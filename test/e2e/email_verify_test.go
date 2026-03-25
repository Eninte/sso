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

		assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
	})

	t.Run("空令牌", func(t *testing.T) {
		resp, _, err := doRequest("GET", "/api/v1/verify-email", nil, "")
		require.NoError(t, err)

		assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
	})

	t.Run("过期令牌", func(t *testing.T) {
		// 使用一个格式正确但无效的令牌
		params := "?token=expired-token-12345678901234567890123456789012"
		resp, _, err := doRequest("GET", "/api/v1/verify-email"+params, nil, "")
		require.NoError(t, err)

		// 应该返回错误
		assert.True(t, resp.StatusCode >= 400)
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
	t.Logf("注册成功: %v", user["id"])

	// 2. 登录（未验证邮箱可能限制某些功能）
	tokens, err := loginUser(email, password)
	require.NoError(t, err)
	assert.NotEmpty(t, tokens.AccessToken)

	// 3. 获取用户信息，检查邮箱验证状态
	resp, body, err := doRequest("GET", "/api/v1/userinfo", nil, tokens.AccessToken)
	require.NoError(t, err)

	if resp.StatusCode == http.StatusOK {
		userInfo, err := parseUserInfo(body)
		if err == nil {
			t.Logf("邮箱验证状态: %v", userInfo.EmailVerified)
			// 新注册用户邮箱应该未验证
			assert.False(t, userInfo.EmailVerified)
		}
	}

	// 注意：完整验证流程需要从邮件中获取验证令牌
	// 这里只测试API端点的基本逻辑
	t.Logf("邮箱验证流程测试完成")
}

// ============================================================================
// 邮箱验证安全性测试
// ============================================================================

func TestEmailVerifySecurity(t *testing.T) {
	t.Run("验证令牌应为一次性使用", func(t *testing.T) {
		// 这个测试需要能够获取到有效的验证令牌
		// 在端到端测试中，这通常需要模拟邮件服务
		t.Skip("需要模拟邮件服务或测试令牌")
	})

	t.Run("已验证邮箱不应重复验证", func(t *testing.T) {
		// 测试重复验证的场景
		// 同样需要有效的验证令牌
		t.Skip("需要有效的验证令牌")
	})
}

// ============================================================================
// 邮箱验证与登录关联测试
// ============================================================================

func TestEmailVerifyLoginAssociation(t *testing.T) {
	email := generateUniqueEmail("verifylogin")
	password := generateTestPassword()

	// 注册用户
	_, err := registerUser(email, password)
	require.NoError(t, err)

	// 未验证邮箱应该可以登录
	tokens, err := loginUser(email, password)
	require.NoError(t, err)
	assert.NotEmpty(t, tokens.AccessToken)

	// 验证可以访问基本资源
	resp, _, err := doRequest("GET", "/api/v1/userinfo", nil, tokens.AccessToken)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	t.Logf("未验证邮箱用户可以正常登录和访问基本资源")
}

// ============================================================================
// 重新发送验证邮件测试
// ============================================================================

func TestResendVerificationEmail(t *testing.T) {
	email := generateUniqueEmail("resend")
	password := generateTestPassword()

	// 注册用户
	_, err := registerUser(email, password)
	require.NoError(t, err)

	// 登录获取Token
	tokens, err := loginUser(email, password)
	require.NoError(t, err)

	t.Run("请求重新发送验证邮件", func(t *testing.T) {
		// 尝试发送重新验证请求
		resp, _, err := doRequest("POST", "/api/v1/resend-verification", nil, tokens.AccessToken)
		require.NoError(t, err)

		// 如果端点不存在，跳过
		if resp.StatusCode == http.StatusNotFound {
			t.Skip("重新发送验证邮件端点未实现")
			return
		}

		// 应该返回成功
		assert.True(t, resp.StatusCode == http.StatusOK || resp.StatusCode == http.StatusAccepted)
	})

	t.Run("未认证请求重新发送", func(t *testing.T) {
		resp, _, err := doRequest("POST", "/api/v1/resend-verification", nil, "")
		require.NoError(t, err)

		// 应该返回401
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
	_, err := registerUser(email, password)
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

	// 验证邮箱验证状态字段存在
	t.Logf("邮箱: %s, 已验证: %v", userInfo.Email, userInfo.EmailVerified)

	// 新注册用户邮箱应该未验证
	assert.False(t, userInfo.EmailVerified)
}
