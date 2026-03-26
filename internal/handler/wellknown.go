// Package handler OIDC Discovery处理器
// 提供OpenID Connect Discovery端点
package handler

import (
	"crypto/rsa"
	"encoding/base64"
	"math/big"
	"net/http"

	"github.com/your-org/sso/internal/crypto"
)

// ============================================================================
// WellKnownHandler OIDC Discovery处理器
// ============================================================================

// WellKnownHandler OIDC Discovery处理器
type WellKnownHandler struct {
	baseURL   string             // 服务基础URL
	publicKey *rsa.PublicKey     // RSA公钥（向后兼容）
	jwtSvc    *crypto.JWTService // JWT服务（用于获取多密钥JWKS）
}

// NewWellKnownHandler 创建OIDC Discovery处理器
func NewWellKnownHandler(baseURL string, publicKey *rsa.PublicKey) *WellKnownHandler {
	return &WellKnownHandler{
		baseURL:   baseURL,
		publicKey: publicKey,
	}
}

// NewWellKnownHandlerWithJWTService 创建OIDC Discovery处理器（支持多密钥JWKS）
func NewWellKnownHandlerWithJWTService(baseURL string, jwtSvc *crypto.JWTService) *WellKnownHandler {
	return &WellKnownHandler{
		baseURL: baseURL,
		jwtSvc:  jwtSvc,
	}
}

// HandleDiscovery 处理OIDC Discovery请求
// GET /.well-known/openid-configuration
// 返回OIDC配置信息，供客户端发现服务端点
func (h *WellKnownHandler) HandleDiscovery(w http.ResponseWriter, r *http.Request) {
	discovery := map[string]interface{}{
		// 基本信息
		"issuer": h.baseURL,

		// 端点
		"authorization_endpoint": h.baseURL + "/authorize",
		"token_endpoint":         h.baseURL + "/api/v1/token",
		"userinfo_endpoint":      h.baseURL + "/api/v1/userinfo",
		"jwks_uri":               h.baseURL + "/.well-known/jwks.json",
		"revocation_endpoint":    h.baseURL + "/api/v1/token/revoke",

		// 支持的响应类型
		"response_types_supported": []string{"code"},

		// 支持的授权类型
		"grant_types_supported": []string{
			"authorization_code",
			"refresh_token",
		},

		// 支持的主体类型
		"subject_types_supported": []string{"public"},

		// 支持的签名算法
		"id_token_signing_alg_values_supported": []string{"RS256"},

		// 支持的权限范围
		"scopes_supported": []string{
			"openid",
			"profile",
			"email",
		},

		// 支持的客户端认证方式
		"token_endpoint_auth_methods_supported": []string{
			"client_secret_basic",
			"client_secret_post",
			"none",
		},

		// PKCE支持
		"code_challenge_methods_supported": []string{"S256"},

		// 支持的声明
		"claims_supported": []string{
			"sub",
			"iss",
			"aud",
			"exp",
			"iat",
			"email",
			"email_verified",
			"name",
			"picture",
		},
	}

	writeJSON(w, http.StatusOK, discovery)
}

// HandleJWKS 处理JWKS请求
// GET /.well-known/jwks.json
// 返回公钥信息，供客户端验证JWT签名
func (h *WellKnownHandler) HandleJWKS(w http.ResponseWriter, r *http.Request) {
	// 如果有JWTService，使用它的GetJWKS方法获取多密钥JWKS
	if h.jwtSvc != nil {
		jwks := h.jwtSvc.GetJWKS()
		writeJSON(w, http.StatusOK, jwks)
		return
	}

	// 向后兼容：单密钥模式
	if h.publicKey == nil {
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"keys": []interface{}{},
		})
		return
	}

	// 构建JWK（单密钥）
	// 参考: https://datatracker.ietf.org/doc/html/rfc7517
	jwk := map[string]interface{}{
		"kty": "RSA",                                                     // 密钥类型
		"use": "sig",                                                     // 用途: 签名
		"kid": "sso-key-1",                                               // 密钥ID
		"n":   base64URLEncode(h.publicKey.N.Bytes()),                    // 模数
		"e":   base64URLEncode(big.NewInt(int64(h.publicKey.E)).Bytes()), // 指数
	}

	jwks := map[string]interface{}{
		"keys": []interface{}{jwk},
	}

	writeJSON(w, http.StatusOK, jwks)
}

// base64URLEncode URL安全的Base64编码
// 不使用填充字符
func base64URLEncode(data []byte) string {
	return base64.RawURLEncoding.EncodeToString(data)
}
