import type {
  SSOClientOptions,
  TokenResponse,
  LoginMFAVerifyRequest,
  RegisterResponse,
  UserInfo,
  MessageResponse,
  MFASetupResponse,
  MFAStatusResponse,
  AuthorizeResponse,
  AuthorizeApproveRequest,
  AuthorizeDenyRequest,
  AuthorizeDenyResponse,
  UserListResponse,
  UserItem,
  HealthResponse,
  DiscoveryResponse,
  JWKSResponse,
  OAuthProvider,
} from './types';
import { parseError, SSOError } from './errors';

export class SSOClient {
  private readonly baseURL: string;
  private readonly timeout: number;
  private readonly fetchFn: typeof fetch;

  private _accessToken: string;
  private _refreshToken: string;
  private _tokenExpiry: number = 0;

  constructor(baseURL: string, options: SSOClientOptions = {}) {
    this.baseURL = baseURL.replace(/\/+$/, '');
    this.timeout = options.timeout ?? 30000;
    this._accessToken = options.accessToken ?? '';
    this._refreshToken = options.refreshToken ?? '';
    this.fetchFn = options.fetch ?? globalThis.fetch.bind(globalThis);
  }

  getBaseURL(): string { return this.baseURL; }
  getAccessToken(): string { return this._accessToken; }

  setTokens(accessToken: string, refreshToken: string, expiresIn: number): void {
    this._accessToken = accessToken;
    this._refreshToken = refreshToken;
    this._tokenExpiry = Date.now() + expiresIn * 1000;
  }

  clearTokens(): void {
    this._accessToken = '';
    this._refreshToken = '';
    this._tokenExpiry = 0;
  }

  // =======================================================================
  // HTTP 请求
  // =======================================================================

  private async request<T>(method: string, path: string, body?: unknown, auth = false): Promise<T> {
    const url = `${this.baseURL}${path}`;
    const headers: Record<string, string> = {
      'Content-Type': 'application/json',
      Accept: 'application/json',
    };

    if (auth) {
      headers['Authorization'] = `Bearer ${await this.ensureToken()}`;
    }

    const controller = new AbortController();
    const timer = setTimeout(() => controller.abort(), this.timeout);

    try {
      const resp = await this.fetchFn(url, {
        method, headers,
        body: body ? JSON.stringify(body) : undefined,
        signal: controller.signal,
      });
      const text = await resp.text();
      if (!resp.ok) throw parseError(resp.status, text);
      return text ? JSON.parse(text) : ({} as T);
    } catch (err) {
      if (err instanceof DOMException && err.name === 'AbortError') {
        throw new Error(`sso: request timeout after ${this.timeout}ms`);
      }
      throw err;
    } finally {
      clearTimeout(timer);
    }
  }

  private async ensureToken(): Promise<string> {
    if (!this._accessToken) throw new Error('sso: no access token available, please login first');

    if (Date.now() > this._tokenExpiry - 30_000 && this._refreshToken) {
      const resp = await this.request<TokenResponse>('POST', '/api/v1/token', {
        grant_type: 'refresh_token', refresh_token: this._refreshToken,
      });
      // 阶段 B 审查修复：access_token/refresh_token 改为可选后需兜底
      const at = resp.access_token ?? '';
      if (!at) throw new Error('sso: refresh token response missing access_token');
      this.setTokens(at, resp.refresh_token ?? '', resp.expires_in);
      return at;
    }
    return this._accessToken;
  }

  // =======================================================================
  // 认证
  // =======================================================================

  async register(email: string, password: string): Promise<RegisterResponse> {
    return this.request<RegisterResponse>('POST', '/api/v1/register', { email, password });
  }

  /**
   * 用户登录（第一阶段）
   *
   * 阶段 5.4 契约扩展：当用户启用 MFA 时，服务端返回 mfa_required=true 与
   * 一次性 mfa_challenge 令牌（TTL 5 分钟），此时 access_token/refresh_token 为空。
   * 本方法在这种情况下不会调用 setTokens；调用方应检查 resp.mfa_required，
   * 若为 true 则提示用户输入 MFA 验证码并调用 verifyMFALogin 完成第二阶段登录。
   */
  async login(email: string, password: string): Promise<TokenResponse> {
    const resp = await this.request<TokenResponse>('POST', '/api/v1/login', { email, password });
    if (!resp.mfa_required) {
      // 阶段 B 审查修复：access_token/refresh_token 改为可选后需兜底
      this.setTokens(resp.access_token ?? '', resp.refresh_token ?? '', resp.expires_in);
    }
    return resp;
  }

