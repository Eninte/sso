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

// ============================================================================
// 响应类型
// ============================================================================

#[derive(Debug, Deserialize)]
pub struct TokenResponse {
    pub access_token: String,
    pub refresh_token: String,
    pub token_type: String,
    pub expires_in: u64,
    #[serde(default)]
    pub scopes: Vec<String>,
    #[serde(default)]
    pub scope: String,
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

#[derive(Debug, Deserialize)]
pub struct AuthorizeResponse {
    pub code: String,
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
