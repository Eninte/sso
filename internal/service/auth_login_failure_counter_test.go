// Package service 测试登录失败计数器安全性
package service

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/example/sso/internal/crypto"
	apperrors "github.com/example/sso/internal/errors"
	"github.com/example/sso/internal/model"
	"github.com/example/sso/internal/store/mock"
)

// createTestJWTServiceForLoginTest 创建测试用的JWT服务
func createTestJWTServiceForLoginTest() *crypto.JWTService {
	privateKey, _ := rsa.GenerateKey(rand.Reader, 2048)
	return crypto.NewJWTService(
		privateKey,
		&privateKey.PublicKey,
		"test-issuer",
		15*time.Minute,
		7*24*time.Hour,
	)
}

// TestLoginFailureCounter_DatabaseError 测试数据库错误时登录失败计数器不会被绕过
// 安全问题 #13: 登录失败计数器可能被绕过
//
// 测试场景:
//  1. 用户使用错误密码登录（密码校验必然先失败，进入 handleLoginFailure）
//  2. IncrementLoginAttempts 返回数据库错误（通过 IncrementLoginAttemptsErr 注入）
//  3. 验证登录请求被拒绝，且返回的不是 ErrInvalidCredentials 而是包装后的服务错误
//     ——若此处错误被吞、退化为 ErrInvalidCredentials，攻击者即可通过触发 DB 错误
//     使计数不递增，从而绕过账户锁定
//  4. 验证错误消息不泄露 DSN/内部细节
func TestLoginFailureCounter_DatabaseError(t *testing.T) {
	// 准备测试数据
	hashedPassword, err := crypto.NewPasswordService(10).HashPassword("correct-password")
	require.NoError(t, err)

	user := &model.User{
		ID:            "user-123",
		Email:         "test@example.com",
		PasswordHash:  hashedPassword,
		EmailVerified: true,
		Status:        model.UserStatusActive,
		Role:          "user",
	}

	// 创建Mock Store并注入数据库错误
	mockStore := mock.New()
	mockStore.AddUser(user)
	dbErr := errors.New("connection reset by peer: postgres://sso:supersecret@10.0.0.9:5432/sso")
	mockStore.IncrementLoginAttemptsErr = dbErr

	// 创建服务
	passwordSvc := crypto.NewPasswordService(10)
	jwtSvc := createTestJWTServiceForLoginTest()
	authSvc := NewAuthService(mockStore, passwordSvc, jwtSvc, 5, 30*time.Minute)

	// 执行登录（使用错误密码）
	req := &model.LoginRequest{
		Email:    user.Email,
		Password: "wrong-password",
	}

	resp, err := authSvc.Login(context.Background(), req)

	// 验证结果：DB 错误必须导致登录失败，且不得退化为普通凭据错误
	require.Error(t, err, "数据库错误时登录必须被拒绝")
	assert.Nil(t, resp, "数据库错误时不应返回响应")
	assert.False(t, apperrors.Is(err, ErrInvalidCredentials),
		"数据库错误不得包装为 ErrInvalidCredentials，否则攻击者可借 DB 错误绕过锁定计数")

	// 错误消息不得泄露 DSN 等内部细节
	assert.NotContains(t, err.Error(), "supersecret", "错误消息不得泄露 DSN 凭据")
	assert.NotContains(t, err.Error(), "10.0.0.9", "错误消息不得泄露数据库地址")

	// 计数器不得在 DB 错误时被记为已递增
	updatedUser, err := mockStore.GetByID(context.Background(), user.ID)
	require.NoError(t, err)
	assert.Equal(t, 0, updatedUser.LoginAttempts, "DB 错误时计数器不应递增")
	assert.NotEqual(t, model.UserStatusLocked, updatedUser.Status, "DB 错误时不得锁定账户")
}

