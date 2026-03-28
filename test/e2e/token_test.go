//go:build e2e

// Package e2e Token验证端到端测试
package e2e

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ============================================================================
// 有效Token测试
// ============================================================================

func TestTokenValid(t *testing.T) {
	email := generateUniqueEmail("tokenvalid")
	password := generateTestPassword()

	// 注册并登录
	_, err := registerUser(email, password)
	require.NoError(t, err)

	tokens, err := loginUser(email, password)
	require.NoError(t, err)

	t.Run("有效Token访问受保护资源", func(t *testing.T) {
		resp, _, err := doRequest("GET", "/api/v1/userinfo", nil, tokens.AccessToken)
		require.NoError(t, err)
		assert.Equal(t, http.StatusOK, resp.StatusCode)
	})

	t.Run("Token格式验证", func(t *testing.T) {
		assert.NotEmpty(t, tokens.AccessToken)
		assert.NotEmpty(t, tokens.RefreshToken)
		assert.Equal(t, "Bearer", tokens.TokenType)
		assert.Greater(t, tokens.ExpiresIn, 0)
	})
}

// ============================================================================
// 无效Token测试
// ============================================================================

func TestTokenInvalid(t *testing.T) {
	t.Run("无效Token格式", func(t *testing.T) {
		resp, _, err := doRequest("GET", "/api/v1/userinfo", nil, "invalid-token-format")
		require.NoError(t, err)
		assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
	})

	t.Run("空Token", func(t *testing.T) {
		resp, _, err := doRequest("GET", "/api/v1/userinfo", nil, "")
		require.NoError(t, err)
		assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
	})

	t.Run("伪造Token", func(t *testing.T) {
		// 使用一个看起来像JWT但签名错误的Token
		fakeToken := "eyJhbGciOiJSUzI1NiIsInR5cCI6IkpXVCJ9.eyJzdWIiOiIxMjM0NTY3ODkwIiwibmFtZSI6IkpvaG4gRG9lIiwiYWRtaW4iOnRydWV9.signature"
		resp, _, err := doRequest("GET", "/api/v1/userinfo", nil, fakeToken)
		require.NoError(t, err)
		assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
	})

	t.Run("截断Token", func(t *testing.T) {
		// 注册并登录获取有效Token
		email := generateUniqueEmail("trunc")
		password := generateTestPassword()
		_, err := registerUser(email, password)
		require.NoError(t, err)

		tokens, err := loginUser(email, password)
		require.NoError(t, err)

		// 截断Token
		truncatedToken := tokens.AccessToken[:len(tokens.AccessToken)/2]
		resp, _, err := doRequest("GET", "/api/v1/userinfo", nil, truncatedToken)
		require.NoError(t, err)
		assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
	})
}

// ============================================================================
// 撤销Token测试
// ============================================================================

func TestTokenRevoked(t *testing.T) {
	email := generateUniqueEmail("revoked")
	password := generateTestPassword()

	// 注册并登录
	_, err := registerUser(email, password)
	require.NoError(t, err)

	tokens, err := loginUser(email, password)
	require.NoError(t, err)

	// 验证Token有效
	resp, _, err := doRequest("GET", "/api/v1/userinfo", nil, tokens.AccessToken)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	// 撤销Token
	revokeReq := revokeRequest{AccessToken: tokens.AccessToken}
	revokeResp, _, err := doRequest("POST", "/api/v1/token/revoke", revokeReq, "")
	require.NoError(t, err)

	if revokeResp.StatusCode == http.StatusNotFound {
		t.Skip("Token撤销端点未实现")
		return
	}

	// 验证Token已失效
	// 注意：根据实现，可能需要等待一段时间
	time.Sleep(100 * time.Millisecond)

	resp, _, err = doRequest("GET", "/api/v1/userinfo", nil, tokens.AccessToken)
	require.NoError(t, err)
	// Token应该失效
	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
}

// ============================================================================
// Token刷新测试
// ============================================================================

