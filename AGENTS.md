# SSO 服务 - AI 代理协作指南

> 本文件面向首次接触本项目的 AI 编码代理。阅读前应已了解：项目为 Go 1.26+ 单体 Web 服务，提供 OAuth 2.0 / OpenID Connect 认证能力。
> 所有操作请在 `/home/dev/SSO` 工作目录内进行，不要读取或提交 `.env.test` 等含真实凭据的文件。

## 1. 项目概述

SSO 服务是一个生产级单点登录（Single Sign-On）微服务，基于 Go 1.26+ 构建，支持：

- 用户注册、登录、登出、Token 刷新与撤销
- 基于 RS256 的 JWT Access/Refresh Token
- OAuth 2.0 授权码流程（Authorization Code + PKCE）与客户端管理
- OpenID Connect Discovery / JWKS 端点
- TOTP 多因素认证（MFA）与恢复码
- 社交登录（Google、GitHub）
- 账户锁定、限流、密码强度校验、安全 HTTP 头
- 审计日志、管理员接口、Prometheus 指标
- 异步邮件验证与密码重置
- 代码质量仪表盘与 k6 压力测试

主入口：`cmd/server/main.go` → 委托给 `internal/app.Run()` 完成配置加载、服务装配、路由装配、服务器启动。

## 2. 关键文件与技术栈

### 2.1 关键文件

| 文件/目录 | 作用 |
|----------|------|
| `go.mod` | Go 模块定义，模块名 `github.com/example/sso`，Go 版本 `1.26.1` |
| `Makefile` | 构建、测试、覆盖率、迁移、Docker、压力测试、代码分析等全量命令 |
| `.env.example` | 配置模板，所有配置项的默认值与说明 |
| `.env.test` | 测试环境配置（含真实凭据，**不可提交/分发**） |
| `.golangci.yml` | golangci-lint 配置，`enable-all: true` 并显式禁用部分规则 |
| `.github/workflows/ci.yml` | CI/CD 流水线：test、lint、security、e2e、build、docker |
| `docker/Dockerfile` | 多阶段 Docker 构建，基于 `golang:1.26.5-alpine` |
| `docker/docker-compose.yml` | 开发/部署 Compose，含 postgres、redis、sso 三服务 |
| `docker/entrypoint.sh` | 容器启动脚本：自动构造 `DATABASE_URL` 并执行 `migrate up` |
| `migrations/` | 数据库迁移脚本，共 018 个版本（3 位序号命名） |
| `keys/` | RSA 密钥对（脚本生成，Git 忽略） |

### 2.2 技术栈

| 层级 | 技术 | 版本/说明 |
|------|------|----------|
| 语言 | Go | 1.26.5（CI 固定版本） |
| HTTP 路由 | gorilla/mux | 1.8.1 |
| 数据库 | PostgreSQL | 15+（pgx/v5 驱动） |
| 缓存 | Redis | 7+（go-redis/v9），也支持内存回退 |
| JWT | golang-jwt/jwt/v5 | 5.3.1，RS256 签名 |
| 密码哈希 | bcrypt | `golang.org/x/crypto` |
| 测试 | testify + gotestsum | 含 `integration` / `e2e` build tag |
| 压测 | k6 | 场景脚本在 `loadtest/scenarios/` |
| 迁移 | golang-migrate | 需单独安装 |
| 部署 | Docker Compose / Kubernetes / TrueNAS / 裸机 |

## 3. 架构与代码组织

### 3.1 分层架构

```
客户端 → 中间件 → Handler → Service → Store → PostgreSQL
                      ↓         ↓         ↓
                    Model    Model    Cache/Redis
```

| 层级 | 包路径 | 职责 |
|------|--------|------|
| 组合根 | `internal/app/` | 配置加载、依赖装配、路由注册、服务器生命周期 |
| Handler | `internal/handler/` | HTTP 路由、请求解析、响应格式化（通过 `handlerutil`） |
| Service | `internal/service/` | 业务逻辑、事务、权限控制、接口定义在 `interfaces.go` |
| Store | `internal/store/` | 数据访问接口定义 + Postgres 实现 + Mock 实现 |
| Model | `internal/model/` | 数据结构与常量（JSON/DB tag） |
| 中间件 | `internal/middleware/` | 认证、限流、CORS、日志、安全头、recover 等 |
| 缓存 | `internal/cache/` | Redis 与 LRU 内存缓存 |
| 工具 | `internal/util/` | `auditutil`、`handlerutil`、`serviceutil`、`retryutil`、`safego`、`testutil` |

