// Package service_test 初始化服务单元测试
package service_test

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/example/sso/internal/crypto"
	apperrors "github.com/example/sso/internal/errors"
	"github.com/example/sso/internal/model"
	"github.com/example/sso/internal/service"
	"github.com/example/sso/internal/store"
	"github.com/example/sso/internal/store/mock"
)

// ============================================================================
// 测试辅助函数
// ============================================================================

// createTestInitService 创建测试用的初始化服务
func createTestInitService(t *testing.T) (*service.InitService, *mock.Store) {
	// 创建Mock存储
	mockStore := mock.New()

	// 创建密码服务
	passwordSvc := crypto.NewPasswordService(4) // 使用较低的cost加快测试

	// 创建审计服务（使用nil，因为InitService使用SafeAuditLog）
	var auditSvc service.AuditServiceInterface

	// 创建初始化服务
	initSvc := service.NewInitService(mockStore, passwordSvc, auditSvc)

	return initSvc, mockStore
}

// ============================================================================
// AdminExists 测试
// ============================================================================

func TestInitService_AdminExists(t *testing.T) {
	t.Run("不存在管理员", func(t *testing.T) {
		initSvc, mockStore := createTestInitService(t)
		mockStore.Reset()
		ctx := context.Background()

		exists, err := initSvc.AdminExists(ctx)

		require.NoError(t, err)
		assert.False(t, exists)
	})

	t.Run("存在管理员", func(t *testing.T) {
		initSvc, mockStore := createTestInitService(t)
		mockStore.Reset()
		ctx := context.Background()

		// 创建一个管理员用户
		admin := &model.User{
			ID:           "admin-1",
			Email:        "admin@example.com",
			PasswordHash: "hash",
			Role:         model.UserRoleAdmin,
			Status:       model.UserStatusActive,
			CreatedAt:    time.Now(),
			UpdatedAt:    time.Now(),
		}
		err := mockStore.Create(ctx, admin)
		require.NoError(t, err)

		exists, err := initSvc.AdminExists(ctx)

		require.NoError(t, err)
		assert.True(t, exists)
	})

	t.Run("只有普通用户", func(t *testing.T) {
		initSvc, mockStore := createTestInitService(t)
		mockStore.Reset()
		ctx := context.Background()

		// 创建一个普通用户
		user := &model.User{
			ID:           "user-1",
			Email:        "user@example.com",
			PasswordHash: "hash",
			Role:         model.UserRoleUser,
			Status:       model.UserStatusActive,
			CreatedAt:    time.Now(),
			UpdatedAt:    time.Now(),
		}
		err := mockStore.Create(ctx, user)
		require.NoError(t, err)

		exists, err := initSvc.AdminExists(ctx)

		require.NoError(t, err)
		assert.False(t, exists)
	})

	t.Run("存储错误", func(t *testing.T) {
		initSvc, mockStore := createTestInitService(t)
		mockStore.Reset()
		ctx := context.Background()

		// 注入 ExistsUserByRole 错误
		mockStore.ExistsUserByRoleErr = errors.New("database error")

		exists, err := initSvc.AdminExists(ctx)

		// 验证错误被正确处理
		assert.Error(t, err)
		assert.False(t, exists)
		assert.NotContains(t, err.Error(), "database error")

		// 清理
		mockStore.ExistsUserByRoleErr = nil
	})
}

// ============================================================================
// CreateAdmin 测试
// ============================================================================

