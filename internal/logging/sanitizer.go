// Package logging 日志脱敏
// 提供敏感信息脱敏功能，防止日志泄露
package logging

import (
	"net/url"
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

// SanitizeSecret 脱敏密钥/密码字段
// 永远返回固定掩码，避免泄露长度信息
func SanitizeSecret() string {
	return "***"
}

// SanitizeDBURL 脱敏数据库连接 URL 中的密码
//
// 用于处理 pgx 等驱动返回的错误消息中可能包含完整 DSN 的情况。
//   - 输入: postgres://user:secret@host:5432/db?sslmode=require
//   - 输出: postgres://user:***@host:5432/db?sslmode=require
//
// 解析失败或非 URL 格式时回退到 SanitizeSecret()，避免泄露原始字符串。
// 不使用 u.String() 避免 Go url 包对密码中的特殊字符做 percent-encoding
// 导致输出不一致。
func SanitizeDBURL(dsn string) string {
	if dsn == "" {
		return ""
	}

	u, err := url.Parse(dsn)
	if err != nil || u.User == nil {
		// 解析失败或无用户信息，保守地整体隐藏
		// 避免错误消息中包含密码时被泄露
		return SanitizeSecret()
	}

	// 无密码字段，原样返回（已经过 url.Parse 验证为合法 URL）
	if _, hasPwd := u.User.Password(); !hasPwd {
		return dsn
	}

	// 有密码字段：直接对原 DSN 字符串做替换
	// 用 "user:password@" 定位，把 password 部分替换为 ***
	// 这种方式避免 u.String() 的 percent-encoding 导致 "***" 变成 "%2A%2A%2A"
	scheme := u.Scheme
	if scheme == "" {
		return SanitizeSecret()
	}
	schemePrefix := scheme + "://"
	idx := strings.Index(dsn, schemePrefix)
	if idx < 0 {
		return SanitizeSecret()
	}

	// 取 scheme:// 之后的字符串
	afterScheme := dsn[idx+len(schemePrefix):]
	// 查找 user:password@ 的 @ 分隔符
	atIdx := strings.Index(afterScheme, "@")
	if atIdx < 0 {
		return SanitizeSecret()
	}

	// 在 afterScheme 中查找第一个 ':' （分隔 user 和 password）
	userInfo := afterScheme[:atIdx]
	colonIdx := strings.Index(userInfo, ":")
	if colonIdx < 0 {
		// 无密码部分（user@host 格式），原样返回
		return dsn
	}

	// 重建: scheme:// + user + ":***@" + rest
	username := userInfo[:colonIdx]
	rest := afterScheme[atIdx+1:]
	return schemePrefix + username + ":***@" + rest
}

// ============================================================================
// 通用字段名识别脱敏
// ============================================================================

// sensitiveKeyPatterns 敏感字段名关键词（小写匹配）
//
// 命中任一关键词的字段值将被脱敏：
//   - password/pwd/pass: 密码
//   - secret: 密钥
//   - token: 各种 token（access_token/refresh_token/csrf_token 等）
//   - private_key/privatekey: 私钥
//   - hmac_key: HMAC 密钥
//   - api_key/apikey: API 密钥
//   - client_secret: OAuth 客户端密钥
//
// 注意：token_id 不算敏感（不是真正的 token 值），id 类字段单独排除
var sensitiveKeyPatterns = []string{
	"password",
	"passwd",
	"pwd",
	"secret",
	"private_key",
	"privatekey",
	"priv_key",
	"privatekey",
	"hmac_key",
	"hmackey",
	"api_key",
	"apikey",
	"client_secret",
	"clientsecret",
	"refresh_token",
	"accesstoken",
	"access_token",
	"auth_token",
	"authtoken",
	"database_url",
	"db_url",
	"dsn",
	"connection_string",
}

// safeKeyPatterns 安全字段名（即使包含敏感关键词也不脱敏）
//
// 例如：
//   - "token_id" 包含 "token" 但属于标识符，不是 token 值
//   - "token_type" 是 "Bearer" 等类型，不是 token 值
//   - "access_token_ttl" 是数值配置，不是 token 值
//   - "secret_length" 是数值
//   - "password_policy" 是策略描述
var safeKeyPatterns = []string{
	"token_id",
	"token_type",
	"token_ttl",
	"access_token_ttl",
	"refresh_token_ttl",
	"refresh_token_length",
	"access_token_length",
	"token_length",
	"token_prefix",
	"token_preview",
	"secret_length",
	"password_length",
	"password_policy",
	"password_hash", // 已是 bcrypt 哈希，不是明文
	"password_cost",  // bcrypt cost 数值
}

// isSensitiveKey 判断字段名是否需要脱敏
func isSensitiveKey(key string) bool {
	if key == "" {
		return false
	}
	lowerKey := strings.ToLower(key)

	// 先检查白名单（安全字段）
	for _, safe := range safeKeyPatterns {
		if strings.Contains(lowerKey, safe) {
			return false
		}
	}

	// 再检查敏感关键词
	for _, pattern := range sensitiveKeyPatterns {
		if strings.Contains(lowerKey, pattern) {
			return true
		}
	}

	// 单独处理 "token"（不包含其他敏感词的组合）
	// 例如 plain "token" 字段需要脱敏，但 token_xxx 上面已判断
	if lowerKey == "token" {
		return true
	}

	return false
}

// SanitizeValue 根据字段名自动判断并脱敏
//
// 用于 LogSecurity 等需要遍历 map 的场景，按 key 名识别敏感字段并自动脱敏。
// 非敏感字段保持原值。
//
// 用法：
//
//	for k, v := range details {
//	    attrs = append(attrs, k, logging.SanitizeValue(k, v))
//	}
func SanitizeValue(key string, value any) any {
	if !isSensitiveKey(key) {
		return value
	}

	// 敏感字段根据类型脱敏
	switch v := value.(type) {
	case string:
		// 对 database_url / dsn / connection_string 用 SanitizeDBURL
		lowerKey := strings.ToLower(key)
		if strings.Contains(lowerKey, "database_url") ||
			strings.Contains(lowerKey, "db_url") ||
			strings.Contains(lowerKey, "dsn") ||
			strings.Contains(lowerKey, "connection_string") {
			return SanitizeDBURL(v)
		}
		// token 类用 SanitizeToken 保留前缀便于排查
		if strings.Contains(lowerKey, "token") {
			return SanitizeToken(v)
		}
		// 其他敏感字符串完全隐藏
		return SanitizeSecret()
	case *string:
		if v == nil {
			return nil
		}
		return SanitizeValue(key, *v)
	default:
		// 非字符串类型（int/bool 等）一般不脱敏
		return value
	}
}
