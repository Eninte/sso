// S9: 安全保护专项
// 验证系统在恶意流量下自我保护而不雪崩

import { check, group, sleep } from 'k6';
import http from 'k6/http';
import { generateUniqueEmail } from '../helpers.js';
import {
  BASE_URL,
  TEST_PASSWORD,
  MALICIOUS_POOL_SIZE,
} from '../config.js';

export const options = {
  stages: [
    { duration: '1m', target: 50 },
    { duration: '3m', target: 200 },
    { duration: '3m', target: 500 },
    { duration: '2m', target: 0 },
  ],
  thresholds: {
    http_req_failed: ['rate<0.5'],
  },
};

// 恶意账号池：从文件加载
// 格式: [{"email": "malicious-001@example.com", "password": "WrongPassword1!"}, ...]
const maliciousPool = JSON.parse(open(__ENV.MALICIOUS_POOL_FILE || 'data/malicious_users.json', 'r'));

// 有效用户：用于验证保护触发后正常请求仍可处理
const userPool = JSON.parse(open(__ENV.USER_POOL_FILE || 'data/users.json', 'r'));

export default function () {
  const scenario = Math.random();

  if (scenario < 0.30) {
    // 30% 错误密码高频登录
    attackWrongPassword();
  } else if (scenario < 0.50) {
    // 20% 同邮箱并发注册
    attackDuplicateEmail();
  } else if (scenario < 0.70) {
    // 20% 无效 Token 风暴
    attackInvalidToken();
  } else if (scenario < 0.85) {
    // 15% 无效授权码
    attackInvalidCode();
  } else {
    // 15% 正常请求（验证服务未雪崩）
    normalRequest();
  }

  sleep(0.1);
}

function attackWrongPassword() {
  const user = maliciousPool[Math.floor(Math.random() * maliciousPool.length)];
  const res = http.post(`${BASE_URL}/api/v1/login`, JSON.stringify({
    email: user.email,
    password: 'WrongPassword!',
  }), { headers: { 'Content-Type': 'application/json' } });
  check(res, {
    'wrong password: 401 or 403 or 429': (r) => r.status === 401 || r.status === 403 || r.status === 429,
  });
}

function attackDuplicateEmail() {
  const email = maliciousPool.length > 0
    ? maliciousPool[Math.floor(Math.random() * maliciousPool.length)].email
    : 'duplicate@example.com';
  const res = http.post(`${BASE_URL}/api/v1/register`, JSON.stringify({
    email: email,
    password: TEST_PASSWORD,
  }), { headers: { 'Content-Type': 'application/json' } });
  check(res, {
    'duplicate email: 409 or 429': (r) => r.status === 409 || r.status === 429,
  });
}

function attackInvalidToken() {
  const res = http.get(`${BASE_URL}/api/v1/userinfo`, {
    headers: { 'Authorization': 'Bearer invalid-token-' + Math.random().toString(36).substring(7) },
  });
  check(res, {
    'invalid token: 401': (r) => r.status === 401,
  });
}

function attackInvalidCode() {
  const res = http.post(`${BASE_URL}/api/v1/token`, JSON.stringify({
    grant_type: 'authorization_code',
    code: 'invalid-code-' + Math.random().toString(36).substring(7),
    redirect_uri: 'http://localhost:3000/callback',
    client_id: 'public-test-client',
  }), { headers: { 'Content-Type': 'application/json' } });
  check(res, {
    'invalid code: 400 or 401': (r) => r.status === 400 || r.status === 401,
  });
}

function normalRequest() {
  const user = userPool[Math.floor(Math.random() * userPool.length)];
  const res = http.post(`${BASE_URL}/api/v1/login`, JSON.stringify({
    email: user.email,
    password: user.password,
  }), { headers: { 'Content-Type': 'application/json' } });
  check(res, {
    'normal login: service not collapsed': (r) => r.status === 200 || r.status === 429,
  });
}
