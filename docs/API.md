# API 文档

SSO服务提供RESTful API接口，支持用户认证、OAuth 2.0授权和管理功能。

## 基本信息

- **Base URL**: `http://localhost:9090`
- **API版本**: `/api/v1`
- **数据格式**: JSON
- **字符编码**: UTF-8

## 认证方式

### Bearer Token

在请求头中添加：
```
Authorization: Bearer <access_token>
```

## 系统端点

### 健康检查

检查服务运行状态。

```
GET /health
```

**响应示例**:
```json
{
  "status": "ok",
  "service": "sso",
  "timestamp": "2024-01-15T10:30:00Z"
}
```

### Kubernetes 探针

为 k8s 暴露的探针端点，独立路由器注册，**绕过限流和 metrics 中间件**，避免探针流量干扰业务指标。

```
GET /healthz    # 存活探针（liveness）
GET /readyz     # 就绪探针（检查 DB 连通性）
```

**`/readyz` 响应示例**（DB 不可达时返回 503）:
```json
{"status":"ready","service":"sso"}
```

### Prometheus指标

获取服务监控指标（如果配置了`METRICS_USERNAME`和`METRICS_PASSWORD`，需要Basic Auth认证）。

```
GET /metrics
Authorization: Basic base64(username:password)
```

**认证**: 可选（配置`METRICS_USERNAME`和`METRICS_PASSWORD`后启用）

**响应**: Prometheus文本格式

---

## 初始化端点

首次启动且尚无管理员账户时，可通过初始化面板创建管理员与 OAuth 客户端。一旦存在管理员，`/init` 页面会返回 404。

### 初始化面板页面

```
GET /init
```

**响应**: HTML 页面（包含系统状态、创建管理员表单、创建客户端表单）

### 系统初始化状态

```
GET /api/v1/init/status
```

**成功响应** `200 OK`:
```json
{
  "initialized": false,
  "version": "dev",
  "buildTime": "unknown",
  "adminCount": 0,
  "clientCount": 0
}
```

### 创建管理员账户

```
POST /api/v1/init/admin
Content-Type: application/json
```

**请求参数**:

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| email | string | 是 | 管理员邮箱 |
| password | string | 是 | 管理员密码 |

### 创建 OAuth 客户端

```
POST /api/v1/init/client
Content-Type: application/json
```

**请求参数**:

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| client_id | string | 是 | 客户端ID |
| client_secret | string | 否 | 客户端密钥（公开客户端不传） |
| name | string | 是 | 客户端显示名称 |
| redirect_uris | string[] | 是 | 允许的回调地址 |
| public_client | bool | 否 | 是否公开客户端，默认 false |

---

## 验证码端点

### 获取图形验证码

注册、登录失败次数达到阈值（`CAPTCHA_FAIL_THRESHOLD`，默认 3 次）时触发验证码。前端通过 `X-Captcha-Id` 与 `X-Captcha-Answer` 头传递验证结果。

```
GET /api/v1/captcha
```

**成功响应** `200 OK`:
```json
{
  "captcha_id": "uuid-string",
  "image": "data:image/png;base64,..."
}
```

**请求头（提交验证码时）**:

| 请求头 | 说明 |
|--------|------|
| `X-Captcha-Id` | 获取验证码时返回的 captcha_id |
| `X-Captcha-Answer` | 用户输入的验证码答案 |

---

## 认证端点

### 用户注册

注册新用户账户。

```
POST /api/v1/register
Content-Type: application/json
```

**请求参数**:

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| email | string | 是 | 用户邮箱 |
| password | string | 是 | 密码（8-72位） |

**请求示例**:
```json
{
  "email": "user@example.com",
  "password": "Password123!"
}
```

**成功响应** `201 Created`:
```json
{
  "message": "注册成功",
  "data": {
    "user_id": "550e8400-e29b-41d4-a716-446655440000",
    "email": "user@example.com"
  }
}
```

**错误响应**:

| 状态码 | 说明 |
|--------|------|
| 400 | 请求格式错误或参数无效 |
| 409 | 邮箱已注册 |
| 500 | 服务器内部错误 |

---

### 用户登录

使用邮箱和密码登录。

