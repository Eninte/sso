// Package service 业务逻辑层
// 处理用户认证相关的业务逻辑
package service

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/your-org/sso/internal/cache"
	"github.com/your-org/sso/internal/crypto"
	apperrors "github.com/your-org/sso/internal/errors"
	"github.com/your-org/sso/internal/metrics"
	"github.com/your-org/sso/internal/model"
	"github.com/your-org/sso/internal/store"
	"github.com/your-org/sso/internal/util/auditutil"
	"github.com/your-org/sso/internal/util/retryutil"
	"github.com/your-org/sso/internal/util/serviceutil"
	"github.com/your-org/sso/internal/validator"
)

// ============================================================================
// 使用统一的错误定义
// ============================================================================

// 重新导出统一的错误，保持向后兼容
var (
	ErrInvalidCredentials = apperrors.ErrInvalidCredentials
	ErrAccountLocked      = apperrors.ErrAccountLocked
	ErrAccountDisabled    = apperrors.ErrAccountDisabled
	ErrInvalidToken       = apperrors.ErrInvalidToken
	ErrEmailNotVerified   = apperrors.ErrEmailNotVerified
)

// ============================================================================
// AuthService 认证服务
// ============================================================================

// AuthServiceOption AuthService配置选项
type AuthServiceOption func(*AuthService)

// WithCache 设置缓存服务
func WithCache(cacheSvc cache.Cache) AuthServiceOption {
	return func(s *AuthService) {
		s.cache = cacheSvc
	}
}

// WithUserService 设置用户服务
func WithUserService(userSvc *UserService) AuthServiceOption {
	return func(s *AuthService) {
		s.userSvc = userSvc
	}
}

// WithAudit 设置审计服务
func WithAudit(auditSvc *AuditService) AuthServiceOption {
	return func(s *AuthService) {
		s.auditSvc = auditSvc
	}
}

// WithMetrics 设置指标服务
func WithMetrics(metricsSvc *metrics.Service) AuthServiceOption {
	return func(s *AuthService) {
		s.metricsSvc = metricsSvc
	}
}

// WithLoginRateLimit 设置登录频率限制器
func WithLoginRateLimit(limiter *LoginRateLimiter) AuthServiceOption {
	return func(s *AuthService) {
		s.loginRateLimit = limiter
	}
}

// AuthService 认证服务
// 处理用户认证相关的业务逻辑
type AuthService struct {
	store           store.Store             // 数据存储
	passwordSvc     *crypto.PasswordService // 密码服务
	jwtSvc          *crypto.JWTService      // JWT服务
	tokenSvc        *TokenService           // Token生成服务
	userSvc         *UserService            // 用户服务（用于发送验证邮件）
	maxAttempts     int                     // 最大登录尝试次数
	lockoutDuration time.Duration           // 锁定时长
	metricsSvc      *metrics.Service        // 指标服务（可选）
	auditSvc        *AuditService           // 审计服务
	cache           cache.Cache             // 缓存服务（可选）
	loginRateLimit  *LoginRateLimiter       // 登录频率限制器（可选）
}

// NewAuthService 创建AuthService实例
// 支持通过选项函数配置可选依赖（缓存、审计服务等）
func NewAuthService(
	store store.Store,
	passwordSvc *crypto.PasswordService,
	jwtSvc *crypto.JWTService,
	maxAttempts int,
	lockoutDuration time.Duration,
	metricsSvc ...*metrics.Service,
) *AuthService {
	var m *metrics.Service
	if len(metricsSvc) > 0 {
		m = metricsSvc[0]
	}
	return &AuthService{
		store:           store,
		passwordSvc:     passwordSvc,
		jwtSvc:          jwtSvc,
		tokenSvc:        NewTokenService(jwtSvc, store),
		maxAttempts:     maxAttempts,
		lockoutDuration: lockoutDuration,
		metricsSvc:      m,
		auditSvc:        NewAuditService(store),
	}
}

// NewAuthServiceWithOptions 创建带选项的AuthService实例
func NewAuthServiceWithOptions(
	store store.Store,
	passwordSvc *crypto.PasswordService,
	jwtSvc *crypto.JWTService,
	maxAttempts int,
	lockoutDuration time.Duration,
	options ...AuthServiceOption,
) *AuthService {
	svc := &AuthService{
		store:           store,
		passwordSvc:     passwordSvc,
		jwtSvc:          jwtSvc,
		tokenSvc:        NewTokenService(jwtSvc, store),
		maxAttempts:     maxAttempts,
		lockoutDuration: lockoutDuration,
		auditSvc:        NewAuditService(store),
	}

	for _, opt := range options {
		opt(svc)
	}

	return svc
}

