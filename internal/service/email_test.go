// Package service_test 邮件服务单元测试
package service_test

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/example/sso/internal/service"
)

// ============================================================================
// Mock MailSender
// ============================================================================

type mockMailSender struct {
	mu           sync.Mutex
	sentMessages []mockMessage
	shouldError  bool
	// callCount 记录 Send() 调用次数（无论 shouldError 如何）
	// CI/CD 修复：异步 goroutine 中 shouldError=true 时不会追加 sentMessages，
	// 测试无法用 Count() 等待 goroutine 完成。用原子计数器记录调用次数，
	// 测试可轮询 CallCount() 确认 goroutine 已进入 Send()
	callCount atomic.Int64
}

type mockMessage struct {
	Addr   string
	From   string
	To     []string
	Msg    []byte
	Config *service.EmailConfig
}

func (m *mockMailSender) Send(addr, from string, to []string, msg []byte, config *service.EmailConfig) error {
	// CI/CD 修复：先递增 callCount（无论 shouldError 如何），让测试可等待 goroutine 进入 Send()
	m.callCount.Add(1)
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.shouldError {
		return assert.AnError
	}
	m.sentMessages = append(m.sentMessages, mockMessage{addr, from, to, msg, config})
	return nil
}

// GetSentMessages 线程安全地返回已发送邮件快照（阶段 D 修复 L9）
// 异步发送邮件后测试需读取结果
func (m *mockMailSender) GetSentMessages() []mockMessage {
	m.mu.Lock()
	defer m.mu.Unlock()
	cp := make([]mockMessage, len(m.sentMessages))
	copy(cp, m.sentMessages)
	return cp
}

// Count 线程安全地返回已发送邮件数量
func (m *mockMailSender) Count() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.sentMessages)
}

// CallCount 返回 Send() 调用次数（包括 shouldError=true 的调用）
// CI/CD 修复：用于异步测试等待 goroutine 进入 Send()，替代固定 sleep
func (m *mockMailSender) CallCount() int64 {
	return m.callCount.Load()
}

// Reset 线程安全地清空 sentMessages 和 callCount（CI/CD 修复 L2）
// 替代测试中直接访问字段 `mockSender.sentMessages = nil`，避免潜在 data race
func (m *mockMailSender) Reset() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.sentMessages = nil
	m.callCount.Store(0)
}

// SetShouldError 线程安全地设置 shouldError 标志（阶段 D 修复 L9）
// 异步发送邮件时，直接修改 shouldError 会与 goroutine 中的读取产生 data race
func (m *mockMailSender) SetShouldError(v bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.shouldError = v
}

// ============================================================================
// NewEmailService 测试
// ============================================================================

func TestNewEmailService(t *testing.T) {
	config := &service.EmailConfig{
		SMTPHost: "smtp.example.com",
		SMTPPort: 587,
		Username: "user@example.com",
		Password: "password",
		From:     "noreply@example.com",
	}

	t.Run("默认sender", func(t *testing.T) {
		svc, err := service.NewEmailService(config)
		require.NoError(t, err)
		assert.NotNil(t, svc)
	})

	t.Run("注入mock sender", func(t *testing.T) {
		mock := &mockMailSender{}
		svc, err := service.NewEmailService(config, mock)
		require.NoError(t, err)
		assert.NotNil(t, svc)
	})

	t.Run("nil sender使用默认", func(t *testing.T) {
		svc, err := service.NewEmailService(config, nil)
		require.NoError(t, err)
		assert.NotNil(t, svc)
	})
}

// ============================================================================
// SendEmail 测试 (使用mock sender)
// ============================================================================

