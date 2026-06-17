// Package service_test 服务构造函数测试
package service_test

import (
	"crypto/rand"
	"crypto/rsa"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/your-org/sso/internal/crypto"
	"github.com/your-org/sso/internal/service"
	"github.com/your-org/sso/internal/store/mock"
)

// ============================================================================
// LogSystemStart 测试
// ============================================================================

func TestAuditService_LogSystemStart(t *testing.T) {
	store := mock.New()
	auditSvc := service.NewAuditService(store)

	// LogSystemStart 不应该panic
	auditSvc.LogSystemStart(t.Context(), "1.0.0")
}

// ============================================================================
// OAuthService GetAccessTokenTTL 测试
// ============================================================================

func TestOAuthService_GetAccessTokenTTL(t *testing.T) {
	storeInst := mock.New()
	tokenSvc := service.NewTokenService(
		crypto.NewJWTService(func() *rsa.PrivateKey {
			key, _ := rsa.GenerateKey(rand.Reader, 2048)
			return key
		}(), nil, "test", 15*time.Minute, 7*24*time.Hour),
		storeInst,
	)

	oauthSvc := service.NewOAuthService(storeInst, tokenSvc)
	ttl := oauthSvc.GetAccessTokenTTL()

	assert.Equal(t, 15*time.Minute, ttl)
}

// ============================================================================
// SocialLoginService Close 测试
// ============================================================================

func TestSocialLoginService_Close(t *testing.T) {
	privateKey, _ := rsa.GenerateKey(rand.Reader, 2048)
	jwtSvc := crypto.NewJWTService(privateKey, &privateKey.PublicKey, "test", 15*time.Minute, 7*24*time.Hour)

	socialSvc := service.NewSocialLoginService(mock.New(), jwtSvc, "http://localhost:9000", "", "", "", "")

	// Close不应该panic
	socialSvc.Close()
}
