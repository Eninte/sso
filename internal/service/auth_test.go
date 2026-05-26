// Package service_test 认证服务单元测试
package service_test

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/your-org/sso/internal/cache"
	"github.com/your-org/sso/internal/crypto"
	"github.com/your-org/sso/internal/metrics"
	"github.com/your-org/sso/internal/model"
	"github.com/your-org/sso/internal/service"
	"github.com/your-org/sso/internal/store/mock"

	store2 "github.com/your-org/sso/internal/store"
)

// ============================================================================
// 测试辅助函数
// ============================================================================

// createTestAuthService 创建测试用的认证服务
func createTestAuthService(t *testing.T) (*service.AuthService, *mock.Store) {
	// 创建Mock存储
	store := mock.New()

	// 创建密码服务
	passwordSvc := crypto.NewPasswordService(4) // 使用较低的cost加快测试

	// 创建JWT服务
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)

	jwtSvc := crypto.NewJWTService(
		privateKey,
		&privateKey.PublicKey,
		"test-issuer",
		15*time.Minute,
		7*24*time.Hour,
	)

	// 创建认证服务
	authSvc := service.NewAuthService(store, passwordSvc, jwtSvc, 5, 30*time.Minute)

	return authSvc, store
}

// ============================================================================
// Register 测试
// ============================================================================

func TestAuthService_Register(t *testing.T) {
	authSvc, store := createTestAuthService(t)
	ctx := context.Background()

	t.Run("成功注册", func(t *testing.T) {
		store.Reset()

		req := &model.RegisterRequest{
			Email:    "test@example.com",
			Password: "Password123!!",
		}

		user, err := authSvc.Register(ctx, req)

		require.NoError(t, err)
		assert.NotEmpty(t, user.ID)
		assert.Equal(t, "test@example.com", user.Email)
		assert.NotEmpty(t, user.PasswordHash)
		assert.Equal(t, model.UserStatusActive, user.Status)
		assert.False(t, user.EmailVerified)
	})

	t.Run("邮箱已存在", func(t *testing.T) {
		store.Reset()

		// 先注册一个用户
		req := &model.RegisterRequest{
			Email:    "existing@example.com",
			Password: "Password123!!",
		}
		_, err := authSvc.Register(ctx, req)
		require.NoError(t, err)

		// 尝试用相同邮箱注册
		_, err = authSvc.Register(ctx, req)

		assert.Error(t, err)
		assert.ErrorIs(t, err, store2.ErrDuplicateEmail)
	})

	t.Run("邮箱格式无效", func(t *testing.T) {
		store.Reset()

		req := &model.RegisterRequest{
			Email:    "invalid-email",
			Password: "Password123!!",
		}

		_, err := authSvc.Register(ctx, req)

		assert.Error(t, err)
	})

	t.Run("密码太短", func(t *testing.T) {
		store.Reset()

		req := &model.RegisterRequest{
			Email:    "test2@example.com",
			Password: "short",
		}

		_, err := authSvc.Register(ctx, req)

		assert.Error(t, err)
	})
}

// ============================================================================
// Login 测试
// ============================================================================

func TestAuthService_Login(t *testing.T) {
	authSvc, store := createTestAuthService(t)
	ctx := context.Background()

	// 预先创建一个用户
	store.Reset()
	hashedPassword, err := crypto.NewPasswordService(4).HashPassword("Password123!")
	require.NoError(t, err)

	testUser := &model.User{
		ID:            "test-user-id",
		Email:         "test@example.com",
		PasswordHash:  hashedPassword,
		Status:        model.UserStatusActive,
		EmailVerified: true,
		CreatedAt:     time.Now(),
		UpdatedAt:     time.Now(),
	}
	store.AddUser(testUser)

	t.Run("成功登录", func(t *testing.T) {
		req := &model.LoginRequest{
			Email:    "test@example.com",
			Password: "Password123!",
		}

		resp, err := authSvc.Login(ctx, req)

		require.NoError(t, err)
		assert.NotEmpty(t, resp.AccessToken)
		assert.NotEmpty(t, resp.RefreshToken)
		assert.Equal(t, "Bearer", resp.TokenType)
		assert.Greater(t, resp.ExpiresIn, 0)
	})

	t.Run("密码错误", func(t *testing.T) {
		req := &model.LoginRequest{
			Email:    "test@example.com",
			Password: "WrongPassword",
		}

		_, err := authSvc.Login(ctx, req)

		assert.Error(t, err)
		assert.ErrorIs(t, err, service.ErrInvalidCredentials)
	})

	t.Run("用户不存在", func(t *testing.T) {
		req := &model.LoginRequest{
			Email:    "nonexistent@example.com",
			Password: "Password123!",
		}

		_, err := authSvc.Login(ctx, req)

		assert.Error(t, err)
		assert.ErrorIs(t, err, service.ErrInvalidCredentials)
	})

	t.Run("账户被禁用", func(t *testing.T) {
		disabledUser := &model.User{
			ID:            "disabled-user-id",
			Email:         "disabled@example.com",
			PasswordHash:  hashedPassword,
			Status:        model.UserStatusDisabled,
			EmailVerified: true,
			CreatedAt:     time.Now(),
			UpdatedAt:     time.Now(),
		}
		store.AddUser(disabledUser)

		req := &model.LoginRequest{
			Email:    "disabled@example.com",
			Password: "Password123!",
		}

		_, err := authSvc.Login(ctx, req)

		assert.Error(t, err)
		assert.ErrorIs(t, err, service.ErrAccountDisabled)
	})

	t.Run("账户被锁定", func(t *testing.T) {
		lockedUntil := time.Now().Add(30 * time.Minute)
		lockedUser := &model.User{
			ID:            "locked-user-id",
			Email:         "locked@example.com",
			PasswordHash:  hashedPassword,
			Status:        model.UserStatusLocked,
			LockedUntil:   &lockedUntil,
			EmailVerified: true,
			CreatedAt:     time.Now(),
			UpdatedAt:     time.Now(),
		}
		store.AddUser(lockedUser)

		req := &model.LoginRequest{
			Email:    "locked@example.com",
			Password: "Password123!",
		}

		_, err := authSvc.Login(ctx, req)

		assert.Error(t, err)
		assert.ErrorIs(t, err, service.ErrAccountLocked)
	})

	t.Run("账户锁定已过期-自动解锁", func(t *testing.T) {
		lockedUntil := time.Now().Add(-10 * time.Minute)
		unlockedUser := &model.User{
			ID:            "unlocked-user-id",
			Email:         "unlocked@example.com",
			PasswordHash:  hashedPassword,
			Status:        model.UserStatusLocked,
			LockedUntil:   &lockedUntil,
			EmailVerified: true,
			CreatedAt:     time.Now(),
			UpdatedAt:     time.Now(),
		}
		store.AddUser(unlockedUser)

		req := &model.LoginRequest{
			Email:    "unlocked@example.com",
			Password: "Password123!",
		}

		resp, err := authSvc.Login(ctx, req)

		require.NoError(t, err)
		assert.NotEmpty(t, resp.AccessToken)
	})

	t.Run("连续失败触发账户锁定", func(t *testing.T) {
		failUser := &model.User{
			ID:            "fail-user-id",
			Email:         "fail@example.com",
			PasswordHash:  hashedPassword,
			Status:        model.UserStatusActive,
			LoginAttempts: 4,
			EmailVerified: true,
			CreatedAt:     time.Now(),
			UpdatedAt:     time.Now(),
		}
		store.AddUser(failUser)

		req := &model.LoginRequest{
			Email:    "fail@example.com",
			Password: "WrongPassword!",
		}

		_, err := authSvc.Login(ctx, req)

		assert.Error(t, err)
		assert.ErrorIs(t, err, service.ErrInvalidCredentials)

		// 验证账户已锁定
		updatedUser, _ := store.GetByEmail(ctx, "fail@example.com")
		assert.Equal(t, model.UserStatusLocked, updatedUser.Status)
	})
}

