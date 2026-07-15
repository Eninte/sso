# 架构文档

本文档描述SSO服务的系统架构、设计决策和技术实现。

## 系统概述

SSO（Single Sign-On）服务是一个基于Go语言开发的认证授权服务，提供统一的用户认证、OAuth 2.0授权和OpenID Connect功能。

## 架构图

```
┌─────────────────────────────────────────────────────────────────┐
│                           客户端应用                              │
│  (Web App, Mobile App, SPA, 第三方应用)                          │
└─────────────────────────────────────────────────────────────────┘
                              │
                              │ HTTPS
                              ▼
┌─────────────────────────────────────────────────────────────────┐
│                         反向代理 (Nginx)                         │
│  - SSL终止                                                       │
│  - 负载均衡                                                      │
│  - 限流                                                          │
└─────────────────────────────────────────────────────────────────┘
                              │
                              ▼
┌─────────────────────────────────────────────────────────────────┐
│                         SSO 服务                                  │
│  ┌──────────────────────────────────────────────────────────┐   │
│  │                     中间件层                                │   │
│  │  - 安全头  - CORS  - 日志  - 认证  - 限流                   │   │
│  └──────────────────────────────────────────────────────────┘   │
│                              │                                    │
│  ┌──────────────────────────────────────────────────────────┐   │
│  │                     处理器层 (Handler)                      │   │
│  │  - 登录  - 注册  - Token  - OAuth  - 用户  - MFA  - 管理   │   │
│  └──────────────────────────────────────────────────────────┘   │
│                              │                                    │
│  ┌──────────────────────────────────────────────────────────┐   │
│  │                     服务层 (Service)                        │   │
│  │  - AuthService    - OAuthService    - UserService          │   │
│  │  - EmailService   - MFAService      - SocialLoginService   │   │
│  │  - AuditService   - AdminService    - MetricsService       │   │
│  └──────────────────────────────────────────────────────────┘   │
│                              │                                    │
│  ┌──────────────────────────────────────────────────────────┐   │
│  │                     存储层 (Store)                          │   │
│  │  - PostgreSQL Store  - Mock Store (测试)                    │   │
│  └──────────────────────────────────────────────────────────┘   │
│                              │                                    │
│  ┌──────────────────────────────────────────────────────────┐   │
│  │                     基础设施层                              │   │
│  │  - JWT (crypto)  - 密码哈希  - 验证器  - 缓存               │   │
│  └──────────────────────────────────────────────────────────┘   │
└─────────────────────────────────────────────────────────────────┘
                              │
          ┌───────────────────┼───────────────────┐
          ▼                   ▼                   ▼
   ┌──────────────┐   ┌──────────────┐   ┌──────────────┐
   │  PostgreSQL  │   │    Redis     │   │  SMTP/邮件   │
   │  (主数据库)   │   │  (缓存/会话)  │   │  (邮件服务)   │
   └──────────────┘   └──────────────┘   └──────────────┘
```

## 目录结构

