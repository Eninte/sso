// S10: 突刺与恢复
// 验证峰值冲击与恢复能力

import { check, group, sleep } from 'k6';
import http from 'k6/http';
import { BASE_URL } from '../config.js';

const SPIKE_BASE = parseInt(__ENV.SPIKE_BASE_VUS || '50', 10);
const SPIKE_MULTIPLIER = parseInt(__ENV.SPIKE_MULTIPLIER || '5', 10);

export const options = {
  stages: [
    // 常态
    { duration: '2m', target: SPIKE_BASE },
    // 突刺
    { duration: '30s', target: SPIKE_BASE * SPIKE_MULTIPLIER },
    // 峰值保持
    { duration: '1m', target: SPIKE_BASE * SPIKE_MULTIPLIER },
    // 回落
    { duration: '30s', target: SPIKE_BASE },
    // 恢复观察
    { duration: '2m', target: SPIKE_BASE },
  ],
  thresholds: {
    http_req_duration: ['p(95)<2000'],
    http_req_failed: ['rate<0.1'],
  },
};

const tokenPool = JSON.parse(open(__ENV.ACCESS_TOKEN_POOL_FILE || 'data/access_tokens.json', 'r'));

export default function () {
  const entry = tokenPool[Math.floor(Math.random() * tokenPool.length)];

  group('userinfo_spike', function () {
    const res = http.get(`${BASE_URL}/api/v1/userinfo`, {
      headers: { 'Authorization': `Bearer ${entry.access_token}` },
    });
    check(res, {
      'userinfo: status ok': (r) => r.status === 200 || r.status === 429,
    });
  });

  sleep(0.05);
}