### 3.2 主要目录结构

```
SSO/
├── cmd/
│   ├── server/                    # 服务主入口
│   ├── api_docs_preview/          # API 文档本地预览（开发工具）
│   └── mockserver/                # Mock 服务器
├── internal/
│   ├── app/                       # 组合根
│   ├── audit/                     # 审计子系统（配置、合规、扫描、报告等）
│   ├── cache/                     # Redis + 内存缓存
│   ├── captcha/                   # 图形验证码
│   ├── common/                    # 语言检测、随机数
│   ├── config/                    # 环境变量配置加载与校验
│   ├── crypto/                    # JWT、bcrypt、密钥加载
│   ├── errors/                    # 统一错误类型与错误码
│   ├── handler/                   # HTTP 处理器与初始化页面模板
│   ├── logging/                   # 结构化日志与敏感信息脱敏
│   ├── metrics/                   # Prometheus 指标
│   ├── middleware/                # HTTP 中间件
│   ├── model/                     # 数据模型
│   ├── quality/                   # 代码质量仪表盘（dashboard 子包）
│   ├── service/                   # 业务逻辑层（含邮件模板引擎）
│   ├── store/                     # 存储接口 + Postgres 实现 + Mock 实现
│   ├── testing/                   # 测试基础设施（coverage / e2e 工具）
│   ├── util/                      # 通用工具模块
│   └── validator/                 # 输入验证
├── migrations/                    # 数据库迁移脚本
├── scripts/                       # 工具脚本（密钥生成、E2E 数据准备、部署等）
├── docker/                        # Docker 配置与入口脚本
├── sdks/                          # 多语言 SDK（Go/JS/Kotlin/Python/Rust/Swift）
├── loadtest/                      # k6 压力测试脚本与数据
├── test/e2e/                      # E2E 端到端测试
└── docs/                          # 项目文档
```

### 3.3 核心接口

- `store.Store`：组合 `UserStore` / `ClientStore` / `TokenStore` / `AuditLogStore` / `KeyStore` / `MFARecoveryCodeStore`
- `service.AuthServiceInterface` / `OAuthServiceInterface` / `MFAServiceInterface` / `UserServiceInterface` / `SocialLoginServiceInterface` 等
- 测试使用 `internal/store/mock/mock.go` 实现 `store.Store`

## 4. 环境配置

### 4.1 配置文件

| 文件 | 用途 | 安全性 |
|------|------|--------|
| `.env.example` | 配置模板，可安全分发 | 安全 |
| `.env` | 本地开发配置 | 不提交 |
| `.env.test` | 测试环境配置（含真实凭据） | **不可提交/分发** |

> ⚠️ 测试数据库和 Redis 已在远程主机运行，**禁止在本地安装 PostgreSQL 或 Redis**。直接使用 `.env.test` 中的连接信息。

### 4.2 配置加载机制

`internal/config/config.go` 通过 `Load()` 从环境变量读取配置，并遵循 12-Factor App 原则。`.env` 文件由 `godotenv` 加载，但**环境变量优先级高于文件**（`os.Getenv` 不会被覆盖）。

关键配置项：

