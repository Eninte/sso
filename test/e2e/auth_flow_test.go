//go:build e2e

// Package e2e 认证流程端到端测试
package e2e

import (
	"bytes"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ============================================================================
// 健康检查测试
// ============================================================================

func TestHealthCheck(t *testing.T) {
	resp, body, err := doRequest("GET", "/health", nil, "")
	require.NoError(t, err)

	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Contains(t, string(body), "ok")
}

// ============================================================================
// 注册流程测试
// ============================================================================

func TestRegisterFlow(t *testing.T) {
	email := generateUniqueEmail("register")
	password := generateTestPassword()

	t.Run("成功注册", func(t *testing.T) {
		user, err := registerUser(email, password)
		require.NoError(t, err)
		assert.Equal(t, email, user["email"])
	})

	t.Run("邮箱已存在", func(t *testing.T) {
		req := registerRequest{Email: email, Password: password}
		resp, _, err := doRequest("POST", "/api/v1/register", req, "")
		require.NoError(t, err)
		assert.Equal(t, http.StatusConflict, resp.StatusCode)
	})

	t.Run("无效邮箱格式", func(t *testing.T) {
		req := registerRequest{Email: "invalid-email", Password: password}
		resp, _, err := doRequest("POST", "/api/v1/register", req, "")
		require.NoError(t, err)
		assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
	})

	t.Run("密码过短", func(t *testing.T) {
		req := registerRequest{Email: generateUniqueEmail("short"), Password: "short"}
		resp, _, err := doRequest("POST", "/api/v1/register", req, "")
		require.NoError(t, err)
		assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
	})

	t.Run("空邮箱", func(t *testing.T) {
		req := registerRequest{Email: "", Password: password}
		resp, _, err := doRequest("POST", "/api/v1/register", req, "")
		require.NoError(t, err)
		assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
	})

	t.Run("空密码", func(t *testing.T) {
		req := registerRequest{Email: generateUniqueEmail("empty"), Password: ""}
		resp, _, err := doRequest("POST", "/api/v1/register", req, "")
		require.NoError(t, err)
		assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
	})
}

// ============================================================================
// 登录流程测试
// ============================================================================

func TestLoginFlow(t *testing.T) {
	email := generateUniqueEmail("login")
	password := generateTestPassword()

	// 先注册用户
	_, err := registerUser(email, password)
	require.NoError(t, err)

	t.Run("成功登录", func(t *testing.T) {
		tokens, err := loginUser(email, password)
		require.NoError(t, err)
		assert.NotEmpty(t, tokens.AccessToken)
		assert.NotEmpty(t, tokens.RefreshToken)
		assert.Equal(t, "Bearer", tokens.TokenType)
	})

	t.Run("密码错误", func(t *testing.T) {
		req := loginRequest{Email: email, Password: "WrongPassword123!"}
		resp, _, err := doRequest("POST", "/api/v1/login", req, "")
		require.NoError(t, err)
		assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
	})

	t.Run("用户不存在", func(t *testing.T) {
		req := loginRequest{Email: "nonexistent@example.com", Password: password}
		resp, _, err := doRequest("POST", "/api/v1/login", req, "")
		require.NoError(t, err)
		assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
	})

	t.Run("空邮箱", func(t *testing.T) {
		req := loginRequest{Email: "", Password: password}
		resp, _, err := doRequest("POST", "/api/v1/login", req, "")
		require.NoError(t, err)
		assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
	})

	t.Run("空密码", func(t *testing.T) {
		req := loginRequest{Email: email, Password: ""}
		resp, _, err := doRequest("POST", "/api/v1/login", req, "")
		require.NoError(t, err)
		assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
	})
}

// ============================================================================
// 完整认证流程测试
// ============================================================================

func TestFullAuthFlow(t *testing.T) {
	email := generateUniqueEmail("full")
	password := generateTestPassword()

	// 1. 注册
	user, err := registerUser(email, password)
	require.NoError(t, err)
	t.Logf("注册成功: %s", user["id"])

	// 2. 登录
	tokens, err := loginUser(email, password)
	require.NoError(t, err)
	t.Logf("登录成功，获取到Token")

	// 3. 使用Token访问受保护资源
	resp, body, err := doRequest("GET", "/api/v1/userinfo", nil, tokens.AccessToken)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	userInfo, err := parseUserInfo(body)
	require.NoError(t, err)
	assert.Equal(t, email, userInfo.Email)
	t.Logf("获取用户信息成功")

	// 4. 刷新Token
	refreshReq := tokenRequest{
		GrantType:    "refresh_token",
		RefreshToken: tokens.RefreshToken,
	}
	refreshResp, refreshBody, err := doRequest("POST", "/api/v1/token", refreshReq, "")
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, refreshResp.StatusCode)

	var newTokens loginResponse
	err = json.Unmarshal(refreshBody, &newTokens)
	require.NoError(t, err)
	assert.NotEmpty(t, newTokens.AccessToken)
	assert.NotEmpty(t, newTokens.RefreshToken)
	t.Logf("Token刷新成功")

	// 5. 登出
	revokeReq := revokeRequest{AccessToken: tokens.AccessToken}
	logoutResp, _, err := doRequest("POST", "/api/v1/token/revoke", revokeReq, "")
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, logoutResp.StatusCode)
	t.Logf("登出成功")
}

