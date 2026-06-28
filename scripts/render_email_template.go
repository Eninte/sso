//go:build ignore

// Package main 邮件模板渲染测试脚本
// 用于测试邮件模板渲染功能（不实际发送邮件）
package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/example/sso/internal/service/email"
)

func main() {
	// 命令行参数
	emailType := flag.String("type", "verification", "邮件类型: verification 或 password_reset")
	lang := flag.String("lang", "zh", "语言: zh 或 en")
	username := flag.String("username", "测试用户", "用户名")
	output := flag.String("output", "", "输出HTML文件路径（可选，不指定则输出到stdout）")
	flag.Parse()

	fmt.Printf("📧 邮件模板渲染测试\n")
	fmt.Printf("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")
	fmt.Printf("邮件类型: %s\n", *emailType)
	fmt.Printf("语言: %s\n", *lang)
	fmt.Printf("用户名: %s\n", *username)
	fmt.Printf("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n\n")

	// 获取当前工作目录
	wd, err := os.Getwd()
	if err != nil {
		log.Fatalf("❌ 获取工作目录失败: %v", err)
	}

	// 构建模板目录路径
	// go run scripts/render_email_template.go 从项目根目录运行
	templateDir := filepath.Join(wd, "internal/service/email/templates")

	// 如果从 scripts/ 目录运行，向上查找项目根目录
	if _, err := os.Stat(templateDir); os.IsNotExist(err) {
		templateDir = filepath.Join(filepath.Dir(wd), "internal/service/email/templates")
	}

	// 验证模板目录存在
	if _, err := os.Stat(templateDir); os.IsNotExist(err) {
		log.Fatalf("❌ 模板目录不存在: %s", templateDir)
	}

	// 初始化模板引擎（使用操作系统文件系统）
	templateConfig := email.TemplateConfig{
		TemplateFS:  nil, // 使用操作系统文件系统
		TemplateDir: templateDir,
		DefaultLang: "zh",
		CompanyName: "SSO服务",
		SupportEmail: "support@example.com",
		LogoURL:      "",
	}

	templateEngine, err := email.NewTemplateEngine(templateConfig)
	if err != nil {
		log.Fatalf("❌ 初始化模板引擎失败: %v", err)
	}

	// 准备模板数据
	data := email.TemplateData{
		Username:  *username,
		ActionURL: "https://example.com/verify?token=test123",
		ActionText: "验证邮箱",
		SecurityNote: "如果您没有注册账户，请忽略此邮件。",
		Year:      time.Now().Year(),
		Language:  *lang,
	}

	// 渲染邮件
	var subject, htmlBody string
	var renderErr error

	switch *emailType {
	case "verification":
		subject, htmlBody, renderErr = templateEngine.RenderVerificationEmail(*lang, data)
	case "password_reset":
		subject, htmlBody, renderErr = templateEngine.RenderPasswordResetEmail(*lang, data)
	default:
		log.Fatalf("❌ 未知的邮件类型: %s", *emailType)
	}

	if renderErr != nil {
		log.Fatalf("❌ 渲染邮件失败: %v", renderErr)
	}

	// 输出结果
	fmt.Printf("📨 邮件主题: %s\n\n", subject)
	fmt.Printf("📄 HTML内容:\n")
	fmt.Printf("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")
	
	if *output != "" {
		// 写入文件
		err := os.WriteFile(*output, []byte(htmlBody), 0644)
		if err != nil {
			log.Fatalf("❌ 写入文件失败: %v", err)
		}
		fmt.Printf("\n✅ 邮件已保存到: %s\n", *output)
	} else {
		// 输出到stdout
		fmt.Println(htmlBody)
	}
	
	fmt.Printf("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")
	fmt.Printf("✅ 邮件渲染成功！\n")
}
