package model

import "time"

// MFASetupResponse MFA设置响应
type MFASetupResponse struct {
	Secret      string `json:"secret"`       // MFA密钥
	QRCodeURL   string `json:"qr_code_url"`  // QR码URL
	ManualEntry string `json:"manual_entry"` // 手动输入密钥
}

// MFAStatusResponse MFA状态响应
type MFAStatusResponse struct {
	Enabled bool `json:"enabled"` // MFA是否启用
}

// ============================================================================
// 两阶段登录 MFA Challenge
// ============================================================================

// MFAChallenge MFA 登录挑战
// 第一阶段密码验证成功后生成，第二阶段 MFA 验证通过后立即删除
//
// 安全设计：
//   - Token：32 字节 CSPRNG 随机数，仅生成时返回给客户端，服务端不持久化明文
//   - TTL：默认 5 分钟（由 MFA_CHALLENGE_TTL 配置）
//   - 绑定：UserID + IPAddress + UserAgent，防止跨设备/跨网络使用
//   - 一次性：验证成功或失败次数超限后立即删除
//   - 尝试次数：默认最多 5 次，防止暴力枚举 TOTP 或恢复码
type MFAChallenge struct {
	UserID    string    `json:"user_id"`              // 关联用户 ID
	Email     string    `json:"email"`                // 冗余字段，签发 Token 时避免再查一次
	Role      string    `json:"role"`                 // 冗余字段
	Scopes    []string  `json:"scopes,omitempty"`     // 申请的 scopes（透传到 Token 签发）
	ClientID  *string   `json:"client_id,omitempty"`  // OAuth client ID（透传，可空）
	IPAddress string    `json:"ip_address"`           // 绑定客户端 IP
	UserAgent string    `json:"user_agent"`           // 绑定客户端 UA
	Attempts  int       `json:"attempts"`             // 已尝试次数
	CreatedAt time.Time `json:"created_at"`            // 创建时间
	ExpiresAt time.Time `json:"expires_at"`            // 过期时间
}

// MFAVerifyRequest MFA 两阶段登录第二阶段请求
// 客户端提交 challenge token + 验证方法 + 验证码
type MFAVerifyRequest struct {
	MFAChallenge string `json:"mfa_challenge"` // 第一阶段返回的 mfa_challenge 令牌
	Method       string `json:"method"`        // 验证方法："totp" 或 "recovery_code"
	Code         string `json:"code"`          // TOTP 6 位数字 或 recovery code
}

// MFA 验证方法常量
const (
	MFAMethodTOTP          = "totp"
	MFAMethodRecoveryCode  = "recovery_code"
)

// MaxMFALoginAttempts MFA 验证最大尝试次数
// 超过此次数后 Challenge 失效，需重新走第一阶段登录
const MaxMFALoginAttempts = 5
