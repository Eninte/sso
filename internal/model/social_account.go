// Package model 社交账号身份模型
// 阶段 2.3：将社交账号身份从 users.email 解耦，避免账号接管攻击
package model

import (
	"encoding/json"
	"time"
)

// ============================================================================
// SocialAccount 社交账号身份模型
// ============================================================================

// SocialAccount 表示一个用户在第三方登录提供商（Google/GitHub 等）的身份
//
// 安全设计：
//   - (provider, provider_user_id) 全局唯一，确保同一社交账号只能绑定一个本系统用户
//   - (user_id, provider) 唯一，确保一个用户在同一 provider 下只能绑定一个账号
//   - user_id 外键关联 users.id，删除用户时级联删除
//
// 流程：
//   1. 用户首次社交登录时，系统创建 user（如不存在）+ 创建 social_account
//   2. 后续社交登录通过 (provider, provider_user_id) 查找 social_account
//   3. 通过 social_account.user_id 关联到本系统用户
type SocialAccount struct {
	ID                string            `json:"id" db:"id"`
	Provider          string            `json:"provider" db:"provider"`
	ProviderUserID    string            `json:"provider_user_id" db:"provider_user_id"`
	UserID            string            `json:"user_id" db:"user_id"`
	ProviderEmail     string            `json:"provider_email,omitempty" db:"provider_email"`
	EmailVerified     bool              `json:"email_verified" db:"email_verified"`
	ProviderMetadata  map[string]string `json:"provider_metadata,omitempty" db:"provider_metadata"`
	CreatedAt         time.Time         `json:"created_at" db:"created_at"`
	UpdatedAt         time.Time         `json:"updated_at" db:"updated_at"`
}

// 支持的社交登录提供商常量
const (
	ProviderGoogle = "google"
	ProviderGitHub = "github"
)

// IsSupportedProvider 检查 provider 是否在支持列表内
func IsSupportedProvider(provider string) bool {
	switch provider {
	case ProviderGoogle, ProviderGitHub:
		return true
	default:
		return false
	}
}

// ProviderMetadataFromJSON 从 JSON 字节流解析 provider_metadata 字段
// 用于 postgres 实现中从 JSONB 列读取
func ProviderMetadataFromJSON(raw []byte) map[string]string {
	if len(raw) == 0 {
		return nil
	}
	var m map[string]string
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil
	}
	return m
}

// ProviderMetadataToJSON 将 provider_metadata 序列化为 JSON 字节流
func ProviderMetadataToJSON(m map[string]string) []byte {
	if m == nil {
		return nil
	}
	b, err := json.Marshal(m)
	if err != nil {
		return nil
	}
	return b
}
