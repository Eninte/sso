// Package service 邮件服务
// 负责发送各种通知邮件
package service

import (
	"context"
	"crypto/tls"
	"fmt"
	"log/slog"
	"net"
	"net/smtp"
	"time"

	"github.com/your-org/sso/internal/service/email"
	"github.com/your-org/sso/internal/util/serviceutil"
)

// ============================================================================
// 邮件配置
// ============================================================================

// EmailConfig 邮件配置
type EmailConfig struct {
	SMTPHost string // SMTP服务器地址
	SMTPPort int    // SMTP端口
	Username string // SMTP用户名
	Password string // SMTP密码
	From     string // 发件人地址
}

// ============================================================================
// 邮件发送接口
// ============================================================================

// MailSender 邮件发送接口
// 支持注入mock用于测试
type MailSender interface {
	Send(addr, from string, to []string, msg []byte, config *EmailConfig) error
}

// defaultMailSender 默认邮件发送实现
type defaultMailSender struct{}

func (d *defaultMailSender) Send(addr, from string, to []string, msg []byte, config *EmailConfig) error {
	if config.SMTPPort == 465 {
		return sendEmailSSL(addr, from, to, msg, config)
	}
	return sendEmailSTARTTLS(addr, from, to, msg, config)
}

// ============================================================================
// 邮件服务
// ============================================================================

// EmailService 邮件服务
type EmailService struct {
	config      *EmailConfig
	sender      MailSender
	logger      *slog.Logger
	templateMgr *email.TemplateManager
}

// NewEmailService 创建邮件服务
func NewEmailService(config *EmailConfig, sender ...MailSender) (*EmailService, error) {
	var s MailSender = &defaultMailSender{}
	if len(sender) > 0 && sender[0] != nil {
		s = sender[0]
	}

	// 初始化模板管理器
	templateMgr, err := email.NewTemplateManager()
	if err != nil {
		return nil, fmt.Errorf("初始化模板管理器失败: %w", err)
	}

	return &EmailService{
		config:      config,
		sender:      s,
		logger:      slog.Default().With("component", "email"),
		templateMgr: templateMgr,
	}, nil
}

// ============================================================================
// 邮件发送
// ============================================================================
// 邮件发送
// ============================================================================

// SendEmail 发送邮件
func (s *EmailService) SendEmail(ctx context.Context, to, subject, htmlBody string) error {
	// 构建邮件头
	headers := make(map[string]string)
	headers["From"] = s.config.From
	headers["To"] = to
	headers["Subject"] = subject
	headers["MIME-Version"] = "1.0"
	headers["Content-Type"] = "text/html; charset=UTF-8"
	headers["Date"] = time.Now().Format(time.RFC1123Z)

	// 构建邮件内容
	message := ""
	for k, v := range headers {
		message += fmt.Sprintf("%s: %s\r\n", k, v)
	}
	message += "\r\n" + htmlBody

	// 构建收件人列表
	recipients := []string{to}

	// 根据端口选择发送方式
	addr := fmt.Sprintf("%s:%d", s.config.SMTPHost, s.config.SMTPPort)

	var err error
	if s.config.SMTPPort == 465 {
		err = s.sender.Send(addr, s.config.From, recipients, []byte(message), s.config)
	} else {
		err = s.sender.Send(addr, s.config.From, recipients, []byte(message), s.config)
	}

	if err != nil {
		s.logger.ErrorContext(ctx, "发送邮件失败", "to", to, "error", err)
		return serviceutil.WrapServiceError("发送邮件", err)
	}

	s.logger.InfoContext(ctx, "邮件发送成功", "to", to, "subject", subject)
	return nil
}

// sendEmailSSL 使用 SSL/TLS 发送邮件 (端口 465)
func sendEmailSSL(addr, from string, to []string, msg []byte, config *EmailConfig) error {
	// 建立 TLS 连接
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return err
	}

	tlsConfig := &tls.Config{
		ServerName: host,
		MinVersion: tls.VersionTLS12,
	}

	conn, err := tls.Dial("tcp", addr, tlsConfig)
	if err != nil {
		return err
	}
	defer conn.Close()

	// 创建 SMTP 客户端
	client, err := smtp.NewClient(conn, host)
	if err != nil {
		return err
	}
	defer client.Close()

	// 认证
	if config.Username != "" {
		auth := smtp.PlainAuth("", config.Username, config.Password, host)
		if err = client.Auth(auth); err != nil {
			return err
		}
	}

	// 发送邮件
	if err = client.Mail(from); err != nil {
		return err
	}
	for _, addr := range to {
		if err = client.Rcpt(addr); err != nil {
			return err
		}
	}

	w, err := client.Data()
	if err != nil {
		return err
	}
	_, err = w.Write(msg)
	if err != nil {
		return err
	}
	err = w.Close()
	if err != nil {
		return err
	}

	return client.Quit()
}

// sendEmailSTARTTLS 使用 STARTTLS 发送邮件 (端口 587/25)
func sendEmailSTARTTLS(addr, from string, to []string, msg []byte, config *EmailConfig) error {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return err
	}

	// 认证
	var auth smtp.Auth
	if config.Username != "" {
		auth = smtp.PlainAuth("", config.Username, config.Password, host)
	}

	// 发送邮件 (smtp.SendMail 会自动处理 STARTTLS)
	// 安全设计：不允许 TLS 降级，证书验证失败直接返回错误
	return smtp.SendMail(addr, auth, from, to, msg)
}

// ============================================================================
// 验证邮件
// ============================================================================

// SendVerificationEmail 发送验证邮件
func (s *EmailService) SendVerificationEmail(ctx context.Context, to, username, verifyLink string) error {
	body, err := s.templateMgr.Render("verification.html", map[string]string{
		"Username":   username,
		"VerifyLink": verifyLink,
	})
	if err != nil {
		return serviceutil.WrapServiceError("渲染验证邮件模板", err)
	}

	return s.SendEmail(ctx, to, "验证您的邮箱 - SSO服务", body)
}

// ============================================================================
// 密码重置邮件
// ============================================================================

// SendPasswordResetEmail 发送密码重置邮件
func (s *EmailService) SendPasswordResetEmail(ctx context.Context, to, username, resetLink string) error {
	body, err := s.templateMgr.Render("reset.html", map[string]string{
		"Username":  username,
		"ResetLink": resetLink,
	})
	if err != nil {
		return serviceutil.WrapServiceError("渲染密码重置邮件模板", err)
	}

	return s.SendEmail(ctx, to, "重置您的密码 - SSO服务", body)
}
