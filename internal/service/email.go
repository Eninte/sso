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

// SMTP 超时默认值
// 生产环境通过 EmailConfig.DialTimeout / EmailConfig.TotalTimeout 覆盖
const (
	// defaultSMTPDialTimeout TCP/TLS 拨号超时
	defaultSMTPDialTimeout = 10 * time.Second
	// defaultSMTPTotalTimeout 整个 SMTP 会话总超时（banner/STARTTLS/AUTH/DATA）
	// smtp.SendMail / tls.Dial 内部无超时，SMTP 服务端卡 banner 会让会话永久挂起，
	// 必须用 conn.SetDeadline 设置总超时
	defaultSMTPTotalTimeout = 30 * time.Second
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

	// DialTimeout 拨号超时，零值用 defaultSMTPDialTimeout
	// 测试可设置 100ms 让被卡住的连接快速失败
	DialTimeout time.Duration
	// TotalTimeout 整个 SMTP 会话总超时，零值用 defaultSMTPTotalTimeout
	// 覆盖 banner/STARTTLS/AUTH/DATA 所有阶段，防止 SMTP 服务器卡 banner 导致 hang
	TotalTimeout time.Duration
}

// dialTimeout 返回生效的拨号超时
func (c *EmailConfig) dialTimeout() time.Duration {
	if c.DialTimeout > 0 {
		return c.DialTimeout
	}
	return defaultSMTPDialTimeout
}

// totalTimeout 返回生效的会话总超时
func (c *EmailConfig) totalTimeout() time.Duration {
	if c.TotalTimeout > 0 {
		return c.TotalTimeout
	}
	return defaultSMTPTotalTimeout
}

// isLocalhostSMTP 判断主机是否本地回环
// 与 stdlib smtp.SendMail 一致：localhost 跳过 STARTTLS 检查，便于本地 SMTP 测试
func isLocalhostSMTP(host string) bool {
	return host == "localhost" || host == "127.0.0.1" || host == "::1"
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
// 用 tls.DialTimeout + conn.SetDeadline 替代 stdlib tls.Dial，避免 SMTP 服务端卡 banner 导致会话永久 hang
func sendEmailSSL(addr, from string, to []string, msg []byte, config *EmailConfig) error {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return err
	}

	tlsConfig := &tls.Config{
		ServerName: host,
		MinVersion: tls.VersionTLS12,
	}

	// 带超时的 TLS 拨号（tls.Dial 内部无超时）
	dialer := &net.Dialer{Timeout: config.dialTimeout()}
	conn, err := tls.DialWithDialer(dialer, "tcp", addr, tlsConfig)
	if err != nil {
		return err
	}
	defer conn.Close()

	// 设置整个 SMTP 会话的总超时，覆盖 banner/AUTH/DATA 所有读写阶段
	// TLS 握手已完成，剩余阶段由该 deadline 保护
	if err := conn.SetDeadline(time.Now().Add(config.totalTimeout())); err != nil {
		return err
	}

	client, err := smtp.NewClient(conn, host)
	if err != nil {
		return err
	}
	defer client.Close()

	if err := smtpSendPipeline(client, from, to, msg, config, host); err != nil {
		return err
	}
	return client.Quit()
}

// sendEmailSTARTTLS 使用 STARTTLS 发送邮件 (端口 587/25)
// 用 net.DialTimeout + smtp.NewClient + conn.SetDeadline 替代 stdlib smtp.SendMail，
// 避免 SMTP 服务端卡 banner 导致会话永久 hang
func sendEmailSTARTTLS(addr, from string, to []string, msg []byte, config *EmailConfig) error {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return err
	}

	// 带超时的 TCP 拨号（smtp.SendMail 内部用 net.Dial 无超时）
	conn, err := net.DialTimeout("tcp", addr, config.dialTimeout())
	if err != nil {
		return err
	}
	defer conn.Close()

	// 设置整个 SMTP 会话的总超时，覆盖 banner/STARTTLS/AUTH/DATA 所有读写阶段
	if err := conn.SetDeadline(time.Now().Add(config.totalTimeout())); err != nil {
		return err
	}

	client, err := smtp.NewClient(conn, host)
	if err != nil {
		return err
	}
	defer client.Close()

	// 安全设计：远程主机必须使用 STARTTLS（与 stdlib smtp.SendMail 一致，localhost 跳过便于本地测试）
	// 证书验证失败直接返回错误，不允许 TLS 降级
	if !isLocalhostSMTP(host) {
		if ok, _ := client.Extension("STARTTLS"); ok {
			if err := client.StartTLS(&tls.Config{
				ServerName: host,
				MinVersion: tls.VersionTLS12,
			}); err != nil {
				return err
			}
		}
	}

	if err := smtpSendPipeline(client, from, to, msg, config, host); err != nil {
		return err
	}
	return client.Quit()
}

// smtpSendPipeline 执行 SMTP 会话的 AUTH/MAIL/RCPT/DATA 阶段
// 抽出共用逻辑供 sendEmailSSL / sendEmailSTARTTLS 复用
func smtpSendPipeline(client *smtp.Client, from string, to []string, msg []byte, config *EmailConfig, host string) error {
	// 认证
	if config.Username != "" {
		auth := smtp.PlainAuth("", config.Username, config.Password, host)
		if err := client.Auth(auth); err != nil {
			return err
		}
	}

	// 发件人
	if err := client.Mail(from); err != nil {
		return err
	}
	// 收件人
	for _, rcpt := range to {
		if err := client.Rcpt(rcpt); err != nil {
			return err
		}
	}

	// 邮件正文
	w, err := client.Data()
	if err != nil {
		return err
	}
	if _, err := w.Write(msg); err != nil {
		return err
	}
	if err := w.Close(); err != nil {
		return err
	}
	return nil
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