// ============================================================================
// RefreshToken 测试
// ============================================================================

func TestAuthService_RefreshToken(t *testing.T) {
	authSvc, store := createTestAuthService(t)
	ctx := context.Background()

	// 先登录获取Token
	store.Reset()
	hashedPassword, _ := crypto.NewPasswordService(4).HashPassword("Password123!")
	testUser := &model.User{
		ID:            "test-user-id",
		Email:         "test@example.com",
		PasswordHash:  hashedPassword,
		Status:        model.UserStatusActive,
		EmailVerified: true,
		CreatedAt:     time.Now(),
		UpdatedAt:     time.Now(),
	}
	store.AddUser(testUser)

	loginResp, err := authSvc.Login(ctx, &model.LoginRequest{
		Email:    "test@example.com",
		Password: "Password123!",
	})
	require.NoError(t, err)

	t.Run("成功刷新Token", func(t *testing.T) {
		// 等待一小段时间确保Token不同
		time.Sleep(10 * time.Millisecond)

		resp, err := authSvc.RefreshToken(ctx, loginResp.RefreshToken)

		require.NoError(t, err)
		assert.NotEmpty(t, resp.AccessToken)
		assert.NotEmpty(t, resp.RefreshToken)
		// Refresh Token应该不同（轮换机制）
		assert.NotEqual(t, loginResp.RefreshToken, resp.RefreshToken)
	})

	t.Run("无效的Refresh Token", func(t *testing.T) {
		_, err := authSvc.RefreshToken(ctx, "invalid-refresh-token")

		assert.Error(t, err)
		assert.ErrorIs(t, err, service.ErrInvalidToken)
	})
}

// ============================================================================
// Logout 测试
// ============================================================================

func TestAuthService_Logout(t *testing.T) {
	authSvc, store := createTestAuthService(t)
	ctx := context.Background()

	store.Reset()
	hashedPassword, _ := crypto.NewPasswordService(4).HashPassword("Password123!")
	testUser := &model.User{
		ID:            "test-user-id",
		Email:         "test@example.com",
		PasswordHash:  hashedPassword,
		Status:        model.UserStatusActive,
		EmailVerified: true,
		CreatedAt:     time.Now(),
		UpdatedAt:     time.Now(),
	}
	store.AddUser(testUser)

	loginResp, err := authSvc.Login(ctx, &model.LoginRequest{
		Email:    "test@example.com",
		Password: "Password123!",
	})
	require.NoError(t, err)

	t.Run("成功登出", func(t *testing.T) {
		err := authSvc.Logout(ctx, loginResp.AccessToken)

		assert.NoError(t, err)

		// 验证Token已被撤销
		_, err = authSvc.ValidateToken(ctx, loginResp.AccessToken)
		assert.Error(t, err)
	})
}

// ============================================================================
// ValidateToken 测试
// ============================================================================

func TestAuthService_ValidateToken(t *testing.T) {
	authSvc, store := createTestAuthService(t)
	ctx := context.Background()

	store.Reset()
	hashedPassword, _ := crypto.NewPasswordService(4).HashPassword("Password123!")
	testUser := &model.User{
		ID:            "test-user-id",
		Email:         "test@example.com",
		PasswordHash:  hashedPassword,
		Status:        model.UserStatusActive,
		EmailVerified: true,
		CreatedAt:     time.Now(),
		UpdatedAt:     time.Now(),
	}
	store.AddUser(testUser)

	loginResp, err := authSvc.Login(ctx, &model.LoginRequest{
		Email:    "test@example.com",
		Password: "Password123!",
	})
	require.NoError(t, err)

	t.Run("验证有效Token", func(t *testing.T) {
		claims, err := authSvc.ValidateToken(ctx, loginResp.AccessToken)

		require.NoError(t, err)
		assert.Equal(t, "test-user-id", claims.Subject)
		assert.Equal(t, "test@example.com", claims.Email)
	})

	t.Run("验证无效Token", func(t *testing.T) {
		_, err := authSvc.ValidateToken(ctx, "invalid-token")

		assert.Error(t, err)
	})

	t.Run("验证已撤销Token", func(t *testing.T) {
		// 先登出
		err := authSvc.Logout(ctx, loginResp.AccessToken)
		require.NoError(t, err)

		// 再验证
		_, err = authSvc.ValidateToken(ctx, loginResp.AccessToken)

		assert.Error(t, err)
		assert.ErrorIs(t, err, service.ErrInvalidToken)
	})
}

// ============================================================================
// LogoutAll 测试
// ============================================================================

func TestAuthService_LogoutAll(t *testing.T) {
	authSvc, store := createTestAuthService(t)
	ctx := context.Background()

	t.Run("成功登出所有设备", func(t *testing.T) {
		store.Reset()

		hashedPassword, _ := crypto.NewPasswordService(4).HashPassword("Password123!")
		testUser := &model.User{
			ID:            "test-user-logoutall",
			Email:         "logoutall@example.com",
			PasswordHash:  hashedPassword,
			Status:        model.UserStatusActive,
			EmailVerified: true,
			CreatedAt:     time.Now(),
			UpdatedAt:     time.Now(),
		}
		store.AddUser(testUser)

		var tokens []string
		for i := 0; i < 3; i++ {
			resp, err := authSvc.Login(ctx, &model.LoginRequest{
				Email:    "logoutall@example.com",
				Password: "Password123!",
			})
			require.NoError(t, err)
			tokens = append(tokens, resp.AccessToken)
		}

		// 登出所有设备
		err := authSvc.LogoutAll(ctx, testUser.ID)
		require.NoError(t, err)

		// 验证所有Token都已失效
		for _, token := range tokens {
			_, err := authSvc.ValidateToken(ctx, token)
			assert.Error(t, err, "Token应该已失效: %s", token)
		}
	})
}

