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
	EmailVerified bool       `json:"email_verified" db:"email_verified"`
	MFAEnabled    bool       `json:"mfa_enabled" db:"mfa_enabled"`
	MFASecret     string     `json:"-" db:"mfa_secret"`
	Status        string     `json:"status" db:"status"`
	LoginAttempts int        `json:"-" db:"login_attempts"`
	LockedUntil   *time.Time `json:"locked_until,omitempty" db:"locked_until"`
	CreatedAt     time.Time  `json:"created_at" db:"created_at"`
	UpdatedAt     time.Time  `json:"updated_at" db:"updated_at"`
}

const (
	UserStatusActive   = "active"
	UserStatusLocked   = "locked"
	UserStatusDisabled = "disabled"
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
	AccessToken  string   `json:"access_token"`
	RefreshToken string   `json:"refresh_token"`
	TokenType    string   `json:"token_type"`
	ExpiresIn    int      `json:"expires_in"`
	Scopes       []string `json:"scopes,omitempty"`
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
	PublicClient bool      `json:"public_client" db:"public_client"`
	CreatedAt    time.Time `json:"created_at" db:"created_at"`
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
	ClientID     string     `json:"client_id" db:"client_id"`
	Scopes       []string   `json:"scopes" db:"scopes"`
	ExpiresAt    time.Time  `json:"expires_at" db:"expires_at"`
	CreatedAt    time.Time  `json:"created_at" db:"created_at"`
	RevokedAt    *time.Time `json:"revoked_at" db:"revoked_at"`
}

type AuthorizationCode struct {
	Code                string     `json:"code" db:"code"`
	ClientID            string     `json:"client_id" db:"client_id"`
	UserID              string     `json:"user_id" db:"user_id"`
	RedirectURI         string     `json:"redirect_uri" db:"redirect_uri"`
	Scopes              []string   `json:"scopes" db:"scopes"`
	CodeChallenge       string     `json:"code_challenge" db:"code_challenge"`
	CodeChallengeMethod string     `json:"code_challenge_method" db:"code_challenge_method"`
	ExpiresAt           time.Time  `json:"expires_at" db:"expires_at"`
	CreatedAt           time.Time  `json:"created_at" db:"created_at"`
	UsedAt              *time.Time `json:"used_at" db:"used_at"`
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
