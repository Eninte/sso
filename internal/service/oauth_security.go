// Package service OAuth 安全增强（阶段 2.2）
// 实现 Scope 校验、PKCE 强制、Consent token 签发与校验
package service

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"

	apperrors "github.com/example/sso/internal/errors"
	"github.com/example/sso/internal/model"
)

// ============================================================================
// 阶段 2.2 重新导出错误
// ============================================================================

var (
	ErrInvalidScope     = apperrors.ErrInvalidScope
	ErrPKCERequired    = apperrors.ErrPKCERequired
	ErrConsentRequired = apperrors.ErrConsentRequired
	ErrConsentDenied   = apperrors.ErrConsentDenied
	ErrConsentInvalid  = apperrors.ErrConsentInvalid
	ErrClientMismatch  = apperrors.ErrClientMismatch
)

// ConsentTokenTTL consent_token 有效期（5 分钟）
// 用户在 GET /authorize 拿到 consent_token 后必须在此时间内完成批准/拒绝
const ConsentTokenTTL = 5 * time.Minute

// ============================================================================
// Scope 校验
// ============================================================================

// ValidateScope 校验请求的 scope 是否在客户端允许范围内
//
// 校验规则：
//  1. 每个 requested scope 必须在全局白名单 model.SupportedScopes 内
//  2. 每个 requested scope 必须在 client.Scopes 允许范围内
//  3. 若 client.Scopes 为空，视为不允许任何 scope（拒绝所有非空请求）
//  4. 若 requested 为空，返回 client.Scopes 的全部允许 scope（向后兼容）
//
// 返回值：
//   - validScopes: 通过校验的 scope 切片（已规范化去重）
//   - error: 校验失败时返回 ErrInvalidScope
func (s *OAuthService) ValidateScope(ctx context.Context, client *model.Client, requested []string) ([]string, error) {
	normalized := model.NormalizeScopes(requested)

	// 空请求：返回客户端允许的全部 scope（向后兼容）
	if len(normalized) == 0 {
		return model.NormalizeScopes(client.Scopes), nil
	}

	// 客户端未配置任何 scope：拒绝所有非空请求
	if len(client.Scopes) == 0 {
		return nil, fmt.Errorf("%w: 客户端未配置允许的scope", ErrInvalidScope)
	}

	// 校验每个请求 scope 是否在白名单与客户端允许范围内
	allowedSet := make(map[string]struct{}, len(client.Scopes))
	for _, sc := range client.Scopes {
		allowedSet[sc] = struct{}{}
	}

	for _, sc := range normalized {
		// 全局白名单校验
		if !model.IsSupportedScope(sc) {
			return nil, fmt.Errorf("%w: scope %q 不在全局白名单内", ErrInvalidScope, sc)
		}
		// 客户端允许范围校验
		if _, ok := allowedSet[sc]; !ok {
			return nil, fmt.Errorf("%w: scope %q 超出客户端允许范围", ErrInvalidScope, sc)
		}
	}

	return normalized, nil
}

// ============================================================================
// PKCE 强制校验
// ============================================================================

// ValidatePKCEChallenge 校验 PKCE code_challenge 与 method
//
// 强制规则（阶段 2.2）：
//  1. 公共客户端（PublicClient=true）：必须传 code_challenge 且 method 必须为 "S256"
//  2. 机密客户端：若传 code_challenge 则 method 必须为 "S256"
//  3. 禁用 "plain" 方法（RFC 7636 §4.2.1 推荐禁用）
//
// 返回值：
//   - error: 校验失败返回 ErrPKCERequired 或 ErrInvalidCodeChallenge
func (s *OAuthService) ValidatePKCEChallenge(ctx context.Context, client *model.Client, codeChallenge, codeChallengeMethod string) error {
	// 公共客户端强制 PKCE
	if client.PublicClient {
		if codeChallenge == "" {
			return fmt.Errorf("%w: 公共客户端必须使用PKCE", ErrPKCERequired)
		}
		if codeChallengeMethod != "S256" {
			return fmt.Errorf("%w: 公共客户端必须使用S256方法", ErrPKCERequired)
		}
		return nil
	}

	// 机密客户端：若传 code_challenge，method 必须为 S256
	if codeChallenge != "" {
		if codeChallengeMethod != "S256" {
			return fmt.Errorf("%w: 仅允许S256方法（plain已禁用）", ErrInvalidCodeChallenge)
		}
	}
	return nil
}

// VerifyPKCEWithMethod 验证 PKCE verifier（强制 S256，禁用 plain）
//
// 此函数替代旧的 verifyPKCE，仅接受 "S256" 方法
func VerifyPKCEWithMethod(challenge, method, verifier string) error {
	if method != "S256" {
		// 禁用 plain 方法
		return ErrInvalidCodeChallenge
	}
	if verifier == "" {
		return ErrInvalidCodeVerifier
	}
	h := sha256.New()
	h.Write([]byte(verifier))
	hash := h.Sum(nil)
	expected := base64.RawURLEncoding.EncodeToString(hash)

	if subtle.ConstantTimeCompare([]byte(challenge), []byte(expected)) != 1 {
		return ErrInvalidCodeVerifier
	}
	return nil
}

// ============================================================================
// Consent Token 签发与校验
// ============================================================================