// ============================================================================
// NewAuthServiceWithAudit 测试
// ============================================================================

func TestAuthService_NewAuthServiceWithAudit(t *testing.T) {
	store := mock.New()
	passwordSvc := crypto.NewPasswordService(4)
	privateKey, _ := rsa.GenerateKey(rand.Reader, 2048)
	jwtSvc := crypto.NewJWTService(
		privateKey,
		&privateKey.PublicKey,
		"test-issuer",
		15*time.Minute,
		7*24*time.Hour,
	)

	t.Run("创建带审计的认证服务", func(t *testing.T) {
		// 创建审计服务
		auditSvc := service.NewAuditService(store)
		defer auditSvc.Close()

		authSvc := service.NewAuthServiceWithOptions(store, passwordSvc, jwtSvc, 5, 30*time.Minute, service.WithAudit(auditSvc))
		assert.NotNil(t, authSvc)
	})
}

// ============================================================================
// LoginWithAudit 测试
// ============================================================================

func TestAuthService_LoginWithAudit(t *testing.T) {
	authSvc, store := createTestAuthService(t)
	ctx := context.Background()

	t.Run("带审计上下文的登录", func(t *testing.T) {
		store.Reset()

		// 创建测试用户
		hashedPassword, _ := crypto.NewPasswordService(4).HashPassword("Password123!")
		testUser := &model.User{
			ID:            "test-user-audit-login",
			Email:         "auditlogin@example.com",
			PasswordHash:  hashedPassword,
			Status:        model.UserStatusActive,
			EmailVerified: true,
			CreatedAt:     time.Now(),
			UpdatedAt:     time.Now(),
		}
		store.AddUser(testUser)

		// 带审计上下文登录
		auditCtx := &service.AuditContext{
			IPAddress: "192.168.1.1",
			UserAgent: "Mozilla/5.0",
		}
		loginResp, err := authSvc.LoginWithAudit(ctx, &model.LoginRequest{
			Email:    "auditlogin@example.com",
			Password: "Password123!",
		}, auditCtx)

		require.NoError(t, err)
		assert.NotEmpty(t, loginResp.AccessToken)
	})
}

// ============================================================================
// NewAuthServiceWithCache 测试
// ============================================================================

func TestNewAuthServiceWithCache(t *testing.T) {
	store := mock.New()
	passwordSvc := crypto.NewPasswordService(4)

	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)

	jwtSvc := crypto.NewJWTService(
		privateKey,
		&privateKey.PublicKey,
		"test-issuer",
		15*time.Minute,
		7*24*time.Hour,
	)

	// 创建内存缓存
	memCache := cache.NewMemoryCache()
	defer memCache.Close()

	// 创建带缓存的AuthService
	authSvc := service.NewAuthServiceWithOptions(
		store,
		passwordSvc,
		jwtSvc,
		5,
		30*time.Minute,
		service.WithCache(memCache),
	)

	assert.NotNil(t, authSvc)

	// 验证可以正常使用
	ctx := context.Background()
	user, err := authSvc.Register(ctx, &model.RegisterRequest{
		Email:    "cache-test@example.com",
		Password: "Password123!!",
	})
	require.NoError(t, err)
	assert.Equal(t, "cache-test@example.com", user.Email)
}

// ============================================================================
// LogoutWithAudit 测试
// ============================================================================

func TestAuthService_LogoutWithAudit(t *testing.T) {
	authSvc, store := createTestAuthService(t)
	ctx := context.Background()

	t.Run("带审计上下文的登出", func(t *testing.T) {
		store.Reset()

		hashedPassword, _ := crypto.NewPasswordService(4).HashPassword("Password123!")
		testUser := &model.User{
			ID:            "test-user-logout",
			Email:         "logout@example.com",
			PasswordHash:  hashedPassword,
			Role:          model.UserRoleUser,
			Status:        model.UserStatusActive,
			EmailVerified: true,
			CreatedAt:     time.Now(),
			UpdatedAt:     time.Now(),
		}
		store.AddUser(testUser)

		// 登录获取token
		loginResp, err := authSvc.Login(ctx, &model.LoginRequest{
			Email:    "logout@example.com",
			Password: "Password123!",
		})
		require.NoError(t, err)
		require.NotEmpty(t, loginResp.AccessToken)

		// 带审计上下文登出
		auditCtx := &service.AuditContext{
			IPAddress: "192.168.1.1",
			UserAgent: "Mozilla/5.0",
		}
		err = authSvc.LogoutWithAudit(ctx, loginResp.AccessToken, auditCtx)
		assert.NoError(t, err)
	})

	t.Run("无效token登出", func(t *testing.T) {
		auditCtx := &service.AuditContext{
			IPAddress: "192.168.1.1",
		}
		err := authSvc.LogoutWithAudit(ctx, "invalid-token", auditCtx)
		// 应该返回错误（token无效）
		assert.Error(t, err)
	})
}

// ============================================================================
// LogoutAllWithAudit 测试
// ============================================================================