| 变量 | 默认值 | 生产要求 |
|------|--------|----------|
| `SERVER_ENV` | `development` | `production` |
| `DB_PASSWORD` | 无 | **必填** |
| `DB_SSL_MODE` | `prefer` | **必须为 `require` 或更高** |
| `BCRYPT_COST` | 12 | **必须 >= 12** |
| `MFA_RECOVERY_HMAC_KEY` | 空 | **生产必须设置强密钥（>= 32 字节）** |
| `JWT_KEY_ENCRYPTION_KEY` | 空 | **启用密钥轮换（`KEY_ROTATION_ENABLED=true`）时生产必填**，64 位 hex（`openssl rand -hex 32`），用于 DB 中私钥的 AES-256-GCM 信封加密 |
| `CORS_ALLOWED_ORIGINS` | `http://localhost:3000` | **必须配置生产域名** |
| `JWT_PRIVATE_KEY_PATH` / `JWT_PUBLIC_KEY_PATH` | `./keys/*.pem` | 必填 |
| `SMTP_HOST` / `SMTP_USER` / `SMTP_PASSWORD` / `SMTP_FROM` | 邮件相关 | 邮件功能必填 |
| `LAN_DEPLOYMENT` | `false` | 仅内网部署可放宽生产校验 |

生产环境校验：

- `DB_SSL_MODE` 不能为 `disable`（`LAN_DEPLOYMENT=true` 除外）
- `BCRYPT_COST >= 12`
- `CORS_ALLOWED_ORIGINS` 不能包含 `localhost`，不能为默认值
- `JWT_ISSUER` 不能为默认 `sso`
- `SMTP_HOST` 不能为 `localhost`
- `MFA_RECOVERY_HMAC_KEY` 必须非空（`LAN_DEPLOYMENT=true` 除外）

### 4.3 配置向导

当 `.env` 不存在或配置加载失败时，服务启动配置向导（`internal/app/wizard.go`），在 `:9090` 提供 Web 初始化面板。配置正常时不会进入向导，因此不存在配置正常时暴露向导的风险。

## 5. 构建与运行命令

```bash
# 构建与运行
make build              # 构建到 ./bin/sso（自动注入版本信息）
make release            # 清理后构建发布版本
make run                # 直接运行服务
make dev                # 启动 Docker 依赖（postgres/redis）并运行服务
make clean              # 清理构建产物与覆盖率文件

# 密钥与迁移
make generate-keys      # 生成 RSA 密钥对
make migrate-up         # 执行数据库迁移（需 DATABASE_URL）
make migrate-down       # 回滚数据库迁移
make migrate-create NAME=xxx  # 创建新迁移文件

# 代码质量
make fmt                # go fmt ./...
make lint               # go vet + golangci-lint（提交前必须运行）
make test-security      # go vet + govulncheck

# Docker 相关
make docker-build       # 构建 Docker 镜像
make docker-up          # 启动 Docker Compose 全部服务
make docker-down        # 停止 Docker 服务
make docker-logs        # 查看 Docker 日志

# TrueNAS 部署
make deploy             # 部署到 TrueNAS（默认 192.168.1.3）
make deploy TRUENAS_HOST=192.168.1.3  # 指定主机
```

## 6. 测试策略与命令

### 6.1 测试分层

| 层级 | Build Tag | 命令 | 说明 |
|------|-----------|------|------|
| 单元测试 | 无 | `make test-unit` 或 `go test -short ./...` | 无外部依赖，使用 mock |
| 集成测试 | `//go:build integration` | `make test-integration` | 需要真实 PostgreSQL/Redis |
| E2E 测试 | `//go:build e2e` | `make test-e2e` | 需要运行中的 SSO 服务 |

- `internal/store/postgres/*_test.go` 使用 `integration` tag
- `test/e2e/*_test.go` 使用 `e2e` tag
- 其他 `internal/**/*_test.go` 为单元测试（无 tag）

### 6.2 常用测试命令

```bash
make test                           # 运行所有测试（含 -race，默认 120s 超时）
make test-verbose                   # 详细输出测试
make test-unit                      # 仅单元测试（短测试）
make test-integration               # 集成测试（-tags=integration）
make test-e2e                       # E2E 测试（需要服务运行中）
make test-e2e-prepare               # 准备 E2E 测试数据（启用自动验证触发器）
make test-e2e-cleanup               # 清理 E2E 测试数据（禁用触发器）
make test-e2e-full                  # 完整流程：准备 + 测试 + 清理
make test-coverage                  # 生成覆盖率报告并检查 80% 阈值
make test-e2e-coverage              # E2E 测试覆盖率（覆盖 internal/...）
make test-coverage-full             # 合并单元/集成/E2E 覆盖率
make test-report                    # 生成 JUnit XML 测试报告
make test-failed                    # 仅重跑失败的测试
make test-security                  # 运行 go vet + govulncheck
make test-error-handling            # 验证 Makefile 错误处理机制

# 基准测试
make bench                          # 运行所有基准测试
make bench-db                       # 数据库基准测试（需要 DATABASE_URL）
make bench-cache                    # 缓存基准测试
make bench-service                  # 服务基准测试
make bench-password                 # 密码服务基准测试
make bench-jwt                      # JWT 服务基准测试
make bench-report                   # 生成基准测试报告到 docs/reports/
```

