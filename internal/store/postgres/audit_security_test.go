//go:build integration

// Package postgres_test 审计日志SQL安全测试
package postgres_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/example/sso/internal/model"
)

// TestListAuditLogs_SQLInjectionPrevention 测试SQL注入防护
func TestListAuditLogs_SQLInjectionPrevention(t *testing.T) {
	if testing.Short() {
		t.Skip("跳过数据库集成测试")
	}

	store, db := setupTestStore(t)
	t.Cleanup(func() {
		cleanupTestData(t, db)
		db.Close()
	})

	ctx := context.Background()

	// 使用唯一ID避免与其他测试冲突
	testID := uuid.New().String()[:8]
	userID1 := "sqlinj-user-" + testID + "-1"
	userID2 := "sqlinj-user-" + testID + "-2"

	// 创建测试数据
	testLogs := []*model.AuditLog{
		{
			ID:        uuid.New().String(),
			EventType: "user_login",
			UserID:    userID1,
			IPAddress: "192.168.1.1",
			Details:   `{}`,
			Success:   true,
			Timestamp: time.Now(),
		},
		{
			ID:        uuid.New().String(),
			EventType: "user_logout",
			UserID:    userID2,
			IPAddress: "192.168.1.2",
			Details:   `{}`,
			Success:   true,
			Timestamp: time.Now(),
		},
	}

	for _, log := range testLogs {
		err := store.StoreAuditLog(ctx, log)
		require.NoError(t, err)
	}

	tests := []struct {
		name          string
		userID        string
		eventType     string
		expectedCount int
		description   string
	}{
		{
			name:          "正常查询_按用户ID",
			userID:        userID1,
			eventType:     "",
			expectedCount: 1,
			description:   "正常的用户ID查询应该返回1条记录",
		},
		{
			name:          "正常查询_按事件类型",
			userID:        "",
			eventType:     "user_login",
			expectedCount: -1, // 使用-1表示至少1条
			description:   "正常的事件类型查询应该返回至少1条记录",
		},
		{
			name:          "SQL注入尝试_用户ID中的OR条件",
			userID:        userID1 + "' OR '1'='1",
			eventType:     "",
			expectedCount: 0,
			description:   "SQL注入尝试应该被参数化查询阻止，返回0条记录",
		},
		{
			name:          "SQL注入尝试_事件类型中的UNION",
			userID:        "",
			eventType:     "user_login' UNION SELECT * FROM users--",
			expectedCount: 0,
			description:   "UNION注入尝试应该被阻止",
		},
		{
			name:          "SQL注入尝试_用户ID中的注释",
			userID:        userID1 + "'--",
			eventType:     "",
			expectedCount: 0,
			description:   "SQL注释注入应该被阻止",
		},
		{
			name:          "SQL注入尝试_事件类型中的DROP",
			userID:        "",
			eventType:     "user_login'; DROP TABLE audit_logs--",
			expectedCount: 0,
			description:   "DROP TABLE注入应该被阻止",
		},
		{
			name:          "SQL注入尝试_用户ID中的分号",
			userID:        userID1 + "'; DELETE FROM audit_logs WHERE '1'='1",
			eventType:     "",
			expectedCount: 0,
			description:   "DELETE注入应该被阻止",
		},
		{
			name:          "特殊字符_单引号",
			userID:        "user'123",
			eventType:     "",
			expectedCount: 0,
			description:   "包含单引号的用户ID应该被安全处理",
		},
		{
			name:          "特殊字符_反斜杠",
			userID:        "user\\123",
			eventType:     "",
			expectedCount: 0,
			description:   "包含反斜杠的用户ID应该被安全处理",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logs, total, err := store.ListAuditLogs(ctx, tt.userID, tt.eventType, 0, 100)

			// 查询不应该返回错误（参数化查询会安全处理所有输入）
			assert.NoError(t, err, "查询不应该返回错误")

			// 对于"至少"类型的测试，使用GreaterOrEqual
			if tt.expectedCount == -1 {
				assert.GreaterOrEqual(t, len(logs), 1, tt.description)
				assert.GreaterOrEqual(t, total, 1, "总数应该大于等于1")
			} else {
				assert.Equal(t, tt.expectedCount, len(logs), tt.description)
				assert.Equal(t, tt.expectedCount, total, "总数应该与记录数一致")
			}
		})
	}
}