  /**
   * MFA 两阶段登录第二阶段验证
   *
   * 阶段 5.4 契约扩展：POST /api/v1/login/mfa/verify
   * 使用 login 返回的 mfa_challenge 与用户输入的验证码完成登录。
   * 成功后服务端返回标准 TokenResponse，本方法会调用 setTokens 持久化。
   *
   * 失败错误码：MFA_CHALLENGE_INVALID / MFA_CHALLENGE_EXPIRED /
   * INVALID_MFA_CODE / TOO_MANY_MFA_ATTEMPTS / MFA_SERVICE_UNAVAILABLE
   */
  async verifyMFALogin(req: LoginMFAVerifyRequest): Promise<TokenResponse> {
    const resp = await this.request<TokenResponse>('POST', '/api/v1/login/mfa/verify', req);
    this.setTokens(resp.access_token ?? '', resp.refresh_token ?? '', resp.expires_in);
    return resp;
  }

  async refreshToken(): Promise<TokenResponse> {
    if (!this._refreshToken) throw new Error('sso: no refresh token available');
    const resp = await this.request<TokenResponse>('POST', '/api/v1/token', {
      grant_type: 'refresh_token', refresh_token: this._refreshToken,
    });
    this.setTokens(resp.access_token ?? '', resp.refresh_token ?? '', resp.expires_in);
    return resp;
  }

  async exchangeCode(code: string, clientId: string, clientSecret: string, redirectUri: string, codeVerifier?: string): Promise<TokenResponse> {
    // 服务端 authorization_code 响应使用 handlerutil.WriteJSONSuccess 包裹在 {"data":{...}}，
    // 需先剥离外层 data 后再返回 TokenResponse。
    const wrapper = await this.request<{ data: TokenResponse }>('POST', '/api/v1/token', {
      grant_type: 'authorization_code', code, client_id: clientId,
      client_secret: clientSecret, redirect_uri: redirectUri, code_verifier: codeVerifier,
    });
    return wrapper.data;
  }

  async revokeToken(): Promise<MessageResponse> {
    if (!this._accessToken) return { message: 'no token to revoke' };
    const resp = await this.request<MessageResponse>('POST', '/api/v1/token/revoke', { token: this._accessToken });
    this.clearTokens();
    return resp;
  }

  async forgotPassword(email: string): Promise<MessageResponse> {
    return this.request<MessageResponse>('POST', '/api/v1/forgot-password', { email });
  }

  async resetPassword(token: string, userId: string, newPassword: string): Promise<MessageResponse> {
    return this.request<MessageResponse>('POST', '/api/v1/reset-password', { token, user_id: userId, new_password: newPassword });
  }

  async verifyEmail(token: string, userId: string): Promise<MessageResponse> {
    return this.request<MessageResponse>('GET', `/api/v1/verify-email?token=${encodeURIComponent(token)}&user_id=${encodeURIComponent(userId)}`);
  }

  async sendVerificationEmail(): Promise<MessageResponse> {
    return this.request<MessageResponse>('POST', '/api/v1/verify-email/send', undefined, true);
  }

  // =======================================================================
  // 用户
  // =======================================================================

  async userInfo(): Promise<UserInfo> {
    return this.request<UserInfo>('GET', '/api/v1/userinfo', undefined, true);
  }

  async changePassword(oldPassword: string, newPassword: string): Promise<MessageResponse> {
    return this.request<MessageResponse>('POST', '/api/v1/change-password', { old_password: oldPassword, new_password: newPassword }, true);
  }

  // =======================================================================
  // OAuth2
  // =======================================================================

  async authorize(clientId: string, redirectUri: string, scope: string, state: string): Promise<AuthorizeResponse> {
    const p = new URLSearchParams({ client_id: clientId, redirect_uri: redirectUri, response_type: 'code', scope, state });
    return this.request<AuthorizeResponse>('GET', `/api/v1/authorize?${p}`, undefined, true);
  }

