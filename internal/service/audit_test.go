// Package service_test 审计日志服务单元测试
package service_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/your-org/sso/internal/model"
	"github.com/your-org/sso/internal/service"
	"github.com/your-org/sso/internal/store/mock"
)

// ============================================================================
// 测试辅助函数
// ============================================================================

// createTestAuditService 创建测试用的审计服务
func createTestAuditService() (*service.AuditService, *mock.Store) {
	store := mock.New()
	auditSvc := service.NewAuditService(store)
	return auditSvc, store
}

// waitForAuditLogs 轮询等待异步审计日志写入完成
func waitForAuditLogs(t *testing.T, ctx context.Context, store *mock.Store, userID, eventType string, minCount int) {
	t.Helper()
	require.Eventually(t, func() bool {
		logs, _, err := store.ListAuditLogs(ctx, userID, eventType, 0, 100)
		if err != nil {
			return false
		}
		return len(logs) >= minCount
	}, 2*time.Second, 10*time.Millisecond, "等待审计日志超时: userID=%s eventType=%s", userID, eventType)
}

// ============================================================================
// Log 测试
// ============================================================================

func TestAuditService_Log(t *testing.T) {
	auditSvc, store := createTestAuditService()
	ctx := context.Background()

	t.Run("记录审计日志", func(t *testing.T) {
		store.Reset()

		log := &model.AuditLog{
			EventType: string(model.EventUserLogin),
			UserID:    "test-user-1",
			IPAddress: "127.0.0.1",
			UserAgent: "Mozilla/5.0",
			Details:   `{"email":"test@example.com"}`,
			Success:   true,
			Timestamp: time.Now(),
		}

		auditSvc.Log(ctx, log)

		waitForAuditLogs(t, ctx, store, "test-user-1", string(model.EventUserLogin), 1)
	})

	t.Run("自动生成ID和时间戳", func(t *testing.T) {
		store.Reset()

		log := &model.AuditLog{
			EventType: string(model.EventUserRegister),
			UserID:    "test-user-2",
			Success:   true,
			// 不设置ID和Timestamp
		}

		auditSvc.Log(ctx, log)

		// 验证ID和Timestamp已自动生成
		assert.NotEmpty(t, log.ID)
		assert.False(t, log.Timestamp.IsZero())
	})
}

// ============================================================================
// LogUserRegister 测试
// ============================================================================

func TestAuditService_LogUserRegister(t *testing.T) {
	auditSvc, store := createTestAuditService()
	ctx := context.Background()

	t.Run("记录用户注册成功", func(t *testing.T) {
		store.Reset()

		auditSvc.LogUserRegister(ctx, "user-1", "test@example.com", "192.168.1.1", true)

		waitForAuditLogs(t, ctx, store, "user-1", string(model.EventUserRegister), 1)
	})

	t.Run("记录用户注册失败", func(t *testing.T) {
		store.Reset()

		auditSvc.LogUserRegister(ctx, "", "invalid@example.com", "192.168.1.1", false)

		waitForAuditLogs(t, ctx, store, "", string(model.EventUserRegister), 1)
	})
}

// ============================================================================
// LogUserLogin 测试
// ============================================================================

func TestAuditService_LogUserLogin(t *testing.T) {
	auditSvc, store := createTestAuditService()
	ctx := context.Background()

	t.Run("记录用户登录成功", func(t *testing.T) {
		store.Reset()

		auditSvc.LogUserLogin(ctx, "user-1", "test@example.com", "192.168.1.1", "Mozilla/5.0", true)

		waitForAuditLogs(t, ctx, store, "user-1", string(model.EventUserLogin), 1)
	})

	t.Run("记录用户登录失败", func(t *testing.T) {
		store.Reset()

		auditSvc.LogUserLogin(ctx, "user-1", "test@example.com", "192.168.1.1", "Mozilla/5.0", false)

		waitForAuditLogs(t, ctx, store, "user-1", string(model.EventUserLogin), 1)
	})
}

// ============================================================================
// LogTokenIssued 测试
// ============================================================================

func TestAuditService_LogTokenIssued(t *testing.T) {
	auditSvc, store := createTestAuditService()
	ctx := context.Background()

	t.Run("记录Token签发", func(t *testing.T) {
		store.Reset()

		auditSvc.LogTokenIssued(ctx, "user-1", "client-1", "192.168.1.1")

		waitForAuditLogs(t, ctx, store, "user-1", string(model.EventTokenIssued), 1)
	})
}