// TestListAuditLogs_Pagination 测试分页功能
func TestListAuditLogs_Pagination(t *testing.T) {
	if testing.Short() {
		t.Skip("跳过数据库集成测试")
	}

	store, db := setupTestStore(t)
	t.Cleanup(func() {
		cleanupTestData(t, db)
		db.Close()
	})

	ctx := context.Background()

	// 使用唯一ID避免与其他测试冲突
	testUserID := "pagination-user-" + uuid.New().String()[:8]

	// 创建10条测试数据
	for i := 0; i < 10; i++ {
		log := &model.AuditLog{
			ID:        uuid.New().String(),
			EventType: "test_event",
			UserID:    testUserID,
			Details:   `{}`,
			Success:   true,
			Timestamp: time.Now().Add(time.Duration(i) * time.Second),
		}
		err := store.StoreAuditLog(ctx, log)
		require.NoError(t, err)
	}

	tests := []struct {
		name          string
		offset        int
		limit         int
		expectedCount int
		expectedTotal int
	}{
		{
			name:          "第一页_5条",
			offset:        0,
			limit:         5,
			expectedCount: 5,
			expectedTotal: 10,
		},
		{
			name:          "第二页_5条",
			offset:        5,
			limit:         5,
			expectedCount: 5,
			expectedTotal: 10,
		},
		{
			name:          "超出范围",
			offset:        15,
			limit:         5,
			expectedCount: 0,
			expectedTotal: 10,
		},
		{
			name:          "全部记录",
			offset:        0,
			limit:         100,
			expectedCount: 10,
			expectedTotal: 10,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logs, total, err := store.ListAuditLogs(ctx, testUserID, "", tt.offset, tt.limit)

			assert.NoError(t, err)
			assert.Equal(t, tt.expectedCount, len(logs))
			assert.Equal(t, tt.expectedTotal, total)
		})
	}
}

// TestListAuditLogs_Filtering 测试过滤功能
func TestListAuditLogs_Filtering(t *testing.T) {
	if testing.Short() {
		t.Skip("跳过数据库集成测试")
	}

	store, db := setupTestStore(t)
	t.Cleanup(func() {
		cleanupTestData(t, db)
		db.Close()
	})

	ctx := context.Background()

	// 使用唯一ID避免与其他测试冲突
	testID := uuid.New().String()[:8]
	user1 := "filter-user-" + testID + "-1"
	user2 := "filter-user-" + testID + "-2"

	// 创建不同用户和事件类型的测试数据
	testData := []struct {
		userID    string
		eventType string
	}{
		{user1, "login"},
		{user1, "logout"},
		{user2, "login"},
		{user2, "register"},
	}

	for _, data := range testData {
		log := &model.AuditLog{
			ID:        uuid.New().String(),
			EventType: data.eventType,
			UserID:    data.userID,
			Details:   `{}`,
			Success:   true,
			Timestamp: time.Now(),
		}
		err := store.StoreAuditLog(ctx, log)
		require.NoError(t, err)
	}

	tests := []struct {
		name          string
		userID        string
		eventType     string
		expectedCount int
	}{
		{
			name:          "按用户ID过滤",
			userID:        user1,
			eventType:     "",
			expectedCount: 2,
		},
		{
			name:          "按事件类型过滤",
			userID:        "",
			eventType:     "login",
			expectedCount: -1, // 至少2条（可能有其他测试的login事件）
		},
		{
			name:          "同时按用户ID和事件类型过滤",
			userID:        user1,
			eventType:     "login",
			expectedCount: 1,
		},
		{
			name:          "不存在的用户",
			userID:        "user-999-nonexistent",
			eventType:     "",
			expectedCount: 0,
		},
		{
			name:          "不存在的事件类型",
			userID:        "",
			eventType:     "unknown-event-type-xyz",
			expectedCount: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logs, total, err := store.ListAuditLogs(ctx, tt.userID, tt.eventType, 0, 100)

			assert.NoError(t, err)
			// 对于"按事件类型过滤"，可能有其他测试创建的login事件，所以使用GreaterOrEqual
			if tt.expectedCount == -1 {
				assert.GreaterOrEqual(t, len(logs), 2)
				assert.GreaterOrEqual(t, total, 2)
			} else {
				assert.Equal(t, tt.expectedCount, len(logs))
				assert.Equal(t, tt.expectedCount, total)
			}
		})
	}
}
