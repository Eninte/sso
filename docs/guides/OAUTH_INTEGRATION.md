# OAuth 2.0 / OIDC 集成指南

本文档说明如何将 SSO 服务作为 OAuth 2.0 / OpenID Connect 提供商集成到客户端应用中。

## 概述

SSO 服务实现以下协议：

| 协议 | 支持情况 |
|------|----------|
| OAuth 2.0 Authorization Code Flow | ✅ |
| OAuth 2.0 PKCE (RFC 7636) | ✅ S256 |
| OAuth 2.0 Token Refresh | ✅ |
| OAuth 2.0 Token Revocation (RFC 7009) | ✅ |
| OpenID Connect Discovery | ✅ |
| OpenID Connect Core | ✅ (ID Token via Access Token claims) |

**支持的授权类型**：
- `authorization_code` — 授权码流程（推荐）
- `refresh_token` — 刷新令牌

**不支持的授权类型**：
- `implicit` — 已废弃，不推荐
- `client_credentials` — 已定义但未实现
- `password` — 资源所有者密码凭证，不推荐

## 发现端点

### OpenID 配置

```
GET /.well-known/openid-configuration
```

返回完整的 OIDC 发现文档，包含所有端点、支持的算法和权限范围。

### JWKS 公钥

```
GET /.well-known/jwks.json
```

返回用于验证 Access Token 签名的 RSA 公钥（JWKS 格式）。

## 授权码流程（推荐）

适用于所有类型的客户端，特别是 Web 应用和移动应用。

### 步骤 1：发起授权请求

将用户重定向到授权端点：

```
GET /api/v1/authorize?
  client_id=<client_id>
  &redirect_uri=<redirect_uri>
  &response_type=code
  &scope=openid%20profile%20email
  &state=<random_state>
  &code_challenge=<code_challenge>
  &code_challenge_method=S256
```

**参数说明**：

| 参数 | 必填 | 说明 |
|------|------|------|
| `client_id` | 是 | 客户端 ID |
| `redirect_uri` | 是 | 回调地址（必须在客户端注册时配置） |
| `response_type` | 是 | 固定值 `code` |
| `scope` | 否 | 权限范围，空格分隔，默认 `openid profile email` |
| `state` | 是 | CSRF 保护参数，最少 16 字符随机字符串 |
| `code_challenge` | 推荐 | PKCE 挑战码（S256 推荐） |
| `code_challenge_method` | 推荐 | PKCE 方法，推荐 `S256` |

**认证要求**：用户必须已登录（持有有效 JWT）。未登录时返回 `login_required` 错误。

**成功响应** `200 OK`：
```json
{
  "code": "auth_code_string",
  "state": "random_state"
}
```

> **注意**：当前实现直接返回授权码 JSON，而非 302 重定向。客户端需要自行处理重定向逻辑。

### 步骤 2：用户批准授权（可选）

如果应用需要用户显式批准授权范围：

```
POST /api/v1/authorize/approve
Authorization: Bearer <access_token>
Content-Type: application/json
```

**请求体**：
```json
{
  "client_id": "your-client-id",
  "redirect_uri": "https://your-app.com/callback",
  "scope": "openid profile email",
  "state": "random_state",
  "code_challenge": "base64url(SHA256(code_verifier))",
  "code_challenge_method": "S256"
}
```

### 步骤 3：用授权码交换 Token

```
POST /api/v1/token
Content-Type: application/json
```

**请求体**：
```json
{
  "grant_type": "authorization_code",
  "code": "auth_code_string",
  "client_id": "your-client-id",
  "redirect_uri": "https://your-app.com/callback",
  "client_secret": "your-client-secret",
  "code_verifier": "original_random_verifier"
}
```

**参数说明**：

| 参数 | 必填 | 说明 |
|------|------|------|
| `grant_type` | 是 | 固定值 `authorization_code` |
| `code` | 是 | 步骤 1 获取的授权码 |
| `client_id` | 是 | 客户端 ID |
| `redirect_uri` | 是 | 必须与步骤 1 中的完全一致 |
| `client_secret` | 条件 | 机密客户端必填，公开客户端可省略 |
| `code_verifier` | 条件 | 如果步骤 1 使用了 PKCE，则必填 |

**成功响应** `200 OK`：
```json
{
  "message": "success",
  "data": {
    "access_token": "eyJhbGciOiJSUzI1NiIs...",
    "refresh_token": "random_32_byte_string",
    "token_type": "Bearer",
    "expires_in": 900,
    "scope": "openid profile email"
  }
}
```

## PKCE 实现（公开客户端必须）

PKCE（Proof Key for Code Exchange）防止授权码拦截攻击，SPA 和移动应用必须使用。

### 生成 code_verifier

```
code_verifier = 43-128 字符的随机字符串（URL-safe Base64 编码）
```

### 生成 code_challenge

```
# S256 方法（推荐）
code_challenge = base64url(SHA256(code_verifier))

# plain 方法（不推荐，仅兼容不支持 S256 的客户端）
code_challenge = code_verifier
```

### Go 示例

```go
import (
    "crypto/rand"
    "crypto/sha256"
    "encoding/base64"
)

func generateCodeVerifier() (string, error) {
    b := make([]byte, 32)
    _, err := rand.Read(b)
    if err != nil {
        return "", err
    }
    return base64.RawURLEncoding.EncodeToString(b), nil
}

func generateCodeChallenge(verifier string) string {
    h := sha256.Sum256([]byte(verifier))
    return base64.RawURLEncoding.EncodeToString(h[:])
}
```

