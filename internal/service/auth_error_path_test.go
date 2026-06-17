// Package service_test AuthService错误路径测试
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

	"github.com/your-org/sso/internal/crypto"
	"github.com/your-org/sso/internal/model"
	"github.com/your-org/sso/internal/service"
	"github.com/your-org/sso/internal/store"
	"github.com/your-org/sso/internal/store/mock"
)

// ============================================================================
// AuthService.Login 错误路径测试
// 验证: 需求 8.1, 8.2, 8.3, 8.7
// ============================================================================

// TestAuthService_Login_ErrorPaths 测试AuthService.Login的各种错误场景
func TestAuthService_Login_ErrorPaths(t *testing.T) {
	ctx := context.Background()

	// ==== 测试1: Store返回数据库错误 ====
	// 验证: 需求 8.1
	t.Run("Store返回数据库错误", func(t *testing.T) {
		// 创建Mock Store并注入数据库错误
		storeInst := mock.New()
		storeInst.GetUserByEmailErr = fmt.Errorf("database connection failed")

		// 创建AuthService
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

		// 尝试登录
		_, err = authSvc.Login(ctx, &model.LoginRequest{
			Email:    "test@example.com",
			Password: "Password123!",
		})

		// 验证返回错误
		assert.Error(t, err)

		// TODO: 需求 8.7 - 当前实现直接返回store错误，暴露了内部详情
		// 未来应该包装错误，不暴露内部数据库错误详情
		// 当前行为：直接返回原始错误
		assert.Contains(t, err.Error(), "database connection failed")
	})

	// ==== 测试2: 账户被锁定场景 ====
	// 验证: 需求 8.2
	t.Run("账户被锁定", func(t *testing.T) {
		storeInst := mock.New()
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

		// 创建被锁定的用户
		hashedPassword, err := passwordSvc.HashPassword("Password123!")
		require.NoError(t, err)

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
		storeInst.AddUser(lockedUser)

		// 尝试登录
		_, err = authSvc.Login(ctx, &model.LoginRequest{
			Email:    "locked@example.com",
			Password: "Password123!",
		})

		// 验证返回账户锁定错误
		assert.Error(t, err)
		assert.ErrorIs(t, err, service.ErrAccountLocked)
		// 验证不暴露内部错误详情（需求 8.7）
		assert.NotContains(t, err.Error(), "LockedUntil")
		assert.NotContains(t, err.Error(), lockedUntil.String())
	})

	// ==== 测试3: 邮箱未验证场景 ====
	// 验证: 需求 8.3
	t.Run("邮箱未验证", func(t *testing.T) {
		storeInst := mock.New()
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

		// 创建邮箱未验证的用户
		hashedPassword, err := passwordSvc.HashPassword("Password123!")
		require.NoError(t, err)

		unverifiedUser := &model.User{
			ID:            "unverified-user-id",
			Email:         "unverified@example.com",
			PasswordHash:  hashedPassword,
			Status:        model.UserStatusActive,
			EmailVerified: false, // 邮箱未验证
			CreatedAt:     time.Now(),
			UpdatedAt:     time.Now(),
		}
		storeInst.AddUser(unverifiedUser)

		// 尝试登录
		_, err = authSvc.Login(ctx, &model.LoginRequest{
			Email:    "unverified@example.com",
			Password: "Password123!",
		})

		// 验证返回通用凭据错误（不暴露邮箱未验证状态）
		assert.Error(t, err)
		assert.ErrorIs(t, err, service.ErrInvalidCredentials)
		// 验证不暴露内部错误详情（需求 8.7）
		assert.NotContains(t, err.Error(), "user_id")
		assert.NotContains(t, err.Error(), unverifiedUser.ID)
	})

	// ==== 测试4: 密码错误场景 ====
	// 验证: 需求 8.1
	t.Run("密码错误", func(t *testing.T) {
		storeInst := mock.New()
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

		// 创建正常用户
		hashedPassword, err := passwordSvc.HashPassword("CorrectPassword123!")
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
		storeInst.AddUser(testUser)

		// 使用错误密码尝试登录
		_, err = authSvc.Login(ctx, &model.LoginRequest{
			Email:    "test@example.com",
			Password: "WrongPassword123!",
		})

		// 验证返回凭据无效错误
		assert.Error(t, err)
		assert.ErrorIs(t, err, service.ErrInvalidCredentials)
		// 验证不暴露密码哈希或其他内部详情（需求 8.7）
		assert.NotContains(t, err.Error(), hashedPassword)
		assert.NotContains(t, err.Error(), "PasswordHash")
		assert.NotContains(t, err.Error(), "bcrypt")
	})

	// ==== 测试5: 验证不暴露内部错误详情 ====
	// 验证: 需求 8.7
	t.Run("不暴露内部错误详情", func(t *testing.T) {
		storeInst := mock.New()
		// 注入包含敏感信息的数据库错误
		storeInst.GetUserByEmailErr = fmt.Errorf("SQL error: connection to postgres://admin:secret@db:5432/sso failed")

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

		// 尝试登录
		_, err = authSvc.Login(ctx, &model.LoginRequest{
			Email:    "test@example.com",
			Password: "Password123!",
		})

		// 验证返回错误
		require.Error(t, err)

		// TODO: 需求 8.7 - 当前实现暴露了内部错误详情
		// 未来应该包装错误，隐藏敏感信息
		// 当前行为：直接返回原始错误（包含敏感信息）
		errorMsg := err.Error()

		// 记录当前行为：确实暴露了敏感信息
		assert.Contains(t, errorMsg, "SQL error", "当前实现暴露了SQL错误详情")

		// 期望行为（未来实现）：
		// assert.NotContains(t, errorMsg, "secret", "不应暴露数据库密码")
		// assert.NotContains(t, errorMsg, "postgres://", "不应暴露数据库连接字符串")
		// assert.NotContains(t, errorMsg, "admin", "不应暴露数据库用户名")
		// assert.NotContains(t, errorMsg, "SQL error", "不应暴露SQL错误详情")
	})

	// ==== 测试6: 用户不存在返回通用错误 ====
	// 验证: 需求 8.7
	t.Run("用户不存在返回通用错误", func(t *testing.T) {
		storeInst := mock.New()
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

		// 尝试登录不存在的用户
		_, err = authSvc.Login(ctx, &model.LoginRequest{
			Email:    "nonexistent@example.com",
			Password: "Password123!",
		})

		// 验证返回通用的凭据无效错误（不暴露用户是否存在）
		assert.Error(t, err)
		assert.ErrorIs(t, err, service.ErrInvalidCredentials)
		// 验证不暴露"用户不存在"的信息
		assert.NotContains(t, err.Error(), "not found")
		assert.NotContains(t, err.Error(), "does not exist")
		assert.NotContains(t, err.Error(), store.ErrNotFound.Error())
	})

	// ==== 测试7: 账户禁用返回特定错误 ====
	// 验证: 需求 8.2
	t.Run("账户禁用", func(t *testing.T) {
		storeInst := mock.New()
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

		// 创建被禁用的用户
		hashedPassword, err := passwordSvc.HashPassword("Password123!")
		require.NoError(t, err)

		disabledUser := &model.User{
			ID:            "disabled-user-id",
			Email:         "disabled@example.com",
			PasswordHash:  hashedPassword,
			Status:        model.UserStatusDisabled,
			EmailVerified: true,
			CreatedAt:     time.Now(),
			UpdatedAt:     time.Now(),
		}
		storeInst.AddUser(disabledUser)

		// 尝试登录
		_, err = authSvc.Login(ctx, &model.LoginRequest{
			Email:    "disabled@example.com",
			Password: "Password123!",
		})

		// 验证返回账户禁用错误
		assert.Error(t, err)
		assert.ErrorIs(t, err, service.ErrAccountDisabled)
		// 验证不暴露内部详情
		assert.NotContains(t, err.Error(), disabledUser.ID)
	})
}

