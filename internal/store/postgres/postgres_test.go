// Package postgres PostgreSQL存储集成测试
package postgres_test

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	_ "github.com/lib/pq"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/your-org/sso/internal/model"
	"github.com/your-org/sso/internal/store/postgres"
)

// ============================================================================
// 测试辅助函数
// ============================================================================

func getTestDB(t *testing.T) *sql.DB {
	t.Helper()
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		t.Skip("跳过集成测试：未设置DATABASE_URL环境变量")
	}
	db, err := sql.Open("postgres", dbURL)
	require.NoError(t, err)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	require.NoError(t, db.PingContext(ctx))
	return db
}

func setupTestStore(t *testing.T) (*postgres.Store, *sql.DB) {
	t.Helper()
	db := getTestDB(t)
	return postgres.New(db), db
}

func cleanupTestData(t *testing.T, db *sql.DB) {
	t.Helper()
	ctx := context.Background()
	_, _ = db.ExecContext(ctx, "DELETE FROM audit_logs WHERE user_id LIKE 'test-%'")
	_, _ = db.ExecContext(ctx, "DELETE FROM verification_tokens WHERE token LIKE 'test-%'")
	_, _ = db.ExecContext(ctx, "DELETE FROM reset_tokens WHERE token LIKE 'test-%'")
	_, _ = db.ExecContext(ctx, "DELETE FROM tokens WHERE access_token LIKE 'test-%'")
	_, _ = db.ExecContext(ctx, "DELETE FROM authorization_codes WHERE code LIKE 'test-%'")
	_, _ = db.ExecContext(ctx, "DELETE FROM users WHERE email LIKE 'test-%@%'")
}

func ptrTo(s string) *string {
	return &s
}

func newTestUser(email string) *model.User {
	return &model.User{
		ID:            uuid.New().String(),
		Email:         "test-" + email,
		PasswordHash:  "$2a$10$testhashvalue0123456789abc",
		EmailVerified: false,
		MFASecret:     "",
		Role:          model.UserRoleUser,
		Status:        model.UserStatusActive,
		CreatedAt:     time.Now(),
		UpdatedAt:     time.Now(),
	}
}

// ============================================================================
// 用户存储测试
// ============================================================================

func TestStore_CreateUser(t *testing.T) {
	store, db := setupTestStore(t)
	defer db.Close()
	defer cleanupTestData(t, db)
	ctx := context.Background()

	t.Run("成功创建用户", func(t *testing.T) {
		user := newTestUser("create1@example.com")
		err := store.Create(ctx, user)
		assert.NoError(t, err)

		retrieved, err := store.GetByID(ctx, user.ID)
		require.NoError(t, err)
		assert.Equal(t, user.Email, retrieved.Email)
	})

	t.Run("邮箱重复", func(t *testing.T) {
		user1 := newTestUser("dup@example.com")
		require.NoError(t, store.Create(ctx, user1))

		user2 := newTestUser("dup@example.com")
		assert.Error(t, store.Create(ctx, user2))
	})
}

func TestStore_GetUserByEmail(t *testing.T) {
	store, db := setupTestStore(t)
	defer db.Close()
	defer cleanupTestData(t, db)
	ctx := context.Background()

	user := newTestUser("getbyemail@example.com")
	require.NoError(t, store.Create(ctx, user))

	t.Run("成功获取用户", func(t *testing.T) {
		retrieved, err := store.GetByEmail(ctx, user.Email)
		require.NoError(t, err)
		assert.Equal(t, user.ID, retrieved.ID)
	})

	t.Run("用户不存在", func(t *testing.T) {
		_, err := store.GetByEmail(ctx, "nonexistent@example.com")
		assert.Error(t, err)
	})
}

func TestStore_GetUserByID(t *testing.T) {
	store, db := setupTestStore(t)
	defer db.Close()
	defer cleanupTestData(t, db)
	ctx := context.Background()

	user := newTestUser("getbyid@example.com")
	require.NoError(t, store.Create(ctx, user))

	t.Run("成功获取", func(t *testing.T) {
		retrieved, err := store.GetByID(ctx, user.ID)
		require.NoError(t, err)
		assert.Equal(t, user.Email, retrieved.Email)
	})

	t.Run("不存在", func(t *testing.T) {
		_, err := store.GetByID(ctx, uuid.New().String())
		assert.Error(t, err)
	})
}

