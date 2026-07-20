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

/**
 * 授权批准请求
 *
 * 阶段 5.3 契约修复：服务端 /api/v1/authorize/approve 实际期望 {consent_token, state}。
 * 旧字段（client_id/redirect_uri/scope/code_challenge 等）已通过 consent_token JWT 携带，
 * 不再需要重复传递。请求体启用 DisallowUnknownFields，多余字段会被拒绝。
 */
export interface AuthorizeApproveRequest {
  consent_token: string;
  state: string;
}

/**
 * 授权拒绝请求
 *
 * 阶段 5.3 新增：用户主动拒绝授权时调用 /api/v1/authorize/deny。
 */
export interface AuthorizeDenyRequest {
  consent_token: string;
  state: string;
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
  /**
   * Access Token（MFA 第一阶段响应中会省略，故为可选）
   * 来源：POST /api/v1/login（非 MFA）/ /api/v1/login/mfa/verify / /api/v1/token / /auth/{provider}/callback
   */
  access_token?: string;
  /** Refresh Token（MFA 第一阶段响应中会省略，故为可选） */
  refresh_token?: string;
  /** Token 类型（通常为 "Bearer"；MFA 第一阶段响应中会省略） */
  token_type?: string;
  /** 过期秒数（始终返回；MFA 场景下为 challenge 的 TTL） */
  expires_in: number;
  /** scopes 数组（来自 /login 与 /login/mfa/verify 端点） */
  scopes?: string[];
  /** scope 空格分隔字符串（来自 /api/v1/token authorization_code grant） */
  scope?: string;
  // 阶段 5.4 契约扩展：MFA 两阶段登录
  // 当 mfa_required 为 true 时，access_token/refresh_token 为空，
  // expires_in 表示 mfa_challenge 的 TTL（300 秒）。
  // 客户端需提示用户输入 MFA 验证码，再调用 verifyMFALogin 完成第二阶段登录。
  mfa_required?: boolean;
  mfa_challenge?: string;
  mfa_methods?: string[];
}

/**
 * MFA 两阶段登录第二阶段验证请求
 *
 * 阶段 5.4 契约扩展：当 login 返回 mfa_required=true 时，
 * 客户端使用返回的 mfa_challenge 调用本接口完成登录。
 *
 * - method: "totp"（6 位数字验证码）或 "recovery_code"（恢复码字符串）
 * - code: 与 method 对应的验证值
 *
 * 成功后服务端返回标准 TokenResponse（含 access_token/refresh_token）。
 */
export interface LoginMFAVerifyRequest {
  mfa_challenge: string;
  method: string;
  code: string;
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

/**
 * 授权响应
 *
 * 阶段 5.3 契约修复：服务端 GET /api/v1/authorize 与 POST /api/v1/authorize/approve
 * 返回的响应结构不同。使用同一个 interface 并通过可选字段兼容两种场景：
 *   - GET /authorize 返回：consent_token/client_id/redirect_uri/scope/state/require_approval
 *     （code 为空，前端需展示授权同意页面）
 *   - POST /authorize/approve 返回：code/state
 *     （consent_token 等字段为空，客户端使用 code 调用 /token 端点换取 Access Token）
 *
 * 集成方应根据 require_approval 判断当前是处于"待同意"还是"已批准"状态。
 */
export interface AuthorizeResponse {
  // GET /authorize 返回字段
  consent_token?: string;
  client_id?: string;
  redirect_uri?: string;
  scope?: string;
  require_approval?: boolean;

  // POST /authorize/approve 返回字段
  code?: string;
  state?: string;
}

/**
 * 授权拒绝响应
 *
 * 阶段 5.3 新增：服务端返回 HTTP 403，error 固定为 "access_denied"。
 * SDK 不应将其视为成功响应；调用方拿到此响应后应向客户端应用回传
 * ?error=access_denied&state=xxx。
 */
export interface AuthorizeDenyResponse {
  error: string;
  error_description: string;
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