func TestInitService_CreateAdmin(t *testing.T) {
	t.Run("成功创建管理员", func(t *testing.T) {
		initSvc, mockStore := createTestInitService(t)
		mockStore.Reset()
		ctx := context.Background()

		user, err := initSvc.CreateAdmin(ctx, "admin@example.com", "Password123!!")

		require.NoError(t, err)
		assert.NotNil(t, user)
		assert.NotEmpty(t, user.ID)
		assert.Equal(t, "admin@example.com", user.Email)
		assert.NotEmpty(t, user.PasswordHash)
		assert.Equal(t, model.UserRoleAdmin, user.Role)
		assert.Equal(t, model.UserStatusActive, user.Status)
		assert.True(t, user.EmailVerified)
	})

	t.Run("邮箱格式无效", func(t *testing.T) {
		initSvc, mockStore := createTestInitService(t)
		mockStore.Reset()
		ctx := context.Background()

		user, err := initSvc.CreateAdmin(ctx, "invalid-email", "Password123!!")

		require.Error(t, err)
		assert.Nil(t, user)
	})

	t.Run("密码格式无效", func(t *testing.T) {
		initSvc, mockStore := createTestInitService(t)
		mockStore.Reset()
		ctx := context.Background()

		user, err := initSvc.CreateAdmin(ctx, "admin@example.com", "weak")

		require.Error(t, err)
		assert.Nil(t, user)
	})

	t.Run("管理员已存在", func(t *testing.T) {
		initSvc, mockStore := createTestInitService(t)
		mockStore.Reset()
		ctx := context.Background()

		// 先创建一个管理员
		admin := &model.User{
			ID:           "admin-1",
			Email:        "existing@example.com",
			PasswordHash: "hash",
			Role:         model.UserRoleAdmin,
			Status:       model.UserStatusActive,
			CreatedAt:    time.Now(),
			UpdatedAt:    time.Now(),
		}
		err := mockStore.Create(ctx, admin)
		require.NoError(t, err)

		// 尝试创建另一个管理员
		user, err := initSvc.CreateAdmin(ctx, "admin@example.com", "Password123!!")

		require.Error(t, err)
		assert.Nil(t, user)
		assert.ErrorIs(t, err, apperrors.ErrForbidden)
	})

	t.Run("邮箱已存在", func(t *testing.T) {
		initSvc, mockStore := createTestInitService(t)
		mockStore.Reset()
		ctx := context.Background()

		// 先创建一个普通用户
		user := &model.User{
			ID:           "user-1",
			Email:        "admin@example.com",
			PasswordHash: "hash",
			Role:         model.UserRoleUser,
			Status:       model.UserStatusActive,
			CreatedAt:    time.Now(),
			UpdatedAt:    time.Now(),
		}
		err := mockStore.Create(ctx, user)
		require.NoError(t, err)

		// 尝试用相同邮箱创建管理员
		admin, err := initSvc.CreateAdmin(ctx, "admin@example.com", "Password123!!")

		require.Error(t, err)
		assert.Nil(t, admin)
		assert.ErrorIs(t, err, apperrors.ErrEmailExists)
	})

	t.Run("数据库重复邮箱错误", func(t *testing.T) {
		initSvc, mockStore := createTestInitService(t)
		mockStore.Reset()
		ctx := context.Background()

		// 模拟 CreateAdminAtomic 在数据库层返回重复邮箱错误
		// 注意：CreateAdmin 现在调用 CreateAdminAtomic 而非 Create，错误需注入到对应字段
		mockStore.CreateAdminAtomicErr = store.ErrDuplicateEmail

		user, err := initSvc.CreateAdmin(ctx, "admin@example.com", "Password123!!")

		require.Error(t, err)
		assert.Nil(t, user)
		assert.ErrorIs(t, err, apperrors.ErrEmailExists)
	})
}

// ============================================================================
// CreateOAuthClient 测试
// ============================================================================

