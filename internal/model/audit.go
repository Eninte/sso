// Package model 审计日志模型
// 定义审计日志相关的数据结构
package model

import (
	"time"
)

// ============================================================================
// 审计日志模型
// ============================================================================

// AuditLog 审计日志
type AuditLog struct {
	ID        string    `json:"id" db:"id"`                 // 日志ID
	EventType string    `json:"event_type" db:"event_type"` // 事件类型
	UserID    string    `json:"user_id" db:"user_id"`       // 用户ID
	ClientID  string    `json:"client_id" db:"client_id"`   // 客户端ID
	IPAddress string    `json:"ip_address" db:"ip_address"` // IP地址
	UserAgent string    `json:"user_agent" db:"user_agent"` // 用户代理
	Details   string    `json:"details" db:"details"`       // 事件详情 (JSON)
	Timestamp time.Time `json:"timestamp" db:"timestamp"`   // 时间戳
	Success   bool      `json:"success" db:"success"`       // 是否成功
}

// AuditEventType 审计事件类型
type AuditEventType string

const (
	EventUserRegister    AuditEventType = "user.register"     // 用户注册
	EventUserLogin       AuditEventType = "user.login"        // 用户登录
	EventUserLoginFailed AuditEventType = "user.login_failed" // 登录失败
	EventUserLogout      AuditEventType = "user.logout"       // 用户登出
	EventUserLocked      AuditEventType = "user.locked"       // 账户锁定
	EventUserUnlocked    AuditEventType = "user.unlocked"     // 账户解锁
	EventLogoutAll       AuditEventType = "user.logout_all"   // 登出所有设备

	EventTokenIssued  AuditEventType = "token.issued"  // Token签发
	EventTokenRefresh AuditEventType = "token.refresh" // Token刷新
	EventTokenRevoke  AuditEventType = "token.revoke"  // Token撤销

	EventAuthCodeCreated AuditEventType = "oauth.code_created" // 授权码创建
	EventAuthCodeUsed    AuditEventType = "oauth.code_used"    // 授权码使用
	EventAuthCodeInvalid AuditEventType = "oauth.code_invalid" // 授权码无效

	EventRateLimitExceeded  AuditEventType = "security.rate_limit"       // 限流触发
	EventSuspiciousActivity AuditEventType = "security.suspicious"       // 可疑活动
	EventPasswordChanged    AuditEventType = "security.password_changed" // 密码修改
	EventPasswordReset      AuditEventType = "security.password_reset"   // 密码重置
	EventAccountLocked      AuditEventType = "security.account_locked"   // 账户锁定
	EventAccountUnlocked    AuditEventType = "security.account_unlocked" // 账户解锁

	EventMFASetup    AuditEventType = "mfa.setup"    // MFA设置
	EventMFAEnabled  AuditEventType = "mfa.enabled"  // MFA启用
	EventMFADisabled AuditEventType = "mfa.disabled" // MFA禁用

	// 阶段 2.3：社交登录安全审计事件
	EventSocialLoginRejected AuditEventType = "social.login_rejected" // 社交登录被拒绝（email 未验证 / 冲突等）
	EventSocialAccountBound  AuditEventType = "social.account_bound"  // 社交账号绑定到本地用户

	EventKeyRotated AuditEventType = "key.rotated" // 密钥轮换
	EventKeyRevoked AuditEventType = "key.revoked" // 密钥撤销

	EventUserDisabled  AuditEventType = "admin.user_disabled"  // 管理员禁用用户
	EventUserEnabled   AuditEventType = "admin.user_enabled"   // 管理员启用用户
	EventUserDeleted   AuditEventType = "admin.user_deleted"   // 管理员删除用户
	EventSystemCleanup AuditEventType = "admin.system_cleanup" // 管理员清理过期数据
	// 阶段 4 安全增强：管理员查看审计日志这一动作本身也需要被审计，防止审计日志被静默窃取
	EventAuditLogsViewed AuditEventType = "admin.audit_logs_viewed" // 管理员查看审计日志

	EventSystemStart AuditEventType = "system.start" // 系统启动
	EventSystemStop  AuditEventType = "system.stop"  // 系统停止
)