  async authorizeWithPKCE(clientId: string, redirectUri: string, scope: string, state: string, codeChallenge: string): Promise<AuthorizeResponse> {
    const p = new URLSearchParams({ client_id: clientId, redirect_uri: redirectUri, response_type: 'code', scope, state, code_challenge: codeChallenge, code_challenge_method: 'S256' });
    return this.request<AuthorizeResponse>('GET', `/api/v1/authorize?${p}`, undefined, true);
  }

  /**
   * 批准 OAuth2 授权
   *
   * 阶段 5.3 契约修复：服务端期望请求体 {consent_token, state}，
   * 不再接受 client_id/redirect_uri/scope 等字段（consent_token JWT 内部已携带）。
   * 调用方需先调用 authorize/authorizeWithPKCE 获取 consent_token，再传给本方法。
   *
   * 成功后返回 {code, state}，使用 code 调用 exchangeCodeForToken 换取 Access Token。
   */
  async approveAuthorization(req: AuthorizeApproveRequest): Promise<AuthorizeResponse> {
    return this.request<AuthorizeResponse>('POST', '/api/v1/authorize/approve', req, true);
  }

  /**
   * 拒绝 OAuth2 授权
   *
   * 阶段 5.3 新增：用户主动拒绝授权时调用 /api/v1/authorize/deny。
   * 服务端固定返回 HTTP 403 + {error:"access_denied", error_description, state}，
   * 本方法将此响应当作正常的 DenyResponse 返回（不视为错误），
   * 调用方拿到后应向客户端应用回传 ?error=access_denied&state=xxx。
   *
   * 注意：仅在用户主动拒绝时调用；其他场景的 403 仍按错误处理。
   */
  async denyAuthorization(req: AuthorizeDenyRequest): Promise<AuthorizeDenyResponse> {
    try {
      const text = await this.requestRaw('POST', '/api/v1/authorize/deny', req, true);
      // 理论上不会进入此分支（服务端固定返回 403），但保留兼容性
      return JSON.parse(text) as AuthorizeDenyResponse;
    } catch (err) {
      if (err instanceof SSOError && err.httpStatus === 403) {
        try {
          return JSON.parse(err.rawBody) as AuthorizeDenyResponse;
        } catch {
          // 解析失败则重新抛出原错误
        }
      }
      throw err;
    }
  }

  /**
   * 发送原始请求并返回文本（不解析 JSON，不抛出非 2xx 异常的 raw body 通过 SSOError.rawBody 暴露）
   */
  private async requestRaw(method: string, path: string, body: unknown, auth: boolean): Promise<string> {
    const url = `${this.baseURL}${path}`;
    const headers: Record<string, string> = {
      'Content-Type': 'application/json',
      Accept: 'application/json',
    };
    if (auth) {
      headers['Authorization'] = `Bearer ${await this.ensureToken()}`;
    }
    const controller = new AbortController();
    const timer = setTimeout(() => controller.abort(), this.timeout);
    try {
      const resp = await this.fetchFn(url, {
        method, headers,
        body: body ? JSON.stringify(body) : undefined,
        signal: controller.signal,
      });
      const text = await resp.text();
      if (!resp.ok) throw parseError(resp.status, text);
      return text;
    } finally {
      clearTimeout(timer);
    }
  }

  // =======================================================================
  // Social Login 社交登录
  //
  // 阶段 5.5 新增：服务端契约
  //   - GET /auth/providers         公开端点，直接返回数组（不包裹 data）
  //   - GET /auth/{provider}?state= 公开端点，返回 HTTP 307 重定向到 provider 授权页面
  //   - GET /auth/{provider}/callback?code=&state= 公开端点，平铺返回 TokenResponse
  // =======================================================================

  /**
   * 获取支持的社交登录提供商列表
   *
   * 阶段 5.5 新增：调用 GET /auth/providers 公开端点。
   * 服务端直接返回数组（不包裹在 data 中），无需认证。
   */
  async getProviders(): Promise<OAuthProvider[]> {
    return this.request<OAuthProvider[]>('GET', '/auth/providers');
  }

  /**
   * 构造发起社交登录的 URL
   *
   * 阶段 5.5 新增：直接构造 URL 字符串，不发起 HTTP 请求。
   * 调用方应使用浏览器重定向到此 URL（服务端会返回 307 到 provider 授权页面），
   * 而不是 SDK 直接 GET。
   *
   * @param provider 社交登录提供商名称（如 "google" / "github"）
   * @param state    可选，CSRF 防护 state；为空时由服务端自动生成 UUID
   */
  getSocialLoginURL(provider: string, state?: string): string {
    const encoded = encodeURIComponent(provider);
    let url = `${this.baseURL}/auth/${encoded}`;
    if (state) {
      url += `?state=${encodeURIComponent(state)}`;
    }
    return url;
  }

