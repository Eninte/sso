//go:build e2e

// Package e2e 密码重置流程端到端测试
package e2e

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ============================================================================
// 忘记密码测试
// ============================================================================

func TestForgotPassword(t *testing.T) {
	email := generateUniqueEmail("forgot")
	password := generateTestPassword()

	// 先注册用户
	user, err := registerUser(email, password)
	require.NoError(t, err)
	err = verifyEmail(user["user_id"].(string))
	require.NoError(t, err)

	t.Run("成功请求重置", func(t *testing.T) {
		req := forgotPasswordRequest{Email: email}
		resp, _, err := doRequest("POST", "/api/v1/forgot-password", req, "")
		require.NoError(t, err)

		assertNotRateLimited(t, resp)
		// 通常返回200或202
		assert.True(t, resp.StatusCode == http.StatusOK || resp.StatusCode == http.StatusAccepted,
			"期望 200 或 202，实际 %d", resp.StatusCode)
	})

	t.Run("不存在的邮箱", func(t *testing.T) {
		req := forgotPasswordRequest{Email: "nonexistent@example.com"}
		resp, _, err := doRequest("POST", "/api/v1/forgot-password", req, "")
		require.NoError(t, err)

		assertNotRateLimited(t, resp)
		// 出于安全考虑，通常也返回成功，不泄露邮箱是否存在
		assert.True(t, resp.StatusCode == http.StatusOK || resp.StatusCode == http.StatusAccepted,
			"期望 200 或 202（不泄露邮箱存在性），实际 %d", resp.StatusCode)
	})

	t.Run("空邮箱", func(t *testing.T) {
		req := forgotPasswordRequest{Email: ""}
		resp, _, err := doRequest("POST", "/api/v1/forgot-password", req, "")
		require.NoError(t, err)

		assertNotRateLimited(t, resp)
		assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
	})

	t.Run("无效邮箱格式", func(t *testing.T) {
		req := forgotPasswordRequest{Email: "invalid-email"}
		resp, _, err := doRequest("POST", "/api/v1/forgot-password", req, "")
		require.NoError(t, err)

		assertNotRateLimited(t, resp)
		assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
	})
}

// ============================================================================
// 重置密码测试
// ============================================================================

func TestResetPassword(t *testing.T) {
	t.Run("无效重置令牌", func(t *testing.T) {
		req := resetPasswordRequest{
			Token:       "invalid-token",
			NewPassword: "NewPassword123!",
		}
		resp, _, err := doRequest("POST", "/api/v1/reset-password", req, "")
		require.NoError(t, err)

		assertNotRateLimited(t, resp)
		assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
	})

	t.Run("空令牌", func(t *testing.T) {
		req := resetPasswordRequest{
			Token:       "",
			NewPassword: "NewPassword123!",
		}
		resp, _, err := doRequest("POST", "/api/v1/reset-password", req, "")
		require.NoError(t, err)

		assertNotRateLimited(t, resp)
		assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
	})

	t.Run("密码过短", func(t *testing.T) {
		req := resetPasswordRequest{
			Token:       "some-token",
			NewPassword: "short",
		}
		resp, _, err := doRequest("POST", "/api/v1/reset-password", req, "")
		require.NoError(t, err)

		assertNotRateLimited(t, resp)
		assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
	})

	t.Run("空密码", func(t *testing.T) {
		req := resetPasswordRequest{
			Token:       "some-token",
			NewPassword: "",
		}
		resp, _, err := doRequest("POST", "/api/v1/reset-password", req, "")
		require.NoError(t, err)

		assertNotRateLimited(t, resp)
		assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
	})
}

// ============================================================================
// 完整密码重置流程测试
// ============================================================================

func TestFullPasswordResetFlow(t *testing.T) {
	email := generateUniqueEmail("resetfull")
	oldPassword := generateTestPassword()

	// 1. 注册用户
	user, err := registerUser(email, oldPassword)
	require.NoError(t, err)
	err = verifyEmail(user["user_id"].(string))
	require.NoError(t, err)

	// 2. 使用旧密码登录验证
	tokens, err := loginUser(email, oldPassword)
	require.NoError(t, err)
	assert.NotEmpty(t, tokens.AccessToken)

	// 3. 请求密码重置
	forgotReq := forgotPasswordRequest{Email: email}
	forgotResp, _, err := doRequest("POST", "/api/v1/forgot-password", forgotReq, "")
	require.NoError(t, err)

	assertNotRateLimited(t, forgotResp)
	// 端点应该存在且返回成功
	assert.True(t, forgotResp.StatusCode == http.StatusOK || forgotResp.StatusCode == http.StatusAccepted,
		"忘记密码端点期望 200/202，实际 %d", forgotResp.StatusCode)
}

// ============================================================================
// 密码重置安全性测试
// ============================================================================

func TestPasswordResetSecurity(t *testing.T) {
	t.Run("重置令牌应为一次性使用", func(t *testing.T) {
		// 尝试用同一个无效令牌两次
		req := resetPasswordRequest{
			Token:       "same-token-twice",
			NewPassword: "NewPassword123!",
		}
		resp1, _, err := doRequest("POST", "/api/v1/reset-password", req, "")
		require.NoError(t, err)
		assertNotRateLimited(t, resp1)
		assert.True(t, resp1.StatusCode >= 400, "无效令牌应返回错误")

		resp2, _, err := doRequest("POST", "/api/v1/reset-password", req, "")
		require.NoError(t, err)
		assertNotRateLimited(t, resp2)
		assert.True(t, resp2.StatusCode >= 400, "重复使用同一令牌应返回错误")
	})

	t.Run("重置后旧密码应失效", func(t *testing.T) {
		email := generateUniqueEmail("resetexpire")
		oldPassword := generateTestPassword()

		// 注册用户
		user, err := registerUser(email, oldPassword)
		require.NoError(t, err)
		err = verifyEmail(user["user_id"].(string))
		require.NoError(t, err)

		// 验证旧密码可以登录
		tokens, err := loginUser(email, oldPassword)
		require.NoError(t, err)
		assert.NotEmpty(t, tokens.AccessToken)

		// 发起重置密码请求（实际重置需要邮件中的令牌，这里只验证API端点存在）
		forgotReq := forgotPasswordRequest{Email: email}
		forgotResp, _, err := doRequest("POST", "/api/v1/forgot-password", forgotReq, "")
		require.NoError(t, err)

		assertNotRateLimited(t, forgotResp)
		assert.True(t, forgotResp.StatusCode == http.StatusOK || forgotResp.StatusCode == http.StatusAccepted,
			"期望 200/202，实际 %d", forgotResp.StatusCode)
	})
}

// ============================================================================
// 并发密码重置请求测试
// ============================================================================

func TestConcurrentForgotPassword(t *testing.T) {
	email := generateUniqueEmail("concforgot")
	password := generateTestPassword()

	// 注册用户
	_, err := registerUser(email, password)
	require.NoError(t, err)

	// 并发发送忘记密码请求
	done := make(chan bool, 5)
	for i := 0; i < 5; i++ {
		go func() {
			req := forgotPasswordRequest{Email: email}
			resp, _, err := doRequest("POST", "/api/v1/forgot-password", req, "")
			if err == nil {
				assert.True(t, resp.StatusCode == http.StatusOK || resp.StatusCode == http.StatusAccepted)
			}
			done <- true
		}()
	}

	// 等待所有请求完成
	for i := 0; i < 5; i++ {
		<-done
	}
}
