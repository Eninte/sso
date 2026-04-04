// S5: UserInfo 高频读取
// 压测 GET /api/v1/userinfo

import { check, group, sleep } from 'k6';
import { getJSON } from '../helpers.js';
import { BASE_URL } from '../config.js';

export const options = {
  stages: [
    { duration: '1m', target: 100 },
    { duration: '2m', target: 300 },
    { duration: '3m', target: 500 },
    { duration: '3m', target: 800 },
    { duration: '3m', target: 1000 },
    { duration: '2m', target: 0 },
  ],
  thresholds: {
    http_req_duration: ['p(95)<200'],
    http_req_failed: ['rate<0.001'],
  },
};

// Access Token池：从文件加载
// 格式: [{"access_token": "..."}, ...]
const tokenPool = JSON.parse(open(__ENV.ACCESS_TOKEN_POOL_FILE || 'data/access_tokens.json', 'r'));

export default function () {
  const entry = tokenPool[Math.floor(Math.random() * tokenPool.length)];

  group('userinfo', function () {
    const res = getJSON(`${BASE_URL}/api/v1/userinfo`, entry.access_token);

    check(res, {
      'userinfo: status 200': (r) => r.status === 200,
      'userinfo: returns sub': (r) => {
        if (r.status === 200) {
          const body = r.json();
          return body.sub !== undefined;
        }
        return false;
      },
    });
  });

  sleep(0.05);
}
