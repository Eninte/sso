# SSO服务 - AI代理协作指南

Go 1.26+ 单点登录服务，提供OAuth 2.0/OpenID Connect认证功能。

## 快速开始

```bash
# 1. 生成JWT密钥
make generate-keys

# 2. 配置测试环境变量
cp .env.example .env.test
# 编辑 .env.test 填写真实的测试环境凭据

# 3. 运行测试
make test

# 4. 启动服务
make dev
```

## 构建与运行

```bash
make build              # 构建到 ./bin/sso
make run                # 运行服务
make dev                # 启动依赖(Docker)并运行
make clean              # 清理构建产物
```

## 测试

```bash
make test              # 运行所有测试
make test-coverage     # 生成覆盖率报告
```

详细测试指南请参考：[TESTING.md](./TESTING.md)

## 代码检查

```bash
make lint               # go vet + golangci-lint
make fmt                # go fmt ./...
make test-security      # go vet + govulncheck
```

Linter配置：`.golangci.yml`（enable-all选择性禁用）。提交前必须运行 `make lint`。

## 环境配置

**⚠️ 重要：禁止安装 PostgreSQL 或 Redis！测试服务已在远程主机运行，直接使用。**

### 配置文件

- `.env.example` - 配置模板（可安全分发）
- `.env.test` - 测试环境配置（包含真实凭据，不可分发）
- `.env` - 本地开发配置（自行创建，不提交到Git）

### 配置说明

所有配置项详见 `.env.example` 文件，主要配置项包括：

1. 服务器配置：`SERVER_HOST`, `SERVER_PORT`, `SERVER_ENV`
2. 数据库配置：`DB_HOST`, `DB_PORT`, `DB_NAME`, `DB_USER`, `DB_PASSWORD`, `DB_SSL_MODE`
3. Redis配置：`REDIS_HOST`, `REDIS_PORT`, `REDIS_PASSWORD`
4. JWT配置：`JWT_PRIVATE_KEY_PATH`, `JWT_PUBLIC_KEY_PATH`, `JWT_ACCESS_TOKEN_TTL`, `JWT_REFRESH_TOKEN_TTL`
5. 安全配置：`BCRYPT_COST`, `RATE_LIMIT_REQUESTS`, `MAX_LOGIN_ATTEMPTS`, `LOCKOUT_DURATION`
6. SMTP配置：`SMTP_HOST`, `SMTP_PORT`, `SMTP_USER`, `SMTP_PASSWORD`, `SMTP_FROM`
7. CORS配置：`CORS_ALLOWED_ORIGINS`
8. Metrics配置：`METRICS_USERNAME`, `METRICS_PASSWORD`
9. E2E测试配置：`E2E_ADMIN_EMAIL`, `E2E_ADMIN_PASSWORD`

### 环境差异

| 配置项 | 测试环境 | 生产环境 |
|--------|---------|---------|
| `BCRYPT_COST` | `10` (加快测试) | `>=12` (必须) |
| `DB_SSL_MODE` | `disable` (测试) | `require` (必须) |
| `DB_PASSWORD` | 测试密码 | 强密码 (必须) |
| `REDIS_PASSWORD` | 无密码 | 强密码 (必须) |
| `CORS_ALLOWED_ORIGINS` | `localhost` | 生产域名 (必须) |

运行数据库测试：
```bash
DATABASE_URL="postgres://sso:sso@192.168.1.3:5432/sso_test?sslmode=disable" make test
```

### 数据库迁移与工具

```bash
make migrate-up                    # 执行数据库迁移
make migrate-down                  # 回滚数据库迁移
make migrate-create NAME=xxx       # 创建新迁移文件
make generate-keys                 # 生成RSA密钥对
make docker-up                     # 启动Docker服务
make docker-down                   # 停止Docker服务
```

## 架构设计

### 分层架构

```
Handler → Service → Store → Database
  ↓         ↓         ↓
Model    Model     Model
```

| 层级 | 路径 | 职责 |
|------|------|------|
| **Handler** | `internal/handler/` | HTTP路由、请求验证、响应格式化 |
| **Service** | `internal/service/` | 业务逻辑、事务管理、权限控制 |
| **Store** | `internal/store/` | 数据访问接口（Postgres实现在`store/postgres/`） |
| **Model** | `internal/model/` | 数据结构定义（含JSON/DB标签） |

### 依赖注入

- 通过接口实现松耦合：`store.Store`、`service.AuthServiceInterface`
- 测试使用Mock实现：`internal/store/mock`
- 便于单元测试和集成测试

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

## 代码风格规范

### 导入规则

