// Token 刷新、登出、验证逻辑（从 auth.go 拆分）
package service

import (
	"context"
	"errors"
	"time"

	"github.com/example/sso/internal/cache"
	"github.com/example/sso/internal/crypto"
	apperrors "github.com/example/sso/internal/errors"
	"github.com/example/sso/internal/logging"
	"github.com/example/sso/internal/model"
	"github.com/example/sso/internal/store"
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

// handleRefreshTokenReplay 处理 Refresh Token 重放攻击
//
// 触发场景：
//   - RotateRefreshToken 返回 ErrTokenRotated（旧 token 已被轮换/撤销）
//   - 即同一个 refresh token 被第二次提交
//
// 防御措施（fail-secure）：
//  1. 记录 CriticalAuditLog 标记可疑活动
//  2. 撤销该用户的全部 token（防止攻击者已获取的新 token 继续使用）
//  3. 返回 ErrTokenRotated 提示用户重新登录
//
// 注意：即使撤销全部失败也返回错误，安全优先于可用性
func (s *AuthService) handleRefreshTokenReplay(ctx context.Context, userID, refreshToken string, auditCtx *AuditContext) error {
	logger := logging.WithContext(ctx)
	logger.Error("检测到 Refresh Token 重放攻击，撤销用户全部 Token",
		"user_id", userID,
		"refresh_token_length", len(refreshToken),
	)

	// 记录关键审计日志（同步返回错误）
	ipAddress := ""
	if auditCtx != nil {
		ipAddress = auditCtx.IPAddress
	}
	auditutil.CriticalAuditLog(ctx, s.auditSvc, string(model.EventSuspiciousActivity), userID, map[string]interface{}{
		"reason":           "refresh_token_replay",
		"client_id":        "",
		"ip_address":       ipAddress,
		"refresh_token_len": len(refreshToken),
	})

	// 撤销该用户的全部 token（失败也返回错误，但优先记录日志）
	if err := s.store.RevokeAllUserTokens(ctx, userID); err != nil {
		logger.Error("重放攻击下撤销全部 Token 失败",
			"error", err,
			"user_id", userID,
		)
		return serviceutil.WrapServiceError("撤销全部Token", err)
	}

	return apperrors.ErrTokenRotated
}

// RefreshTokenWithAudit 原子地轮换 Refresh Token
//
// 流程（阶段 2.1 安全增强 + 阶段 2.2 客户端归属校验）：
//  1. 获取旧 token 记录；不存在/DB 错误 → ErrInvalidToken（不暴露具体原因）
//  2. 检查 refresh_expires_at：已过期 → ErrInvalidToken
//  3. 检查旧 token revoked_at：已撤销 → 视为重放，调用 handleRefreshTokenReplay
//  4. 阶段 2.2: 校验客户端归属 — 若 tokenRecord.ClientID 不为空且 clientID 与之不一致
//     则返回 ErrClientMismatch（RFC 6749 §10.4 防御 token 替换攻击）
//  5. 获取用户、检查状态（禁用/锁定）
//  6. 生成新 token 记录（含 RefreshExpiresAt）
//  7. 调用 store.RotateRefreshToken 原子轮换（事务内 UPDATE+INSERT）
//     - 若返回 ErrTokenRotated（已被并发轮换）→ handleRefreshTokenReplay
//  8. 清除旧 token 缓存
//  9. 记录审计日志，返回新 token
//
// 安全设计：
//   - 原子性：UPDATE+INSERT 在同一事务内，避免 TOCTOU 竞态
//   - 一次性：WHERE rotated_at IS NULL 保证只能轮换一次
//   - 重放检测：RowsAffected==0 时视为被盗用，撤销全部 token
//   - 客户端归属：拒绝跨客户端使用 refresh token
//
// 参数：
//   - clientID: 调用方传入的客户端 ID；空字符串表示不校验（向后兼容登录流程）
//     OAuth token 流程必须传 clientID
func (s *AuthService) RefreshTokenWithAudit(ctx context.Context, refreshToken, clientID string, auditCtx *AuditContext) (*model.LoginResponse, error) {
	logger := logging.WithContext(ctx)
	logger.Debug("RefreshToken: 开始刷新Token", "refresh_token_length", len(refreshToken))

	// 1. 获取旧 token 记录
	tokenRecord, err := s.store.GetTokenByRefreshToken(ctx, refreshToken)
	if err != nil {
		logger.Error("RefreshToken: 查询Token失败", "error", err, "refresh_token_length", len(refreshToken))
		// 安全设计：不暴露token是否存在，所有错误都返回ErrInvalidToken
		return nil, ErrInvalidToken
	}
	logger.Debug("RefreshToken: 查询到Token", "token_id", tokenRecord.ID, "user_id", tokenRecord.UserID)

	// 2. 检查 refresh token 独立过期时间
	// 兼容旧数据：RefreshExpiresAt 为 nil 时回退到 ExpiresAt
	refreshExpiry := tokenRecord.RefreshExpiresAt
	if refreshExpiry == nil {
		refreshExpiry = &tokenRecord.ExpiresAt
	}
	if refreshExpiry.Before(time.Now()) {
		logger.Warn("RefreshToken: Refresh Token 已过期",
			"token_id", tokenRecord.ID,
			"refresh_expires_at", refreshExpiry,
		)
		return nil, ErrInvalidToken
	}

	// 3. 检查旧 token 是否已撤销
	// 已撤销的 token 再次出现 = 重放攻击特征
	if tokenRecord.RevokedAt != nil {
		logger.Warn("RefreshToken: Token 已撤销，触发重放防御",
			"token_id", tokenRecord.ID,
			"revoked_at", tokenRecord.RevokedAt,
		)
		return nil, s.handleRefreshTokenReplay(ctx, tokenRecord.UserID, refreshToken, auditCtx)
	}

	// 4. 阶段 2.2: 校验客户端归属
	// 若 token 由 OAuth 流程签发（ClientID 不为 nil），则调用方必须传相同的 clientID
	// 防御 token 替换攻击：攻击者无法用自己客户端的凭据刷新其他客户端签发的 token
	if tokenRecord.ClientID != nil && *tokenRecord.ClientID != "" {
		if clientID == "" {
			logger.Warn("RefreshToken: OAuth签发的Token未传client_id",
				"token_id", tokenRecord.ID,
				"token_client_id", *tokenRecord.ClientID,
			)
			return nil, ErrClientMismatch
		}
		if *tokenRecord.ClientID != clientID {
			logger.Warn("RefreshToken: client_id与Token归属不一致",
				"token_id", tokenRecord.ID,
				"token_client_id", *tokenRecord.ClientID,
				"request_client_id", clientID,
			)
			auditutil.SafeAuditLog(ctx, s.auditSvc, string(model.EventSuspiciousActivity), tokenRecord.UserID, map[string]interface{}{
				"reason":            "refresh_token_client_mismatch",
				"token_client_id":   *tokenRecord.ClientID,
				"request_client_id": clientID,
			})
			return nil, ErrClientMismatch
		}
	}

	// 5. 获取用户、检查状态
	user, err := s.store.GetByID(ctx, tokenRecord.UserID)
	if err != nil {
		logger.Error("RefreshToken: 查询用户失败", "error", err, "user_id", tokenRecord.UserID)
		return nil, serviceutil.WrapServiceError("查询用户", err)
	}
	logger.Debug("RefreshToken: 查询到用户", "user_id", user.ID, "email", user.Email)

	if user.Status == model.UserStatusDisabled {
		logger.Warn("RefreshToken: 用户已被禁用", "user_id", user.ID)
		return nil, ErrAccountDisabled
	}
	if user.Status == model.UserStatusLocked {
		logger.Warn("RefreshToken: 用户已被锁定", "user_id", user.ID)
		return nil, ErrAccountLocked
	}

	// 6. 生成新 token 记录（不写入存储）
	newToken, resp, err := s.tokenSvc.GenerateTokenRecord(
		ctx, user.ID, user.Email, user.Role, tokenRecord.Scopes, tokenRecord.ClientID,
	)
	if err != nil {
		logger.Error("RefreshToken: 生成新Token失败",
			"error", err,
			"user_id", user.ID,
		)
		return nil, err
	}

	// 7. 原子轮换：事务内标记旧 token 已轮换 + 插入新 token
	if err := s.store.RotateRefreshToken(ctx, refreshToken, newToken); err != nil {
		if errors.Is(err, store.ErrTokenRotated) {
			// 并发重放或重放攻击：旧 token 已被轮换/撤销
			logger.Warn("RefreshToken: 检测到 Token 已被轮换（重放攻击或并发请求）",
				"token_id", tokenRecord.ID,
				"user_id", user.ID,
			)
			return nil, s.handleRefreshTokenReplay(ctx, user.ID, refreshToken, auditCtx)
		}
		logger.Error("RefreshToken: 原子轮换失败",
			"error", err,
			"token_id", tokenRecord.ID,
			"user_id", user.ID,
		)
		return nil, serviceutil.WrapServiceError("轮换RefreshToken", err)
	}

	// 8. 清除旧 token 缓存（失败不影响主流程）
	if s.cache != nil {
		cacheKey := cache.TokenKey(tokenRecord.AccessToken)
		if err := s.cache.Delete(ctx, cacheKey); err != nil {
			logger.Warn("清除旧Token缓存失败",
				"error", err,
				"token_prefix", maskToken(tokenRecord.AccessToken),
			)
		}
	}

	s.incrementMetric("auth_token_refresh_total")

	// 9. 记录审计日志
	if auditCtx != nil {
		auditutil.SafeAuditLog(ctx, s.auditSvc, string(model.EventTokenRefresh), user.ID, map[string]interface{}{
			"client_id":  tokenRecord.GetClientID(),
			"ip_address": auditCtx.IPAddress,
		})
	}

	return resp, nil
}

// RefreshToken 兼容旧接口：刷新 Token（不校验 clientID）
// 保留用于登录流程签发的 token（ClientID 为 nil）刷新
func (s *AuthService) RefreshToken(ctx context.Context, refreshToken string) (*model.LoginResponse, error) {
	return s.RefreshTokenWithAudit(ctx, refreshToken, "", nil)
}

// RefreshTokenWithClientID 携带 clientID 刷新 Token（阶段 2.2）
// 用于 OAuth 流程签发的 token 刷新，校验 clientID 与 token 归属一致
func (s *AuthService) RefreshTokenWithClientID(ctx context.Context, refreshToken, clientID string) (*model.LoginResponse, error) {
	return s.RefreshTokenWithAudit(ctx, refreshToken, clientID, nil)
}

// ============================================================================
// 登出功能
// ============================================================================

// LogoutWithAudit 用户登出（带审计日志）
func (s *AuthService) LogoutWithAudit(ctx context.Context, accessToken string, auditCtx *AuditContext) error {
	logger := logging.WithContext(ctx)
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
