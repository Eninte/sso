import { describe, it, expect, beforeAll, afterAll } from 'vitest';
import { createServer, IncomingMessage, ServerResponse } from 'node:http';
import { createSSOClient, SSOClient, SSOError } from '../src/index';

// ============================================================================
// 测试辅助：创建 mock HTTP 服务器
// ============================================================================

function mockServer(
  handler: (req: IncomingMessage, body: string) => { status: number; body: unknown },
): Promise<{ url: string; close: () => void }> {
  return new Promise((resolve) => {
    const server = createServer((req: IncomingMessage, res: ServerResponse) => {
      let body = '';
      req.on('data', (chunk: Buffer) => {
        body += chunk.toString();
      });
      req.on('end', () => {
        const result = handler(req, body);
        res.writeHead(result.status, { 'Content-Type': 'application/json' });
        res.end(JSON.stringify(result.body));
      });
    });

    server.listen(0, '127.0.0.1', () => {
      const addr = server.address();
      if (typeof addr === 'object' && addr) {
        resolve({
          url: `http://127.0.0.1:${addr.port}`,
          close: () => server.close(),
        });
      }
    });
  });
}

// ============================================================================
// 测试
// ============================================================================

describe('SSOClient', () => {
  describe('createSSOClient', () => {
    it('创建客户端实例', () => {
      const client = createSSOClient('http://localhost:9090');
      expect(client).toBeDefined();
      expect(client.getBaseURL()).toBe('http://localhost:9090');
    });

    it('去除尾部斜杠', () => {
      const client = createSSOClient('http://localhost:9090/');
      expect(client.getBaseURL()).toBe('http://localhost:9090');
    });
  });

  describe('register', () => {
    let server: { url: string; close: () => void };

    beforeAll(async () => {
      server = await mockServer((req) => {
        expect(req.method).toBe('POST');
        expect(req.url).toBe('/api/v1/register');
        return {
          status: 201,
          body: {
            message: '注册成功',
            data: { user_id: 'user-123', email: 'test@example.com' },
          },
        };
      });
    });

    afterAll(() => server.close());

    it('成功注册', async () => {
      const client = createSSOClient(server.url);
      const resp = await client.register('test@example.com', 'P@ssw0rd1');
      expect(resp.message).toBe('注册成功');
      expect(resp.data?.user_id).toBe('user-123');
    });
  });

  describe('register - 邮箱已存在', () => {
    let server: { url: string; close: () => void };

    beforeAll(async () => {
      server = await mockServer(() => ({
        status: 409,
        body: { code: 'EMAIL_EXISTS', message: '邮箱已存在' },
      }));
    });

    afterAll(() => server.close());

    it('抛出 SSOError', async () => {
      const client = createSSOClient(server.url);
      await expect(
        client.register('exist@example.com', 'P@ssw0rd1'),
      ).rejects.toThrow(SSOError);

      try {
        await client.register('exist@example.com', 'P@ssw0rd1');
      } catch (err) {
        expect(err).toBeInstanceOf(SSOError);
        const ssoErr = err as SSOError;
        expect(ssoErr.isConflict()).toBe(true);
        expect(ssoErr.code).toBe('EMAIL_EXISTS');
      }
    });
  });

  describe('login', () => {
    let server: { url: string; close: () => void };

    beforeAll(async () => {
      server = await mockServer((req) => {
        expect(req.method).toBe('POST');
        expect(req.url).toBe('/api/v1/login');
        return {
          status: 200,
          body: {
            access_token: 'access-123',
            refresh_token: 'refresh-456',
            token_type: 'Bearer',
            expires_in: 900,
          },
        };
      });
    });

    afterAll(() => server.close());

    it('成功登录并保存 Token', async () => {
      const client = createSSOClient(server.url);
      const resp = await client.login('test@example.com', 'P@ssw0rd1');

      expect(resp.access_token).toBe('access-123');
      expect(resp.refresh_token).toBe('refresh-456');
      expect(client.getAccessToken()).toBe('access-123');
    });
  });

  describe('login - 凭据错误', () => {
    let server: { url: string; close: () => void };

    beforeAll(async () => {
      server = await mockServer(() => ({
        status: 401,
        body: { code: 'INVALID_CREDENTIALS', message: '邮箱或密码错误' },
      }));
    });

    afterAll(() => server.close());

    it('抛出 401 错误', async () => {
      const client = createSSOClient(server.url);
      try {
        await client.login('test@example.com', 'wrong');
        expect.unreachable('should have thrown');
      } catch (err) {
        expect(err).toBeInstanceOf(SSOError);
        const ssoErr = err as SSOError;
        expect(ssoErr.isUnauthorized()).toBe(true);
      }
    });
  });

  describe('userInfo', () => {
    let server: { url: string; close: () => void };
    let receivedAuth: string | undefined;

    beforeAll(async () => {
      server = await mockServer((req) => {
        receivedAuth = req.headers.authorization;
        return {
          status: 200,
          body: { sub: 'user-123', email: 'test@example.com', email_verified: true },
        };
      });
    });

    afterAll(() => server.close());

    it('获取用户信息', async () => {
      const client = createSSOClient(server.url, { accessToken: 'access-123' });
      const info = await client.userInfo();

      expect(info.sub).toBe('user-123');
      expect(info.email).toBe('test@example.com');
      expect(receivedAuth).toBe('Bearer access-123');
    });
  });

  describe('userInfo - 无 Token', () => {
    it('抛出无 Token 错误', async () => {
      const client = createSSOClient('http://localhost:9090');
      await expect(client.userInfo()).rejects.toThrow('no access token');
    });
  });

  describe('revokeToken', () => {
    let server: { url: string; close: () => void };

    beforeAll(async () => {
      server = await mockServer(() => ({
        status: 200,
        body: { message: 'Token已撤销' },
      }));
    });

    afterAll(() => server.close());

    it('登出并清除 Token', async () => {
      const client = createSSOClient(server.url, { accessToken: 'access-123' });
      await client.revokeToken();

      expect(client.getAccessToken()).toBe('');
    });
  });

  describe('forgotPassword / resetPassword', () => {
    let server: { url: string; close: () => void };

    beforeAll(async () => {
      server = await mockServer((req) => {
        if (req.url === '/api/v1/forgot-password') {
          return { status: 200, body: { message: '重置邮件已发送' } };
        }
        return { status: 200, body: { message: '密码重置成功' } };
      });
    });

    afterAll(() => server.close());

    it('发送重置邮件', async () => {
      const client = createSSOClient(server.url);
      const resp = await client.forgotPassword('test@example.com');
      expect(resp.message).toContain('重置邮件');
    });

    it('重置密码', async () => {
      const client = createSSOClient(server.url);
      const resp = await client.resetPassword('token', 'user-1', 'NewP@ss1');
      expect(resp.message).toBe('密码重置成功');
    });
  });

  describe('MFA', () => {
    let server: { url: string; close: () => void };

    beforeAll(async () => {
      server = await mockServer((req) => {
        if (req.url === '/api/v1/mfa/setup') {
          return {
            status: 200,
            body: {
              secret: 'JBSWY3DPEHPK3PXP',
              qr_code_url: 'otpauth://totp/SSO:test@example.com',
              manual_entry: 'JBSWY3DPEHPK3PXP',
            },
          };
        }
        if (req.url === '/api/v1/mfa/status') {
          return { status: 200, body: { enabled: true } };
        }
        if (req.url === '/api/v1/mfa/verify') {
          return { status: 200, body: { message: 'MFA已启用' } };
        }
        return { status: 200, body: { message: 'MFA已禁用' } };
      });
    });

    afterAll(() => server.close());

    it('MFA 设置', async () => {
      const client = createSSOClient(server.url, { accessToken: 'tok' });
      const resp = await client.mfaSetup();
      expect(resp.secret).toBe('JBSWY3DPEHPK3PXP');
    });

    it('MFA 状态', async () => {
      const client = createSSOClient(server.url, { accessToken: 'tok' });
      const resp = await client.mfaStatus();
      expect(resp.enabled).toBe(true);
    });

    it('MFA 验证', async () => {
      const client = createSSOClient(server.url, { accessToken: 'tok' });
      const resp = await client.mfaVerify('123456');
      expect(resp.message).toBe('MFA已启用');
    });
  });

  describe('admin', () => {
    let server: { url: string; close: () => void };

    beforeAll(async () => {
      server = await mockServer((req) => {
        if (req.url === '/admin/health') {
          return {
            status: 200,
            body: { status: 'ok', database: 'connected', version: '1.0.0' },
          };
        }
        if (req.url?.startsWith('/admin/users?page=')) {
          return {
            status: 200,
            body: {
              users: [{ id: 'u1', email: 'a@b.com', status: 'active' }],
              total: 1,
              page: 1,
              page_size: 10,
            },
          };
        }
        return { status: 200, body: { message: '用户已禁用' } };
      });
    });

    afterAll(() => server.close());

    it('健康检查', async () => {
      const client = createSSOClient(server.url, { accessToken: 'admin' });
      const resp = await client.adminHealth();
      expect(resp.status).toBe('ok');
    });

    it('用户列表', async () => {
      const client = createSSOClient(server.url, { accessToken: 'admin' });
      const resp = await client.listUsers(1, 10);
      expect(resp.total).toBe(1);
    });

    it('禁用用户', async () => {
      const client = createSSOClient(server.url, { accessToken: 'admin' });
      const resp = await client.disableUser('u1');
      expect(resp.message).toBe('用户已禁用');
    });
  });

  describe('OIDC', () => {
    let server: { url: string; close: () => void };

    beforeAll(async () => {
      server = await mockServer((req) => {
        if (req.url === '/.well-known/openid-configuration') {
          return {
            status: 200,
            body: {
              issuer: 'http://test',
              grant_types_supported: ['authorization_code', 'refresh_token'],
              code_challenge_methods_supported: ['S256'],
            },
          };
        }
        return {
          status: 200,
          body: {
            keys: [{ kty: 'RSA', use: 'sig', kid: 'key-1', n: 'abc', e: 'def' }],
          },
        };
      });
    });

    afterAll(() => server.close());

    it('Discovery', async () => {
      const client = createSSOClient(server.url);
      const resp = await client.discovery();
      expect(resp.issuer).toBe('http://test');
      expect(resp.grant_types_supported).toContain('authorization_code');
    });

    it('JWKS', async () => {
      const client = createSSOClient(server.url);
      const resp = await client.jwks();
      expect(resp.keys).toHaveLength(1);
      expect(resp.keys[0].kty).toBe('RSA');
    });
  });

  describe('SSOError', () => {
    it('便捷判断方法', () => {
      const err = new SSOError(404, 'NOT_FOUND', 'not found', '{}');
      expect(err.isNotFound()).toBe(true);
      expect(err.isUnauthorized()).toBe(false);
      expect(err.toString()).toContain('NOT_FOUND');
    });

    it('所有状态码判断', () => {
      expect(new SSOError(401, '', '', '').isUnauthorized()).toBe(true);
      expect(new SSOError(403, '', '', '').isForbidden()).toBe(true);
      expect(new SSOError(409, '', '', '').isConflict()).toBe(true);
      expect(new SSOError(429, '', '', '').isRateLimited()).toBe(true);
    });
  });

  describe('Token 管理', () => {
    it('setTokens / clearTokens', () => {
      const client = createSSOClient('http://localhost:9090');
      client.setTokens('a', 'b', 900);
      expect(client.getAccessToken()).toBe('a');

      client.clearTokens();
      expect(client.getAccessToken()).toBe('');
    });
  });
});
