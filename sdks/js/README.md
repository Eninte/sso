# SSO JavaScript/TypeScript SDK

SSO 单点登录服务的 JavaScript/TypeScript 客户端 SDK。

## 安装

```bash
npm install @sso/sdk
```

## 快速开始

```typescript
import { createSSOClient } from '@sso/sdk';

const client = createSSOClient('http://localhost:9090');

// 注册
await client.register('user@example.com', 'P@ssw0rd1');

// 登录（自动保存 Token）
await client.login('user@example.com', 'P@ssw0rd1');

// 获取用户信息（Token 过期自动刷新）
const info = await client.userInfo();
console.log(info.email);

// 登出
await client.revokeToken();
```

## 客户端配置

```typescript
import { createSSOClient } from '@sso/sdk';

// 基本用法
const client = createSSOClient('http://localhost:9090');

// 自定义超时（毫秒）
const client = createSSOClient('http://localhost:9090', { timeout: 10000 });

// 预设 Token
const client = createSSOClient('http://localhost:9090', {
  accessToken: 'eyJhbGciOi...',
  refreshToken: 'dGhpcyBpc...',
});

// 自定义 fetch
const client = createSSOClient('http://localhost:9090', {
  fetch: customFetch,
});
```

## API 方法

### 认证

| 方法 | 说明 | 需要 Token |
|------|------|-----------|
| `register(email, password)` | 注册 | 否 |
| `login(email, password)` | 登录 | 否 |
| `refreshToken()` | 刷新 Token | 否 |
| `exchangeCode(code, clientId, clientSecret, redirectUri, codeVerifier?)` | OAuth2 授权码换 Token | 否 |
| `revokeToken()` | 登出 | 否 |
| `forgotPassword(email)` | 发送重置邮件 | 否 |
| `resetPassword(token, userId, newPassword)` | 重置密码 | 否 |
| `verifyEmail(token, userId)` | 验证邮箱 | 否 |
| `sendVerificationEmail()` | 发送验证邮件 | 是 |

### 用户

| 方法 | 说明 | 需要 Token |
|------|------|-----------|
| `userInfo()` | 获取用户信息 | 是 |
| `changePassword(oldPassword, newPassword)` | 修改密码 | 是 |

### OAuth2

| 方法 | 说明 | 需要 Token |
|------|------|-----------|
| `authorize(clientId, redirectUri, scope, state)` | 获取授权码 | 是 |
| `authorizeWithPKCE(clientId, redirectUri, scope, state, codeChallenge)` | 获取授权码（PKCE） | 是 |
| `approveAuthorization(req)` | 批准授权 | 是 |

### MFA

| 方法 | 说明 | 需要 Token |
|------|------|-----------|
| `mfaSetup()` | 初始化 MFA | 是 |
| `mfaVerify(code)` | 验证并启用 MFA | 是 |
| `mfaDisable(code)` | 禁用 MFA | 是 |
| `mfaStatus()` | 获取 MFA 状态 | 是 |

### 管理员

| 方法 | 说明 | 需要 Token |
|------|------|-----------|
| `adminHealth()` | 健康检查 | 是 |
| `adminCleanup()` | 清理过期数据 | 是 |
| `listUsers(page, pageSize)` | 用户列表 | 是 |
| `getUser(userId)` | 用户详情 | 是 |
| `disableUser(userId)` | 禁用用户 | 是 |
| `enableUser(userId)` | 启用用户 | 是 |

### OIDC

| 方法 | 说明 | 需要 Token |
|------|------|-----------|
| `discovery()` | OIDC Discovery 配置 | 否 |
| `jwks()` | JWKS 公钥 | 否 |

## 错误处理

```typescript
import { SSOError } from '@sso/sdk';

try {
  await client.login('user@example.com', 'wrong');
} catch (err) {
  if (err instanceof SSOError) {
    console.log(err.httpStatus);  // 401
    console.log(err.code);        // 'INVALID_CREDENTIALS'
    console.log(err.message);     // '邮箱或密码错误'

    if (err.isUnauthorized()) { /* ... */ }
    if (err.isConflict()) { /* ... */ }
    if (err.isRateLimited()) { /* ... */ }
  }
}
```

## Token 自动管理

SDK 在 `login()` 后自动管理 Token：

- 保存 access_token 和 refresh_token
- 每次请求前检查是否即将过期（提前 30 秒）
- 自动使用 refresh_token 刷新

## 目录结构

```
sdks/js/
├── src/
│   ├── index.ts        # 导出入口
│   ├── client.ts       # SSOClient 类（所有 API 方法）
│   ├── types.ts        # TypeScript 类型定义
│   ├── errors.ts       # SSOError 类、错误码常量
│   └── client.test.ts  # 测试（22 个用例）
├── package.json
├── tsconfig.json
└── README.md
```
