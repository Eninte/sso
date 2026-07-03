// Package service Token生成服务
// 提供统一的Token生成功能，消除代码重复
package service

import (
	"context"
	"log/slog"
	"time"

	"github.com/google/uuid"

	"github.com/example/sso/internal/crypto"
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
func (s *TokenService) GenerateTokenPair(
	ctx context.Context,
	userID, email, role string,
	scopes []string,
	clientID *string,
) (*model.LoginResponse, error) {
	slog.Debug("GenerateTokenPair: 生成access token", "userID", userID)
	accessToken, err := s.jwtSvc.GenerateAccessToken(userID, email, role, scopes)
	if err != nil {
		slog.Error("GenerateTokenPair: 生成access token失败", "error", err, "userID", userID)
		return nil, serviceutil.WrapServiceError("生成access token", err)
	}
	slog.Debug("GenerateTokenPair: access token生成成功", "length", len(accessToken))

	slog.Debug("GenerateTokenPair: 生成refresh token", "userID", userID)
	refreshToken, err := s.jwtSvc.GenerateRefreshToken()
	if err != nil {
		slog.Error("GenerateTokenPair: 生成refresh token失败", "error", err, "userID", userID)
		return nil, serviceutil.WrapServiceError("生成refresh token", err)
	}
	slog.Debug("GenerateTokenPair: refresh token生成成功", "length", len(refreshToken))

	tokenRecord := &model.Token{
		ID:           uuid.New().String(),
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
		UserID:       userID,
		ClientID:     clientID,
		Scopes:       scopes,
		ExpiresAt:    time.Now().Add(s.jwtSvc.GetAccessTokenTTL()),
		CreatedAt:    time.Now(),
	}

	slog.Debug("GenerateTokenPair: 准备存储token", "userID", userID, "tokenID", tokenRecord.ID)
	if err := s.store.StoreToken(ctx, tokenRecord); err != nil {
		slog.Error("GenerateTokenPair: 存储token失败", "error", err, "userID", userID)
		return nil, serviceutil.WrapServiceError("存储token", err)
	}
	slog.Debug("GenerateTokenPair: token存储成功", "userID", userID)

	return &model.LoginResponse{
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
		TokenType:    "Bearer",
		ExpiresIn:    int(s.jwtSvc.GetAccessTokenTTL().Seconds()),
	}, nil
}