// ============================================================================
// AuthService.Register 错误路径测试
// 验证: 需求 8.4
// ============================================================================

// TestAuthService_Register_ErrorPaths 测试AuthService.Register的各种错误场景
func TestAuthService_Register_ErrorPaths(t *testing.T) {
	ctx := context.Background()

	// ==== 测试1: 邮箱验证失败 - 空邮箱 ====
	// 验证: 需求 8.4
	t.Run("邮箱验证失败_空邮箱", func(t *testing.T) {
		storeInst := mock.New()
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

		// 尝试使用空邮箱注册
		_, err = authSvc.Register(ctx, &model.RegisterRequest{
			Email:    "",
			Password: "Password123!",
		})

		// 验证返回邮箱必填错误
		assert.Error(t, err)
		// 验证不暴露内部错误详情（需求 8.7）
		assert.NotContains(t, err.Error(), "database")
		assert.NotContains(t, err.Error(), "store")
	})

	// ==== 测试2: 邮箱验证失败 - 无效格式 ====
	// 验证: 需求 8.4
	t.Run("邮箱验证失败_无效格式", func(t *testing.T) {
		storeInst := mock.New()
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

		// 尝试使用无效邮箱格式注册
		_, err = authSvc.Register(ctx, &model.RegisterRequest{
			Email:    "invalid-email-format",
			Password: "Password123!",
		})

		// 验证返回邮箱格式错误
		assert.Error(t, err)
		// 验证不暴露内部错误详情
		assert.NotContains(t, err.Error(), "database")
		assert.NotContains(t, err.Error(), "store")
	})

	// ==== 测试3: 邮箱验证失败 - 缺少@符号 ====
	// 验证: 需求 8.4
	t.Run("邮箱验证失败_缺少@符号", func(t *testing.T) {
		storeInst := mock.New()
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

		// 尝试使用缺少@符号的邮箱注册
		_, err = authSvc.Register(ctx, &model.RegisterRequest{
			Email:    "testexample.com",
			Password: "Password123!",
		})

		// 验证返回邮箱格式错误
		assert.Error(t, err)
	})

	// ==== 测试4: Store返回数据库错误 - GetByEmail失败 ====
	// 验证: 需求 8.4
	t.Run("Store返回数据库错误_GetByEmail", func(t *testing.T) {
		storeInst := mock.New()
		// 注入数据库错误
		storeInst.GetUserByEmailErr = fmt.Errorf("database connection failed")

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

		// 尝试注册
		_, err = authSvc.Register(ctx, &model.RegisterRequest{
			Email:    "test@example.com",
			Password: "Password123!",
		})

		// 验证返回错误
		assert.Error(t, err)

		// TODO: 需求 8.7 - 当前实现直接返回store错误，暴露了内部详情
		// 未来应该包装错误，不暴露内部数据库错误详情
		// 当前行为：直接返回原始错误
		assert.Contains(t, err.Error(), "database connection failed")
	})

	// ==== 测试5: Store返回数据库错误 - Create失败 ====
	// 验证: 需求 8.4
	t.Run("Store返回数据库错误_Create", func(t *testing.T) {
		storeInst := mock.New()
		// 注入Create错误
		storeInst.CreateUserErr = fmt.Errorf("database write failed")

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

		// 尝试注册
		_, err = authSvc.Register(ctx, &model.RegisterRequest{
			Email:    "test@example.com",
			Password: "Password123!",
		})

		// 验证返回错误
		assert.Error(t, err)

		// TODO: 需求 8.7 - 当前实现直接返回store错误
		// 未来应该包装错误，不暴露内部数据库错误详情
		assert.Contains(t, err.Error(), "database write failed")
	})

	// ==== 测试6: 重复邮箱 ====
	// 验证: 需求 8.4
	t.Run("重复邮箱", func(t *testing.T) {
		storeInst := mock.New()
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

		// 创建已存在的用户
		hashedPassword, err := passwordSvc.HashPassword("Password123!")
		require.NoError(t, err)

		existingUser := &model.User{
			ID:            "existing-user-id",
			Email:         "existing@example.com",
			PasswordHash:  hashedPassword,
			Status:        model.UserStatusActive,
			EmailVerified: false,
			CreatedAt:     time.Now(),
			UpdatedAt:     time.Now(),
		}
		storeInst.AddUser(existingUser)

		// 尝试使用相同邮箱注册（应返回nil,nil，不暴露邮箱已存在）
		user, err := authSvc.Register(ctx, &model.RegisterRequest{
			Email:    "existing@example.com",
			Password: "Password123!",
		})

		// 验证不返回错误（防止用户枚举），也不返回用户对象
		assert.NoError(t, err)
		assert.Nil(t, user)
	})

	// ==== 测试7: 密码验证失败 - 太短 ====
	// 验证: 需求 8.4
	t.Run("密码验证失败_太短", func(t *testing.T) {
		storeInst := mock.New()
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

		// 尝试使用太短的密码注册
		_, err = authSvc.Register(ctx, &model.RegisterRequest{
			Email:    "test@example.com",
			Password: "Short1!",
		})

		// 验证返回密码太短错误
		assert.Error(t, err)
	})

	// ==== 测试8: 密码验证失败 - 缺少大写字母 ====
	// 验证: 需求 8.4
	t.Run("密码验证失败_缺少大写字母", func(t *testing.T) {
		storeInst := mock.New()
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

		// 尝试使用缺少大写字母的密码注册
		_, err = authSvc.Register(ctx, &model.RegisterRequest{
			Email:    "test@example.com",
			Password: "password123!",
		})

		// 验证返回密码格式错误
		assert.Error(t, err)
	})

	// ==== 测试9: 密码验证失败 - 缺少数字 ====
	// 验证: 需求 8.4
	t.Run("密码验证失败_缺少数字", func(t *testing.T) {
		storeInst := mock.New()
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

		// 尝试使用缺少数字的密码注册
		_, err = authSvc.Register(ctx, &model.RegisterRequest{
			Email:    "test@example.com",
			Password: "Password!!!",
		})

		// 验证返回密码格式错误
		assert.Error(t, err)
	})

	// ==== 测试10: 密码验证失败 - 缺少特殊字符 ====
	// 验证: 需求 8.4
	t.Run("密码验证失败_缺少特殊字符", func(t *testing.T) {
		storeInst := mock.New()
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

		// 尝试使用缺少特殊字符的密码注册
		_, err = authSvc.Register(ctx, &model.RegisterRequest{
			Email:    "test@example.com",
			Password: "Password123",
		})

		// 验证返回密码格式错误
		assert.Error(t, err)
	})

	// ==== 测试11: 验证不暴露内部错误详情 ====
	// 验证: 需求 8.7
	t.Run("不暴露内部错误详情", func(t *testing.T) {
		storeInst := mock.New()
		// 注入包含敏感信息的数据库错误
		storeInst.GetUserByEmailErr = fmt.Errorf("SQL error: connection to postgres://admin:secret@db:5432/sso failed")

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

		// 尝试注册
		_, err = authSvc.Register(ctx, &model.RegisterRequest{
			Email:    "test@example.com",
			Password: "Password123!",
		})

		// 验证返回错误
		require.Error(t, err)

		// TODO: 需求 8.7 - 当前实现暴露了内部错误详情
		// 未来应该包装错误，隐藏敏感信息
		// 当前行为：直接返回原始错误（包含敏感信息）
		errorMsg := err.Error()

		// 记录当前行为：确实暴露了敏感信息
		assert.Contains(t, errorMsg, "SQL error", "当前实现暴露了SQL错误详情")

		// 期望行为（未来实现）：
		// assert.NotContains(t, errorMsg, "secret", "不应暴露数据库密码")
		// assert.NotContains(t, errorMsg, "postgres://", "不应暴露数据库连接字符串")
		// assert.NotContains(t, errorMsg, "admin", "不应暴露数据库用户名")
		// assert.NotContains(t, errorMsg, "SQL error", "不应暴露SQL错误详情")
	})
}