func TestAuthService_LogoutAllWithAudit(t *testing.T) {
	authSvc, store := createTestAuthService(t)
	ctx := context.Background()

	t.Run("带审计上下文的登出所有设备", func(t *testing.T) {
		store.Reset()

		hashedPassword, _ := crypto.NewPasswordService(4).HashPassword("Password123!")
		testUser := &model.User{
			ID:            "test-user-logoutall",
			Email:         "logoutall@example.com",
			PasswordHash:  hashedPassword,
			Role:          model.UserRoleUser,
			Status:        model.UserStatusActive,
			EmailVerified: true,
			CreatedAt:     time.Now(),
			UpdatedAt:     time.Now(),
		}
		store.AddUser(testUser)

		auditCtx := &service.AuditContext{
			IPAddress: "192.168.1.1",
			UserAgent: "Mozilla/5.0",
		}
		err := authSvc.LogoutAllWithAudit(ctx, testUser.ID, auditCtx)
		assert.NoError(t, err)
	})

	t.Run("撤销Token失败", func(t *testing.T) {
		store := mock.New()
		store.RevokeAllUserTokensErr = assert.AnError
		passwordSvc := crypto.NewPasswordService(4)
		privateKey, _ := rsa.GenerateKey(rand.Reader, 2048)
		jwtSvc := crypto.NewJWTService(privateKey, &privateKey.PublicKey, "test-issuer", 15*time.Minute, 7*24*time.Hour)
		authSvc := service.NewAuthService(store, passwordSvc, jwtSvc, 5, 30*time.Minute)

		// 创建测试用户
		testUser := &model.User{
			ID:           "test-user-revoke-fail",
			Email:        "revokefail@example.com",
			PasswordHash: "hash",
			Status:       model.UserStatusActive,
			CreatedAt:    time.Now(),
			UpdatedAt:    time.Now(),
		}
		store.AddUser(testUser)

		err := authSvc.LogoutAllWithAudit(ctx, testUser.ID, nil)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "登出所有设备失败")
	})
}

// ============================================================================
// RefreshTokenWithAudit 测试
// ============================================================================

func TestAuthService_RefreshTokenWithAudit(t *testing.T) {
	authSvc, store := createTestAuthService(t)
	ctx := context.Background()

	t.Run("带审计上下文的刷新token", func(t *testing.T) {
		store.Reset()

		hashedPassword, _ := crypto.NewPasswordService(4).HashPassword("Password123!")
		testUser := &model.User{
			ID:            "test-user-refresh",
			Email:         "refresh@example.com",
			PasswordHash:  hashedPassword,
			Role:          model.UserRoleUser,
			Status:        model.UserStatusActive,
			EmailVerified: true,
			CreatedAt:     time.Now(),
			UpdatedAt:     time.Now(),
		}
		store.AddUser(testUser)

		loginResp, err := authSvc.Login(ctx, &model.LoginRequest{
			Email:    "refresh@example.com",
			Password: "Password123!",
		})
		require.NoError(t, err)
		require.NotEmpty(t, loginResp.RefreshToken)

		// 带审计上下文刷新token
		auditCtx := &service.AuditContext{
			IPAddress: "192.168.1.1",
			UserAgent: "Mozilla/5.0",
		}
		newTokenResp, err := authSvc.RefreshTokenWithAudit(ctx, loginResp.RefreshToken, auditCtx)
		require.NoError(t, err)
		assert.NotEmpty(t, newTokenResp.AccessToken)
		assert.NotEmpty(t, newTokenResp.RefreshToken)
	})

	t.Run("无效refresh token", func(t *testing.T) {
		auditCtx := &service.AuditContext{
			IPAddress: "192.168.1.1",
		}
		_, err := authSvc.RefreshTokenWithAudit(ctx, "invalid-refresh-token", auditCtx)
		assert.Error(t, err)
	})
}

// ============================================================================
// AuthService with metrics 测试
// ============================================================================

func TestAuthService_WithMetrics(t *testing.T) {
	// 创建Mock存储
	store := mock.New()

	// 创建密码服务
	passwordSvc := crypto.NewPasswordService(4)

	// 创建JWT服务
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)

	jwtSvc := crypto.NewJWTService(
		privateKey,
		&privateKey.PublicKey,
		"test-issuer",
		15*time.Minute,
		7*24*time.Hour,
	)

	// 创建metrics服务
	metricsSvc := metrics.NewService()

	// 创建带metrics的认证服务
	authSvc := service.NewAuthService(store, passwordSvc, jwtSvc, 5, 30*time.Minute, metricsSvc)

	ctx := context.Background()

	t.Run("登录触发metrics", func(t *testing.T) {
		store.Reset()

		hashedPassword, _ := passwordSvc.HashPassword("Password123!")
		testUser := &model.User{
			ID:            "test-user-metrics",
			Email:         "metrics@example.com",
			PasswordHash:  hashedPassword,
			Role:          model.UserRoleUser,
			Status:        model.UserStatusActive,
			EmailVerified: true,
			CreatedAt:     time.Now(),
			UpdatedAt:     time.Now(),
		}
		store.AddUser(testUser)

		loginResp, err := authSvc.Login(ctx, &model.LoginRequest{
			Email:    "metrics@example.com",
			Password: "Password123!",
		})
		require.NoError(t, err)
		assert.NotEmpty(t, loginResp.AccessToken)
	})

	t.Run("登录失败触发metrics", func(t *testing.T) {
		store.Reset()

		hashedPassword, _ := passwordSvc.HashPassword("Password123!")
		testUser := &model.User{
			ID:            "test-user-metrics-fail",
			Email:         "metricsfail@example.com",
			PasswordHash:  hashedPassword,
			Role:          model.UserRoleUser,
			Status:        model.UserStatusActive,
			EmailVerified: true,
			CreatedAt:     time.Now(),
			UpdatedAt:     time.Now(),
		}
		store.AddUser(testUser)

		_, err := authSvc.Login(ctx, &model.LoginRequest{
			Email:    "metricsfail@example.com",
			Password: "WrongPassword!",
		})
		assert.Error(t, err)
	})
}

// ============================================================================
// Mock Store 错误注入测试
// 验证存储层故障时服务的错误处理行为
// ============================================================================

func TestAuthService_Register_StoreErrors(t *testing.T) {
	ctx := context.Background()

	t.Run("GetByEmail失败-返回错误", func(t *testing.T) {
		storeInst := mock.New()
		storeInst.GetUserByEmailErr = fmt.Errorf("database connection lost")
		passwordSvc := crypto.NewPasswordService(4)
		privateKey, _ := rsa.GenerateKey(rand.Reader, 2048)
		jwtSvc := crypto.NewJWTService(privateKey, &privateKey.PublicKey, "test", 15*time.Minute, 7*24*time.Hour)
		authSvc := service.NewAuthService(storeInst, passwordSvc, jwtSvc, 5, 30*time.Minute)

		_, err := authSvc.Register(ctx, &model.RegisterRequest{
			Email:    "test@example.com",
			Password: "Password123!!",
		})

		assert.Error(t, err)
	})

	t.Run("Create失败-返回错误", func(t *testing.T) {
		storeInst := mock.New()
		storeInst.CreateUserErr = fmt.Errorf("disk full")
		passwordSvc := crypto.NewPasswordService(4)
		privateKey, _ := rsa.GenerateKey(rand.Reader, 2048)
		jwtSvc := crypto.NewJWTService(privateKey, &privateKey.PublicKey, "test", 15*time.Minute, 7*24*time.Hour)
		authSvc := service.NewAuthService(storeInst, passwordSvc, jwtSvc, 5, 30*time.Minute)

		_, err := authSvc.Register(ctx, &model.RegisterRequest{
			Email:    "test@example.com",
			Password: "Password123!!",
		})

		assert.Error(t, err)
	})
}

