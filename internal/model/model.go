// Package model 数据模型定义
// 包含所有业务实体的数据结构
package model

import (
	"time"
)

// ============================================================================
// 用户相关模型
// ============================================================================

// User 用户模型
type User struct {
	ID            string     `json:"id" db:"id"`
	Email         string     `json:"email" db:"email"`
	PasswordHash  string     `json:"-" db:"password_hash"`
	MFASecret     string     `json:"-" db:"mfa_secret"`
	Role          string     `json:"role" db:"role"`
	Status        string     `json:"status" db:"status"`
	LockedUntil   *time.Time `json:"locked_until,omitempty" db:"locked_until"`
	CreatedAt     time.Time  `json:"created_at" db:"created_at"`
	UpdatedAt     time.Time  `json:"updated_at" db:"updated_at"`
	EmailVerified bool       `json:"email_verified" db:"email_verified"`
	MFAEnabled    bool       `json:"mfa_enabled" db:"mfa_enabled"`
	LoginAttempts int        `json:"-" db:"login_attempts"`
}

const (
	UserStatusActive   = "active"
	UserStatusLocked   = "locked"
	UserStatusDisabled = "disabled"
	UserStatusPending  = "pending" // 注册后待邮箱验证
)

// 用户角色常量
const (
	UserRoleUser  = "user"
	UserRoleAdmin = "admin"
)

type RegisterRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type LoginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type LoginResponse struct {
	AccessToken  string   `json:"access_token,omitempty"`
	RefreshToken string   `json:"refresh_token,omitempty"`
	TokenType    string   `json:"token_type,omitempty"`
	Scopes       []string `json:"scopes,omitempty"`
	ExpiresIn    int      `json:"expires_in"`

	// MFA 两阶段登录字段
	// 第一阶段密码验证成功后，若用户启用了 MFA，则不返回 access_token/refresh_token，
	// 而是返回 mfa_required=true 和一次性 mfa_challenge 令牌
	// 客户端在第二阶段 POST /api/v1/login/mfa/verify 提交 challenge + code 换取 Token
	MFARequired  bool     `json:"mfa_required,omitempty"`
	MFAChallenge string   `json:"mfa_challenge,omitempty"`           // 一次性高熵随机令牌，仅生成时返回
	MFAMethods   []string `json:"mfa_methods,omitempty"`              // 可用的 MFA 验证方法，如 ["totp","recovery_code"]
}

// ============================================================================
// OAuth2客户端相关模型
// ============================================================================

type Client struct {
	ID           string    `json:"id" db:"id"`
	ClientID     string    `json:"client_id" db:"client_id"`
	ClientSecret string    `json:"-" db:"client_secret"`
	Name         string    `json:"name" db:"name"`
	RedirectURIs []string  `json:"redirect_uris" db:"redirect_uris"`
	GrantTypes   []string  `json:"grant_types" db:"grant_types"`
	Scopes       []string  `json:"scopes" db:"scopes"`
	CreatedAt    time.Time `json:"created_at" db:"created_at"`
	PublicClient bool      `json:"public_client" db:"public_client"`
}

const (
	GrantTypeAuthorizationCode = "authorization_code"
	GrantTypeRefreshToken      = "refresh_token"
	GrantTypeClientCredentials = "client_credentials"
)

type AuthorizeRequest struct {
	ClientID            string `json:"client_id"`
	RedirectURI         string `json:"redirect_uri"`
	ResponseType        string `json:"response_type"`
	Scope               string `json:"scope"`
	State               string `json:"state"`
	CodeChallenge       string `json:"code_challenge"`
	CodeChallengeMethod string `json:"code_challenge_method"`
}

type TokenRequest struct {
	GrantType    string `json:"grant_type"`
	Code         string `json:"code"`
	RedirectURI  string `json:"redirect_uri"`
	ClientID     string `json:"client_id"`
	ClientSecret string `json:"client_secret"`
	RefreshToken string `json:"refresh_token"`
	CodeVerifier string `json:"code_verifier"`
}

// ============================================================================
// Token相关模型
// ============================================================================

type Token struct {
	ID           string     `json:"id" db:"id"`
	AccessToken  string     `json:"access_token" db:"access_token"`
	RefreshToken string     `json:"refresh_token" db:"refresh_token"`
	UserID       string     `json:"user_id" db:"user_id"`
	Scopes       []string   `json:"scopes" db:"scopes"`
	ClientID     *string    `json:"client_id,omitempty" db:"client_id"`
	RevokedAt    *time.Time `json:"revoked_at" db:"revoked_at"`
	ExpiresAt    time.Time  `json:"expires_at" db:"expires_at"`
	CreatedAt    time.Time  `json:"created_at" db:"created_at"`

	// Refresh Token 轮换字段（阶段 2.1 安全增强）
	// RotatedAt：refresh token 被轮换使用的时间，NULL 表示未使用
	// 用于检测重放：若已被轮换的 token 再次出现，说明被盗用
	RotatedAt *time.Time `json:"rotated_at,omitempty" db:"rotated_at"`
	// ReplacedByTokenID：轮换后新 token 的 ID，用于 token 家族追踪
	ReplacedByTokenID *string `json:"replaced_by_token_id,omitempty" db:"replaced_by_token_id"`
	// RefreshExpiresAt：refresh token 独立过期时间
	// NULL 表示沿用 ExpiresAt（兼容旧数据）
	RefreshExpiresAt *time.Time `json:"refresh_expires_at,omitempty" db:"refresh_expires_at"`
}

func (t *Token) GetClientID() string {
	if t.ClientID != nil {
		return *t.ClientID
	}
	return ""
}

type AuthorizationCode struct {
	Code                string     `json:"code" db:"code"`
	ClientID            string     `json:"client_id" db:"client_id"`
	UserID              string     `json:"user_id" db:"user_id"`
	RedirectURI         string     `json:"redirect_uri" db:"redirect_uri"`
	Scopes              []string   `json:"scopes" db:"scopes"`
	CodeChallenge       string     `json:"code_challenge" db:"code_challenge"`
	CodeChallengeMethod string     `json:"code_challenge_method" db:"code_challenge_method"`
	UsedAt              *time.Time `json:"used_at" db:"used_at"`
	ExpiresAt           time.Time  `json:"expires_at" db:"expires_at"`
	CreatedAt           time.Time  `json:"created_at" db:"created_at"`
}

// ============================================================================
// 通用响应模型
// ============================================================================

type ErrorResponse struct {
	Error string `json:"error"`
}

type SuccessResponse struct {
	Message string      `json:"message"`
	Data    interface{} `json:"data,omitempty"`
}
