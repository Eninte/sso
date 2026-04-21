// Package email 邮件服务
// 提供邮件模板引擎，支持响应式设计、深色模式、多语言等功能
package email

import (
	"bytes"
	"fmt"
	"html/template"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
)

// ============================================================================
// 数据结构定义
// ============================================================================

// TemplateData 邮件模板数据
// 包含邮件渲染所需的所有数据字段
type TemplateData struct {
	// 通用字段
	Subject       string // 邮件主题
	PreheaderText string // 预览文本（在邮件客户端显示的摘要）
	Language      string // 语言代码（zh, en）

	// 品牌字段
	LogoURL      string // Logo URL或Base64编码的图片
	CompanyName  string // 公司名称
	SupportEmail string // 支持邮箱

	// 内容字段
	Title      string // 邮件标题
	Body       string // 主要内容（HTML格式）
	ActionURL  string // 行动按钮URL
	ActionText string // 行动按钮文本

	// 页脚字段
	FooterText     string // 页脚文本
	UnsubscribeURL string // 取消订阅链接

	// 安全提示
	SecurityNote string // 安全提示信息

	// 其他字段
	Username string // 用户名
	Year     int    // 当前年份（用于页脚版权）
}

// TemplateConfig 模板引擎配置
// 定义模板引擎的运行参数
type TemplateConfig struct {
	TemplateDir  string // 模板文件目录路径
	DefaultLang  string // 默认语言代码（zh或en）
	LogoURL      string // 默认Logo URL
	CompanyName  string // 默认公司名称
	SupportEmail string // 默认支持邮箱
}

// TemplateEngine 邮件模板引擎
// 负责加载、管理和渲染邮件模板
type TemplateEngine struct {
	config    TemplateConfig
	templates map[string]*template.Template
	mu        sync.RWMutex
	logger    *slog.Logger
}

// ============================================================================
// 构造函数
// ============================================================================

// NewTemplateEngine 创建邮件模板引擎
// 加载指定目录下的所有HTML模板文件
//
// 参数:
//   - config: 模板引擎配置
//
// 返回:
//   - *TemplateEngine: 初始化后的模板引擎
//   - error: 初始化过程中的错误（如模板目录不存在、模板解析失败等）
func NewTemplateEngine(config TemplateConfig) (*TemplateEngine, error) {
	// 验证配置
	if config.TemplateDir == "" {
		return nil, fmt.Errorf("template directory cannot be empty")
	}

	if config.DefaultLang == "" {
		config.DefaultLang = "zh"
	}

	// 检查模板目录是否存在
	if _, err := os.Stat(config.TemplateDir); err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("template directory does not exist: %s", config.TemplateDir)
		}
		return nil, fmt.Errorf("cannot access template directory: %w", err)
	}

	engine := &TemplateEngine{
		config:    config,
		templates: make(map[string]*template.Template),
		logger:    slog.Default().With("component", "email_engine"),
	}

	// 加载所有模板
	if err := engine.loadTemplates(); err != nil {
		return nil, fmt.Errorf("failed to load templates: %w", err)
	}

	return engine, nil
}

// ============================================================================
// 私有方法
// ============================================================================

// loadTemplates 加载模板目录下的所有HTML文件
// 递归遍历模板目录，解析所有.html文件
func (e *TemplateEngine) loadTemplates() error {
	return filepath.Walk(e.config.TemplateDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// 跳过目录
		if info.IsDir() {
			return nil
		}

		// 只处理HTML文件
		if filepath.Ext(path) != ".html" {
			return nil
		}

		// 获取相对路径作为模板名称
		relPath, err := filepath.Rel(e.config.TemplateDir, path)
		if err != nil {
			return fmt.Errorf("failed to get relative path: %w", err)
		}

		// 规范化路径分隔符（Windows兼容性）
		templateName := filepath.ToSlash(relPath)

		// 解析模板
		tmpl, err := template.ParseFiles(path)
		if err != nil {
			return fmt.Errorf("failed to parse template %s: %w", templateName, err)
		}

		e.mu.Lock()
		e.templates[templateName] = tmpl
		e.mu.Unlock()

		e.logger.Debug("template loaded", "name", templateName, "path", path)

		return nil
	})
}

