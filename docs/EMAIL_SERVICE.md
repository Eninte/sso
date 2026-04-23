# 邮件服务文档

## 概述

SSO服务集成了完整的邮件通知系统，用于发送验证邮件、密码重置邮件等关键通知。邮件模板采用现代化设计，支持响应式布局、深色模式和多语言。

## 功能特性

### 邮件类型

1. **验证邮件** - 用户注册后发送邮箱验证链接
2. **密码重置邮件** - 用户忘记密码时发送重置链接

### 设计特点

- ✅ **响应式设计** - 自动适配桌面和移动设备
- ✅ **深色模式支持** - 自动检测系统主题偏好
- ✅ **多语言支持** - 支持中文和英文
- ✅ **现代化配色** - 专业的蓝色主题
- ✅ **高对比度** - 按钮文字清晰可见
- ✅ **邮件客户端兼容** - 支持Gmail、Outlook、QQ邮箱、网易邮箱等

### 技术架构

```
EmailService (邮件服务)
    ↓
TemplateEngine (模板引擎)
    ↓
Templates (模板文件)
    ├── base.html (基础布局)
    ├── verification/ (验证邮件)
    │   ├── verification_zh.html
    │   └── verification_en.html
    └── password_reset/ (密码重置)
        ├── password_reset_zh.html
        └── password_reset_en.html
```

## SMTP配置

### 环境变量

在 `.env` 文件中配置以下变量：

```env
# SMTP服务器配置
SMTP_HOST=smtp.example.com          # SMTP服务器地址
SMTP_PORT=25                         # SMTP端口 (25/465/587)
SMTP_USER=noreply@example.com       # SMTP用户名
SMTP_PASSWORD=your-password-here     # SMTP密码或授权码
SMTP_FROM=noreply@example.com       # 发件人邮箱地址
```

### 端口说明

| 端口 | 协议 | 加密方式 | 说明 |
|------|------|---------|------|
| 25 | SMTP | STARTTLS | 标准SMTP端口，支持STARTTLS升级 |
| 465 | SMTPS | SSL/TLS | 使用SSL/TLS加密的SMTP |
| 587 | SMTP | STARTTLS | 提交端口，推荐使用 |

### 常见SMTP服务商配置

#### 阿里云企业邮箱

```env
SMTP_HOST=smtp.qiye.aliyun.com
SMTP_PORT=25
SMTP_USER=your-email@yourdomain.com
SMTP_PASSWORD=your-authorization-code
SMTP_FROM=your-email@yourdomain.com
```

**注意：** 阿里云企业邮箱需要使用授权码而非账户密码。

#### Gmail

```env
SMTP_HOST=smtp.gmail.com
SMTP_PORT=587
SMTP_USER=your-email@gmail.com
SMTP_PASSWORD=your-app-password
SMTP_FROM=your-email@gmail.com
```

**注意：** Gmail需要生成应用专用密码。

#### QQ邮箱

```env
SMTP_HOST=smtp.qq.com
SMTP_PORT=587
SMTP_USER=your-email@qq.com
SMTP_PASSWORD=your-authorization-code
SMTP_FROM=your-email@qq.com
```

**注意：** QQ邮箱需要在设置中开启SMTP服务并生成授权码。

#### SendGrid

```env
SMTP_HOST=smtp.sendgrid.net
SMTP_PORT=587
SMTP_USER=apikey
SMTP_PASSWORD=your-sendgrid-api-key
SMTP_FROM=your-verified-sender@example.com
```

## 邮件模板

### 模板结构

所有邮件模板基于 `base.html` 布局，使用模板继承机制：

```html
<!-- base.html -->
{{define "base"}}
<!DOCTYPE html>
<html>
<head>
    <!-- 样式定义 -->
</head>
<body>
    <div class="email-header">
        <!-- 头部 -->
    </div>
    <div class="email-content">
        {{template "content" .}}
    </div>
    <div class="email-footer">
        <!-- 页脚 -->
    </div>
</body>
</html>
{{end}}

<!-- verification_zh.html -->
{{define "content"}}
<h2>您好，{{.Username}}！</h2>
<p>感谢您注册我们的服务...</p>
<a href="{{.ActionURL}}" class="button">{{.ActionText}}</a>
{{end}}
```

### 模板数据

邮件模板接收以下数据字段：