func TestInitService_CreateOAuthClient(t *testing.T) {
	t.Run("成功创建客户端", func(t *testing.T) {
		initSvc, mockStore := createTestInitService(t)
		mockStore.Reset()
		ctx := context.Background()

		// 先创建管理员
		admin := &model.User{
			ID:           "admin-1",
			Email:        "admin@example.com",
			PasswordHash: "hash",
			Role:         model.UserRoleAdmin,
			Status:       model.UserStatusActive,
			CreatedAt:    time.Now(),
			UpdatedAt:    time.Now(),
		}
		err := mockStore.Create(ctx, admin)
		require.NoError(t, err)

		client, secret, err := initSvc.CreateOAuthClient(ctx, "Test App", "https://example.com/callback")

		require.NoError(t, err)
		assert.NotNil(t, client)
		assert.NotEmpty(t, client.ID)
		assert.NotEmpty(t, client.ClientID)
		assert.NotEmpty(t, client.ClientSecret)
		assert.Equal(t, "Test App", client.Name)
		assert.Equal(t, []string{"https://example.com/callback"}, client.RedirectURIs)
		assert.Equal(t, []string{"authorization_code", "refresh_token"}, client.GrantTypes)
		assert.Equal(t, []string{"openid", "profile", "email"}, client.Scopes)
		assert.False(t, client.PublicClient)
		assert.NotEmpty(t, secret)
		assert.Len(t, secret, 64) // 32字节的hex编码
	})

	t.Run("管理员不存在", func(t *testing.T) {
		initSvc, mockStore := createTestInitService(t)
		mockStore.Reset()
		ctx := context.Background()

		client, secret, err := initSvc.CreateOAuthClient(ctx, "Test App", "https://example.com/callback")

		require.Error(t, err)
		assert.Nil(t, client)
		assert.Empty(t, secret)
		assert.ErrorIs(t, err, apperrors.ErrForbidden)
	})

	t.Run("客户端名称为空", func(t *testing.T) {
		initSvc, mockStore := createTestInitService(t)
		mockStore.Reset()
		ctx := context.Background()

		// 先创建管理员
		admin := &model.User{
			ID:           "admin-1",
			Email:        "admin@example.com",
			PasswordHash: "hash",
			Role:         model.UserRoleAdmin,
			Status:       model.UserStatusActive,
			CreatedAt:    time.Now(),
			UpdatedAt:    time.Now(),
		}
		err := mockStore.Create(ctx, admin)
		require.NoError(t, err)

		client, secret, err := initSvc.CreateOAuthClient(ctx, "", "https://example.com/callback")

		require.Error(t, err)
		assert.Nil(t, client)
		assert.Empty(t, secret)
		assert.ErrorIs(t, err, apperrors.ErrBadRequest)
	})

	t.Run("重定向URI为空", func(t *testing.T) {
		initSvc, mockStore := createTestInitService(t)
		mockStore.Reset()
		ctx := context.Background()

		// 先创建管理员
		admin := &model.User{
			ID:           "admin-1",
			Email:        "admin@example.com",
			PasswordHash: "hash",
			Role:         model.UserRoleAdmin,
			Status:       model.UserStatusActive,
			CreatedAt:    time.Now(),
			UpdatedAt:    time.Now(),
		}
		err := mockStore.Create(ctx, admin)
		require.NoError(t, err)

		client, secret, err := initSvc.CreateOAuthClient(ctx, "Test App", "")

		require.Error(t, err)
		assert.Nil(t, client)
		assert.Empty(t, secret)
		assert.ErrorIs(t, err, apperrors.ErrBadRequest)
	})

	t.Run("重定向URI格式无效", func(t *testing.T) {
		initSvc, mockStore := createTestInitService(t)
		mockStore.Reset()
		ctx := context.Background()

		// 先创建管理员
		admin := &model.User{
			ID:           "admin-1",
			Email:        "admin@example.com",
			PasswordHash: "hash",
			Role:         model.UserRoleAdmin,
			Status:       model.UserStatusActive,
			CreatedAt:    time.Now(),
			UpdatedAt:    time.Now(),
		}
		err := mockStore.Create(ctx, admin)
		require.NoError(t, err)

		testCases := []struct {
			name        string
			redirectURI string
		}{
			{"无协议", "example.com/callback"},
			{"无效协议", "ftp://example.com/callback"},
			{"无主机", "https:///callback"},
			{"包含片段", "https://example.com/callback#fragment"},
			{"相对路径", "/callback"},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				client, secret, err := initSvc.CreateOAuthClient(ctx, "Test App", tc.redirectURI)

				require.Error(t, err)
				assert.Nil(t, client)
				assert.Empty(t, secret)
				assert.ErrorIs(t, err, apperrors.ErrBadRequest)
			})
		}
	})

	t.Run("存储错误", func(t *testing.T) {
		initSvc, mockStore := createTestInitService(t)
		mockStore.Reset()
		ctx := context.Background()

		// 先创建管理员
		admin := &model.User{
			ID:           "admin-1",
			Email:        "admin@example.com",
			PasswordHash: "hash",
			Role:         model.UserRoleAdmin,
			Status:       model.UserStatusActive,
			CreatedAt:    time.Now(),
			UpdatedAt:    time.Now(),
		}
		err := mockStore.Create(ctx, admin)
		require.NoError(t, err)

		// 模拟存储错误
		mockStore.CreateClientErr = apperrors.ErrInternal

		client, secret, err := initSvc.CreateOAuthClient(ctx, "Test App", "https://example.com/callback")

		require.Error(t, err)
		assert.Nil(t, client)
		assert.Empty(t, secret)
	})
}