func TestStore_UpdateUser(t *testing.T) {
	store, db := setupTestStore(t)
	defer db.Close()
	defer cleanupTestData(t, db)
	ctx := context.Background()

	user := newTestUser("update@example.com")
	require.NoError(t, store.Create(ctx, user))

	t.Run("更新用户信息", func(t *testing.T) {
		user.EmailVerified = true
		user.Status = model.UserStatusLocked
		user.UpdatedAt = time.Now()
		require.NoError(t, store.Update(ctx, user))

		retrieved, err := store.GetByID(ctx, user.ID)
		require.NoError(t, err)
		assert.True(t, retrieved.EmailVerified)
		assert.Equal(t, model.UserStatusLocked, retrieved.Status)
	})

	t.Run("更新登录尝试次数", func(t *testing.T) {
		lockedUntil := time.Now().Add(30 * time.Minute)
		require.NoError(t, store.UpdateLoginAttempts(ctx, user.ID, 3, &lockedUntil))

		retrieved, err := store.GetByID(ctx, user.ID)
		require.NoError(t, err)
		assert.Equal(t, 3, retrieved.LoginAttempts)
		assert.NotNil(t, retrieved.LockedUntil)
	})
}

func TestStore_IncrementLoginAttempts(t *testing.T) {
	store, db := setupTestStore(t)
	defer db.Close()
	defer cleanupTestData(t, db)
	ctx := context.Background()

	t.Run("递增登录尝试次数", func(t *testing.T) {
		user := newTestUser("increment@example.com")
		require.NoError(t, store.Create(ctx, user))

		// 递增登录尝试次数
		attempts, locked, lockedUntil, err := store.IncrementLoginAttempts(ctx, user.ID, 5, 30*time.Minute)
		require.NoError(t, err)
		assert.Equal(t, 1, attempts)
		assert.False(t, locked)
		assert.Nil(t, lockedUntil)
	})

	t.Run("达到最大次数触发锁定", func(t *testing.T) {
		user := newTestUser("lock@example.com")
		require.NoError(t, store.Create(ctx, user))

		// 递增4次（未达到锁定阈值）
		for i := 0; i < 4; i++ {
			_, _, _, err := store.IncrementLoginAttempts(ctx, user.ID, 5, 30*time.Minute)
			require.NoError(t, err)
		}

		// 第5次应该触发锁定
		attempts, locked, lockedUntil, err := store.IncrementLoginAttempts(ctx, user.ID, 5, 30*time.Minute)
		require.NoError(t, err)
		assert.Equal(t, 5, attempts)
		assert.True(t, locked)
		assert.NotNil(t, lockedUntil)
	})

	t.Run("用户不存在返回错误", func(t *testing.T) {
		_, _, _, err := store.IncrementLoginAttempts(ctx, "nonexistent-id", 5, 30*time.Minute)
		assert.Error(t, err)
	})
}

func TestStore_ResetLoginAttempts(t *testing.T) {
	store, db := setupTestStore(t)
	defer db.Close()
	defer cleanupTestData(t, db)
	ctx := context.Background()

	t.Run("重置登录尝试次数", func(t *testing.T) {
		user := newTestUser("reset@example.com")
		require.NoError(t, store.Create(ctx, user))

		// 先设置登录尝试次数
		lockedUntil := time.Now().Add(30 * time.Minute)
		require.NoError(t, store.UpdateLoginAttempts(ctx, user.ID, 5, &lockedUntil))

		// 重置
		err := store.ResetLoginAttempts(ctx, user.ID)
		require.NoError(t, err)

		// 验证已重置
		retrieved, err := store.GetByID(ctx, user.ID)
		require.NoError(t, err)
		assert.Equal(t, 0, retrieved.LoginAttempts)
		assert.Nil(t, retrieved.LockedUntil)
	})

	t.Run("用户不存在返回错误", func(t *testing.T) {
		err := store.ResetLoginAttempts(ctx, "nonexistent-id")
		assert.Error(t, err)
	})
}

func TestStore_UnlockExpiredAccount(t *testing.T) {
	store, db := setupTestStore(t)
	defer db.Close()
	defer cleanupTestData(t, db)
	ctx := context.Background()

	t.Run("解锁过期账户", func(t *testing.T) {
		user := newTestUser("unlock@example.com")
		user.Status = model.UserStatusLocked
		// 使用UTC时间避免时区问题
		pastTime := time.Now().UTC().Add(-2 * time.Hour)
		user.LockedUntil = &pastTime
		require.NoError(t, store.Create(ctx, user))

		// 解锁过期账户
		err := store.UnlockExpiredAccount(ctx, user.ID)
		require.NoError(t, err)

		// 验证已解锁
		retrieved, err := store.GetByID(ctx, user.ID)
		require.NoError(t, err)
		assert.Equal(t, model.UserStatusActive, retrieved.Status)
		assert.Equal(t, 0, retrieved.LoginAttempts)
	})

	t.Run("未过期账户不解锁", func(t *testing.T) {
		user := newTestUser("notexpired@example.com")
		user.Status = model.UserStatusLocked
		futureTime := time.Now().Add(1 * time.Hour)
		user.LockedUntil = &futureTime
		require.NoError(t, store.Create(ctx, user))

		// 尝试解锁未过期账户应该返回ErrNotFound
		err := store.UnlockExpiredAccount(ctx, user.ID)
		assert.Error(t, err)
	})

	t.Run("非锁定账户不解锁", func(t *testing.T) {
		user := newTestUser("active@example.com")
		user.Status = model.UserStatusActive
		require.NoError(t, store.Create(ctx, user))

		// 尝试解锁活跃账户应该返回ErrNotFound
		err := store.UnlockExpiredAccount(ctx, user.ID)
		assert.Error(t, err)
	})

	t.Run("用户不存在返回错误", func(t *testing.T) {
		err := store.UnlockExpiredAccount(ctx, "nonexistent-id")
		assert.Error(t, err)
	})
}

