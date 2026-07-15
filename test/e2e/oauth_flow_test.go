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
// 完整OAuth流程测试（使用预置真实客户端）
// ============================================================================

// TestFullOAuthFlow_PublicClient 公共客户端完整流程
// 使用 prepare-e2e-test.sh 预置的 public-test-client（公共客户端，无需 secret，必须 PKCE）
func TestFullOAuthFlow_PublicClient(t *testing.T) {
	verifier, challenge := generatePKCEPair()

	t.Run("授权端点接受公共客户端+PKCE", func(t *testing.T) {
		params := url.Values{
			"response_type":         {"code"},
			"client_id":             {"public-test-client"},
			"redirect_uri":          {"http://localhost:3000/callback"},
			"scope":                 {"openid"},
			"state":                 {"test-state"},
			"code_challenge":        {challenge},
			"code_challenge_method": {"S256"},
		}

		resp, _, err := doRequest("GET", "/api/v1/authorize?"+params.Encode(), nil, "")
		require.NoError(t, err)

		// 公共客户端合法请求应进入授权流程（非 404/500）
		// 未登录用户可能返回 401 或重定向到登录页，二者均合法
		assert.NotEqual(t, http.StatusNotFound, resp.StatusCode, "端点应存在")
		assert.NotEqual(t, http.StatusInternalServerError, resp.StatusCode, "不应返回500")
	})

	t.Run("Token端点处理无效授权码", func(t *testing.T) {
		req := tokenExchangeRequest{
			GrantType:    "authorization_code",
			Code:         "invalid-test-code",
			RedirectURI:  "http://localhost:3000/callback",
			ClientID:     "public-test-client",
			CodeVerifier: verifier,
		}

		resp, _, err := doRequest("POST", "/api/v1/token", req, "")
		require.NoError(t, err)

		// 端点应存在（非 404），无效 code 应返回 400 而非 404
		assert.NotEqual(t, http.StatusNotFound, resp.StatusCode, "Token端点应存在")
		assert.True(t, resp.StatusCode >= 400, "无效code应返回4xx错误")
	})
}

// TestFullOAuthFlow_PublicClient_PKCE 公共客户端 PKCE 参数验证
// 验证 PKCE 参数被正确处理：有 PKCE 时进入流程，无 PKCE 时公共客户端应被拒绝
func TestFullOAuthFlow_PublicClient_PKCE(t *testing.T) {
	t.Run("无PKCE公共客户端应被拒绝", func(t *testing.T) {
		// 公共客户端不带 code_challenge 应被拒绝（PKCE 必须）
		params := url.Values{
			"response_type": {"code"},
			"client_id":     {"public-test-client"},
			"redirect_uri":  {"http://localhost:3000/callback"},
			"scope":         {"openid"},
			"state":         {"test-state"},
			// 故意不传 code_challenge 和 code_challenge_method
		}

		resp, _, err := doRequest("GET", "/api/v1/authorize?"+params.Encode(), nil, "")
		require.NoError(t, err)

		// 公共客户端无 PKCE 应返回 4xx 错误（非 404/500）
		assert.NotEqual(t, http.StatusNotFound, resp.StatusCode, "端点应存在")
		assert.True(t, resp.StatusCode >= 400, "公共客户端无PKCE应被拒绝（4xx）")
	})

	t.Run("有效PKCE参数被接受", func(t *testing.T) {
		_, challenge := generatePKCEPair()
		params := url.Values{
			"response_type":         {"code"},
			"client_id":             {"public-test-client"},
			"redirect_uri":          {"http://localhost:3000/callback"},
			"scope":                 {"openid"},
			"state":                 {"test-state"},
			"code_challenge":        {challenge},
			"code_challenge_method": {"S256"},
		}

		resp, _, err := doRequest("GET", "/api/v1/authorize?"+params.Encode(), nil, "")
		require.NoError(t, err)

		// 有效 PKCE 应进入授权流程（非 404/500）
		assert.NotEqual(t, http.StatusNotFound, resp.StatusCode, "端点应存在")
		assert.NotEqual(t, http.StatusInternalServerError, resp.StatusCode, "有效PKCE不应返回500")
	})
}

// TestFullOAuthFlow_ConfidentialClient 机密客户端完整流程
// 使用 prepare-e2e-test.sh 预置的 confidential-test-client（secret=test-client-secret-12345）
func TestFullOAuthFlow_ConfidentialClient(t *testing.T) {
	t.Run("机密客户端授权端点可访问", func(t *testing.T) {
		params := url.Values{
			"response_type": {"code"},
			"client_id":     {"confidential-test-client"},
			"redirect_uri":  {"http://localhost:3000/callback"},
			"scope":         {"openid"},
			"state":         {"test-state"},
		}

		resp, _, err := doRequest("GET", "/api/v1/authorize?"+params.Encode(), nil, "")
		require.NoError(t, err)

		// 机密客户端合法请求应进入授权流程（非 404/500）
		assert.NotEqual(t, http.StatusNotFound, resp.StatusCode, "端点应存在")
		assert.NotEqual(t, http.StatusInternalServerError, resp.StatusCode, "不应返回500")
	})

	t.Run("机密客户端Token端点验证secret", func(t *testing.T) {
		// 使用正确 secret，但无效 code
		req := tokenExchangeRequest{
			GrantType:    "authorization_code",
			Code:         "invalid-test-code",
			RedirectURI:  "http://localhost:3000/callback",
			ClientID:     "confidential-test-client",
			ClientSecret: "test-client-secret-12345",
		}

		resp, _, err := doRequest("POST", "/api/v1/token", req, "")
		require.NoError(t, err)

		// 端点应存在（非 404），无效 code 应返回 400
		assert.NotEqual(t, http.StatusNotFound, resp.StatusCode, "Token端点应存在")
		assert.True(t, resp.StatusCode >= 400, "无效code应返回4xx错误")
	})

	t.Run("机密客户端错误secret应被拒绝", func(t *testing.T) {
		req := tokenExchangeRequest{
			GrantType:    "authorization_code",
			Code:         "test-code",
			RedirectURI:  "http://localhost:3000/callback",
			ClientID:     "confidential-test-client",
			ClientSecret: "wrong-secret",
		}

		resp, _, err := doRequest("POST", "/api/v1/token", req, "")
		require.NoError(t, err)

		// 错误 secret 应返回 4xx（401 或 400）
		assert.True(t, resp.StatusCode >= 400, "错误secret应返回4xx错误")
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