func TestAuthService_Login_StoreErrors(t *testing.T) {
	ctx := context.Background()

	t.Run("GetByEmail失败-返回InvalidCredentials", func(t *testing.T) {
		storeInst := mock.New()
		storeInst.GetUserByEmailErr = fmt.Errorf("database timeout")
		passwordSvc := crypto.NewPasswordService(4)
		privateKey, _ := rsa.GenerateKey(rand.Reader, 2048)
		jwtSvc := crypto.NewJWTService(privateKey, &privateKey.PublicKey, "test", 15*time.Minute, 7*24*time.Hour)
		authSvc := service.NewAuthService(storeInst, passwordSvc, jwtSvc, 5, 30*time.Minute)

		_, err := authSvc.Login(ctx, &model.LoginRequest{
			Email:    "test@example.com",
			Password: "Password123!",
		})

		assert.Error(t, err)
	})
}

func TestAuthService_RefreshToken_StoreErrors(t *testing.T) {
	ctx := context.Background()

	t.Run("GetTokenByRefreshToken失败-返回InvalidToken", func(t *testing.T) {
		storeInst := mock.New()
		storeInst.GetTokenByRefreshTokenErr = fmt.Errorf("database error")
		passwordSvc := crypto.NewPasswordService(4)
		privateKey, _ := rsa.GenerateKey(rand.Reader, 2048)
		jwtSvc := crypto.NewJWTService(privateKey, &privateKey.PublicKey, "test", 15*time.Minute, 7*24*time.Hour)
		authSvc := service.NewAuthService(storeInst, passwordSvc, jwtSvc, 5, 30*time.Minute)

		_, err := authSvc.RefreshToken(ctx, "some-refresh-token")

		assert.ErrorIs(t, err, service.ErrInvalidToken)
	})

	t.Run("GetByID失败-返回错误", func(t *testing.T) {
		storeInst := mock.New()
		// 先创建token记录
		storeInst.AddToken(&model.Token{
			ID:           "token-1",
			UserID:       "user-1",
			RefreshToken: "valid-refresh",
			AccessToken:  "valid-access",
		})
		// 然后让GetByID失败
		storeInst.GetUserByIDErr = fmt.Errorf("user not found in db")
		passwordSvc := crypto.NewPasswordService(4)
		privateKey, _ := rsa.GenerateKey(rand.Reader, 2048)
		jwtSvc := crypto.NewJWTService(privateKey, &privateKey.PublicKey, "test", 15*time.Minute, 7*24*time.Hour)
		authSvc := service.NewAuthService(storeInst, passwordSvc, jwtSvc, 5, 30*time.Minute)

		_, err := authSvc.RefreshToken(ctx, "valid-refresh")

		assert.Error(t, err)
	})
}

func TestAuthService_Logout_StoreErrors(t *testing.T) {
	ctx := context.Background()

	t.Run("RevokeToken失败-返回错误", func(t *testing.T) {
		storeInst := mock.New()
		storeInst.RevokeTokenErr = fmt.Errorf("token table locked")
		passwordSvc := crypto.NewPasswordService(4)
		privateKey, _ := rsa.GenerateKey(rand.Reader, 2048)
		jwtSvc := crypto.NewJWTService(privateKey, &privateKey.PublicKey, "test", 15*time.Minute, 7*24*time.Hour)
		authSvc := service.NewAuthService(storeInst, passwordSvc, jwtSvc, 5, 30*time.Minute)

		err := authSvc.Logout(ctx, "some-access-token")

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "登出失败")
	})
}

func TestAuthService_LogoutAll_StoreErrors(t *testing.T) {
	ctx := context.Background()

	t.Run("RevokeAllUserTokens失败-返回错误", func(t *testing.T) {
		storeInst := mock.New()
		storeInst.RevokeAllUserTokensErr = fmt.Errorf("database error")
		passwordSvc := crypto.NewPasswordService(4)
		privateKey, _ := rsa.GenerateKey(rand.Reader, 2048)
		jwtSvc := crypto.NewJWTService(privateKey, &privateKey.PublicKey, "test", 15*time.Minute, 7*24*time.Hour)
		authSvc := service.NewAuthService(storeInst, passwordSvc, jwtSvc, 5, 30*time.Minute)

		err := authSvc.LogoutAll(ctx, "user-123")

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "登出所有设备失败")
	})
}

// ============================================================================
// 审计日志写入验证测试
// ============================================================================

func TestAuthService_LoginWithAudit_VerifyLog(t *testing.T) {
	authSvc, storeInst := createTestAuthService(t)
	ctx := context.Background()

	t.Run("登录成功写入审计日志", func(t *testing.T) {
		storeInst.Reset()

		hashedPassword, _ := crypto.NewPasswordService(4).HashPassword("Password123!")
		storeInst.AddUser(&model.User{
			ID:            "audit-login-user",
			Email:         "auditlogin@example.com",
			PasswordHash:  hashedPassword,
			Status:        model.UserStatusActive,
			EmailVerified: true,
			CreatedAt:     time.Now(),
			UpdatedAt:     time.Now(),
		})

		auditCtx := &service.AuditContext{
			IPAddress: "192.168.1.100",
			UserAgent: "TestAgent/1.0",
		}
		_, err := authSvc.LoginWithAudit(ctx, &model.LoginRequest{
			Email:    "auditlogin@example.com",
			Password: "Password123!",
		}, auditCtx)
		require.NoError(t, err)

		// 验证审计日志已写入
		require.Eventually(t, func() bool {
			logs, _, err := storeInst.ListAuditLogs(ctx, "audit-login-user", string(model.EventUserLogin), 0, 10)
			return err == nil && len(logs) >= 1
		}, 2*time.Second, 10*time.Millisecond, "审计日志未写入")

		logs, _, _ := storeInst.ListAuditLogs(ctx, "audit-login-user", string(model.EventUserLogin), 0, 10)
		assert.Equal(t, "192.168.1.100", logs[0].IPAddress)
		assert.True(t, logs[0].Success)
	})

	t.Run("登录失败写入审计日志", func(t *testing.T) {
		storeInst.Reset()

		hashedPassword, _ := crypto.NewPasswordService(4).HashPassword("Password123!")
		storeInst.AddUser(&model.User{
			ID:            "audit-login-fail-user",
			Email:         "auditfail@example.com",
			PasswordHash:  hashedPassword,
			Status:        model.UserStatusActive,
			EmailVerified: true,
			CreatedAt:     time.Now(),
			UpdatedAt:     time.Now(),
		})

		auditCtx := &service.AuditContext{
			IPAddress: "10.0.0.1",
		}
		_, err := authSvc.LoginWithAudit(ctx, &model.LoginRequest{
			Email:    "auditfail@example.com",
			Password: "WrongPassword!",
		}, auditCtx)
		assert.Error(t, err)

		// 验证登录事件审计日志已写入（success=false）
		require.Eventually(t, func() bool {
			logs, _, err := storeInst.ListAuditLogs(ctx, "audit-login-fail-user", string(model.EventUserLogin), 0, 10)
			return err == nil && len(logs) >= 1
		}, 2*time.Second, 10*time.Millisecond, "登录失败审计日志未写入")

		logs, _, _ := storeInst.ListAuditLogs(ctx, "audit-login-fail-user", string(model.EventUserLogin), 0, 10)
		assert.False(t, logs[0].Success)
	})
}

