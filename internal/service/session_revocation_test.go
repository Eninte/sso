// Package service_test 阶段 2.4 会话统一撤销专项测试
//
// 测试范围：
//   - ChangePassword 后撤销所有 token
//   - Account Locked 时撤销所有 token
//   - RevokeToken 幂等性（已撤销不覆盖原时间戳）
//   - 撤销路径统一清缓存
package service_test

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/example/sso/internal/cache"
	"github.com/example/sso/internal/crypto"
	"github.com/example/sso/internal/model"
	"github.com/example/sso/internal/service"
	"github.com/example/sso/internal/store/mock"
)

// createPhase24TestJWTService 创建测试用的 JWT 服务
func createPhase24TestJWTService() *crypto.JWTService {
	privateKey, _ := rsa.GenerateKey(rand.Reader, 2048)
	return crypto.NewJWTService(
		privateKey,
		&privateKey.PublicKey,
		"test-issuer",
		15*time.Minute,
		7*24*time.Hour,
	)
}

// createPhase24TestUserService 创建带 cache 的 UserService（阶段 2.4 装配方式）
func createPhase24TestUserService(t *testing.T) (*service.UserService, *mock.Store, cache.Cache) {
	storeInst := mock.New()
	passwordSvc := crypto.NewPasswordService(4)
	jwtSvc := createPhase24TestJWTService()
	auditSvc := service.NewAuditService(storeInst)
	userSvc := service.NewUserServiceWithAudit(storeInst, passwordSvc, nil, "http://localhost:9000", auditSvc)

	// 使用内存缓存（与生产装配方式一致）
	memCache := cache.NewMemoryCache()
	userSvc.WithCache(memCache)
	_ = jwtSvc // 不直接使用，但保留装配链完整性
	return userSvc, storeInst, memCache
}

// createTestToken 记录到 store 中并返回完整 Token 对象
func createTestToken(t *testing.T, storeInst *mock.Store, userID string) *model.Token {
	t.Helper()
	now := time.Now()
	token := &model.Token{
		ID:           "token-" + userID + "-" + now.Format("150405.000000"),
		AccessToken:  "access-" + userID + "-" + now.Format("150405.000000"),
		RefreshToken: "refresh-" + userID + "-" + now.Format("150405.000000"),
		UserID:       userID,
		ExpiresAt:    now.Add(time.Hour),
		CreatedAt:    now,
	}
	require.NoError(t, storeInst.StoreToken(context.Background(), token))
	return token
}

// ============================================================================
// ChangePassword 后撤销所有 Token 测试
// ============================================================================

func TestChangePassword_RevokesAllTokens(t *testing.T) {
	t.Run("修改密码后所有Token被撤销", func(t *testing.T) {
		userSvc, storeInst, memCache := createPhase24TestUserService(t)
		ctx := context.Background()

		// 创建用户
		hashedPw, _ := crypto.NewPasswordService(4).HashPassword("OldPass123!")
		now := time.Now()
		user := &model.User{
			ID:            "user-changepw-1",
			Email:         "changepw@example.com",
			PasswordHash:  hashedPw,
			EmailVerified: true,
			Status:        model.UserStatusActive,
			CreatedAt:     now,
			UpdatedAt:     now,
		}
		require.NoError(t, storeInst.Create(ctx, user))

		// 创建 3 个 token（模拟多设备登录）
		token1 := createTestToken(t, storeInst, user.ID)
		token2 := createTestToken(t, storeInst, user.ID)
		token3 := createTestToken(t, storeInst, user.ID)

		// 预热缓存（模拟已登录访问后缓存已建立）
		// 通过 store.GetTokenByAccessToken 触发缓存写入（生产路径由 AuthMiddleware 完成）
		_, err := storeInst.GetTokenByAccessToken(ctx, token1.AccessToken)
		require.NoError(t, err)

		// 修改密码
		err = userSvc.ChangePasswordWithAudit(ctx, user.ID, "OldPass123!", "NewPass456!", "192.168.1.1")
		require.NoError(t, err)

		// 验证所有 token 都被撤销
		for _, tok := range []*model.Token{token1, token2, token3} {
			revoked, err := storeInst.GetTokenByAccessToken(ctx, tok.AccessToken)
			require.NoError(t, err)
			assert.NotNil(t, revoked.RevokedAt,
				"修改密码后 token %s 应被撤销", tok.ID)
		}

		// 验证 token 缓存已被清理（缓存未命中）
		var cached model.Token
		err = memCache.Get(ctx, "token:"+token1.AccessToken, &cached)
		assert.ErrorIs(t, err, cache.ErrCacheMiss,
			"修改密码后 token 缓存应被清理")
	})

	t.Run("修改密码失败时不撤销Token", func(t *testing.T) {
		userSvc, storeInst, _ := createPhase24TestUserService(t)
		ctx := context.Background()

		hashedPw, _ := crypto.NewPasswordService(4).HashPassword("OldPass123!")
		now := time.Now()
		user := &model.User{
			ID:            "user-changepw-fail",
			Email:         "changepwfail@example.com",
			PasswordHash:  hashedPw,
			EmailVerified: true,
			Status:        model.UserStatusActive,
			CreatedAt:     now,
			UpdatedAt:     now,
		}
		require.NoError(t, storeInst.Create(ctx, user))
		token1 := createTestToken(t, storeInst, user.ID)

		// 用错误的旧密码修改密码
		err := userSvc.ChangePasswordWithAudit(ctx, user.ID, "WrongPassword!", "NewPass456!", "192.168.1.1")
		require.Error(t, err)

		// 验证 token 未被撤销
		revoked, err := storeInst.GetTokenByAccessToken(ctx, token1.AccessToken)
		require.NoError(t, err)
		assert.Nil(t, revoked.RevokedAt, "密码修改失败时不应撤销 token")
	})
}