```
POST /api/v1/login
Content-Type: application/json
```

**请求参数**:

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| email | string | 是 | 用户邮箱 |
| password | string | 是 | 密码 |

**请求示例**:
```json
{
  "email": "user@example.com",
  "password": "Password123!"
}
```

**成功响应** `200 OK`:
```json
{
  "access_token": "eyJhbGciOiJSUzI1NiIsInR5cCI6IkpXVCJ9...",
  "refresh_token": "dGhpcyBpcyBhIHJlZnJlc2ggdG9rZW4...",
  "token_type": "Bearer",
  "expires_in": 900,
  "scopes": ["openid", "profile", "email"]
}
```

**错误响应**:

| 状态码 | 说明 |
|--------|------|
| 400 | 请求格式错误 |
| 401 | 邮箱或密码错误 |
| 403 | 账户已锁定或已禁用 |
| 500 | 服务器内部错误 |

---

### Token操作

#### 刷新Token

使用Refresh Token获取新的Token对。

```
POST /api/v1/token
Content-Type: application/json
```

**请求参数**:

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| grant_type | string | 是 | 固定值: `refresh_token` |
| refresh_token | string | 是 | Refresh Token |

**请求示例**:
```json
{
  "grant_type": "refresh_token",
  "refresh_token": "dGhpcyBpcyBhIHJlZnJlc2ggdG9rZW4..."
}
```

**成功响应** `200 OK`:
```json
{
  "access_token": "eyJhbGciOiJSUzI1NiIsInR5cCI6IkpXVCJ9...",
  "refresh_token": "bmV3IHJlZnJlc2ggdG9rZW4...",
  "token_type": "Bearer",
  "expires_in": 900,
  "scopes": ["openid", "profile", "email"]
}
```

#### 交换授权码

使用OAuth 2.0授权码获取Token。

```
POST /api/v1/token
Content-Type: application/json
```

**请求参数**:

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| grant_type | string | 是 | 固定值: `authorization_code` |
| code | string | 是 | 授权码 |
| client_id | string | 是 | 客户端ID |
| redirect_uri | string | 是 | 回调地址 |
| client_secret | string | 否 | 客户端密钥（机密客户端必填） |
| code_verifier | string | 否 | PKCE验证器 |

**成功响应** `200 OK`:
```json
{
  "message": "success",
  "data": {
    "access_token": "eyJhbGciOiJSUzI1NiIsInR5cCI6IkpXVCJ9...",
    "refresh_token": "bmV3IHJlZnJlc2ggdG9rZW4...",
    "token_type": "Bearer",
    "expires_in": 900,
    "scope": "openid profile email"
  }
}
```

#### 撤销Token

撤销指定的Access Token或Refresh Token。

```
POST /api/v1/token/revoke
Content-Type: application/json
```

**请求参数**:

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| token | string | 是 | 要撤销的Token |

**请求示例**:
```json
{
  "token": "eyJhbGciOiJSUzI1NiIsInR5cCI6IkpXVCJ9..."
}
```

**成功响应** `200 OK`:
```json
{
  "message": "Token已撤销",
  "data": null
}
```

**错误响应**:

| 状态码 | 说明 |
|--------|------|
| 422 | 缺少token参数 |
| 500 | 服务器内部错误 |

---

## 用户端点

### 获取用户信息

获取当前认证用户的信息。需要认证。

```
GET /api/v1/userinfo
Authorization: Bearer <access_token>
```

**成功响应** `200 OK`:
```json
{
  "sub": "550e8400-e29b-41d4-a716-446655440000",
  "email": "user@example.com",
  "email_verified": true,
  "scope": ["openid", "profile", "email"],
  "created_at": "2024-01-01T00:00:00Z"
}
```

### 登出所有设备

撤销当前用户的所有Token，实现全部设备登出。需要认证。

```
POST /api/v1/logout-all
Authorization: Bearer <access_token>
```

**成功响应** `200 OK`:
```json
{
  "message": "已登出所有设备",
  "data": null
}
```

### 忘记密码

发送密码重置邮件。

```
POST /api/v1/forgot-password
Content-Type: application/json
```

**请求参数**:

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| email | string | 是 | 用户邮箱 |

