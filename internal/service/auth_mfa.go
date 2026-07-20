// 两阶段 MFA 登录 Challenge 管理逻辑
// 安全设计：
//   - Challenge Token：32 字节 CSPRNG 随机数，仅生成时返回给客户端
//   - 绑定：UserID + IPAddress + UserAgent，防止跨设备/跨网络使用
//   - 一次性：验证成功后立即删除；失败次数超限后也删除
//   - 尝试次数：默认最多 5 次（model.MaxMFALoginAttempts），防止暴力枚举
//   - 存储：复用 AuthService.cache（Redis 或内存），TTL 由 config.MFAChallengeTTL 决定
package service

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"time"

	"github.com/example/sso/internal/cache"
	apperrors "github.com/example/sso/internal/errors"
	"github.com/example/sso/internal/logging"
	"github.com/example/sso/internal/model"
	"github.com/example/sso/internal/util/auditutil"
	"github.com/example/sso/internal/util/serviceutil"
)

// handleMFARequiredLogin 处理启用 MFA 用户的登录：生成一次性 Challenge 返回
// 此时不签发 Token，客户端需要再调用 VerifyMFALogin 完成第二阶段验证
func (s *AuthService) handleMFARequiredLogin(ctx context.Context, user *model.User, auditCtx *AuditContext) (*model.LoginResponse, error) {
	// 检查 MFA 服务依赖是否装配
	if s.mfaSvc == nil {
		logging.WithContext(ctx).Error("用户启用 MFA 但 AuthService 未装配 MFA 服务",
			"user_id", user.ID, "email", user.Email)
		// 不暴露内部错误细节，返回 MFA 服务不可用
		return nil, apperrors.ErrMFAServiceUnavailable
	}
	if s.cache == nil {
		logging.WithContext(ctx).Error("AuthService 未装配 cache，无法存储 MFA Challenge",
			"user_id", user.ID)
		return nil, apperrors.ErrMFAServiceUnavailable
	}

	// 生成 Challenge Token（32 字节 CSPRNG）
	tokenBytes := make([]byte, 32)
	if _, err := rand.Read(tokenBytes); err != nil {
		return nil, serviceutil.WrapServiceError("生成 MFA Challenge", err)
	}
	challengeToken := hex.EncodeToString(tokenBytes)

	// 计算 TTL（若未设置则使用默认 5 分钟）
	ttl := s.mfaChallengeTTL
	if ttl <= 0 {
		ttl = 5 * time.Minute
	}

	now := time.Now()
	ipAddress := ""
	userAgent := ""
	if auditCtx != nil {
		ipAddress = auditCtx.IPAddress
		userAgent = auditCtx.UserAgent
	}

	challenge := &model.MFAChallenge{
		UserID:    user.ID,
		Email:     user.Email,
		Role:      user.Role,
		Scopes:    []string{"openid", "profile", "email"},
		ClientID:  nil,
		IPAddress: ipAddress,
		UserAgent: userAgent,
		Attempts:  0,
		CreatedAt: now,
		ExpiresAt: now.Add(ttl),
	}

	// 存储 Challenge，TTL 自动过期
	key := cache.MFAChallengeKey(challengeToken)
	if err := s.cache.Set(ctx, key, challenge, ttl); err != nil {
		return nil, serviceutil.WrapServiceError("存储 MFA Challenge", err)
	}

	// 记录审计日志（第一阶段密码验证成功，等待 MFA 二次验证）
	auditutil.SafeAuditLog(ctx, s.auditSvc, "mfa_challenge_issued", user.ID, map[string]interface{}{
		"ip_address":  ipAddress,
		"user_agent":  userAgent,
		"expires_in":  int(ttl.Seconds()),
	})

	// 返回 Challenge 响应（不包含 access_token / refresh_token）
	return &model.LoginResponse{
		MFARequired:  true,
		MFAChallenge: challengeToken,
		ExpiresIn:    int(ttl.Seconds()),
		MFAMethods:   []string{model.MFAMethodTOTP, model.MFAMethodRecoveryCode},
	}, nil
}

