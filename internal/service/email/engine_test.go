package email

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ============================================================================
// 测试辅助函数
// ============================================================================

// createTestTemplateDir 创建测试模板目录结构
func createTestTemplateDir(t *testing.T) string {
	tmpDir := t.TempDir()

	require.NoError(t, os.MkdirAll(filepath.Join(tmpDir, "verification"), 0755))
	require.NoError(t, os.MkdirAll(filepath.Join(tmpDir, "password_reset"), 0755))

	baseTemplate := `{{define "base"}}
<!DOCTYPE html>
<html lang="{{.Language}}">
<head>
    <meta charset="UTF-8">
    <title>{{.Subject}}</title>
</head>
<body>
    <div class="email-header">
        {{if .LogoURL}}<img src="{{.LogoURL}}" alt="{{.CompanyName}}" class="logo">{{end}}
        <h1>{{.CompanyName}}</h1>
    </div>
    <div class="email-content">
        {{template "content" .}}
    </div>
    <div class="email-footer">
        <div class="footer-content">
            {{if eq .Language "en"}}
            <p>&copy; {{.Year}} {{.CompanyName}}. All rights reserved.</p>
            {{if .SupportEmail}}<p><strong>Support:</strong> <a href="mailto:{{.SupportEmail}}">{{.SupportEmail}}</a></p>{{end}}
            {{else}}
            <p>&copy; {{.Year}} {{.CompanyName}}。版权所有。</p>
            {{if .SupportEmail}}<p><strong>支持：</strong><a href="mailto:{{.SupportEmail}}">{{.SupportEmail}}</a></p>{{end}}
            {{end}}
        </div>
        {{if .UnsubscribeURL}}
        <div class="footer-links">
            {{if eq .Language "en"}}<a href="{{.UnsubscribeURL}}">Unsubscribe</a>{{else}}<a href="{{.UnsubscribeURL}}">取消订阅</a>{{end}}
        </div>
        {{end}}
        {{if .FooterText}}<div class="copyright">{{.FooterText}}</div>{{end}}
    </div>
</body>
</html>
{{end}}`

	verificationZhTemplate := `{{define "content"}}
<h1>验证您的邮箱</h1>
<p>亲爱的 {{.Username}}，</p>
<p>感谢您注册我们的服务。请点击下方按钮验证您的邮箱地址。</p>
<a href="{{.ActionURL}}">{{.ActionText}}</a>
<p class="security-note">{{.SecurityNote}}</p>
{{end}}`

	verificationEnTemplate := `{{define "content"}}
<h1>Verify Your Email</h1>
<p>Dear {{.Username}},</p>
<p>Thank you for registering with us. Please click the button below to verify your email address.</p>
<a href="{{.ActionURL}}">{{.ActionText}}</a>
<p class="security-note">{{.SecurityNote}}</p>
{{end}}`

	passwordResetZhTemplate := `{{define "content"}}
<h1>重置您的密码</h1>
<p>亲爱的 {{.Username}}，</p>
<p>我们收到了您的密码重置请求。请点击下方按钮重置您的密码。</p>
<a href="{{.ActionURL}}">{{.ActionText}}</a>
<p class="security-note">{{.SecurityNote}}</p>
{{end}}`

	passwordResetEnTemplate := `{{define "content"}}
<h1>Reset Your Password</h1>
<p>Dear {{.Username}},</p>
<p>We received a request to reset your password. Please click the button below to reset your password.</p>
<a href="{{.ActionURL}}">{{.ActionText}}</a>
<p class="security-note">{{.SecurityNote}}</p>
{{end}}`

	require.NoError(t, os.WriteFile(filepath.Join(tmpDir, "base.html"), []byte(baseTemplate), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(tmpDir, "verification", "verification_zh.html"), []byte(verificationZhTemplate), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(tmpDir, "verification", "verification_en.html"), []byte(verificationEnTemplate), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(tmpDir, "password_reset", "password_reset_zh.html"), []byte(passwordResetZhTemplate), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(tmpDir, "password_reset", "password_reset_en.html"), []byte(passwordResetEnTemplate), 0644))

	return tmpDir
}

// ============================================================================
// 测试用例
// ============================================================================

// TestNewTemplateEngine_Success 测试成功创建模板引擎
func TestNewTemplateEngine_Success(t *testing.T) {
	tmpDir := createTestTemplateDir(t)

	config := TemplateConfig{
		TemplateDir:  tmpDir,
		DefaultLang:  "zh",
		LogoURL:      "https://example.com/logo.png",
		CompanyName:  "Test Company",
		SupportEmail: "support@example.com",
	}

	engine, err := NewTemplateEngine(config)
	require.NoError(t, err)
	assert.NotNil(t, engine)
	assert.Equal(t, config.TemplateDir, engine.config.TemplateDir)
	assert.Equal(t, config.DefaultLang, engine.config.DefaultLang)
}