**请求示例**:
```json
{
  "email": "user@example.com"
}
```

**成功响应** `200 OK`:
```json
{
  "message": "如果该邮箱已注册，重置邮件已发送",
  "data": null
}
```

> **安全说明**: 无论邮箱是否存在，均返回相同响应，防止用户枚举。

### 重置密码

使用重置令牌设置新密码。

```
POST /api/v1/reset-password
Content-Type: application/json
```

**请求参数**:

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| token | string | 是 | 重置令牌 |
| user_id | string | 是 | 用户ID |
| new_password | string | 是 | 新密码 |

**请求示例**:
```json
{
  "token": "reset-token-string",
  "user_id": "550e8400-e29b-41d4-a716-446655440000",
  "new_password": "NewPassword123!"
}
```

**成功响应** `200 OK`:
```json
{
  "message": "密码重置成功",
  "data": null
}
```

**错误响应**:

| 状态码 | 说明 |
|--------|------|
| 400 | 缺少参数或重置失败 |

### 修改密码

修改当前用户密码。需要认证。

```
POST /api/v1/change-password
Authorization: Bearer <access_token>
Content-Type: application/json
```

**请求参数**:

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| old_password | string | 是 | 当前密码 |
| new_password | string | 是 | 新密码 |

**请求示例**:
```json
{
  "old_password": "OldPassword123!",
  "new_password": "NewPassword123!"
}
```

**成功响应** `200 OK`:
```json
{
  "message": "密码修改成功",
  "data": null
}
```

### 邮箱验证

#### 发送验证邮件

发送邮箱验证邮件。需要认证。

```
POST /api/v1/verify-email/send
Authorization: Bearer <access_token>
```

**成功响应** `200 OK`:
```json
{
  "message": "验证邮件已发送",
  "data": null
}
```

#### 验证邮箱

使用验证令牌验证邮箱。

```
GET /api/v1/verify-email?token=<verification_token>&user_id=<user_id>
```

**查询参数**:

| 参数 | 类型 | 必填 | 说明 |
|------|------|------|------|
| token | string | 是 | 验证令牌 |
| user_id | string | 是 | 用户ID |

**成功响应** `200 OK`:
```json
{
  "message": "邮箱验证成功",
  "data": null
}
```

**错误响应**:

| 状态码 | 说明 |
|--------|------|
| 400 | 缺少参数或验证失败 |

---

## MFA端点

### 设置MFA

初始化MFA设置。需要认证。

```
POST /api/v1/mfa/setup
Authorization: Bearer <access_token>
```

**成功响应** `200 OK`:
```json
{
  "secret": "JBSWY3DPEHPK3PXP",
  "qr_code_url": "otpauth://totp/SSO:user@example.com?secret=JBSWY3DPEHPK3PXP&issuer=SSO",
  "manual_entry": "JBSWY3DPEHPK3PXP"
}
```

**错误响应**:

| 状态码 | 说明 |
|--------|------|
| 401 | 未认证 |
| 409 | MFA已启用 |
| 500 | 服务器内部错误 |

### 验证MFA

验证TOTP代码并启用MFA。需要认证。

```
POST /api/v1/mfa/verify
Authorization: Bearer <access_token>
Content-Type: application/json
```

**请求参数**:

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| code | string | 是 | TOTP代码 |

**请求示例**:
```json
{
  "code": "123456"
}
```

**成功响应** `200 OK`:
```json
{
  "message": "MFA已启用",
  "data": null
}
```

### 禁用MFA

禁用多因素认证。需要认证。

```
POST /api/v1/mfa/disable
Authorization: Bearer <access_token>
Content-Type: application/json
```

**请求参数**:

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| code | string | 是 | 当前TOTP代码 |

**请求示例**:
```json
{
  "code": "123456"
}
```

**成功响应** `200 OK`:
```json
{
  "message": "MFA已禁用",
  "data": null
}
```

**错误响应**:

| 状态码 | 说明 |
|--------|------|
| 400 | 缺少代码、MFA未启用或TOTP无效 |
| 401 | 未认证 |
| 500 | 服务器内部错误 |

### MFA状态

获取当前用户的MFA状态。需要认证。

