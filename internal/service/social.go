// Package service 第三方登录服务
// 处理Google、GitHub等第三方OAuth登录
package service

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/your-org/sso/internal/crypto"
	apperrors "github.com/your-org/sso/internal/errors"
	"github.com/your-org/sso/internal/model"
	"github.com/your-org/sso/internal/store"
)

// ============================================================================
// 使用统一的错误定义
// ============================================================================

var (
	ErrProviderNotSupported = apperrors.ErrProviderNotSupported
	ErrOAuthStateInvalid    = apperrors.ErrInvalidToken // 临时映射
	ErrOAuthCodeInvalid     = apperrors.ErrInvalidCode  // 临时映射
	ErrSocialLoginFailed    = apperrors.ErrInternal     // 临时映射
)

// ============================================================================
// OAuth提供商
// ============================================================================

type OAuthProvider struct {
	Name         string
	ClientID     string
	ClientSecret string
	AuthURL      string
	TokenURL     string
	UserInfoURL  string
	Scopes       []string
}

// HTTPClient HTTP客户端接口
// 支持注入mock用于测试
type HTTPClient interface {
	Do(req *http.Request) (*http.Response, error)
}

// ============================================================================
// SocialLoginService 第三方登录服务
// ============================================================================

type SocialLoginService struct {
	store      store.Store
	jwtSvc     *crypto.JWTService
	providers  map[string]*OAuthProvider
	httpClient HTTPClient
}

func NewSocialLoginService(
	store store.Store,
	jwtSvc *crypto.JWTService,
	googleClientID, googleClientSecret string,
	githubClientID, githubClientSecret string,
) *SocialLoginService {
	providers := make(map[string]*OAuthProvider)

	if googleClientID != "" {
		providers["google"] = &OAuthProvider{
			Name:         "google",
			ClientID:     googleClientID,
			ClientSecret: googleClientSecret,
			AuthURL:      "https://accounts.google.com/o/oauth2/v2/auth",
			TokenURL:     "https://oauth2.googleapis.com/token",
			UserInfoURL:  "https://www.googleapis.com/oauth2/v2/userinfo",
			Scopes:       []string{"email", "profile"},
		}
	}

	if githubClientID != "" {
		providers["github"] = &OAuthProvider{
			Name:         "github",
			ClientID:     githubClientID,
			ClientSecret: githubClientSecret,
			AuthURL:      "https://github.com/login/oauth/authorize",
			TokenURL:     "https://github.com/login/oauth/access_token",
			UserInfoURL:  "https://api.github.com/user",
			Scopes:       []string{"user:email"},
		}
	}

	return &SocialLoginService{
		store:      store,
		jwtSvc:     jwtSvc,
		providers:  providers,
		httpClient: http.DefaultClient,
	}
}

// NewSocialLoginServiceWithProviders 使用自定义providers创建社交登录服务
// 支持测试注入mock provider和HTTP client
func NewSocialLoginServiceWithProviders(
	store store.Store,
	jwtSvc *crypto.JWTService,
	providers map[string]*OAuthProvider,
	httpClient HTTPClient,
) *SocialLoginService {
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	return &SocialLoginService{
		store:      store,
		jwtSvc:     jwtSvc,
		providers:  providers,
		httpClient: httpClient,
	}
}

// ============================================================================
// 公开方法
// ============================================================================

func (s *SocialLoginService) GetProviders() []string {
	providers := make([]string, 0, len(s.providers))
	for name := range s.providers {
		providers = append(providers, name)
	}
	return providers
}

func (s *SocialLoginService) GetAuthorizationURL(provider, redirectURI, state string) (string, error) {
	p, ok := s.providers[provider]
	if !ok {
		return "", ErrProviderNotSupported
	}

	if redirectURI == "" {
		redirectURI = "http://localhost:9090/auth/" + provider + "/callback"
	}

	params := url.Values{
		"client_id":     {p.ClientID},
		"redirect_uri":  {redirectURI},
		"response_type": {"code"},
		"scope":         {strings.Join(p.Scopes, " ")},
		"state":         {state},
	}

	return p.AuthURL + "?" + params.Encode(), nil
}