// TestLoginFailureCounter_NormalFlow 测试正常流程下登录失败计数器工作正常
func TestLoginFailureCounter_NormalFlow(t *testing.T) {
	// 准备测试数据
	hashedPassword, err := crypto.NewPasswordService(10).HashPassword("correct-password")
	require.NoError(t, err)

	user := &model.User{
		ID:            "user-123",
		Email:         "test@example.com",
		PasswordHash:  hashedPassword,
		EmailVerified: true,
		Status:        model.UserStatusActive,
	}

	// 创建Mock Store
	mockStore := mock.New()
	mockStore.AddUser(user)

	// 创建服务
	passwordSvc := crypto.NewPasswordService(10)
	jwtSvc := createTestJWTServiceForLoginTest()
	authSvc := NewAuthService(mockStore, passwordSvc, jwtSvc, 5, 30*time.Minute)

	// 测试1: 第一次失败
	req := &model.LoginRequest{
		Email:    user.Email,
		Password: "wrong-password",
	}

	resp, err := authSvc.Login(context.Background(), req)
	assert.Error(t, err)
	assert.Nil(t, resp)
	assert.True(t, apperrors.Is(err, ErrInvalidCredentials))

	updatedUser, _ := mockStore.GetByID(context.Background(), user.ID)
	assert.Equal(t, 1, updatedUser.LoginAttempts, "应该记录1次失败")

	// 测试2: 第二次失败
	resp, err = authSvc.Login(context.Background(), req)
	assert.Error(t, err)
	assert.Nil(t, resp)
	assert.True(t, apperrors.Is(err, ErrInvalidCredentials))

	updatedUser, _ = mockStore.GetByID(context.Background(), user.ID)
	assert.Equal(t, 2, updatedUser.LoginAttempts, "应该记录2次失败")

	// 测试3: 第五次失败（达到最大次数）
	for i := 3; i <= 5; i++ {
		resp, err = authSvc.Login(context.Background(), req)
		assert.Error(t, err)
		assert.Nil(t, resp)
	}

	updatedUser, _ = mockStore.GetByID(context.Background(), user.ID)
	assert.Equal(t, 5, updatedUser.LoginAttempts, "应该记录5次失败")
	assert.Equal(t, model.UserStatusLocked, updatedUser.Status, "账户应该被锁定")
	assert.NotNil(t, updatedUser.LockedUntil, "应该设置锁定时间")

	// 测试4: 账户被锁定后继续尝试登录
	resp, err = authSvc.Login(context.Background(), req)
	assert.Error(t, err)
	assert.Nil(t, resp)
	assert.True(t, apperrors.Is(err, ErrAccountLocked), "账户应该被锁定")
}

// TestLoginFailureCounter_SuccessfulLoginResetsCounter 测试成功登录重置计数器
func TestLoginFailureCounter_SuccessfulLoginResetsCounter(t *testing.T) {
	// 准备测试数据
	hashedPassword, err := crypto.NewPasswordService(10).HashPassword("correct-password")
	require.NoError(t, err)

	user := &model.User{
		ID:            "user-123",
		Email:         "test@example.com",
		PasswordHash:  hashedPassword,
		EmailVerified: true,
		Status:        model.UserStatusActive,
		Role:          "user",
		LoginAttempts: 3, // 假设之前已经失败3次
	}

	// 创建Mock Store
	mockStore := mock.New()
	mockStore.AddUser(user)

	// 创建服务
	passwordSvc := crypto.NewPasswordService(10)
	jwtSvc := createTestJWTServiceForLoginTest()
	authSvc := NewAuthService(mockStore, passwordSvc, jwtSvc, 5, 30*time.Minute)

	// 测试1: 失败一次
	req := &model.LoginRequest{
		Email:    user.Email,
		Password: "wrong-password",
	}

	resp, err := authSvc.Login(context.Background(), req)
	assert.Error(t, err)
	assert.Nil(t, resp)

	updatedUser, _ := mockStore.GetByID(context.Background(), user.ID)
	assert.Equal(t, 4, updatedUser.LoginAttempts, "应该记录4次失败")

	// 测试2: 成功登录
	req.Password = "correct-password"
	resp, err = authSvc.Login(context.Background(), req)
	assert.NoError(t, err)
	assert.NotNil(t, resp)

	updatedUser, _ = mockStore.GetByID(context.Background(), user.ID)
	assert.Equal(t, 0, updatedUser.LoginAttempts, "计数器应该被重置为0")
}