```
GET /api/v1/mfa/status
Authorization: Bearer <access_token>
```

**成功响应** `200 OK`:
```json
{
  "enabled": true,
  "method": "totp"
}
```

---

## OAuth端点

### 授权端点

发起OAuth 2.0授权请求。需要认证。

```
GET /api/v1/authorize?client_id=<client_id>&redirect_uri=<redirect_uri>&response_type=code&scope=<scope>&state=<state>
Authorization: Bearer <access_token>
```

**查询参数**:

| 参数 | 必填 | 说明 |
|------|------|------|
| client_id | 是 | 客户端ID |
| redirect_uri | 是 | 回调地址 |
| response_type | 是 | 固定值: `code` |
| scope | 否 | 权限范围（空格分隔） |
| state | 是 | 状态参数（最小16字符，CSRF保护） |
| code_challenge | 否 | PKCE挑战 |
| code_challenge_method | 否 | PKCE方法 |

**成功响应** `200 OK`:
```json
{
  "code": "authorization_code",
  "state": "xyz"
}
```

**错误响应**:

| 状态码 | 说明 |
|--------|------|
| 400 | 请求参数错误或state无效 |
| 401 | 需要登录 |

### 批准授权

用户批准授权请求。需要认证。

```
POST /api/v1/authorize/approve
Authorization: Bearer <access_token>
Content-Type: application/json
```

**请求参数**:

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| client_id | string | 是 | 客户端ID |
| redirect_uri | string | 是 | 回调地址 |
| scope | string | 否 | 权限范围（空格分隔） |
| state | string | 是 | 状态参数 |
| code_challenge | string | 否 | PKCE挑战 |
| code_challenge_method | string | 否 | PKCE方法 |

**成功响应** `200 OK`:
```json
{
  "code": "authorization_code",
  "state": "xyz"
}
```

---

## OIDC Discovery端点

### OpenID配置

获取OpenID Connect配置信息。

```
GET /.well-known/openid-configuration
```

**成功响应** `200 OK`:
```json
{
  "issuer": "http://localhost:9090",
  "authorization_endpoint": "http://localhost:9090/api/v1/authorize",
  "token_endpoint": "http://localhost:9090/api/v1/token",
  "userinfo_endpoint": "http://localhost:9090/api/v1/userinfo",
  "jwks_uri": "http://localhost:9090/.well-known/jwks.json",
  "revocation_endpoint": "http://localhost:9090/api/v1/token/revoke",
  "response_types_supported": ["code"],
  "grant_types_supported": ["authorization_code", "refresh_token"],
  "subject_types_supported": ["public"],
  "id_token_signing_alg_values_supported": ["RS256"],
  "scopes_supported": ["openid", "profile", "email"],
  "token_endpoint_auth_methods_supported": ["client_secret_post", "none"],
  "code_challenge_methods_supported": ["S256"],
  "claims_supported": ["sub", "iss", "aud", "exp", "iat", "email", "email_verified", "name", "picture"]
}
```

### JWKS公钥

获取JWT签名公钥。

```
GET /.well-known/jwks.json
```

**成功响应** `200 OK`:
```json
{
  "keys": [
    {
      "kty": "RSA",
      "use": "sig",
      "kid": "sso-key-1",
      "n": "0vx7agoebGcQSuuPiLJXZpt...",
      "e": "AQAB"
    }
  ]
}
```

---

## 第三方登录端点

### 获取提供商列表

获取支持的第三方登录提供商。

```
GET /auth/providers
```

**成功响应** `200 OK`:
```json
[
  {
    "name": "google",
    "label": "Google",
    "icon": "https://www.google.com/favicon.ico"
  },
  {
    "name": "github",
    "label": "GitHub",
    "icon": "https://github.com/favicon.ico"
  }
]
```

### 第三方登录

重定向到第三方登录页面。

```
GET /auth/{provider}
```

**路径参数**:

| 参数 | 说明 |
|------|------|
| provider | 提供商名称（google/github） |

**查询参数**:

| 参数 | 必填 | 说明 |
|------|------|------|
| provider | 否 | 优先使用路径参数，其次查询参数 |
| redirect_uri | 否 | 回调地址，默认Referer头 |
| state | 否 | CSRF保护参数 |

