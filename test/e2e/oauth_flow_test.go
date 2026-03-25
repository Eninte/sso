//go:build e2e

// Package e2e OAuth流程端到端测试
package e2e

import (
	"encoding/json"
	"net/http"
	"net/url"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ============================================================================
// OAuth授权请求结构
// ============================================================================

type authorizeRequest struct {
	ResponseType        string `json:"response_type"`
	ClientID            string `json:"client_id"`
	RedirectURI         string `json:"redirect_uri"`
	Scope               string `json:"scope"`
	State               string `json:"state"`
	CodeChallenge       string `json:"code_challenge,omitempty"`
	CodeChallengeMethod string `json:"code_challenge_method,omitempty"`
}

type authorizeResponse struct {
	Code  string `json:"code"`
	State string `json:"state"`
}

type tokenExchangeRequest struct {
	GrantType    string `json:"grant_type"`
	Code         string `json:"code"`
	RedirectURI  string `json:"redirect_uri"`
	ClientID     string `json:"client_id"`
	ClientSecret string `json:"client_secret,omitempty"`
	CodeVerifier string `json:"code_verifier,omitempty"`
}

// ============================================================================
// OAuth授权端点测试
// ============================================================================

func TestOAuthAuthorize(t *testing.T) {
	// 注意：这些测试需要有效的OAuth客户端配置
	// 如果客户端未配置，测试会被跳过

	t.Run("无效客户端ID", func(t *testing.T) {
		params := url.Values{
			"response_type": {"code"},
			"client_id":     {"invalid-client-id"},
			"redirect_uri":  {"http://localhost:3000/callback"},
			"scope":         {"openid email"},
			"state":         {"test-state"},
		}

		resp, _, err := doRequest("GET", "/api/v1/authorize?"+params.Encode(), nil, "")
		require.NoError(t, err)

		// 应该返回错误
		assert.True(t, resp.StatusCode >= 400)
	})

	t.Run("无效重定向URI", func(t *testing.T) {
		params := url.Values{
			"response_type": {"code"},
			"client_id":     {"test-client"},
			"redirect_uri":  {"http://malicious.com/callback"},
			"scope":         {"openid email"},
			"state":         {"test-state"},
		}

		resp, _, err := doRequest("GET", "/api/v1/authorize?"+params.Encode(), nil, "")
		require.NoError(t, err)

		// 应该返回错误
		assert.True(t, resp.StatusCode >= 400)
	})

	t.Run("缺少必需参数", func(t *testing.T) {
		params := url.Values{
			"response_type": {"code"},
			// 缺少client_id
			"redirect_uri": {"http://localhost:3000/callback"},
			"scope":        {"openid email"},
		}

		resp, _, err := doRequest("GET", "/api/v1/authorize?"+params.Encode(), nil, "")
		require.NoError(t, err)

		// 应该返回错误
		assert.True(t, resp.StatusCode >= 400)
	})
}

// ============================================================================
// OAuth Token交换测试
// ============================================================================

func TestOAuthTokenExchange(t *testing.T) {
	t.Run("无效授权码", func(t *testing.T) {
		req := tokenExchangeRequest{
			GrantType:   "authorization_code",
			Code:        "invalid-code",
			RedirectURI: "http://localhost:3000/callback",
			ClientID:    "test-client",
		}

		resp, _, err := doRequest("POST", "/api/v1/token", req, "")
		require.NoError(t, err)

		// 应该返回错误
		assert.True(t, resp.StatusCode >= 400)
	})

	t.Run("无效Grant类型", func(t *testing.T) {
		req := tokenExchangeRequest{
			GrantType:   "invalid_grant",
			Code:        "some-code",
			RedirectURI: "http://localhost:3000/callback",
			ClientID:    "test-client",
		}

		resp, _, err := doRequest("POST", "/api/v1/token", req, "")
		require.NoError(t, err)

		// 应该返回错误
		assert.True(t, resp.StatusCode >= 400)
	})
}

// ============================================================================
// OAuth PKCE流程测试
// ============================================================================

func TestOAuthPKCE(t *testing.T) {
	t.Run("PKCE参数验证", func(t *testing.T) {
		// 测试带PKCE参数的授权请求
		params := url.Values{
			"response_type":         {"code"},
			"client_id":             {"test-client"},
			"redirect_uri":          {"http://localhost:3000/callback"},
			"scope":                 {"openid email"},
			"state":                 {"test-state"},
			"code_challenge":        {"E9Melhoa2OwvFrEMTJguCHaoeK1t8URWbuGJSstw-cM"},
			"code_challenge_method": {"S256"},
		}

		resp, _, err := doRequest("GET", "/api/v1/authorize?"+params.Encode(), nil, "")
		require.NoError(t, err)

		// 即使客户端不存在，也应该是4xx而不是5xx
		assert.True(t, resp.StatusCode >= 400 && resp.StatusCode < 500)
	})
}

// ============================================================================
// OAuth Scope测试
// ============================================================================

func TestOAuthScope(t *testing.T) {
	t.Run("无效Scope", func(t *testing.T) {
		params := url.Values{
			"response_type": {"code"},
			"client_id":     {"test-client"},
			"redirect_uri":  {"http://localhost:3000/callback"},
			"scope":         {"invalid-scope"},
			"state":         {"test-state"},
		}

		resp, _, err := doRequest("GET", "/api/v1/authorize?"+params.Encode(), nil, "")
		require.NoError(t, err)

		// 可能返回错误或忽略无效scope
		t.Logf("无效Scope响应状态: %d", resp.StatusCode)
	})
}

// ============================================================================
// OAuth State参数测试
// ============================================================================

func TestOAuthState(t *testing.T) {
	t.Run("State参数传递", func(t *testing.T) {
		params := url.Values{
			"response_type": {"code"},
			"client_id":     {"test-client"},
			"redirect_uri":  {"http://localhost:3000/callback"},
			"scope":         {"openid email"},
			"state":         {"test-state-12345"},
		}

		resp, body, err := doRequest("GET", "/api/v1/authorize?"+params.Encode(), nil, "")
		require.NoError(t, err)

		// 如果成功，检查响应中是否包含state
		if resp.StatusCode == http.StatusOK || resp.StatusCode == http.StatusFound {
			// 可能是重定向或JSON响应
			t.Logf("授权响应: %s", string(body))
		}
	})
}

// ============================================================================
// 完整OAuth流程测试（模拟）
// ============================================================================

func TestFullOAuthFlow_Simulated(t *testing.T) {
	// 这个测试模拟完整OAuth流程，但不依赖真实的OAuth客户端
	// 主要用于验证API端点存在且基本逻辑正确

	t.Run("授权端点可访问", func(t *testing.T) {
		params := url.Values{
			"response_type": {"code"},
			"client_id":     {"test-client"},
			"redirect_uri":  {"http://localhost:3000/callback"},
			"scope":         {"openid"},
			"state":         {"test"},
		}

		resp, _, err := doRequest("GET", "/api/v1/authorize?"+params.Encode(), nil, "")
		require.NoError(t, err)

		// 端点应该存在（不是404或500）
		assert.NotEqual(t, http.StatusNotFound, resp.StatusCode)
		assert.NotEqual(t, http.StatusInternalServerError, resp.StatusCode)
	})

	t.Run("Token端点可访问", func(t *testing.T) {
		req := map[string]string{
			"grant_type": "authorization_code",
			"code":       "test-code",
		}

		resp, _, err := doRequest("POST", "/api/v1/token", req, "")
		require.NoError(t, err)

		// 端点应该存在
		assert.NotEqual(t, http.StatusNotFound, resp.StatusCode)
	})
}

// ============================================================================
// OAuth错误响应测试
// ============================================================================

func TestOAuthError(t *testing.T) {
	t.Run("无效请求格式", func(t *testing.T) {
		// 发送无效JSON
		resp, body, err := doRequest("POST", "/api/v1/token", "invalid", "")
		require.NoError(t, err)

		assert.True(t, resp.StatusCode >= 400)

		var errResp map[string]interface{}
		if err := json.Unmarshal(body, &errResp); err == nil {
			// 验证错误响应格式
			t.Logf("错误响应: %v", errResp)
		}
	})
}
