// Package main 邮件模板渲染测试脚本
// 用于测试邮件模板渲染功能（不实际发送邮件）
package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/your-org/sso/internal/service/email"
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

	// 初始化模板引擎
	templateConfig := email.TemplateConfig{
		TemplateDir:  "internal/service/email/templates",
		DefaultLang:  "zh",
		CompanyName:  "SSO服务",
		SupportEmail: "support@example.com",
		LogoURL:      "",
	}

	templateEngine, err := email.NewTemplateEngine(templateConfig)
	if err != nil {
		log.Fatalf("❌ 初始化模板引擎失败: %v", err)
	}

	// 准备模板数据
	data := email.TemplateData{
		Username: *username,
		Year:     time.Now().Year(),
	}

	var subject, htmlBody string

	// 根据类型渲染模板
	switch *emailType {
	case "verification":
		data.ActionURL = "https://sso.example.com/verify?token=test-token-123456"
		if *lang == "en" {
			data.ActionText = "Verify Email"
		} else {
			data.ActionText = "验证邮箱"
		}
		subject, htmlBody, err = templateEngine.RenderVerificationEmail(*lang, data)
		if err != nil {
			log.Fatalf("❌ 渲染验证邮件模板失败: %v", err)
		}

	case "password_reset":
		data.ActionURL = "https://sso.example.com/reset?token=test-reset-token-789"
		if *lang == "en" {
			data.ActionText = "Reset Password"
		} else {
			data.ActionText = "重置密码"
		}
		subject, htmlBody, err = templateEngine.RenderPasswordResetEmail(*lang, data)
		if err != nil {
			log.Fatalf("❌ 渲染密码重置邮件模板失败: %v", err)
		}

	default:
		log.Fatalf("错误: 不支持的邮件类型 '%s'，支持的类型: verification, password_reset", *emailType)
	}

	fmt.Printf("✅ 模板渲染成功！\n\n")
	fmt.Printf("主题: %s\n", subject)
	fmt.Printf("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n\n")

	// 输出HTML
	if *output != "" {
		// 写入文件
		if err := os.WriteFile(*output, []byte(htmlBody), 0644); err != nil {
			log.Fatalf("❌ 写入文件失败: %v", err)
		}
		fmt.Printf("📄 HTML已保存到: %s\n", *output)
		fmt.Printf("💡 在浏览器中打开该文件查看效果\n")
	} else {
		// 输出到stdout
		fmt.Println("HTML内容:")
		fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
		fmt.Println(htmlBody)
	}

	fmt.Printf("\n🎉 渲染完成！\n")
}