// getTemplate 获取指定名称的模板
// 如果模板不存在，返回错误
func (e *TemplateEngine) getTemplate(name string) (*template.Template, error) {
	e.mu.RLock()
	defer e.mu.RUnlock()

	tmpl, ok := e.templates[name]
	if !ok {
		return nil, fmt.Errorf("template not found: %s", name)
	}

	return tmpl, nil
}

// ============================================================================
// 公共方法
// ============================================================================

// renderEmailTemplate 通用邮件模板渲染方法
// 根据指定语言和模板类型渲染邮件模板
//
// 参数:
//   - templateType: 模板类型（如"verification"、"password_reset"）
//   - lang: 语言代码（zh或en）
//   - data: 模板数据
//   - defaultSubjectEN: 英文默认主题
//   - defaultSubjectZH: 中文默认主题
//
// 返回:
//   - subject: 邮件主题
//   - htmlBody: 邮件HTML内容
//   - error: 渲染过程中的错误
func (e *TemplateEngine) renderEmailTemplate(
	templateType string,
	lang string,
	data TemplateData,
	defaultSubjectEN string,
	defaultSubjectZH string,
) (subject, htmlBody string, err error) {
	// 设置默认语言
	if lang == "" {
		lang = e.config.DefaultLang
	}

	// 设置默认数据
	if data.Language == "" {
		data.Language = lang
	}
	if data.LogoURL == "" {
		data.LogoURL = e.config.LogoURL
	}
	if data.CompanyName == "" {
		data.CompanyName = e.config.CompanyName
	}
	if data.SupportEmail == "" {
		data.SupportEmail = e.config.SupportEmail
	}

	// 确定模板文件名
	templateName := fmt.Sprintf("%s/%s_%s.html", templateType, templateType, lang)

	// 如果指定语言的模板不存在，回退到默认语言
	if _, err := e.getTemplate(templateName); err != nil {
		e.logger.Warn("template not found, using default language",
			"requested", templateName,
			"default", e.config.DefaultLang)
		templateName = fmt.Sprintf("%s/%s_%s.html", templateType, templateType, e.config.DefaultLang)
	}

	// 获取模板
	tmpl, err := e.getTemplate(templateName)
	if err != nil {
		return "", "", fmt.Errorf("failed to get %s template: %w", templateType, err)
	}

	// 渲染模板
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return "", "", fmt.Errorf("failed to render %s template: %w", templateType, err)
	}

	// 如果data中没有设置Subject，使用默认值
	if data.Subject == "" {
		if lang == "en" {
			data.Subject = defaultSubjectEN
		} else {
			data.Subject = defaultSubjectZH
		}
	}

	return data.Subject, buf.String(), nil
}

// RenderVerificationEmail 渲染验证邮件
// 根据指定语言渲染验证邮件模板
//
// 参数:
//   - lang: 语言代码（zh或en）
//   - data: 模板数据
//
// 返回:
//   - subject: 邮件主题
//   - htmlBody: 邮件HTML内容
//   - error: 渲染过程中的错误
func (e *TemplateEngine) RenderVerificationEmail(lang string, data TemplateData) (subject, htmlBody string, err error) {
	return e.renderEmailTemplate(
		"verification",
		lang,
		data,
		"Verify Your Email - SSO Service",
		"验证您的邮箱 - SSO服务",
	)
}

// RenderPasswordResetEmail 渲染密码重置邮件
// 根据指定语言渲染密码重置邮件模板
//
// 参数:
//   - lang: 语言代码（zh或en）
//   - data: 模板数据
//
// 返回:
//   - subject: 邮件主题
//   - htmlBody: 邮件HTML内容
//   - error: 渲染过程中的错误
func (e *TemplateEngine) RenderPasswordResetEmail(lang string, data TemplateData) (subject, htmlBody string, err error) {
	return e.renderEmailTemplate(
		"password_reset",
		lang,
		data,
		"Reset Your Password - SSO Service",
		"重置您的密码 - SSO服务",
	)
}
