// Package service_test 服务构造函数测试
package service_test

import (
	"crypto/rand"
	"crypto/rsa"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/your-org/sso/internal/cache"
	"github.com/your-org/sso/internal/crypto"
	"github.com/your-org/sso/internal/metrics"
	"github.com/your-org/sso/internal/service"
	"github.com/your-org/sso/internal/store/mock"
)

// ============================================================================
// AuthService 构造函数测试
// ============================================================================

func TestNewAuthServiceWithAudit(t *testing.T) {
	store := mock.New()
	passwordSvc := crypto.NewPasswordService(10)

	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)

	jwtSvc := crypto.NewJWTService(
		privateKey,
		&privateKey.PublicKey,
		"test-issuer",
		15*time.Minute,
		7*24*time.Hour,
	)

	auditSvc := service.NewAuditService(store)

	authSvc := service.NewAuthServiceWithOptions(
		store,
		passwordSvc,
		jwtSvc,
		5,
		30*time.Minute,
		service.WithAudit(auditSvc),
	)

	assert.NotNil(t, authSvc)
}

// ============================================================================
// AdminService 构造函数测试
// ============================================================================

func TestNewAdminServiceWithCache(t *testing.T) {
	store := mock.New()
	memCache := cache.NewMemoryCache()
	defer memCache.Close()

	adminSvc := service.NewAdminServiceWithCache(store, memCache)

	assert.NotNil(t, adminSvc)
}

// ============================================================================
// MFAService 构造函数测试
// ============================================================================

func TestNewMFAServiceWithAudit(t *testing.T) {
	store := mock.New()
	auditSvc := service.NewAuditService(store)

	mfaSvc := service.NewMFAServiceWithAudit(store, auditSvc)

	assert.NotNil(t, mfaSvc)
}

// ============================================================================
// UserService 构造函数测试
// ============================================================================

func TestNewUserServiceWithAudit(t *testing.T) {
	store := mock.New()
	passwordSvc := crypto.NewPasswordService(10)
	emailSvc := service.NewEmailService(nil)
	auditSvc := service.NewAuditService(store)

	userSvc := service.NewUserServiceWithAudit(
		store,
		passwordSvc,
		emailSvc,
		"http://localhost:9090",
		auditSvc,
	)

	assert.NotNil(t, userSvc)
}

// ============================================================================
// OAuthService 构造函数测试
// ============================================================================

func TestNewOAuthServiceWithCache(t *testing.T) {
	store := mock.New()
	memCache := cache.NewMemoryCache()
	defer memCache.Close()

	tokenSvc := service.NewTokenService(
		crypto.NewJWTService(func() *rsa.PrivateKey {
			key, _ := rsa.GenerateKey(rand.Reader, 2048)
			return key
		}(), nil, "test", 15*time.Minute, 7*24*time.Hour),
		store,
	)

	oauthSvc := service.NewOAuthServiceWithCache(store, memCache, tokenSvc)

	assert.NotNil(t, oauthSvc)
}

func TestNewOAuthServiceWithAudit(t *testing.T) {
	store := mock.New()
	auditSvc := service.NewAuditService(store)

	tokenSvc := service.NewTokenService(
		crypto.NewJWTService(func() *rsa.PrivateKey {
			key, _ := rsa.GenerateKey(rand.Reader, 2048)
			return key
		}(), nil, "test", 15*time.Minute, 7*24*time.Hour),
		store,
	)

	oauthSvc := service.NewOAuthServiceWithAudit(store, auditSvc, tokenSvc)

	assert.NotNil(t, oauthSvc)
}

// ============================================================================
// WithMetrics 选项测试
// ============================================================================

func TestNewAuthServiceWithMetrics(t *testing.T) {
	store := mock.New()
	passwordSvc := crypto.NewPasswordService(10)

	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)

	jwtSvc := crypto.NewJWTService(
		privateKey,
		&privateKey.PublicKey,
		"test-issuer",
		15*time.Minute,
		7*24*time.Hour,
	)

	metricsSvc := metrics.NewService()

	authSvc := service.NewAuthServiceWithOptions(
		store,
		passwordSvc,
		jwtSvc,
		5,
		30*time.Minute,
		service.WithMetrics(metricsSvc),
	)

	assert.NotNil(t, authSvc)
}

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
// NewAdminServiceWithVersion 测试
// ============================================================================

func TestNewAdminServiceWithVersion(t *testing.T) {
	storeInst := mock.New()
	memCache := cache.NewMemoryCache()
	defer memCache.Close()

	adminSvc := service.NewAdminServiceWithVersion(storeInst, memCache, "1.2.3")
	assert.NotNil(t, adminSvc)
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

	socialSvc := service.NewSocialLoginService(mock.New(), jwtSvc, "", "", "", "")

	// Close不应该panic
	socialSvc.Close()
}