// incrementMetric 增加指标计数（安全调用）
func (s *AuthService) incrementMetric(name string) {
	if s.metricsSvc != nil {
		s.metricsSvc.Increment(name)
	}
}

// ============================================================================
// 注册功能
// ============================================================================

// Register 用户注册
// 1. 验证输入
// 2. 检查邮箱是否已注册
// 3. 哈希密码
// 4. 创建用户记录
func (s *AuthService) Register(ctx context.Context, req *model.RegisterRequest) (*model.User, error) {
	// 1. 验证输入参数
	if err := validator.ValidateRegisterRequest(req.Email, req.Password); err != nil {
		return nil, err
	}

	// 2. 检查邮箱是否已注册
	existingUser, err := s.store.GetByEmail(ctx, req.Email)
	if err != nil && !apperrors.Is(err, store.ErrNotFound) {
		return nil, serviceutil.WrapServiceError("检查邮箱", err)
	}
	if existingUser != nil {
		// 不暴露邮箱已存在：返回nil错误和nil用户
		// handler层对nil用户返回通用成功消息
		return nil, nil
	}

	// 3. 哈希密码
	hashedPassword, err := s.passwordSvc.HashPassword(req.Password)
	if err != nil {
		return nil, serviceutil.WrapServiceError("哈希密码", err)
	}

	// 4. 创建用户
	now := time.Now()
	user := &model.User{
		ID:           uuid.New().String(),
		Email:        req.Email,
		PasswordHash: hashedPassword,
		Status:       model.UserStatusPending,
		CreatedAt:    now,
		UpdatedAt:    now,
	}

	if err := s.store.Create(ctx, user); err != nil {
		return nil, serviceutil.WrapServiceError("创建用户", err)
	}

	// 5. 异步发送验证邮件（不阻塞注册响应）
	go func() {
		if err := s.sendVerificationEmail(context.Background(), user); err != nil {
			slog.Warn("发送验证邮件失败", "error", err, "userID", user.ID)
		}
	}()

	// 记录注册成功指标
	s.incrementMetric("auth_register_total")

	return user, nil
}

// sendVerificationEmail 发送验证邮件（内部方法）
func (s *AuthService) sendVerificationEmail(ctx context.Context, user *model.User) error {
	if s.userSvc == nil {
		return nil
	}
	return s.userSvc.SendVerificationEmail(ctx, user.ID)
}

// ============================================================================
// 登录功能
// ============================================================================

// Login 用户登录
// 1. 验证输入
// 2. 获取用户
// 3. 检查账户状态
// 4. 验证密码
// 5. 生成Token
type AuditContext struct {
	IPAddress string
	UserAgent string
}

// validateUserCredentials 验证用户凭据
// 包含用户查询、密码验证、账户状态检查
// 返回验证通过的用户对象或统一的错误类型
// validateUserCredentials 验证用户凭据（邮箱和密码）
// 此函数从LoginWithAudit中提取，用于降低主函数的复杂度
//
// 职责:
//   - 按邮箱查询用户
//   - 检查邮箱是否已验证
//   - 检查账户状态（禁用/锁定）
//   - 验证密码哈希
//   - 处理账户锁定过期的情况
//
// 参数:
//   - ctx: 请求上下文
//   - email: 用户邮箱
//   - password: 用户密码（明文）
//
// 返回:
//   - 如果验证成功，返回用户对象
//   - 如果用户不存在或密码错误，返回ErrInvalidCredentials
//   - 如果邮箱未验证，返回ErrEmailNotVerified
//   - 如果账户被禁用，返回ErrAccountDisabled
//   - 如果账户被锁定，返回ErrAccountLocked
//
// 重构原因: 从LoginWithAudit中提取验证逻辑，降低主函数复杂度（21→<10）
func (s *AuthService) validateUserCredentials(ctx context.Context, email, password string) (*model.User, error) {
	// 查询用户
	user, err := s.store.GetByEmail(ctx, email)
	if err != nil {
		return nil, serviceutil.HandleStoreError(err, ErrInvalidCredentials)
	}

	// 检查邮箱是否已验证
	if !user.EmailVerified {
		slog.Debug("用户尝试使用未验证邮箱登录", "user_id", user.ID)
		// 不暴露邮箱未验证状态，返回通用凭据错误
		// 同时触发发送验证邮件，帮助用户完成验证
		if s.userSvc != nil {
			_ = s.userSvc.SendVerificationEmail(ctx, user.Email)
		}
		return user, ErrInvalidCredentials
	}

	// 检查账户状态
	if user.Status == model.UserStatusDisabled {
		return nil, ErrAccountDisabled
	}

	if user.Status == model.UserStatusLocked {
		if user.LockedUntil != nil && user.LockedUntil.After(time.Now()) {
			return nil, ErrAccountLocked
		}
		// 使用原子操作解锁过期账户，避免竞态条件
		if unlockErr := s.store.UnlockExpiredAccount(ctx, user.ID); unlockErr != nil {
			if !apperrors.Is(unlockErr, store.ErrNotFound) {
				slog.Warn("解锁过期账户失败", "error", unlockErr, "user_id", user.ID)
			}
			// 即使解锁失败也继续尝试登录（可能是并发解锁）
		}
	}

	// 验证密码
	if err := s.passwordSvc.VerifyPassword(user.PasswordHash, password); err != nil {
		// 密码错误时仍返回user对象，避免handleLoginFailure重复查询DB
		return user, ErrInvalidCredentials
	}

	return user, nil
}

