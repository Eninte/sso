// Package service_test 邮件服务单元测试
package service_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/example/sso/internal/service"
)

// ============================================================================
// Mock MailSender
// ============================================================================

type mockMailSender struct {
	sentMessages []mockMessage
	shouldError  bool
}

type mockMessage struct {
	Addr   string
	From   string
	To     []string
	Msg    []byte
	Config *service.EmailConfig
}

func (m *mockMailSender) Send(addr, from string, to []string, msg []byte, config *service.EmailConfig) error {
	if m.shouldError {
		return assert.AnError
	}
	m.sentMessages = append(m.sentMessages, mockMessage{addr, from, to, msg, config})
	return nil
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
		assert.Contains(t, err.Error(), "发送邮件失败")
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
		assert.Contains(t, err.Error(), "发送邮件失败")
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