// TestNewTemplateEngine_EmptyTemplateDir 测试空模板目录错误
func TestNewTemplateEngine_EmptyTemplateDir(t *testing.T) {
	config := TemplateConfig{
		TemplateDir: "",
		DefaultLang: "zh",
	}

	engine, err := NewTemplateEngine(config)
	assert.Error(t, err)
	assert.Nil(t, engine)
	assert.Contains(t, err.Error(), "cannot be empty")
}

// TestNewTemplateEngine_NonexistentDir 测试不存在的模板目录
func TestNewTemplateEngine_NonexistentDir(t *testing.T) {
	config := TemplateConfig{
		TemplateDir: "/nonexistent/path/to/templates",
		DefaultLang: "zh",
	}

	engine, err := NewTemplateEngine(config)
	assert.Error(t, err)
	assert.Nil(t, engine)
	assert.Contains(t, err.Error(), "does not exist")
}

// TestNewTemplateEngine_DefaultLanguage 测试默认语言设置
func TestNewTemplateEngine_DefaultLanguage(t *testing.T) {
	tmpDir := createTestTemplateDir(t)

	config := TemplateConfig{
		TemplateDir: tmpDir,
		DefaultLang: "", // 空值应该被设置为默认值
	}

	engine, err := NewTemplateEngine(config)
	require.NoError(t, err)
	assert.Equal(t, "zh", engine.config.DefaultLang)
}

// TestRenderVerificationEmail_Chinese 测试渲染中文验证邮件
func TestRenderVerificationEmail_Chinese(t *testing.T) {
	tmpDir := createTestTemplateDir(t)

	config := TemplateConfig{
		TemplateDir:  tmpDir,
		DefaultLang:  "zh",
		LogoURL:      "https://example.com/logo.png",
		CompanyName:  "Test Company",
		SupportEmail: "support@example.com",
	}

	engine, err := NewTemplateEngine(config)
	require.NoError(t, err)

	data := TemplateData{
		Username:     "testuser",
		ActionURL:    "https://example.com/verify?token=abc123",
		ActionText:   "验证邮箱",
		SecurityNote: "如果您没有注册账户，请忽略此邮件。",
	}

	subject, htmlBody, err := engine.RenderVerificationEmail("zh", data)
	require.NoError(t, err)
	assert.NotEmpty(t, subject)
	assert.NotEmpty(t, htmlBody)
	assert.Contains(t, htmlBody, "testuser")
	assert.Contains(t, htmlBody, "https://example.com/verify?token=abc123")
	assert.Contains(t, htmlBody, "验证邮箱")
}

// TestRenderVerificationEmail_English 测试渲染英文验证邮件
func TestRenderVerificationEmail_English(t *testing.T) {
	tmpDir := createTestTemplateDir(t)

	config := TemplateConfig{
		TemplateDir:  tmpDir,
		DefaultLang:  "zh",
		LogoURL:      "https://example.com/logo.png",
		CompanyName:  "Test Company",
		SupportEmail: "support@example.com",
	}

	engine, err := NewTemplateEngine(config)
	require.NoError(t, err)

	data := TemplateData{
		Username:     "testuser",
		ActionURL:    "https://example.com/verify?token=abc123",
		ActionText:   "Verify Email",
		SecurityNote: "If you did not register an account, please ignore this email.",
	}

	subject, htmlBody, err := engine.RenderVerificationEmail("en", data)
	require.NoError(t, err)
	assert.NotEmpty(t, subject)
	assert.NotEmpty(t, htmlBody)
	assert.Contains(t, htmlBody, "testuser")
	assert.Contains(t, htmlBody, "https://example.com/verify?token=abc123")
}

// TestRenderVerificationEmail_DefaultLanguageFallback 测试语言回退机制
func TestRenderVerificationEmail_DefaultLanguageFallback(t *testing.T) {
	tmpDir := createTestTemplateDir(t)

	config := TemplateConfig{
		TemplateDir:  tmpDir,
		DefaultLang:  "zh",
		LogoURL:      "https://example.com/logo.png",
		CompanyName:  "Test Company",
		SupportEmail: "support@example.com",
	}

	engine, err := NewTemplateEngine(config)
	require.NoError(t, err)

	data := TemplateData{
		Username:     "testuser",
		ActionURL:    "https://example.com/verify?token=abc123",
		ActionText:   "验证邮箱",
		SecurityNote: "如果您没有注册账户，请忽略此邮件。",
	}

	// 使用不存在的语言，应该回退到默认语言
	subject, htmlBody, err := engine.RenderVerificationEmail("fr", data)
	require.NoError(t, err)
	assert.NotEmpty(t, subject)
	assert.NotEmpty(t, htmlBody)
}

