// Package service Token生成服务
// 提供统一的Token生成功能，消除代码重复
package service

import (
	"context"
	"time"

	"github.com/google/uuid"

	"github.com/your-org/sso/internal/crypto"
	"github.com/your-org/sso/internal/model"
	"github.com/your-org/sso/internal/store"
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
	userID, email string,
	scopes []string,
	clientID string,
) (*model.LoginResponse, error) {
	// 生成access token
	accessToken, err := s.jwtSvc.GenerateAccessToken(userID, email, scopes)
	if err != nil {
		return nil, err
	}

	// 生成refresh token
	refreshToken, err := s.jwtSvc.GenerateRefreshToken()
	if err != nil {
		return nil, err
	}

	// 创建token记录
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

	// 存储token到数据库
	if err := s.store.StoreToken(ctx, tokenRecord); err != nil {
		return nil, err
	}

	// 返回登录响应
	return &model.LoginResponse{
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
		TokenType:    "Bearer",
		ExpiresIn:    int(s.jwtSvc.GetAccessTokenTTL().Seconds()),
	}, nil
}