// handleLoginFailure 处理登录失败的情况
// 此函数从LoginWithAudit中提取，用于降低主函数的复杂度
//
// 职责:
//   - 递增登录失败次数（原子操作）
//   - 检查是否超过最大尝试次数，如果是则锁定账户
//   - 记录登录失败指标
//   - 记录审计日志（使用SafeAuditLog确保失败不影响主流程）
//   - 如果账户被锁定，记录账户锁定指标和审计日志
//
// 参数:
//   - ctx: 请求上下文
//   - user: 已查询的用户对象（避免重复查询DB）
//   - auditCtx: 审计上下文（可以为nil）
//
// 返回:
//   - 如果递增登录尝试次数失败，返回错误（防止绕过账户锁定机制）
//   - 其他错误（如审计日志失败）不返回，确保不影响主流程
//
// 优化: 接收user对象而非email，消除冗余的GetByEmail查询
// 安全修复: 数据库错误时返回错误，防止攻击者通过触发DB错误绕过账户锁定
func (s *AuthService) handleLoginFailure(ctx context.Context, user *model.User, auditCtx *AuditContext) error {
	// 使用原子操作递增登录尝试次数，避免竞态条件
	attempts, locked, _, incErr := s.store.IncrementLoginAttempts(ctx, user.ID, s.maxAttempts, s.lockoutDuration)
	if incErr != nil {
		// 安全修复：数据库错误时返回错误，防止绕过账户锁定机制
		slog.Error("更新登录尝试次数失败", "error", incErr, "user_id", user.ID)
		return serviceutil.WrapServiceError("更新登录尝试次数", incErr)
	}

	// 账户被锁定
	if locked {
		s.incrementMetric("auth_account_locked_total")
		// 使用统一的审计日志工具记录账户锁定事件
		if auditCtx != nil {
			auditutil.SafeAuditLog(ctx, s.auditSvc, string(model.EventAccountLocked), user.ID, map[string]interface{}{
				"ip_address": auditCtx.IPAddress,
				"attempts":   attempts,
			})
		}
		slog.Warn("账户因多次登录失败被锁定", "user_id", user.ID, "attempts", attempts)
	}

	// 记录登录失败指标
	s.incrementMetric("auth_login_failed_total")

	// 使用统一的审计日志工具记录登录失败事件
	if auditCtx != nil {
		auditutil.SafeAuditLog(ctx, s.auditSvc, string(model.EventUserLogin), user.ID, map[string]interface{}{
			"email":      user.Email,
			"ip_address": auditCtx.IPAddress,
			"user_agent": auditCtx.UserAgent,
			"success":    false,
		})
	}

	return nil
}