func TestStore_DeleteUser(t *testing.T) {
	store, db := setupTestStore(t)
	defer db.Close()
	defer cleanupTestData(t, db)
	ctx := context.Background()

	user := newTestUser("delete@example.com")
	require.NoError(t, store.Create(ctx, user))

	t.Run("删除用户", func(t *testing.T) {
		require.NoError(t, store.Delete(ctx, user.ID))
		_, err := store.GetByID(ctx, user.ID)
		assert.Error(t, err)
	})
}

func TestStore_ListUsers(t *testing.T) {
	store, db := setupTestStore(t)
	defer db.Close()
	defer cleanupTestData(t, db)
	ctx := context.Background()

	for i := 0; i < 5; i++ {
		require.NoError(t, store.Create(ctx, newTestUser(fmt.Sprintf("list%d@example.com", i))))
	}

	t.Run("列出所有用户", func(t *testing.T) {
		users, total, err := store.ListUsers(ctx, 0, 10)
		require.NoError(t, err)
		assert.GreaterOrEqual(t, total, 5)
		assert.GreaterOrEqual(t, len(users), 5)
	})

	t.Run("分页", func(t *testing.T) {
		users, _, err := store.ListUsers(ctx, 0, 2)
		require.NoError(t, err)
		assert.Equal(t, 2, len(users))
	})
}

// ============================================================================
// Token存储测试
// ============================================================================

func TestStore_TokenOperations(t *testing.T) {
	store, db := setupTestStore(t)
	defer db.Close()
	defer cleanupTestData(t, db)
	ctx := context.Background()

	user := newTestUser("token@example.com")
	require.NoError(t, store.Create(ctx, user))

	// 创建测试客户端（Token表有client_id外键）
	testClient := &model.Client{
		ID:           uuid.New().String(),
		ClientID:     "test-token-client",
		ClientSecret: "secret",
		Name:         "Token Test Client",
		RedirectURIs: []string{"http://localhost"},
		GrantTypes:   []string{"authorization_code"},
		Scopes:       []string{"openid"},
		CreatedAt:    time.Now(),
	}
	_ = store.CreateClient(ctx, testClient)

	t.Run("存储和获取Token", func(t *testing.T) {
		token := &model.Token{
			ID:           uuid.New().String(),
			AccessToken:  "test-access-" + uuid.New().String(),
			RefreshToken: "test-refresh-" + uuid.New().String(),
			UserID:       user.ID,
			ClientID:     ptrTo("test-token-client"),
			Scopes:       []string{"openid", "profile"},
			ExpiresAt:    time.Now().Add(1 * time.Hour),
			CreatedAt:    time.Now(),
		}
		require.NoError(t, store.StoreToken(ctx, token))

		retrieved, err := store.GetTokenByAccessToken(ctx, token.AccessToken)
		require.NoError(t, err)
		assert.Equal(t, token.ID, retrieved.ID)

		retrieved, err = store.GetTokenByRefreshToken(ctx, token.RefreshToken)
		require.NoError(t, err)
		assert.Equal(t, token.ID, retrieved.ID)
	})

	t.Run("撤销Token", func(t *testing.T) {
		token := &model.Token{
			ID:           uuid.New().String(),
			AccessToken:  "test-access-revoke-" + uuid.New().String(),
			RefreshToken: "test-refresh-revoke-" + uuid.New().String(),
			UserID:       user.ID,
			ClientID:     ptrTo("test-token-client"),
			Scopes:       []string{"openid"},
			ExpiresAt:    time.Now().Add(1 * time.Hour),
			CreatedAt:    time.Now(),
		}
		require.NoError(t, store.StoreToken(ctx, token))
		require.NoError(t, store.RevokeToken(ctx, token.AccessToken))

		retrieved, err := store.GetTokenByAccessToken(ctx, token.AccessToken)
		require.NoError(t, err)
		assert.NotNil(t, retrieved.RevokedAt)
	})

	t.Run("撤销用户所有Token", func(t *testing.T) {
		for i := 0; i < 3; i++ {
			token := &model.Token{
				ID:           uuid.New().String(),
				AccessToken:  fmt.Sprintf("test-all-%d-%s", i, uuid.New().String()),
				RefreshToken: fmt.Sprintf("test-all-r-%d-%s", i, uuid.New().String()),
				UserID:       user.ID,
				ClientID:     ptrTo("test-token-client"),
				Scopes:       []string{"openid"},
				ExpiresAt:    time.Now().Add(1 * time.Hour),
				CreatedAt:    time.Now(),
			}
			require.NoError(t, store.StoreToken(ctx, token))
		}
		assert.NoError(t, store.RevokeAllUserTokens(ctx, user.ID))
	})
}

