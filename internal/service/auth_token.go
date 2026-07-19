// Token 刷新、登出、验证逻辑（从 auth.go 拆分）
package service

import (
	"context"

	"github.com/example/sso/internal/cache"
	"github.com/example/sso/internal/crypto"
	"github.com/example/sso/internal/logging"
	"github.com/example/sso/internal/model"
	"github.com/example/sso/internal/util/auditutil"
	"github.com/example/sso/internal/util/retryutil"
	"github.com/example/sso/internal/util/serviceutil"
)

// revokeTokenWithRetry 使用指数退避算法重试撤销Token
// 使用retryutil.ExponentialBackoffRetry实现重试逻辑
// 保持缓存清除逻辑不变，确保Token内容在日志中被掩码
func (s *AuthService) revokeTokenWithRetry(ctx context.Context, accessToken string) error {
	logger := logging.WithContext(ctx)
	config := retryutil.DefaultRetryConfig()

	return retryutil.ExponentialBackoffRetry(ctx, func(ctx context.Context) error {
		if err := s.store.RevokeToken(ctx, accessToken); err != nil {
			// 重试循环内不包装错误，避免与retryutil的警告日志重复记录
			// 最终错误由调用方通过WrapServiceError统一包装
			return err
		}

		// 清除缓存（失败不影响主流程）
		if s.cache != nil {
			cacheKey := cache.TokenKey(accessToken)
			if err := s.cache.Delete(ctx, cacheKey); err != nil {
				logger.Warn("清除Token缓存失败", "error", err, "token", maskToken(accessToken))
			}
		}

		return nil
	}, config)
}

// RefreshToken 刷新Token
func (s *AuthService) RefreshTokenWithAudit(ctx context.Context, refreshToken string, auditCtx *AuditContext) (*model.LoginResponse, error) {
	logger := logging.WithContext(ctx)
	logger.Debug("RefreshToken: 开始刷新Token", "refresh_token_length", len(refreshToken))
	tokenRecord, err := s.store.GetTokenByRefreshToken(ctx, refreshToken)
	if err != nil {
		logger.Error("RefreshToken: 查询Token失败", "error", err, "refresh_token_length", len(refreshToken))
		// 安全设计：不暴露token是否存在，所有错误都返回ErrInvalidToken
		return nil, ErrInvalidToken
	}
	logger.Debug("RefreshToken: 查询到Token", "token_id", tokenRecord.ID, "user_id", tokenRecord.UserID)

	if tokenRecord.RevokedAt != nil {
		logger.Warn("RefreshToken: Token已撤销", "token_id", tokenRecord.ID, "revoked_at", tokenRecord.RevokedAt)
		return nil, ErrInvalidToken
	}

	user, err := s.store.GetByID(ctx, tokenRecord.UserID)
	if err != nil {
		logger.Error("RefreshToken: 查询用户失败", "error", err, "user_id", tokenRecord.UserID)
		return nil, serviceutil.WrapServiceError("查询用户", err)
	}
	logger.Debug("RefreshToken: 查询到用户", "user_id", user.ID, "email", user.Email)

	// 检查用户状态（被禁用/锁定的用户不能刷新Token）
	if user.Status == model.UserStatusDisabled {
		logger.Warn("RefreshToken: 用户已被禁用", "user_id", user.ID)
		return nil, ErrAccountDisabled
	}
	if user.Status == model.UserStatusLocked {
		logger.Warn("RefreshToken: 用户已被锁定", "user_id", user.ID)
		return nil, ErrAccountLocked
	}

	// 先撤销旧 Token，成功后再生成新 Token
	// 这确保旧 Refresh Token 不会被重复使用换取新 Token
	// 注意：如果撤销成功但新 Token 生成失败，用户需重新登录（安全优先于可用性）
	if revokeErr := s.revokeTokenWithRetry(ctx, tokenRecord.AccessToken); revokeErr != nil {
		logger.Error("撤销旧Token失败，拒绝刷新以防止Token重用",
			"error", revokeErr,
			"user_id", tokenRecord.UserID,
			"token_id", tokenRecord.ID,
		)
		return nil, serviceutil.WrapServiceError("撤销旧Token", revokeErr)
	}

	resp, err := s.generateTokenPair(ctx, user.ID, user.Email, user.Role, tokenRecord.Scopes, tokenRecord.ClientID)
	if err != nil {
		// 旧 Token 已撤销，新 Token 生成失败，用户需重新登录
		logger.Error("旧Token已撤销但新Token生成失败，用户需重新登录",
			"error", err,
			"user_id", user.ID,
		)
		return nil, err
	}

	s.incrementMetric("auth_token_refresh_total")

	if auditCtx != nil {
		auditutil.SafeAuditLog(ctx, s.auditSvc, string(model.EventTokenRefresh), user.ID, map[string]interface{}{
			"client_id":  tokenRecord.GetClientID(),
			"ip_address": auditCtx.IPAddress,
		})
	}

	return resp, nil
}

func (s *AuthService) RefreshToken(ctx context.Context, refreshToken string) (*model.LoginResponse, error) {
	return s.RefreshTokenWithAudit(ctx, refreshToken, nil)
}

// ============================================================================
// 登出功能
// ============================================================================

// LogoutWithAudit 用户登出（带审计日志）
func (s *AuthService) LogoutWithAudit(ctx context.Context, accessToken string, auditCtx *AuditContext) error {
	logger := logging.WithContext(ctx)
	//nolint:contextcheck // ValidateAccessToken 是纯内存操作（RLock + JWT parse），不涉及 I/O，不需要 ctx
	claims, err := s.jwtSvc.ValidateAccessToken(accessToken)

	if err := s.revokeTokenWithRetry(ctx, accessToken); err != nil {
		logger.Error("登出时撤销Token失败",
			"error", err,
			"token_prefix", maskToken(accessToken),
		)
		return serviceutil.WrapServiceError("登出", err)
	}

	// 撤销成功后记录审计日志
	if err == nil && auditCtx != nil {
		auditutil.SafeAuditLog(ctx, s.auditSvc, string(model.EventUserLogout), claims.Subject, map[string]interface{}{
			"ip_address": auditCtx.IPAddress,
		})
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
	logger := logging.WithContext(ctx)
	if err := s.store.RevokeAllUserTokens(ctx, userID); err != nil {
		logger.Error("撤销所有Token失败",
			"error", err,
			"user_id", userID,
		)
		return serviceutil.WrapServiceError("登出所有设备", err)
	}

	// 清除该用户相关的缓存（失败不影响主流程）
	if s.cache != nil {
		if err := s.cache.DeletePattern(ctx, cache.TokenCachePrefix+"*"); err != nil {
			logger.Warn("清除用户Token缓存失败", "error", err, "user_id", userID)
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
		return nil, serviceutil.WrapServiceError("验证access token", err)
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
			logging.WithContext(ctx).Warn("缓存Token记录失败", "error", err)
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
	logger := logging.WithContext(ctx)
	logger.Debug("generateTokenPair开始", "userID", userID, "email", email)
	resp, err := s.tokenSvc.GenerateTokenPair(ctx, userID, email, role, scopes, clientID)
	if err != nil {
		logger.Error("generateTokenPair失败", "error", err, "userID", userID)
		return nil, serviceutil.WrapServiceError("生成Token对", err)
	}
	return resp, nil
}