```
sso/
├── cmd/                        # 应用入口
│   └── server/
│       └── main.go             # 主程序入口
│
├── internal/                   # 私有代码（不可外部导入）
│   ├── app/                    # 组合根（composition root）
│   │   ├── services.go         # 依赖装配：初始化所有服务和存储
│   │   ├── router.go           # 路由注册：中间件链 + 路由分组
│   │   └── server.go           # HTTP服务器生命周期管理
│   │
│   ├── audit/                  # 审计子系统（仅 service/audit.go 实现，非独立目录）
│   │
│   ├── cache/                  # 缓存层
│   │   ├── redis.go            # Redis客户端实现
│   │   ├── lru.go              # LRU淘汰策略（内存回退）
│   │   └── redis_test.go
│   │
│   ├── captcha/                # 验证码服务
│   │   └── captcha.go          # 验证码生成与验证
│   │
│   ├── common/                 # 公共工具
│   │   ├── language.go         # 语言检测
│   │   └── random.go           # 随机数生成
│   │
│   ├── config/                 # 配置管理
│   │   └── config.go           # 配置加载和验证
│   │
│   ├── crypto/                 # 加密工具
│   │   ├── jwt.go              # JWT服务（支持密钥轮换）
│   │   ├── password.go         # 密码哈希（bcrypt）
│   │   ├── keyloader.go        # 密钥加载
│   │   └── *_test.go
│   │
│   ├── errors/                 # 统一错误定义 + 多语言消息
│   │   ├── errors.go           # 错误常量和类型
│   │   ├── messages.go         # 多语言消息
│   │   └── locales/            # zh-CN.json, en-US.json
│   │
│   ├── handler/                # HTTP处理器
│   │   ├── register.go         # 注册处理器
│   │   ├── login.go            # 登录处理器
│   │   ├── token.go            # Token处理器
│   │   ├── authorize.go        # OAuth授权处理器
│   │   ├── userinfo.go         # 用户信息处理器
│   │   ├── user.go             # 用户管理处理器
│   │   ├── mfa.go              # MFA处理器
│   │   ├── social.go           # 第三方登录处理器
│   │   ├── admin.go            # 管理员处理器
│   │   ├── wellknown.go        # OIDC Discovery
│   │   ├── metrics.go          # 指标处理器
│   │   ├── init.go             # 初始化面板处理器
│   │   ├── setup.go            # 配置向导处理器
│   │   ├── captcha.go          # 验证码处理器
│   │   ├── helpers.go          # 辅助函数
│   │   ├── templates/          # 内嵌的初始化页面模板
│   │   └── *_test.go
│   │
│   ├── logging/                # 日志工具
│   │   ├── logger.go           # 日志初始化和辅助函数
│   │   └── sanitizer.go        # 日志脱敏（邮箱、Token、手机号）
│   │
│   ├── metrics/                # Prometheus指标
│   │   └── metrics.go          # 指标定义和收集
│   │
│   ├── middleware/             # HTTP中间件
│   │   ├── auth.go             # 认证中间件 + 上下文键定义
│   │   ├── cors.go             # CORS中间件
│   │   ├── logging.go          # 日志中间件
│   │   ├── security.go         # 安全头中间件（含CSP nonce）
│   │   ├── requestid.go        # 请求ID中间件
│   │   ├── ratelimit.go        # 限流中间件（分片锁优化）
│   │   ├── ratelimit_distributed.go # Redis分布式限流
│   │   ├── recover.go          # panic恢复中间件
│   │   ├── language.go         # 语言中间件
│   │   └── middleware_test.go
│   │
│   ├── model/                  # 数据模型
│   │   ├── model.go            # 用户/Token/请求等模型
│   │   ├── audit.go            # 审计事件模型（含事件类型常量）
│   │   ├── mfa.go              # MFA模型
│   │   └── key.go              # 密钥版本模型
│   │
│   ├── quality/                # 代码质量仪表盘
│   │   └── dashboard/          # 质量指标API和报告
│   │
│   ├── service/                # 业务逻辑层
│   │   ├── interfaces.go       # Service接口定义
│   │   ├── auth.go             # 认证服务
│   │   ├── auth_login.go       # 登录逻辑（含业务层限流）
│   │   ├── auth_token.go       # Token服务
│   │   ├── login_ratelimit.go  # 登录限流（Redis, per IP）
│   │   ├── email_ratelimit.go  # 邮件限流（per email）
│   │   ├── oauth.go            # OAuth服务
│   │   ├── user.go             # 用户服务
│   │   ├── email.go            # 邮件服务
│   │   ├── mfa.go              # MFA服务
│   │   ├── mfa_recovery.go     # MFA恢复码
│   │   ├── mfa_setup.go        # MFA设置流程
│   │   ├── social.go           # 第三方登录服务
│   │   ├── admin.go            # 管理员服务
│   │   ├── audit.go            # 审计服务
│   │   ├── keyrotation.go      # 密钥轮换服务
│   │   ├── init.go             # 初始化服务
│   │   ├── email/              # 邮件模板引擎
│   │   │   ├── engine.go
│   │   │   └── templates/      # base.html + verification/ + password_reset/
│   │   └── *_test.go
│   │
│   ├── store/                  # 数据存储层
│   │   ├── store.go            # Store接口定义
│   │   ├── postgres/           # PostgreSQL实现
│   │   │   ├── postgres.go
│   │   │   ├── user.go
│   │   │   ├── token.go
│   │   │   ├── client.go
│   │   │   ├── audit.go
│   │   │   ├── key.go
│   │   │   ├── mfa_recovery.go
│   │   │   ├── verification.go
│   │   │   ├── quality_metrics.go
│   │   │   └── *_test.go       # 集成测试（build tag: integration）
│   │   └── mock/               # Mock实现（单元测试）
│   │       └── mock.go
│   │
│   ├── testing/                # 测试基础设施
│   │   ├── coverage/           # 覆盖率分析工具
│   │   └── e2e/                # E2E 测试运行器和隔离工具
│   │
│   ├── util/                   # 工具模块
│   │   ├── auditutil/          # 审计日志工具
│   │   ├── handlerutil/        # Handler响应工具
│   │   ├── serviceutil/        # Service错误处理工具
│   │   ├── retryutil/          # 重试工具
│   │   ├── safego/             # 安全 goroutine（panic recovery）
│   │   └── testutil/           # 测试辅助工具（DB/Redis连接重试）
│   │
│   └── validator/              # 输入验证
│       ├── validator.go        # 验证函数
│       └── validator_test.go
│
├── test/                       # E2E测试
│   └── e2e/                    # 端到端测试（//go:build e2e）
│
├── migrations/                 # 数据库迁移（001-015，3位序号命名）
│
├── docker/                     # Docker配置
│   ├── Dockerfile
│   ├── Dockerfile.dev
│   ├── docker-compose.yml
│   ├── docker-compose.test.yml
│   └── docker-compose.truenas.yml
│
├── scripts/                    # 工具脚本
│   ├── generate-keys.sh
│   ├── prepare-e2e-test.sh     # E2E测试数据准备
│   ├── cleanup-e2e-test.sh     # E2E测试数据清理
│   ├── run_e2e_no_ratelimit.sh # E2E服务启动（处理限流）
│   ├── generate_secrets.sh     # 生产密钥生成
│   ├── check_production_env.sh # 生产环境检查
│   └── deploy-truenas.sh       # TrueNAS 部署
│
├── keys/                       # RSA密钥（脚本生成，不提交）
│
├── sdks/                       # SDK客户端（Go/JS/Kotlin/Python/Rust/Swift）
├── loadtest/                   # 压力测试（k6脚本）
└── docs/                       # 文档
```

