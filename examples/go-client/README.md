# Go客户端集成示例

## 简介

本示例演示如何使用Go语言集成SSO服务，实现OAuth2授权码流程和PKCE支持。

## 功能

- OAuth2授权码流程
- PKCE (Proof Key for Code Exchange) 支持
- Token刷新
- 用户信息获取
- Token撤销

## 使用方法

### 1. 配置SSO服务

确保SSO服务正在运行:

```bash
# 在SSO服务目录下
make run
```

### 2. 配置客户端

修改 `main.go` 中的配置:

```go
const (
    SSOBaseURL      = "http://localhost:9090"
    ClientID        = "your-client-id"
    ClientSecret    = "your-client-secret"
    RedirectURI     = "http://localhost:3000/callback"
    Scopes          = "openid profile email"
)
```

### 3. 运行示例

```bash
go run main.go
```

### 4. 完成授权流程

1. 访问输出的授权URL
2. 登录并授权
3. 获取授权码
4. 使用授权码交换Token

## 代码说明

### 创建SSO客户端

```go
client := NewSSOClient(
    "http://localhost:9090",
    "your-client-id",
    "your-client-secret",
    "http://localhost:3000/callback",
)
```

### 获取授权URL (使用PKCE)

```go
authURL, codeVerifier, err := client.GetAuthorizationURL("state-123", true)
// 将codeVerifier保存起来，后续交换Token时使用
```

### 交换授权码获取Token

```go
token, err := client.ExchangeCode(ctx, authCode, codeVerifier)
```

### 获取用户信息

```go
userInfo, err := client.GetUserInfo(ctx, token.AccessToken)
fmt.Printf("用户ID: %s\n", userInfo.Sub)
fmt.Printf("邮箱: %s\n", userInfo.Email)
```

### 刷新Token

```go
newToken, err := client.RefreshToken(ctx, token.RefreshToken)
```

### 撤销Token (登出)

```go
err := client.RevokeToken(ctx, token.AccessToken)
```

## 安全建议

1. **使用PKCE**: 始终启用PKCE以增强安全性
2. **安全存储**: 不要在客户端存储Client Secret
3. **HTTPS**: 生产环境必须使用HTTPS
4. **State参数**: 使用随机生成的state参数防止CSRF攻击
5. **Token存储**: 使用安全的存储方式保存Token