```go
import (
    // 标准库
    "context"
    "fmt"
    
    // 第三方库
    "github.com/golang-jwt/jwt/v5"
    
    // 项目包
    apperrors "github.com/your-org/sso/internal/errors"
    "github.com/your-org/sso/internal/model"
)
```

- 分组顺序：标准库 → 第三方 → 项目包
- 仅必要时使用别名
- 禁止点导入（`. "package"`）

### 命名约定

| 类型 | 规则 | 示例 |
|------|------|------|
| 包名 | 小写无下划线 | `store`（非`data_store`） |
| 接口 | 单方法以`-er`结尾 | `Reader`、`Writer`、`Store` |
| 错误变量 | `Err`前缀 | `ErrInvalidCredentials` |
| 导出标识符 | 大写开头 | `CreateUser`、`UserModel` |
| 未导出标识符 | camelCase | `validateEmail`、`userStore` |

### 注释规范

```go
// Package service 提供业务逻辑实现
package service

// CreateUser 创建新用户
// 参数email必须是有效的邮箱地址
func CreateUser(email string) error {
    // ==== 验证邮箱格式 ====
    if !isValidEmail(email) {
        return ErrInvalidEmail
    }
    
    // ==== 创建用户 ====
    return store.Create(email)
}
```

- 包注释必须在`package`声明上方
- 导出函数必须有文档注释
- 使用`// ====`分隔符组织代码块
- 允许中文注释，但错误消息使用英文

### 结构体标签

```go
type User struct {
    ID        int64     `json:"id" db:"id"`
    Email     string    `json:"email" db:"email"`
    CreatedAt time.Time `json:"created_at,omitempty" db:"created_at"`
}
```

- Model结构体必须有JSON标签
- 使用`omitempty`处理可选字段



## 安全机制

### JWT令牌

| 令牌类型 | 算法 | 说明 |
|---------|------|------|
| Access Token | RS256 | 必须验证签名算法为`jwt.SigningMethodRS256` |
| Refresh Token | Random | 32字节随机字符串，不含用户信息 |

### 密码安全

- 测试环境：`BCRYPT_COST=10`（加快测试速度）
- 生产环境：`BCRYPT_COST>=12`（必须）

### 账户保护

- 登录失败锁定：5次失败 → 锁定30分钟
- 限流：默认100请求/分钟（`middleware.RateLimiter`）

### 传输安全

- 数据库连接：生产环境必须`DB_SSL_MODE=require`
- CORS：生产环境必须配置`CORS_ALLOWED_ORIGINS`

### 安全检查

```bash
make test-security    # 运行安全扫描（go vet + govulncheck）
make lint             # 代码质量检查
```

## 故障排查

### JWT相关

| 问题 | 解决方案 |
|------|---------|
| JWT验证失败 | 检查签名算法是否为RS256 |
| 密钥错误 | 运行`make generate-keys`生成`./keys/private.pem`和`./keys/public.pem` |
| Token过期 | 检查系统时间是否正确 |

### 数据库相关

| 问题 | 解决方案 |
|------|---------|
| 连接失败 | 检查`DB_PASSWORD`环境变量和网络连接 |
| 迁移失败 | 检查数据库权限，运行`make migrate-down`后重试 |
| SSL错误 | 确认`DB_SSL_MODE`配置正确 |

### 配置相关

| 问题 | 解决方案 |
|------|---------|
| CORS错误 | 检查`CORS_ALLOWED_ORIGINS`是否包含请求源 |
| 邮件发送失败 | 检查SMTP配置和网络连接 |
| 生产环境启动失败 | 确认`DB_SSL_MODE=require`且`BCRYPT_COST>=12` |



## 开发工作流

### 新功能开发

1. 创建功能分支：`git checkout -b feature/xxx`
2. 编写测试：先写测试用例（TDD，参考[TESTING.md](./TESTING.md)）
3. 实现功能：按照分层架构实现
4. 运行测试：`make test`
5. 代码检查：`make lint`
6. 提交代码：`git commit -m "feat: xxx"`

### Bug修复

1. 创建修复分支：`git checkout -b fix/xxx`
2. 编写复现测试：确保测试失败
3. 修复Bug：修改代码
4. 验证修复：确保测试通过
5. 回归测试：`make test`
6. 提交代码：`git commit -m "fix: xxx"`

### 代码审查检查清单

- [ ] 所有测试通过（`make test`）
- [ ] 代码检查通过（`make lint`）
- [ ] 安全扫描通过（`make test-security`）
- [ ] 测试覆盖率 >= 80%（`make test-coverage`）
- [ ] 遵循错误处理规范
- [ ] 遵循代码风格规范
- [ ] 添加必要的文档注释