// ============================================================================
// LogAuthCodeCreated 测试
// ============================================================================

func TestAuditService_LogAuthCodeCreated(t *testing.T) {
	auditSvc, store := createTestAuditService()
	ctx := context.Background()

	t.Run("记录授权码创建", func(t *testing.T) {
		store.Reset()

		auditSvc.LogAuthCodeCreated(ctx, "user-1", "client-1", "192.168.1.1")

		waitForAuditLogs(t, ctx, store, "user-1", string(model.EventAuthCodeCreated), 1)
	})
}

// ============================================================================
// ListAuditLogs 测试
// ============================================================================

func TestAuditService_ListAuditLogs(t *testing.T) {
	_, store := createTestAuditService()
	ctx := context.Background()

	t.Run("列出审计日志", func(t *testing.T) {
		store.Reset()

		// 添加一些测试日志
		for i := 0; i < 5; i++ {
			log := &model.AuditLog{
				ID:        fmt.Sprintf("log-%d", i),
				EventType: string(model.EventUserLogin),
				UserID:    "user-1",
				Success:   true,
				Timestamp: time.Now(),
			}
			err := store.StoreAuditLog(ctx, log)
			require.NoError(t, err)
		}

		logs, total, err := store.ListAuditLogs(ctx, "user-1", "", 0, 10)
		require.NoError(t, err)
		assert.Equal(t, 5, total)
		assert.Equal(t, 5, len(logs))
	})

	t.Run("按事件类型过滤", func(t *testing.T) {
		store.Reset()

		// 添加不同类型的日志
		loginLog := &model.AuditLog{
			ID:        "log-login",
			EventType: string(model.EventUserLogin),
			UserID:    "user-1",
			Success:   true,
			Timestamp: time.Now(),
		}
		registerLog := &model.AuditLog{
			ID:        "log-register",
			EventType: string(model.EventUserRegister),
			UserID:    "user-1",
			Success:   true,
			Timestamp: time.Now(),
		}

		err := store.StoreAuditLog(ctx, loginLog)
		require.NoError(t, err)
		err = store.StoreAuditLog(ctx, registerLog)
		require.NoError(t, err)

		// 只查询登录事件
		logs, total, err := store.ListAuditLogs(ctx, "user-1", string(model.EventUserLogin), 0, 10)
		require.NoError(t, err)
		assert.Equal(t, 1, total)
		assert.Equal(t, 1, len(logs))
		assert.Equal(t, string(model.EventUserLogin), logs[0].EventType)
	})

	t.Run("分页查询", func(t *testing.T) {
		store.Reset()

		// 添加10条日志
		for i := 0; i < 10; i++ {
			log := &model.AuditLog{
				ID:        fmt.Sprintf("log-page-%d", i),
				EventType: string(model.EventUserLogin),
				UserID:    "user-page",
				Success:   true,
				Timestamp: time.Now(),
			}
			err := store.StoreAuditLog(ctx, log)
			require.NoError(t, err)
		}

		// 第一页
		logs1, total1, err := store.ListAuditLogs(ctx, "user-page", "", 0, 5)
		require.NoError(t, err)
		assert.Equal(t, 10, total1)
		assert.Equal(t, 5, len(logs1))

		// 第二页
		logs2, total2, err := store.ListAuditLogs(ctx, "user-page", "", 5, 5)
		require.NoError(t, err)
		assert.Equal(t, 10, total2)
		assert.Equal(t, 5, len(logs2))

		// 验证两页内容不同
		assert.NotEqual(t, logs1[0].ID, logs2[0].ID)
	})
}

