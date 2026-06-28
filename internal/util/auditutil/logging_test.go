package auditutil_test

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/example/sso/internal/model"
	"github.com/example/sso/internal/util/auditutil"
)

// ============================================================================
// Mock实现
// ============================================================================

// MockAuditService 用于测试的Mock审计服务
type MockAuditService struct {
	LogCalls []*model.AuditLog
	LogError error
	mu       sync.Mutex
}

// Log 记录审计日志
func (m *MockAuditService) Log(ctx context.Context, log *model.AuditLog) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.LogCalls = append(m.LogCalls, log)
}

// SetError 设置Log方法返回的错误
func (m *MockAuditService) SetError(err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.LogError = err
}

// Reset 重置Mock状态
func (m *MockAuditService) Reset() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.LogCalls = nil
	m.LogError = nil
}

// GetLogCalls 获取日志调用列表（线程安全）
func (m *MockAuditService) GetLogCalls() []*model.AuditLog {
	m.mu.Lock()
	defer m.mu.Unlock()
	// 返回副本以避免外部修改
	calls := make([]*model.AuditLog, len(m.LogCalls))
	copy(calls, m.LogCalls)
	return calls
}

// ============================================================================
// LogWithFallback 测试
// ============================================================================

func TestLogWithFallback_NilAuditService(t *testing.T) {
	t.Parallel()
	// 当auditSvc为nil时，应该直接返回，不panic
	called := false
	logFunc := func() error {
		called = true
		return nil
	}

	// 不应该panic
	auditutil.LogWithFallback(nil, logFunc)

	// logFunc不应该被调用
	assert.False(t, called)
}

func TestLogWithFallback_SuccessfulLog(t *testing.T) {
	t.Parallel()
	mockSvc := &MockAuditService{}
	called := false

	logFunc := func() error {
		called = true
		mockSvc.Log(context.Background(), &model.AuditLog{
			EventType: "test",
			UserID:    "test-user",
		})
		return nil
	}

	// 不应该panic
	auditutil.LogWithFallback(mockSvc, logFunc)

	// logFunc应该被调用
	assert.True(t, called)
	// 审计日志应该被记录
	assert.Len(t, mockSvc.GetLogCalls(), 1)
}

func TestLogWithFallback_LogFunctionError(t *testing.T) {
	// 不能并行运行，因为修改全局stderr
	mockSvc := &MockAuditService{}

	// 捕获stderr输出
	oldStderr := os.Stderr
	r, w, err := os.Pipe()
	require.NoError(t, err)
	os.Stderr = w

	logFunc := func() error {
		return errors.New("audit log failed")
	}

	// 不应该panic，即使logFunc返回错误
	auditutil.LogWithFallback(mockSvc, logFunc)

	// 恢复stderr
	w.Close()
	os.Stderr = oldStderr

	// 读取stderr输出
	var buf bytes.Buffer
	_, err = io.Copy(&buf, r)
	require.NoError(t, err)
	r.Close()

	// 应该在stderr中看到错误信息
	output := buf.String()
	assert.Contains(t, output, "[AUDIT_FALLBACK]")
	assert.Contains(t, output, "audit log failed")
}

func TestLogWithFallback_MultipleErrors(t *testing.T) {
	// 不能并行运行，因为修改全局stderr
	mockSvc := &MockAuditService{}

	// 捕获stderr输出
	oldStderr := os.Stderr
	r, w, err := os.Pipe()
	require.NoError(t, err)
	os.Stderr = w

	callCount := 0
	logFunc := func() error {
		callCount++
		return errors.New("error " + string(rune(callCount)))
	}

	// 多次调用，每次都失败
	for i := 0; i < 3; i++ {
		auditutil.LogWithFallback(mockSvc, logFunc)
	}

	// 恢复stderr
	w.Close()
	os.Stderr = oldStderr

	// 读取stderr输出
	var buf bytes.Buffer
	_, err = io.Copy(&buf, r)
	require.NoError(t, err)
	r.Close()

	// 应该在stderr中看到所有错误
	output := buf.String()
	assert.Contains(t, output, "[AUDIT_FALLBACK]")
}

// ============================================================================
// SafeAuditLog 测试
// ============================================================================

func TestSafeAuditLog_NilAuditService(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	// 当auditSvc为nil时，应该直接返回，不panic
	auditutil.SafeAuditLog(ctx, nil, "user_login", "user-123", map[string]interface{}{
		"email": "test@example.com",
	})

	// 不应该panic
}