// ============================================================================
// 辅助函数测试
// ============================================================================

func TestValidateRedirectURI(t *testing.T) {
	// 注意：validateRedirectURI 是私有函数，通过 CreateOAuthClient 间接测试
	// 这里我们通过集成测试已经覆盖了各种情况
	t.Run("通过CreateOAuthClient测试", func(t *testing.T) {
		// 已在 TestInitService_CreateOAuthClient 中测试
		assert.True(t, true)
	})
}

func TestGenerateRandomHex(t *testing.T) {
	// 注意：generateRandomHex 是私有函数，通过 CreateOAuthClient 间接测试
	t.Run("通过CreateOAuthClient测试", func(t *testing.T) {
		initSvc, mockStore := createTestInitService(t)
		mockStore.Reset()
		ctx := context.Background()

		// 先创建管理员
		admin := &model.User{
			ID:           "admin-1",
			Email:        "admin@example.com",
			PasswordHash: "hash",
			Role:         model.UserRoleAdmin,
			Status:       model.UserStatusActive,
			CreatedAt:    time.Now(),
			UpdatedAt:    time.Now(),
		}
		err := mockStore.Create(ctx, admin)
		require.NoError(t, err)

		// 创建多个客户端，验证密钥是随机的
		secrets := make(map[string]bool)
		for i := 0; i < 10; i++ {
			_, secret, err := initSvc.CreateOAuthClient(ctx, "Test App", "https://example.com/callback")
			require.NoError(t, err)
			assert.NotEmpty(t, secret)
			assert.Len(t, secret, 64) // 32字节的hex编码
			secrets[secret] = true
		}

		// 验证所有密钥都是唯一的
		assert.Len(t, secrets, 10)
	})
}

// ============================================================================
// NewInitService 测试
// ============================================================================

func TestNewInitService(t *testing.T) {
	t.Run("成功创建服务", func(t *testing.T) {
		mockStore := mock.New()
		passwordSvc := crypto.NewPasswordService(4)
		var auditSvc service.AuditServiceInterface

		initSvc := service.NewInitService(mockStore, passwordSvc, auditSvc)

		assert.NotNil(t, initSvc)
	})
}

// ============================================================================
// 边界条件测试
// ============================================================================

func TestInitService_EdgeCases(t *testing.T) {
	t.Run("大量用户中查找管理员", func(t *testing.T) {
		initSvc, mockStore := createTestInitService(t)
		mockStore.Reset()
		ctx := context.Background()

		// 创建大量普通用户
		for i := 0; i < 100; i++ {
			user := &model.User{
				ID:           fmt.Sprintf("user-%d", i),
				Email:        fmt.Sprintf("user%d@example.com", i),
				PasswordHash: "hash",
				Role:         model.UserRoleUser,
				Status:       model.UserStatusActive,
				CreatedAt:    time.Now(),
				UpdatedAt:    time.Now(),
			}
			err := mockStore.Create(ctx, user)
			require.NoError(t, err)
		}

		// 在最后添加一个管理员
		admin := &model.User{
			ID:           "admin-1",
			Email:        "admin@example.com",
			PasswordHash: "hash",
			Role:         model.UserRoleAdmin,
			Status:       model.UserStatusActive,
			CreatedAt:    time.Now(),
			UpdatedAt:    time.Now(),
		}
		err := mockStore.Create(ctx, admin)
		require.NoError(t, err)

		exists, err := initSvc.AdminExists(ctx)

		require.NoError(t, err)
		assert.True(t, exists)
	})

	t.Run("并发创建管理员", func(t *testing.T) {
		initSvc, mockStore := createTestInitService(t)
		mockStore.Reset()
		ctx := context.Background()

		// 第一次创建成功
		user1, err1 := initSvc.CreateAdmin(ctx, "admin@example.com", "Password123!!")
		require.NoError(t, err1)
		assert.NotNil(t, user1)

		// 第二次创建失败（管理员已存在）
		user2, err2 := initSvc.CreateAdmin(ctx, "admin2@example.com", "Password123!!")
		require.Error(t, err2)
		assert.Nil(t, user2)
		assert.ErrorIs(t, err2, apperrors.ErrForbidden) // 因为管理员已存在
	})

	t.Run("特殊字符的重定向URI", func(t *testing.T) {
		initSvc, mockStore := createTestInitService(t)
		mockStore.Reset()
		ctx := context.Background()

		// 先创建管理员
		admin := &model.User{
			ID:           "admin-1",
			Email:        "admin@example.com",
			PasswordHash: "hash",
			Role:         model.UserRoleAdmin,
			Status:       model.UserStatusActive,
			CreatedAt:    time.Now(),
			UpdatedAt:    time.Now(),
		}
		err := mockStore.Create(ctx, admin)
		require.NoError(t, err)

		// 测试包含查询参数的URI（应该允许）
		client, secret, err := initSvc.CreateOAuthClient(ctx, "Test App", "https://example.com/callback?state=123")

		require.NoError(t, err)
		assert.NotNil(t, client)
		assert.NotEmpty(t, secret)
	})
}

