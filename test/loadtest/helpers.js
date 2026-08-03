// k6 辅助函数
import http from 'k6/http';
import { check, sleep } from 'k6';
import crypto from 'k6/crypto';
import encoding from 'k6/encoding';

// ============================================================================
// HTTP 请求封装
// ============================================================================

export function postJSON(url, body, token) {
  const headers = { 'Content-Type': 'application/json' };
  if (token) {
    headers['Authorization'] = `Bearer ${token}`;
  }
  return http.post(url, JSON.stringify(body), { headers });
}

export function getJSON(url, token) {
  const headers = { 'Content-Type': 'application/json' };
  if (token) {
    headers['Authorization'] = `Bearer ${token}`;
  }
  return http.get(url, { headers });
}

export function deleteJSON(url, token) {
  const headers = { 'Content-Type': 'application/json' };
  if (token) {
    headers['Authorization'] = `Bearer ${token}`;
  }
  return http.del(url, null, { headers });
}

// ============================================================================
// 认证辅助
// ============================================================================

export function registerUser(baseUrl, email, password) {
  const res = postJSON(`${baseUrl}/api/v1/register`, { email, password });
  check(res, {
    'register: status 201': (r) => r.status === 201,
  });
  if (res.status === 201) {
    const body = res.json();
    return body.data ? body.data.user_id : null;
  }
  return null;
}

export function loginUser(baseUrl, email, password) {
  const res = postJSON(`${baseUrl}/api/v1/login`, { email, password });
  check(res, {
    'login: status 200': (r) => r.status === 200,
  });
  if (res.status === 200) {
    const body = res.json();
    return {
      accessToken: body.access_token,
      refreshToken: body.refresh_token,
    };
  }
  return null;
}

export function refreshToken(baseUrl, refreshToken) {
  const res = postJSON(`${baseUrl}/api/v1/token`, {
    grant_type: 'refresh_token',
    refresh_token: refreshToken,
  });
  check(res, {
    'refresh: status 200': (r) => r.status === 200,
  });
  if (res.status === 200) {
    const body = res.json();
    return {
      accessToken: body.access_token,
      refreshToken: body.refresh_token,
    };
  }
  return null;
}

export function getUserInfo(baseUrl, token) {
  const res = getJSON(`${baseUrl}/api/v1/userinfo`, token);
  check(res, {
    'userinfo: status 200': (r) => r.status === 200,
  });
  return res;
}

// ============================================================================
// OAuth 辅助
// ============================================================================

export function generatePKCE() {
  const codeVerifier = generateRandomString(64);
  const codeChallenge = encodePKCE(codeVerifier);
  return { codeVerifier, codeChallenge };
}

export function encodePKCE(verifier) {
  const hash = crypto.sha256(verifier, 'binary');
  return encoding.b64encode(hash, 'rawstd');
}

export function generateState() {
  return generateRandomString(32);
}

export function authorizeGet(baseUrl, token, clientId, redirectUri, state, codeChallenge) {
  let url = `${baseUrl}/api/v1/authorize?response_type=code&client_id=${clientId}&redirect_uri=${encodeURIComponent(redirectUri)}&scope=openid+profile+email&state=${state}`;
  if (codeChallenge) {
    url += `&code_challenge=${codeChallenge}&code_challenge_method=S256`;
  }
  return getJSON(url, token);
}

export function authorizeApprove(baseUrl, token, clientId, redirectUri, state, codeChallenge) {
  const body = {
    client_id: clientId,
    redirect_uri: redirectUri,
    scope: 'openid profile email',
    state: state,
  };
  if (codeChallenge) {
    body.code_challenge = codeChallenge;
    body.code_challenge_method = 'S256';
  }
  return postJSON(`${baseUrl}/api/v1/authorize/approve`, body, token);
}

export function exchangeCode(baseUrl, code, clientId, redirectUri, clientSecret, codeVerifier) {
  const body = {
    grant_type: 'authorization_code',
    code: code,
    redirect_uri: redirectUri,
    client_id: clientId,
  };
  if (clientSecret) {
    body.client_secret = clientSecret;
  }
  if (codeVerifier) {
    body.code_verifier = codeVerifier;
  }
  return postJSON(`${baseUrl}/api/v1/token`, body);
}

// ============================================================================
// 工具函数
// ============================================================================

function generateRandomString(length) {
  const chars = 'ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789-._~';
  let result = '';
  for (let i = 0; i < length; i++) {
    result += chars.charAt(Math.floor(Math.random() * chars.length));
  }
  return result;
}

export function generateUniqueEmail(prefix, index) {
  return `${prefix}-${index}-${Date.now()}@example.com`;
}

export function randomItem(arr) {
  return arr[Math.floor(Math.random() * arr.length)];
}

export function sleepRandom(min, max) {
  sleep(min + Math.random() * (max - min));
}
