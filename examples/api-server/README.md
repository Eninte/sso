# API服务集成示例

## 简介

本示例演示如何在API服务中集成SSO服务，实现JWT Token验证。

## 功能

- JWT Token验证
- 用户信息提取
- 基于Token的访问控制

## 使用方法

### 1. 启动SSO服务

```bash
# 在SSO服务目录下
make run
```

### 2. 运行API服务

```bash
go run main.go
```

### 3. 测试API

```bash
# 公开端点 (不需要Token)
curl http://localhost:8081/api/public

# 受保护的端点 (需要有效的JWT Token)
curl -H "Authorization: Bearer YOUR_ACCESS_TOKEN" http://localhost:8081/api/protected
```

## 代码说明

### JWT验证中间件

```go
validator, _ := NewJWTValidator(SSOJWKSURL, "sso")

// 保护路由
mux.Handle("/api/protected", validator.Middleware(http.HandlerFunc(ProtectedHandler)))
```

### 获取用户信息

在处理器中，可以从请求上下文获取用户信息:

```go
func ProtectedHandler(w http.ResponseWriter, r *http.Request) {
    userID := r.Context().Value("userID")
    userEmail := r.Context().Value("userEmail")
    
    // 使用用户信息...
}
```

## 完整集成步骤

### 1. 获取公钥

从SSO服务的JWKS端点获取公钥:

```bash
curl http://localhost:9090/.well-known/jwks.json
```

### 2. 验证JWT

使用公钥验证JWT签名:

```go
token, err := jwt.Parse(tokenString, func(token *jwt.Token) (interface{}, error) {
    return publicKey, nil
})
```

### 3. 提取Claims

```go
claims := token.Claims.(jwt.MapClaims)
userID := claims["sub"]
email := claims["email"]
```

## 安全建议

1. **缓存公钥**: 缓存JWKS公钥，避免每次请求都获取
2. **验证过期时间**: 始终验证Token的过期时间
3. **验证签发者**: 验证Token的iss字段
4. **使用HTTPS**: 生产环境必须使用HTTPS
5. **定期轮换**: 定期轮换公钥

## 生产环境建议

使用成熟的JWT库，如:

- Go: `github.com/lestrrat-go/jwx`
- Node.js: `jose`
- Python: `PyJWT`
- Java: `jjwt`