  /**
   * 用回调返回的 code+state 完成社交登录
   *
   * 阶段 5.5 新增：调用 GET /auth/{provider}/callback?code={code}&state={state} 公开端点。
   * 服务端直接平铺返回 TokenResponse（不包裹 data），无需认证。
   * 成功后调用 setTokens 缓存到客户端。
   *
   * 失败错误码：MISSING_AUTH_CODE / OAUTH_STATE_INVALID / OAUTH_STATE_EXPIRED /
   * PROVIDER_NOT_SUPPORTED / OAUTH_CODE_EXCHANGE_FAILED / SOCIAL_LOGIN_FAILED /
   * PROVIDER_USER_ID_MISSING / PROVIDER_EMAIL_NOT_VERIFIED /
   * SOCIAL_ACCOUNT_CONFLICT / EMAIL_CONFLICT_WITH_LOCAL / ACCOUNT_DISABLED / ACCOUNT_LOCKED
   */
  async exchangeSocialCode(provider: string, code: string, state: string): Promise<TokenResponse> {
    const path = `/auth/${encodeURIComponent(provider)}/callback?code=${encodeURIComponent(code)}&state=${encodeURIComponent(state)}`;
    const resp = await this.request<TokenResponse>('GET', path);
    this.setTokens(resp.access_token ?? '', resp.refresh_token ?? '', resp.expires_in);
    return resp;
  }

  // =======================================================================
  // MFA
  // =======================================================================

  async mfaSetup(): Promise<MFASetupResponse> {
    return this.request<MFASetupResponse>('POST', '/api/v1/mfa/setup', undefined, true);
  }

  async mfaVerify(code: string): Promise<MessageResponse> {
    return this.request<MessageResponse>('POST', '/api/v1/mfa/verify', { code }, true);
  }

  async mfaDisable(code: string): Promise<MessageResponse> {
    return this.request<MessageResponse>('POST', '/api/v1/mfa/disable', { code }, true);
  }

  async mfaStatus(): Promise<MFAStatusResponse> {
    return this.request<MFAStatusResponse>('GET', '/api/v1/mfa/status', undefined, true);
  }

  // =======================================================================
  // 管理员
  // =======================================================================

  async adminHealth(): Promise<HealthResponse> {
    return this.request<HealthResponse>('GET', '/api/v1/admin/health', undefined, true);
  }

  async adminCleanup(): Promise<MessageResponse> {
    return this.request<MessageResponse>('POST', '/api/v1/admin/cleanup', undefined, true);
  }

  async listUsers(page: number, pageSize: number): Promise<UserListResponse> {
    return this.request<UserListResponse>('GET', `/api/v1/admin/users?page=${page}&pageSize=${pageSize}`, undefined, true);
  }

  async getUser(userId: string): Promise<UserItem> {
    // 使用路径参数（服务端契约：GET /api/v1/admin/users/{id}）
    return this.request<UserItem>('GET', `/api/v1/admin/users/${encodeURIComponent(userId)}`, undefined, true);
  }

  async disableUser(userId: string): Promise<MessageResponse> {
    // 使用路径参数，不发送请求体（服务端契约：POST /api/v1/admin/users/{id}/disable）
    return this.request<MessageResponse>('POST', `/api/v1/admin/users/${encodeURIComponent(userId)}/disable`, undefined, true);
  }

  async enableUser(userId: string): Promise<MessageResponse> {
    // 使用路径参数，不发送请求体（服务端契约：POST /api/v1/admin/users/{id}/enable）
    return this.request<MessageResponse>('POST', `/api/v1/admin/users/${encodeURIComponent(userId)}/enable`, undefined, true);
  }

  // =======================================================================
  // OIDC
  // =======================================================================

  async discovery(): Promise<DiscoveryResponse> {
    return this.request<DiscoveryResponse>('GET', '/.well-known/openid-configuration');
  }

  async jwks(): Promise<JWKSResponse> {
    return this.request<JWKSResponse>('GET', '/.well-known/jwks.json');
  }
}
