// S4: Refresh Token 单接口
// 压测 POST /api/v1/token (grant_type=refresh_token)

import { check, group, sleep } from 'k6';
import { postJSON } from '../helpers.js';
import { BASE_URL } from '../config.js';

export const options = {
  stages: [
    { duration: '1m', target: 20 },
    { duration: '2m', target: 50 },
    { duration: '3m', target: 100 },
    { duration: '3m', target: 200 },
    { duration: '3m', target: 300 },
    { duration: '2m', target: 0 },
  ],
  thresholds: {
    http_req_duration: ['p(95)<500'],
    http_req_failed: ['rate<0.01'],
  },
};

// Refresh Token池：从文件加载
// 格式: [{"refresh_token": "..."}, ...]
const tokenPool = JSON.parse(open(__ENV.REFRESH_TOKEN_POOL_FILE || 'data/refresh_tokens.json', 'r'));

export default function () {
  const entry = tokenPool[Math.floor(Math.random() * tokenPool.length)];

  group('refresh_token', function () {
    const res = postJSON(`${BASE_URL}/api/v1/token`, {
      grant_type: 'refresh_token',
      refresh_token: entry.refresh_token,
    });

    check(res, {
      'refresh: status 200 or 401': (r) => r.status === 200 || r.status === 401,
      'refresh: success returns new tokens': (r) => {
        if (r.status === 200) {
          const body = r.json();
          return body.access_token && body.refresh_token;
        }
        return true;
      },
    });
  });

  sleep(0.3);
}
