// Package service Token生成服务
// 提供统一的Token生成功能，消除代码重复
package service

import (
	"context"
	"log/slog"
	"time"

	"github.com/google/uuid"

	"github.com/example/sso/internal/crypto"
	"github.com/example/sso/internal/logging"
	"github.com/example/sso/internal/model"
	"github.com/example/sso/internal/store"
	"github.com/example/sso/internal/util/serviceutil"
)

// ============================================================================
// TokenService Token生成服务
// ============================================================================

// TokenService Token生成服务
// 提供统一的Token对生成功能，供AuthService和SocialLoginService使用
type TokenService struct {
	jwtSvc *crypto.JWTService
	store  store.Store
}

// NewTokenService 创建Token生成服务
func NewTokenService(jwtSvc *crypto.JWTService, store store.Store) *TokenService {
	return &TokenService{
		jwtSvc: jwtSvc,
		store:  store,
	}
}

// GenerateTokenPair 生成Token对
// 生成access_token和refresh_token，存储到数据库并返回响应
//
// RefreshExpiresAt 字段使用 jwtSvc.GetRefreshTokenTTL() 计算，
// 与 access token 的 ExpiresAt 解耦，支持 refresh token 独立过期检查
func (s *TokenService) GenerateTokenPair(
	ctx context.Context,
	userID, email, role string,
	scopes []string,
	clientID *string,
) (*model.LoginResponse, error) {
	tokenRecord, resp, err := s.GenerateTokenRecord(ctx, userID, email, role, scopes, clientID)
	if err != nil {
		return nil, err
	}

	slog.Debug("GenerateTokenPair: 准备存储token", "userID", userID, "tokenID", tokenRecord.ID)
	if err := s.store.StoreToken(ctx, tokenRecord); err != nil {
		// 阶段 D 审查修复（H5）：store 错误可能含 DSN（pgx 错误消息含完整连接串）
		// 使用 SanitizeDBURL 脱敏 password，非 DSN 字符串原样返回不破坏错误上下文
		slog.Error("GenerateTokenPair: 存储token失败", "error", logging.SanitizeDBURL(err.Error()), "userID", userID)
		return nil, serviceutil.WrapServiceError("存储token", err)
	}
	slog.Debug("GenerateTokenPair: token存储成功", "userID", userID)

	return resp, nil
}

// GenerateTokenRecord 生成 Token 记录与响应，但不写入存储
//
// 供调用方在事务内自行决定如何持久化（如 RefreshToken 流程使用 RotateRefreshToken
// 在同一事务内完成"标记旧 token + 插入新 token"，避免 TOCTOU 竞态）。
//
// 返回值：
//   - *model.Token: 已填充字段的新 token 记录（包含 RefreshExpiresAt）
//   - *model.LoginResponse: 与 token 对应的响应
//   - error: 生成失败时返回
func (s *TokenService) GenerateTokenRecord(
	ctx context.Context,
	userID, email, role string,
	scopes []string,
	clientID *string,
) (*model.Token, *model.LoginResponse, error) {
	slog.Debug("GenerateTokenRecord: 生成access token", "userID", userID)
	accessToken, err := s.jwtSvc.GenerateAccessToken(userID, email, role, scopes)
	if err != nil {
		slog.Error("GenerateTokenRecord: 生成access token失败", "error", err, "userID", userID)
		return nil, nil, serviceutil.WrapServiceError("生成access token", err)
	}
	slog.Debug("GenerateTokenRecord: access token生成成功", "length", len(accessToken))

	slog.Debug("GenerateTokenRecord: 生成refresh token", "userID", userID)
	refreshToken, err := s.jwtSvc.GenerateRefreshToken()
	if err != nil {
		slog.Error("GenerateTokenRecord: 生成refresh token失败", "error", err, "userID", userID)
		return nil, nil, serviceutil.WrapServiceError("生成refresh token", err)
	}
	slog.Debug("GenerateTokenRecord: refresh token生成成功", "length", len(refreshToken))

	now := time.Now()
	refreshExpiresAt := now.Add(s.jwtSvc.GetRefreshTokenTTL())
	tokenRecord := &model.Token{
		ID:               uuid.New().String(),
		AccessToken:      accessToken,
		RefreshToken:     refreshToken,
		UserID:           userID,
		ClientID:         clientID,
		Scopes:           scopes,
		ExpiresAt:        now.Add(s.jwtSvc.GetAccessTokenTTL()),
		CreatedAt:        now,
		RefreshExpiresAt: &refreshExpiresAt,
	}

	resp := &model.LoginResponse{
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
		TokenType:    "Bearer",
		ExpiresIn:    int(s.jwtSvc.GetAccessTokenTTL().Seconds()),
	}

	return tokenRecord, resp, nil
}
