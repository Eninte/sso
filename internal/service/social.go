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

	"github.com/example/sso/internal/cache"
	"github.com/example/sso/internal/crypto"
	apperrors "github.com/example/sso/internal/errors"
	"github.com/example/sso/internal/model"
	"github.com/example/sso/internal/store"
	"github.com/example/sso/internal/util/auditutil"
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
	auditSvc   *AuditService // 审计日志服务（可选）
	baseURL    string        // 服务基础URL，用于构建固定的回调地址
	providers  map[string]*OAuthProvider
	httpClient HTTPClient
	cache      cache.Cache   // 阶段 2.3：state 缓存（Redis 优先，内存回退）
	stateCache sync.Map      // 内存回退缓存（cache 为 nil 时使用）
	stopChan   chan struct{} // 用于停止清理goroutine
	stopOnce   sync.Once     // 确保 Close 只执行一次
}

// SetAuditService 设置审计日志服务
func (s *SocialLoginService) SetAuditService(auditSvc *AuditService) {
	s.auditSvc = auditSvc
}

// SetCache 设置缓存服务（阶段 2.3：用于 state 的 Redis 存储）
func (s *SocialLoginService) SetCache(cache cache.Cache) {
	s.cache = cache
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

	// 启动后台清理goroutine（仅用于内存回退缓存清理）
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
	s.stopOnce.Do(func() {
		close(s.stopChan)
	})
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

// OAuthStateTTL state 缓存 TTL（5 分钟）
const OAuthStateTTL = 5 * time.Minute

// stateCachePrefix state 在 Redis 中的 key 前缀
const stateCachePrefix = "oauth:state:"

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

	// 阶段 2.3：state 存入 Redis（如可用）+ 内存回退
	info := stateInfo{
		provider:    provider,
		redirectURI: redirectURI,
		createdAt:   time.Now(),
	}

	if s.cache != nil {
		// Redis 存储：写入 state，TTL 由 cache 层处理
		// 注意：Redis 不会自动删除 key（除非 TTL 到期），调用方需在 HandleCallback 中 Delete
		if err := s.cache.Set(context.Background(), stateCachePrefix+state, info, OAuthStateTTL); err != nil {
			// Redis 写入失败，回退到内存（保证可用性）
			s.stateCache.Store(state, info)
		}
	} else {
		// 内存回退
		s.stateCache.Store(state, info)
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

func (s *SocialLoginService) HandleCallback(ctx context.Context, provider, code, state string) (*model.LoginResponse, error) {
	p, ok := s.providers[provider]
	if !ok {
		return nil, ErrProviderNotSupported
	}

	// 验证state参数（防止CSRF攻击）
	if state == "" {
		return nil, ErrOAuthStateInvalid
	}

	// 阶段 2.3：从 Redis（优先）或内存中获取并删除 state（原子操作防止 TOCTOU/重放）
	info, err := s.loadAndDeleteState(ctx, state)
	if err != nil {
		return nil, err
	}

	// 使用state缓存中存储的redirectURI（由GetAuthorizationURL设置）
	// 不接受客户端传入的redirectURI，防止开放重定向攻击
	redirectURI := info.redirectURI

	accessToken, err := s.exchangeCode(ctx, p, code, redirectURI)
	if err != nil {
		// 外部OAuth调用失败，映射为具体错误码而非ErrInternal
		return nil, apperrors.Wrap(apperrors.ErrCodeOAuthCodeExchangeFailed, "OAuth授权码交换失败", 400, err)
	}

	userInfo, err := s.getUserInfo(ctx, p, accessToken)
	if err != nil {
		return nil, apperrors.Wrap(apperrors.ErrCodeSocialLoginFailed, "社交登录失败", 400, err)
	}

	// 阶段 2.3：从 provider 返回的 userInfo 提取身份信息
	identity, err := ExtractProviderIdentity(provider, userInfo)
	if err != nil {
		auditutil.SafeAuditLog(ctx, s.auditSvc, string(model.EventSocialLoginRejected), "", map[string]interface{}{
			"provider": provider,
			"reason":   "provider_identity_extract_failed",
			"error":    err.Error(),
		})
		return nil, err
	}

	// 阶段 2.3：通过 (provider, provider_user_id) 查找或创建用户
	user, err := s.findOrCreateSocialUser(ctx, provider, identity)
	if err != nil {
		return nil, err
	}

	// Token 生成成功后记录社交登录审计日志
	// 避免生成失败时审计中留下假阳性的登录成功事件
	resp, err := s.generateTokenPair(ctx, user)
	if err != nil {
		return nil, err
	}

	if s.auditSvc != nil {
		auditutil.SafeAuditLog(ctx, s.auditSvc, string(model.EventUserLogin), user.ID, map[string]interface{}{
			"provider":         provider,
			"provider_user_id": identity.ProviderUserID,
			"email":             user.Email,
		})
	}

	return resp, nil
}

// loadAndDeleteState 原子地从缓存中加载并删除 state
//
// 优先使用 Redis（如配置），fallback 到内存 sync.Map
// 原子性保证：使用 Redis 的 GET + DEL（非事务但足够防 TOCTOU），或 sync.Map.LoadAndDelete
// 5 分钟过期：Redis 通过 TTL，内存通过 createdAt 检查
func (s *SocialLoginService) loadAndDeleteState(ctx context.Context, state string) (*stateInfo, error) {
	// 1. 优先从 Redis 加载
	if s.cache != nil {
		var info stateInfo
		err := s.cache.Get(ctx, stateCachePrefix+state, &info)
		if err == nil {
			// 加载成功，立即删除（防重放）
			_ = s.cache.Delete(ctx, stateCachePrefix+state)
			// 检查过期（双保险，Redis TTL 应已处理）
			if time.Since(info.createdAt) > OAuthStateTTL {
				return nil, ErrOAuthStateExpired
			}
			return &info, nil
		}
		// Redis 未命中，回退到内存（可能服务刚启动，Redis 还没数据）
	}

	// 2. 内存回退
	stateVal, loaded := s.stateCache.LoadAndDelete(state)
	if !loaded {
		return nil, ErrOAuthStateInvalid
	}

	info, ok := stateVal.(stateInfo)
	if !ok {
		return nil, ErrOAuthStateInvalid
	}

	if time.Since(info.createdAt) > OAuthStateTTL {
		return nil, ErrOAuthStateExpired
	}

	return &info, nil
}

// ============================================================================
// 内部方法
// ============================================================================

func (s *SocialLoginService) exchangeCode(ctx context.Context, p *OAuthProvider, code, redirectURI string) (string, error) {
	data := url.Values{
		"client_id":     {p.ClientID},
		"client_secret": {p.ClientSecret},
		"code":          {code},
		"grant_type":    {"authorization_code"},
		"redirect_uri":  {redirectURI},
	}

	req, err := http.NewRequestWithContext(ctx, "POST", p.TokenURL, strings.NewReader(data.Encode()))
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

func (s *SocialLoginService) getUserInfo(ctx context.Context, p *OAuthProvider, accessToken string) (map[string]interface{}, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", p.UserInfoURL, nil)
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

func (s *SocialLoginService) generateTokenPair(ctx context.Context, user *model.User) (*model.LoginResponse, error) {
	return s.tokenSvc.GenerateTokenPair(ctx, user.ID, user.Email, user.Role, []string{"openid", "profile", "email"}, nil)
}
