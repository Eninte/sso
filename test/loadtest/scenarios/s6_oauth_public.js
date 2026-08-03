// S6: OAuth 公共客户端完整流程 (PKCE)
// 链路: GET /api/v1/authorize → POST /api/v1/authorize/approve → POST /api/v1/token → GET /api/v1/userinfo

import { check, group, sleep } from 'k6';
import http from 'k6/http';
import {
  loginUser,
  authorizeGet,
  authorizeApprove,
  exchangeCode,
  getUserInfo,
  generatePKCE,
  generateState,
  randomItem,
} from '../helpers.js';
import {
  BASE_URL,
  PUBLIC_CLIENT_ID,
  REDIRECT_URI,
} from '../config.js';

export const options = {
  stages: [
    { duration: '1m', target: 10 },
    { duration: '2m', target: 20 },
    { duration: '3m', target: 50 },
    { duration: '3m', target: 100 },
    { duration: '3m', target: 150 },
    { duration: '2m', target: 0 },
  ],
  thresholds: {
    http_req_duration: ['p(95)<500'],
    http_req_failed: ['rate<0.02'],
  },
};

const userPool = JSON.parse(open(__ENV.USER_POOL_FILE || 'data/users.json', 'r'));

export default function () {
  const user = randomItem(userPool);
  const pkce = generatePKCE();
  const state = generateState();

  // 1. 登录获取 token
  const tokens = loginUser(BASE_URL, user.email, user.password);
  if (!tokens) return;

  // 2. GET /authorize (发起授权请求)
  const authRes = authorizeGet(BASE_URL, tokens.accessToken, PUBLIC_CLIENT_ID, REDIRECT_URI, state, pkce.codeChallenge);
  if (!check(authRes, { 'authorize GET: status 200': (r) => r.status === 200 })) return;

  // 3. POST /authorize/approve (批准授权)
  const approveRes = authorizeApprove(BASE_URL, tokens.accessToken, PUBLIC_CLIENT_ID, REDIRECT_URI, state, pkce.codeChallenge);
  if (!check(approveRes, { 'authorize approve: status 200': (r) => r.status === 200 })) return;

  const code = approveRes.json().code;

  // 4. 用 code 换 token (PKCE)
  const tokenRes = exchangeCode(BASE_URL, code, PUBLIC_CLIENT_ID, REDIRECT_URI, null, pkce.codeVerifier);
  if (!check(tokenRes, { 'token exchange: status 200': (r) => r.status === 200 })) return;

  const newAccessToken = tokenRes.json().access_token;

  // 5. 获取 userinfo
  getUserInfo(BASE_URL, newAccessToken);

  sleep(1);
}