**响应**: 307重定向到第三方授权页面

### 第三方回调

处理第三方登录回调。

```
GET /auth/{provider}/callback?code=<code>&state=<state>
```

**路径参数**:

| 参数 | 说明 |
|------|------|
| provider | 提供商名称 |

**查询参数**:

| 参数 | 必填 | 说明 |
|------|------|------|
| code | 是 | OAuth授权码 |
| state | 是 | CSRF验证 |
| provider | 否 | 优先使用路径参数 |
| redirect_uri | 否 | 回调地址 |

**成功响应** `200 OK`:
```json
{
  "access_token": "eyJhbGciOiJSUzI1NiIsInR5cCI6IkpXVCJ9...",
  "refresh_token": "bmV3IHJlZnJlc2ggdG9rZW4...",
  "token_type": "Bearer",
  "expires_in": 900
}
```

**错误响应**:

| 状态码 | 说明 |
|--------|------|
| 400 | 缺少code或state无效 |
| 500 | 登录失败 |

---

## 管理员端点

所有管理员端点需要认证且用户具有管理员权限。

### 系统健康检查

获取详细的系统健康信息。

```
GET /api/v1/admin/health
Authorization: Bearer <access_token>
```

**成功响应** `200 OK`:
```json
{
  "status": "healthy",
  "timestamp": "2024-01-15T10:30:00Z",
  "database": "connected",
  "version": "1.0.0"
}
```

### 清理过期数据

清理过期的Token和其他数据。

```
POST /api/v1/admin/cleanup
Authorization: Bearer <access_token>
```

**成功响应** `200 OK`:
```json
{
  "message": "清理完成",
  "data": null
}
```

### 用户列表

获取用户列表。

```
GET /api/v1/admin/users?page=1&pageSize=20
Authorization: Bearer <access_token>
```

**查询参数**:

| 参数 | 必填 | 默认值 | 说明 |
|------|------|--------|------|
| page | 否 | 1 | 页码，必须>0 |
| pageSize | 否 | 20 | 每页数量，必须>0且<=100 |

**成功响应** `200 OK`:
```json
{
  "users": [
    {
      "id": "550e8400-e29b-41d4-a716-446655440000",
      "email": "user@example.com",
      "email_verified": true,
      "mfa_enabled": false,
      "status": "active",
      "created_at": "2024-01-01T00:00:00Z",
      "updated_at": "2024-01-15T10:30:00Z"
    }
  ],
  "total": 100,
  "page": 1,
  "page_size": 20,
  "total_pages": 5
}
```

### 用户详情

获取指定用户信息。

```
GET /api/v1/admin/users/{id}
Authorization: Bearer <access_token>
```

**路径参数**:

| 参数 | 说明 |
|------|------|
| id | 用户ID |

**成功响应** `200 OK`:
```json
{
  "id": "550e8400-e29b-41d4-a716-446655440000",
  "email": "user@example.com",
  "email_verified": true,
  "mfa_enabled": false,
  "status": "active",
  "created_at": "2024-01-01T00:00:00Z",
  "updated_at": "2024-01-15T10:30:00Z"
}
```

**错误响应**:

| 状态码 | 说明 |
|--------|------|
| 400 | 缺少用户ID |
| 404 | 用户不存在 |

### 禁用用户

禁用指定用户账户。

```
POST /api/v1/admin/users/{id}/disable
Authorization: Bearer <access_token>
```

**路径参数**:

| 参数 | 说明 |
|------|------|
| id | 用户ID |

**请求体** (可选，也可通过路径参数指定):
```json
{
  "user_id": "550e8400-e29b-41d4-a716-446655440000"
}
```

**成功响应** `200 OK`:
```json
{
  "message": "用户已禁用",
  "data": null
}
```

### 启用用户

启用指定用户账户。

```
POST /api/v1/admin/users/{id}/enable
Authorization: Bearer <access_token>
```

**路径参数**:

| 参数 | 说明 |
|------|------|
| id | 用户ID |

**请求体** (可选，也可通过路径参数指定):
```json
{
  "user_id": "550e8400-e29b-41d4-a716-446655440000"
}
```

