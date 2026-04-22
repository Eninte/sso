// Package main 邮件测试脚本
// 用于测试邮件模板渲染和发送功能
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"

	"github.com/joho/godotenv"
	"github.com/your-org/sso/internal/service"
)

func main() {
	// 命令行参数
	to := flag.String("to", "", "收件人邮箱地址 (必填)")
	emailType := flag.String("type", "verification", "邮件类型: verification 或 password_reset")
	username := flag.String("username", "测试用户", "用户名")
	envFile := flag.String("env", ".env.test", "环境配置文件路径")
	flag.Parse()

	if *to == "" {
		log.Fatal("错误: 必须指定收件人邮箱地址 (-to)")
	}

	// 加载环境变量
	if err := godotenv.Load(*envFile); err != nil {
		log.Printf("警告: 无法加载 %s: %v", *envFile, err)
	}

	// 读取SMTP配置
	config := &service.EmailConfig{
		SMTPHost: getEnv("SMTP_HOST", ""),
		SMTPPort: getEnvInt("SMTP_PORT", 465),
		Username: getEnv("SMTP_USER", ""),
		Password: getEnv("SMTP_PASSWORD", ""),
		From:     getEnv("SMTP_FROM", ""),
	}

	// 验证配置
	if config.SMTPHost == "" || config.Username == "" || config.Password == "" {
		log.Fatal("错误: SMTP配置不完整，请检查环境变量")
	}

	fmt.Printf("📧 邮件测试工具\n")
	fmt.Printf("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")
	fmt.Printf("收件人: %s\n", *to)
	fmt.Printf("邮件类型: %s\n", *emailType)
	fmt.Printf("用户名: %s\n", *username)
	fmt.Printf("SMTP服务器: %s:%d\n", config.SMTPHost, config.SMTPPort)
	fmt.Printf("发件人: %s\n", config.From)
	fmt.Printf("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n\n")

	// 创建邮件服务
	emailSvc, err := service.NewEmailService(config)
	if err != nil {
		log.Fatalf("创建邮件服务失败: %v", err)
	}

	ctx := context.Background()

	// 根据类型发送邮件
	switch *emailType {
	case "verification":
		verifyLink := "https://sso.example.com/verify?token=test-token-123456"
		fmt.Printf("📤 正在发送验证邮件...\n")
		err = emailSvc.SendVerificationEmail(ctx, *to, *username, verifyLink)
		if err != nil {
			log.Fatalf("❌ 发送验证邮件失败: %v", err)
		}
		fmt.Printf("✅ 验证邮件发送成功！\n")
		fmt.Printf("   验证链接: %s\n", verifyLink)

	case "password_reset":
		resetLink := "https://sso.example.com/reset?token=test-reset-token-789"
		fmt.Printf("📤 正在发送密码重置邮件...\n")
		err = emailSvc.SendPasswordResetEmail(ctx, *to, *username, resetLink)
		if err != nil {
			log.Fatalf("❌ 发送密码重置邮件失败: %v", err)
		}
		fmt.Printf("✅ 密码重置邮件发送成功！\n")
		fmt.Printf("   重置链接: %s\n", resetLink)

	default:
		log.Fatalf("错误: 不支持的邮件类型 '%s'，支持的类型: verification, password_reset", *emailType)
	}

	fmt.Printf("\n🎉 测试完成！请检查邮箱 %s\n", *to)
}

// getEnv 获取环境变量，如果不存在则返回默认值
func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

// getEnvInt 获取整数类型的环境变量
func getEnvInt(key string, defaultValue int) int {
	if value := os.Getenv(key); value != "" {
		var intValue int
		if _, err := fmt.Sscanf(value, "%d", &intValue); err == nil {
			return intValue
		}
	}
	return defaultValue
}
