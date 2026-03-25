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

		// Log方法是异步的，我们需要等待一下
		auditSvc.Log(ctx, log)

		// 等待异步操作完成
		time.Sleep(100 * time.Millisecond)

		// 验证日志已存储
		logs, _, err := store.ListAuditLogs(ctx, "test-user-1", "", 0, 10)
		require.NoError(t, err)
		assert.GreaterOrEqual(t, len(logs), 1)
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

		time.Sleep(100 * time.Millisecond)

		logs, _, err := store.ListAuditLogs(ctx, "user-1", string(model.EventUserRegister), 0, 10)
		require.NoError(t, err)
		assert.GreaterOrEqual(t, len(logs), 1)
	})

	t.Run("记录用户注册失败", func(t *testing.T) {
		store.Reset()

		auditSvc.LogUserRegister(ctx, "", "invalid@example.com", "192.168.1.1", false)

		time.Sleep(100 * time.Millisecond)

		logs, _, err := store.ListAuditLogs(ctx, "", string(model.EventUserRegister), 0, 10)
		require.NoError(t, err)
		assert.GreaterOrEqual(t, len(logs), 1)
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

		time.Sleep(100 * time.Millisecond)

		logs, _, err := store.ListAuditLogs(ctx, "user-1", string(model.EventUserLogin), 0, 10)
		require.NoError(t, err)
		assert.GreaterOrEqual(t, len(logs), 1)
	})

	t.Run("记录用户登录失败", func(t *testing.T) {
		store.Reset()

		auditSvc.LogUserLogin(ctx, "user-1", "test@example.com", "192.168.1.1", "Mozilla/5.0", false)

		time.Sleep(100 * time.Millisecond)

		logs, _, err := store.ListAuditLogs(ctx, "user-1", string(model.EventUserLogin), 0, 10)
		require.NoError(t, err)
		assert.GreaterOrEqual(t, len(logs), 1)
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

		time.Sleep(100 * time.Millisecond)

		logs, _, err := store.ListAuditLogs(ctx, "user-1", string(model.EventTokenIssued), 0, 10)
		require.NoError(t, err)
		assert.GreaterOrEqual(t, len(logs), 1)
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

		time.Sleep(100 * time.Millisecond)

		logs, _, err := store.ListAuditLogs(ctx, "user-1", string(model.EventAuthCodeCreated), 0, 10)
		require.NoError(t, err)
		assert.GreaterOrEqual(t, len(logs), 1)
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