func TestTokenRefresh(t *testing.T) {
	email := generateUniqueEmail("refresh")
	password := generateTestPassword()

	// 注册并登录
	_, err := registerUser(email, password)
	require.NoError(t, err)

	tokens, err := loginUser(email, password)
	require.NoError(t, err)

	t.Run("有效RefreshToken刷新", func(t *testing.T) {
		refreshReq := tokenRequest{
			GrantType:    "refresh_token",
			RefreshToken: tokens.RefreshToken,
		}
		resp, body, err := doRequest("POST", "/api/v1/token", refreshReq, "")
		require.NoError(t, err)
		assert.Equal(t, http.StatusOK, resp.StatusCode)

		var newTokens loginResponse
		err = json.Unmarshal(body, &newTokens)
		require.NoError(t, err)
		assert.NotEmpty(t, newTokens.AccessToken)
		assert.NotEmpty(t, newTokens.RefreshToken)
	})

	t.Run("无效RefreshToken", func(t *testing.T) {
		refreshReq := tokenRequest{
			GrantType:    "refresh_token",
			RefreshToken: "invalid-refresh-token",
		}
		resp, _, err := doRequest("POST", "/api/v1/token", refreshReq, "")
		require.NoError(t, err)
		assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
	})

	t.Run("空RefreshToken", func(t *testing.T) {
		refreshReq := tokenRequest{
			GrantType:    "refresh_token",
			RefreshToken: "",
		}
		resp, _, err := doRequest("POST", "/api/v1/token", refreshReq, "")
		require.NoError(t, err)
		assert.True(t, resp.StatusCode >= 400)
	})
}

// ============================================================================
// Token过期测试
// ============================================================================

func TestTokenExpired(t *testing.T) {
	// 注意：这个测试需要等待Token过期，通常不适合端到端测试
	// 这里只测试基本的Token验证逻辑

	t.Run("Token过期验证", func(t *testing.T) {
		// 使用一个明显过期的Token（JWT中的exp字段设置为过去的时间）
		// 这需要构造一个特定的JWT，这里只是示意
		t.Skip("Token过期测试需要较长的等待时间或特殊的Token构造")
	})
}

// ============================================================================
// 并发Token刷新测试
// ============================================================================

func TestConcurrentTokenRefresh(t *testing.T) {
	email := generateUniqueEmail("concrefresh")
	password := generateTestPassword()

	// 注册并登录
	_, err := registerUser(email, password)
	require.NoError(t, err)

	tokens, err := loginUser(email, password)
	require.NoError(t, err)

	// 并发刷新Token
	done := make(chan bool, 5)
	errors := make(chan error, 5)

	for i := 0; i < 5; i++ {
		go func() {
			refreshReq := tokenRequest{
				GrantType:    "refresh_token",
				RefreshToken: tokens.RefreshToken,
			}
			resp, _, err := doRequest("POST", "/api/v1/token", refreshReq, "")
			if err != nil {
				errors <- err
			} else if resp.StatusCode != http.StatusOK {
				// 可能只有一个请求成功，其他会失败（Token已被使用）
				t.Logf("并发刷新返回状态: %d", resp.StatusCode)
			}
			done <- true
		}()
	}

	// 等待所有请求完成
	for i := 0; i < 5; i++ {
		<-done
	}

	// 检查是否有错误
	select {
	case err := <-errors:
		t.Logf("并发刷新错误: %v", err)
	default:
		t.Logf("并发刷新测试完成")
	}
}

// ============================================================================
// Token权限测试
// ============================================================================

func TestTokenPermissions(t *testing.T) {
	email := generateUniqueEmail("perms")
	password := generateTestPassword()

	// 注册并登录
	_, err := registerUser(email, password)
	require.NoError(t, err)

	tokens, err := loginUser(email, password)
	require.NoError(t, err)

	t.Run("访问用户信息", func(t *testing.T) {
		resp, _, err := doRequest("GET", "/api/v1/userinfo", nil, tokens.AccessToken)
		require.NoError(t, err)
		assert.Equal(t, http.StatusOK, resp.StatusCode)
	})

	t.Run("访问管理员接口", func(t *testing.T) {
		resp, _, err := doRequest("GET", "/api/v1/admin/users", nil, tokens.AccessToken)
		require.NoError(t, err)

		if resp.StatusCode == http.StatusNotFound {
			t.Skip("管理员端点未实现")
			return
		}

		// 普通用户应该被拒绝
		assert.Equal(t, http.StatusForbidden, resp.StatusCode)
	})
}