### 6.3 覆盖率要求

- 整体覆盖率 >= 80%
- 覆盖率检查使用 Go 标准工具链：`go test -coverprofile` + `go tool cover -func/-html`；多 profile 合并由 `scripts/merge_coverage.go` 完成
- 阈值强制：Makefile `test-coverage` / `test-coverage-check` 目标提取 `go tool cover -func` 的 total 值，低于 80% 时退出非零码
- 覆盖率统计排除：`internal/app`（由 E2E 覆盖）、`internal/store/mock`（生成代码）、`internal/testing/`（测试基础设施）、`cmd/`（入口）、`sdks/`（客户端 SDK）

### 6.4 E2E 测试流程

```bash
# 1. 启动服务（禁用限流）
RATE_LIMIT_REQUESTS=0 make run &

# 2. 准备测试数据（启用自动验证触发器）
make test-e2e-prepare

# 3. 运行 E2E 测试
make test-e2e

# 4. 清理测试环境（禁用触发器）
make test-e2e-cleanup
```

E2E 测试数据准备机制：使用 PostgreSQL 触发器自动验证 `@example.com` 测试用户。触发器不污染生产代码，测试后可完全移除。详见 `docs/E2E_TESTING.md`。

### 6.5 真实 DB/Redis 集成测试连接

所有需要真实 PostgreSQL / Redis 的集成测试统一通过 `internal/util/testutil` 建立连接：

```bash
# 加载测试环境变量后运行
set -a && source .env.test && set +a
go test ./internal/handler/ -run "TestHandleSetupTestDB_RealDB|TestHandleSetupTestRedis_RealRedis" -v
```

关键环境变量：

| 变量 | 用途 | 默认值 |
|------|------|--------|
| `DATABASE_URL` / `DB_*` | PostgreSQL 连接 | 无 |
| `REDIS_TEST_ADDR` / `REDIS_PASSWORD` | Redis 连接 | 无 |
| `TEST_CONN_MAX_RETRIES` | 最大重试次数 | 3 |
| `TEST_CONN_BASE_DELAY` | 重试基础延迟 | 500ms |
| `TEST_CONN_TIMEOUT` | 单次测试超时 | 30s |

使用 `testutil.ConnectTestDB(t)` / `testutil.ConnectTestRedis(t)` 可自动重试、超时和 `t.Cleanup` 关闭。禁止在测试层之外给被测代码加重试。

## 7. 代码风格规范

### 7.1 导入规则

分组顺序：标准库 → 第三方 → 项目包。仅必要时使用别名，**禁止点导入**。

### 7.2 命名约定

| 类型 | 规则 | 示例 |
|------|------|------|
| 包名 | 小写无下划线 | `store`（非 `data_store`） |
| 接口 | 单方法以 `-er` 结尾 | `Reader`、`Writer` |
| 错误变量 | `Err` 前缀 | `ErrInvalidCredentials` |
| 导出标识符 | 大写开头 | `CreateUser` |
| 未导出标识符 | camelCase | `validateEmail` |

### 7.3 注释规范

- 包注释必须在 `package` 声明上方
- 导出函数必须有文档注释
- 使用 `// ====` 分隔符组织代码块
- 允许中文注释，**错误消息使用英文**

### 7.4 结构体标签

Model 结构体必须有 JSON 标签，使用 `omitempty` 处理可选字段。

### 7.5 测试规范

