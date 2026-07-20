// SSO SDK - JavaScript/TypeScript 客户端

export { SSOClient } from './client';
export { SSOError, ErrorCode } from './errors';
export type { ErrorCodeType } from './errors';

export type {
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
  JWK,
  OAuthProvider,
} from './types';

import { SSOClient } from './client';
import type { SSOClientOptions } from './types';

/** 创建 SSO 客户端 */
export function createSSOClient(baseURL: string, options?: SSOClientOptions): SSOClient {
  return new SSOClient(baseURL, options);
}
