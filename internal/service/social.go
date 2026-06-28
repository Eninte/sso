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
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/example/sso/internal/crypto"
	apperrors "github.com/example/sso/internal/errors"
	"github.com/example/sso/internal/model"
	"github.com/example/sso/internal/store"
	"github.com/example/sso/internal/util/serviceutil"
)

// ============================================================================
// 使用统一的错误定义
// ============================================================================

var (
	ErrProviderNotSupported = apperrors.ErrProviderNotSupported
	ErrOAuthStateInvalid    = apperrors.ErrOAuthStateInvalid
	ErrOAuthStateExpired    = apperrors.ErrOAuthStateExpired
	ErrOAuthCodeInvalid     = apperrors.ErrInvalidCode
	ErrSocialLoginFailed    = apperrors.ErrSocialLoginFailed
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

// stateInfo OAuth state信息
// 用于验证回调请求，防止CSRF攻击
type stateInfo struct {
	provider    string    // 提供商名称
	redirectURI string    // 回调URL
	createdAt   time.Time // 创建时间
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
	tokenSvc   *TokenService
	baseURL    string // 服务基础URL，用于构建固定的回调地址
	providers  map[string]*OAuthProvider
	httpClient HTTPClient
	stateCache sync.Map      // 用于存储OAuth state，防止CSRF攻击
	stopChan   chan struct{} // 用于停止清理goroutine
}

func NewSocialLoginService(
	store store.Store,
	jwtSvc *crypto.JWTService,
	baseURL string,
	googleClientID, googleClientSecret string,
	githubClientID, githubClientSecret string,
) *SocialLoginService {
	providers := make(map[string]*OAuthProvider)

	if googleClientID != "" {
		providers["google"] = &OAuthProvider{ // #nosec G101 -- 这是OAuth提供商配置，不是凭证。ClientID和ClientSecret来自环境变量，不是硬编码的凭证
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
		providers["github"] = &OAuthProvider{ // #nosec G101 -- 这是OAuth提供商配置，不是凭证。ClientID和ClientSecret来自环境变量，不是硬编码的凭证
			Name:         "github",
			ClientID:     githubClientID,
			ClientSecret: githubClientSecret,
			AuthURL:      "https://github.com/login/oauth/authorize",
			TokenURL:     "https://github.com/login/oauth/access_token",
			UserInfoURL:  "https://api.github.com/user",
			Scopes:       []string{"user:email"},
		}
	}

	svc := &SocialLoginService{
		store:      store,
		jwtSvc:     jwtSvc,
		tokenSvc:   NewTokenService(jwtSvc, store),
		baseURL:    baseURL,
		providers:  providers,
		httpClient: &http.Client{Timeout: 10 * time.Second},
		stopChan:   make(chan struct{}),
	}

	// 启动后台清理goroutine
	go svc.cleanupExpiredStates()

	return svc
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
	svc := &SocialLoginService{
		store:      store,
		jwtSvc:     jwtSvc,
		tokenSvc:   NewTokenService(jwtSvc, store),
		baseURL:    "http://localhost:9000",
		providers:  providers,
		httpClient: httpClient,
		stopChan:   make(chan struct{}),
	}

	// 启动后台清理goroutine
	go svc.cleanupExpiredStates()

	return svc
}

// cleanupExpiredStates 定期清理过期的OAuth state缓存
// 防止内存泄漏和拒绝服务攻击
func (s *SocialLoginService) cleanupExpiredStates() {
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			s.stateCache.Range(func(key, value interface{}) bool {
				info, ok := value.(stateInfo)
				if !ok {
					// 无效数据，删除
					s.stateCache.Delete(key)
					return true
				}
				// 删除超过5分钟的state
				if time.Since(info.createdAt) > 5*time.Minute {
					s.stateCache.Delete(key)
				}
				return true
			})
		case <-s.stopChan:
			return
		}
	}
}

// Close 关闭服务，停止后台清理goroutine
func (s *SocialLoginService) Close() {
	close(s.stopChan)
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

func (s *SocialLoginService) GetAuthorizationURL(provider, state string) (string, error) {
	p, ok := s.providers[provider]
	if !ok {
		return "", ErrProviderNotSupported
	}

	// 使用固定的回调地址，不接受客户端传入的redirectURI，防止开放重定向
	redirectURI := s.baseURL + "/auth/" + provider + "/callback"

	// 如果未提供state，生成随机state
	if state == "" {
		state = uuid.New().String()
	}

	// 存储state到缓存（5分钟过期），用于后续验证
	s.stateCache.Store(state, stateInfo{
		provider:    provider,
		redirectURI: redirectURI,
		createdAt:   time.Now(),
	})

	params := url.Values{
		"client_id":     {p.ClientID},
		"redirect_uri":  {redirectURI},
		"response_type": {"code"},
		"scope":         {strings.Join(p.Scopes, " ")},
		"state":         {state},
	}

	return p.AuthURL + "?" + params.Encode(), nil
}

func (s *SocialLoginService) HandleCallback(ctx context.Context, provider, code, state string) (*model.LoginResponse, error) {
	p, ok := s.providers[provider]
	if !ok {
		return nil, ErrProviderNotSupported
	}

	// 验证state参数（防止CSRF攻击）
	if state == "" {
		return nil, ErrOAuthStateInvalid
	}

	// 原子操作：从缓存中获取并删除state（防止TOCTOU竞争和重放攻击）
	stateVal, loaded := s.stateCache.LoadAndDelete(state)
	if !loaded {
		return nil, ErrOAuthStateInvalid
	}

	// 安全类型断言
	info, ok := stateVal.(stateInfo)
	if !ok {
		return nil, ErrOAuthStateInvalid
	}

	// 验证state是否过期（5分钟）
	if time.Since(info.createdAt) > 5*time.Minute {
		return nil, ErrOAuthStateExpired
	}

	// 使用state缓存中存储的redirectURI（由GetAuthorizationURL设置）
	// 不接受客户端传入的redirectURI，防止开放重定向攻击
	redirectURI := info.redirectURI

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

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20)) // 限制响应体最大1MB
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

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20)) // 限制响应体最大1MB
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
			return nil, serviceutil.WrapServiceError("查询用户", err)
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
			return nil, serviceutil.WrapServiceError("创建用户", err)
		}
	}

	return user, nil
}

func (s *SocialLoginService) generateTokenPair(ctx context.Context, user *model.User) (*model.LoginResponse, error) {
	return s.tokenSvc.GenerateTokenPair(ctx, user.ID, user.Email, user.Role, []string{"openid", "profile", "email"}, nil)
}