// ============================================================================
// 验证令牌测试
// ============================================================================

func TestStore_VerificationTokens(t *testing.T) {
	store, db := setupTestStore(t)
	defer db.Close()
	defer cleanupTestData(t, db)
	ctx := context.Background()

	user := newTestUser("verify@example.com")
	require.NoError(t, store.Create(ctx, user))

	t.Run("验证令牌", func(t *testing.T) {
		token := "vtoken-" + uuid.New().String()
		require.NoError(t, store.StoreVerificationToken(ctx, user.ID, token, time.Now().Add(24*time.Hour)))

		retrieved, err := store.GetVerificationToken(ctx, user.ID)
		require.NoError(t, err)
		assert.Equal(t, token, retrieved.Token)

		assert.NoError(t, store.DeleteVerificationToken(ctx, user.ID))
	})

	t.Run("重置令牌", func(t *testing.T) {
		token := "rtoken-" + uuid.New().String()
		require.NoError(t, store.StoreResetToken(ctx, user.ID, token, time.Now().Add(1*time.Hour)))

		retrieved, err := store.GetResetToken(ctx, user.ID)
		require.NoError(t, err)
		assert.Equal(t, token, retrieved.Token)

		assert.NoError(t, store.DeleteResetToken(ctx, user.ID))
	})
}

// ============================================================================
// 授权码测试
// ============================================================================

func TestStore_AuthorizationCode(t *testing.T) {
	store, db := setupTestStore(t)
	defer db.Close()
	defer cleanupTestData(t, db)
	ctx := context.Background()

	user := newTestUser("authcode@example.com")
	require.NoError(t, store.Create(ctx, user))

	// 创建测试客户端（使用有效UUID）
	testClientID := uuid.New().String()
	testClient := &model.Client{
		ID:           testClientID,
		ClientID:     "test-authcode-client",
		ClientSecret: "test-secret",
		Name:         "Test AuthCode Client",
		RedirectURIs: []string{"http://localhost/callback"},
		GrantTypes:   []string{"authorization_code"},
		Scopes:       []string{"openid"},
		CreatedAt:    time.Now(),
	}
	err := store.CreateClient(ctx, testClient)
	if err != nil {
		t.Logf("CreateClient warning: %v (可能已存在)", err)
	}

	t.Run("创建和获取授权码", func(t *testing.T) {
		code := &model.AuthorizationCode{
			Code:        "test-ac-" + uuid.New().String(),
			ClientID:    "test-authcode-client",
			UserID:      user.ID,
			RedirectURI: "http://localhost/callback",
			Scopes:      []string{"openid"},
			ExpiresAt:   time.Now().Add(10 * time.Minute),
			CreatedAt:   time.Now(),
		}
		require.NoError(t, store.StoreAuthorizationCode(ctx, code))

		retrieved, err := store.GetAuthorizationCode(ctx, code.Code)
		require.NoError(t, err)
		assert.Equal(t, code.UserID, retrieved.UserID)
		assert.Equal(t, code.ClientID, retrieved.ClientID)
	})

	t.Run("标记授权码已使用", func(t *testing.T) {
		code := &model.AuthorizationCode{
			Code:        "test-ac-used-" + uuid.New().String(),
			ClientID:    "test-authcode-client",
			UserID:      user.ID,
			RedirectURI: "http://localhost/callback",
			Scopes:      []string{"openid"},
			ExpiresAt:   time.Now().Add(10 * time.Minute),
			CreatedAt:   time.Now(),
		}
		require.NoError(t, store.StoreAuthorizationCode(ctx, code))

		now := time.Now()
		code.UsedAt = &now
		assert.NoError(t, store.UpdateAuthorizationCode(ctx, code))
	})
}

// ============================================================================
// 审计日志测试
// ============================================================================

