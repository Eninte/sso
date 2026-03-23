// Package service_test 认证服务单元测试
package service_test

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/your-org/sso/internal/crypto"
	"github.com/your-org/sso/internal/model"
	"github.com/your-org/sso/internal/service"
	"github.com/your-org/sso/internal/store/mock"

	store2 "github.com/your-org/sso/internal/store"
)

// ============================================================================
// 测试辅助函数
// ============================================================================

// createTestAuthService 创建测试用的认证服务
func createTestAuthService(t *testing.T) (*service.AuthService, *mock.MockStore) {
	// 创建Mock存储
	store := mock.New()

	// 创建密码服务
	passwordSvc := crypto.NewPasswordService(10) // 使用较低的cost加快测试

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
	hashedPassword, err := crypto.NewPasswordService(10).HashPassword("Password123!")
	require.NoError(t, err)

	testUser := &model.User{
		ID:           "test-user-id",
		Email:        "test@example.com",
		PasswordHash: hashedPassword,
		Status:       model.UserStatusActive,
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
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
		// 创建被禁用的用户
		disabledUser := &model.User{
			ID:           "disabled-user-id",
			Email:        "disabled@example.com",
			PasswordHash: hashedPassword,
			Status:       model.UserStatusDisabled,
			CreatedAt:    time.Now(),
			UpdatedAt:    time.Now(),
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
		// 创建被锁定的用户
		lockedUntil := time.Now().Add(30 * time.Minute)
		lockedUser := &model.User{
			ID:           "locked-user-id",
			Email:        "locked@example.com",
			PasswordHash: hashedPassword,
			Status:       model.UserStatusLocked,
			LockedUntil:  &lockedUntil,
			CreatedAt:    time.Now(),
			UpdatedAt:    time.Now(),
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
		// 创建锁定已过期的用户
		lockedUntil := time.Now().Add(-10 * time.Minute) // 10分钟前过期
		unlockedUser := &model.User{
			ID:           "unlocked-user-id",
			Email:        "unlocked@example.com",
			PasswordHash: hashedPassword,
			Status:       model.UserStatusLocked,
			LockedUntil:  &lockedUntil,
			CreatedAt:    time.Now(),
			UpdatedAt:    time.Now(),
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
		// maxAttempts默认为5
		failUser := &model.User{
			ID:            "fail-user-id",
			Email:         "fail@example.com",
			PasswordHash:  hashedPassword,
			Status:        model.UserStatusActive,
			LoginAttempts: 4, // 已经失败4次，再失败1次就锁定
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
	hashedPassword, _ := crypto.NewPasswordService(10).HashPassword("Password123!")
	testUser := &model.User{
		ID:           "test-user-id",
		Email:        "test@example.com",
		PasswordHash: hashedPassword,
		Status:       model.UserStatusActive,
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
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

	// 先登录获取Token
	store.Reset()
	hashedPassword, _ := crypto.NewPasswordService(10).HashPassword("Password123!")
	testUser := &model.User{
		ID:           "test-user-id",
		Email:        "test@example.com",
		PasswordHash: hashedPassword,
		Status:       model.UserStatusActive,
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
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

	// 先登录获取Token
	store.Reset()
	hashedPassword, _ := crypto.NewPasswordService(10).HashPassword("Password123!")
	testUser := &model.User{
		ID:           "test-user-id",
		Email:        "test@example.com",
		PasswordHash: hashedPassword,
		Status:       model.UserStatusActive,
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
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

		// 创建测试用户
		hashedPassword, _ := crypto.NewPasswordService(10).HashPassword("Password123!")
		testUser := &model.User{
			ID:           "test-user-logoutall",
			Email:        "logoutall@example.com",
			PasswordHash: hashedPassword,
			Status:       model.UserStatusActive,
			CreatedAt:    time.Now(),
			UpdatedAt:    time.Now(),
		}
		store.AddUser(testUser)

		// 多次登录获取多个Token
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
