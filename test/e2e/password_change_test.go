//go:build e2e

// Package e2e 修改密码端到端测试
package e2e

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// changePasswordRequest 修改密码请求体
type changePasswordRequest struct {
	OldPassword string `json:"old_password"`
	NewPassword string `json:"new_password"`
}

// ============================================================================
// 修改密码成功流程
// ============================================================================

func TestChangePassword_Success(t *testing.T) {
	email, tokens, err := registerAndLoginWithCleanup(t)
	require.NoError(t, err, "注册登录失败")
	oldPassword := generateTestPassword()
	newPassword := "NewPassword456!"

	// 1. 修改密码
	req := changePasswordRequest{
		OldPassword: oldPassword,
		NewPassword: newPassword,
	}
	resp, body, err := doRequest("POST", "/api/v1/change-password", req, tokens.AccessToken)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode,
		"期望200，实际%d，body=%s", resp.StatusCode, string(body))

	// 2. 用新密码登录应成功
	newTokens, err := loginUser(email, newPassword)
	require.NoError(t, err, "用新密码登录应成功")
	assert.NotEmpty(t, newTokens.AccessToken, "新密码登录应返回有效 token")

	// 3. 用旧密码登录应失败
	_, err = loginUser(email, oldPassword)
	assert.Error(t, err, "用旧密码登录应失败")
}

// ============================================================================
// 旧密码错误
// ============================================================================

func TestChangePassword_WrongOldPassword(t *testing.T) {
	_, tokens, err := registerAndLoginWithCleanup(t)
	require.NoError(t, err, "注册登录失败")

	req := changePasswordRequest{
		OldPassword: "WrongOldPassword999!",
		NewPassword: "NewPassword456!",
	}
	resp, body, err := doRequest("POST", "/api/v1/change-password", req, tokens.AccessToken)
	require.NoError(t, err)
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode,
		"旧密码错误应返回400，实际%d，body=%s", resp.StatusCode, string(body))
}

// ============================================================================
// 新密码不满足复杂度
// ============================================================================

func TestChangePassword_InvalidNewPassword(t *testing.T) {
	_, tokens, err := registerAndLoginWithCleanup(t)
	require.NoError(t, err, "注册登录失败")

	// 新密码过短，不满足复杂度要求
	req := changePasswordRequest{
		OldPassword: generateTestPassword(),
		NewPassword: "weak",
	}
	resp, body, err := doRequest("POST", "/api/v1/change-password", req, tokens.AccessToken)
	require.NoError(t, err)
	assert.True(t, resp.StatusCode >= 400,
		"弱密码应返回4xx错误，实际%d，body=%s", resp.StatusCode, string(body))
}

// ============================================================================
// 未认证请求
// ============================================================================

func TestChangePassword_Unauthorized(t *testing.T) {
	req := changePasswordRequest{
		OldPassword: "SomePassword123!",
		NewPassword: "NewPassword456!",
	}
	resp, _, err := doRequest("POST", "/api/v1/change-password", req, "")
	require.NoError(t, err)
	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode, "未携带 token 应返回401")
}

// ============================================================================
// 空请求体
// ============================================================================

func TestChangePassword_EmptyBody(t *testing.T) {
	_, tokens, err := registerAndLoginWithCleanup(t)
	require.NoError(t, err, "注册登录失败")

	// 发送空 old_password 和 new_password
	req := changePasswordRequest{}
	resp, body, err := doRequest("POST", "/api/v1/change-password", req, tokens.AccessToken)
	require.NoError(t, err)
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode,
		"空请求体应返回400，实际%d，body=%s", resp.StatusCode, string(body))
}