func TestAuthService_LogoutWithAudit_VerifyLog(t *testing.T) {
	authSvc, storeInst := createTestAuthService(t)
	ctx := context.Background()

	t.Run("登出写入审计日志", func(t *testing.T) {
		storeInst.Reset()

		hashedPassword, _ := crypto.NewPasswordService(4).HashPassword("Password123!")
		storeInst.AddUser(&model.User{
			ID:            "audit-logout-user",
			Email:         "auditlogout@example.com",
			PasswordHash:  hashedPassword,
			Role:          model.UserRoleUser,
			Status:        model.UserStatusActive,
			EmailVerified: true,
			CreatedAt:     time.Now(),
			UpdatedAt:     time.Now(),
		})

		loginResp, err := authSvc.Login(ctx, &model.LoginRequest{
			Email:    "auditlogout@example.com",
			Password: "Password123!",
		})
		require.NoError(t, err)

		auditCtx := &service.AuditContext{IPAddress: "172.16.0.1"}
		err = authSvc.LogoutWithAudit(ctx, loginResp.AccessToken, auditCtx)
		require.NoError(t, err)

		require.Eventually(t, func() bool {
			logs, _, err := storeInst.ListAuditLogs(ctx, "audit-logout-user", string(model.EventUserLogout), 0, 10)
			return err == nil && len(logs) >= 1
		}, 2*time.Second, 10*time.Millisecond, "登出审计日志未写入")

		logs, _, _ := storeInst.ListAuditLogs(ctx, "audit-logout-user", string(model.EventUserLogout), 0, 10)
		assert.Equal(t, "172.16.0.1", logs[0].IPAddress)
	})
}

func TestAuthService_RefreshTokenWithAudit_VerifyLog(t *testing.T) {
	authSvc, storeInst := createTestAuthService(t)
	ctx := context.Background()

	t.Run("刷新Token写入审计日志", func(t *testing.T) {
		storeInst.Reset()

		hashedPassword, _ := crypto.NewPasswordService(4).HashPassword("Password123!")
		storeInst.AddUser(&model.User{
			ID:            "audit-refresh-user",
			Email:         "auditrefresh@example.com",
			PasswordHash:  hashedPassword,
			Role:          model.UserRoleUser,
			Status:        model.UserStatusActive,
			EmailVerified: true,
			CreatedAt:     time.Now(),
			UpdatedAt:     time.Now(),
		})

		loginResp, err := authSvc.Login(ctx, &model.LoginRequest{
			Email:    "auditrefresh@example.com",
			Password: "Password123!",
		})
		require.NoError(t, err)

		auditCtx := &service.AuditContext{IPAddress: "192.168.2.1"}
		_, err = authSvc.RefreshTokenWithAudit(ctx, loginResp.RefreshToken, auditCtx)
		require.NoError(t, err)

		require.Eventually(t, func() bool {
			logs, _, err := storeInst.ListAuditLogs(ctx, "audit-refresh-user", string(model.EventTokenRefresh), 0, 10)
			return err == nil && len(logs) >= 1
		}, 2*time.Second, 10*time.Millisecond, "Token刷新审计日志未写入")

		logs, _, _ := storeInst.ListAuditLogs(ctx, "audit-refresh-user", string(model.EventTokenRefresh), 0, 10)
		assert.Equal(t, "192.168.2.1", logs[0].IPAddress)
	})
}

// ============================================================================
// revokeTokenWithRetry 集成测试
// ============================================================================

// MockCache 模拟缓存实现
type MockCache struct {
	data             map[string]interface{}
	deleteErr        error
	deleteCallCount  int
	deletedKeys      []string
	setErr           error
	getErr           error
	deletePatternErr error
	closeErr         error
}

// NewMockCache 创建模拟缓存
func NewMockCache() *MockCache {
	return &MockCache{
		data:        make(map[string]interface{}),
		deletedKeys: make([]string, 0),
	}
}

// Get 获取缓存值
func (m *MockCache) Get(ctx context.Context, key string, dest interface{}) error {
	if m.getErr != nil {
		return m.getErr
	}
	value, ok := m.data[key]
	if !ok {
		return cache.ErrCacheMiss
	}
	// 简单的赋值（实际应该是JSON反序列化）
	if v, ok := dest.(*interface{}); ok {
		*v = value
	}
	return nil
}

// Set 设置缓存值
func (m *MockCache) Set(ctx context.Context, key string, value interface{}, ttl time.Duration) error {
	if m.setErr != nil {
		return m.setErr
	}
	m.data[key] = value
	return nil
}

// SetWithNilProtection 设置缓存值（带空值保护）
func (m *MockCache) SetWithNilProtection(ctx context.Context, key string, value interface{}, ttl time.Duration, nilTTL time.Duration) error {
	if m.setErr != nil {
		return m.setErr
	}
	m.data[key] = value
	return nil
}

// Delete 删除缓存值
func (m *MockCache) Delete(ctx context.Context, key string) error {
	m.deleteCallCount++
	m.deletedKeys = append(m.deletedKeys, key)
	if m.deleteErr != nil {
		return m.deleteErr
	}
	delete(m.data, key)
	return nil
}

// DeletePattern 删除匹配模式的缓存值
func (m *MockCache) DeletePattern(ctx context.Context, pattern string) error {
	if m.deletePatternErr != nil {
		return m.deletePatternErr
	}
	return nil
}

// Close 关闭缓存
func (m *MockCache) Close() error {
	return m.closeErr
}