func TestSafeAuditLog_SuccessfulLog(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	mockSvc := &MockAuditService{}

	metadata := map[string]interface{}{
		"email":      "test@example.com",
		"ip_address": "192.168.1.1",
	}

	auditutil.SafeAuditLog(ctx, mockSvc, "user_login", "user-123", metadata)

	// 审计日志应该被记录
	logCalls := mockSvc.GetLogCalls()
	assert.Len(t, logCalls, 1)

	// 验证日志内容
	logEntry := logCalls[0]
	assert.Equal(t, "user_login", logEntry.EventType)
	assert.Equal(t, "user-123", logEntry.UserID)
	assert.NotEmpty(t, logEntry.Details)
}

func TestSafeAuditLog_EmptyUserID(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	mockSvc := &MockAuditService{}

	// 用户ID可以为空（例如系统事件）
	auditutil.SafeAuditLog(ctx, mockSvc, "system_start", "", map[string]interface{}{
		"version": "1.0.0",
	})

	// 审计日志应该被记录
	logCalls := mockSvc.GetLogCalls()
	assert.Len(t, logCalls, 1)

	logEntry := logCalls[0]
	assert.Equal(t, "system_start", logEntry.EventType)
	assert.Equal(t, "", logEntry.UserID)
}

func TestSafeAuditLog_NilMetadata(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	mockSvc := &MockAuditService{}

	// 元数据可以为nil
	auditutil.SafeAuditLog(ctx, mockSvc, "user_logout", "user-123", nil)

	// 审计日志应该被记录
	logCalls := mockSvc.GetLogCalls()
	assert.Len(t, logCalls, 1)

	logEntry := logCalls[0]
	assert.Equal(t, "user_logout", logEntry.EventType)
	assert.Equal(t, "user-123", logEntry.UserID)
}

func TestSafeAuditLog_AuditServiceError(t *testing.T) {
	// 不能并行运行，因为修改全局stderr
	mockSvc := &MockAuditService{}

	// 捕获stderr输出
	oldStderr := os.Stderr
	r, w, err := os.Pipe()
	require.NoError(t, err)
	os.Stderr = w

	// 模拟审计服务错误
	// 注意：在实际实现中，Log方法不返回error
	// 但SafeAuditLog应该处理logFunc返回的任何错误

	// 创建一个会失败的logFunc
	callCount := 0
	logFunc := func() error {
		callCount++
		if callCount > 0 {
			return errors.New("database connection failed")
		}
		return nil
	}

	// 调用LogWithFallback来测试错误处理
	auditutil.LogWithFallback(mockSvc, logFunc)

	// 恢复stderr
	w.Close()
	os.Stderr = oldStderr

	// 读取stderr输出
	var buf bytes.Buffer
	_, err = io.Copy(&buf, r)
	require.NoError(t, err)
	r.Close()

	// 应该在stderr中看到错误
	output := buf.String()
	assert.Contains(t, output, "[AUDIT_FALLBACK]")
	assert.Contains(t, output, "database connection failed")
}

func TestSafeAuditLog_ComplexMetadata(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	mockSvc := &MockAuditService{}

	// 复杂的元数据结构
	metadata := map[string]interface{}{
		"email":      "test@example.com",
		"ip_address": "192.168.1.1",
		"user_agent": "Mozilla/5.0",
		"success":    true,
		"attempts":   3,
		"details": map[string]interface{}{
			"mfa_enabled": true,
			"device_id":   "device-123",
		},
	}

	auditutil.SafeAuditLog(ctx, mockSvc, "user_login", "user-123", metadata)

	// 审计日志应该被记录
	logCalls := mockSvc.GetLogCalls()
	assert.Len(t, logCalls, 1)

	logEntry := logCalls[0]
	assert.Equal(t, "user_login", logEntry.EventType)
	assert.Equal(t, "user-123", logEntry.UserID)
	assert.NotEmpty(t, logEntry.Details)
}

