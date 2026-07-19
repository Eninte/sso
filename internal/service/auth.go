// Package service 业务逻辑层
// 处理用户认证相关的业务逻辑
package service

import (
	"context"
	"log/slog"
	"time"

	"github.com/google/uuid"

	"github.com/example/sso/internal/cache"
	"github.com/example/sso/internal/crypto"
	apperrors "github.com/example/sso/internal/errors"
	"github.com/example/sso/internal/metrics"
	"github.com/example/sso/internal/model"
	"github.com/example/sso/internal/store"
	"github.com/example/sso/internal/util/auditutil"
	"github.com/example/sso/internal/util/safego"
	"github.com/example/sso/internal/util/serviceutil"
	"github.com/example/sso/internal/validator"
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
// AuthService 协作依赖的最小接口
// 定义这些最小接口（而非依赖具体类型）便于单元测试时替换协作服务，
// 遵循接口隔离原则（ISP），AuthService 只看到它真正需要的方法。
// ============================================================================

// verificationEmailSender 发送验证邮件的最小接口
type verificationEmailSender interface {
	SendVerificationEmail(ctx context.Context, userID string) error
}

// tokenPairGenerator 生成 Token 对的最小接口
type tokenPairGenerator interface {
	GenerateTokenPair(
		ctx context.Context,
		userID, email, role string,
		scopes []string,
		clientID *string,
	) (*model.LoginResponse, error)
}

// metricIncrementer 指标计数的最小接口
type metricIncrementer interface {
	Increment(name string)
}

// loginRateChecker 登录频率检查的最小接口
type loginRateChecker interface {
	CheckAndRecord(ctx context.Context, clientIP string) (bool, int, error)
}

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

// WithUserService 设置用户服务（用于发送验证邮件）
func WithUserService(userSvc verificationEmailSender) AuthServiceOption {
	return func(s *AuthService) {
		s.userSvc = userSvc
	}
}

// WithAudit 设置审计服务
func WithAudit(auditSvc auditutil.AuditService) AuthServiceOption {
	return func(s *AuthService) {
		s.auditSvc = auditSvc
	}
}

// WithMetrics 设置指标服务
func WithMetrics(metricsSvc metricIncrementer) AuthServiceOption {
	return func(s *AuthService) {
		s.metricsSvc = metricsSvc
	}
}

// WithLoginRateLimit 设置登录频率限制器
func WithLoginRateLimit(limiter loginRateChecker) AuthServiceOption {
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
	tokenSvc        tokenPairGenerator      // Token生成服务
	userSvc         verificationEmailSender // 用户服务（用于发送验证邮件）
	maxAttempts     int                     // 最大登录尝试次数
	lockoutDuration time.Duration           // 锁定时长
	metricsSvc      metricIncrementer       // 指标服务（可选）
	auditSvc        auditutil.AuditService  // 审计服务
	cache           cache.Cache             // 缓存服务（可选）
	loginRateLimit  loginRateChecker        // 登录频率限制器（可选）
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
	svc := &AuthService{
		store:           store,
		passwordSvc:     passwordSvc,
		jwtSvc:          jwtSvc,
		tokenSvc:        NewTokenService(jwtSvc, store),
		maxAttempts:     maxAttempts,
		lockoutDuration: lockoutDuration,
		auditSvc:        NewAuditService(store),
	}
	// 仅在传入非 nil 时设置，避免 Go 接口 nil 陷阱
	// （nil *metrics.Service 赋给 metricIncrementer 接口会得到非 nil 接口）
	if len(metricsSvc) > 0 && metricsSvc[0] != nil {
		svc.metricsSvc = metricsSvc[0]
	}
	return svc
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
		// 执行 dummy bcrypt 哈希以消除时序侧信道
		// 邮箱已存在和不存在的路径耗时一致，防止通过响应时间枚举邮箱
		_, _ = s.passwordSvc.HashPassword(req.Password)
		//nolint:nilnil // 故意返回 nil,nil：调用层统一返回相同成功响应，防止邮箱枚举
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
		Role:         model.UserRoleUser,
		Status:       model.UserStatusPending,
		CreatedAt:    now,
		UpdatedAt:    now,
	}

	if err := s.store.Create(ctx, user); err != nil {
		return nil, serviceutil.WrapServiceError("创建用户", err)
	}

	// 5. 异步发送验证邮件（不阻塞注册响应）
	safego.Go(slog.Default(), "发送验证邮件", func() {
		// nosec G118 -- 异步邮件发送使用 context.Background() 是有意为之，
		// 避免请求结束后 context 取消导致邮件发送中断
		if err := s.sendVerificationEmail(context.Background(), user); err != nil {
			slog.Warn("发送验证邮件失败", "error", err, "userID", user.ID)
		}
	})

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

// AuditContext 登录审计上下文，携带 IP 与 UserAgent 用于审计日志
type AuditContext struct {
	IPAddress string
	UserAgent string
}