func TestEmailService_SendEmail_Mock(t *testing.T) {
	ctx := context.Background()

	t.Run("成功发送邮件", func(t *testing.T) {
		mock := &mockMailSender{}
		config := &service.EmailConfig{
			SMTPHost: "smtp.example.com",
			SMTPPort: 587,
			From:     "noreply@example.com",
		}
		svc, err := service.NewEmailService(config, mock)
		require.NoError(t, err)

		err = svc.SendEmail(ctx, "to@example.com", "Test Subject", "<html>body</html>")

		require.NoError(t, err)
		assert.Len(t, mock.sentMessages, 1)
		assert.Equal(t, "noreply@example.com", mock.sentMessages[0].From)
		assert.Contains(t, string(mock.sentMessages[0].Msg), "Test Subject")
		assert.Contains(t, string(mock.sentMessages[0].Msg), "<html>body</html>")
	})

	t.Run("SSL端口发送", func(t *testing.T) {
		mock := &mockMailSender{}
		config := &service.EmailConfig{
			SMTPHost: "smtp.gmail.com",
			SMTPPort: 465,
			Username: "user@gmail.com",
			Password: "password",
			From:     "user@gmail.com",
		}
		svc, err := service.NewEmailService(config, mock)
		require.NoError(t, err)

		err = svc.SendEmail(ctx, "to@example.com", "SSL Test", "<html>body</html>")

		require.NoError(t, err)
		assert.Len(t, mock.sentMessages, 1)
		assert.Equal(t, "smtp.gmail.com:465", mock.sentMessages[0].Addr)
	})

	t.Run("发送失败返回错误", func(t *testing.T) {
		mock := &mockMailSender{shouldError: true}
		config := &service.EmailConfig{
			SMTPHost: "smtp.example.com",
			SMTPPort: 587,
			From:     "noreply@example.com",
		}
		svc, err := service.NewEmailService(config, mock)
		require.NoError(t, err)

		err = svc.SendEmail(ctx, "to@example.com", "Test", "<html>body</html>")

		assert.Error(t, err)
		assert.NotContains(t, err.Error(), "发送邮件失败")
	})
}

// ============================================================================
// SendVerificationEmail 测试 (使用mock sender)
// ============================================================================

func TestEmailService_SendVerificationEmail_Mock(t *testing.T) {
	ctx := context.Background()

	t.Run("成功发送验证邮件", func(t *testing.T) {
		mock := &mockMailSender{}
		config := &service.EmailConfig{
			SMTPHost: "smtp.example.com",
			SMTPPort: 587,
			From:     "noreply@example.com",
		}
		svc, err := service.NewEmailService(config, mock)
		require.NoError(t, err)

		err = svc.SendVerificationEmail(ctx, "user@example.com", "TestUser", "https://example.com/verify?token=abc")

		require.NoError(t, err)
		assert.Len(t, mock.sentMessages, 1)
		assert.Equal(t, "user@example.com", mock.sentMessages[0].To[0])
		assert.Contains(t, string(mock.sentMessages[0].Msg), "TestUser")
		assert.Contains(t, string(mock.sentMessages[0].Msg), "https://example.com/verify?token=abc")
		assert.Contains(t, string(mock.sentMessages[0].Msg), "验证您的邮箱")
	})

	t.Run("中文用户名", func(t *testing.T) {
		mock := &mockMailSender{}
		config := &service.EmailConfig{
			SMTPHost: "smtp.example.com",
			SMTPPort: 587,
			From:     "noreply@example.com",
		}
		svc, err := service.NewEmailService(config, mock)
		require.NoError(t, err)

		err = svc.SendVerificationEmail(ctx, "user@example.com", "测试用户", "https://example.com/verify?token=xyz")

		require.NoError(t, err)
		assert.Len(t, mock.sentMessages, 1)
		assert.Contains(t, string(mock.sentMessages[0].Msg), "测试用户")
	})

	t.Run("邮件发送失败", func(t *testing.T) {
		mock := &mockMailSender{shouldError: true}
		config := &service.EmailConfig{
			SMTPHost: "smtp.example.com",
			SMTPPort: 587,
			From:     "noreply@example.com",
		}
		svc, err := service.NewEmailService(config, mock)
		require.NoError(t, err)

		err = svc.SendVerificationEmail(ctx, "user@example.com", "TestUser", "https://example.com/verify")

		assert.Error(t, err)
	})
}

// ============================================================================
// SendPasswordResetEmail 测试 (使用mock sender)
// ============================================================================