// TestRenderVerificationEmail_DefaultData 测试默认数据填充
func TestRenderVerificationEmail_DefaultData(t *testing.T) {
	tmpDir := createTestTemplateDir(t)

	config := TemplateConfig{
		TemplateDir:  tmpDir,
		DefaultLang:  "zh",
		LogoURL:      "https://example.com/logo.png",
		CompanyName:  "Test Company",
		SupportEmail: "support@example.com",
	}

	engine, err := NewTemplateEngine(config)
	require.NoError(t, err)

	data := TemplateData{
		Username:     "testuser",
		ActionURL:    "https://example.com/verify?token=abc123",
		ActionText:   "验证邮箱",
		SecurityNote: "如果您没有注册账户，请忽略此邮件。",
		// 不设置LogoURL、CompanyName、SupportEmail，应该使用默认值
	}

	subject, htmlBody, err := engine.RenderVerificationEmail("zh", data)
	require.NoError(t, err)
	assert.NotEmpty(t, subject)
	assert.NotEmpty(t, htmlBody)
	// 验证默认值被使用
	assert.Contains(t, htmlBody, "Test Company")
	assert.Contains(t, htmlBody, "support@example.com")
}

// TestRenderPasswordResetEmail_Chinese 测试渲染中文密码重置邮件
func TestRenderPasswordResetEmail_Chinese(t *testing.T) {
	tmpDir := createTestTemplateDir(t)

	config := TemplateConfig{
		TemplateDir:  tmpDir,
		DefaultLang:  "zh",
		LogoURL:      "https://example.com/logo.png",
		CompanyName:  "Test Company",
		SupportEmail: "support@example.com",
	}

	engine, err := NewTemplateEngine(config)
	require.NoError(t, err)

	data := TemplateData{
		Username:     "testuser",
		ActionURL:    "https://example.com/reset?token=xyz789",
		ActionText:   "重置密码",
		SecurityNote: "如果您没有请求重置密码，请忽略此邮件。",
	}

	subject, htmlBody, err := engine.RenderPasswordResetEmail("zh", data)
	require.NoError(t, err)
	assert.NotEmpty(t, subject)
	assert.NotEmpty(t, htmlBody)
	assert.Contains(t, htmlBody, "testuser")
	assert.Contains(t, htmlBody, "https://example.com/reset?token=xyz789")
	assert.Contains(t, htmlBody, "重置密码")
}

// TestRenderPasswordResetEmail_English 测试渲染英文密码重置邮件
func TestRenderPasswordResetEmail_English(t *testing.T) {
	tmpDir := createTestTemplateDir(t)

	config := TemplateConfig{
		TemplateDir:  tmpDir,
		DefaultLang:  "zh",
		LogoURL:      "https://example.com/logo.png",
		CompanyName:  "Test Company",
		SupportEmail: "support@example.com",
	}

	engine, err := NewTemplateEngine(config)
	require.NoError(t, err)

	data := TemplateData{
		Username:     "testuser",
		ActionURL:    "https://example.com/reset?token=xyz789",
		ActionText:   "Reset Password",
		SecurityNote: "If you did not request a password reset, please ignore this email.",
	}

	subject, htmlBody, err := engine.RenderPasswordResetEmail("en", data)
	require.NoError(t, err)
	assert.NotEmpty(t, subject)
	assert.NotEmpty(t, htmlBody)
	assert.Contains(t, htmlBody, "testuser")
	assert.Contains(t, htmlBody, "https://example.com/reset?token=xyz789")
}

// TestRenderPasswordResetEmail_DefaultLanguageFallback 测试密码重置邮件语言回退
func TestRenderPasswordResetEmail_DefaultLanguageFallback(t *testing.T) {
	tmpDir := createTestTemplateDir(t)

	config := TemplateConfig{
		TemplateDir:  tmpDir,
		DefaultLang:  "zh",
		LogoURL:      "https://example.com/logo.png",
		CompanyName:  "Test Company",
		SupportEmail: "support@example.com",
	}

	engine, err := NewTemplateEngine(config)
	require.NoError(t, err)

	data := TemplateData{
		Username:     "testuser",
		ActionURL:    "https://example.com/reset?token=xyz789",
		ActionText:   "重置密码",
		SecurityNote: "如果您没有请求重置密码，请忽略此邮件。",
	}

	// 使用不存在的语言，应该回退到默认语言
	subject, htmlBody, err := engine.RenderPasswordResetEmail("de", data)
	require.NoError(t, err)
	assert.NotEmpty(t, subject)
	assert.NotEmpty(t, htmlBody)
}