- 黑盒测试：优先使用 `package service_test`（非 `package service`）
- 测试框架：`testify/assert` + `testify/require`
- 风格：表驱动测试
- 命名：`TestFunctionName_场景`
- 每个测试独立创建 mock，禁止共享全局状态
- 精确断言：`assert.Equal(t, http.StatusBadRequest, code)`，避免 `assert.True(t, code >= 400)`
- **禁止用 `t.Skip()` 逃避未实现功能**；唯一允许的 skip 是真实 DB/Redis 环境未配置时
- **Mock 禁止原地修改共享对象**：`internal/store/mock` 的 map 存的是 `*model.*` 共享指针，getter 返回同一指针。写方法（如 `RotateRefreshToken`）不得原地修改已存入的对象——必须先浅拷贝、修改副本、再替换 map 中的指针（拷贝-替换），否则调用方锁外读取会与原地写产生数据竞争（CI `-race` 会检出）。此语义也与真实 DB 行更新一致：已取出的 struct 不应随后续写入而变化

### 7.6 Lint 配置

`.golangci.yml` 启用绝大部分 linter，但禁用了部分不适合本项目的规则（如 `exhaustruct`、`gochecknoglobals`、`wsl`、`lll`、`funlen`、`cyclop`、`nestif`、`gocritic`、`goconst`、`unused` 等）。提交前必须运行 `make lint`。

## 8. 统一错误处理

### 8.1 核心规则

- 使用 `apperrors.Err*` 预定义错误变量，**禁止自行创建错误类型**
- Store 层返回 `store.ErrNotFound`、`store.ErrDuplicateEmail` 等预定义错误
- Service 层用 `fmt.Errorf("上下文: %w", err)` 包装，或直接返回预定义错误
- Handler 层映射为 HTTP 状态码，使用 `handlerutil.WriteJSONError(w, err)` 统一响应

### 8.2 错误构造

```go
apperrors.New(code, message, httpStatus)          // 创建新错误
apperrors.Wrap(code, message, httpStatus, err)    // 包装错误
apperrors.Is(err, store.ErrNotFound)              // 判断错误类型
```

### 8.3 工具模块（必须复用）

| 工具模块 | 路径 | 用途 |
|----------|------|------|
| `serviceutil` | `internal/util/serviceutil/errors.go` | `HandleStoreError(err, notFoundErr)` 映射 Store 错误，不暴露内部细节；`WrapServiceError(operation, err)` 添加操作上下文 |
| `auditutil` | `internal/util/auditutil/logging.go` | `SafeAuditLog(ctx, auditSvc, event, userID, metadata)` 记录审计日志，失败回退 stderr；`CriticalAuditLog(...)` 用于关键操作 |
| `handlerutil` | `internal/util/handlerutil/response.go` | `WriteJSONError`、`WriteJSONSuccess`、`WriteValidationError` 统一响应格式 |

### 8.4 禁止事项

- ❌ 禁止 `errors.New("message")` 创建原始错误
- ❌ 禁止在响应中暴露内部错误详情
- ❌ 禁止忽略错误（除 `_ =` 标注的审计日志调用）
- ❌ Service 层直接处理 Store 错误（必须用 `serviceutil.HandleStoreError`）
- ❌ Service 层直接调用 `auditSvc.Log()`（必须用 `auditutil.SafeAuditLog`）
- ❌ Handler 层直接写 JSON 错误响应（必须用 `handlerutil.WriteJSONError`）

## 9. 安全机制

| 机制 | 规则 |
|------|------|
| JWT | Access Token 用 RS256，Refresh Token 用 32 字节随机字符串 |
| 密码 | bcrypt，测试 `BCRYPT_COST=10`，生产 >= 12 |
| 账户保护 | 默认 5 次登录失败锁定 30 分钟 |
| 限流 | 默认 100 请求/分钟；敏感端点（注册/登录/重置密码）为全局限流的 1/10 |
| MFA | TOTP 支持；恢复码 HMAC-SHA256 哈希，O(1) 查找，使用后立即失效；生产必须配置 `MFA_RECOVERY_HMAC_KEY` |
| 缓存 | UserInfo 5 分钟 TTL；密码/角色变更时失效；Singleflight 防击穿 |
| 传输 | 生产 `DB_SSL_MODE=require`；生产必须配置 CORS；安全 HTTP 头（CSP、HSTS、X-Frame-Options 等） |
| 邮件 | 异步发送，记录发送日志，禁止邮件中包含敏感信息 |
| 密钥 | RSA 至少 2048 位；生产检查密钥文件权限；支持密钥轮换 |

