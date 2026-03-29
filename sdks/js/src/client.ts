import type {
  SSOClientOptions,
  TokenResponse,
  RegisterResponse,
  UserInfo,
  MessageResponse,
  MFASetupResponse,
  MFAStatusResponse,
  AuthorizeResponse,
  AuthorizeApproveRequest,
  UserListResponse,
  UserItem,
  HealthResponse,
  DiscoveryResponse,
  JWKSResponse,
} from './types';
import { parseError } from './errors';

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
      this.setTokens(resp.access_token, resp.refresh_token, resp.expires_in);
      return resp.access_token;
    }
    return this._accessToken;
  }

  // =======================================================================
  // 认证
  // =======================================================================

  async register(email: string, password: string): Promise<RegisterResponse> {
    return this.request<RegisterResponse>('POST', '/api/v1/register', { email, password });
  }

  async login(email: string, password: string): Promise<TokenResponse> {
    const resp = await this.request<TokenResponse>('POST', '/api/v1/login', { email, password });
    this.setTokens(resp.access_token, resp.refresh_token, resp.expires_in);
    return resp;
  }

  async refreshToken(): Promise<TokenResponse> {
    if (!this._refreshToken) throw new Error('sso: no refresh token available');
    const resp = await this.request<TokenResponse>('POST', '/api/v1/token', {
      grant_type: 'refresh_token', refresh_token: this._refreshToken,
    });
    this.setTokens(resp.access_token, resp.refresh_token, resp.expires_in);
    return resp;
  }

  async exchangeCode(code: string, clientId: string, clientSecret: string, redirectUri: string, codeVerifier?: string): Promise<TokenResponse> {
    return this.request<TokenResponse>('POST', '/api/v1/token', {
      grant_type: 'authorization_code', code, client_id: clientId,
      client_secret: clientSecret, redirect_uri: redirectUri, code_verifier: codeVerifier,
    });
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

  async approveAuthorization(req: AuthorizeApproveRequest): Promise<AuthorizeResponse> {
    return this.request<AuthorizeResponse>('POST', '/api/v1/authorize/approve', req, true);
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
    return this.request<HealthResponse>('GET', '/admin/health', undefined, true);
  }

  async adminCleanup(): Promise<MessageResponse> {
    return this.request<MessageResponse>('POST', '/admin/cleanup', undefined, true);
  }

  async listUsers(page: number, pageSize: number): Promise<UserListResponse> {
    return this.request<UserListResponse>('GET', `/admin/users?page=${page}&pageSize=${pageSize}`, undefined, true);
  }

  async getUser(userId: string): Promise<UserItem> {
    return this.request<UserItem>('GET', `/admin/users?id=${userId}`, undefined, true);
  }

  async disableUser(userId: string): Promise<MessageResponse> {
    return this.request<MessageResponse>('POST', '/admin/users/disable', { user_id: userId }, true);
  }

  async enableUser(userId: string): Promise<MessageResponse> {
    return this.request<MessageResponse>('POST', '/admin/users/enable', { user_id: userId }, true);
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
