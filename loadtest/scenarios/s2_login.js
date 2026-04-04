// S2: 登录单接口
// 压测 POST /api/v1/login

import { check, group, sleep } from 'k6';
import { postJSON } from '../helpers.js';
import { BASE_URL, TEST_PASSWORD } from '../config.js';

export const options = {
  stages: [
    { duration: '1m', target: 10 },
    { duration: '2m', target: 50 },
    { duration: '3m', target: 100 },
    { duration: '3m', target: 200 },
    { duration: '3m', target: 300 },
    { duration: '2m', target: 0 },
  ],
  thresholds: {
    http_req_duration: ['p(95)<2000'],
    http_req_failed: ['rate<0.05'],
  },
};

// 用户池：从环境变量或文件加载
// 格式: [{"email": "user-0001@example.com", "password": "TestPassword123!"}, ...]
const userPool = JSON.parse(open(__ENV.USER_POOL_FILE || 'data/users.json', 'r'));

export default function () {
  const user = userPool[Math.floor(Math.random() * userPool.length)];

  group('login', function () {
    const res = postJSON(`${BASE_URL}/api/v1/login`, {
      email: user.email,
      password: user.password,
    });

    check(res, {
      'login: status 200 or 401': (r) => r.status === 200 || r.status === 401,
      'login: success returns tokens': (r) => {
        if (r.status === 200) {
          const body = r.json();
          return body.access_token && body.refresh_token;
        }
        return true;
      },
    });
  });

  sleep(0.5);
}