func (s *SocialLoginService) HandleCallback(ctx context.Context, provider, code, redirectURI string) (*model.LoginResponse, error) {
	p, ok := s.providers[provider]
	if !ok {
		return nil, ErrProviderNotSupported
	}

	if redirectURI == "" {
		redirectURI = "http://localhost:9090/auth/" + provider + "/callback"
	}

	accessToken, err := s.exchangeCode(p, code, redirectURI)
	if err != nil {
		return nil, err
	}

	userInfo, err := s.getUserInfo(p, accessToken)
	if err != nil {
		return nil, err
	}

	user, err := s.findOrCreateUser(ctx, provider, userInfo)
	if err != nil {
		return nil, err
	}

	return s.generateTokenPair(ctx, user)
}

// ============================================================================
// 内部方法
// ============================================================================

func (s *SocialLoginService) exchangeCode(p *OAuthProvider, code, redirectURI string) (string, error) {
	data := url.Values{
		"client_id":     {p.ClientID},
		"client_secret": {p.ClientSecret},
		"code":          {code},
		"grant_type":    {"authorization_code"},
		"redirect_uri":  {redirectURI},
	}

	req, err := http.NewRequest("POST", p.TokenURL, strings.NewReader(data.Encode()))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	var result map[string]interface{}
	if err := json.Unmarshal(body, &result); err != nil {
		return "", err
	}

	if token, ok := result["access_token"].(string); ok {
		return token, nil
	}

	return "", ErrOAuthCodeInvalid
}

func (s *SocialLoginService) getUserInfo(p *OAuthProvider, accessToken string) (map[string]interface{}, error) {
	req, err := http.NewRequest("GET", p.UserInfoURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var userInfo map[string]interface{}
	if err := json.Unmarshal(body, &userInfo); err != nil {
		return nil, err
	}

	return userInfo, nil
}

func (s *SocialLoginService) findOrCreateUser(ctx context.Context, provider string, userInfo map[string]interface{}) (*model.User, error) {
	var email string

	switch provider {
	case "google":
		email, _ = userInfo["email"].(string)
	case "github":
		email, _ = userInfo["email"].(string)
		if email == "" {
			if login, ok := userInfo["login"].(string); ok {
				email = login + "@github.com"
			}
		}
	}

	if email == "" {
		return nil, ErrSocialLoginFailed
	}

	user, err := s.store.GetByEmail(ctx, email)
	if err != nil {
		if !apperrors.Is(err, store.ErrNotFound) {
			return nil, err
		}

		now := time.Now()
		user = &model.User{
			ID:            uuid.New().String(),
			Email:         email,
			EmailVerified: true,
			Status:        model.UserStatusActive,
			CreatedAt:     now,
			UpdatedAt:     now,
		}

		if err := s.store.Create(ctx, user); err != nil {
			return nil, err
		}
	}

	return user, nil
}

func (s *SocialLoginService) generateTokenPair(ctx context.Context, user *model.User) (*model.LoginResponse, error) {
	accessToken, err := s.jwtSvc.GenerateAccessToken(user.ID, user.Email, []string{"openid", "profile", "email"})
	if err != nil {
		return nil, err
	}

	refreshToken, err := s.jwtSvc.GenerateRefreshToken()
	if err != nil {
		return nil, err
	}

	tokenRecord := &model.Token{
		ID:           uuid.New().String(),
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
		UserID:       user.ID,
		Scopes:       []string{"openid", "profile", "email"},
		ExpiresAt:    time.Now().Add(s.jwtSvc.GetAccessTokenTTL()),
		CreatedAt:    time.Now(),
	}

	if err := s.store.StoreToken(ctx, tokenRecord); err != nil {
		return nil, err
	}

	return &model.LoginResponse{
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
		TokenType:    "Bearer",
		ExpiresIn:    int(s.jwtSvc.GetAccessTokenTTL().Seconds()),
	}, nil
}
