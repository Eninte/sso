# SSO服务 - AI代理协作指南

Go 1.26+ 单点登录服务，提供OAuth 2.0/OpenID Connect认证功能。

## 快速开始

```bash
make generate-keys          # 生成JWT密钥
cp .env.example .env.test   # 配置测试环境（编辑填写凭据）
make test                   # 运行所有测试
make dev                    # 启动服务
```

## 构建与运行

```bash
make build              # 构建到 ./bin/sso
make run                # 运行服务
make dev                # 启动依赖(Docker)并运行
make clean              # 清理构建产物
make lint               # go vet + golangci-lint（提交前必须运行）
make fmt                # go fmt ./...
make test-security      # go vet + govulncheck
make test-coverage      # 生成覆盖率报告
make migrate-up         # 执行数据库迁移
make migrate-down       # 回滚数据库迁移
make migrate-create NAME=xxx  # 创建新迁移文件
```

## 环境配置

**⚠️ 禁止安装 PostgreSQL 或 Redis！测试服务已在远程主机运行，直接使用。**

- `.env.example` - 配置模板（可安全分发）
- `.env.test` - 测试环境配置（含真实凭据，不可分发）
- `.env` - 本地开发配置（自行创建，不提交）

所有配置项详见 `.env.example`。测试环境关键差异：

| 配置项 | 测试环境 | 生产环境 |
|--------|---------|---------|
| `BCRYPT_COST` | `10` | `>=12` (必须) |
| `DB_SSL_MODE` | `disable` | `require` (必须) |
| `MFA_RECOVERY_HMAC_KEY` | 可选 | **必须设置强密钥** |
| `CORS_ALLOWED_ORIGINS` | `localhost` | 生产域名 (必须) |

**生产环境启动检查**：`SERVER_ENV=production` 且 `MFA_RECOVERY_HMAC_KEY` 为空时拒绝启动。

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

- 通过接口实现松耦合：`store.Store`、`service.AuthServiceInterface`
- 测试使用Mock实现：`internal/store/mock`

## 统一错误处理

使用 `internal/errors` 包，**所有层级必须遵守以下规范**。

### 核心规则

- 使用 `apperrors.Err*` 预定义错误变量，禁止自行创建错误类型
- Store层返回 `store.ErrNotFound`、`store.ErrDuplicateEmail` 等
- Service层用 `fmt.Errorf("上下文: %w", err)` 包装，或直接返回预定义错误
- Handler层映射为HTTP状态码，使用 `ErrCode*` 常量作为响应消息

### 错误构造

```go
apperrors.New(code, message, httpStatus)   // 创建新错误
apperrors.Wrap(code, message, httpStatus, err)  // 包装错误
apperrors.Is(err, store.ErrNotFound)       // 判断错误类型
```

### 禁止事项

- ❌ 禁止 `errors.New("message")` 创建原始错误
- ❌ 禁止在响应中暴露内部错误详情
- ❌ 禁止忽略错误（除 `_ =` 标注的审计日志调用）

## 代码风格规范

### 导入规则

分组顺序：标准库 → 第三方 → 项目包。仅必要时使用别名，禁止点导入。

### 命名约定

| 类型 | 规则 | 示例 |
|------|------|------|
| 包名 | 小写无下划线 | `store`（非`data_store`） |
| 接口 | 单方法以`-er`结尾 | `Reader`、`Writer` |
| 错误变量 | `Err`前缀 | `ErrInvalidCredentials` |
| 导出标识符 | 大写开头 | `CreateUser` |
| 未导出标识符 | camelCase | `validateEmail` |

### 注释规范

- 包注释必须在 `package` 声明上方
- 导出函数必须有文档注释
- 使用 `// ====` 分隔符组织代码块
- 允许中文注释，错误消息使用英文

### 结构体标签

Model结构体必须有JSON标签，使用 `omitempty` 处理可选字段。

## 安全机制

| 机制 | 规则 |
|------|------|
| JWT | Access Token用RS256，Refresh Token用32字节随机字符串 |
| 密码 | 测试BCRYPT_COST=10，生产>=12 |
| 账户保护 | 5次登录失败锁定30分钟，限流100请求/分钟 |
| MFA | TOTP支持，恢复码HMAC-SHA256哈希（O(1)查找），使用后立即失效 |
| 缓存 | UserInfo 5分钟TTL，密码/角色变更时失效，Singleflight防击穿 |
| 传输 | 生产DB_SSL_MODE=require，生产必须配置CORS |

## 工具模块使用指南

所有新代码**必须**使用以下工具模块，禁止重复实现。

### 1. serviceutil (`internal/util/serviceutil/errors.go`)

Service层错误处理标准。Store错误用 `HandleStoreError(err, notFoundErr)` 映射，保持错误语义不暴露内部细节。

### 2. auditutil (`internal/util/auditutil/logging.go`)

审计日志标准。用 `SafeAuditLog(ctx, auditSvc, event, userID, metadata)` 记录操作，失败自动回退stderr不影响主流程。

### 3. handlerutil (`internal/util/handlerutil/response.go`)

HTTP响应标准。用 `WriteJSONError(w, err)` / `WriteJSONSuccess(w, data)` / `WriteValidationError(w, field, message)` 统一响应格式。

### 禁止事项

- ❌ Service层直接处理Store错误（用 `HandleStoreError`）
- ❌ Service层直接调用 `auditSvc.Log()`（用 `SafeAuditLog`）
- ❌ Handler层直接写JSON错误响应（用 `WriteJSONError`）

## 开发工作流

### 新功能/修复

1. 创建分支 → 2. 写测试(TDD) → 3. 实现功能 → 4. 使用工具模块 → 5. `make test` → 6. `make lint` → 7. 提交

### 代码审查检查清单

- [ ] `make test` 通过
- [ ] `make lint` 通过
- [ ] `make test-security` 通过
- [ ] 覆盖率 >= 80%
- [ ] Service层使用 `serviceutil` + `auditutil`
- [ ] Handler层使用 `handlerutil`
- [ ] 遵循错误处理和代码风格规范