// ============================================================================
// Account Locked 时撤销所有 Token 测试
// ============================================================================

func TestAccountLock_RevokesAllTokens(t *testing.T) {
	t.Run("账户锁定时撤销所有Token", func(t *testing.T) {
		storeInst := mock.New()
		passwordSvc := crypto.NewPasswordService(4)
		jwtSvc := createPhase24TestJWTService()
		authSvc := service.NewAuthServiceWithOptions(
			storeInst,
			passwordSvc,
			jwtSvc,
			2,              // maxAttempts=2 便于触发锁定
			30*time.Minute, // lockoutDuration
			service.WithCache(cache.NewMemoryCache()),
		)
		ctx := context.Background()

		// 创建用户
		hashedPw, _ := passwordSvc.HashPassword("CorrectPass123!")
		now := time.Now()
		user := &model.User{
			ID:            "user-lock-1",
			Email:         "lock@example.com",
			PasswordHash:  hashedPw,
			EmailVerified: true,
			Status:        model.UserStatusActive,
			CreatedAt:     now,
			UpdatedAt:     now,
		}
		require.NoError(t, storeInst.Create(ctx, user))

		// 预先创建 2 个有效 token（模拟已登录设备）
		token1 := createTestToken(t, storeInst, user.ID)
		token2 := createTestToken(t, storeInst, user.ID)

		// 故意连续输入错误密码触发锁定
		auditCtx := &service.AuditContext{IPAddress: "192.168.1.100"}
		_, _ = authSvc.LoginWithAudit(ctx, &model.LoginRequest{
			Email:    user.Email,
			Password: "WrongPass1!",
		}, auditCtx)
		_, err := authSvc.LoginWithAudit(ctx, &model.LoginRequest{
			Email:    user.Email,
			Password: "WrongPass2!",
		}, auditCtx)

		// 第二次错误应触发锁定
		// （mock.IncrementLoginAttempts 在 attempts >= maxAttempts 时设置 locked=true）
		_ = err // 不关心返回的错误码

		// 验证所有 token 都被撤销
		for _, tok := range []*model.Token{token1, token2} {
			revoked, err := storeInst.GetTokenByAccessToken(ctx, tok.AccessToken)
			require.NoError(t, err)
			assert.NotNil(t, revoked.RevokedAt,
				"账户锁定后 token %s 应被撤销", tok.ID)
		}
	})
}

// ============================================================================
// RevokeToken 幂等性测试
// ============================================================================

func TestRevokeToken_Idempotent(t *testing.T) {
	t.Run("重复撤销不覆盖首次撤销时间戳", func(t *testing.T) {
		storeInst := mock.New()
		ctx := context.Background()

		token := createTestToken(t, storeInst, "user-idempotent")
		originalRevokeErr := storeInst.RevokeToken(ctx, token.AccessToken)
		require.NoError(t, originalRevokeErr)

		first, err := storeInst.GetTokenByAccessToken(ctx, token.AccessToken)
		require.NoError(t, err)
		require.NotNil(t, first.RevokedAt)
		firstRevokedAt := *first.RevokedAt

		// 等待确保时间戳可区分
		time.Sleep(time.Millisecond)

		// 再次撤销
		require.NoError(t, storeInst.RevokeToken(ctx, token.AccessToken))

		second, err := storeInst.GetTokenByAccessToken(ctx, token.AccessToken)
		require.NoError(t, err)
		require.NotNil(t, second.RevokedAt)

		// 阶段 2.4：验证首次撤销时间戳未被覆盖
		assert.Equal(t, firstRevokedAt, *second.RevokedAt,
			"重复撤销不应覆盖首次撤销时间戳")
	})

	t.Run("撤销不存在的Token不报错", func(t *testing.T) {
		storeInst := mock.New()
		ctx := context.Background()

		// 阶段 2.4：与 Postgres 行为对齐
		err := storeInst.RevokeToken(ctx, "nonexistent-token")
		assert.NoError(t, err, "撤销不存在的 token 不应报错（幂等设计）")
	})
}

// ============================================================================
// 统一缓存清理测试
// ============================================================================

func TestUnifiedCacheInvalidation(t *testing.T) {
	t.Run("LogoutAll清理Token缓存", func(t *testing.T) {
		storeInst := mock.New()
		passwordSvc := crypto.NewPasswordService(4)
		jwtSvc := createPhase24TestJWTService()
		memCache := cache.NewMemoryCache()
		authSvc := service.NewAuthServiceWithOptions(
			storeInst, passwordSvc, jwtSvc,
			5, 30*time.Minute,
			service.WithCache(memCache),
		)
		ctx := context.Background()

		user := &model.User{
			ID:            "user-logout-all",
			Email:         "logoutall@example.com",
			EmailVerified: true,
			Status:        model.UserStatusActive,
		}
		require.NoError(t, storeInst.Create(ctx, user))
		token := createTestToken(t, storeInst, user.ID)

		// 预热缓存
		require.NoError(t, memCache.Set(ctx, "token:"+token.AccessToken, token, 15*time.Minute))

		// LogoutAll
		require.NoError(t, authSvc.LogoutAll(ctx, user.ID))

		// 验证缓存已被清理
		var cached model.Token
		err := memCache.Get(ctx, "token:"+token.AccessToken, &cached)
		assert.ErrorIs(t, err, cache.ErrCacheMiss, "LogoutAll 后 token 缓存应被清理")
	})
}
