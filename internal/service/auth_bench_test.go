// Package service_test 认证服务基准测试
package service_test

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"fmt"
	"testing"
	"time"

	"github.com/example/sso/internal/crypto"
	"github.com/example/sso/internal/model"
	"github.com/example/sso/internal/service"
	"github.com/example/sso/internal/store/mock"
)

// ============================================================================
// 基准测试辅助函数
// ============================================================================

func setupBenchAuthService(b *testing.B) (*service.AuthService, *mock.Store) {
	b.Helper()

	store := mock.New()
	passwordSvc := crypto.NewPasswordService(4)

	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		b.Fatal(err)
	}

	jwtSvc := crypto.NewJWTService(
		privateKey,
		&privateKey.PublicKey,
		"test-issuer",
		15*time.Minute,
		7*24*time.Hour,
	)

	authSvc := service.NewAuthService(store, passwordSvc, jwtSvc, 5, 30*time.Minute)
	return authSvc, store
}

func createBenchTestUser(b *testing.B, store *mock.Store, email, password string) *model.User {
	b.Helper()

	hashedPassword, err := crypto.NewPasswordService(4).HashPassword(password)
	if err != nil {
		b.Fatal(err)
	}

	user := &model.User{
		ID:           "bench-user-" + email,
		Email:        email,
		PasswordHash: hashedPassword,
		Status:       model.UserStatusActive,
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
	}
	store.AddUser(user)
	return user
}

// ============================================================================
// 密码服务基准测试
// ============================================================================

func BenchmarkPasswordService_Hash(b *testing.B) {
	passwordSvc := crypto.NewPasswordService(4)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := passwordSvc.HashPassword("Password123!")
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkPasswordService_Verify(b *testing.B) {
	passwordSvc := crypto.NewPasswordService(4)
	hashedPassword, _ := passwordSvc.HashPassword("Password123!")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := passwordSvc.VerifyPassword(hashedPassword, "Password123!"); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkPasswordService_HashVerify(b *testing.B) {
	passwordSvc := crypto.NewPasswordService(4)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		hashedPassword, _ := passwordSvc.HashPassword("Password123!")
		passwordSvc.VerifyPassword(hashedPassword, "Password123!")
	}
}

// ============================================================================
// JWT服务基准测试
// ============================================================================

func setupBenchJWTService(b *testing.B) *crypto.JWTService {
	b.Helper()

	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		b.Fatal(err)
	}

	return crypto.NewJWTService(
		privateKey,
		&privateKey.PublicKey,
		"test-issuer",
		15*time.Minute,
		7*24*time.Hour,
	)
}

func BenchmarkJWTService_GenerateAccessToken(b *testing.B) {
	jwtSvc := setupBenchJWTService(b)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := jwtSvc.GenerateAccessToken("user-123", "test@example.com", "user", []string{"openid", "profile"})
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkJWTService_GenerateRefreshToken(b *testing.B) {
	jwtSvc := setupBenchJWTService(b)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := jwtSvc.GenerateRefreshToken()
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkJWTService_ValidateAccessToken(b *testing.B) {
	jwtSvc := setupBenchJWTService(b)
	token, _ := jwtSvc.GenerateAccessToken("user-123", "test@example.com", "user", []string{"openid"})

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := jwtSvc.ValidateAccessToken(token)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkJWTService_GenerateAndValidate(b *testing.B) {
	jwtSvc := setupBenchJWTService(b)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		token, _ := jwtSvc.GenerateAccessToken("user-123", "test@example.com", "user", []string{"openid"})
		jwtSvc.ValidateAccessToken(token)
	}
}

// ============================================================================
// 认证服务基准测试
// ============================================================================

func BenchmarkAuthService_Login(b *testing.B) {
	authSvc, store := setupBenchAuthService(b)
	ctx := context.Background()

	createBenchTestUser(b, store, "bench-login@example.com", "Password123!")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := authSvc.Login(ctx, &model.LoginRequest{
			Email:    "bench-login@example.com",
			Password: "Password123!",
		})
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkAuthService_Login_Parallel(b *testing.B) {
	authSvc, store := setupBenchAuthService(b)
	ctx := context.Background()

	createBenchTestUser(b, store, "bench-login-parallel@example.com", "Password123!")

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_, err := authSvc.Login(ctx, &model.LoginRequest{
				Email:    "bench-login-parallel@example.com",
				Password: "Password123!",
			})
			if err != nil {
				b.Error(err)
			}
		}
	})
}

func BenchmarkAuthService_ValidateToken(b *testing.B) {
	authSvc, store := setupBenchAuthService(b)
	ctx := context.Background()

	createBenchTestUser(b, store, "bench-validate@example.com", "Password123!")

	loginResp, err := authSvc.Login(ctx, &model.LoginRequest{
		Email:    "bench-validate@example.com",
		Password: "Password123!",
	})
	if err != nil {
		b.Fatal(err)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := authSvc.ValidateToken(ctx, loginResp.AccessToken)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkAuthService_RefreshToken(b *testing.B) {
	authSvc, store := setupBenchAuthService(b)
	ctx := context.Background()

	createBenchTestUser(b, store, "bench-refresh@example.com", "Password123!")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		// 每次重新登录获取新的refresh token
		loginResp, err := authSvc.Login(ctx, &model.LoginRequest{
			Email:    "bench-refresh@example.com",
			Password: "Password123!",
		})
		if err != nil {
			b.Fatal(err)
		}

		_, err = authSvc.RefreshToken(ctx, loginResp.RefreshToken)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkAuthService_Register(b *testing.B) {
	authSvc, _ := setupBenchAuthService(b)
	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		email := fmt.Sprintf("bench-register-%d@example.com", i)
		_, err := authSvc.Register(ctx, &model.RegisterRequest{
			Email:    email,
			Password: "Password123!",
		})
		if err != nil {
			b.Fatal(err)
		}
	}
}

// ============================================================================
// 完整登录流程基准测试
// ============================================================================

func BenchmarkAuthService_LoginFlow(b *testing.B) {
	authSvc, store := setupBenchAuthService(b)
	ctx := context.Background()

	createBenchTestUser(b, store, "bench-flow@example.com", "Password123!")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		// 登录
		loginResp, err := authSvc.Login(ctx, &model.LoginRequest{
			Email:    "bench-flow@example.com",
			Password: "Password123!",
		})
		if err != nil {
			b.Fatal(err)
		}

		// 验证Token
		_, err = authSvc.ValidateToken(ctx, loginResp.AccessToken)
		if err != nil {
			b.Fatal(err)
		}

		// 登出
		authSvc.Logout(ctx, loginResp.AccessToken)
	}
}
