# 中间件指南

SSO 服务使用多层中间件架构，提供认证、安全保护、限流等功能。所有中间件定义在 `internal/middleware/` 包中。

## 中间件概览

| 中间件 | 文件 | 用途 |
|--------|------|------|
| Auth | `auth.go` | JWT Bearer Token 认证、角色检查、Basic Auth |
| CORS | `cors.go` | 跨域资源共享 |
| Language | `language.go` | 多语言检测与上下文注入 |
| Logging | `logging.go` | HTTP 请求日志 |
| RateLimit (内存) | `ratelimit.go` | 基于 Token Bucket 的内存限流 |
| RateLimit (分布式) | `ratelimit_distributed.go` | 基于 Redis 滑动窗口的分布式限流 |
| RequestID | `requestid.go` | 请求 ID 生成成与传递 |
| Security | `security.go` | 安全响应头（CSP、HSTS 等） |

## 1. Auth 中间件

### JWT Bearer Token 认证

从请求头提取 `Authorization: Bearer <token>`，验证 JWT 并将用户信息注入上下文。

**函数签名**：
```go
func AuthMiddleware(jwtSvc *crypto.JWTService) mux.MiddlewareFunc
func AuthMiddlewareWithStore(jwtSvc *crypto.JWTService, store store.Store) mux.MiddlewareFunc
func AuthMiddlewareWithCache(jwtSvc *crypto.JWTService, store store.Store, cache cache.Cache) mux.MiddlewareFunc
func AuthMiddlewareWithMetrics(jwtSvc *crypto.JWTService, store store.Store, cache cache.Cache, invalidTokenFunc func()) mux.MiddlewareFunc
```

**变体说明**：

| 变体 | 撤销检查 | 适用场景 |
|------|----------|----------|
| `AuthMiddleware` | 无 | 最轻量级，不检查 Token 撤销状态 |
| `AuthMiddlewareWithStore` | 每次查数据库 | 强一致性，但性能开销大 |
| `AuthMiddlewareWithCache` | 先查缓存，miss 回源数据库 | 推荐生产使用 |
| `AuthMiddlewareWithMetrics` | 同 Cache + 指标回调 | 需要监控的场景 |

**认证流程**：
1. 提取 `Authorization: Bearer <token>` 头
2. 检查 Token 撤销状态（根据变体不同，检查方式不同）
3. 通过 `jwtSvc.ValidateAccessToken()` 验证 JWT 签名和过期
4. 将用户信息注入请求上下文

### 上下文键

| 常量 | 类型 | 说明 |
|------|------|------|
| `UserIDKey` | string | 用户 UUID |
| `UserEmailKey` | string | 用户邮箱 |
| `UserScopesKey` | string | 权限范围（空格分隔） |
| `UserRoleKey` | string | 用户角色（user/admin） |
| `IsAdminKey` | string | 是否为管理员（布尔值字符串） |
| `RequestIDKey` | string | 请求 ID |

### 从上下文获取用户信息

```go
userID := middleware.GetUserID(ctx)
email := middleware.GetUserEmail(ctx)
scopes := middleware.GetUserScopes(ctx)
role := middleware.GetUserRole(ctx)
isAdmin := middleware.GetIsAdmin(ctx)
```

### 角色检查

```go
// 要求特定角色
RequireRole("admin", "moderator")(next)

// 要求管理员（便捷方法）
RequireAdmin()(next)
```

### Basic Auth

```go
func BasicAuth(username, password string) mux.MiddlewareFunc
```

- 使用 `subtle.ConstantTimeCompare` 防止时序攻击
- 如果用户名和密码为空，直接放行（不启用认证）
- 用于 `/metrics` 端点保护

## 2. CORS 中间件

### 配置

```go
type CORSConfig struct {
    AllowedOrigins   []string // 允许的源
    AllowedMethods   []string // 允许的 HTTP 方法
    AllowedHeaders   []string // 允许的请求头
    MaxAge           int      // 预检请求缓存时间（秒）
}
```