// handleLoginSuccess 处理登录成功的情况
// 此函数从LoginWithAudit中提取，用于降低主函数的复杂度
//
// 职责:
//   - 重置登录失败次数
//   - 记录登录成功指标
//   - 记录审计日志（使用SafeAuditLog确保失败不影响主流程）
//   - 生成并返回token对（access token和refresh token）
//
// 参数:
//   - ctx: 请求上下文
//   - user: 已验证的用户对象
//   - auditCtx: 审计上下文（可以为nil）
//
// 返回:
//   - 如果成功，返回LoginResponse（包含access token和refresh token）
//   - 如果生成token失败，返回错误
//
// 重构原因: 从LoginWithAudit中提取成功处理逻辑，降低主函数复杂度（21→<10）
func (s *AuthService) handleLoginSuccess(ctx context.Context, user *model.User, auditCtx *AuditContext) (*model.LoginResponse, error) {
	// 并行执行：重置登录尝试 + 审计日志
	var wg sync.WaitGroup

	// 异步重置登录尝试次数
	wg.Add(1)
	go func() {
		defer wg.Done()
		if err := s.store.ResetLoginAttempts(ctx, user.ID); err != nil {
			slog.Warn("重置登录尝试次数失败", "error", err, "user_id", user.ID)
		}
	}()

	// 异步记录审计日志
	wg.Add(1)
	go func() {
		defer wg.Done()
		if auditCtx != nil {
			auditutil.SafeAuditLog(ctx, s.auditSvc, string(model.EventUserLogin), user.ID, map[string]interface{}{
				"email":      user.Email,
				"ip_address": auditCtx.IPAddress,
				"user_agent": auditCtx.UserAgent,
			})
		}
	}()

	// 记录登录成功指标（轻量操作，无需异步）
	s.incrementMetric("auth_login_total")

	// 生成token对（必须等待，依赖用户信息）
	resp, err := s.generateTokenPair(ctx, user.ID, user.Email, user.Role, []string{"openid", "profile", "email"}, nil)

	// 等待异步操作完成
	wg.Wait()

	return resp, err
}

// LoginWithAudit 执行登录操作并记录审计日志
// 此函数已重构以降低复杂度，通过提取验证、失败处理、成功处理逻辑
//
// 职责:
//   - 验证登录请求格式
//   - 验证用户凭据（调用validateUserCredentials）
//   - 处理登录失败（调用handleLoginFailure）
//   - 处理登录成功（调用handleLoginSuccess）
//
// 参数:
//   - ctx: 请求上下文
//   - req: 登录请求（包含email和password）
//   - auditCtx: 审计上下文（包含IP地址、User-Agent等）
//
// 返回:
//   - 如果登录成功，返回LoginResponse（包含tokens）
//   - 如果登录失败，返回错误
//
// 重构原因: 原始复杂度为21，通过提取验证、失败处理、成功处理逻辑，降低到<10
// 提取的函数:
//   - validateUserCredentials: 验证用户凭据
//   - handleLoginFailure: 处理登录失败
//   - handleLoginSuccess: 处理登录成功
func (s *AuthService) LoginWithAudit(ctx context.Context, req *model.LoginRequest, auditCtx *AuditContext) (*model.LoginResponse, error) {
	if err := validator.ValidateLoginRequest(req.Email, req.Password); err != nil {
		return nil, err
	}

	// IP维度登录频率限制（防止撞库和账户锁定DoS）
	if s.loginRateLimit != nil && auditCtx != nil && auditCtx.IPAddress != "" {
		allowed, _, rateLimitErr := s.loginRateLimit.CheckAndRecord(ctx, auditCtx.IPAddress)
		if rateLimitErr != nil {
			slog.Error("IP登录限流检查失败", "error", rateLimitErr, "ip", auditCtx.IPAddress)
		}
		if !allowed {
			slog.Warn("IP登录频率超限", "ip", auditCtx.IPAddress)
			s.incrementMetric("auth_login_rate_limited_total")
			auditutil.SafeAuditLog(ctx, s.auditSvc, string(model.EventUserLogin), "", map[string]interface{}{
				"ip_address": auditCtx.IPAddress,
				"email":      req.Email,
				"success":    false,
				"reason":     "ip_rate_limited",
			})
			return nil, apperrors.ErrTooManyRequests
		}
	}

	user, err := s.validateUserCredentials(ctx, req.Email, req.Password)
	if err != nil {
		// 处理密码验证失败的情况
		if apperrors.Is(err, ErrInvalidCredentials) && user != nil {
			// validateUserCredentials在密码错误时返回user对象，避免重复查询DB
			// 安全修复：检查handleLoginFailure的返回值，防止绕过账户锁定
			if failErr := s.handleLoginFailure(ctx, user, auditCtx); failErr != nil {
				slog.Error("处理登录失败时出错", "error", failErr, "user_id", user.ID)
				// 安全修复：数据库错误时返回服务错误，防止绕过账户锁定机制
				// 不返回ErrInvalidCredentials，因为我们无法确定是否成功记录失败次数
				return nil, serviceutil.WrapServiceError("记录登录失败", failErr)
			}
		}
		return nil, err
	}

	// 处理登录成功
	return s.handleLoginSuccess(ctx, user, auditCtx)
}