```go
type TemplateData struct {
    // 通用字段
    Subject       string // 邮件主题
    PreheaderText string // 预览文本
    Language      string // 语言代码（zh, en）
    
    // 品牌字段
    LogoURL      string // Logo URL
    CompanyName  string // 公司名称
    SupportEmail string // 支持邮箱
    
    // 内容字段
    Username   string // 用户名
    ActionURL  string // 行动按钮URL
    ActionText string // 行动按钮文本
    
    // 安全提示
    SecurityNote string // 安全提示信息
    
    // 页脚字段
    Year           int    // 当前年份
    FooterText     string // 页脚文本
    UnsubscribeURL string // 取消订阅链接
}
```

### 配色方案

#### 主色调

```css
/* 主蓝色 */
--primary-blue: #1e88e5;

/* 深蓝色（悬停） */
--primary-blue-dark: #1565c0;

/* 更深蓝色（点击） */
--primary-blue-darker: #0d47a1;
```

#### 按钮样式

```css
.button {
    background: #1e88e5;
    color: #ffffff !important;
    padding: 14px 32px;
    border-radius: 6px;
    font-weight: 600;
}

.button:hover {
    background: #1565c0;
}
```

#### 安全提示

```css
.security-note {
    background-color: #fff8e1;
    border-left: 4px solid #ffa726;
    color: #e65100;
}
```

## 开发指南

### 添加新邮件类型

1. **创建模板文件**

```bash
# 创建目录
mkdir -p internal/service/email/templates/welcome

# 创建中文模板
cat > internal/service/email/templates/welcome/welcome_zh.html << 'EOF'
{{define "content"}}
<h2>欢迎，{{.Username}}！</h2>
<p>感谢您加入我们...</p>
{{end}}
EOF

# 创建英文模板
cat > internal/service/email/templates/welcome/welcome_en.html << 'EOF'
{{define "content"}}
<h2>Welcome, {{.Username}}!</h2>
<p>Thank you for joining us...</p>
{{end}}
EOF
```

2. **在TemplateEngine中添加渲染方法**

编辑 `internal/service/email/engine.go`：

```go
// RenderWelcomeEmail 渲染欢迎邮件
func (e *TemplateEngine) RenderWelcomeEmail(lang string, data TemplateData) (subject, htmlBody string, err error) {
    return e.renderEmailTemplate(
        "welcome",
        lang,
        data,
        "Welcome to SSO Service",
        "欢迎使用SSO服务",
    )
}
```

3. **在EmailService中添加发送方法**

编辑 `internal/service/email.go`：

```go
// SendWelcomeEmail 发送欢迎邮件
func (s *EmailService) SendWelcomeEmail(ctx context.Context, to, username string) error {
    data := email.TemplateData{
        Username:   username,
        ActionURL:  "https://example.com/dashboard",
        ActionText: "开始使用",
    }
    
    subject, body, err := s.templateEngine.RenderWelcomeEmail("zh", data)
    if err != nil {
        return serviceutil.WrapServiceError("渲染欢迎邮件模板", err)
    }
    
    return s.SendEmail(ctx, to, subject, body)
}
```

### 测试邮件发送

使用提供的测试工具：

```bash
# 测试验证邮件
go run scripts/test_email.go \
  -to recipient@example.com \
  -type verification \
  -username "测试用户"

# 测试密码重置邮件
go run scripts/test_email.go \
  -to recipient@example.com \
  -type password_reset \
  -username "测试用户"
```

### 渲染模板预览

不实际发送邮件，仅渲染HTML：

```bash
# 渲染中文验证邮件
go run scripts/render_email_template.go \
  -type verification \
  -lang zh \
  -username "测试用户" \
  -output /tmp/email.html

# 在浏览器中打开预览
open /tmp/email.html
```

## 故障排查

### SMTP认证失败

**错误信息：** `526 Authentication failure`

**可能原因：**
1. SMTP密码错误
2. 需要使用授权码而非账户密码
3. IP地址未在白名单中
4. 账户未开启SMTP服务

**解决方案：**
1. 检查SMTP配置是否正确
2. 使用授权码（阿里云、QQ邮箱等）
3. 在邮箱管理后台添加IP白名单
4. 开启SMTP服务并生成授权码

### 邮件发送成功但未收到

**可能原因：**
1. 邮件被标记为垃圾邮件
2. 发件人域名未配置SPF/DKIM
3. 收件人邮箱已满

**解决方案：**
1. 检查垃圾邮件文件夹
2. 配置SPF和DKIM记录
3. 使用专业的邮件服务商（SendGrid、Mailgun等）

### 邮件样式显示异常

**可能原因：**
1. 邮件客户端不支持某些CSS特性
2. 图片被阻止加载

**解决方案：**
1. 使用内联样式
2. 避免使用复杂的CSS特性
3. 提供纯文本版本

### 模板渲染失败

**错误信息：** `template not found`

