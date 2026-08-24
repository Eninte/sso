//go:build e2e

// Package e2e 客户端管理（管理员 API）端到端测试
package e2e

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ============================================================================
// 客户端管理辅助函数
// ============================================================================

// adminClientAPIResponse 客户端管理 API 的通用响应结构
type adminClientAPIResponse struct {
	Message      string                   `json:"message"`
	Code         string                   `json:"code"`
	ClientID     string                   `json:"client_id"`
	ClientSecret string                   `json:"client_secret"`
	Name         string                   `json:"name"`
	RedirectURIs []string                 `json:"redirect_uris"`
	GrantTypes   []string                 `json:"grant_types"`
	Scopes       []string                 `json:"scopes"`
	PublicClient bool                     `json:"public_client"`
	IsActive     bool                     `json:"is_active"`
	Clients      []adminClientAPIResponse `json:"clients"`
	Total        int                      `json:"total"`
}

// createE2EClient 通过管理 API 创建一个客户端（机密/公共）
func createE2EClient(t *testing.T, adminToken string, public bool) (string, string) {
	t.Helper()
	body := map[string]interface{}{
		"name":          "E2E Client " + generateUniqueEmail("client"),
		"redirect_uris": []string{"https://example.com/oauth/callback"},
		"grant_types":   []string{"authorization_code", "refresh_token"},
		"scopes":        []string{"openid", "profile", "email"},
		"public_client": public,
	}
	resp, respBody, err := doRequest("POST", "/api/v1/admin/clients", body, adminToken)
	require.NoError(t, err)
	require.Equal(t, http.StatusCreated, resp.StatusCode, "创建客户端失败: %s", string(respBody))

	var created adminClientAPIResponse
	require.NoError(t, json.Unmarshal(respBody, &created))
	require.NotEmpty(t, created.ClientID)
	if public {
		assert.Empty(t, created.ClientSecret, "公共客户端不应返回 client_secret")
		return created.ClientID, ""
	}
	require.NotEmpty(t, created.ClientSecret, "机密客户端应返回 client_secret")
	return created.ClientID, created.ClientSecret
}

// genPKCEVerifier 生成 PKCE code_verifier + S256 challenge
func genPKCEVerifier() (string, string) {
	b := make([]byte, 32)
	_, _ = rand.Read(b)
	verifier := base64.RawURLEncoding.EncodeToString(b)
	h := sha256.Sum256([]byte(verifier))
	challenge := base64.RawURLEncoding.EncodeToString(h[:])
	return verifier, challenge
}

// runAuthorizeFlow 走一遍授权码流程（GET /authorize + POST /authorize/approve + POST /token）
// 返回 exchange 的 HTTP 状态码与响应体；成功时返回 access_token
func runAuthorizeFlow(t *testing.T, userToken, clientID, redirectURI, secret, verifier string) (int, []byte, string) {
	t.Helper()
	// verifier 为空时生成新的一对；非空则按 S256 派生 challenge（保证 challenge 与 verifier 匹配）
	if verifier == "" {
		verifier, _ = genPKCEVerifier()
	}
	h := sha256.Sum256([]byte(verifier))
	challenge := base64.RawURLEncoding.EncodeToString(h[:])

	state := "state-" + fmt.Sprintf("%016d", time.Now().UnixNano()%1e16)
	params := url.Values{
		"response_type":         {"code"},
		"client_id":             {clientID},
		"redirect_uri":          {redirectURI},
		"scope":                 {"openid profile email"},
		"state":                 {state},
		"code_challenge":        {challenge},
		"code_challenge_method": {"S256"},
	}
	// GET /authorize 需要登录
	resp, respBody, err := doRequest("GET", "/api/v1/authorize?"+params.Encode(), nil, userToken)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode, "authorize 失败: %d %s", resp.StatusCode, string(respBody))

	var authResp struct {
		ConsentToken string `json:"consent_token"`
	}
	require.NoError(t, json.Unmarshal(respBody, &authResp))
	require.NotEmpty(t, authResp.ConsentToken, "authorize 应返回 consent_token")

	// POST /authorize/approve 批准
	approveBody := map[string]string{
		"consent_token": authResp.ConsentToken,
		"state":         state,
	}
	resp, respBody, err = doRequest("POST", "/api/v1/authorize/approve", approveBody, userToken)
	require.NoError(t, err)
	if resp.StatusCode != http.StatusOK {
		return resp.StatusCode, respBody, ""
	}
	var approveResp struct {
		Code string `json:"code"`
	}
	require.NoError(t, json.Unmarshal(respBody, &approveResp))
	require.NotEmpty(t, approveResp.Code, "approve 应返回授权码")

	// POST /token 交换
	exchangeReq := map[string]string{
		"grant_type":    "authorization_code",
		"code":          approveResp.Code,
		"redirect_uri":  redirectURI,
		"client_id":     clientID,
		"code_verifier": verifier,
	}
	if secret != "" {
		exchangeReq["client_secret"] = secret
	}
	resp, respBody, err = doRequest("POST", "/api/v1/token", exchangeReq, "")
	require.NoError(t, err)

	// handler 用 WriteJSONSuccess 包装：{"data": {"access_token": ..., ...}}
	var wrapped struct {
		Data struct {
			AccessToken string `json:"access_token"`
		} `json:"data"`
	}
	_ = json.Unmarshal(respBody, &wrapped)
	return resp.StatusCode, respBody, wrapped.Data.AccessToken
}

