// 登录相关逻辑（从 auth.go 拆分）
package service

import (
	"context"
	"sync"
	"time"

	apperrors "github.com/example/sso/internal/errors"
	"github.com/example/sso/internal/logging"
	"github.com/example/sso/internal/model"
	"github.com/example/sso/internal/store"
	"github.com/example/sso/internal/util/auditutil"
	"github.com/example/sso/internal/util/serviceutil"
	"github.com/example/sso/internal/validator"
)

// validateUserCredentials 验证用户凭据：查询用户 → 检查邮箱验证/账户状态 → 验证密码
// 密码错误时返回 user 对象（非 nil），供 handleLoginFailure 复用避免重复查询
func (s *AuthService) validateUserCredentials(ctx context.Context, email, password string) (*model.User, error) {
	logger := logging.WithContext(ctx)
	// 查询用户
	user, err := s.store.GetByEmail(ctx, email)
	if err != nil {
		return nil, serviceutil.HandleStoreError(err, ErrInvalidCredentials)
	}

	// 检查邮箱是否已验证
	if !user.EmailVerified {
		logger.Debug("用户尝试使用未验证邮箱登录", "user_id", user.ID)
		// 不暴露邮箱未验证状态，返回通用凭据错误
		// 同时触发发送验证邮件，帮助用户完成验证
		if s.userSvc != nil {
			_ = s.userSvc.SendVerificationEmail(ctx, user.ID)
		}
		// 返回nil用户，避免触发handleLoginFailure的登录失败计数
		// 合法用户未验证邮箱不应被锁定
		return nil, ErrInvalidCredentials
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
				logger.Warn("解锁过期账户失败", "error", unlockErr, "user_id", user.ID)
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

// handleLoginFailure 处理登录失败：递增失败次数（超阈值则锁定）+ 记录指标与审计日志
// 数据库错误时返回错误，防止攻击者通过触发DB错误绕过账户锁定机制
func (s *AuthService) handleLoginFailure(ctx context.Context, user *model.User, auditCtx *AuditContext) error {
	logger := logging.WithContext(ctx)
	// 使用原子操作递增登录尝试次数，避免竞态条件
	attempts, locked, _, incErr := s.store.IncrementLoginAttempts(ctx, user.ID, s.maxAttempts, s.lockoutDuration)
	if incErr != nil {
		// 安全修复：数据库错误时返回错误，防止绕过账户锁定机制
		logger.Error("更新登录尝试次数失败", "error", incErr, "user_id", user.ID)
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
		logger.Warn("账户因多次登录失败被锁定", "user_id", user.ID, "attempts", attempts)
	}

	// 记录登录失败指标
	s.incrementMetric("auth_login_failed_total")

	// 使用统一的审计日志工具记录登录失败事件
	if auditCtx != nil {
		auditutil.SafeAuditLog(ctx, s.auditSvc, string(model.EventUserLoginFailed), user.ID, map[string]interface{}{
			"email":      user.Email,
			"ip_address": auditCtx.IPAddress,
			"user_agent": auditCtx.UserAgent,
			"success":    false,
		})
	}

	return nil
}

// handleLoginSuccess 处理登录成功：并行重置失败次数与审计日志，同步生成 token 对
func (s *AuthService) handleLoginSuccess(ctx context.Context, user *model.User, auditCtx *AuditContext) (*model.LoginResponse, error) {
	// 并行执行：重置登录尝试 + 审计日志
	var wg sync.WaitGroup

	// 异步重置登录尝试次数
	// 使用 context.WithoutCancel 避免请求返回后 ctx 被取消导致 DB 调用失败
	bgCtx := context.WithoutCancel(ctx)
	wg.Add(1)
	go func() {
		defer wg.Done()
		if err := s.store.ResetLoginAttempts(bgCtx, user.ID); err != nil {
			logging.WithContext(bgCtx).Warn("重置登录尝试次数失败", "error", err, "user_id", user.ID)
		}
	}()

	// 异步记录审计日志
	wg.Add(1)
	go func() {
		defer wg.Done()
		if auditCtx != nil {
			auditutil.SafeAuditLog(bgCtx, s.auditSvc, string(model.EventUserLogin), user.ID, map[string]interface{}{
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
// 流程: 验证请求格式 → 验证凭据 → 处理失败/成功（含审计日志与指标）
func (s *AuthService) LoginWithAudit(ctx context.Context, req *model.LoginRequest, auditCtx *AuditContext) (*model.LoginResponse, error) {
	logger := logging.WithContext(ctx)
	if err := validator.ValidateLoginRequest(req.Email, req.Password); err != nil {
		return nil, err
	}

	// IP维度登录频率限制（防止撞库和账户锁定DoS）
	if s.loginRateLimit != nil && auditCtx != nil && auditCtx.IPAddress != "" {
		allowed, _, rateLimitErr := s.loginRateLimit.CheckAndRecord(ctx, auditCtx.IPAddress)
		if rateLimitErr != nil {
			logger.Error("IP登录限流检查失败", "error", rateLimitErr, "ip", auditCtx.IPAddress)
		}
		if !allowed {
			logger.Warn("IP登录频率超限", "ip", auditCtx.IPAddress)
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
				logger.Error("处理登录失败时出错", "error", failErr, "user_id", user.ID)
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
