use serde::{Deserialize, Serialize};

// ============================================================================
// 请求类型
// ============================================================================

#[derive(Debug, Serialize)]
pub struct RegisterRequest {
    pub email: String,
    pub password: String,
}

#[derive(Debug, Serialize)]
pub struct LoginRequest {
    pub email: String,
    pub password: String,
}

#[derive(Debug, Serialize)]
pub struct TokenRequest {
    pub grant_type: String,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub refresh_token: Option<String>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub code: Option<String>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub redirect_uri: Option<String>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub client_id: Option<String>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub client_secret: Option<String>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub code_verifier: Option<String>,
}

#[derive(Debug, Serialize)]
pub struct RevokeRequest {
    pub token: String,
}

#[derive(Debug, Serialize)]
pub struct ChangePasswordRequest {
    pub old_password: String,
    pub new_password: String,
}

#[derive(Debug, Serialize)]
pub struct MFAVerifyRequest {
    pub code: String,
}

#[derive(Debug, Serialize)]
pub struct DisableUserRequest {
    pub user_id: String,
}

#[derive(Debug, Serialize)]
pub struct EnableUserRequest {
    pub user_id: String,
}

/// MFA 两阶段登录第二阶段验证请求
///
/// 阶段 5.4 契约扩展：当 login 返回 mfa_required=true 时，
/// 客户端使用返回的 mfa_challenge 调用本接口完成登录。
///
/// - method: "totp"（6 位数字验证码）或 "recovery_code"（恢复码字符串）
/// - code: 与 method 对应的验证值
///
/// 成功后服务端返回标准 TokenResponse（含 access_token/refresh_token）。
#[derive(Debug, Serialize)]
pub struct LoginMFAVerifyRequest {
    pub mfa_challenge: String,
    pub method: String,
    pub code: String,
}

// ============================================================================
// 响应类型
// ============================================================================

/// 登录/Token 响应
///
/// 阶段 5.4 契约扩展：当用户启用 MFA 时，服务端在第一阶段不返回 access_token/
/// refresh_token（服务端 model 使用 omitempty），而是返回 mfa_required=true 与
/// 一次性 mfa_challenge 令牌（TTL 5 分钟）。
///
/// 因此所有原本"必填"的字段都加上了 #[serde(default)]，以便在 MFA 场景下也能
/// 成功反序列化。调用方应通过 mfa_required 判断是否需要第二阶段验证。
#[derive(Debug, Deserialize)]
pub struct TokenResponse {
    #[serde(default)]
    pub access_token: String,
    #[serde(default)]
    pub refresh_token: String,
    #[serde(default)]
    pub token_type: String,
    pub expires_in: u64,
    #[serde(default)]
    pub scopes: Vec<String>,
    #[serde(default)]
    pub scope: String,
    // 阶段 5.4 契约扩展：MFA 两阶段登录字段
    #[serde(default)]
    pub mfa_required: bool,
    #[serde(default)]
    pub mfa_challenge: String,
    #[serde(default)]
    pub mfa_methods: Vec<String>,
}

#[derive(Debug, Deserialize)]
pub struct RegisterResponse {
    pub message: String,
    pub data: Option<RegisterData>,
}

#[derive(Debug, Deserialize)]
pub struct RegisterData {
    pub user_id: String,
    pub email: String,
}

#[derive(Debug, Deserialize)]
pub struct UserInfo {
    pub sub: String,
    pub email: String,
    pub email_verified: bool,
}

#[derive(Debug, Deserialize)]
pub struct MessageResponse {
    pub message: String,
}

#[derive(Debug, Deserialize)]
pub struct MFASetupResponse {
    pub secret: String,
    pub qr_code_url: String,
    pub manual_entry: String,
}

#[derive(Debug, Deserialize)]
pub struct MFAStatusResponse {
    pub enabled: bool,
}

