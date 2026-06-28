// Package handler_test WellKnown Handler单元测试
package handler_test

import (
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/example/sso/internal/crypto"
	"github.com/example/sso/internal/handler"
)

// ============================================================================
// WellKnownHandler 测试
// ============================================================================

func createTestWellKnownHandler(t *testing.T) *handler.WellKnownHandler {
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	return handler.NewWellKnownHandler("http://localhost:9090", &privateKey.PublicKey)
}

func TestWellKnownHandler_HandleDiscovery(t *testing.T) {
	h := createTestWellKnownHandler(t)

	t.Run("返回完整OIDC配置", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/.well-known/openid-configuration", nil)
		w := httptest.NewRecorder()

		h.HandleDiscovery(w, req)

		assert.Equal(t, http.StatusOK, w.Code)

		var resp map[string]interface{}
		err := json.Unmarshal(w.Body.Bytes(), &resp)
		require.NoError(t, err)

		// 验证关键字段
		assert.Equal(t, "http://localhost:9090", resp["issuer"])
		assert.Equal(t, "http://localhost:9090/authorize", resp["authorization_endpoint"])
		assert.Equal(t, "http://localhost:9090/api/v1/token", resp["token_endpoint"])
		assert.Equal(t, "http://localhost:9090/.well-known/jwks.json", resp["jwks_uri"])

		// 验证支持的响应类型
		responseTypes, ok := resp["response_types_supported"].([]interface{})
		require.True(t, ok)
		assert.Contains(t, responseTypes, "code")

		// 验证支持的授权类型
		grantTypes, ok := resp["grant_types_supported"].([]interface{})
		require.True(t, ok)
		assert.Contains(t, grantTypes, "authorization_code")
		assert.Contains(t, grantTypes, "refresh_token")

		// 验证PKCE支持
		challengeMethods, ok := resp["code_challenge_methods_supported"].([]interface{})
		require.True(t, ok)
		assert.Contains(t, challengeMethods, "S256")

		// 验证签名算法
		signingAlgs, ok := resp["id_token_signing_alg_values_supported"].([]interface{})
		require.True(t, ok)
		assert.Contains(t, signingAlgs, "RS256")
	})
}

func TestWellKnownHandler_HandleJWKS(t *testing.T) {
	h := createTestWellKnownHandler(t)

	t.Run("返回JWKS公钥", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/.well-known/jwks.json", nil)
		w := httptest.NewRecorder()

		h.HandleJWKS(w, req)

		assert.Equal(t, http.StatusOK, w.Code)

		var resp map[string]interface{}
		err := json.Unmarshal(w.Body.Bytes(), &resp)
		require.NoError(t, err)

		// 验证keys数组
		keys, ok := resp["keys"].([]interface{})
		require.True(t, ok)
		assert.Len(t, keys, 1)

		// 验证JWK格式
		jwk, ok := keys[0].(map[string]interface{})
		require.True(t, ok)
		assert.Equal(t, "RSA", jwk["kty"])
		assert.Equal(t, "sig", jwk["use"])
		assert.Equal(t, "sso-key-1", jwk["kid"])
		assert.NotEmpty(t, jwk["n"])
		assert.NotEmpty(t, jwk["e"])
	})
}

// ============================================================================
// NewWellKnownHandlerWithJWTService 测试
// ============================================================================

func TestNewWellKnownHandlerWithJWTService(t *testing.T) {
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)

	jwtSvc := crypto.NewJWTService(
		privateKey,
		&privateKey.PublicKey,
		"test-issuer",
		15*time.Minute,
		7*24*time.Hour,
	)

	h := handler.NewWellKnownHandlerWithJWTService("http://localhost:9090", jwtSvc)
	require.NotNil(t, h)

	t.Run("HandleDiscovery正常工作", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/.well-known/openid-configuration", nil)
		w := httptest.NewRecorder()

		h.HandleDiscovery(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
	})

	t.Run("HandleJWKS使用JWTService", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/.well-known/jwks.json", nil)
		w := httptest.NewRecorder()

		h.HandleJWKS(w, req)

		assert.Equal(t, http.StatusOK, w.Code)

		var resp map[string]interface{}
		err := json.Unmarshal(w.Body.Bytes(), &resp)
		require.NoError(t, err)

		// 验证返回了keys字段
		_, ok := resp["keys"].([]interface{})
		require.True(t, ok)
	})
}