// Increment 原子递增计数器
func (m *MockCache) Increment(ctx context.Context, key string) (int, error) {
	value, ok := m.data[key]
	if !ok {
		m.data[key] = 1
		return 1, nil
	}
	count, ok := value.(int)
	if !ok {
		count = 0
	}
	count++
	m.data[key] = count
	return count, nil
}

// SetTTL 设置键的过期时间
func (m *MockCache) SetTTL(ctx context.Context, key string, ttl time.Duration) error {
	// Mock实现不需要实际设置TTL
	return nil
}

// GetTTL 获取键的剩余过期时间
func (m *MockCache) GetTTL(ctx context.Context, key string) (time.Duration, error) {
	// Mock实现返回固定值
	return 1 * time.Hour, nil
}

// TestAuthService_RevokeTokenWithRetry_SuccessAfterRetry 测试重试成功场景
// 验证: 需求 1.1, 1.6, 1.7
func TestAuthService_RevokeTokenWithRetry_SuccessAfterRetry(t *testing.T) {
	ctx := context.Background()

	t.Run("数据库临时不可用-重试成功后Token被正确撤销", func(t *testing.T) {
		storeInst := mock.New()
		mockCache := NewMockCache()

		// 创建Token
		token := &model.Token{
			ID:           "token-1",
			UserID:       "user-1",
			AccessToken:  "test-access-token-123",
			RefreshToken: "test-refresh-token-456",
			CreatedAt:    time.Now(),
			ExpiresAt:    time.Now().Add(15 * time.Minute),
		}
		storeInst.AddToken(token)

		// 在缓存中存储Token信息
		mockCache.data[cache.TokenKey("test-access-token-123")] = token

		// 创建认证服务（带缓存）
		passwordSvc := crypto.NewPasswordService(4)
		privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
		require.NoError(t, err)
		jwtSvc := crypto.NewJWTService(
			privateKey,
			&privateKey.PublicKey,
			"test-issuer",
			15*time.Minute,
			7*24*time.Hour,
		)
		authSvc := service.NewAuthServiceWithOptions(
			storeInst, passwordSvc, jwtSvc, 5, 30*time.Minute,
			service.WithCache(mockCache),
		)

		// 执行撤销
		err = authSvc.Logout(ctx, "test-access-token-123")

		// 验证成功
		require.NoError(t, err)

		// 验证Token被撤销
		revokedToken, err := storeInst.GetTokenByAccessToken(ctx, "test-access-token-123")
		require.NoError(t, err)
		assert.NotNil(t, revokedToken.RevokedAt)

		// 验证缓存被清除
		assert.Equal(t, 1, mockCache.deleteCallCount)
		assert.Contains(t, mockCache.deletedKeys, cache.TokenKey("test-access-token-123"))
		_, ok := mockCache.data[cache.TokenKey("test-access-token-123")]
		assert.False(t, ok, "缓存应该被删除")
	})

	t.Run("缓存清除失败不影响主流程", func(t *testing.T) {
		storeInst := mock.New()
		mockCache := NewMockCache()
		mockCache.deleteErr = fmt.Errorf("缓存服务暂时不可用")

		// 创建Token
		token := &model.Token{
			ID:           "token-2",
			UserID:       "user-2",
			AccessToken:  "test-access-token-789",
			RefreshToken: "test-refresh-token-012",
			CreatedAt:    time.Now(),
			ExpiresAt:    time.Now().Add(15 * time.Minute),
		}
		storeInst.AddToken(token)

		// 创建认证服务（带缓存）
		passwordSvc := crypto.NewPasswordService(4)
		privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
		require.NoError(t, err)
		jwtSvc := crypto.NewJWTService(
			privateKey,
			&privateKey.PublicKey,
			"test-issuer",
			15*time.Minute,
			7*24*time.Hour,
		)
		authSvc := service.NewAuthServiceWithOptions(
			storeInst, passwordSvc, jwtSvc, 5, 30*time.Minute,
			service.WithCache(mockCache),
		)

		// 执行撤销
		err = authSvc.Logout(ctx, "test-access-token-789")

		// 验证成功（缓存清除失败不影响主流程）
		require.NoError(t, err)

		// 验证Token被撤销
		revokedToken, err := storeInst.GetTokenByAccessToken(ctx, "test-access-token-789")
		require.NoError(t, err)
		assert.NotNil(t, revokedToken.RevokedAt)

		// 验证尝试清除缓存
		assert.Equal(t, 1, mockCache.deleteCallCount)
	})
}

// TestAuthService_RevokeTokenWithRetry_MaxRetriesExceeded 测试达到最大重试次数
// 验证: 需求 1.1, 1.6, 1.7
func TestAuthService_RevokeTokenWithRetry_MaxRetriesExceeded(t *testing.T) {
	ctx := context.Background()

	t.Run("达到最大重试次数后返回错误", func(t *testing.T) {
		storeInst := mock.New()
		mockCache := NewMockCache()

		// 注入持续失败的错误
		storeInst.RevokeTokenErr = fmt.Errorf("database lock timeout")

		// 创建Token
		token := &model.Token{
			ID:           "token-3",
			UserID:       "user-3",
			AccessToken:  "test-access-token-fail",
			RefreshToken: "test-refresh-token-fail",
			CreatedAt:    time.Now(),
			ExpiresAt:    time.Now().Add(15 * time.Minute),
		}
		storeInst.AddToken(token)

		// 创建认证服务（带缓存）
		passwordSvc := crypto.NewPasswordService(4)
		privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
		require.NoError(t, err)
		jwtSvc := crypto.NewJWTService(
			privateKey,
			&privateKey.PublicKey,
			"test-issuer",
			15*time.Minute,
			7*24*time.Hour,
		)
		authSvc := service.NewAuthServiceWithOptions(
			storeInst, passwordSvc, jwtSvc, 5, 30*time.Minute,
			service.WithCache(mockCache),
		)

		// 执行撤销
		err = authSvc.Logout(ctx, "test-access-token-fail")

		// 验证返回错误
		require.Error(t, err)
		assert.Contains(t, err.Error(), "登出失败")
		assert.Contains(t, err.Error(), "after 3 retries")

		// 验证Token未被撤销
		revokedToken, err := storeInst.GetTokenByAccessToken(ctx, "test-access-token-fail")
		require.NoError(t, err)
		assert.Nil(t, revokedToken.RevokedAt)

		// 验证缓存未被清除
		assert.Equal(t, 0, mockCache.deleteCallCount)
	})
}

