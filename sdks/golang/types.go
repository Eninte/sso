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
//
// 阶段 5.3 契约修复：服务端 /api/v1/authorize/approve 实际期望 {consent_token, state}。
// 旧字段（client_id/redirect_uri/scope/code_challenge 等）已通过 consent_token JWT 携带，
// 不再需要重复传递。请求体启用 DisallowUnknownFields，多余字段会被拒绝。
type AuthorizeApproveRequest struct {
	ConsentToken string `json:"consent_token"`
	State        string `json:"state"`
}

// AuthorizeDenyRequest 授权拒绝请求
//
// 阶段 5.3 新增：用户主动拒绝授权时调用 /api/v1/authorize/deny。
type AuthorizeDenyRequest struct {
	ConsentToken string `json:"consent_token"`
	State        string `json:"state"`
}

// MFAVerifyRequest MFA验证请求
// 用于已登录用户在 /api/v1/mfa/verify 端点验证 TOTP 码以启用 MFA。
type MFAVerifyRequest struct {
	Code string `json:"code"`
}

// LoginMFAVerifyRequest 登录阶段 MFA 验证请求
//
// 阶段 5.4 新增：用于两阶段登录的第二阶段，调用 /api/v1/login/mfa/verify 端点。
// 字段说明：
//   - MFAChallenge：第一阶段登录返回的 mfa_challenge 令牌（64 字符 hex）
//   - Method：验证方式，"totp" 或 "recovery_code"
//   - Code：TOTP 6 位数字 或 恢复码字符串
type LoginMFAVerifyRequest struct {
	MFAChallenge string `json:"mfa_challenge"`
	Method       string `json:"method"`
	Code         string `json:"code"`
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
//
// 阶段 5.4 契约扩展：当用户启用 MFA 时，登录响应不再直接返回 Token，
// 而是返回 mfa_challenge + mfa_required + mfa_methods，调用方需展示 MFA 输入页面，
// 收到用户输入的 TOTP/恢复码后调用 VerifyMFALogin 完成第二阶段验证。
//
// 字段语义：
//   - 普通登录（无 MFA）：access_token/refresh_token/token_type/scopes 有值，
//     expires_in 是 access_token 的 TTL（秒）
//   - MFA required：mfa_required=true，mfa_challenge 是 64 字符 hex 令牌，
//     mfa_methods 是支持的验证方式（如 ["totp","recovery_code"]），
//     expires_in 是 challenge 的 TTL（默认 300 秒），access_token 等字段为空
type TokenResponse struct {
	// 普通登录字段
	AccessToken  string   `json:"access_token,omitempty"`
	RefreshToken string   `json:"refresh_token,omitempty"`
	TokenType    string   `json:"token_type,omitempty"`
	Scopes       []string `json:"scopes,omitempty"`
	Scope        string   `json:"scope,omitempty"`
	ExpiresIn    int      `json:"expires_in"`

	// MFA 两阶段登录字段（阶段 5.4）
	MFARequired  bool     `json:"mfa_required,omitempty"`
	MFAChallenge string   `json:"mfa_challenge,omitempty"`
	MFAMethods   []string `json:"mfa_methods,omitempty"`
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
//
// 阶段 5.3 契约修复：服务端 GET /api/v1/authorize 与 POST /api/v1/authorize/approve
// 返回的响应结构不同。使用同一个类型并通过 omitempty 兼容两种场景：
//   - GET /authorize 返回：ConsentToken/ClientID/RedirectURI/Scope/State/RequireApproval
//     （Code 为空，前端需展示授权同意页面）
//   - POST /authorize/approve 返回：Code/State
//     （ConsentToken 等字段为空，客户端使用 Code 调用 /token 端点换取 Access Token）
//
// 集成方应根据 RequireApproval 判断当前是处于"待同意"还是"已批准"状态。
type AuthorizeResponse struct {
	// GET /authorize 返回字段
	ConsentToken    string `json:"consent_token,omitempty"`
	ClientID        string `json:"client_id,omitempty"`
	RedirectURI     string `json:"redirect_uri,omitempty"`
	Scope           string `json:"scope,omitempty"`
	RequireApproval bool   `json:"require_approval,omitempty"`

	// POST /authorize/approve 返回字段
	Code  string `json:"code,omitempty"`
	State string `json:"state,omitempty"`
}

// AuthorizeDenyResponse 授权拒绝响应
//
// 阶段 5.3 新增：服务端返回 HTTP 403，error 固定为 "access_denied"。
// SDK 不应将其视为成功响应；调用方拿到此响应后应向客户端应用回传
// ?error=access_denied&state=xxx。
type AuthorizeDenyResponse struct {
	Error            string `json:"error"`
	ErrorDescription string `json:"error_description"`
	State            string `json:"state"`
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
