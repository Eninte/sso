// Package service 邮件服务
// 负责发送各种通知邮件
package service

import (
	"context"
	"crypto/tls"
	"embed"
	"fmt"
	"log/slog"
	"net"
	"net/smtp"
	"time"

	apperrors "github.com/example/sso/internal/errors"
	"github.com/example/sso/internal/logging"
	"github.com/example/sso/internal/service/email"
	"github.com/example/sso/internal/util/serviceutil"
)

//go:embed email/templates email/templates/* email/templates/*/*
var templateFS embed.FS

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
	config         *EmailConfig
	sender         MailSender
	logger         *slog.Logger
	templateEngine *email.TemplateEngine
}

// NewEmailService 创建邮件服务
func NewEmailService(config *EmailConfig, sender ...MailSender) (*EmailService, error) {
	var s MailSender = &defaultMailSender{}
	if len(sender) > 0 && sender[0] != nil {
		s = sender[0]
	}

	// 初始化模板引擎（使用嵌入的文件系统）
	templateConfig := email.TemplateConfig{
		TemplateFS:   templateFS,
		TemplateDir:  "email/templates",
		DefaultLang:  "zh",
		CompanyName:  "SSO服务",
		SupportEmail: config.From,
	}

	templateEngine, err := email.NewTemplateEngine(templateConfig)
	if err != nil {
		return nil, fmt.Errorf("初始化模板引擎失败: %w", err)
	}

	return &EmailService{
		config:         config,
		sender:         s,
		logger:         slog.Default().With("component", "email"),
		templateEngine: templateEngine,
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
		// 阶段 4 安全增强：详细错误记录到日志，但向调用方返回通用错误
		// 避免 SMTP 错误消息（如 "550 Authentication failed for user noreply@example.com"）
		// 通过 handlerutil.WriteJSONError 暴露到 HTTP 响应，泄露 SMTP 用户名/服务器
		// T3：收件人邮箱脱敏记录
		s.logger.ErrorContext(ctx, "发送邮件失败", "to", logging.SanitizeEmail(to), "error", err)
		return apperrors.ErrEmailSendFailed
	}

	s.logger.InfoContext(ctx, "邮件发送成功", "to", logging.SanitizeEmail(to), "subject", subject)
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
	// 准备模板数据
	data := email.TemplateData{
		Username:   username,
		ActionURL:  verifyLink,
		ActionText: "验证邮箱",
	}

	// 渲染模板（默认使用中文）
	subject, body, err := s.templateEngine.RenderVerificationEmail("zh", data)
	if err != nil {
		return serviceutil.WrapServiceError("渲染验证邮件模板", err)
	}

	return s.SendEmail(ctx, to, subject, body)
}

// ============================================================================
// 密码重置邮件
// ============================================================================

// SendPasswordResetEmail 发送密码重置邮件
func (s *EmailService) SendPasswordResetEmail(ctx context.Context, to, username, resetLink string) error {
	// 准备模板数据
	data := email.TemplateData{
		Username:   username,
		ActionURL:  resetLink,
		ActionText: "重置密码",
	}

	// 渲染模板（默认使用中文）
	subject, body, err := s.templateEngine.RenderPasswordResetEmail("zh", data)
	if err != nil {
		return serviceutil.WrapServiceError("渲染密码重置邮件模板", err)
	}

	return s.SendEmail(ctx, to, subject, body)
}
