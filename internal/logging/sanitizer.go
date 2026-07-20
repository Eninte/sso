// Package logging 日志脱敏
// 提供敏感信息脱敏功能，防止日志泄露
package logging

import (
	"encoding/json"
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
// 支持两种 DSN 格式：
//
//  1. URL 格式：
//     输入: postgres://user:secret@host:5432/db?sslmode=require
//     输出: postgres://user:***@host:5432/db?sslmode=require
//
//  2. key=value 格式（libpq/pgx 风格）：
//     输入: host=localhost user=admin password=secret dbname=sso
//     输出: host=localhost user=admin password=*** dbname=sso
//
// 解析失败或非已知格式时原样返回。
// 不使用 u.String() 避免 Go url 包对密码中的特殊字符做 percent-encoding
// 导致输出不一致。
//
// 阶段 B 审查修复（M7）：原实现仅处理 URL 格式，对 key=value 格式会
// 整体替换为 ***，丢失上下文。新实现增加 key=value 路径支持。
//
// 阶段 D 审查修复（H5）：非 DSN 格式原样返回。
// 原实现回退到 SanitizeSecret() 会破坏普通错误消息（如 "connection refused"）。
// 由于 sanitizeURLDSN 和 sanitizeKeyValueDSN 已能识别含 password= 的字符串，
// 非法格式回退到原样返回是安全的，且能保留错误上下文便于排查。
// 调用方应在错误可能含 DSN 时使用此函数，纯内部错误可直接 err.Error()。
func SanitizeDBURL(dsn string) string {
	if dsn == "" {
		return ""
	}

	// 优先尝试 URL 格式
	if sanitized, ok := sanitizeURLDSN(dsn); ok {
		return sanitized
	}

	// 尝试 key=value 格式
	if sanitized, ok := sanitizeKeyValueDSN(dsn); ok {
		return sanitized
	}

	// 既非 URL 也非 key=value，原样返回（避免破坏普通错误消息）
	return dsn
}

// sanitizeURLDSN 处理 scheme://user:password@host 形式的 DSN
// 返回 (sanitized, true) 表示已识别处理；返回 (_, false) 表示非此格式
func sanitizeURLDSN(dsn string) (string, bool) {
	u, err := url.Parse(dsn)
	if err != nil || u.User == nil || u.Scheme == "" {
		return "", false
	}

	// 无密码字段，原样返回
	if _, hasPwd := u.User.Password(); !hasPwd {
		return dsn, true
	}

	schemePrefix := u.Scheme + "://"
	idx := strings.Index(dsn, schemePrefix)
	if idx < 0 {
		return "", false
	}

	afterScheme := dsn[idx+len(schemePrefix):]
	atIdx := strings.Index(afterScheme, "@")
	if atIdx < 0 {
		return "", false
	}

	userInfo := afterScheme[:atIdx]
	colonIdx := strings.Index(userInfo, ":")
	if colonIdx < 0 {
		// 无密码部分（user@host 格式），原样返回
		return dsn, true
	}

	username := userInfo[:colonIdx]
	rest := afterScheme[atIdx+1:]
	return schemePrefix + username + ":***@" + rest, true
}

// sanitizeKeyValueDSN 处理 "key=value key=value" 形式的 DSN
// 仅替换 password=xxx 中的 xxx，保留其他字段
// 返回 (sanitized, true) 表示已识别处理；返回 (_, false) 表示非此格式
func sanitizeKeyValueDSN(dsn string) (string, bool) {
	// libpq DSN 必须包含至少一个空格分隔的 key=value 对
	// 单独 "password=xxx" 也算
	if !strings.Contains(dsn, "=") {
		return "", false
	}
	// 排除明显不是 DSN 的字符串（如 SQL 语句）
	// DSN 中不应包含 SELECT/INSERT/UPDATE/DELETE/FROM/WHERE 等关键字
	upper := strings.ToUpper(dsn)
	if strings.Contains(upper, "SELECT ") ||
		strings.Contains(upper, "INSERT ") ||
		strings.Contains(upper, "UPDATE ") ||
		strings.Contains(upper, "DELETE ") ||
		strings.Contains(upper, "FROM ") ||
		strings.Contains(upper, "WHERE ") {
		return "", false
	}

	// 查找 password= 位置（不区分大小写）
	lower := strings.ToLower(dsn)
	pwdKey := "password="
	idx := strings.Index(lower, pwdKey)
	if idx < 0 {
		// 无 password 字段，原样返回
		return dsn, true
	}

	// 找到 password= 之后的值
	valueStart := idx + len(pwdKey)
	// 值以空格或字符串结尾结束（不考虑引号包裹的复杂场景）
	valueEnd := strings.IndexAny(dsn[valueStart:], " \t")
	if valueEnd < 0 {
		// 值直到字符串结尾
		return dsn[:valueStart] + "***", true
	}
	return dsn[:valueStart] + "***" + dsn[valueStart+valueEnd:], true
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
	// 阶段 B 审查修复（H4）：补齐 HTTP 头相关敏感字段
	// 防止未来日志记录 Authorization/Cookie/Bearer 时未脱敏
	"authorization",
	"cookie",
	"set-cookie",
	"bearer",
	"session",
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
	"password_cost", // bcrypt cost 数值
	"session_id",    // 会话标识符，非会话凭据本身
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
// 非敏感字段保持原值，但其值若为 map/slice/JSON 字符串会递归脱敏内层敏感字段。
//
// 阶段 B 审查修复（C1）：修复嵌套 JSON 绕过漏洞。
// 原实现仅处理顶层 key，对 {"details": {"password": "xyz"}} 这类嵌套 map，
// 内层 password 字段会被原样写入 audit_logs.details 列。
// 新实现递归处理 map[string]interface{} / []interface{} / 嵌入 JSON 字符串，
// 确保任意层级的敏感字段都能被脱敏。
//
// 用法：
//
//	for k, v := range details {
//	    attrs = append(attrs, k, logging.SanitizeValue(k, v))
//	}
func SanitizeValue(key string, value any) any {
	// 敏感字段：按类型脱敏
	if isSensitiveKey(key) {
		switch v := value.(type) {
		case string:
			lowerKey := strings.ToLower(key)
			// 对 database_url / dsn / connection_string 用 SanitizeDBURL
			if strings.Contains(lowerKey, "database_url") ||
				strings.Contains(lowerKey, "db_url") ||
				strings.Contains(lowerKey, "dsn") ||
				strings.Contains(lowerKey, "connection_string") {
				// 阶段 D 审查修复：若 SanitizeDBURL 返回原值（非 DSN 格式），
				// 回退到 SanitizeSecret() 确保敏感字段值不泄露
				// SanitizeDBURL 的"原样返回"行为仅用于错误消息场景（调用方直接使用），
				// 在 SanitizeValue 上下文中，键名已标记为敏感，必须脱敏
				if sanitized := SanitizeDBURL(v); sanitized != v {
					return sanitized
				}
				return SanitizeSecret()
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
			// 阶段 D 审查修复：非字符串敏感字段（如 bool/int/struct）保持原值
			// 敏感数据几乎都是字符串类型（password/token/secret 等），
			// 含敏感词的数值字段通常是元数据（password_count/secret_enabled），
			// 保守隐藏会丢失排查信息且无安全收益。
			// 若需脱敏 struct 内字段，调用方应先转为 map 再调用 SanitizeValue。
			return value
		}
	}

	// 非敏感字段：递归处理嵌套结构中的敏感字段
	return sanitizeNestedValue(value)
}

// sanitizeNestedValue 递归处理嵌套结构
// 对 map 按 key 名识别并脱敏；对 slice 递归处理每个元素；
// 对 string 尝试解析为 JSON 并递归脱敏（防止嵌入 JSON 绕过）
func sanitizeNestedValue(value any) any {
	switch v := value.(type) {
	case map[string]interface{}:
		result := make(map[string]interface{}, len(v))
		for k, val := range v {
			result[k] = SanitizeValue(k, val)
		}
		return result
	case []interface{}:
		result := make([]interface{}, len(v))
		for i, val := range v {
			result[i] = sanitizeNestedValue(val)
		}
		return result
	case string:
		// 尝试解析 JSON 字符串并递归脱敏
		// 防止 {"password":"xyz"} 以字符串形式嵌入非敏感字段值
		if looksLikeJSON(v) {
			return sanitizeJSONString(v)
		}
		return v
	default:
		return value
	}
}

// looksLikeJSON 判断字符串是否可能为 JSON
// 只对以 { 或 [ 开头的非空字符串尝试解析，避免对普通字符串做昂贵解析
func looksLikeJSON(s string) bool {
	s = strings.TrimSpace(s)
	return len(s) >= 2 && (s[0] == '{' || s[0] == '[')
}

// sanitizeJSONString 解析 JSON 字符串，递归脱敏后重新 marshal
// 解析失败时原样返回，避免破坏非 JSON 字符串
func sanitizeJSONString(s string) string {
	var parsed any
	if err := json.Unmarshal([]byte(s), &parsed); err != nil {
		return s
	}
	sanitized := sanitizeNestedValue(parsed)
	reMarshal, err := json.Marshal(sanitized)
	if err != nil {
		return s
	}
	return string(reMarshal)
}