/// 授权响应
///
/// 阶段 5.3 契约修复：服务端 GET /api/v1/authorize 与 POST /api/v1/authorize/approve
/// 返回的响应结构不同。使用同一个 struct 并通过 serde(default) 兼容两种场景：
///   - GET /authorize 返回：consent_token/client_id/redirect_uri/scope/state/require_approval
///     （code 为空，前端需展示授权同意页面）
///   - POST /authorize/approve 返回：code/state
///     （consent_token 等字段为空，客户端使用 code 调用 /token 端点换取 Access Token）
///
/// 集成方应根据 require_approval 判断当前是处于"待同意"还是"已批准"状态。
#[derive(Debug, Deserialize, Default)]
pub struct AuthorizeResponse {
    // GET /authorize 返回字段
    #[serde(default)]
    pub consent_token: String,
    #[serde(default)]
    pub client_id: String,
    #[serde(default)]
    pub redirect_uri: String,
    #[serde(default)]
    pub scope: String,
    #[serde(default)]
    pub require_approval: bool,

    // POST /authorize/approve 返回字段
    #[serde(default)]
    pub code: String,
    #[serde(default)]
    pub state: String,
}

/// 授权批准请求
///
/// 阶段 5.3 契约修复：服务端 /api/v1/authorize/approve 实际期望 {consent_token, state}。
/// 旧字段（client_id/redirect_uri/scope/code_challenge 等）已通过 consent_token JWT 携带，
/// 不再需要重复传递。请求体启用 DisallowUnknownFields，多余字段会被拒绝。
#[derive(Debug, Serialize)]
pub struct AuthorizeApproveRequest {
    pub consent_token: String,
    pub state: String,
}

/// 授权拒绝请求
///
/// 阶段 5.3 新增：用户主动拒绝授权时调用 /api/v1/authorize/deny。
#[derive(Debug, Serialize)]
pub struct AuthorizeDenyRequest {
    pub consent_token: String,
    pub state: String,
}

/// 授权拒绝响应
///
/// 阶段 5.3 新增：服务端返回 HTTP 403，error 固定为 "access_denied"。
/// SDK 不应将其视为成功响应；调用方拿到此响应后应向客户端应用回传
/// ?error=access_denied&state=xxx。
#[derive(Debug, Deserialize, Default)]
pub struct AuthorizeDenyResponse {
    #[serde(default)]
    pub error: String,
    #[serde(default)]
    pub error_description: String,
    #[serde(default)]
    pub state: String,
}

#[derive(Debug, Deserialize)]
pub struct UserListResponse {
    pub users: Vec<UserItem>,
    pub total: u64,
    pub page: u64,
    pub page_size: u64,
    pub total_pages: u64,
}

#[derive(Debug, Deserialize)]
pub struct UserItem {
    pub id: String,
    pub email: String,
    pub email_verified: bool,
    pub mfa_enabled: bool,
    pub status: String,
    pub created_at: String,
    pub updated_at: String,
}

#[derive(Debug, Deserialize)]
pub struct HealthResponse {
    pub status: String,
    pub timestamp: String,
    pub database: String,
    pub version: String,
}

#[derive(Debug, Deserialize)]
pub struct DiscoveryResponse {
    pub issuer: String,
    pub authorization_endpoint: String,
    pub token_endpoint: String,
    pub userinfo_endpoint: String,
    pub jwks_uri: String,
    pub revocation_endpoint: String,
    pub grant_types_supported: Vec<String>,
    pub code_challenge_methods_supported: Vec<String>,
}

#[derive(Debug, Deserialize)]
pub struct JWK {
    pub kty: String,
    #[serde(rename = "use")]
    pub use_: String,
    pub kid: String,
    pub n: String,
    pub e: String,
}

#[derive(Debug, Deserialize)]
pub struct JWKSResponse {
    pub keys: Vec<JWK>,
}

/// OAuth 提供商
///
/// 阶段 5.5 新增：社交登录提供商信息，由 GET /auth/providers 返回。
/// 服务端直接返回数组（不包裹在 data 中）。所有字段使用 #[serde(default)]
/// 以兼容服务端可能省略某些字段的情况（如 icon 缺失）。
#[derive(Debug, Deserialize, Default)]
pub struct OAuthProvider {
    #[serde(default)]
    pub name: String,
    #[serde(default)]
    pub label: String,
    #[serde(default)]
    pub icon: String,
}
