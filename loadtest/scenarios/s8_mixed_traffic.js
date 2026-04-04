// S8: 混合流量
// 模拟真实 SSO 业务流量配比

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
  generateUniqueEmail,
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

export const options = {
  stages: [
    { duration: '2m', target: 25 },
    { duration: '3m', target: 50 },
    { duration: '5m', target: 100 },
    { duration: '5m', target: 150 },
    { duration: '3m', target: 0 },
  ],
  thresholds: {
    http_req_duration: ['p(95)<1000'],
    http_req_failed: ['rate<0.01'],
  },
};

// 数据池：模块顶层读取一次，避免每次迭代重复 I/O
const userPool = JSON.parse(open(__ENV.USER_POOL_FILE || 'data/users.json', 'r'));
const adminTokenPool = JSON.parse(open(__ENV.ADMIN_TOKEN_POOL_FILE || 'data/admin_tokens.json', 'r'));
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
  } else if (r < 0.83) {
    scenarioRegister();
  } else if (r < 0.93) {
    scenarioOAuthPublic();
  } else if (r < 0.98) {
    scenarioOAuthConfidential();
  } else {
    scenarioAdmin();
  }

  sleep(0.2);
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
  check(res, { 'login: 200 or 401': (r) => r.status === 200 || r.status === 401 });
}

function scenarioRegister() {
  const email = generateUniqueEmail('mixed', `${__VU}-${__ITER}`);
  const res = http.post(`${BASE_URL}/api/v1/register`, JSON.stringify({
    email: email,
    password: TEST_PASSWORD,
  }), { headers: { 'Content-Type': 'application/json' } });
  check(res, { 'register: 201 or 409': (r) => r.status === 201 || r.status === 409 });
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
  check(tokenRes, { 'oauth public token: 200': (r) => r.status === 200 });
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
  check(tokenRes, { 'oauth confidential token: 200': (r) => r.status === 200 });
}

function scenarioAdmin() {
  const entry = randomItem(adminTokenPool);
  const res = http.get(`${BASE_URL}/api/v1/admin/health`, {
    headers: { 'Authorization': `Bearer ${entry.access_token}` },
  });
  check(res, { 'admin health: 200': (r) => r.status === 200 });
}