// VerifyMFALogin 验证 MFA 登录（两阶段登录的第二阶段）
//
// 流程：
//  1. 取出 Challenge（cache miss 返回 ErrMFAChallengeInvalid）
//  2. 检查过期、IP/UA 绑定、尝试次数
//  3. 调用 mfaSvc.VerifyMFALoginCode 验证 code
//  4. 验证成功：删除 Challenge（一次性）+ 签发 Token
//  5. 验证失败：递增尝试次数；超限则删除 Challenge
//
// 安全设计：
//   - 上下文不匹配（IP/UA）立即删除 Challenge，视为 token 被盗用
//   - 尝试次数超限立即删除 Challenge，强制重新走第一阶段
//   - 验证成功后重新查询用户状态，防止 challenge 期间账户被禁用
//   - Challenge 验证成功后立即删除（一次性使用），即使后续 Token 签发失败也保留删除状态
func (s *AuthService) VerifyMFALogin(ctx context.Context, req *model.MFAVerifyRequest, ipAddress, userAgent string) (*model.LoginResponse, error) {
	// 基础校验
	if req.MFAChallenge == "" {
		return nil, apperrors.ErrMFAChallengeInvalid
	}
	if req.Code == "" {
		return nil, apperrors.ErrBadRequest.WithDetails("code is required")
	}
	if req.Method != model.MFAMethodTOTP && req.Method != model.MFAMethodRecoveryCode {
		return nil, apperrors.ErrBadRequest.WithDetails("invalid method: " + req.Method)
	}

	if s.cache == nil || s.mfaSvc == nil {
		return nil, apperrors.ErrMFAServiceUnavailable
	}

	key := cache.MFAChallengeKey(req.MFAChallenge)

	// 1. 取出 Challenge
	var challenge model.MFAChallenge
	if err := s.cache.Get(ctx, key, &challenge); err != nil {
		if errors.Is(err, cache.ErrCacheMiss) {
			// Challenge 不存在或已被消费
			return nil, apperrors.ErrMFAChallengeInvalid
		}
		return nil, serviceutil.WrapServiceError("读取 MFA Challenge", err)
	}

	// 2. 检查过期
	if time.Now().After(challenge.ExpiresAt) {
		_ = s.cache.Delete(ctx, key)
		return nil, apperrors.ErrMFAChallengeExpired
	}

	// 3. 检查 IP/UA 绑定
	// 上下文不匹配视为 token 被盗用，立即失效
	if challenge.IPAddress != ipAddress || challenge.UserAgent != userAgent {
		_ = s.cache.Delete(ctx, key)
		auditutil.SafeAuditLog(ctx, s.auditSvc, "mfa_challenge_context_mismatch", challenge.UserID, map[string]interface{}{
			"expected_ip":  challenge.IPAddress,
			"actual_ip":    ipAddress,
			"user_agent":   userAgent,
		})
		return nil, apperrors.ErrMFAChallengeInvalid
	}

	// 4. 检查尝试次数
	if challenge.Attempts >= model.MaxMFALoginAttempts {
		_ = s.cache.Delete(ctx, key)
		auditutil.SafeAuditLog(ctx, s.auditSvc, "mfa_challenge_attempts_exceeded", challenge.UserID, map[string]interface{}{
			"ip_address": ipAddress,
		})
		return nil, apperrors.ErrTooManyMFAAttempts
	}

	// 5. 调用 MFA 服务验证 code
	if err := s.mfaSvc.VerifyMFALoginCode(ctx, challenge.UserID, req.Method, req.Code, ipAddress); err != nil {
		// 验证失败：递增尝试次数后回写
		challenge.Attempts++
		remainingTTL := time.Until(challenge.ExpiresAt)
		if remainingTTL > 0 {
			_ = s.cache.Set(ctx, key, &challenge, remainingTTL)
		}
		// 透传验证错误（ErrInvalidMFACode / ErrTooManyRecoveryAttempts 等）
		return nil, err
	}

	// 6. 验证成功，立即删除 Challenge（一次性使用）
	// 即使后续 Token 签发失败也保留删除状态，防止重放
	_ = s.cache.Delete(ctx, key)

	// 7. 重新查询用户，再次校验状态
	// 防止 challenge 期间账户被禁用、锁定、或 MFA 被关闭
	user, err := s.store.GetByID(ctx, challenge.UserID)
	if err != nil {
		return nil, serviceutil.HandleStoreError(err, apperrors.ErrMFAChallengeInvalid)
	}
	if user.Status == model.UserStatusDisabled {
		return nil, apperrors.ErrAccountDisabled
	}
	if user.Status == model.UserStatusLocked {
		return nil, apperrors.ErrAccountLocked
	}

	// 8. 签发 Token
	resp, err := s.generateTokenPair(ctx, user.ID, user.Email, user.Role, challenge.Scopes, challenge.ClientID)
	if err != nil {
		return nil, err
	}

	// 9. 记录 MFA 登录成功审计日志
	auditutil.SafeAuditLog(ctx, s.auditSvc, "mfa_login_success", user.ID, map[string]interface{}{
		"email":      user.Email,
		"ip_address": ipAddress,
		"user_agent": userAgent,
		"method":     req.Method,
	})

	s.incrementMetric("auth_mfa_login_total")

	return resp, nil
}