## 分层架构

### 1. 处理器层 (Handler)

**职责**：
- 解析HTTP请求
- 调用服务层处理业务逻辑
- 格式化HTTP响应
- 错误处理和状态码映射

**设计原则**：
- 保持轻薄，不包含业务逻辑
- 使用依赖注入获取服务实例
- 统一错误响应格式

```go
type LoginHandler struct {
    authSvc service.AuthServiceInterface
}

func (h *LoginHandler) Handle(w http.ResponseWriter, r *http.Request) {
    // 1. 解析请求
    var req model.LoginRequest
    if err := decodeJSON(r, &req); err != nil {
        writeError(w, http.StatusBadRequest, "无效的请求格式")
        return
    }
    
    // 2. 调用服务
    resp, err := h.authSvc.Login(r.Context(), &req)
    if err != nil {
        // 错误处理
    }
    
    // 3. 返回响应
    writeJSON(w, http.StatusOK, resp)
}
```

### 2. 服务层 (Service)

**职责**：
- 实现业务逻辑
- 协调多个依赖（存储、缓存、外部服务）
- 事务管理
- 业务规则验证

**设计原则**：
- 接口驱动设计
- 依赖注入
- 可测试性

```go
// Service接口定义
type AuthServiceInterface interface {
    Register(ctx context.Context, req *model.RegisterRequest) (*model.User, error)
    Login(ctx context.Context, req *model.LoginRequest) (*model.LoginResponse, error)
    RefreshToken(ctx context.Context, refreshToken string) (*model.LoginResponse, error)
    Logout(ctx context.Context, accessToken string) error
    ValidateToken(ctx context.Context, accessToken string) (*crypto.AccessTokenClaims, error)
}

// Service实现
type AuthService struct {
    store       store.Store
    passwordSvc *crypto.PasswordService
    jwtSvc      *crypto.JWTService
    maxAttempts int
    lockoutDuration time.Duration
}

func (s *AuthService) Login(ctx context.Context, req *model.LoginRequest) (*model.LoginResponse, error) {
    // 业务逻辑实现
}
```

### 3. 存储层 (Store)

**职责**：
- 数据持久化
- 数据库操作抽象
- 查询优化

**设计原则**：
- 接口定义与实现分离
- 支持多种存储后端
- 便于Mock测试

```go
type Store interface {
    Create(ctx context.Context, user *model.User) error
    GetByID(ctx context.Context, id string) (*model.User, error)
    GetByEmail(ctx context.Context, email string) (*model.User, error)
    Update(ctx context.Context, user *model.User) error
    Delete(ctx context.Context, id string) error
    // ...
}
```

### 4. 中间件层 (Middleware)

**职责**：
- 横切关注点处理
- 请求/响应增强
- 链式处理

**中间件类型**：
- `SecurityHeaders` - 安全HTTP头（含 CSP nonce）
- `RequestID` - 请求追踪 ID（X-Request-ID）
- `CORS` - 跨域资源共享
- `Logger` - 请求日志（自动关联 request_id）
- `AuthMiddleware` - JWT认证
- `AdminMiddleware` - 管理员权限检查
- `Language` - 多语言支持