**可能原因：**
1. 模板文件路径错误
2. 模板文件不存在
3. 模板语法错误

**解决方案：**
1. 检查模板文件路径
2. 确认模板文件存在
3. 验证模板语法正确

## 性能优化

### 异步发送

邮件发送操作应该异步执行，避免阻塞主流程：

```go
// 异步发送验证邮件
go func() {
    if err := emailSvc.SendVerificationEmail(ctx, user.Email, user.Username, verifyLink); err != nil {
        logger.Error("发送验证邮件失败", "error", err)
    }
}()
```

### 重试机制

对于临时性失败，实现重试机制：

```go
func sendEmailWithRetry(ctx context.Context, emailSvc *EmailService, to, subject, body string) error {
    maxRetries := 3
    for i := 0; i < maxRetries; i++ {
        err := emailSvc.SendEmail(ctx, to, subject, body)
        if err == nil {
            return nil
        }
        
        if i < maxRetries-1 {
            time.Sleep(time.Second * time.Duration(i+1))
        }
    }
    return fmt.Errorf("发送邮件失败，已重试%d次", maxRetries)
}
```

### 批量发送

对于批量邮件，使用连接池和并发控制：

```go
func sendBulkEmails(ctx context.Context, emailSvc *EmailService, recipients []string) {
    sem := make(chan struct{}, 10) // 限制并发数为10
    var wg sync.WaitGroup
    
    for _, recipient := range recipients {
        wg.Add(1)
        go func(to string) {
            defer wg.Done()
            sem <- struct{}{}        // 获取信号量
            defer func() { <-sem }() // 释放信号量
            
            if err := emailSvc.SendEmail(ctx, to, "主题", "内容"); err != nil {
                log.Printf("发送邮件到 %s 失败: %v", to, err)
            }
        }(recipient)
    }
    
    wg.Wait()
}
```

## 监控与日志

### 日志记录

邮件服务会记录以下日志：

```
# 成功日志
INFO 邮件发送成功 component=email to=user@example.com subject="验证您的邮箱 - SSO服务"

# 失败日志
ERROR 发送邮件失败 component=email to=user@example.com error="526 Authentication failure"
```

### 指标监控

建议监控以下指标：

- 邮件发送成功率
- 邮件发送延迟
- SMTP连接失败次数
- 模板渲染失败次数

## 安全建议

### 敏感信息保护

1. **不要在邮件中包含敏感信息**
   - 不要发送密码
   - 不要发送完整的信用卡号
   - 使用短期有效的Token

2. **使用HTTPS链接**
   - 所有邮件中的链接必须使用HTTPS
   - 验证链接应该有过期时间

3. **防止邮件欺诈**
   - 配置SPF记录
   - 配置DKIM签名
   - 配置DMARC策略

### SMTP凭据管理

1. **使用环境变量**
   - 不要在代码中硬编码SMTP密码
   - 使用环境变量或密钥管理服务

2. **使用授权码**
   - 优先使用应用专用密码/授权码
   - 定期轮换SMTP密码

3. **限制访问权限**
   - 仅授予必要的SMTP权限
   - 使用专用的发件账户

## 最佳实践

### 邮件内容

1. **清晰的主题行**
   - 简洁明了
   - 包含关键信息
   - 避免垃圾邮件触发词

2. **友好的正文**
   - 使用用户的名字
   - 说明邮件目的
   - 提供明确的行动指引

3. **安全提示**
   - 告知链接有效期
   - 提醒用户注意安全
   - 提供联系方式

### 用户体验

1. **响应式设计**
   - 确保在移动设备上可读
   - 按钮足够大，易于点击
   - 文字大小适中

2. **可访问性**
   - 使用语义化HTML
   - 提供足够的颜色对比度
   - 支持屏幕阅读器

3. **品牌一致性**
   - 使用统一的配色
   - 包含公司Logo
   - 保持视觉风格一致

## 参考资料

- [Go html/template文档](https://pkg.go.dev/html/template)
- [SMTP协议规范](https://tools.ietf.org/html/rfc5321)
- [邮件HTML最佳实践](https://www.campaignmonitor.com/css/)
- [SPF配置指南](https://www.spf-record.com/)
- [DKIM配置指南](https://www.dkim.org/)

## 更新日志

### 2026-04-22

- ✅ 重构邮件模板系统，采用模板继承机制
- ✅ 优化配色方案，从紫色改为蓝色主题
- ✅ 提升按钮对比度，确保文字清晰可见
- ✅ 添加邮件测试工具
- ✅ 完善文档和使用指南

---

**维护者：** SSO开发团队  
**最后更新：** 2026-04-22
