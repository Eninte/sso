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

	authSvc := service.NewAuthServiceWithAudit(
		store,
		passwordSvc,
		jwtSvc,
		5,
		30*time.Minute,
		auditSvc,
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

	oauthSvc := service.NewOAuthServiceWithCache(store, memCache)

	assert.NotNil(t, oauthSvc)
}

func TestNewOAuthServiceWithAudit(t *testing.T) {
	store := mock.New()
	auditSvc := service.NewAuditService(store)

	oauthSvc := service.NewOAuthServiceWithAudit(store, auditSvc)

	assert.NotNil(t, oauthSvc)
}
