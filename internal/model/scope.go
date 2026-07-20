// Package model OAuth/OIDC Scope 常量与白名单
// 阶段 2.2：统一 scope 定义，防止散落的硬编码字符串导致的拼写错误
package model

// ============================================================================
// OAuth/OIDC Scope 常量
// ============================================================================
//
// OIDC 标准 scope（参考 OpenID Connect Core 1.0 §5.4）：
//   - openid:   必须包含，表示 OIDC 请求，对应 sub claim
//   - profile:  返回用户基础信息（name, preferred_username 等）
//   - email:    返回 email 与 email_verified claim
//   - address:  返回 address claim（本项目暂不实现）
//   - phone:    返回 phone_number 与 phone_number_verified claim（本项目暂不实现）
//
// 本项目支持的白名单：
const (
	ScopeOpenID        = "openid"
	ScopeProfile       = "profile"
	ScopeEmail         = "email"
	ScopeOfflineAccess = "offline_access" // 允许 refresh_token（RFC 6749 §3.3）
)

// SupportedScopes 全局支持的 scope 白名单
// 客户端注册时与授权请求中请求的 scope 必须在此白名单内
var SupportedScopes = []string{
	ScopeOpenID,
	ScopeProfile,
	ScopeEmail,
	ScopeOfflineAccess,
}

// IsSupportedScope 判断 scope 是否在全局白名单内
func IsSupportedScope(scope string) bool {
	for _, s := range SupportedScopes {
		if s == scope {
			return true
		}
	}
	return false
}

// NormalizeScopes 对 scope 切片去重与规范化
// - 移除空字符串
// - 移除重复项（保留首次出现顺序）
// - 不校验是否在白名单内（由调用方通过 IsSupportedScope 检查）
func NormalizeScopes(scopes []string) []string {
	if len(scopes) == 0 {
		return []string{}
	}
	seen := make(map[string]struct{}, len(scopes))
	result := make([]string, 0, len(scopes))
	for _, s := range scopes {
		if s == "" {
			continue
		}
		if _, exists := seen[s]; exists {
			continue
		}
		seen[s] = struct{}{}
		result = append(result, s)
	}
	return result
}

// IsScopesSubset 判断 requested 是否是 allowed 的子集
// 空切片视为空集，任何集合都是空集的子集
func IsScopesSubset(requested, allowed []string) bool {
	allowedSet := make(map[string]struct{}, len(allowed))
	for _, s := range allowed {
		allowedSet[s] = struct{}{}
	}
	for _, s := range requested {
		if _, exists := allowedSet[s]; !exists {
			return false
		}
	}
	return true
}