func TestStore_AuditLog(t *testing.T) {
	store, db := setupTestStore(t)
	defer db.Close()
	ctx := context.Background()

	t.Run("记录审计日志", func(t *testing.T) {
		log := &model.AuditLog{
			ID:        uuid.New().String(),
			EventType: "user.login",
			UserID:    "test-user-audit",
			IPAddress: "192.168.1.1",
			UserAgent: "test-agent",
			Details:   `{"email":"test@example.com"}`,
			Success:   true,
			Timestamp: time.Now(),
		}
		assert.NoError(t, store.StoreAuditLog(ctx, log))
	})

	t.Run("列出审计日志", func(t *testing.T) {
		logs, total, err := store.ListAuditLogs(ctx, "test-user-audit", "", 0, 10)
		require.NoError(t, err)
		assert.GreaterOrEqual(t, total, 1)
		assert.GreaterOrEqual(t, len(logs), 1)
	})

	t.Run("按事件类型过滤", func(t *testing.T) {
		logs, _, err := store.ListAuditLogs(ctx, "", "user.login", 0, 10)
		require.NoError(t, err)
		for _, log := range logs {
			assert.Equal(t, "user.login", log.EventType)
		}
	})
}

// ============================================================================
// 连接测试
// ============================================================================

func TestStore_Ping(t *testing.T) {
	store, db := setupTestStore(t)
	defer db.Close()
	assert.NoError(t, store.Ping(context.Background()))
}

func TestStore_Close(t *testing.T) {
	db := getTestDB(t)
	assert.NoError(t, postgres.New(db).Close())
}

// ============================================================================
// Client查询测试
// ============================================================================

func TestStore_GetByClientID(t *testing.T) {
	store, db := setupTestStore(t)
	defer db.Close()
	ctx := context.Background()

	// 创建测试客户端
	client := &model.Client{
		ID:           uuid.New().String(),
		ClientID:     "test-getclient-" + uuid.New().String()[:8],
		ClientSecret: "secret",
		Name:         "GetByClientID Test",
		RedirectURIs: []string{"http://localhost/callback"},
		GrantTypes:   []string{"authorization_code"},
		Scopes:       []string{"openid"},
		CreatedAt:    time.Now(),
	}
	require.NoError(t, store.CreateClient(ctx, client))

	t.Run("通过clientID获取客户端", func(t *testing.T) {
		retrieved, err := store.GetByClientID(ctx, client.ClientID)
		require.NoError(t, err)
		assert.Equal(t, client.ClientID, retrieved.ClientID)
		assert.Equal(t, client.Name, retrieved.Name)
		assert.Contains(t, retrieved.RedirectURIs, "http://localhost/callback")
	})

	t.Run("客户端不存在", func(t *testing.T) {
		_, err := store.GetByClientID(ctx, "nonexistent-client-id")
		assert.Error(t, err)
	})
}

func TestStore_ValidateRedirectURI(t *testing.T) {
	store, db := setupTestStore(t)
	defer db.Close()
	ctx := context.Background()

	clientID := "test-validate-uri-" + uuid.New().String()[:8]
	client := &model.Client{
		ID:           uuid.New().String(),
		ClientID:     clientID,
		ClientSecret: "secret",
		Name:         "ValidateURI Test",
		RedirectURIs: []string{"http://localhost/callback", "https://app.example.com/callback"},
		GrantTypes:   []string{"authorization_code"},
		Scopes:       []string{"openid"},
		CreatedAt:    time.Now(),
	}
	require.NoError(t, store.CreateClient(ctx, client))

	t.Run("有效重定向URI", func(t *testing.T) {
		assert.True(t, store.ValidateRedirectURI(ctx, clientID, "http://localhost/callback"))
		assert.True(t, store.ValidateRedirectURI(ctx, clientID, "https://app.example.com/callback"))
	})

	t.Run("无效重定向URI", func(t *testing.T) {
		assert.False(t, store.ValidateRedirectURI(ctx, clientID, "http://evil.com/callback"))
		assert.False(t, store.ValidateRedirectURI(ctx, clientID, ""))
	})

	t.Run("不存在的客户端", func(t *testing.T) {
		assert.False(t, store.ValidateRedirectURI(ctx, "nonexistent", "http://localhost"))
	})
}

// ============================================================================
// Constructor 测试
// ============================================================================

func TestStore_NewFromURL(t *testing.T) {
	t.Run("有效URL", func(t *testing.T) {
		dbURL := os.Getenv("DATABASE_URL")
		if dbURL == "" {
			t.Skip("跳过：未设置DATABASE_URL")
		}
		store, err := postgres.NewFromURL(dbURL)
		require.NoError(t, err)
		assert.NotNil(t, store)
		store.Close()
	})

	t.Run("无效URL格式", func(t *testing.T) {
		_, err := postgres.NewFromURL("://invalid-url")
		assert.Error(t, err)
	})
}

func TestStore_NewFromConfig(t *testing.T) {
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		t.Skip("跳过：未设置DATABASE_URL")
	}

	store, err := postgres.NewFromConfig(dbURL, 10, 5, 5*time.Minute, 30*time.Second)
	require.NoError(t, err)
	assert.NotNil(t, store)
	store.Close()
}

// ============================================================================
// MarkAuthorizationCodeUsed 测试
// ============================================================================

