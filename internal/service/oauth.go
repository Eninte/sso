// Package service OAuth服务
// 处理OAuth2授权码流程和PKCE验证
package service

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"fmt"
	"time"

	"github.com/google/uuid"

	apperrors "github.com/your-org/sso/internal/errors"
	"github.com/your-org/sso/internal/model"
	"github.com/your-org/sso/internal/store"
)

// ============================================================================
// 使用统一的错误定义
// ============================================================================

// 重新导出统一的错误，保持向后兼容
var (
	ErrInvalidClient        = apperrors.ErrInvalidClient
	ErrInvalidRedirectURI   = apperrors.ErrInvalidRedirectURI
	ErrInvalidGrantType     = apperrors.ErrInvalidGrantType
	ErrInvalidCode          = apperrors.ErrInvalidCode
	ErrCodeExpired          = apperrors.ErrCodeExpiredErr
	ErrCodeUsed             = apperrors.ErrCodeUsedErr
	ErrInvalidCodeVerifier  = apperrors.ErrInvalidCodeVerifier
	ErrInvalidCodeChallenge = apperrors.ErrInvalidPKCEChallenge
)

// ============================================================================
// OAuthService OAuth服务
// ============================================================================

// OAuthService OAuth服务
// 处理OAuth2授权码流程
type OAuthService struct {
	store store.Store // 数据存储
}

// NewOAuthService 创建OAuth服务
func NewOAuthService(store store.Store) *OAuthService {
	return &OAuthService{store: store}
}

// ============================================================================
// 授权码流程
// ============================================================================

// CreateAuthorizationCode 创建授权码
func (s *OAuthService) CreateAuthorizationCode(
	ctx context.Context,
	clientID string,
	userID string,
	redirectURI string,
	scopes []string,
	codeChallenge string,
	codeChallengeMethod string,
) (string, error) {
	client, err := s.store.GetByClientID(ctx, clientID)
	if err != nil {
		return "", ErrInvalidClient
	}

	if !s.store.ValidateRedirectURI(ctx, clientID, redirectURI) {
		return "", ErrInvalidRedirectURI
	}

	hasAuthCodeGrant := false
	for _, grant := range client.GrantTypes {
		if grant == model.GrantTypeAuthorizationCode {
			hasAuthCodeGrant = true
			break
		}
	}
	if !hasAuthCodeGrant {
		return "", ErrInvalidGrantType
	}

	if codeChallenge != "" {
		if codeChallengeMethod != "S256" && codeChallengeMethod != "plain" {
			return "", ErrInvalidCodeChallenge
		}
	}

	code, err := generateRandomString(32)
	if err != nil {
		return "", fmt.Errorf("生成授权码失败: %w", err)
	}

	authCode := &model.AuthorizationCode{
		Code:                code,
		ClientID:            clientID,
		UserID:              userID,
		RedirectURI:         redirectURI,
		Scopes:              scopes,
		CodeChallenge:       codeChallenge,
		CodeChallengeMethod: codeChallengeMethod,
		ExpiresAt:           time.Now().Add(10 * time.Minute),
		CreatedAt:           time.Now(),
	}

	if err := s.store.StoreAuthorizationCode(ctx, authCode); err != nil {
		return "", fmt.Errorf("存储授权码失败: %w", err)
	}

	return code, nil
}

// ExchangeAuthorizationCode 交换授权码
func (s *OAuthService) ExchangeAuthorizationCode(
	ctx context.Context,
	code string,
	clientID string,
	clientSecret string,
	redirectURI string,
	codeVerifier string,
) (*model.LoginResponse, error) {
	authCode, err := s.store.GetAuthorizationCode(ctx, code)
	if err != nil {
		return nil, ErrInvalidCode
	}

	if authCode.ClientID != clientID {
		return nil, ErrInvalidClient
	}

	if authCode.RedirectURI != redirectURI {
		return nil, ErrInvalidRedirectURI
	}

	if authCode.ExpiresAt.Before(time.Now()) {
		return nil, ErrCodeExpired
	}

	if authCode.UsedAt != nil {
		return nil, ErrCodeUsed
	}

	client, err := s.store.GetByClientID(ctx, clientID)
	if err != nil {
		return nil, ErrInvalidClient
	}

	if !client.PublicClient {
		if !compareClientSecret(client.ClientSecret, clientSecret) {
			return nil, ErrInvalidClient
		}
	}

	if authCode.CodeChallenge != "" {
		if err := verifyPKCE(authCode.CodeChallenge, authCode.CodeChallengeMethod, codeVerifier); err != nil {
			return nil, ErrInvalidCodeVerifier
		}
	}

	now := time.Now()
	authCode.UsedAt = &now
	if err := s.store.UpdateAuthorizationCode(ctx, authCode); err != nil {
		return nil, fmt.Errorf("更新授权码状态失败: %w", err)
	}

	user, err := s.store.GetByID(ctx, authCode.UserID)
	if err != nil {
		return nil, fmt.Errorf("获取用户信息失败: %w", err)
	}

	tokenRecord := &model.Token{
		ID:           uuid.New().String(),
		AccessToken:  "",
		RefreshToken: "",
		UserID:       authCode.UserID,
		ClientID:     authCode.ClientID,
		Scopes:       authCode.Scopes,
		ExpiresAt:    time.Now().Add(15 * time.Minute),
		CreatedAt:    time.Now(),
	}

	_ = user
	_ = tokenRecord

	return &model.LoginResponse{
		AccessToken:  "access_token_placeholder",
		RefreshToken: "refresh_token_placeholder",
		TokenType:    "Bearer",
		ExpiresIn:    900,
	}, nil
}

// RevokeToken 撤销Token
func (s *OAuthService) RevokeToken(ctx context.Context, token string) error {
	return s.store.RevokeToken(ctx, token)
}

// ============================================================================
// 辅助函数
// ============================================================================

func verifyPKCE(challenge, method, verifier string) error {
	if method == "plain" {
		if challenge != verifier {
			return ErrInvalidCodeVerifier
		}
		return nil
	}

	if method == "S256" {
		h := sha256.New()
		h.Write([]byte(verifier))
		hash := h.Sum(nil)
		expected := base64.RawURLEncoding.EncodeToString(hash)

		if subtle.ConstantTimeCompare([]byte(challenge), []byte(expected)) != 1 {
			return ErrInvalidCodeVerifier
		}
		return nil
	}

	return ErrInvalidCodeChallenge
}

func generateRandomString(length int) (string, error) {
	bytes := make([]byte, length)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(bytes), nil
}

func compareClientSecret(stored, provided string) bool {
	return subtle.ConstantTimeCompare([]byte(stored), []byte(provided)) == 1
}