### 默认值

| 配置项 | 默认值 |
|--------|--------|
| AllowedOrigins | `["http://localhost:3000"]` |
| AllowedMethods | `["GET", "POST", "PUT", "DELETE", "OPTIONS"]` |
| AllowedHeaders | `["Content-Type", "Authorization", "X-Requested-With"]` |
| MaxAge | `86400`（24 小时） |

### 源匹配规则

| 模式 | 说明 | 示例 |
|------|------|------|
| 精确匹配 | 完全相等 | `https://example.com` |
| 通配符 `*` | 匹配所有源 | `*` |
| 子域名通配 | 匹配子域名 | `*.example.com` 匹配 `api.example.com` |

### 响应头

| 头 | 值 |
|---|-----|
| `Access-Control-Allow-Origin` | 匹配的源 |
| `Access-Control-Allow-Methods` | 允许的方法 |
| `Access-Control-Allow-Headers` | 允许的头 |
| `Access-Control-Max-Age` | 缓存时间 |
| `Access-Control-Allow-Credentials` | `true` |

### 预检请求

`OPTIONS` 请求立即返回 `204 No Content`，不执行后续中间件和处理函数。

## 3. Language 中间件

### 语言检测优先级

1. 查询参数 `?lang=<code>`（最高优先级）
2. `Accept-Language` 请求头
3. 默认 `zh-CN`

### 语言规范化

使用 `common.NormalizeLanguage()` 进行标准化：
- `zh`, `zh-CN`, `zh-TW`, `ZH` → `zh-CN`
- `en`, `en-US`, `en-GB`, `EN` → `en-US`
- 其他语言保持原样

### 从上下文获取语言

```go
lang := middleware.GetLanguageFromContext(ctx)
// 返回 "zh-CN"（默认）或检测到的语言代码
```

## 4. Logging 中间件

### 日志格式

使用 `log/slog` 结构化日志，每条请求记录：

| 字段 | 说明 |
|------|------|
| `method` | HTTP 方法 |
| `path` | 请求路径 |
| `status` | 响应状态码 |
| `duration` | 请求处理时间 |
| `remote_addr` | 客户端 IP |
| `user_agent` | 用户代理 |
| `request_id` | 请求 ID |

### 实现机制

包装 `http.ResponseWriter` 捕获实际写入的状态码，确保日志中的状态码与实际响应一致。

## 5. Rate Limit 中间件（内存版）

### 算法

Token Bucket（令牌桶），按客户端 IP 分桶。

### 分片设计

- 64 个分片（shards），每个分片独立 mutex
- 减少高并发下的锁竞争
- 客户端 IP 通过一致性哈希分配到固定分片

### 客户端识别

| 优先级 | 来源 | 说明 |
|--------|------|------|
| 1 | `X-Real-IP` 头 | 反向代理设置的真实 IP |
| 2 | `RemoteAddr` | 直接连接地址 |

> **安全注意**：不信任 `X-Forwarded-For`，防止 IP 伪造。

### 清理机制

- 后台 goroutine 每 `window * 2` 周期运行一次
- 清除超过 2 个窗口周期未活动的条目

### 配置

```go
func NewRateLimiter(limit int, window time.Duration, opts ...RateLimiterOption) *RateLimiter
```

| 参数 | 说明 |
|------|------|
| `limit` | 窗口内最大请求数 |
| `window` | 时间窗口 |
| `limit <= 0` | 禁用限流 |

### 选项

```go
func WithMetrics(metricFunc func()) RateLimiterOption  // 触发限流时的回调
```

### 响应

触发限流时：
- 返回 `429 Too Many Requests`
- 设置 `Retry-After` 头
- 返回 JSON 错误体

### 生命周期

```go
limiter := middleware.NewRateLimiter(100, time.Minute)
defer limiter.Stop()  // 服务关闭时停止清理 goroutine
```

