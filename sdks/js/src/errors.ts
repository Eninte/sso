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
