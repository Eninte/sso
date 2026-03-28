# SSO服务 - AI代理协作指南

Go 1.26+ 单点登录服务，提供OAuth 2.0/OpenID Connect认证功能。

## 构建与运行

```bash
make build              # 构建到 ./bin/sso
make run                # 运行服务
make dev                # 启动依赖(Docker)并运行
make clean              # 清理构建产物
```

## 测试

```bash
make test                         # 全部测试（含-race）
make test-unit                    # 仅短测试
make test-integration             # 集成测试(-tags=integration)
make test-coverage                # 生成覆盖率报告
make bench                        # 全部基准测试

# 单个测试模式：
go test -v -run TestAuthService_Login ./internal/service/
go test -v -run TestAuthService_Register/邮箱已存在 ./internal/service/
go test -v -race -count=1 ./internal/handler/...
```

## 代码检查

```bash
make lint               # go vet + golangci-lint
make fmt                # go fmt ./...
make test-security      # go vet + govulncheck
```

Linter配置：`.golangci.yml`（enable-all选择性禁用）。提交前必须运行 `make lint`。

## 环境配置

**⚠️ 禁止安装 PostgreSQL 或 Redis！测试服务已在远程主机运行，直接使用。**

| 变量 | 测试环境 | 生产环境 | 说明 |
|------|---------|---------|------|
| `DB_HOST` | `192.168.1.3` | 按需 | |
| `DB_PORT` | `5432` | `5432` | |
| `DB_NAME` | `sso_test` | `sso` | |
| `DB_USER` | `sso` | `sso` | |
| `DB_PASSWORD` | `sso`（测试） | **必须设置强密码** | 生产禁止使用示例值 |
| `DB_SSL_MODE` | `disable` | **`require`** | 生产禁止disable |
| `REDIS_HOST` | `192.168.1.3` | 按需 | |
| `REDIS_PORT` | `30059` | `6379` | |
| `REDIS_PASSWORD` | 无 | 按需 | |
| `BCRYPT_COST` | `10` | **`>=12`** | |
| `CORS_ALLOWED_ORIGINS` | `http://localhost:3000` | **`https://your.com`** | |
| `JWT_PRIVATE_KEY_PATH` | `./keys/private.pem` | `./keys/private.pem` | `make generate-keys` |
| `JWT_PUBLIC_KEY_PATH` | `./keys/public.pem` | `./keys/public.pem` | |

运行需要数据库的测试：`DATABASE_URL="postgres://sso:sso@192.168.1.3:5432/sso_test?sslmode=disable" make test`

配置文件：`.env.test`（测试）、`.env.example`（模板）。

```bash
make migrate-up / make migrate-down              # 数据库迁移
make migrate-create NAME=description             # 创建迁移文件
make generate-keys                               # 生成RSA密钥
make docker-up / make docker-down                # 启动/停止Docker服务
```

## 分层架构

- **Handler**（`internal/handler/`）— HTTP路由、输入验证、错误响应
- **Service**（`internal/service/`）— 业务逻辑、事务管理
- **Store**（`internal/store/`）— 数据访问接口；Postgres实现在`store/postgres/`
- **Model**（`internal/model/`）— 数据结构定义（含JSON标签）

依赖注入通过接口实现（`store.Store`、`service.AuthServiceInterface`）。测试使用 `internal/store/mock` 包。

## 统一错误处理

本项目使用 `internal/errors` 包实现统一错误体系，**所有层级必须遵守以下规范**。

### 错误码与预定义错误

- 使用 `apperrors.Err*` 预定义错误变量，不要自行创建新错误类型
- 预定义错误列表见 `internal/errors/errors.go:154-232`
- 示例：`apperrors.ErrInvalidCredentials`、`apperrors.ErrEmailExists`、`apperrors.ErrAccountLocked`

### 错误构造

```go
// 创建新错误
apperrors.New(code, message, httpStatus)

// 包装已有错误
apperrors.Wrap(code, message, httpStatus, err)

// 添加详情
appErr.WithDetails("extra info")
```