func TestStore_MarkAuthorizationCodeUsed(t *testing.T) {
	store, db := setupTestStore(t)
	defer db.Close()
	defer cleanupTestData(t, db)
	ctx := context.Background()

	user := newTestUser("markused@example.com")
	require.NoError(t, store.Create(ctx, user))

	// 创建测试客户端
	client := &model.Client{
		ID:           uuid.New().String(),
		ClientID:     "test-markused-client",
		ClientSecret: "secret",
		Name:         "MarkUsed Test",
		RedirectURIs: []string{"http://localhost"},
		GrantTypes:   []string{"authorization_code"},
		Scopes:       []string{"openid"},
		CreatedAt:    time.Now(),
	}
	_ = store.CreateClient(ctx, client)

	t.Run("标记授权码已使用", func(t *testing.T) {
		code := &model.AuthorizationCode{
			Code:        "test-markused-" + uuid.New().String(),
			ClientID:    "test-markused-client",
			UserID:      user.ID,
			RedirectURI: "http://localhost",
			Scopes:      []string{"openid"},
			ExpiresAt:   time.Now().Add(10 * time.Minute),
			CreatedAt:   time.Now(),
		}
		require.NoError(t, store.StoreAuthorizationCode(ctx, code))

		err := store.MarkAuthorizationCodeUsed(ctx, code.Code)
		assert.NoError(t, err)

		// 验证已标记
		retrieved, err := store.GetAuthorizationCode(ctx, code.Code)
		require.NoError(t, err)
		assert.NotNil(t, retrieved.UsedAt)
	})
}

// ============================================================================
// 分页边界条件测试
// ============================================================================

func TestStore_ListUsers_Pagination(t *testing.T) {
	store, db := setupTestStore(t)
	defer db.Close()
	defer cleanupTestData(t, db)
	ctx := context.Background()

	// 创建测试用户
	for i := 0; i < 5; i++ {
		user := newTestUser(fmt.Sprintf("pagination%d@example.com", i))
		require.NoError(t, store.Create(ctx, user))
	}

	t.Run("第一页", func(t *testing.T) {
		users, total, err := store.ListUsers(ctx, 0, 2)
		assert.NoError(t, err)
		assert.GreaterOrEqual(t, total, 5)
		assert.LessOrEqual(t, len(users), 2)
	})

	t.Run("第二页", func(t *testing.T) {
		users, total, err := store.ListUsers(ctx, 2, 2)
		assert.NoError(t, err)
		assert.GreaterOrEqual(t, total, 5)
		assert.LessOrEqual(t, len(users), 2)
	})

	t.Run("超出范围", func(t *testing.T) {
		users, total, err := store.ListUsers(ctx, 100, 10)
		assert.NoError(t, err)
		assert.GreaterOrEqual(t, total, 5)
		assert.Empty(t, users)
	})
}

// ============================================================================
// 审计日志过滤测试
// ============================================================================

func TestStore_ListAuditLogs_Filter(t *testing.T) {
	store, db := setupTestStore(t)
	defer db.Close()
	defer cleanupTestData(t, db)
	ctx := context.Background()

	// 创建测试用户
	user := newTestUser("auditfilter@example.com")
	require.NoError(t, store.Create(ctx, user))

	// 创建不同类型的审计日志
	eventTypes := []string{"login", "logout", "register"}
	for _, eventType := range eventTypes {
		log := &model.AuditLog{
			ID:        "test-audit-" + uuid.New().String(),
			EventType: eventType,
			UserID:    user.ID,
			Details:   "{}",
			Success:   true,
			Timestamp: time.Now(),
		}
		require.NoError(t, store.StoreAuditLog(ctx, log))
	}

	t.Run("按用户ID过滤", func(t *testing.T) {
		logs, total, err := store.ListAuditLogs(ctx, user.ID, "", 0, 10)
		assert.NoError(t, err)
		assert.GreaterOrEqual(t, total, 3)
		for _, log := range logs {
			assert.Equal(t, user.ID, log.UserID)
		}
	})

	t.Run("按事件类型过滤", func(t *testing.T) {
		logs, total, err := store.ListAuditLogs(ctx, "", "login", 0, 10)
		assert.NoError(t, err)
		assert.GreaterOrEqual(t, total, 1)
		for _, log := range logs {
			assert.Equal(t, "login", log.EventType)
		}
	})

	t.Run("联合过滤", func(t *testing.T) {
		logs, total, err := store.ListAuditLogs(ctx, user.ID, "logout", 0, 10)
		assert.NoError(t, err)
		assert.GreaterOrEqual(t, total, 1)
		for _, log := range logs {
			assert.Equal(t, user.ID, log.UserID)
			assert.Equal(t, "logout", log.EventType)
		}
	})
}

// ============================================================================
// 过期数据清理测试
// ============================================================================