// TestAuthService_RevokeTokenWithRetry_CacheCleared 测试缓存被正确清除
// 验证: 需求 1.1, 1.6, 1.7
func TestAuthService_RevokeTokenWithRetry_CacheCleared(t *testing.T) {
	ctx := context.Background()

	t.Run("验证缓存被正确清除", func(t *testing.T) {
		storeInst := mock.New()
		mockCache := NewMockCache()

		// 创建多个Token
		tokens := []string{
			"token-cache-1",
			"token-cache-2",
			"token-cache-3",
		}

		for i, tokenStr := range tokens {
			token := &model.Token{
				ID:           fmt.Sprintf("token-%d", i),
				UserID:       fmt.Sprintf("user-%d", i),
				AccessToken:  tokenStr,
				RefreshToken: fmt.Sprintf("refresh-%d", i),
				CreatedAt:    time.Now(),
				ExpiresAt:    time.Now().Add(15 * time.Minute),
			}
			storeInst.AddToken(token)
			mockCache.data[cache.TokenKey(tokenStr)] = token
		}

		// 创建认证服务（带缓存）
		passwordSvc := crypto.NewPasswordService(4)
		privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
		require.NoError(t, err)
		jwtSvc := crypto.NewJWTService(
			privateKey,
			&privateKey.PublicKey,
			"test-issuer",
			15*time.Minute,
			7*24*time.Hour,
		)
		authSvc := service.NewAuthServiceWithOptions(
			storeInst, passwordSvc, jwtSvc, 5, 30*time.Minute,
			service.WithCache(mockCache),
		)

		// 撤销第一个Token
		err = authSvc.Logout(ctx, tokens[0])
		require.NoError(t, err)

		// 验证只有第一个Token的缓存被清除
		assert.Equal(t, 1, mockCache.deleteCallCount)
		assert.Contains(t, mockCache.deletedKeys, cache.TokenKey(tokens[0]))

		// 验证其他Token的缓存仍然存在
		_, ok := mockCache.data[cache.TokenKey(tokens[1])]
		assert.True(t, ok, "第二个Token的缓存应该仍然存在")
		_, ok = mockCache.data[cache.TokenKey(tokens[2])]
		assert.True(t, ok, "第三个Token的缓存应该仍然存在")
	})
}

// TestAuthService_RevokeTokenWithRetry_WithoutCache 测试没有缓存的场景
// 验证: 需求 1.1, 1.6, 1.7
func TestAuthService_RevokeTokenWithRetry_WithoutCache(t *testing.T) {
	ctx := context.Background()

	t.Run("没有缓存时Token仍被正确撤销", func(t *testing.T) {
		storeInst := mock.New()

		// 创建Token
		token := &model.Token{
			ID:           "token-no-cache",
			UserID:       "user-no-cache",
			AccessToken:  "test-access-token-no-cache",
			RefreshToken: "test-refresh-token-no-cache",
			CreatedAt:    time.Now(),
			ExpiresAt:    time.Now().Add(15 * time.Minute),
		}
		storeInst.AddToken(token)

		// 创建认证服务（不带缓存）
		passwordSvc := crypto.NewPasswordService(4)
		privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
		require.NoError(t, err)
		jwtSvc := crypto.NewJWTService(
			privateKey,
			&privateKey.PublicKey,
			"test-issuer",
			15*time.Minute,
			7*24*time.Hour,
		)
		authSvc := service.NewAuthService(storeInst, passwordSvc, jwtSvc, 5, 30*time.Minute)

		// 执行撤销
		err = authSvc.Logout(ctx, "test-access-token-no-cache")

		// 验证成功
		require.NoError(t, err)

		// 验证Token被撤销
		revokedToken, err := storeInst.GetTokenByAccessToken(ctx, "test-access-token-no-cache")
		require.NoError(t, err)
		assert.NotNil(t, revokedToken.RevokedAt)
	})
}

// TestAuthService_RevokeTokenWithRetry_ContextCancellation 测试上下文取消
// 验证: 需求 1.1, 1.6, 1.7
func TestAuthService_RevokeTokenWithRetry_ContextCancellation(t *testing.T) {
	t.Run("上下文取消时停止重试", func(t *testing.T) {
		storeInst := mock.New()
		mockCache := NewMockCache()

		// 注入持续失败的错误
		storeInst.RevokeTokenErr = fmt.Errorf("database lock timeout")

		// 创建Token
		token := &model.Token{
			ID:           "token-cancel",
			UserID:       "user-cancel",
			AccessToken:  "test-access-token-cancel",
			RefreshToken: "test-refresh-token-cancel",
			CreatedAt:    time.Now(),
			ExpiresAt:    time.Now().Add(15 * time.Minute),
		}
		storeInst.AddToken(token)

		// 创建认证服务（带缓存）
		passwordSvc := crypto.NewPasswordService(4)
		privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
		require.NoError(t, err)
		jwtSvc := crypto.NewJWTService(
			privateKey,
			&privateKey.PublicKey,
			"test-issuer",
			15*time.Minute,
			7*24*time.Hour,
		)
		authSvc := service.NewAuthServiceWithOptions(
			storeInst, passwordSvc, jwtSvc, 5, 30*time.Minute,
			service.WithCache(mockCache),
		)

		// 创建可取消的上下文
		ctx, cancel := context.WithCancel(context.Background())
		cancel() // 立即取消

		// 执行撤销
		err = authSvc.Logout(ctx, "test-access-token-cancel")

		// 验证返回错误
		require.Error(t, err)
	})
}

// TestAuthService_RevokeTokenWithRetry_TokenNotFound 测试Token不存在的场景
// 验证: 需求 1.1, 1.6, 1.7
func TestAuthService_RevokeTokenWithRetry_TokenNotFound(t *testing.T) {
	ctx := context.Background()

	t.Run("Token不存在时返回错误", func(t *testing.T) {
		storeInst := mock.New()
		mockCache := NewMockCache()

		// 创建认证服务（带缓存）
		passwordSvc := crypto.NewPasswordService(4)
		privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
		require.NoError(t, err)
		jwtSvc := crypto.NewJWTService(
			privateKey,
			&privateKey.PublicKey,
			"test-issuer",
			15*time.Minute,
			7*24*time.Hour,
		)
		authSvc := service.NewAuthServiceWithOptions(
			storeInst, passwordSvc, jwtSvc, 5, 30*time.Minute,
			service.WithCache(mockCache),
		)

		// 执行撤销（Token不存在）
		err = authSvc.Logout(ctx, "nonexistent-token")

		// 验证返回错误
		require.Error(t, err)
		assert.Contains(t, err.Error(), "登出失败")
	})
}