### 9.1 敏感配置安全

- 不得将 `.env.test` 凭据硬编码到代码或文档
- 生产环境启动时，若 `SERVER_ENV=production` 且 `MFA_RECOVERY_HMAC_KEY` 为空则拒绝启动（`LAN_DEPLOYMENT=true` 除外）

## 10. 路由与 API 概览

所有路由装配在 `internal/app/router.go`：

| 端点 | 方法 | 认证 | 说明 |
|------|------|------|------|
| `/health` | GET | 否 | 健康检查 |
| `/healthz` | GET | 否 | liveness 探针（绕过限流/ metrics） |
| `/readyz` | GET | 否 | readiness 探针（检查 DB 连通性） |
| `/metrics` | GET | Basic Auth | Prometheus 指标 |
| `/.well-known/openid-configuration` | GET | 否 | OIDC Discovery |
| `/.well-known/jwks.json` | GET | 否 | JWKS 公钥 |
| `/auth/providers` | GET | 否 | 支持的社交登录提供商 |
| `/auth/{provider}` | GET | 否 | 社交登录入口 |
| `/auth/{provider}/callback` | GET | 否 | 社交登录回调 |
| `/init` | GET | 否 | 初始化面板页面 |
| `/api/v1/init/status` | GET | 否 | 系统初始化状态 |
| `/api/v1/init/admin` | POST | 否 | 创建管理员账户 |
| `/api/v1/init/client` | POST | 否 | 创建 OAuth 客户端 |
| `/api/v1/register` | POST | 否 | 用户注册（敏感限流） |
| `/api/v1/login` | POST | 否 | 用户登录（敏感限流） |
| `/api/v1/token` | POST | 否 | Token 签发/刷新 |
| `/api/v1/token/revoke` | POST | 否 | Token 撤销 |
| `/api/v1/forgot-password` | POST | 否 | 忘记密码（敏感限流） |
| `/api/v1/reset-password` | POST | 否 | 重置密码（敏感限流） |
| `/api/v1/verify-email` | GET | 否 | 验证邮箱 |
| `/api/v1/captcha` | GET | 否 | 获取图形验证码 |
| `/api/v1/userinfo` | GET | 是 | 获取用户信息 |
| `/api/v1/verify-email/send` | POST | 是 | 发送验证邮件 |
| `/api/v1/change-password` | POST | 是 | 修改密码 |
| `/api/v1/logout-all` | POST | 是 | 登出所有设备 |
| `/api/v1/authorize` | GET | 是 | OAuth 授权端点 |
| `/api/v1/authorize/approve` | POST | 是 | 批准授权 |
| `/api/v1/mfa/*` | - | 是 | MFA 设置/验证/禁用/状态 |
| `/api/v1/admin/*` | - | 是 + admin | 管理员接口 |
| `/api/v1/admin/quality/api/*` | GET | 是 + admin | 代码质量指标 API |

## 11. 部署与运维

### 11.1 Docker Compose 部署

```bash
# 复制并编辑配置
cp .env.example .env
# 生成密钥
make generate-keys
# 启动依赖与 SSO 服务
make docker-up
# 查看日志
make docker-logs
```

容器启动时，`entrypoint.sh` 会自动构造 `DATABASE_URL` 并执行 `migrate up`（可通过 `AUTO_MIGRATE=false` 关闭）。

### 11.2 Kubernetes 部署

参考 `docs/DEPLOYMENT.md#kubernetes部署`：创建 Secret、部署 PostgreSQL/Redis、Deployment + Service + Ingress，使用 `/healthz` 和 `/readyz` 作为探针。

### 11.3 TrueNAS 部署

```bash
make deploy             # 默认部署到 192.168.1.3
```

详见 `docs/TRUENAS.md` 和 `scripts/deploy-truenas.sh`。

### 11.4 生产部署检查清单

生产部署前请核对：

