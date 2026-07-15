# SSO服务 - AI代理协作指南

Go 1.26+ 单点登录服务，提供OAuth 2.0/OpenID Connect认证功能。

## 测试

```bash
make generate-keys          # 生成JWT密钥
cp .env.example .env.test   # 配置测试环境（编辑填写凭据）
make test                   # 运行所有测试
make dev                    # 启动服务
```

### E2E端到端测试

```bash
# 1. 启动服务（禁用限流）
RATE_LIMIT_REQUESTS=0 make run &

# 2. 准备测试数据（启用自动验证触发器）
make test-e2e-prepare

# 3. 运行E2E测试
make test-e2e

# 4. 清理测试环境（禁用触发器）
make test-e2e-cleanup
```

**E2E测试状态**：
- 测试状态以最新 CI 运行为准（GitHub Actions 的 E2E Tests 作业）
- 测试覆盖：注册、登录、Token、OAuth、MFA、管理员、密码重置/修改、社交登录、并发、安全
- 历史失败（TOTP 重放断言、管理员路由前缀等）已在最近的修复中解决，CI 全绿

**测试数据准备机制**：
- 使用PostgreSQL触发器自动验证 `@example.com` 测试用户
- 不污染生产代码，测试后可完全移除
- 详细说明：`docs/E2E_TESTING.md`

**覆盖率报告**：

```bash
make test-coverage          # 单元+集成覆盖率（阈值 80%）
make test-e2e-coverage      # E2E 覆盖率（针对 internal/... ）
make test-coverage-full     # 合并单元+集成+E2E 覆盖率
```

**测试分层**：

| 层级 | Build Tag | 命令 | 说明 |
|------|-----------|------|------|
| 单元测试 | 无 | `make test-unit` 或 `go test -short ./...` | 无外部依赖，使用 mock |
| 集成测试 | `//go:build integration` | `make test-integration` | 需要真实 PostgreSQL/Redis |
| E2E 测试 | `//go:build e2e` | `make test-e2e` | 需要运行中的 SSO 服务 |

- `internal/store/postgres/*_test.go` 使用 `integration` tag（需真实 DB）
- `test/e2e/*_test.go` 使用 `e2e` tag（需运行服务）
- 其他 `internal/**/*_test.go` 为单元测试（无 tag）

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

### 测试环境连接信息

测试数据库和服务已部署在远程主机，配置存储在 `.env.test` 中（不可提交/分发）。

连接方式：

```bash
# 加载测试环境变量
source .env.test

# PostgreSQL 连接
echo $DATABASE_URL

# Redis 连接
echo $REDIS_URL
```

> ⚠️ **安全要求：** 不得将 `.env.test` 中的凭据硬编码到代码或文档中。如需新环境凭据，联系项目维护者。

所有配置项详见 `.env.example`。测试环境关键差异：

| 配置项 | 测试环境 | 生产环境 |
|--------|---------|---------|
| `BCRYPT_COST` | `10` | `>=12` (必须) |
| `DB_SSL_MODE` | `disable` | `require` (必须) |
| `MFA_RECOVERY_HMAC_KEY` | 可选 | **必须设置强密钥** |
| `CORS_ALLOWED_ORIGINS` | `localhost` | 生产域名 (必须) |
| `SMTP_HOST` | 测试SMTP服务器 | 生产SMTP服务器 (必须) |
| `SMTP_PASSWORD` | 测试密码/授权码 | 生产密码/授权码 (必须) |

**生产环境启动检查**：`SERVER_ENV=production` 且 `MFA_RECOVERY_HMAC_KEY` 为空时拒绝启动。

**邮件服务配置**：详见 `docs/EMAIL_SERVICE.md`

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
| 邮件 | 支持验证邮件、密码重置邮件，响应式设计，多语言支持 |

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

## 邮件服务开发指南

### 邮件模板系统

邮件模板采用模板继承机制，所有模板基于 `internal/service/email/templates/base.html`。

**模板结构：**
```
templates/
├── base.html (基础布局，包含所有样式)
├── verification/ (验证邮件)
│   ├── verification_zh.html (中文)
│   └── verification_en.html (英文)
└── password_reset/ (密码重置)
    ├── password_reset_zh.html (中文)
    └── password_reset_en.html (英文)
```

### 添加新邮件类型

1. **创建模板文件**（在对应目录下创建中英文版本）
2. **在 `engine.go` 添加渲染方法**
3. **在 `email.go` 添加发送方法**

详细步骤参考：`docs/EMAIL_SERVICE.md`

### 邮件测试工具

```bash
# 发送测试邮件
go run scripts/test_email.go -to user@example.com -type verification

# 渲染模板预览
go run scripts/render_email_template.go -type verification -lang zh -output /tmp/email.html
```

### 配色规范

- **主色调**：蓝色 (#1e88e5)
- **按钮**：蓝色背景 + 白色文字（高对比度）
- **安全提示**：浅黄背景 + 橙色边框
- **响应式设计**：支持移动端和桌面端
- **深色模式**：自动适配系统主题

### 邮件开发规范

- ✅ 使用 `TemplateEngine` 渲染模板
- ✅ 异步发送邮件，不阻塞主流程
- ✅ 记录发送日志（成功/失败）
- ✅ 使用内联样式确保兼容性
- ❌ 禁止在邮件中包含敏感信息
- ❌ 禁止硬编码SMTP凭据

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