func TestAuditService_NewMethods(t *testing.T) {
	auditSvc, store := createTestAuditService()
	ctx := context.Background()

	t.Run("LogUserLogout", func(t *testing.T) {
		store.Reset()
		auditSvc.LogUserLogout(ctx, "user-1", "192.168.1.1")
		waitForAuditLogs(t, ctx, store, "user-1", string(model.EventUserLogout), 1)
	})

	t.Run("LogTokenRefresh", func(t *testing.T) {
		store.Reset()
		auditSvc.LogTokenRefresh(ctx, "user-1", "client-1", "192.168.1.1")
		waitForAuditLogs(t, ctx, store, "user-1", string(model.EventTokenRefresh), 1)
	})

	t.Run("LogPasswordChanged", func(t *testing.T) {
		store.Reset()
		auditSvc.LogPasswordChanged(ctx, "user-1", "192.168.1.1", true)
		waitForAuditLogs(t, ctx, store, "user-1", string(model.EventPasswordChanged), 1)
	})

	t.Run("LogPasswordReset", func(t *testing.T) {
		store.Reset()
		auditSvc.LogPasswordReset(ctx, "user-1", "192.168.1.1")
		waitForAuditLogs(t, ctx, store, "user-1", string(model.EventPasswordReset), 1)
	})

	t.Run("LogAccountLocked", func(t *testing.T) {
		store.Reset()
		auditSvc.LogAccountLocked(ctx, "user-1", "192.168.1.1")
		waitForAuditLogs(t, ctx, store, "user-1", string(model.EventAccountLocked), 1)
	})

	t.Run("LogMFAEnabled", func(t *testing.T) {
		store.Reset()
		auditSvc.LogMFAEnabled(ctx, "user-1", "192.168.1.1")
		waitForAuditLogs(t, ctx, store, "user-1", string(model.EventMFAEnabled), 1)
	})

	t.Run("LogMFADisabled", func(t *testing.T) {
		store.Reset()
		auditSvc.LogMFADisabled(ctx, "user-1", "192.168.1.1")
		waitForAuditLogs(t, ctx, store, "user-1", string(model.EventMFADisabled), 1)
	})

	t.Run("LogKeyRotated", func(t *testing.T) {
		store.Reset()
		auditSvc.LogKeyRotated(ctx, "key-123")
		waitForAuditLogs(t, ctx, store, "", string(model.EventKeyRotated), 1)
	})

	t.Run("LogKeyRevoked", func(t *testing.T) {
		store.Reset()
		auditSvc.LogKeyRevoked(ctx, "key-123")
		waitForAuditLogs(t, ctx, store, "", string(model.EventKeyRevoked), 1)
	})

	t.Run("LogLogoutAll", func(t *testing.T) {
		store.Reset()
		auditSvc.LogLogoutAll(ctx, "user-1", "192.168.1.1")
		waitForAuditLogs(t, ctx, store, "user-1", string(model.EventLogoutAll), 1)
	})

	t.Run("LogTokenRevoke", func(t *testing.T) {
		store.Reset()
		auditSvc.LogTokenRevoke(ctx, "user-1", "client-1", "192.168.1.1")
		waitForAuditLogs(t, ctx, store, "user-1", string(model.EventTokenRevoke), 1)
	})

	t.Run("LogUserLoginFailed", func(t *testing.T) {
		store.Reset()
		auditSvc.LogUserLoginFailed(ctx, "user-1", "test@example.com", "192.168.1.1", "Mozilla/5.0", "invalid password")
		waitForAuditLogs(t, ctx, store, "user-1", string(model.EventUserLoginFailed), 1)
	})

	t.Run("LogAccountUnlocked", func(t *testing.T) {
		store.Reset()
		auditSvc.LogAccountUnlocked(ctx, "user-1", "192.168.1.1")
		waitForAuditLogs(t, ctx, store, "user-1", string(model.EventAccountUnlocked), 1)
	})

	t.Run("LogAuthCodeUsed", func(t *testing.T) {
		store.Reset()
		auditSvc.LogAuthCodeUsed(ctx, "user-1", "client-1", "192.168.1.1")
		waitForAuditLogs(t, ctx, store, "user-1", string(model.EventAuthCodeUsed), 1)
	})

	t.Run("LogAuthCodeInvalid", func(t *testing.T) {
		store.Reset()
		auditSvc.LogAuthCodeInvalid(ctx, "user-1", "client-1", "192.168.1.1", "invalid code")
		waitForAuditLogs(t, ctx, store, "user-1", string(model.EventAuthCodeInvalid), 1)
	})

	t.Run("LogMFASetup", func(t *testing.T) {
		store.Reset()
		auditSvc.LogMFASetup(ctx, "user-1", "192.168.1.1")
		waitForAuditLogs(t, ctx, store, "user-1", string(model.EventMFASetup), 1)
	})
}

func TestAuditService_Close(t *testing.T) {
	auditSvc, _ := createTestAuditService()

	t.Run("关闭审计服务", func(t *testing.T) {
		// Close应该不会panic
		assert.NotPanics(t, func() {
			auditSvc.Close()
		})
	})
}
