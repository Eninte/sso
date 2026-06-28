// Package auditutil_test 关键审计日志测试
package auditutil_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/example/sso/internal/model"
	"github.com/example/sso/internal/util/auditutil"
)

// mockAuditService 模拟审计服务
type mockAuditService struct {
	logs       []*model.AuditLog
	shouldFail bool
}

func (m *mockAuditService) Log(ctx context.Context, log *model.AuditLog) {
	if !m.shouldFail {
		m.logs = append(m.logs, log)
	}
}

// TestIsCriticalEvent 测试关键事件判断
func TestIsCriticalEvent(t *testing.T) {
	tests := []struct {
		name     string
		event    string
		expected bool
	}{
		{
			name:     "密码修改_是关键事件",
			event:    "password_changed",
			expected: true,
		},
		{
			name:     "MFA禁用_是关键事件",
			event:    "mfa.disabled",
			expected: true,
		},
		{
			name:     "MFA启用_是关键事件",
			event:    "mfa.enabled",
			expected: true,
		},
		{
			name:     "账户锁定_是关键事件",
			event:    "account.locked",
			expected: true,
		},
		{
			name:     "账户禁用_是关键事件",
			event:    "account.disabled",
			expected: true,
		},
		{
			name:     "管理员删除用户_是关键事件",
			event:    "admin.user_deleted",
			expected: true,
		},
		{
			name:     "管理员修改角色_是关键事件",
			event:    "admin.role_changed",
			expected: true,
		},
		{
			name:     "管理员禁用用户_是关键事件",
			event:    "admin.user_disabled",
			expected: true,
		},
		{
			name:     "管理员启用用户_是关键事件",
			event:    "admin.user_enabled",
			expected: true,
		},
		{
			name:     "用户登录_不是关键事件",
			event:    "user_login",
			expected: false,
		},
		{
			name:     "用户注册_不是关键事件",
			event:    "user_register",
			expected: false,
		},
		{
			name:     "Token刷新_不是关键事件",
			event:    "token_refresh",
			expected: false,
		},
		{
			name:     "未知事件_不是关键事件",
			event:    "unknown_event",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := auditutil.IsCriticalEvent(tt.event)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestCriticalAuditLog_Success 测试关键审计日志成功记录
func TestCriticalAuditLog_Success(t *testing.T) {
	ctx := context.Background()
	mockSvc := &mockAuditService{}

	metadata := map[string]interface{}{
		"ip_address": "192.168.1.1",
		"user_agent": "Mozilla/5.0",
		"success":    true,
	}

	err := auditutil.CriticalAuditLog(ctx, mockSvc, "password_changed", "user-123", metadata)

	assert.NoError(t, err, "关键审计日志应该成功记录")
	assert.Len(t, mockSvc.logs, 1, "应该记录1条日志")

	log := mockSvc.logs[0]
	assert.Equal(t, "password_changed", log.EventType)
	assert.Equal(t, "user-123", log.UserID)
	assert.Equal(t, "192.168.1.1", log.IPAddress)
	assert.Equal(t, "Mozilla/5.0", log.UserAgent)
	assert.True(t, log.Success)
}

// TestCriticalAuditLog_NilService 测试审计服务为nil时返回错误
func TestCriticalAuditLog_NilService(t *testing.T) {
	ctx := context.Background()

	err := auditutil.CriticalAuditLog(ctx, nil, "password_changed", "user-123", nil)

	assert.Error(t, err, "审计服务为nil时应该返回错误")
	assert.Contains(t, err.Error(), "audit service required", "错误消息应该说明需要审计服务")
}

// TestCriticalAuditLog_WithoutMetadata 测试没有元数据的情况
func TestCriticalAuditLog_WithoutMetadata(t *testing.T) {
	ctx := context.Background()
	mockSvc := &mockAuditService{}

	err := auditutil.CriticalAuditLog(ctx, mockSvc, "mfa.disabled", "user-456", nil)

	assert.NoError(t, err)
	assert.Len(t, mockSvc.logs, 1)

	log := mockSvc.logs[0]
	assert.Equal(t, "mfa.disabled", log.EventType)
	assert.Equal(t, "user-456", log.UserID)
	assert.Empty(t, log.IPAddress)
	assert.Empty(t, log.UserAgent)
	assert.True(t, log.Success) // 默认为true
}

// TestCriticalAuditLog_WithClientID 测试包含ClientID的情况
func TestCriticalAuditLog_WithClientID(t *testing.T) {
	ctx := context.Background()
	mockSvc := &mockAuditService{}

	metadata := map[string]interface{}{
		"client_id":  "oauth-client-123",
		"ip_address": "10.0.0.1",
		"success":    false,
	}

	err := auditutil.CriticalAuditLog(ctx, mockSvc, "admin.user_deleted", "admin-789", metadata)

	assert.NoError(t, err)
	assert.Len(t, mockSvc.logs, 1)

	log := mockSvc.logs[0]
	assert.Equal(t, "admin.user_deleted", log.EventType)
	assert.Equal(t, "admin-789", log.UserID)
	assert.Equal(t, "oauth-client-123", log.ClientID)
	assert.Equal(t, "10.0.0.1", log.IPAddress)
	assert.False(t, log.Success)
}

// TestCriticalAuditLog_MultipleEvents 测试记录多个事件
func TestCriticalAuditLog_MultipleEvents(t *testing.T) {
	ctx := context.Background()
	mockSvc := &mockAuditService{}

	events := []struct {
		event  string
		userID string
	}{
		{"password_changed", "user-1"},
		{"mfa.enabled", "user-2"},
		{"account.locked", "user-3"},
	}

	for _, e := range events {
		err := auditutil.CriticalAuditLog(ctx, mockSvc, e.event, e.userID, nil)
		assert.NoError(t, err)
	}

	assert.Len(t, mockSvc.logs, 3, "应该记录3条日志")

	for i, e := range events {
		assert.Equal(t, e.event, mockSvc.logs[i].EventType)
		assert.Equal(t, e.userID, mockSvc.logs[i].UserID)
	}
}

// TestCriticalAuditLog_EmptyUserID 测试空用户ID的情况
func TestCriticalAuditLog_EmptyUserID(t *testing.T) {
	ctx := context.Background()
	mockSvc := &mockAuditService{}

	err := auditutil.CriticalAuditLog(ctx, mockSvc, "admin.role_changed", "", map[string]interface{}{
		"target_user": "user-123",
		"new_role":    "admin",
	})

	assert.NoError(t, err)
	assert.Len(t, mockSvc.logs, 1)

	log := mockSvc.logs[0]
	assert.Equal(t, "admin.role_changed", log.EventType)
	assert.Empty(t, log.UserID)
	assert.Contains(t, log.Details, "target_user")
}

// TestSafeAuditLog_StillWorks 确保SafeAuditLog仍然正常工作
func TestSafeAuditLog_StillWorks(t *testing.T) {
	ctx := context.Background()
	mockSvc := &mockAuditService{}

	// SafeAuditLog不应该返回错误，即使审计服务失败
	auditutil.SafeAuditLog(ctx, mockSvc, "user_login", "user-123", map[string]interface{}{
		"ip_address": "192.168.1.1",
	})

	assert.Len(t, mockSvc.logs, 1, "SafeAuditLog应该记录日志")
}

// TestSafeAuditLog_NilService 测试SafeAuditLog在服务为nil时不panic
func TestSafeAuditLog_NilService(t *testing.T) {
	ctx := context.Background()

	// 不应该panic
	assert.NotPanics(t, func() {
		auditutil.SafeAuditLog(ctx, nil, "user_login", "user-123", nil)
	})
}