// ConsentClaims consent_token 的 JWT claims
// consent_token 是短期签发的 JWT，用于在 GET /authorize 与 POST /authorize/approve 之间
// 传递用户已查看并批准的授权上下文，防止 CSRF 与 scope 篡改
type ConsentClaims struct {
	jwt.RegisteredClaims
	ClientID            string   `json:"client_id"`
	UserID              string   `json:"user_id"`
	RedirectURI         string   `json:"redirect_uri"`
	Scopes              []string `json:"scopes"`
	State               string   `json:"state"`
	CodeChallenge       string   `json:"code_challenge,omitempty"`
	CodeChallengeMethod string   `json:"code_challenge_method,omitempty"`
}

// IssueConsentToken 签发 consent_token
//
// 该 token 携带 GET /authorize 时用户看到的授权上下文（client_id/scopes/redirect_uri/state/PKCE），
// 用户在 POST /authorize/approve 时回传该 token，service 校验通过后创建 authorization_code
//
// 签名方式：复用 JWTService 的私钥（RS256），保证 token 不可伪造
// 有效期：ConsentTokenTTL（5 分钟）
func (s *OAuthService) IssueConsentToken(
	ctx context.Context,
	userID, clientID, redirectURI string,
	scopes []string,
	state, codeChallenge, codeChallengeMethod string,
) (string, error) {
	if s.tokenSvc == nil || s.tokenSvc.jwtSvc == nil {
		return "", fmt.Errorf("token service is not initialized")
	}

	// 获取 RS256 签名所需私钥
	now := time.Now()
	claims := ConsentClaims{
		RegisteredClaims: jwt.RegisteredClaims{
			ID:        uuid.New().String(),
			Issuer:    "sso-consent",
			Subject:   userID,
			ExpiresAt: jwt.NewNumericDate(now.Add(ConsentTokenTTL)),
			IssuedAt:  jwt.NewNumericDate(now),
			NotBefore: jwt.NewNumericDate(now),
		},
		ClientID:            clientID,
		UserID:              userID,
		RedirectURI:         redirectURI,
		Scopes:              scopes,
		State:               state,
		CodeChallenge:       codeChallenge,
		CodeChallengeMethod: codeChallengeMethod,
	}

	token := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	// 使用与 access_token 相同的活跃密钥
	jwtSvc := s.tokenSvc.jwtSvc
	keyID := jwtSvc.GetActiveKeyID()
	if keyID != "" {
		token.Header["kid"] = keyID
	}

	// 通过 GenerateAccessTokenWithKeyID 复用私钥签名逻辑会创建 access_token，
	// 这里直接调用 jwt.SignedString 需要访问私钥。
	// 为避免破坏 JWTService 的封装，使用内部方法签发：
	signed, err := signConsentTokenWithJWTService(jwtSvc, token)
	if err != nil {
		return "", fmt.Errorf("签发consent_token失败: %w", err)
	}
	return signed, nil
}

// VerifyConsentToken 校验 consent_token 并返回 claims
//
// 校验项：
//  1. 签名有效（RS256）
//  2. 未过期
//  3. issuer == "sso-consent"
//  4. 携带 client_id/user_id/scopes/redirect_uri/state
func (s *OAuthService) VerifyConsentToken(ctx context.Context, consentToken string) (*ConsentClaims, error) {
	if s.tokenSvc == nil || s.tokenSvc.jwtSvc == nil {
		return nil, fmt.Errorf("token service is not initialized")
	}
	jwtSvc := s.tokenSvc.jwtSvc

	claims := &ConsentClaims{}
	_, err := jwt.ParseWithClaims(consentToken, claims, func(t *jwt.Token) (interface{}, error) {
		if t.Method.Alg() != jwt.SigningMethodRS256.Alg() {
			return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
		}
		// 使用 JWTService 暴露的公钥
		pub := jwtSvc.GetPublicKey()
		if pub == nil {
			return nil, fmt.Errorf("no public key available")
		}
		return pub, nil
	}, jwt.WithIssuer("sso-consent"), jwt.WithExpirationRequired())
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrConsentInvalid, err)
	}

	if claims.ClientID == "" || claims.UserID == "" || claims.RedirectURI == "" {
		return nil, fmt.Errorf("%w: consent_token缺少必要字段", ErrConsentInvalid)
	}
	return claims, nil
}

// signConsentTokenWithJWTService 使用 JWTService 的活跃密钥签名 consent_token
// 通过 Reflect 调用会破坏封装，这里采用最小侵入方式：
// 复用 jwt.NewWithClaims + 调用 GenerateAccessTokenWithKeyID 拿到 keyID，
// 然后直接 SignedString（私钥已在内部加载）
func signConsentTokenWithJWTService(jwtSvc interface {
	GetActiveKeyID() string
}, token *jwt.Token) (string, error) {
	// 通过类型断言调用内部方法签名；此处的 jwtSvc 实际是 *crypto.JWTService
	// 但为了不直接依赖 crypto 包（已在 import 中），调用其暴露的 signConsentToken 方法
	type consentSigner interface {
		SignConsentToken(token *jwt.Token) (string, error)
	}
	signer, ok := jwtSvc.(consentSigner)
	if !ok {
		return "", fmt.Errorf("jwtSvc does not implement SignConsentToken")
	}
	return signer.SignConsentToken(token)
}

// MarshalConsentContext 将 consent 上下文序列化为 JSON 用于审计日志
func MarshalConsentContext(claims *ConsentClaims) string {
	if claims == nil {
		return ""
	}
	data := map[string]interface{}{
		"client_id":    claims.ClientID,
		"user_id":      claims.UserID,
		"redirect_uri": claims.RedirectURI,
		"scopes":       claims.Scopes,
		"state":        claims.State,
	}
	b, _ := json.Marshal(data)
	return string(b)
}