## Token 管理

### 刷新 Token

Access Token 过期后，使用 Refresh Token 获取新的 Token 对：

```
POST /api/v1/token
Content-Type: application/json
```

```json
{
  "grant_type": "refresh_token",
  "refresh_token": "your_refresh_token"
}
```

**成功响应**：与步骤 3 相同，返回新的 Access Token 和 Refresh Token。

> **注意**：旧的 Refresh Token 会被自动轮换（rotation），每次刷新都会返回新的 Refresh Token。

### 撤销 Token

主动撤销 Access Token 或 Refresh Token：

```
POST /api/v1/token/revoke
Content-Type: application/json
```

```json
{
  "token": "access_token_or_refresh_token"
}
```

**成功响应**：
```json
{
  "message": "Token已撤销",
  "data": null
}
```

### 登出所有设备

撤销用户的所有 Token：

```
POST /api/v1/logout-all
Authorization: Bearer <access_token>
```

## 权限范围（Scopes）

| Scope | 说明 | 包含的 Claims |
|-------|------|---------------|
| `openid` | OIDC 基础 scope | `sub` |
| `profile` | 用户基本信息 | `name`, `picture` |
| `email` | 用户邮箱 | `email`, `email_verified` |

**默认范围**：`openid profile email`

## 获取用户信息

使用 Access Token 调用 UserInfo 端点：

```
GET /api/v1/userinfo
Authorization: Bearer <access_token>
```

**成功响应**：
```json
{
  "sub": "user-uuid",
  "email": "user@example.com",
  "email_verified": true,
  "scope": ["openid", "profile", "email"],
  "created_at": "2024-01-01T00:00:00Z"
}
```

## JWT Access Token 验证

Access Token 使用 RS256 签名。客户端应自行验证：

1. 从 `/.well-known/jwks.json` 获取公钥
2. 验证 JWT 签名
3. 验证 `iss`（签发者）匹配 SSO Base URL
4. 验证 `aud`（受众）匹配客户端 ID
5. 验证 `exp`（过期时间）未过期

### Go 验证示例

```go
import (
    "github.com/golang-jwt/jwt/v5"
    "crypto/rsa"
)

func validateToken(tokenString string, publicKey *rsa.PublicKey, issuer string) (*jwt.Token, error) {
    return jwt.Parse(tokenString, func(t *jwt.Token) (interface{}, error) {
        if _, ok := t.Method.(*jwt.SigningMethodRSA); !ok {
            return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
        }
        return publicKey, nil
    },
        jwt.WithIssuer(issuer),
        jwt.WithValidMethods([]string{"RS256"}),
    )
}
```

## 客户端类型

### 公开客户端（Public Clients）

- SPA、移动应用、桌面应用
- **不能**安全存储 `client_secret`
- **必须**使用 PKCE
- 请求 Token 时不需要 `client_secret`

### 机密客户端（Confidential Clients）

- 后端服务器应用
- **可以**安全存储 `client_secret`
- **建议**使用 PKCE（防御纵深）
- 请求 Token 时必须提供 `client_secret`

## 错误处理

### OAuth 错误响应

```json
{
  "error": "invalid_client",
  "error_description": "无效的客户端"
}
```

### 常见错误码

| 错误 | 说明 | 解决方案 |
|------|------|----------|
| `invalid_client` | 客户端 ID 不存在 | 检查 client_id 是否正确 |
| `invalid_redirect_uri` | 回调地址不匹配 | 检查 redirect_uri 是否与注册的一致 |
| `invalid_grant_type` | 不支持的授权类型 | 使用 `authorization_code` 或 `refresh_token` |
| `invalid_code` | 授权码无效 | 授权码只能使用一次，请重新发起授权 |
| `code_expired` | 授权码已过期 | 授权码有效期短，请重新发起授权 |
| `code_used` | 授权码已被使用 | 授权码只能使用一次 |
| `invalid_code_verifier` | PKCE 验证器不匹配 | 确保 code_verifier 与 code_challenge 对应 |
| `invalid_token` | Token 无效或已过期 | 使用 refresh_token 刷新或重新登录 |
| `login_required` | 需要用户登录 | 重定向到登录页面 |

## 第三方登录（Social Login）

SSO 支持 Google 和 GitHub 第三方登录。

### 获取支持的提供商

```
GET /auth/providers
```

### 发起第三方登录

```
GET /auth/{provider}?redirect_uri=https://your-app.com/callback&state=random_state
```

> 响应为 307 重定向到第三方授权页面。

### 处理回调

```
GET /auth/{provider}/callback?code=<code>&state=<state>
```

**成功响应**：
```json
{
  "access_token": "eyJhbGciOiJSUzI1NiIs...",
  "refresh_token": "random_32_byte_string",
  "token_type": "Bearer",
  "expires_in": 900
}
```

## 快速集成检查清单

- [ ] 已在 SSO 注册客户端（获取 `client_id` 和 `client_secret`）
- [ ] 已配置回调地址白名单
- [ ] 实现了 PKCE（公开客户端必须）
- [ ] `state` 参数使用至少 16 字符随机字符串
- [ ] 验证回调中的 `state` 与发起时一致
- [ ] 安全存储 Refresh Token
- [ ] 实现了 Token 自动刷新逻辑
- [ ] 验证 JWT Access Token 签名
- [ ] 处理所有 OAuth 错误场景
- [ ] 实现了 Token 撤销/登出功能