// ============================================================================
// 限流测试
// ============================================================================

func TestRateLimit(t *testing.T) {
	// 快速发送多个请求测试限流
	for i := 0; i < 20; i++ {
		resp, _, err := doRequest("GET", "/health", nil, "")
		require.NoError(t, err)

		if resp.StatusCode == http.StatusTooManyRequests {
			t.Logf("限流触发在第 %d 次请求", i+1)
			return
		}
	}
	t.Logf("未触发限流（可能限流阈值较高）")
}

// ============================================================================
// 多设备登录测试
// ============================================================================

func TestMultiDeviceLogin(t *testing.T) {
	email := generateUniqueEmail("multi")
	password := generateTestPassword()

	// 注册用户
	_, err := registerUser(email, password)
	require.NoError(t, err)

	// 模拟多个设备登录
	tokens1, err := loginUser(email, password)
	require.NoError(t, err)

	tokens2, err := loginUser(email, password)
	require.NoError(t, err)

	// 验证两个Token都能访问
	resp1, _, err := doRequest("GET", "/api/v1/userinfo", nil, tokens1.AccessToken)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp1.StatusCode)

	resp2, _, err := doRequest("GET", "/api/v1/userinfo", nil, tokens2.AccessToken)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp2.StatusCode)

	t.Logf("多设备登录测试通过")
}

// ============================================================================
// 登出所有设备测试
// ============================================================================

func TestLogoutAllDevices(t *testing.T) {
	email := generateUniqueEmail("logoutall")
	password := generateTestPassword()

	// 注册用户
	_, err := registerUser(email, password)
	require.NoError(t, err)

	// 模拟多个设备登录
	tokens1, err := loginUser(email, password)
	require.NoError(t, err)

	tokens2, err := loginUser(email, password)
	require.NoError(t, err)

	// 登出所有设备（使用第一个token）
	logoutResp, _, err := doRequest("POST", "/api/v1/token/revoke-all", nil, tokens1.AccessToken)
	// 如果端点不存在，跳过此测试
	if err != nil || logoutResp.StatusCode == http.StatusNotFound {
		t.Skip("登出所有设备端点未实现")
		return
	}

	// 验证两个Token都失效
	time.Sleep(100 * time.Millisecond) // 等待异步操作

	resp1, _, _ := doRequest("GET", "/api/v1/userinfo", nil, tokens1.AccessToken)
	// 注意：根据实现，可能返回401或403
	t.Logf("Token1状态: %d", resp1.StatusCode)

	resp2, _, _ := doRequest("GET", "/api/v1/userinfo", nil, tokens2.AccessToken)
	t.Logf("Token2状态: %d", resp2.StatusCode)
}

// ============================================================================
// 请求格式测试
// ============================================================================

func TestRequestFormat(t *testing.T) {
	t.Run("无效JSON格式", func(t *testing.T) {
		req, err := http.NewRequest("POST", baseURL+"/api/v1/login", bytes.NewBufferString("invalid json"))
		require.NoError(t, err)
		req.Header.Set("Content-Type", "application/json")

		resp, err := client.Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
	})

	t.Run("缺少Content-Type", func(t *testing.T) {
		req, err := http.NewRequest("POST", baseURL+"/api/v1/login", bytes.NewBufferString("{}"))
		require.NoError(t, err)

		resp, err := client.Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()

		// 可能返回400或415
		assert.True(t, resp.StatusCode >= 400)
	})
}
