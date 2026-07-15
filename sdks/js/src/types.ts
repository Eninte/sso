// ============================================================================
// 请求类型
// ============================================================================

export interface RegisterRequest {
  email: string;
  password: string;
}

export interface LoginRequest {
  email: string;
  password: string;
}

export interface TokenRequest {
  grant_type: string;
  refresh_token?: string;
  code?: string;
  redirect_uri?: string;
  client_id?: string;
  client_secret?: string;
  code_verifier?: string;
}

export interface RevokeRequest {
  token: string;
}

export interface ForgotPasswordRequest {
  email: string;
}

export interface ResetPasswordRequest {
  token: string;
  user_id: string;
  new_password: string;
}

export interface ChangePasswordRequest {
  old_password: string;
  new_password: string;
}

export interface AuthorizeApproveRequest {
  client_id: string;
  redirect_uri: string;
  scope: string;
  state: string;
  code_challenge?: string;
  code_challenge_method?: string;
}

export interface MFAVerifyRequest {
  code: string;
}

export interface DisableUserRequest {
  user_id: string;
}

export interface EnableUserRequest {
  user_id: string;
}

// ============================================================================
// 响应类型
// ============================================================================

export interface TokenResponse {
  access_token: string;
  refresh_token: string;
  token_type: string;
  expires_in: number;
  scopes?: string[];
  scope?: string;
}

export interface RegisterResponse {
  message: string;
  data?: {
    user_id: string;
    email: string;
  };
}

export interface UserInfo {
  sub: string;
  email: string;
  email_verified: boolean;
  // 服务端返回单数 key "scope"（值为字符串数组），详见 userinfo handler
  scope?: string[];
}

export interface MessageResponse {
  message: string;
}

export interface MFASetupResponse {
  secret: string;
  qr_code_url: string;
  manual_entry: string;
}

export interface MFAStatusResponse {
  enabled: boolean;
}

export interface AuthorizeResponse {
  code: string;
  state: string;
}

export interface UserListResponse {
  users: UserItem[];
  total: number;
  page: number;
  page_size: number;
  total_pages: number;
}

export interface UserItem {
  id: string;
  email: string;
  email_verified: boolean;
  mfa_enabled: boolean;
  status: string;
  created_at: string;
  updated_at: string;
}

export interface HealthResponse {
  status: string;
  timestamp: string;
  database: string;
  version: string;
}

export interface DiscoveryResponse {
  issuer: string;
  authorization_endpoint: string;
  token_endpoint: string;
  userinfo_endpoint: string;
  jwks_uri: string;
  revocation_endpoint: string;
  response_types_supported: string[];
  grant_types_supported: string[];
  subject_types_supported: string[];
  id_token_signing_alg_values_supported: string[];
  scopes_supported: string[];
  code_challenge_methods_supported: string[];
}

export interface JWKSResponse {
  keys: JWK[];
}

export interface JWK {
  kty: string;
  use: string;
  kid: string;
  n: string;
  e: string;
}

export interface OAuthProvider {
  name: string;
  label: string;
  icon: string;
}

// ============================================================================
// 配置选项
// ============================================================================

export interface SSOClientOptions {
  /** 请求超时（毫秒），默认 30000 */
  timeout?: number;
  /** 预设 Access Token */
  accessToken?: string;
  /** 预设 Refresh Token */
  refreshToken?: string;
  /** 自定义 fetch 函数 */
  fetch?: typeof fetch;
}
