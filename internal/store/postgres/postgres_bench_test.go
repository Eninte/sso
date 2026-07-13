// Package postgres_test PostgreSQL存储基准测试
package postgres_test

import (
	"context"
	"database/sql"
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/example/sso/internal/model"
	"github.com/example/sso/internal/store/postgres"
	"github.com/example/sso/internal/util/testutil"
)

// ============================================================================
// 基准测试辅助函数
// ============================================================================

// getBenchDB 返回已 ping 通的真实 PostgreSQL 连接（带重试与超时）。
// 复用 testutil.ConnectTestDB，与全仓集成测试共享同一套 TEST_CONN_* 配置。
// 注意：testutil 内部会通过 b.Cleanup 关闭连接，bench 中的 defer db.Close() 是幂等的多余调用，可保留。
func getBenchDB(b *testing.B) *sql.DB {
	b.Helper()
	return testutil.ConnectTestDB(b)
}

func setupBenchStore(b *testing.B) (*postgres.Store, *sql.DB) {
	b.Helper()
	db := getBenchDB(b)
	return postgres.New(db), db
}

func createBenchUser(b *testing.B, store *postgres.Store, email string) *model.User {
	b.Helper()
	user := &model.User{
		ID:            uuid.New().String(),
		Email:         email,
		PasswordHash:  "$2a$10$testhashvalue0123456789abc",
		EmailVerified: true,
		Status:        model.UserStatusActive,
		CreatedAt:     time.Now(),
		UpdatedAt:     time.Now(),
	}
	if err := store.Create(context.Background(), user); err != nil {
		b.Fatal(err)
	}
	return user
}

// ============================================================================
// 用户CRUD基准测试
// ============================================================================

func BenchmarkStore_CreateUser(b *testing.B) {
	store, db := setupBenchStore(b)
	defer db.Close()
	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		email := fmt.Sprintf("bench-create-%d@example.com", i)
		user := &model.User{
			ID:            uuid.New().String(),
			Email:         email,
			PasswordHash:  "$2a$10$testhashvalue0123456789abc",
			EmailVerified: false,
			Status:        model.UserStatusActive,
			CreatedAt:     time.Now(),
			UpdatedAt:     time.Now(),
		}
		if err := store.Create(ctx, user); err != nil {
			b.Fatal(err)
		}
	}

	b.Cleanup(func() {
		db.ExecContext(ctx, "DELETE FROM users WHERE email LIKE 'bench-create-%@%'")
	})
}

func BenchmarkStore_GetUserByID(b *testing.B) {
	store, db := setupBenchStore(b)
	defer db.Close()
	ctx := context.Background()

	user := createBenchUser(b, store, "bench-getbyid@example.com")
	defer db.ExecContext(ctx, "DELETE FROM users WHERE id = $1", user.ID)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := store.GetByID(ctx, user.ID)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkStore_GetUserByEmail(b *testing.B) {
	store, db := setupBenchStore(b)
	defer db.Close()
	ctx := context.Background()

	user := createBenchUser(b, store, "bench-getbyemail@example.com")
	defer db.ExecContext(ctx, "DELETE FROM users WHERE id = $1", user.ID)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := store.GetByEmail(ctx, user.Email)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkStore_UpdateUser(b *testing.B) {
	store, db := setupBenchStore(b)
	defer db.Close()
	ctx := context.Background()

	user := createBenchUser(b, store, "bench-update@example.com")
	defer db.ExecContext(ctx, "DELETE FROM users WHERE id = $1", user.ID)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		user.UpdatedAt = time.Now()
		if err := store.Update(ctx, user); err != nil {
			b.Fatal(err)
		}
	}
}

// ============================================================================
// 登录尝试基准测试
// ============================================================================

func BenchmarkStore_UpdateLoginAttempts(b *testing.B) {
	store, db := setupBenchStore(b)
	defer db.Close()
	ctx := context.Background()

	user := createBenchUser(b, store, "bench-loginattempts@example.com")
	defer db.ExecContext(ctx, "DELETE FROM users WHERE id = $1", user.ID)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		lockedUntil := time.Now().Add(30 * time.Minute)
		if err := store.UpdateLoginAttempts(ctx, user.ID, i%5, &lockedUntil); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkStore_IncrementLoginAttempts(b *testing.B) {
	store, db := setupBenchStore(b)
	defer db.Close()
	ctx := context.Background()

	user := createBenchUser(b, store, "bench-increment@example.com")
	defer db.ExecContext(ctx, "DELETE FROM users WHERE id = $1", user.ID)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if i%10 == 0 {
			store.ResetLoginAttempts(ctx, user.ID)
		}
		_, _, _, err := store.IncrementLoginAttempts(ctx, user.ID, 5, 30*time.Minute)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkStore_ResetLoginAttempts(b *testing.B) {
	store, db := setupBenchStore(b)
	defer db.Close()
	ctx := context.Background()

	user := createBenchUser(b, store, "bench-reset@example.com")
	defer db.ExecContext(ctx, "DELETE FROM users WHERE id = $1", user.ID)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := store.ResetLoginAttempts(ctx, user.ID); err != nil {
			b.Fatal(err)
		}
	}
}

// ============================================================================
// 并发基准测试
// ============================================================================

func BenchmarkStore_GetUserByID_Parallel(b *testing.B) {
	store, db := setupBenchStore(b)
	defer db.Close()
	ctx := context.Background()

	users := make([]*model.User, 10)
	for i := 0; i < 10; i++ {
		users[i] = createBenchUser(b, store, fmt.Sprintf("bench-parallel-%d@example.com", i))
	}
	defer func() {
		for _, u := range users {
			db.ExecContext(ctx, "DELETE FROM users WHERE id = $1", u.ID)
		}
	}()

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			userID := users[i%len(users)].ID
			_, err := store.GetByID(ctx, userID)
			if err != nil {
				b.Error(err)
			}
			i++
		}
	})
}

func BenchmarkStore_GetUserByEmail_Parallel(b *testing.B) {
	store, db := setupBenchStore(b)
	defer db.Close()
	ctx := context.Background()

	users := make([]*model.User, 10)
	for i := 0; i < 10; i++ {
		users[i] = createBenchUser(b, store, fmt.Sprintf("bench-parallel-email-%d@example.com", i))
	}
	defer func() {
		for _, u := range users {
			db.ExecContext(ctx, "DELETE FROM users WHERE id = $1", u.ID)
		}
	}()

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			email := users[i%len(users)].Email
			_, err := store.GetByEmail(ctx, email)
			if err != nil {
				b.Error(err)
			}
			i++
		}
	})
}

func BenchmarkStore_Ping(b *testing.B) {
	store, db := setupBenchStore(b)
	defer db.Close()
	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := store.Ping(ctx); err != nil {
			b.Fatal(err)
		}
	}
}