- [ ] `.env` 中 `SERVER_ENV=production`
- [ ] `DB_PASSWORD` 为强密码，且 `DB_SSL_MODE=require`
- [ ] `BCRYPT_COST >= 12`
- [ ] `MFA_RECOVERY_HMAC_KEY` 已设置强随机密钥
- [ ] `CORS_ALLOWED_ORIGINS` 配置为生产域名，不含 `localhost`
- [ ] `JWT_ISSUER` 自定义，非默认 `sso`
- [ ] `SMTP_HOST` 非 `localhost`，邮件凭据已配置
- [ ] `METRICS_USERNAME` 与 `METRICS_PASSWORD` 已设置
- [ ] RSA 密钥已生成且权限安全（`chmod 600 private.pem`）
- [ ] 数据库迁移已执行
- [ ] 反向代理已配置 HTTPS 与安全头
- [ ] 详细清单见 `docs/PRODUCTION_DEPLOYMENT_CHECKLIST.md`

## 12. CI/CD 流水线

`.github/workflows/ci.yml` 定义以下任务：

1. **test**：在 Ubuntu 上运行 PostgreSQL/Redis service container，执行迁移后运行集成测试（`-tags=integration -race -p 1`），检查覆盖率 >= 80%
2. **lint**：运行 `golangci-lint`（从源码构建 v1.64.8，10 分钟超时）
3. **security**：运行 `gosec`（排除 G118/G201/G202/G706/G710）和 `govulncheck`
4. **e2e**：依赖 lint 成功后，启动服务，准备数据，运行 E2E 测试，清理数据
5. **build**：构建二进制并上传 artifact
6. **docker**：main 分支触发，构建并推送 Docker Hub 镜像（latest + sha）

> 注意：CI 中 gosec 的 G118/G201/G202/G706/G710 被排除，详见 workflow 注释。

## 13. 代码质量分析与压力测试

### 13.1 代码质量分析

```bash
make install-analysis-tools   # 安装 gocyclo、dupl 等工具
make analyze-all              # 运行完整分析（约 30 分钟）
make analyze-quick            # 快速分析：lint + 覆盖率
make analyze-report           # 生成详细分析报告
make analyze-security-scan    # gosec + govulncheck
make analyze-complexity       # 代码复杂度分析（gocyclo）
make analyze-duplication      # 重复代码检测（dupl）
make analyze-clean            # 清理分析报告
```

质量仪表盘：`internal/quality/dashboard/` 提供 `/api/v1/admin/quality/api/metrics` 和 `/api/v1/admin/quality/api/report/weekly` 接口。

### 13.2 压力测试（k6）

脚本目录：`loadtest/scenarios/`，数据目录：`loadtest/data/`，结果目录：`loadtest/results/`。

```bash
make loadtest-prepare          # 生成压测数据池
make loadtest-s1               # S1: 公开读接口基线
make loadtest-s2               # S2: 登录单接口（需要 users.json）
make loadtest-s3               # S3: 注册单接口
make loadtest-s4               # S4: Refresh Token
make loadtest-s5               # S5: UserInfo 高频读取
make loadtest-s6               # S6: OAuth 公共客户端完整流程
make loadtest-s7               # S7: OAuth 机密客户端完整流程
make loadtest-s8               # S8: 混合流量
make loadtest-s9               # S9: 安全保护专项
make loadtest-s10              # S10: 突刺与恢复
make loadtest-soak             # 长稳态测试
make loadtest-clean            # 清理压测数据
```

详细执行清单与报告模板见 `docs/PRESSURE_TESTING_RUNBOOK.md`、`docs/PRESSURE_TESTING_CHECKLIST.md`、`docs/PRESSURE_TESTING_REPORT_TEMPLATE.md`。

## 14. 邮件服务开发

### 14.1 邮件模板结构

所有模板基于 `internal/service/email/templates/base.html`，采用模板继承：

```
internal/service/email/templates/
├── base.html
├── verification/
│   ├── verification_zh.html
│   └── verification_en.html
└── password_reset/
    ├── password_reset_zh.html
    └── password_reset_en.html
```

### 14.2 添加新邮件类型

