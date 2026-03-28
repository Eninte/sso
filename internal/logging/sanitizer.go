// Package logging 日志脱敏
// 提供敏感信息脱敏功能，防止日志泄露
package logging

import (
	"regexp"
)

var (
	// emailRegex 邮箱脱敏正则
	// 保留前1-3个字符和域名部分
	emailRegex = regexp.MustCompile(`^(.{1,3})?[^@]*@(.+)$`)

	// phoneRegex 手机号脱敏正则
	// 只匹配以1开头的11位手机号
	phoneRegex = regexp.MustCompile(`^(1\d{2})\d{4}(\d{4})$`)
)

// SanitizeEmail 脱敏邮箱地址
// "user@example.com" -> "u***@example.com"
func SanitizeEmail(email string) string {
	if email == "" {
		return ""
	}
	matches := emailRegex.FindStringSubmatch(email)
	if len(matches) != 3 {
		return email
	}
	prefix := matches[1]
	if prefix == "" {
		prefix = "u"
	}
	return prefix + "***@" + matches[2]
}

// SanitizeToken 脱敏Token
// 只显示前8位
func SanitizeToken(token string) string {
	if len(token) <= 8 {
		return "***"
	}
	return token[:8] + "..."
}

// SanitizePhone 脱敏手机号
// "13812345678" -> "138****5678"
func SanitizePhone(phone string) string {
	return phoneRegex.ReplaceAllString(phone, "$1****$2")
}