**成功响应** `200 OK`:
```json
{
  "message": "用户已启用",
  "data": null
}
```

### 删除用户

删除指定用户账户。

```
DELETE /api/v1/admin/users/{id}
Authorization: Bearer <access_token>
```

**路径参数**:

| 参数 | 说明 |
|------|------|
| id | 用户ID |

**成功响应** `200 OK`:
```json
{
  "message": "用户已删除",
  "data": null
}
```

**错误响应**:

| 状态码 | 说明 |
|--------|------|
| 400 | 缺少用户ID |
| 404 | 用户不存在 |

### 审计日志

获取审计日志列表。

```
GET /api/v1/admin/audit-logs?page=1&pageSize=20
Authorization: Bearer <access_token>
```

**查询参数**:

| 参数 | 必填 | 默认值 | 说明 |
|------|------|--------|------|
| page | 否 | 1 | 页码 |
| pageSize | 否 | 20 | 每页数量，最大100 |
| event_type | 否 | "" | 事件类型过滤 |

**成功响应** `200 OK`:
```json
{
  "logs": [
    {
      "id": "20260328153000-ABC123",
      "event_type": "user.login",
      "user_id": "550e8400-e29b-41d4-a716-446655440000",
      "ip_address": "192.168.1.1",
      "details": "{}",
      "success": true,
      "timestamp": "2026-03-28T15:30:00Z"
    }
  ],
  "total": 1000,
  "page": 1,
  "page_size": 20,
  "total_pages": 50
}
```

### 质量指标查询

获取代码质量仪表盘的指标数据。

```
GET /api/v1/admin/quality/api/metrics
Authorization: Bearer <access_token>
```

### 周度质量报告

获取代码质量仪表盘的周度报告。

```
GET /api/v1/admin/quality/api/report/weekly
Authorization: Bearer <access_token>
```

---

## 错误处理

所有错误响应遵循统一格式：

```json
{
  "error": "错误描述信息"
}
```

### HTTP状态码

| 状态码 | 说明 |
|--------|------|
| 200 | 请求成功 |
| 201 | 创建成功 |
| 400 | 请求参数错误 |
| 401 | 未认证或Token无效 |
| 403 | 无权限或账户状态异常 |
| 404 | 资源不存在 |
| 409 | 资源冲突（如邮箱已注册） |
| 422 | 请求参数缺失 |
| 429 | 请求过于频繁 |
| 500 | 服务器内部错误 |

### 业务错误码

| 错误信息 | 说明 |
|----------|------|
| 邮箱或密码错误 | 登录凭证无效 |
| 账户已锁定 | 登录失败次数过多 |
| 账户已被禁用 | 账户被管理员禁用 |
| 无效的refresh_token | Token无效或已过期 |
| 无效的授权码 | 授权码无效或已过期 |
| 邮箱格式无效 | 邮箱格式不符合要求 |
| 密码太短 | 密码长度不足8位 |

---

## 限流说明

API请求受多层限流保护（默认值见 `config.go`，可通过环境变量调整）：

| 限流层 | 作用域 | 默认限额 | 配置项 |
|--------|--------|----------|--------|
| 全局HTTP中间件 | 所有路由 | 100 请求/分钟 | `RATE_LIMIT_REQUESTS` / `RATE_LIMIT_WINDOW` |
| 敏感端点中间件 | register / login / forgot-password / reset-password | 全局限额的 1/10（默认 10 请求/分钟） | 自动派生 |
| 业务层登录限流 | login（per IP） | 硬编码 20 次/10 分钟 | 不受 `RATE_LIMIT_REQUESTS` 控制 |
| 业务层邮件限流 | 邮件发送（per address） | 硬编码 5 次/小时 | 不受 `RATE_LIMIT_REQUESTS` 控制 |

- 探针端点 `/healthz`、`/readyz` **绕过所有限流**
- `RATE_LIMIT_REQUESTS=0` 仅禁用前两层（全局 + 敏感端点），不影响业务层限流
- E2E 测试如需完全禁用业务层限流，请使用 `scripts/run_e2e_no_ratelimit.sh`

超限返回 `429 Too Many Requests`。