func TestStore_CleanupExpired(t *testing.T) {
	store, db := setupTestStore(t)
	defer db.Close()
	defer cleanupTestData(t, db)
	ctx := context.Background()

	// 创建测试用户
	user := newTestUser("cleanup@example.com")
	require.NoError(t, store.Create(ctx, user))

	// 创建测试客户端
	client := &model.Client{
		ID:           uuid.New().String(),
		ClientID:     "test-cleanup-client",
		ClientSecret: "secret",
		Name:         "Cleanup Test",
		RedirectURIs: []string{"http://localhost"},
		GrantTypes:   []string{"authorization_code"},
		Scopes:       []string{"openid"},
		CreatedAt:    time.Now(),
	}
	_ = store.CreateClient(ctx, client)

	// 创建过期的Token
	expiredToken := &model.Token{
		ID:           uuid.New().String(),
		AccessToken:  "test-cleanup-access-" + uuid.New().String(),
		RefreshToken: "test-cleanup-refresh-" + uuid.New().String(),
		UserID:       user.ID,
		ClientID:     ptrTo("test-cleanup-client"),
		Scopes:       []string{"openid"},
		ExpiresAt:    time.Now().Add(-1 * time.Hour),
		CreatedAt:    time.Now().Add(-2 * time.Hour),
	}
	require.NoError(t, store.StoreToken(ctx, expiredToken))

	// 创建过期的授权码
	expiredCode := &model.AuthorizationCode{
		Code:        uuid.New().String(),
		ClientID:    "test-cleanup-client",
		UserID:      user.ID,
		RedirectURI: "http://localhost",
		Scopes:      []string{"openid"},
		ExpiresAt:   time.Now().Add(-1 * time.Hour), // 已过期
		CreatedAt:   time.Now().Add(-2 * time.Hour),
	}
	require.NoError(t, store.StoreAuthorizationCode(ctx, expiredCode))

	t.Run("清理过期数据", func(t *testing.T) {
		err := store.CleanupExpired(ctx)
		assert.NoError(t, err)

		// 验证过期Token已被删除
		_, err = store.GetTokenByAccessToken(ctx, expiredToken.AccessToken)
		assert.Error(t, err)

		// 验证过期授权码已被删除
		_, err = store.GetAuthorizationCode(ctx, expiredCode.Code)
		assert.Error(t, err)
	})
}

// ============================================================================
// 字段白名单验证测试
// ============================================================================

func TestStore_GetUserByField_InvalidField(t *testing.T) {
	store, db := setupTestStore(t)
	defer db.Close()
	ctx := context.Background()

	t.Run("无效字段名", func(t *testing.T) {
		// 由于getUserByField是私有方法，我们通过公共方法间接测试
		// 验证GetByID和GetByEmail使用有效的字段名
		_, err := store.GetByID(ctx, "nonexistent-id")
		assert.Error(t, err) // 应该返回ErrNotFound而不是字段名错误

		_, err = store.GetByEmail(ctx, "nonexistent@example.com")
		assert.Error(t, err) // 应该返回ErrNotFound而不是字段名错误
	})
}

// ============================================================================
// 密钥版本测试
// ============================================================================