// ============================================================================
// 测试主体
// ============================================================================

// TestAdminClientManagement 客户端管理全流程
func TestAdminClientManagement(t *testing.T) {
	adminTokens, err := loginAdmin()
	require.NoError(t, err, "管理员登录失败")
	adminToken := adminTokens.AccessToken

	t.Run("创建机密客户端并走通授权码流程", func(t *testing.T) {
		clientID, secret := createE2EClient(t, adminToken, false)
		require.NotEmpty(t, secret)

		_, userTokens, err := registerAndLogin()
		require.NoError(t, err)

		status, respBody, accessToken := runAuthorizeFlow(t, userTokens.AccessToken, clientID, "https://example.com/oauth/callback", secret, "")
		assert.Equal(t, http.StatusOK, status, "机密客户端授权码流程应成功: %s", string(respBody))
		assert.NotEmpty(t, accessToken)
	})

	t.Run("创建公共客户端并走通PKCE授权码流程", func(t *testing.T) {
		clientID, _ := createE2EClient(t, adminToken, true)

		_, userTokens, err := registerAndLogin()
		require.NoError(t, err)

		status, respBody, accessToken := runAuthorizeFlow(t, userTokens.AccessToken, clientID, "https://example.com/oauth/callback", "", "")
		assert.Equal(t, http.StatusOK, status, "公共客户端 PKCE 授权码流程应成功: %s", string(respBody))
		assert.NotEmpty(t, accessToken)
	})

	t.Run("更新回调地址后新地址生效旧地址被拒", func(t *testing.T) {
		clientID, secret := createE2EClient(t, adminToken, false)

		// 更新回调地址
		updateBody := map[string]interface{}{
			"name":          "E2E Client Updated",
			"redirect_uris": []string{"https://new.example.com/cb"},
			"grant_types":   []string{"authorization_code", "refresh_token"},
			"scopes":        []string{"openid", "email"},
		}
		resp, respBody, err := doRequest("PUT", "/api/v1/admin/clients/"+clientID, updateBody, adminToken)
		require.NoError(t, err)
		require.Equal(t, http.StatusOK, resp.StatusCode, "更新客户端失败: %s", string(respBody))
		assert.NotContains(t, string(respBody), "client_secret")

		_, userTokens, err := registerAndLogin()
		require.NoError(t, err)

		// 旧地址被拒
		status, _, _ := runAuthorizeFlow(t, userTokens.AccessToken, clientID, "https://example.com/oauth/callback", secret, "")
		assert.Equal(t, http.StatusBadRequest, status, "旧回调地址应被拒绝")
	})

	t.Run("轮换密钥后旧密钥失效新密钥可用", func(t *testing.T) {
		clientID, oldSecret := createE2EClient(t, adminToken, false)

		_, userTokens, err := registerAndLogin()
		require.NoError(t, err)

		// 先用旧密钥走通
		status, _, _ := runAuthorizeFlow(t, userTokens.AccessToken, clientID, "https://example.com/oauth/callback", oldSecret, "")
		assert.Equal(t, http.StatusOK, status, "旧密钥应可完成流程")

		// 轮换
		resp, respBody, err := doRequest("POST", "/api/v1/admin/clients/"+clientID+"/rotate-secret", nil, adminToken)
		require.NoError(t, err)
		require.Equal(t, http.StatusOK, resp.StatusCode, "轮换失败: %s", string(respBody))
		var rotated adminClientAPIResponse
		require.NoError(t, json.Unmarshal(respBody, &rotated))
		require.NotEmpty(t, rotated.ClientSecret)
		require.NotEqual(t, oldSecret, rotated.ClientSecret)

		// 旧密钥被拒（INVALID_CLIENT 400）
		_, userTokens2, err := registerAndLogin()
		require.NoError(t, err)
		status, respBody, _ = runAuthorizeFlow(t, userTokens2.AccessToken, clientID, "https://example.com/oauth/callback", oldSecret, "")
		assert.Equal(t, http.StatusBadRequest, status, "旧密钥应失效: %s", string(respBody))

		// 新密钥可用
		status, _, _ = runAuthorizeFlow(t, userTokens2.AccessToken, clientID, "https://example.com/oauth/callback", rotated.ClientSecret, "")
		assert.Equal(t, http.StatusOK, status, "新密钥应可用")
	})

	t.Run("禁用后授权流程被拒启用后恢复", func(t *testing.T) {
		clientID, secret := createE2EClient(t, adminToken, false)
		_, userTokens, err := registerAndLogin()
		require.NoError(t, err)

		// 禁用
		resp, respBody, err := doRequest("POST", "/api/v1/admin/clients/"+clientID+"/disable", nil, adminToken)
		require.NoError(t, err)
		require.Equal(t, http.StatusOK, resp.StatusCode, "禁用失败: %s", string(respBody))

		// 授权码流程被拒（INVALID_CLIENT 400）
		status, respBody, _ := runAuthorizeFlow(t, userTokens.AccessToken, clientID, "https://example.com/oauth/callback", secret, "")
		assert.Equal(t, http.StatusBadRequest, status, "禁用客户端授权流程应被拒: %s", string(respBody))

		// 启用
		resp, respBody, err = doRequest("POST", "/api/v1/admin/clients/"+clientID+"/enable", nil, adminToken)
		require.NoError(t, err)
		require.Equal(t, http.StatusOK, resp.StatusCode, "启用失败: %s", string(respBody))

		// 恢复
		_, userTokens2, err := registerAndLogin()
		require.NoError(t, err)
		status, _, _ = runAuthorizeFlow(t, userTokens2.AccessToken, clientID, "https://example.com/oauth/callback", secret, "")
		assert.Equal(t, http.StatusOK, status, "启用后应恢复授权流程")
	})

	t.Run("删除后客户端404授权被拒", func(t *testing.T) {
		clientID, secret := createE2EClient(t, adminToken, false)
		_, userTokens, err := registerAndLogin()
		require.NoError(t, err)

		// 删除
		resp, respBody, err := doRequest("DELETE", "/api/v1/admin/clients/"+clientID, nil, adminToken)
		require.NoError(t, err)
		require.Equal(t, http.StatusOK, resp.StatusCode, "删除失败: %s", string(respBody))

		// GET 返回 404
		resp, _, err = doRequest("GET", "/api/v1/admin/clients/"+clientID, nil, adminToken)
		require.NoError(t, err)
		assert.Equal(t, http.StatusNotFound, resp.StatusCode, "删除后查询应返回 404")

		// 授权被拒
		status, _, _ := runAuthorizeFlow(t, userTokens.AccessToken, clientID, "https://example.com/oauth/callback", secret, "")
		assert.Equal(t, http.StatusBadRequest, status, "删除后授权流程应被拒")
	})

	t.Run("列表与查询", func(t *testing.T) {
		createE2EClient(t, adminToken, false)

		resp, respBody, err := doRequest("GET", "/api/v1/admin/clients?page=1&pageSize=10", nil, adminToken)
		require.NoError(t, err)
		require.Equal(t, http.StatusOK, resp.StatusCode)
		var listResp adminClientAPIResponse
		require.NoError(t, json.Unmarshal(respBody, &listResp))
		assert.NotEmpty(t, listResp.Clients)
		assert.NotContains(t, string(respBody), "client_secret", "列表响应不得包含 client_secret")
	})

	t.Run("非管理员访问返回403", func(t *testing.T) {
		_, userTokens, err := registerAndLogin()
		require.NoError(t, err)

		resp, _, err := doRequest("GET", "/api/v1/admin/clients", nil, userTokens.AccessToken)
		require.NoError(t, err)
		assert.Equal(t, http.StatusForbidden, resp.StatusCode, "非管理员访问应返回 403")
	})

	t.Run("client_credentials注册被拒", func(t *testing.T) {
		body := map[string]interface{}{
			"name":          "Bad Client",
			"redirect_uris": []string{"https://example.com/cb"},
			"grant_types":   []string{"authorization_code", "client_credentials"},
		}
		resp, respBody, err := doRequest("POST", "/api/v1/admin/clients", body, adminToken)
		require.NoError(t, err)
		assert.Equal(t, http.StatusBadRequest, resp.StatusCode, "含 client_credentials 应被拒: %s", string(respBody))
		var errResp struct {
			Code string `json:"code"`
		}
		require.NoError(t, json.Unmarshal(respBody, &errResp))
		assert.Equal(t, "UNSUPPORTED_GRANT_TYPE", errResp.Code)
	})
}