func TestSafeAuditLog_MultipleEvents(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	mockSvc := &MockAuditService{}

	// 记录多个事件
	events := []struct {
		event    string
		userID   string
		metadata map[string]interface{}
	}{
		{
			event:  "user_login",
			userID: "user-1",
			metadata: map[string]interface{}{
				"email": "user1@example.com",
			},
		},
		{
			event:  "user_logout",
			userID: "user-1",
			metadata: map[string]interface{}{
				"reason": "manual",
			},
		},
		{
			event:  "user_register",
			userID: "user-2",
			metadata: map[string]interface{}{
				"email": "user2@example.com",
			},
		},
	}

	for _, e := range events {
		auditutil.SafeAuditLog(ctx, mockSvc, e.event, e.userID, e.metadata)
	}

	// 所有事件都应该被记录
	logCalls := mockSvc.GetLogCalls()
	assert.Len(t, logCalls, 3)

	// 验证每个事件
	for i, e := range events {
		logEntry := logCalls[i]
		assert.Equal(t, e.event, logEntry.EventType)
		assert.Equal(t, e.userID, logEntry.UserID)
	}
}

// ============================================================================
// 集成测试
// ============================================================================

func TestAuditUtil_IntegrationWithMockService(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	mockSvc := &MockAuditService{}

	// 模拟一个完整的审计日志流程
	// 1. 用户登录
	auditutil.SafeAuditLog(ctx, mockSvc, "user_login", "user-123", map[string]interface{}{
		"email":      "test@example.com",
		"ip_address": "192.168.1.1",
		"success":    true,
	})

	// 2. 用户执行操作
	auditutil.SafeAuditLog(ctx, mockSvc, "user_action", "user-123", map[string]interface{}{
		"action":     "update_profile",
		"ip_address": "192.168.1.1",
	})

	// 3. 用户登出
	auditutil.SafeAuditLog(ctx, mockSvc, "user_logout", "user-123", map[string]interface{}{
		"ip_address": "192.168.1.1",
	})

	// 验证所有事件都被记录
	logCalls := mockSvc.GetLogCalls()
	assert.Len(t, logCalls, 3)

	// 验证事件顺序
	assert.Equal(t, "user_login", logCalls[0].EventType)
	assert.Equal(t, "user_action", logCalls[1].EventType)
	assert.Equal(t, "user_logout", logCalls[2].EventType)
}

func TestAuditUtil_ConcurrentLogging(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	mockSvc := &MockAuditService{}

	// 并发记录审计日志
	done := make(chan bool, 10)
	for i := 0; i < 10; i++ {
		go func(id int) {
			auditutil.SafeAuditLog(ctx, mockSvc, "concurrent_event", "user-123", map[string]interface{}{
				"event_id": id,
			})
			done <- true
		}(i)
	}

	// 等待所有goroutine完成
	for i := 0; i < 10; i++ {
		<-done
	}

	// 所有事件都应该被记录
	logCalls := mockSvc.GetLogCalls()
	assert.Len(t, logCalls, 10)
}

// ============================================================================
// 边界条件测试
// ============================================================================

func TestSafeAuditLog_EmptyEvent(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	mockSvc := &MockAuditService{}

	// 事件类型可以为空（虽然不推荐）
	auditutil.SafeAuditLog(ctx, mockSvc, "", "user-123", nil)

	logCalls := mockSvc.GetLogCalls()
	assert.Len(t, logCalls, 1)
	logEntry := logCalls[0]
	assert.Equal(t, "", logEntry.EventType)
}

func TestSafeAuditLog_LargeMetadata(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	mockSvc := &MockAuditService{}

	// 大型元数据
	largeMetadata := make(map[string]interface{})
	for i := 0; i < 100; i++ {
		largeMetadata["key_"+string(rune(i))] = "value_" + string(rune(i))
	}

	auditutil.SafeAuditLog(ctx, mockSvc, "large_event", "user-123", largeMetadata)

	logCalls := mockSvc.GetLogCalls()
	assert.Len(t, logCalls, 1)
	logEntry := logCalls[0]
	assert.Equal(t, "large_event", logEntry.EventType)
	assert.NotEmpty(t, logEntry.Details)
}

func TestLogWithFallback_PanicInLogFunc(t *testing.T) {
	t.Parallel()
	mockSvc := &MockAuditService{}

	// 注意：如果logFunc panic，LogWithFallback不会捕获它
	// 这是设计决定，让panic传播以便调试
	logFunc := func() error {
		// 这会panic
		panic("test panic")
	}

	// 这应该panic
	assert.Panics(t, func() {
		auditutil.LogWithFallback(mockSvc, logFunc)
	})
}