1. 在对应目录下创建中英文模板文件
2. 在 `internal/service/email/engine.go` 添加渲染方法
3. 在 `internal/service/email/email.go` 添加发送方法

### 14.3 测试工具

```bash
# 发送测试邮件
go run scripts/test_email.go -to user@example.com -type verification

# 渲染模板预览
go run scripts/render_email_template.go -type verification -lang zh -output /tmp/email.html
```

### 14.4 配色规范

- 主色调：蓝色 `#1e88e5`
- 按钮：蓝色背景 + 白色文字
- 安全提示：浅黄背景 + 橙色边框
- 响应式设计，支持深色模式
- 禁止在邮件中包含敏感信息，禁止硬编码 SMTP 凭据

详细邮件开发指南见 `docs/EMAIL_SERVICE.md`。

## 15. SDK 客户端

`sdks/` 目录包含多语言客户端 SDK：Go、JS、Kotlin、Python、Rust、Swift。使用 SDK 前请阅读 `sdks/README.md` 和 `sdks/SDK_ANALYSIS_REPORT.md`。服务端代码修改时，需同步检查 SDK 兼容性。

## 16. 开发工作流

### 16.1 新功能/修复流程

1. 创建分支
2. 编写测试（TDD）
3. 实现功能
4. 使用 `serviceutil` / `auditutil` / `handlerutil` 等工具模块
5. 运行 `make test`
6. 运行 `make lint`
7. 运行 `make test-security`
8. 提交

### 16.2 代码审查检查清单

- [ ] `make test` 通过
- [ ] `make lint` 通过
- [ ] `make test-security` 通过
- [ ] 覆盖率 >= 80%（核心业务 >= 90%）
- [ ] Service 层使用 `serviceutil` + `auditutil`
- [ ] Handler 层使用 `handlerutil`
- [ ] 新增/修改的 Model 结构体有 JSON 标签和 `omitempty`
- [ ] 错误处理遵循统一规范，不暴露内部细节
- [ ] 真实 DB/Redis 测试使用 `testutil.ConnectTestDB` / `ConnectTestRedis`
- [ ] 更新相关文档（如 `docs/` 或本文件）

## 17. 给 AI 代理的特别提醒

- 修改代码前，先确认相关测试文件和 build tag（`integration` / `e2e`）
- 新增/修改路由时，同步更新 `internal/app/router.go` 和本文件的第 10 节路由表
- 新增/修改配置时，同步更新 `internal/config/config.go`、`.env.example` 和本文件第 4 节的配置表
- 新增错误码时，在 `internal/errors/errors.go` 定义，并遵循统一错误处理规范
- 修改数据库 Schema 时，使用 `make migrate-create NAME=xxx` 创建新迁移，不要直接修改旧迁移文件
- 涉及邮件模板时，必须同时提供中文和英文版本
- 不要提交 `.env`、`.env.test`、密钥文件、日志或覆盖率文件
- 不要在代码或日志中硬编码密码、Token 等敏感信息

## 18. 相关文档索引

| 文档 | 内容 |
|------|------|
| `README.md` | 项目简介、快速开始、API 端点 |
| `docs/ARCHITECTURE.md` | 系统架构、分层设计、数据模型 |
| `docs/CONFIGURATION.md` | 完整配置说明 |
| `docs/DEPLOYMENT.md` | Docker Compose、K8s、裸机部署 |
| `docs/TRUENAS.md` | TrueNAS 部署 |
| `docs/TESTING.md` | 测试规范、真实 DB/Redis 连接、压测 |
| `docs/E2E_TESTING.md` | E2E 测试详细说明 |
| `docs/EMAIL_SERVICE.md` | 邮件服务开发指南 |
| `docs/SECURITY.md` | 安全特性说明 |
| `docs/PRODUCTION_DEPLOYMENT_CHECKLIST.md` | 生产部署检查清单 |
| `docs/PRESSURE_TESTING_RUNBOOK.md` | 压测执行手册 |
| `docs/DATABASE_SCHEMA.md` | 数据库 Schema 说明 |
| `docs/COVERAGE_ENFORCEMENT.md` | 覆盖率强制策略 |
| `docs/CHANGELOG.md` | 变更日志 |