// TestRenderPasswordResetEmail_DefaultData 测试密码重置邮件默认数据
func TestRenderPasswordResetEmail_DefaultData(t *testing.T) {
	tmpDir := createTestTemplateDir(t)

	config := TemplateConfig{
		TemplateDir:  tmpDir,
		DefaultLang:  "zh",
		LogoURL:      "https://example.com/logo.png",
		CompanyName:  "Test Company",
		SupportEmail: "support@example.com",
	}

	engine, err := NewTemplateEngine(config)
	require.NoError(t, err)

	data := TemplateData{
		Username:     "testuser",
		ActionURL:    "https://example.com/reset?token=xyz789",
		ActionText:   "重置密码",
		SecurityNote: "如果您没有请求重置密码，请忽略此邮件。",
		// 不设置LogoURL、CompanyName、SupportEmail，应该使用默认值
	}

	subject, htmlBody, err := engine.RenderPasswordResetEmail("zh", data)
	require.NoError(t, err)
	assert.NotEmpty(t, subject)
	assert.NotEmpty(t, htmlBody)
	// 验证默认值被使用
	assert.Contains(t, htmlBody, "Test Company")
	assert.Contains(t, htmlBody, "support@example.com")
}

// TestTemplateData_XSSPrevention 测试XSS防护（html/template自动转义）
func TestTemplateData_XSSPrevention(t *testing.T) {
	tmpDir := createTestTemplateDir(t)

	config := TemplateConfig{
		TemplateDir:  tmpDir,
		DefaultLang:  "zh",
		LogoURL:      "https://example.com/logo.png",
		CompanyName:  "Test Company",
		SupportEmail: "support@example.com",
	}

	engine, err := NewTemplateEngine(config)
	require.NoError(t, err)

	// 尝试注入XSS脚本
	data := TemplateData{
		Username:     `<script>alert('XSS')</script>`,
		ActionURL:    "https://example.com/verify?token=abc123",
		ActionText:   "验证邮箱",
		SecurityNote: "如果您没有注册账户，请忽略此邮件。",
	}

	subject, htmlBody, err := engine.RenderVerificationEmail("zh", data)
	require.NoError(t, err)
	assert.NotEmpty(t, subject)
	assert.NotEmpty(t, htmlBody)
	// 验证脚本标签被转义
	assert.NotContains(t, htmlBody, "<script>")
	assert.Contains(t, htmlBody, "&lt;script&gt;")
}

// TestTemplateEngine_TemplateLoading 测试模板加载
func TestTemplateEngine_TemplateLoading(t *testing.T) {
	tmpDir := createTestTemplateDir(t)

	config := TemplateConfig{
		TemplateDir:  tmpDir,
		DefaultLang:  "zh",
		LogoURL:      "https://example.com/logo.png",
		CompanyName:  "Test Company",
		SupportEmail: "support@example.com",
	}

	engine, err := NewTemplateEngine(config)
	require.NoError(t, err)

	// 验证模板被正确加载
	assert.NotEmpty(t, engine.templates)
	assert.Greater(t, len(engine.templates), 0)
}

// TestTemplateEngine_ConcurrentAccess 测试并发访问
func TestTemplateEngine_ConcurrentAccess(t *testing.T) {
	tmpDir := createTestTemplateDir(t)

	config := TemplateConfig{
		TemplateDir:  tmpDir,
		DefaultLang:  "zh",
		LogoURL:      "https://example.com/logo.png",
		CompanyName:  "Test Company",
		SupportEmail: "support@example.com",
	}

	engine, err := NewTemplateEngine(config)
	require.NoError(t, err)

	data := TemplateData{
		Username:     "testuser",
		ActionURL:    "https://example.com/verify?token=abc123",
		ActionText:   "验证邮箱",
		SecurityNote: "如果您没有注册账户，请忽略此邮件。",
	}

	// 并发渲染邮件
	done := make(chan error, 10)
	for i := 0; i < 10; i++ {
		go func() {
			_, _, err := engine.RenderVerificationEmail("zh", data)
			done <- err
		}()
	}

	// 检查所有并发操作都成功
	for i := 0; i < 10; i++ {
		assert.NoError(t, <-done)
	}
}