## 核心流程

### 用户注册流程

```
客户端                Handler              Service              Store
  │                    │                    │                    │
  │  POST /register    │                    │                    │
  │───────────────────>│                    │                    │
  │                    │  Register(req)     │                    │
  │                    │───────────────────>│                    │
  │                    │                    │  验证输入           │
  │                    │                    │  检查邮箱是否存在    │
  │                    │                    │───────────────────>│
  │                    │                    │                    │
  │                    │                    │  哈希密码           │
  │                    │                    │  创建用户           │
  │                    │                    │───────────────────>│
  │                    │                    │                    │
  │                    │                    │<───────────────────│
  │                    │  用户对象          │                    │
  │                    │<───────────────────│                    │
  │  201 Created       │                    │                    │
  │<───────────────────│                    │                    │
```

### 用户登录流程

```
客户端                Handler              Service              Store          JWT
  │                    │                    │                    │              │
  │  POST /login       │                    │                    │              │
  │───────────────────>│                    │                    │              │
  │                    │  Login(req)        │                    │              │
  │                    │───────────────────>│                    │              │
  │                    │                    │  获取用户           │              │
  │                    │                    │───────────────────>│              │
  │                    │                    │  用户数据           │              │
  │                    │                    │<───────────────────│              │
  │                    │                    │  检查账户状态        │              │
  │                    │                    │  验证密码           │              │
  │                    │                    │  生成Token          │              │
  │                    │                    │─────────────────────────────────>│
  │                    │                    │  Token对           │              │
  │                    │                    │<─────────────────────────────────│
  │                    │                    │  存储Token          │              │
  │                    │                    │───────────────────>│              │
  │                    │  Token响应         │                    │              │
  │                    │<───────────────────│                    │              │
  │  200 OK            │                    │                    │              │
  │<───────────────────│                    │                    │              │
```

### OAuth 2.0 授权码流程

```
客户端应用           SSO服务              用户
    │                  │                   │
    │  GET /authorize  │                   │
    │─────────────────>│                   │
    │  302 登录页面    │                   │
    │<─────────────────│                   │
    │                  │                   │
    │                  │  用户登录          │
    │                  │<──────────────────│
    │                  │                   │
    │                  │  显示授权页面      │
    │                  │──────────────────>│
    │                  │                   │
    │                  │  用户批准          │
    │                  │<──────────────────│
    │  302 redirect_uri?code=xxx           │
    │<─────────────────│                   │
    │                  │                   │
    │  POST /token     │                   │
    │  grant_type=authorization_code       │
    │  code=xxx        │                   │
    │─────────────────>│                   │
    │  Token响应       │                   │
    │<─────────────────│                   │
```

## 数据模型

### 用户模型 (User)

```go
type User struct {
    ID              string     `json:"id"`
    Email           string     `json:"email"`
    PasswordHash    string     `json:"-"`
    Status          string     `json:"status"`
    EmailVerified   bool       `json:"email_verified"`
    LoginAttempts   int        `json:"-"`
    LockedUntil     *time.Time `json:"-"`
    MFAEnabled      bool       `json:"mfa_enabled"`
    MFASecret       string     `json:"-"`
    CreatedAt       time.Time  `json:"created_at"`
    UpdatedAt       time.Time  `json:"updated_at"`
}
```

### Token模型 (Token)

```go
type Token struct {
    ID           string    `json:"id"`
    AccessToken  string    `json:"-"`
    RefreshToken string    `json:"-"`
    UserID       string    `json:"user_id"`
    ClientID     string    `json:"client_id"`
    Scopes       []string  `json:"scopes"`
    ExpiresAt    time.Time `json:"expires_at"`
    RevokedAt    *time.Time `json:"revoked_at"`
    CreatedAt    time.Time `json:"created_at"`
}
```

### OAuth客户端模型 (Client)

```go
type Client struct {
    ID           string   `json:"id"`
    ClientID     string   `json:"client_id"`
    ClientSecret string   `json:"-"`
    Name         string   `json:"name"`
    RedirectURIs []string `json:"redirect_uris"`
    GrantTypes   []string `json:"grant_types"`
    PublicClient bool     `json:"public_client"`
}
```

## 安全设计

### 认证安全

