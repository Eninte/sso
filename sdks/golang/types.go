// Package sdk SSO服务Go客户端SDK
// 提供类型安全的API客户端，支持Token自动管理
package sdk

import "time"

// ============================================================================
// 请求类型
// ============================================================================

// RegisterRequest 注册请求
type RegisterRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

// LoginRequest 登录请求
type LoginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

// TokenRequest Token请求（刷新或授权码交换）
type TokenRequest struct {
	GrantType    string `json:"grant_type"`
	RefreshToken string `json:"refresh_token,omitempty"`
	Code         string `json:"code,omitempty"`
	RedirectURI  string `json:"redirect_uri,omitempty"`
	ClientID     string `json:"client_id,omitempty"`
	ClientSecret string `json:"client_secret,omitempty"`
	CodeVerifier string `json:"code_verifier,omitempty"`
}

// RevokeRequest 撤销Token请求
type RevokeRequest struct {
	Token string `json:"token"`
}

// ForgotPasswordRequest 忘记密码请求
type ForgotPasswordRequest struct {
	Email string `json:"email"`
}

// ResetPasswordRequest 重置密码请求
type ResetPasswordRequest struct {
	Token       string `json:"token"`
	UserID      string `json:"user_id"`
	NewPassword string `json:"new_password"`
}

// ChangePasswordRequest 修改密码请求
type ChangePasswordRequest struct {
	OldPassword string `json:"old_password"`
	NewPassword string `json:"new_password"`
}

// AuthorizeApproveRequest 授权批准请求
type AuthorizeApproveRequest struct {
	ClientID            string `json:"client_id"`
	RedirectURI         string `json:"redirect_uri"`
	Scope               string `json:"scope"`
	State               string `json:"state"`
	CodeChallenge       string `json:"code_challenge,omitempty"`
	CodeChallengeMethod string `json:"code_challenge_method,omitempty"`
}

// MFAVerifyRequest MFA验证请求
type MFAVerifyRequest struct {
	Code string `json:"code"`
}

// DisableUserRequest 禁用用户请求
type DisableUserRequest struct {
	UserID string `json:"user_id"`
}

// EnableUserRequest 启用用户请求
type EnableUserRequest struct {
	UserID string `json:"user_id"`
}

// ============================================================================
// 响应类型
// ============================================================================

// TokenResponse Token响应
type TokenResponse struct {
	AccessToken  string   `json:"access_token"`
	RefreshToken string   `json:"refresh_token"`
	TokenType    string   `json:"token_type"`
	ExpiresIn    int      `json:"expires_in"`
	Scopes       []string `json:"scopes,omitempty"`
	Scope        string   `json:"scope,omitempty"`
}

// RegisterResponse 注册响应
type RegisterResponse struct {
	Message string        `json:"message"`
	Data    *RegisterData `json:"data,omitempty"`
}

// RegisterData 注册数据
type RegisterData struct {
	UserID string `json:"user_id"`
	Email  string `json:"email"`
}

// UserInfo 用户信息
type UserInfo struct {
	Sub           string   `json:"sub"`
	Email         string   `json:"email"`
	EmailVerified bool     `json:"email_verified"`
	Scope         []string `json:"scope,omitempty"`
}

// MessageResponse 通用消息响应
type MessageResponse struct {
	Message string `json:"message"`
}

// MFASetupResponse MFA设置响应
type MFASetupResponse struct {
	Secret      string `json:"secret"`
	QRCodeURL   string `json:"qr_code_url"`
	ManualEntry string `json:"manual_entry"`
}

// MFAStatusResponse MFA状态响应
type MFAStatusResponse struct {
	Enabled bool `json:"enabled"`
}

// AuthorizeResponse 授权响应
type AuthorizeResponse struct {
	Code  string `json:"code"`
	State string `json:"state"`
}

// UserListResponse 用户列表响应
type UserListResponse struct {
	Users      []UserItem `json:"users"`
	Total      int        `json:"total"`
	Page       int        `json:"page"`
	PageSize   int        `json:"page_size"`
	TotalPages int        `json:"total_pages"`
}

// UserItem 用户列表项
type UserItem struct {
	ID            string    `json:"id"`
	Email         string    `json:"email"`
	EmailVerified bool      `json:"email_verified"`
	MFAEnabled    bool      `json:"mfa_enabled"`
	Status        string    `json:"status"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

// HealthResponse 健康检查响应
type HealthResponse struct {
	Status    string `json:"status"`
	Timestamp string `json:"timestamp"`
	Database  string `json:"database"`
	Version   string `json:"version"`
	BuildTime string `json:"build_time"`
}

// DiscoveryResponse OIDC Discovery响应
type DiscoveryResponse struct {
	Issuer                           string   `json:"issuer"`
	AuthorizationEndpoint            string   `json:"authorization_endpoint"`
	TokenEndpoint                    string   `json:"token_endpoint"`
	UserinfoEndpoint                 string   `json:"userinfo_endpoint"`
	JWKSUri                          string   `json:"jwks_uri"`
	RevocationEndpoint               string   `json:"revocation_endpoint"`
	ResponseTypesSupported           []string `json:"response_types_supported"`
	GrantTypesSupported              []string `json:"grant_types_supported"`
	SubjectTypesSupported            []string `json:"subject_types_supported"`
	IDTokenSigningAlgValuesSupported []string `json:"id_token_signing_alg_values_supported"`
	ScopesSupported                  []string `json:"scopes_supported"`
	CodeChallengeMethodsSupported    []string `json:"code_challenge_methods_supported"`
}

// JWKSResponse JWKS响应
type JWKSResponse struct {
	Keys []JWK `json:"keys"`
}

// JWK JSON Web Key
type JWK struct {
	Kty string `json:"kty"`
	Use string `json:"use"`
	Kid string `json:"kid"`
	N   string `json:"n"`
	E   string `json:"e"`
}

// OAuthProvider OAuth提供商
type OAuthProvider struct {
	Name  string `json:"name"`
	Label string `json:"label"`
	Icon  string `json:"icon"`
}