func TestEmailService_SendPasswordResetEmail_Mock(t *testing.T) {
	ctx := context.Background()

	t.Run("成功发送重置邮件", func(t *testing.T) {
		mock := &mockMailSender{}
		config := &service.EmailConfig{
			SMTPHost: "smtp.example.com",
			SMTPPort: 587,
			From:     "noreply@example.com",
		}
		svc, err := service.NewEmailService(config, mock)
		require.NoError(t, err)

		err = svc.SendPasswordResetEmail(ctx, "user@example.com", "TestUser", "https://example.com/reset?token=abc")

		require.NoError(t, err)
		assert.Len(t, mock.sentMessages, 1)
		assert.Equal(t, "user@example.com", mock.sentMessages[0].To[0])
		assert.Contains(t, string(mock.sentMessages[0].Msg), "TestUser")
		assert.Contains(t, string(mock.sentMessages[0].Msg), "https://example.com/reset?token=abc")
		assert.Contains(t, string(mock.sentMessages[0].Msg), "重置您的密码")
	})

	t.Run("中文用户名", func(t *testing.T) {
		mock := &mockMailSender{}
		config := &service.EmailConfig{
			SMTPHost: "smtp.example.com",
			SMTPPort: 587,
			From:     "noreply@example.com",
		}
		svc, err := service.NewEmailService(config, mock)
		require.NoError(t, err)

		err = svc.SendPasswordResetEmail(ctx, "user@example.com", "测试用户", "https://example.com/reset?token=xyz")

		require.NoError(t, err)
		assert.Len(t, mock.sentMessages, 1)
		assert.Contains(t, string(mock.sentMessages[0].Msg), "测试用户")
	})

	t.Run("邮件发送失败", func(t *testing.T) {
		mock := &mockMailSender{shouldError: true}
		config := &service.EmailConfig{
			SMTPHost: "smtp.example.com",
			SMTPPort: 587,
			From:     "noreply@example.com",
		}
		svc, err := service.NewEmailService(config, mock)
		require.NoError(t, err)

		err = svc.SendPasswordResetEmail(ctx, "user@example.com", "TestUser", "https://example.com/reset")

		assert.Error(t, err)
	})
}

// ============================================================================
// 集成测试 - 使用真实默认sender (localhost端口1快速拒绝)
// ============================================================================

func TestEmailService_DefaultSender_Integration(t *testing.T) {
	ctx := context.Background()

	t.Run("STARTTLS端口连接失败", func(t *testing.T) {
		config := &service.EmailConfig{
			SMTPHost: "localhost",
			SMTPPort: 1,
			From:     "test@example.com",
		}
		svc, err := service.NewEmailService(config)
		require.NoError(t, err)

		err = svc.SendEmail(ctx, "to@example.com", "Test", "<html>body</html>")

		assert.Error(t, err)
		assert.NotContains(t, err.Error(), "发送邮件失败")
	})

	t.Run("SSL端口连接失败", func(t *testing.T) {
		config := &service.EmailConfig{
			SMTPHost: "localhost",
			SMTPPort: 2,
			From:     "test@example.com",
		}
		svc, err := service.NewEmailService(config)
		require.NoError(t, err)

		err = svc.SendEmail(ctx, "to@example.com", "Test", "<html>body</html>")

		assert.Error(t, err)
	})

	t.Run("验证邮件模板渲染并发送失败", func(t *testing.T) {
		config := &service.EmailConfig{
			SMTPHost: "localhost",
			SMTPPort: 1,
			From:     "test@example.com",
		}
		svc, err := service.NewEmailService(config)
		require.NoError(t, err)

		err = svc.SendVerificationEmail(ctx, "user@example.com", "TestUser", "https://example.com/verify")

		assert.Error(t, err)
	})

	t.Run("重置邮件模板渲染并发送失败", func(t *testing.T) {
		config := &service.EmailConfig{
			SMTPHost: "localhost",
			SMTPPort: 1,
			From:     "test@example.com",
		}
		svc, err := service.NewEmailService(config)
		require.NoError(t, err)

		err = svc.SendPasswordResetEmail(ctx, "user@example.com", "TestUser", "https://example.com/reset")

		assert.Error(t, err)
	})
}