func (s *AuthService) Login(ctx context.Context, req *model.LoginRequest) (*model.LoginResponse, error) {
	return s.LoginWithAudit(ctx, req, nil)
}

// ============================================================================
// Token刷新功能
// ============================================================================

// revokeTokenWithRetry 使用指数退避算法重试撤销Token
// 使用retryutil.ExponentialBackoffRetry实现重试逻辑
// 保持缓存清除逻辑不变，确保Token内容在日志中被掩码
func (s *AuthService) revokeTokenWithRetry(ctx context.Context, accessToken string) error {
	config := retryutil.DefaultRetryConfig()

	return retryutil.ExponentialBackoffRetry(ctx, func(ctx context.Context) error {
		if err := s.store.RevokeToken(ctx, accessToken); err != nil {
			return err
		}

		// 清除缓存（失败不影响主流程）
		if s.cache != nil {
			cacheKey := cache.TokenKey(accessToken)
			if err := s.cache.Delete(ctx, cacheKey); err != nil {
				slog.Warn("清除Token缓存失败", "error", err, "token", maskToken(accessToken))
			}
		}

		return nil
	}, config)
}

// RefreshToken 刷新Token
func (s *AuthService) RefreshTokenWithAudit(ctx context.Context, refreshToken string, auditCtx *AuditContext) (*model.LoginResponse, error) {
	slog.Debug("RefreshToken: 开始刷新Token", "refresh_token_length", len(refreshToken))
	tokenRecord, err := s.store.GetTokenByRefreshToken(ctx, refreshToken)
	if err != nil {
		slog.Error("RefreshToken: 查询Token失败", "error", err, "refresh_token_length", len(refreshToken))
		// 安全设计：不暴露token是否存在，所有错误都返回ErrInvalidToken
		return nil, ErrInvalidToken
	}
	slog.Debug("RefreshToken: 查询到Token", "token_id", tokenRecord.ID, "user_id", tokenRecord.UserID)

	if tokenRecord.RevokedAt != nil {
		slog.Warn("RefreshToken: Token已撤销", "token_id", tokenRecord.ID, "revoked_at", tokenRecord.RevokedAt)
		return nil, ErrInvalidToken
	}

	user, err := s.store.GetByID(ctx, tokenRecord.UserID)
	if err != nil {
		slog.Error("RefreshToken: 查询用户失败", "error", err, "user_id", tokenRecord.UserID)
		return nil, serviceutil.WrapServiceError("查询用户", err)
	}
	slog.Debug("RefreshToken: 查询到用户", "user_id", user.ID, "email", user.Email)

	if revokeErr := s.revokeTokenWithRetry(ctx, tokenRecord.AccessToken); revokeErr != nil {
		slog.Error("撤销旧Token失败，已达到最大重试次数",
			"error", revokeErr,
			"user_id", tokenRecord.UserID,
			"token_id", tokenRecord.ID,
		)
		// 撤销失败时返回错误，避免生成冲突的access_token
		return nil, serviceutil.WrapServiceError("撤销旧Token", revokeErr)
	}
	slog.Debug("RefreshToken: 旧Token已撤销", "token_id", tokenRecord.ID)

	s.incrementMetric("auth_token_refresh_total")

	if auditCtx != nil {
		auditutil.SafeAuditLog(ctx, s.auditSvc, string(model.EventTokenRefresh), user.ID, map[string]interface{}{
			"client_id":  tokenRecord.GetClientID(),
			"ip_address": auditCtx.IPAddress,
		})
	}

	return s.generateTokenPair(ctx, user.ID, user.Email, user.Role, tokenRecord.Scopes, tokenRecord.ClientID)
}

func (s *AuthService) RefreshToken(ctx context.Context, refreshToken string) (*model.LoginResponse, error) {
	return s.RefreshTokenWithAudit(ctx, refreshToken, nil)
}

// ============================================================================
// 登出功能
// ============================================================================