### 各层错误处理规则

| 层级 | 规则 | 示例 |
|------|------|------|
| Store | 返回 `store.ErrNotFound`、`store.ErrDuplicateEmail` 等统一错误 | `return store.ErrNotFound` |
| Service | 用 `fmt.Errorf("context: %w", err)` 包装，或直接返回预定义错误 | `return fmt.Errorf("创建用户失败: %w", err)` |
| Handler | 映射为HTTP状态码，响应消息使用 `ErrCode*` 常量 | 见下方Handler错误响应示例 |

### Handler错误响应

Handler层使用 `ErrCode*` 常量（如 `ErrCodeLoginFailed`、`ErrCodeRegisterFailed`）作为响应消息：

```go
// 使用 apperrors 获取HTTP状态码和错误码
w.WriteHeader(apperrors.GetHTTPStatus(err))
json.NewEncoder(w).Encode(map[string]string{
    "error": string(apperrors.GetErrorCode(err)),
})
```

### 错误判断

```go
// 判断错误类型
apperrors.Is(err, store.ErrNotFound)

// 类型断言
var appErr *apperrors.AppError
apperrors.As(err, &appErr)
```

### 禁止事项

- ❌ 禁止在Handler/Service层直接 `errors.New("message")` 创建原始错误
- ❌ 禁止在响应中暴露内部错误详情（如数据库错误、堆栈信息）
- ❌ 禁止忽略错误（除明确标注 `_ =` 的审计日志调用）

## 代码风格

### 导入
分组顺序：标准库 → 第三方 → 项目包。仅必要时使用别名（`apperrors "github.com/your-org/sso/internal/errors"`）。禁止点导入。

### 命名
- 包名：小写无下划线（`store` 非 `data_store`）
- 接口：单方法以 `-er` 结尾（`Reader`、`Store`）
- 错误变量：`Err` 前缀（`ErrInvalidCredentials`）
- 导出：大写开头；未导出：camelCase

### 注释
- 包注释必须在 `package` 声明上方
- 导出函数必须有文档注释
- 使用 `// ====` 分隔符组织代码块
- 允许中文注释，但错误消息使用英文

### 结构体标签
Model结构体必须有JSON标签：`json:"field_name,omitempty"`。

## 测试规范

- 黑盒测试：`package service_test`（非 `package service`）
- 框架：`testify/assert` + `testify/require`
- 优先使用表驱动测试
- 命名：`TestFunctionName_场景`（如 `TestAuthService_Register_邮箱已存在`）
- Mock：`mock.New()` 创建实例，`mockStore.Reset()` 清空数据
- 错误注入：设置 `store.CreateUserErr`、`store.GetUserByIDErr` 等字段
- 测试中使用 `crypto.NewPasswordService(10)`（降低bcrypt cost）

## JWT与安全

- Access Token：RS256签名，必须验证签名算法（`jwt.SigningMethodRS256`）
- Refresh Token：32字节随机字符串，不含用户信息
- 生产环境bcrypt cost必须 >= 12（测试可用10）
- 生产环境必须设置 `DB_SSL_MODE=require` 或更高
- 登录锁定：5次失败 → 锁定30分钟
- 限流：默认100请求/分钟（通过 `middleware.RateLimiter` 实现）
- CORS：生产环境必须设置 `CORS_ALLOWED_ORIGINS`

## 常见问题

- JWT验证失败 → 检查签名算法是否为RS256
- 数据库连接失败 → 检查 `DB_PASSWORD` 环境变量
- CORS错误 → 检查 `CORS_ALLOWED_ORIGINS` 配置
- 密钥错误 → 运行 `make generate-keys` 创建 `./keys/private.pem` 和 `./keys/public.pem`
- 生产环境启动失败 → 检查 `DB_SSL_MODE` 是否为 `require`，`BCRYPT_COST` 是否 >= 12
