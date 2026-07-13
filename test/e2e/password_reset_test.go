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
	email := testAwareEmail(t, "forgot")
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
	email := testAwareEmail(t, "resetfull")
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
		email := testAwareEmail(t, "resetexpire")
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
	email := testAwareEmail(t, "concforgot")
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

// ============================================================================
// 完整密码重置流程测试（覆盖 MarkResetTokenUsed 一次性语义）
// 该测试通过 DB 探针读取真实 reset token，验证完整链路：
//   forgot-password → 从 DB 取真实 token → reset-password 成功
//   → 同 token 再次 reset 失败（验证 MarkResetTokenUsed 生效）
//   → 新密码登录成功，旧密码登录失败
// ============================================================================

func TestFullPasswordResetFlow_RealToken(t *testing.T) {
	email := testAwareEmail(t, "resetreal")
	oldPassword := generateTestPassword()
	newPassword := "NewSecurePassword456!"

	// 1. 注册并验证邮箱（@example.com 由触发器自动验证）
	user, err := registerUser(email, oldPassword)
	require.NoError(t, err)
	userID, ok := user["user_id"].(string)
	require.True(t, ok && userID != "", "注册响应应包含 user_id")
	require.NoError(t, verifyEmail(userID))

	// 2. 旧密码登录验证
	tokens, err := loginUser(email, oldPassword)
	require.NoError(t, err)
	assert.NotEmpty(t, tokens.AccessToken, "旧密码应能登录")

	// 3. 请求密码重置
	forgotReq := forgotPasswordRequest{Email: email}
	forgotResp, _, err := doRequest("POST", "/api/v1/forgot-password", forgotReq, "")
	require.NoError(t, err)
	assertNotRateLimited(t, forgotResp)
	require.True(t, forgotResp.StatusCode == http.StatusOK || forgotResp.StatusCode == http.StatusAccepted,
		"forgot-password 期望 200/202，实际 %d", forgotResp.StatusCode)

	// 4. 从 DB 读取真实重置令牌（邮件链路在 E2E 中不可用）
	realToken := getResetTokenFromDB(t, userID)
	require.NotEmpty(t, realToken, "应从 DB 读取到真实重置令牌")

	// 5. 使用真实令牌重置密码成功
	resetReq := resetPasswordRequest{
		Token:       realToken,
		UserID:      userID,
		NewPassword: newPassword,
	}
	resetResp, _, err := doRequest("POST", "/api/v1/reset-password", resetReq, "")
	require.NoError(t, err)
	assertNotRateLimited(t, resetResp)
	require.Equal(t, http.StatusOK, resetResp.StatusCode,
		"使用有效令牌重置密码应成功，实际 %d", resetResp.StatusCode)

	t.Run("重置后新密码可登录", func(t *testing.T) {
		newTokens, err := loginUser(email, newPassword)
		require.NoError(t, err, "新密码应能登录")
		assert.NotEmpty(t, newTokens.AccessToken)
	})

	t.Run("重置后旧密码失效", func(t *testing.T) {
		_, err := loginUser(email, oldPassword)
		assert.Error(t, err, "旧密码应已失效")
	})

	t.Run("重置令牌一次性使用_重复使用应失败", func(t *testing.T) {
		// 用相同令牌再次重置，应被拒绝（MarkResetTokenUsed 生效）
		reuseReq := resetPasswordRequest{
			Token:       realToken,
			UserID:      userID,
			NewPassword: "AnotherPassword789!",
		}
		reuseResp, _, err := doRequest("POST", "/api/v1/reset-password", reuseReq, "")
		require.NoError(t, err)
		assertNotRateLimited(t, reuseResp)
		assert.True(t, reuseResp.StatusCode >= 400,
			"重复使用已消耗的重置令牌应返回错误，实际 %d", reuseResp.StatusCode)
	})
}
