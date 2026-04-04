// Soak Test: 长稳态测试
// 以混合流量 50%-70% 强度持续运行

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
  TEST_PASSWORD,
  PUBLIC_CLIENT_ID,
  CONFIDENTIAL_CLIENT_ID,
  CLIENT_SECRET,
  REDIRECT_URI,
} from '../config.js';

const SOAK_DURATION = __ENV.SOAK_DURATION || '60m';
const SOAK_VUS = parseInt(__ENV.SOAK_VUS || '50', 10);

export const options = {
  stages: [
    { duration: '5m', target: Math.floor(SOAK_VUS * 0.6) },
    { duration: SOAK_DURATION, target: SOAK_VUS },
    { duration: '5m', target: 0 },
  ],
  thresholds: {
    http_req_duration: ['p(95)<1500'],
    http_req_failed: ['rate<0.02'],
  },
};

// 数据池：模块顶层读取一次
const userPool = JSON.parse(open(__ENV.USER_POOL_FILE || 'data/users.json', 'r'));
const accessTokenPool = JSON.parse(open(__ENV.ACCESS_TOKEN_POOL_FILE || 'data/access_tokens.json', 'r'));
const refreshTokenPool = JSON.parse(open(__ENV.REFRESH_TOKEN_POOL_FILE || 'data/refresh_tokens.json', 'r'));

export default function () {
  const r = Math.random();

  if (r < 0.40) {
    scenarioUserinfo();
  } else if (r < 0.60) {
    scenarioRefresh();
  } else if (r < 0.75) {
    scenarioLogin();
  } else if (r < 0.85) {
    scenarioOAuthPublic();
  } else {
    scenarioOAuthConfidential();
  }

  sleep(0.5);
}

function scenarioUserinfo() {
  const entry = randomItem(accessTokenPool);
  getUserInfo(BASE_URL, entry.access_token);
}

function scenarioRefresh() {
  const entry = randomItem(refreshTokenPool);
  const res = http.post(`${BASE_URL}/api/v1/token`, JSON.stringify({
    grant_type: 'refresh_token',
    refresh_token: entry.refresh_token,
  }), { headers: { 'Content-Type': 'application/json' } });
  check(res, { 'refresh: 200': (r) => r.status === 200 });
}

function scenarioLogin() {
  const user = randomItem(userPool);
  const res = http.post(`${BASE_URL}/api/v1/login`, JSON.stringify({
    email: user.email,
    password: user.password,
  }), { headers: { 'Content-Type': 'application/json' } });
  check(res, { 'login: 200': (r) => r.status === 200 });
}

function scenarioOAuthPublic() {
  const user = randomItem(userPool);
  const tokens = loginUser(BASE_URL, user.email, user.password);
  if (!tokens) return;

  const pkce = generatePKCE();
  const state = generateState();

  const authRes = authorizeGet(BASE_URL, tokens.accessToken, PUBLIC_CLIENT_ID, REDIRECT_URI, state, pkce.codeChallenge);
  if (authRes.status !== 200) return;

  const approveRes = authorizeApprove(BASE_URL, tokens.accessToken, PUBLIC_CLIENT_ID, REDIRECT_URI, state, pkce.codeChallenge);
  if (approveRes.status !== 200) return;

  const code = approveRes.json().code;
  const tokenRes = exchangeCode(BASE_URL, code, PUBLIC_CLIENT_ID, REDIRECT_URI, null, pkce.codeVerifier);
  check(tokenRes, { 'oauth public: 200': (r) => r.status === 200 });
}

function scenarioOAuthConfidential() {
  const user = randomItem(userPool);
  const tokens = loginUser(BASE_URL, user.email, user.password);
  if (!tokens) return;

  const state = generateState();

  const authRes = authorizeGet(BASE_URL, tokens.accessToken, CONFIDENTIAL_CLIENT_ID, REDIRECT_URI, state);
  if (authRes.status !== 200) return;

  const approveRes = authorizeApprove(BASE_URL, tokens.accessToken, CONFIDENTIAL_CLIENT_ID, REDIRECT_URI, state);
  if (approveRes.status !== 200) return;

  const code = approveRes.json().code;
  const tokenRes = exchangeCode(BASE_URL, code, CONFIDENTIAL_CLIENT_ID, REDIRECT_URI, CLIENT_SECRET);
  check(tokenRes, { 'oauth confidential: 200': (r) => r.status === 200 });
}
