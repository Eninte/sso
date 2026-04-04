// S1: 公开读接口基线
// 压测 /health, /.well-known/openid-configuration, /.well-known/jwks.json

import { check, group, sleep } from 'k6';
import http from 'k6/http';
import { BASE_URL } from '../config.js';

export const options = {
  stages: [
    { duration: '1m', target: 50 },
    { duration: '2m', target: 200 },
    { duration: '2m', target: 500 },
    { duration: '3m', target: 1000 },
    { duration: '2m', target: 1500 },
    { duration: '2m', target: 2000 },
    { duration: '1m', target: 0 },
  ],
  thresholds: {
    http_req_duration: ['p(95)<200'],
    http_req_failed: ['rate<0.001'],
  },
};

export default function () {
  group('health', function () {
    const res = http.get(`${BASE_URL}/health`);
    check(res, {
      'health: status 200': (r) => r.status === 200,
      'health: body ok': (r) => r.json('status') === 'ok',
    });
  });

  group('openid-configuration', function () {
    const res = http.get(`${BASE_URL}/.well-known/openid-configuration`);
    check(res, {
      'oidc config: status 200': (r) => r.status === 200,
      'oidc config: has issuer': (r) => r.json('issuer') !== undefined,
    });
  });

  group('jwks', function () {
    const res = http.get(`${BASE_URL}/.well-known/jwks.json`);
    check(res, {
      'jwks: status 200': (r) => r.status === 200,
      'jwks: has keys': (r) => r.json('keys') !== undefined,
    });
  });

  sleep(0.1);
}
