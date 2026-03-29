# SSO Go SDK

SSO 单点登录服务的 Go 客户端 SDK，提供类型安全的 API 调用和 Token 自动管理。

## 安装

```bash
go get github.com/your-org/sso/sdks/golang
```

## 快速开始

```go
package main

import (
    "context"
    "fmt"
    "log"

    sdk "github.com/your-org/sso/sdks/golang"
)

func main() {
    client := sdk.NewClient("http://localhost:9090")
    ctx := context.Background()

    // 注册
    resp, err := client.Register(ctx, "user@example.com", "P@ssw0rd1")
    if err != nil {
        log.Fatal(err)
    }
    fmt.Println("注册成功:", resp.Data.UserID)

    // 登录（自动保存 Token）
    tokens, err := client.Login(ctx, "user@example.com", "P@ssw0rd1")
    if err != nil {
        log.Fatal(err)
    }
    fmt.Println("登录成功, 过期时间:", tokens.ExpiresIn, "秒")

    // 获取用户信息（Token 过期自动刷新）
    info, err := client.UserInfo(ctx)
    if err != nil {
        log.Fatal(err)
    }
    fmt.Println("用户邮箱:", info.Email)

    // 登出
    client.RevokeToken(ctx)
}
```

## 客户端配置

```go
// 基本用法
client := sdk.NewClient("http://localhost:9090")

// 自定义超时
client := sdk.NewClient("http://localhost:9090", sdk.WithTimeout(10*time.Second))

// 预设 Token
client := sdk.NewClient("http://localhost:9090",
    sdk.WithAccessToken("eyJhbGciOi..."),
    sdk.WithRefreshToken("dGhpcyBpc..."),
)

// 自定义 HTTP 客户端
client := sdk.NewClient("http://localhost:9090",
    sdk.WithHTTPClient(&http.Client{Timeout: 5 * time.Second}),
)
```

## API 方法

### 认证

| 方法 | 说明 | 需要 Token |
|------|------|-----------|
| `Register(ctx, email, password)` | 注册新用户 | 否 |
| `Login(ctx, email, password)` | 登录 | 否 |
| `RefreshToken(ctx)` | 刷新 Token | 否（使用 Refresh Token） |
| `ExchangeCode(ctx, code, clientID, clientSecret, redirectURI, codeVerifier)` | OAuth2 授权码换 Token | 否 |
| `RevokeToken(ctx)` | 撤销 Token（登出） | 否 |
| `ForgotPassword(ctx, email)` | 发送密码重置邮件 | 否 |
| `ResetPassword(ctx, token, userID, newPassword)` | 重置密码 | 否 |
| `VerifyEmail(ctx, token, userID)` | 验证邮箱 | 否 |
| `SendVerificationEmail(ctx)` | 发送验证邮件 | 是 |

### 用户

| 方法 | 说明 | 需要 Token |
|------|------|-----------|
| `UserInfo(ctx)` | 获取当前用户信息 | 是 |
| `ChangePassword(ctx, oldPassword, newPassword)` | 修改密码 | 是 |

### OAuth2

| 方法 | 说明 | 需要 Token |
|------|------|-----------|
| `Authorize(ctx, clientID, redirectURI, scope, state)` | 获取授权码 | 是 |
| `AuthorizeWithPKCE(ctx, clientID, redirectURI, scope, state, codeChallenge)` | 获取授权码（带 PKCE） | 是 |
| `ApproveAuthorization(ctx, req)` | 批准授权 | 是 |

### MFA（多因素认证）

| 方法 | 说明 | 需要 Token |
|------|------|-----------|
| `MFASetup(ctx)` | 初始化 MFA 设置 | 是 |
| `MFAVerify(ctx, code)` | 验证 TOTP 码并启用 MFA | 是 |
| `MFADisable(ctx, code)` | 禁用 MFA | 是 |
| `MFAStatus(ctx)` | 获取 MFA 状态 | 是 |

### 管理员

| 方法 | 说明 | 需要 Token |
|------|------|-----------|
| `AdminHealth(ctx)` | 系统健康检查 | 是 |
| `AdminCleanup(ctx)` | 清理过期数据 | 是 |
| `ListUsers(ctx, page, pageSize)` | 获取用户列表 | 是 |
| `GetUser(ctx, userID)` | 获取用户详情 | 是 |
| `DisableUser(ctx, userID)` | 禁用用户 | 是 |
| `EnableUser(ctx, userID)` | 启用用户 | 是 |

### OIDC

| 方法 | 说明 | 需要 Token |
|------|------|-----------|
| `Discovery(ctx)` | 获取 OIDC Discovery 配置 | 否 |
| `JWKS(ctx)` | 获取 JWKS 公钥 | 否 |

## Token 自动管理

SDK 在 `Login()` 后自动保存 Token，并在后续请求中：

1. 检测 Token 是否即将过期（提前 30 秒）
2. 自动使用 Refresh Token 刷新
3. 将新 Token 应用到当前请求

```go
// 第一次登录
client.Login(ctx, "user@example.com", "P@ssw0rd1")

// 15 分钟后，Token 接近过期
// SDK 自动刷新，用户无感知
info, _ := client.UserInfo(ctx)

// 手动刷新（通常不需要）
tokens, _ := client.RefreshToken(ctx)
```

## 错误处理

所有 API 错误返回 `*sdk.Error` 类型：

```go
_, err := client.Login(ctx, "user@example.com", "wrong")
if err != nil {
    var ssoErr *sdk.Error
    if errors.As(err, &ssoErr) {
        fmt.Println("状态码:", ssoErr.HTTPStatus)
        fmt.Println("错误码:", ssoErr.Code)
        fmt.Println("消息:", ssoErr.Message)

        // 便捷判断方法
        if ssoErr.IsUnauthorized() {
            fmt.Println("认证失败")
        }
        if ssoErr.IsRateLimited() {
            fmt.Println("请求被限流")
        }
    }
}
```

### 错误码常量

```go
sdk.ErrCodeInvalidCredentials  // 邮箱或密码错误
sdk.ErrCodeAccountLocked       // 账户被锁定
sdk.ErrCodeAccountDisabled     // 账户被禁用
sdk.ErrCodeEmailExists         // 邮箱已存在
sdk.ErrCodeEmailInvalid        // 邮箱格式无效
sdk.ErrCodePasswordTooShort    // 密码太短
sdk.ErrCodeInvalidToken        // Token 无效
sdk.ErrCodeTokenExpired        // Token 过期
sdk.ErrCodeTooManyRequests     // 请求被限流
// ... 更多错误码见 errors.go
```

## 目录结构

```
sdks/golang/
├── client.go       # Client 结构体、配置选项、HTTP 请求封装
├── auth.go         # 注册、登录、Token 管理、密码重置
├── user.go         # 用户信息、修改密码
├── oauth.go        # OAuth2 授权码流程
├── mfa.go          # MFA 设置/验证/禁用
├── admin.go        # 管理员 API + OIDC Discovery
├── types.go        # 请求/响应类型定义
├── errors.go       # 错误类型和错误码常量
└── client_test.go  # 单元测试（27 个用例）
```