// TestLoginFailureCounter_ConcurrentFailures 测试并发登录失败
func TestLoginFailureCounter_ConcurrentFailures(t *testing.T) {
	// 准备测试数据
	hashedPassword, err := crypto.NewPasswordService(10).HashPassword("correct-password")
	require.NoError(t, err)

	user := &model.User{
		ID:            "user-123",
		Email:         "test@example.com",
		PasswordHash:  hashedPassword,
		EmailVerified: true,
		Status:        model.UserStatusActive,
	}

	// 创建Mock Store（使用原子操作模拟）
	mockStore := mock.New()
	mockStore.AddUser(user)

	// 创建服务
	passwordSvc := crypto.NewPasswordService(10)
	jwtSvc := createTestJWTServiceForLoginTest()
	authSvc := NewAuthService(mockStore, passwordSvc, jwtSvc, 5, 30*time.Minute)

	// 并发执行10次登录失败
	const concurrency = 10
	errors := make(chan error, concurrency)

	req := &model.LoginRequest{
		Email:    user.Email,
		Password: "wrong-password",
	}

	for i := 0; i < concurrency; i++ {
		go func() {
			_, err := authSvc.Login(context.Background(), req)
			errors <- err
		}()
	}

	// 收集结果
	errorCount := 0
	for i := 0; i < concurrency; i++ {
		err := <-errors
		if err != nil {
			errorCount++
		}
	}

	// 验证所有请求都失败
	assert.Equal(t, concurrency, errorCount, "所有并发请求都应该失败")

	// 验证计数器被正确递增（Mock Store使用原子操作）
	updatedUser, _ := mockStore.GetByID(context.Background(), user.ID)
	assert.GreaterOrEqual(t, updatedUser.LoginAttempts, 1, "计数器应该至少递增1次")
	assert.LessOrEqual(t, updatedUser.LoginAttempts, concurrency, "计数器不应该超过并发数")
}

// TestLoginFailureCounter_ErrorMessagesDoNotExposeSecrets 测试登录失败路径的
// 错误消息不泄露敏感信息（密码、邮箱、DSN）
func TestLoginFailureCounter_ErrorMessagesDoNotExposeSecrets(t *testing.T) {
	// 准备测试数据
	hashedPassword, err := crypto.NewPasswordService(10).HashPassword("correct-password")
	require.NoError(t, err)

	user := &model.User{
		ID:            "user-123",
		Email:         "test@example.com",
		PasswordHash:  hashedPassword,
		EmailVerified: true,
		Status:        model.UserStatusActive,
	}

	// 创建Mock Store
	mockStore := mock.New()
	mockStore.AddUser(user)

	// 创建服务
	passwordSvc := crypto.NewPasswordService(10)
	jwtSvc := createTestJWTServiceForLoginTest()
	authSvc := NewAuthService(mockStore, passwordSvc, jwtSvc, 5, 30*time.Minute)

	// 测试1: 使用错误密码
	req := &model.LoginRequest{
		Email:    user.Email,
		Password: "wrong-password",
	}

	resp, err := authSvc.Login(context.Background(), req)
	assert.Error(t, err)
	assert.Nil(t, resp)
	assert.NotContains(t, err.Error(), "wrong-password", "错误消息不应该包含密码")
	assert.True(t, apperrors.Is(err, ErrInvalidCredentials), "应该返回ErrInvalidCredentials")

	// 测试2: 使用不存在的邮箱
	req.Email = "nonexistent@example.com"
	resp, err = authSvc.Login(context.Background(), req)
	assert.Error(t, err)
	assert.Nil(t, resp)
	assert.True(t, apperrors.Is(err, ErrInvalidCredentials), "不存在的邮箱应该返回ErrInvalidCredentials")
	assert.NotContains(t, err.Error(), "nonexistent@example.com", "错误消息不应该包含邮箱")

	// 测试3: DB 错误路径的错误消息不得包含 DSN（store 错误可能内嵌连接串）
	mockStore.IncrementLoginAttemptsErr = errors.New(
		"failed to connect to `host=10.0.0.9 user=sso password=supersecret database=sso`: dial error")
	req.Email = user.Email
	resp, err = authSvc.Login(context.Background(), req)
	require.Error(t, err)
	assert.Nil(t, resp)
	assert.NotContains(t, err.Error(), "supersecret", "DB 错误消息不得泄露 DSN 凭据")
	assert.NotContains(t, err.Error(), "10.0.0.9", "DB 错误消息不得泄露数据库地址")
}