## 6. Rate Limit 中间件（分布式版）

### 算法

Redis Sorted Set 滑动窗口。

### 实现

1. `ZRemRangeByScore` — 清除窗口外的记录
2. `ZCard` — 统计窗口内请求数
3. `ZAdd` — 添加当前请求记录

### 容错

**Fail-Open**：Redis 操作失败时允许请求通过，不影响正常业务。

### 响应头

| 头 | 说明 |
|---|------|
| `X-RateLimit-Limit` | 窗口内最大请求数 |
| `X-RateLimit-Remaining` | 剩余请求数 |
| `X-RateLimit-Reset` | 窗口重置时间戳 |

### Key 格式

```
{keyPrefix}:{clientIP}
```

### TTL

Key 过期时间为 `window * 2`，自动清理。

## 7. RequestID 中间件

### 行为

1. 检查请求头 `X-Request-ID`
2. 如果存在，复用该值
3. 如果不存在，生成 16 字符随机十六进制 ID
4. 将 ID 设置到响应头 `X-Request-ID`
5. 将 ID 注入请求上下文

### 生成方式

使用 `crypto/rand` 生成 8 字节随机值，转为 16 字符十六进制字符串。

### 从上下文获取

```go
requestID := middleware.GetRequestID(ctx)
```

## 8. Security Headers 中间件

### 响应头

| 头 | 值 | 目的 |
|---|-----|------|
| `X-Frame-Options` | `DENY` | 防止点击劫持 |
| `X-Content-Type-Options` | `nosniff` | 防止 MIME 嗅探 |
| `Strict-Transport-Security` | `max-age=31536000; includeSubDomains` | 强制 HTTPS（1 年） |
| `Referrer-Policy` | `strict-origin-when-cross-origin` | 控制 Referer 头 |
| `Permissions-Policy` | `geolocation=(), microphone=(), camera=()` | 禁用浏览器功能 |

### CSP（Content-Security-Policy）

每请求生成随机 nonce：

```
Content-Security-Policy: default-src 'self'; script-src 'self' 'nonce-{nonce}'; style-src 'self' 'nonce-{nonce}'; img-src 'self' data:; font-src 'self'; frame-ancestors 'none'
```

- 16 字节随机值，通过 `crypto/rand` 生成
- 存储在上下文中，供模板使用
- 防止 XSS 攻击

### 获取 CSP Nonce

```go
nonce := middleware.GetCSPNonce(ctx)
```

## 中间件应用顺序

在 `cmd/server/main.go` 中，中间件按以下顺序应用：

```
Router
├── Global Middleware (applied to all routes)
│   ├── Security Headers
│   ├── CORS
│   ├── Logging
│   └── RequestID
│
├── /metrics
│   └── BasicAuth (optional)
│
├── Public Routes (no auth)
│   ├── /health
│   ├── /.well-known/*
│   ├── /auth/*
│   └── /api/v1/register, /api/v1/login, /api/v1/token, etc.
│   └── RateLimit (IP-based)
│
├── Protected Routes (/api/v1/*)
│   ├── AuthMiddlewareWithCache
│   ├── RateLimit (per-user)
│   └── /api/v1/userinfo, /api/v1/mfa/*, /api/v1/authorize, etc.
│
└── Admin Routes (/api/v1/admin/*)
    ├── AuthMiddlewareWithCache
    ├── RequireAdmin()
    ├── RateLimit (per-user)
    └── /api/v1/admin/*
```

## 自定义中间件

### 添加新中间件

```go
func MyMiddleware(next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        // 前置处理
        next.ServeHTTP(w, r)
        // 后置处理
    })
}
```

### 在路由中应用

```go
r.Use(MyMiddleware)
```

### 子路由中间件

```go
protected := r.PathPrefix("/api/v1").Subrouter()
protected.Use(middleware.AuthMiddlewareWithCache(jwtSvc, store, cache))
protected.HandleFunc("/userinfo", handler)
```
