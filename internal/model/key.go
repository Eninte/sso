package model

import (
	"time"
)

// KeyStatus 密钥状态类型
type KeyStatus string

const (
	KeyStatusActive     KeyStatus = "active"     // 活跃状态，可用于签名和验证
	KeyStatusDeprecated KeyStatus = "deprecated" // 已弃用，仅可用于验证
	KeyStatusRevoked    KeyStatus = "revoked"    // 已撤销，不可使用
)

// KeyVersion 密钥版本模型
// 用于密钥轮换机制，支持多个密钥版本共存
type KeyVersion struct {
	ID         string     `json:"id" db:"id"`                           // 密钥唯一标识
	PublicKey  []byte     `json:"public_key" db:"public_key"`           // RSA公钥
	PrivateKey []byte     `json:"-" db:"private_key"`                   // RSA私钥（JSON不序列化）
	Status     KeyStatus  `json:"status" db:"status"`                   // 密钥状态
	CreatedAt  time.Time  `json:"created_at" db:"created_at"`           // 创建时间
	ExpiresAt  *time.Time `json:"expires_at,omitempty" db:"expires_at"` // 过期时间（可选）
}

// IsActive 检查密钥是否为活跃状态
func (k *KeyVersion) IsActive() bool {
	return k.Status == KeyStatusActive
}

// IsDeprecated 检查密钥是否已弃用
func (k *KeyVersion) IsDeprecated() bool {
	return k.Status == KeyStatusDeprecated
}

// IsRevoked 检查密钥是否已撤销
func (k *KeyVersion) IsRevoked() bool {
	return k.Status == KeyStatusRevoked
}

// CanVerify 检查密钥是否可用于验证签名
// 已撤销的密钥不可用，已过期的密钥不可用
func (k *KeyVersion) CanVerify() bool {
	if k.Status == KeyStatusRevoked {
		return false
	}
	if k.ExpiresAt != nil && k.ExpiresAt.Before(time.Now()) {
		return false
	}
	return true
}
