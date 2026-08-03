// S7: OAuth 机密客户端完整流程
// 链路: GET /api/v1/authorize → POST /api/v1/authorize/approve → POST /api/v1/token → GET /api/v1/userinfo

import { check, group, sleep } from 'k6';
import http from 'k6/http';
import {
  loginUser,
  authorizeGet,
  authorizeApprove,
  exchangeCode,
  getUserInfo,
  generateState,
  randomItem,
} from '../helpers.js';
import {
  BASE_URL,
  CONFIDENTIAL_CLIENT_ID,
  CLIENT_SECRET,
  REDIRECT_URI,
} from '../config.js';

export const options = {
  stages: [
    { duration: '1m', target: 10 },
    { duration: '2m', target: 20 },
    { duration: '3m', target: 50 },
    { duration: '3m', target: 100 },
    { duration: '3m', target: 200 },
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
  const state = generateState();

  // 1. 登录获取 token
  const tokens = loginUser(BASE_URL, user.email, user.password);
  if (!tokens) return;

  // 2. GET /authorize (发起授权请求，机密客户端无需 PKCE)
  const authRes = authorizeGet(BASE_URL, tokens.accessToken, CONFIDENTIAL_CLIENT_ID, REDIRECT_URI, state);
  if (!check(authRes, { 'authorize GET: status 200': (r) => r.status === 200 })) return;

  // 3. POST /authorize/approve (批准授权)
  const approveRes = authorizeApprove(BASE_URL, tokens.accessToken, CONFIDENTIAL_CLIENT_ID, REDIRECT_URI, state);
  if (!check(approveRes, { 'authorize approve: status 200': (r) => r.status === 200 })) return;

  const code = approveRes.json().code;

  // 4. 用 code 换 token (机密客户端使用 client_secret)
  const tokenRes = exchangeCode(BASE_URL, code, CONFIDENTIAL_CLIENT_ID, REDIRECT_URI, CLIENT_SECRET);
  if (!check(tokenRes, { 'token exchange: status 200': (r) => r.status === 200 })) return;

  const newAccessToken = tokenRes.json().access_token;

  // 5. 获取 userinfo
  getUserInfo(BASE_URL, newAccessToken);

  sleep(1);
}