// LogoutWithAudit 用户登出（带审计日志）
func (s *AuthService) LogoutWithAudit(ctx context.Context, accessToken string, auditCtx *AuditContext) error {
	claims, err := s.jwtSvc.ValidateAccessToken(accessToken)
	if err == nil && auditCtx != nil {
		auditutil.SafeAuditLog(ctx, s.auditSvc, string(model.EventUserLogout), claims.Subject, map[string]interface{}{
			"ip_address": auditCtx.IPAddress,
		})
	}
	if err := s.revokeTokenWithRetry(ctx, accessToken); err != nil {
		slog.Error("登出时撤销Token失败",
			"error", err,
			"token_prefix", maskToken(accessToken),
		)
		return serviceutil.WrapServiceError("登出", err)
	}
	s.incrementMetric("auth_token_revoke_total")
	return nil
}

// Logout 用户登出
func (s *AuthService) Logout(ctx context.Context, accessToken string) error {
	return s.LogoutWithAudit(ctx, accessToken, nil)
}

// LogoutAllWithAudit 登出所有设备（带审计日志）
func (s *AuthService) LogoutAllWithAudit(ctx context.Context, userID string, auditCtx *AuditContext) error {
	if err := s.store.RevokeAllUserTokens(ctx, userID); err != nil {
		slog.Error("撤销所有Token失败",
			"error", err,
			"user_id", userID,
		)
		return serviceutil.WrapServiceError("登出所有设备", err)
	}

	// 清除该用户相关的缓存（失败不影响主流程）
	if s.cache != nil {
		if err := s.cache.DeletePattern(ctx, cache.TokenCachePrefix+"*"); err != nil {
			slog.Warn("清除用户Token缓存失败", "error", err, "user_id", userID)
		}
	}

	s.incrementMetric("auth_logout_all_total")
	if auditCtx != nil {
		auditutil.SafeAuditLog(ctx, s.auditSvc, string(model.EventLogoutAll), userID, map[string]interface{}{
			"ip_address": auditCtx.IPAddress,
		})
	}
	return nil
}

// LogoutAll 登出所有设备
func (s *AuthService) LogoutAll(ctx context.Context, userID string) error {
	return s.LogoutAllWithAudit(ctx, userID, nil)
}

// maskToken 掩盖Token用于日志记录（只显示前8位）
func maskToken(token string) string {
	if len(token) <= 8 {
		return "***"
	}
	return token[:8] + "..."
}

// ============================================================================
// Token验证功能
// ============================================================================

// ValidateToken 验证Token
func (s *AuthService) ValidateToken(ctx context.Context, accessToken string) (*crypto.AccessTokenClaims, error) {
	claims, err := s.jwtSvc.ValidateAccessToken(accessToken)
	if err != nil {
		return nil, err
	}

	// 检查缓存
	if s.cache != nil {
		var cachedToken model.Token
		cacheKey := cache.TokenKey(accessToken)
		if err := s.cache.Get(ctx, cacheKey, &cachedToken); err == nil {
			if cachedToken.RevokedAt != nil {
				return nil, ErrInvalidToken
			}
			return claims, nil
		}
	}

	// 缓存未命中，查询数据库
	tokenRecord, err := s.store.GetTokenByAccessToken(ctx, accessToken)
	if err != nil {
		// 安全设计：不暴露token是否存在，所有错误都返回ErrInvalidToken
		return nil, ErrInvalidToken
	}
	if tokenRecord.RevokedAt != nil {
		return nil, ErrInvalidToken
	}

	// 缓存结果（失败不影响主流程）
	if s.cache != nil {
		cacheKey := cache.TokenKey(accessToken)
		if err := s.cache.Set(ctx, cacheKey, tokenRecord, cache.TokenTTL); err != nil {
			slog.Warn("缓存Token记录失败", "error", err)
		}
	}

	return claims, nil
}

// ============================================================================
// 内部辅助方法
// ============================================================================

// generateTokenPair 生成Token对
// 使用TokenService统一处理Token生成逻辑
func (s *AuthService) generateTokenPair(
	ctx context.Context,
	userID, email, role string,
	scopes []string,
	clientID *string,
) (*model.LoginResponse, error) {
	slog.Debug("generateTokenPair开始", "userID", userID, "email", email)
	resp, err := s.tokenSvc.GenerateTokenPair(ctx, userID, email, role, scopes, clientID)
	if err != nil {
		slog.Error("generateTokenPair失败", "error", err, "userID", userID)
	}
	return resp, err
}
