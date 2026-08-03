// S3: 注册单接口
// 压测 POST /api/v1/register

import { check, group, sleep } from 'k6';
import { postJSON, generateUniqueEmail } from '../helpers.js';
import { BASE_URL, TEST_PASSWORD } from '../config.js';

export const options = {
  stages: [
    { duration: '1m', target: 5 },
    { duration: '2m', target: 20 },
    { duration: '3m', target: 50 },
    { duration: '3m', target: 100 },
    { duration: '2m', target: 0 },
  ],
  thresholds: {
    http_req_duration: ['p(95)<1000'],
    http_req_failed: ['rate<0.05'],
  },
};

// 使用 VU ID + 迭代次数生成唯一邮箱
export default function () {
  const email = generateUniqueEmail('loadtest', `${__VU}-${__ITER}`);

  group('register', function () {
    const res = postJSON(`${BASE_URL}/api/v1/register`, {
      email: email,
      password: TEST_PASSWORD,
    });

    check(res, {
      'register: status 201 or 409': (r) => r.status === 201 || r.status === 409,
      'register: success returns user_id': (r) => {
        if (r.status === 201) {
          const body = r.json();
          return body.data && body.data.user_id;
        }
        return true;
      },
    });
  });

  sleep(0.5);
}
