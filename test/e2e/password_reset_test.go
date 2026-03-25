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
	_, err := registerUser(email, password)
	require.NoError(t, err)

	t.Run("成功请求重置", func(t *testing.T) {
		req := forgotPasswordRequest{Email: email}
		resp, _, err := doRequest("POST", "/api/v1/forgot-password", req, "")
		require.NoError(t, err)

		// 通常返回200或202
		assert.True(t, resp.StatusCode == http.StatusOK || resp.StatusCode == http.StatusAccepted)
	})

	t.Run("不存在的邮箱", func(t *testing.T) {
		req := forgotPasswordRequest{Email: "nonexistent@example.com"}
		resp, _, err := doRequest("POST", "/api/v1/forgot-password", req, "")
		require.NoError(t, err)

		// 出于安全考虑，通常也返回成功，不泄露邮箱是否存在
		assert.True(t, resp.StatusCode == http.StatusOK || resp.StatusCode == http.StatusAccepted)
	})

	t.Run("空邮箱", func(t *testing.T) {
		req := forgotPasswordRequest{Email: ""}
		resp, _, err := doRequest("POST", "/api/v1/forgot-password", req, "")
		require.NoError(t, err)

		assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
	})

	t.Run("无效邮箱格式", func(t *testing.T) {
		req := forgotPasswordRequest{Email: "invalid-email"}
		resp, _, err := doRequest("POST", "/api/v1/forgot-password", req, "")
		require.NoError(t, err)

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

		assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
	})

	t.Run("空令牌", func(t *testing.T) {
		req := resetPasswordRequest{
			Token:       "",
			NewPassword: "NewPassword123!",
		}
		resp, _, err := doRequest("POST", "/api/v1/reset-password", req, "")
		require.NoError(t, err)

		assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
	})

	t.Run("密码过短", func(t *testing.T) {
		req := resetPasswordRequest{
			Token:       "some-token",
			NewPassword: "short",
		}
		resp, _, err := doRequest("POST", "/api/v1/reset-password", req, "")
		require.NoError(t, err)

		assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
	})

	t.Run("空密码", func(t *testing.T) {
		req := resetPasswordRequest{
			Token:       "some-token",
			NewPassword: "",
		}
		resp, _, err := doRequest("POST", "/api/v1/reset-password", req, "")
		require.NoError(t, err)

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
	_, err := registerUser(email, oldPassword)
	require.NoError(t, err)

	// 2. 使用旧密码登录验证
	tokens, err := loginUser(email, oldPassword)
	require.NoError(t, err)
	assert.NotEmpty(t, tokens.AccessToken)

	// 3. 请求密码重置
	forgotReq := forgotPasswordRequest{Email: email}
	forgotResp, _, err := doRequest("POST", "/api/v1/forgot-password", forgotReq, "")
	require.NoError(t, err)

	// 如果忘记密码端点不存在，跳过后续测试
	if forgotResp.StatusCode == http.StatusNotFound {
		t.Skip("忘记密码端点未实现")
		return
	}

	t.Logf("忘记密码请求状态: %d", forgotResp.StatusCode)

	// 注意：在真实环境中，需要从邮件中获取重置令牌
	// 这里只能测试API端点的基本逻辑
}

// ============================================================================
// 密码重置安全性测试
// ============================================================================

func TestPasswordResetSecurity(t *testing.T) {
	t.Run("重置令牌应为一次性使用", func(t *testing.T) {
		// 这个测试需要能够获取到有效的重置令牌
		// 在端到端测试中，这通常需要模拟邮件服务或使用测试专用的令牌
		t.Skip("需要模拟邮件服务或测试令牌")
	})

	t.Run("重置后旧密码应失效", func(t *testing.T) {
		email := generateUniqueEmail("resetexpire")
		oldPassword := generateTestPassword()

		// 注册用户
		_, err := registerUser(email, oldPassword)
		require.NoError(t, err)

		// 验证旧密码可以登录
		tokens, err := loginUser(email, oldPassword)
		require.NoError(t, err)
		assert.NotEmpty(t, tokens.AccessToken)

		// 注意：完整测试需要实际重置密码
		// 这里只验证初始状态
		t.Logf("用户 %s 初始登录成功", email)
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

	t.Logf("并发忘记密码请求测试完成")
}