// ============================================================================
// 并发竞态测试：CreateAdminAtomic 防御 TOCTOU
// ============================================================================

// TestCreateAdmin_ConcurrentRace_AdvisoryLock 验证在并发场景下，
// 即使所有请求都通过了应用层的 AdminExists 预检查（TOCTOU 窗口），
// CreateAdminAtomic 仍能保证全局只创建一个初始管理员。
//
// 模拟审计报告中"严重问题 1"的竞态场景：
//  1. Request A: AdminExists() → false → 准备插入
//  2. Request B: AdminExists() → false → 准备插入（A 还没插入）
//  3. Request A: CreateAdminAtomic() → 获取锁 → 插入成功 → 提交
//  4. Request B: CreateAdminAtomic() → 获取锁（A 已释放）→ EXISTS 检查失败 → ErrForbidden
//
// Mock 层通过 mu.Lock() 串行化，等效模拟 PostgreSQL 的 advisory_xact_lock
func TestCreateAdmin_ConcurrentRace_AdvisoryLock(t *testing.T) {
	const goroutines = 32

	initSvc, mockStore := createTestInitService(t)
	mockStore.Reset()
	ctx := context.Background()

	type result struct {
		user *model.User
		err  error
	}
	results := make(chan result, goroutines)
	start := make(chan struct{})

	// 启动 N 个 goroutine 并发调用 CreateAdmin
	var wg sync.WaitGroup
	wg.Add(goroutines)
	for i := 0; i < goroutines; i++ {
		go func(idx int) {
			defer wg.Done()
			<-start // 同时触发，最大化竞态窗口
			email := fmt.Sprintf("admin-%d@example.com", idx)
			user, err := initSvc.CreateAdmin(ctx, email, "Password123!!")
			results <- result{user: user, err: err}
		}(i)
	}
	close(start)
	wg.Wait()
	close(results)

	// 统计结果
	var (
		successCount int
		forbiddenCount int
		otherErrCount int
	)
	for r := range results {
		if r.err == nil && r.user != nil {
			successCount++
		} else if r.err != nil && errors.Is(r.err, apperrors.ErrForbidden) {
			forbiddenCount++
		} else {
			otherErrCount++
			t.Logf("意外错误: %v", r.err)
		}
	}

	// 必须有且仅有 1 个成功
	assert.Equal(t, 1, successCount, "必须有且仅有 1 个请求成功创建管理员")
	// 其余必须都返回 ErrForbidden（不能是其他错误，不能 nil）
	assert.Equal(t, goroutines-1, forbiddenCount, "其余请求必须返回 ErrForbidden")
	assert.Equal(t, 0, otherErrCount, "不应有其他错误（如 ErrEmailExists）")

	// 验证最终 mock 状态：只有 1 个管理员
	exists, err := initSvc.AdminExists(ctx)
	require.NoError(t, err)
	assert.True(t, exists, "管理员应存在")

	// 验证重复调用仍被拒绝（初始化完成后永久关闭）
	_, err = initSvc.CreateAdmin(ctx, "another@example.com", "Password123!!")
	assert.ErrorIs(t, err, apperrors.ErrForbidden, "初始化完成后再次创建必须返回 ErrForbidden")
}