```
┌─────────────────────────────────────────────────────┐
│                    认证安全层                         │
├─────────────────────────────────────────────────────┤
│  密码安全                                            │
│  - bcrypt哈希 (cost=12-14)                          │
│  - 最小长度8位                                       │
│  - 强度验证                                          │
├─────────────────────────────────────────────────────┤
│  Token安全                                           │
│  - RS256签名                                         │
│  - Access Token 15分钟                              │
│  - Refresh Token 7天                                │
│  - Token轮换                                         │
│  - 即时撤销                                          │
├─────────────────────────────────────────────────────┤
│  账户安全                                            │
│  - 登录失败锁定                                      │
│  - MFA双因素认证                                     │
│  - 邮箱验证                                          │
└─────────────────────────────────────────────────────┘
```

### 传输安全

```
┌─────────────────────────────────────────────────────┐
│                    传输安全层                         │
├─────────────────────────────────────────────────────┤
│  HTTPS强制                                          │
│  - HSTS头                                           │
│  - TLS 1.2+                                         │
├─────────────────────────────────────────────────────┤
│  安全HTTP头                                         │
│  - Content-Security-Policy                          │
│  - X-Frame-Options                                  │
│  - X-Content-Type-Options                           │
│  - X-XSS-Protection                                 │
├─────────────────────────────────────────────────────┤
│  CORS配置                                           │
│  - 白名单验证                                        │
│  - 预检请求处理                                      │
└─────────────────────────────────────────────────────┘
```

## 性能设计

### 缓存策略

```go
// 缓存层次
1. 内存缓存 (应用层)
   - 热点数据缓存
   - 会话状态

2. Redis缓存 (分布式)
   - Token黑名单
   - 用户会话
   - 验证码

3. 数据库 (持久化)
   - 用户数据
   - Token记录
```

### 连接池配置

```go
// PostgreSQL连接池（默认值见 config.go）
db.SetMaxOpenConns(100)              // 最大打开连接数（DB_MAX_OPEN_CONNS）
db.SetMaxIdleConns(50)               // 最大空闲连接数（DB_MAX_IDLE_CONNS）
db.SetConnMaxLifetime(5*time.Minute) // 连接最大生命周期（DB_CONN_MAX_LIFETIME）
db.SetConnMaxIdleTime(1*time.Minute) // 连接最大空闲时间（DB_CONN_MAX_IDLE_TIME）

// Redis连接池
poolSize: 10                  // 连接池大小（REDIS_POOL_SIZE）
minIdleConns: 5               // 最小空闲连接（REDIS_MIN_IDLE_CONNS）
```

## 监控设计

### 指标收集

```go
// Prometheus指标
http_requests_total           // HTTP请求总数
http_request_duration_seconds // 请求延迟
auth_login_total             // 登录次数
auth_login_failed_total      // 登录失败次数
auth_register_total          // 注册次数
auth_token_refresh_total     // Token刷新次数
```

### 日志设计

```go
// 结构化日志 (slog)
// 自动关联 request_id
logger := logging.WithContext(ctx)
logger.Info("用户登录成功",
    "user_id", userID,
    "ip", clientIP,
    "duration", duration,
)

// 敏感信息自动脱敏
logging.LogAuth("login", userID, email, true, nil)
// email 自动脱敏为 "u***@example.com"

// 日志级别
- ERROR: 系统错误、安全事件
- WARN: 业务异常、性能警告
- INFO: 关键业务操作
- DEBUG: 调试信息（仅开发环境）
```

## 扩展性设计

### 水平扩展

```
                    负载均衡器
                        │
        ┌───────────────┼───────────────┐
        ▼               ▼               ▼
   SSO实例1        SSO实例2        SSO实例3
        │               │               │
        └───────────────┼───────────────┘
                        │
              ┌─────────┴─────────┐
              ▼                   ▼
         PostgreSQL            Redis
          (主从)              (集群)
```

### 插件化设计

```go
// 存储层可插拔
type Store interface {
    // 接口定义
}

// 实现不同存储后端
- PostgreSQL Store
- MySQL Store (扩展)
- MongoDB Store (扩展)

// 第三方登录可插拔
type SocialProvider interface {
    GetAuthURL(state string) string
    ExchangeCode(code string) (*UserInfo, error)
}

// 实现不同提供商
- Google Provider
- GitHub Provider
- 微信 Provider (扩展)
- 企业微信 Provider (扩展)
```

## 技术选型理由

| 组件 | 选型 | 理由 |
|------|------|------|
| 语言 | Go | 高并发、性能好、部署简单 |
| 路由 | gorilla/mux | 成熟稳定、功能完善 |
| 数据库 | PostgreSQL | 可靠、功能丰富、开源 |
| 缓存 | Redis | 高性能、支持多种数据结构 |
| JWT | golang-jwt | 社区标准、安全可靠 |
| 测试 | testify | 功能强大、社区广泛使用 |
| 部署 | Docker | 环境一致性、易于部署 |
