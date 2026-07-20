// ============================================================================
// 错误码常量
// ============================================================================

export const ErrorCode = {
  INTERNAL: 'INTERNAL_ERROR',
  BAD_REQUEST: 'BAD_REQUEST',
  NOT_FOUND: 'NOT_FOUND',
  CONFLICT: 'CONFLICT',
  UNAUTHORIZED: 'UNAUTHORIZED',
  FORBIDDEN: 'FORBIDDEN',
  TOO_MANY_REQUESTS: 'TOO_MANY_REQUESTS',
  INVALID_CREDENTIALS: 'INVALID_CREDENTIALS',
  ACCOUNT_LOCKED: 'ACCOUNT_LOCKED',
  ACCOUNT_DISABLED: 'ACCOUNT_DISABLED',
  INVALID_TOKEN: 'INVALID_TOKEN',
  TOKEN_EXPIRED: 'TOKEN_EXPIRED',
  EMAIL_EXISTS: 'EMAIL_EXISTS',
  EMAIL_INVALID: 'EMAIL_INVALID',
  EMAIL_REQUIRED: 'EMAIL_REQUIRED',
  PASSWORD_TOO_SHORT: 'PASSWORD_TOO_SHORT',
  PASSWORD_TOO_LONG: 'PASSWORD_TOO_LONG',
  PASSWORD_REQUIRED: 'PASSWORD_REQUIRED',
  INVALID_REQUEST_FORMAT: 'INVALID_REQUEST_FORMAT',
  REQUEST_BODY_TOO_LARGE: 'REQUEST_BODY_TOO_LARGE',
  MISSING_AUTH_CODE: 'MISSING_AUTH_CODE', // 社交登录回调未携带 code 参数

  // === 阶段 5 SDK 同步：服务端阶段 2/3/4 引入的错误码 ===

  // Token 轮换 / 重放（阶段 2.1）
  // Refresh Token 已被使用过又再次出现，重放攻击典型特征
  // SDK 收到此错误应清空本地 Token 并要求用户重新登录
  TOKEN_ROTATED: 'TOKEN_ROTATED',

  // OAuth Scope / PKCE / Consent（阶段 2.2）
  INVALID_SCOPE: 'INVALID_SCOPE',         // scope 超出客户端允许或白名单
  PKCE_REQUIRED: 'PKCE_REQUIRED',         // 公共客户端必须使用 PKCE（S256）
  CONSENT_REQUIRED: 'CONSENT_REQUIRED',  // 需要用户同意授权
  CONSENT_DENIED: 'CONSENT_DENIED',      // 用户拒绝授权
  CONSENT_INVALID: 'CONSENT_INVALID',    // consent_token 无效或已过期
  CLIENT_MISMATCH: 'CLIENT_MISMATCH',    // refresh_token 客户端归属不一致

  // MFA 两阶段登录（阶段 2.x）
  MFA_CHALLENGE_INVALID: 'MFA_CHALLENGE_INVALID',     // Challenge 无效或已被使用
  MFA_CHALLENGE_EXPIRED: 'MFA_CHALLENGE_EXPIRED',     // Challenge 已过期
  INVALID_MFA_CODE: 'INVALID_MFA_CODE',               // TOTP 或恢复码无效
  TOO_MANY_MFA_ATTEMPTS: 'TOO_MANY_MFA_ATTEMPTS',      // 尝试次数过多（默认 5 次）
  MFA_SERVICE_UNAVAILABLE: 'MFA_SERVICE_UNAVAILABLE', // MFA 服务未装配

  // Social Login 基础（阶段 2.2 改造）
  PROVIDER_NOT_SUPPORTED: 'PROVIDER_NOT_SUPPORTED',         // 提供商不支持
  OAUTH_CODE_EXCHANGE_FAILED: 'OAUTH_CODE_EXCHANGE_FAILED', // 授权码交换失败
  SOCIAL_LOGIN_FAILED: 'SOCIAL_LOGIN_FAILED',               // 社交登录失败
  OAUTH_STATE_INVALID: 'OAUTH_STATE_INVALID',               // state 无效
  OAUTH_STATE_EXPIRED: 'OAUTH_STATE_EXPIRED',               // state 已过期

  // Social Login 安全增强（阶段 2.3 新增）
  PROVIDER_EMAIL_NOT_VERIFIED: 'PROVIDER_EMAIL_NOT_VERIFIED', // provider 返回 email 未验证
  SOCIAL_ACCOUNT_CONFLICT: 'SOCIAL_ACCOUNT_CONFLICT',         // 社交账号已绑定到其他用户
  EMAIL_CONFLICT_WITH_LOCAL: 'EMAIL_CONFLICT_WITH_LOCAL',     // email 与本地账号冲突，需手动绑定
  PROVIDER_USER_ID_MISSING: 'PROVIDER_USER_ID_MISSING',       // provider 未返回 user_id

  // 邮件（阶段 4.3）
  // 服务端 SMTP 错误统一返回此通用错误码，不暴露 SMTP 内部信息
  EMAIL_SEND_FAILED: 'EMAIL_SEND_FAILED',
} as const;