func TestStore_KeyOperations(t *testing.T) {
	store, db := setupTestStore(t)
	defer db.Close()
	defer cleanupTestData(t, db)
	ctx := context.Background()

	t.Run("存储密钥", func(t *testing.T) {
		key := &model.KeyVersion{
			ID:         "test-key-" + uuid.New().String(),
			PublicKey:  []byte("-----BEGIN PUBLIC KEY-----\ntest\n-----END PUBLIC KEY-----"),
			PrivateKey: []byte("-----BEGIN PRIVATE KEY-----\ntest\n-----END PRIVATE KEY-----"),
			Status:     model.KeyStatusActive,
			CreatedAt:  time.Now(),
		}
		err := store.StoreKey(ctx, key)
		assert.NoError(t, err)
	})

	t.Run("获取活跃密钥", func(t *testing.T) {
		// 先存储一个活跃密钥
		keyID := "test-active-key-" + uuid.New().String()
		key := &model.KeyVersion{
			ID:         keyID,
			PublicKey:  []byte("-----BEGIN PUBLIC KEY-----\nactive\n-----END PUBLIC KEY-----"),
			PrivateKey: []byte("-----BEGIN PRIVATE KEY-----\nactive\n-----END PRIVATE KEY-----"),
			Status:     model.KeyStatusActive,
			CreatedAt:  time.Now(),
		}
		require.NoError(t, store.StoreKey(ctx, key))

		// 获取活跃密钥
		activeKey, err := store.GetActiveKey(ctx)
		require.NoError(t, err)
		assert.Equal(t, model.KeyStatusActive, activeKey.Status)
	})

	t.Run("获取不存在的活跃密钥", func(t *testing.T) {
		// 清除所有密钥
		_, _ = db.ExecContext(ctx, "DELETE FROM key_versions WHERE id LIKE 'test-%'")

		_, err := store.GetActiveKey(ctx)
		assert.Error(t, err)
	})

	t.Run("按ID获取密钥", func(t *testing.T) {
		keyID := "test-byid-key-" + uuid.New().String()
		key := &model.KeyVersion{
			ID:         keyID,
			PublicKey:  []byte("-----BEGIN PUBLIC KEY-----\nbyid\n-----END PUBLIC KEY-----"),
			PrivateKey: []byte("-----BEGIN PRIVATE KEY-----\nbyid\n-----END PRIVATE KEY-----"),
			Status:     model.KeyStatusActive,
			CreatedAt:  time.Now(),
		}
		require.NoError(t, store.StoreKey(ctx, key))

		// 按ID获取
		retrievedKey, err := store.GetKeyByID(ctx, keyID)
		require.NoError(t, err)
		assert.Equal(t, keyID, retrievedKey.ID)
	})

	t.Run("获取不存在的密钥", func(t *testing.T) {
		_, err := store.GetKeyByID(ctx, "nonexistent-key")
		assert.Error(t, err)
	})

	t.Run("列出活跃密钥", func(t *testing.T) {
		// 存储活跃和弃用密钥
		activeKey := &model.KeyVersion{
			ID:        "test-list-active-" + uuid.New().String(),
			PublicKey: []byte("active"),
			Status:    model.KeyStatusActive,
			CreatedAt: time.Now(),
		}
		deprecatedKey := &model.KeyVersion{
			ID:        "test-list-deprecated-" + uuid.New().String(),
			PublicKey: []byte("deprecated"),
			Status:    model.KeyStatusDeprecated,
			CreatedAt: time.Now(),
		}
		require.NoError(t, store.StoreKey(ctx, activeKey))
		require.NoError(t, store.StoreKey(ctx, deprecatedKey))

		// 列出活跃密钥
		keys, err := store.ListActiveKeys(ctx)
		require.NoError(t, err)
		// 应该包含活跃和弃用密钥（非撤销）
		assert.GreaterOrEqual(t, len(keys), 1)
	})

	t.Run("列出所有密钥", func(t *testing.T) {
		keys, err := store.ListAllKeys(ctx)
		require.NoError(t, err)
		assert.GreaterOrEqual(t, len(keys), 1)
	})

	t.Run("弃用密钥", func(t *testing.T) {
		keyID := "test-deprecate-" + uuid.New().String()
		key := &model.KeyVersion{
			ID:        keyID,
			PublicKey: []byte("deprecate"),
			Status:    model.KeyStatusActive,
			CreatedAt: time.Now(),
		}
		require.NoError(t, store.StoreKey(ctx, key))

		// 弃用密钥
		expiresAt := time.Now().Add(24 * time.Hour)
		err := store.DeprecateKey(ctx, keyID, expiresAt)
		require.NoError(t, err)

		// 验证状态
		retrievedKey, err := store.GetKeyByID(ctx, keyID)
		require.NoError(t, err)
		assert.Equal(t, model.KeyStatusDeprecated, retrievedKey.Status)
	})

	t.Run("弃用不存在的密钥", func(t *testing.T) {
		err := store.DeprecateKey(ctx, "nonexistent-key", time.Now())
		assert.Error(t, err)
	})

	t.Run("撤销密钥", func(t *testing.T) {
		keyID := "test-revoke-" + uuid.New().String()
		key := &model.KeyVersion{
			ID:        keyID,
			PublicKey: []byte("revoke"),
			Status:    model.KeyStatusDeprecated,
			CreatedAt: time.Now(),
		}
		require.NoError(t, store.StoreKey(ctx, key))

		// 撤销密钥
		err := store.RevokeKey(ctx, keyID)
		require.NoError(t, err)

		// 验证状态
		retrievedKey, err := store.GetKeyByID(ctx, keyID)
		require.NoError(t, err)
		assert.Equal(t, model.KeyStatusRevoked, retrievedKey.Status)
	})

	t.Run("撤销不存在的密钥", func(t *testing.T) {
		err := store.RevokeKey(ctx, "nonexistent-key")
		assert.Error(t, err)
	})

	t.Run("删除密钥", func(t *testing.T) {
		keyID := "test-delete-" + uuid.New().String()
		key := &model.KeyVersion{
			ID:        keyID,
			PublicKey: []byte("delete"),
			Status:    model.KeyStatusRevoked,
			CreatedAt: time.Now(),
		}
		require.NoError(t, store.StoreKey(ctx, key))

		// 删除密钥
		err := store.DeleteKey(ctx, keyID)
		require.NoError(t, err)

		// 验证已删除
		_, err = store.GetKeyByID(ctx, keyID)
		assert.Error(t, err)
	})

	t.Run("删除不存在的密钥", func(t *testing.T) {
		err := store.DeleteKey(ctx, "nonexistent-key")
		assert.Error(t, err)
	})
}
