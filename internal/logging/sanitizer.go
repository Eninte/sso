// Package logging 日志脱敏
// 提供敏感信息脱敏功能，防止日志泄露
package logging

import (
	"strings"
)

// SanitizeEmail 脱敏邮箱地址
// "user@example.com" -> "u***@example.com"
func SanitizeEmail(email string) string {
	if email == "" {
		return ""
	}

	// 查找@符号位置
	atIndex := strings.Index(email, "@")
	if atIndex <= 0 {
		return email
	}

	// 获取用户名部分
	username := email[:atIndex]

	// 保留前1-3个字符
	var prefix string
	if len(username) <= 3 {
		prefix = username
	} else {
		prefix = username[:1]
	}

	return prefix + "***@" + email[atIndex+1:]
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
// 只对以1开头的11位手机号进行脱敏
func SanitizePhone(phone string) string {
	if len(phone) != 11 {
		return phone
	}

	if phone[0] != '1' || phone[1] < '3' || phone[1] > '9' {
		return phone
	}

	for _, c := range phone {
		if c < '0' || c > '9' {
			return phone
		}
	}

	return phone[:3] + "****" + phone[7:]
}