export type ErrorCodeType = (typeof ErrorCode)[keyof typeof ErrorCode];

// ============================================================================
// SSOError 错误类
// ============================================================================

export class SSOError extends Error {
  public readonly httpStatus: number;
  public readonly code: string;
  public readonly rawBody: string;

  constructor(httpStatus: number, code: string, message: string, rawBody: string) {
    super(message);
    this.name = 'SSOError';
    this.httpStatus = httpStatus;
    this.code = code;
    this.rawBody = rawBody;
  }

  /** 是否为 404 错误 */
  isNotFound(): boolean {
    return this.httpStatus === 404;
  }

  /** 是否为 401 错误 */
  isUnauthorized(): boolean {
    return this.httpStatus === 401;
  }

  /** 是否为 403 错误 */
  isForbidden(): boolean {
    return this.httpStatus === 403;
  }

  /** 是否为 409 错误 */
  isConflict(): boolean {
    return this.httpStatus === 409;
  }

  /** 是否被限流 */
  isRateLimited(): boolean {
    return this.httpStatus === 429;
  }

  // ==========================================================================
  // 阶段 5.2 辅助判断方法
  //
  // 服务端阶段 2/3/4 引入了大量新的错误码，下游集成方若每次都通过字符串比较
  // 判断错误码可读性差且易错。这里提供一组语义化方法，覆盖最常见的安全处理分支。
  // ==========================================================================

  /** Refresh Token 已被使用过（重放攻击特征）
   *  收到此错误应立即清空本地 Token 并要求用户重新登录 */
  isTokenRotated(): boolean {
    return this.code === ErrorCode.TOKEN_ROTATED;
  }

  /** 需要用户同意授权（CONSENT_REQUIRED 或 CONSENT_INVALID）
   *  收到此错误应重新调用 authorize 获取 consent_token 并展示授权同意页面 */
  isConsentRequired(): boolean {
    return this.code === ErrorCode.CONSENT_REQUIRED || this.code === ErrorCode.CONSENT_INVALID;
  }

  /** 用户主动拒绝授权，应终止授权流程 */
  isConsentDenied(): boolean {
    return this.code === ErrorCode.CONSENT_DENIED;
  }

  /** 公共客户端必须使用 PKCE（S256），应生成 code_verifier 重新发起授权 */
  isPKCERequired(): boolean {
    return this.code === ErrorCode.PKCE_REQUIRED;
  }

  /** 请求的 scope 超出客户端允许范围或不在白名单 */
  isInvalidScope(): boolean {
    return this.code === ErrorCode.INVALID_SCOPE;
  }

  /** Refresh Token 与客户端归属不一致 */
  isClientMismatch(): boolean {
    return this.code === ErrorCode.CLIENT_MISMATCH;
  }

  /** MFA Challenge 无效或已被使用，应重新触发登录 */
  isMFAChallengeInvalid(): boolean {
    return this.code === ErrorCode.MFA_CHALLENGE_INVALID;
  }

  /** MFA Challenge 已过期，应重新触发登录 */
  isMFAChallengeExpired(): boolean {
    return this.code === ErrorCode.MFA_CHALLENGE_EXPIRED;
  }

  /** MFA 验证尝试次数过多（默认 5 次），challenge 已失效 */
  isTooManyMFAAttempts(): boolean {
    return this.code === ErrorCode.TOO_MANY_MFA_ATTEMPTS;
  }

  /** 社交登录相关错误（统一处理） */
  isSocialLoginError(): boolean {
    const socialLoginCodes: string[] = [
      ErrorCode.PROVIDER_NOT_SUPPORTED,
      ErrorCode.OAUTH_CODE_EXCHANGE_FAILED,
      ErrorCode.SOCIAL_LOGIN_FAILED,
      ErrorCode.OAUTH_STATE_INVALID,
      ErrorCode.OAUTH_STATE_EXPIRED,
      ErrorCode.PROVIDER_EMAIL_NOT_VERIFIED,
      ErrorCode.SOCIAL_ACCOUNT_CONFLICT,
      ErrorCode.EMAIL_CONFLICT_WITH_LOCAL,
      ErrorCode.PROVIDER_USER_ID_MISSING,
    ];
    return socialLoginCodes.includes(this.code);
  }

  /** 邮件发送失败（SMTP 错误统一返回此码，不暴露内部信息） */
  isEmailSendFailed(): boolean {
    return this.code === ErrorCode.EMAIL_SEND_FAILED;
  }

  override toString(): string {
    return `sso: ${this.code} (HTTP ${this.httpStatus}): ${this.message}`;
  }
}

/**
 * 解析错误响应体，创建 SSOError
 */
export function parseError(httpStatus: number, body: string): SSOError {
  let code = '';
  let message = '';

  try {
    const parsed = JSON.parse(body);
    code = parsed.code || parsed.error || '';
    message = parsed.message || '';
  } catch {
    // body 不是 JSON
  }

  return new SSOError(httpStatus, code, message || body, body);
}
