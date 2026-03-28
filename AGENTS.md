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

## 数据库与基础设施

```bash
make migrate-up                                    # 执行迁移
make migrate-down                                  # 回滚迁移
make migrate-create NAME=description               # 创建迁移文件
make generate-keys                                 # 生成RSA密钥到./keys/
make docker-up / make docker-down                  # 启动/停止Docker服务
```

## 分层架构

- **Handler**（`internal/handler/`）— HTTP路由、输入验证、错误响应
- **Service**（`internal/service/`）— 业务逻辑、事务管理
- **Store**（`internal/store/`）— 数据访问接口；Postgres实现在`store/postgres/`
- **Model**（`internal/model/`）— 数据结构定义（含JSON标签）

依赖注入通过接口实现（`store.Store`、`service.AuthServiceInterface`）。测试使用 `internal/store/mock` 包。

## 错误处理

- 使用 `internal/errors` 中预定义的错误变量（`apperrors.ErrInvalidCredentials`等）
- 新错误用 `apperrors.New(code, message, httpStatus)` 或 `apperrors.Wrap(...)` 构造
- Store层返回 `store.ErrNotFound`、`store.ErrDuplicateEmail`
- Service层用 `fmt.Errorf("context: %w", err)` 包装错误
- Handler层映射为HTTP状态码；响应消息使用 `ErrCode*` 常量
- 使用 `slog` 记录错误，包含上下文（user_id、request_id）

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
- Mock：`mock.New()` 创建实例，`store.Reset()` 清空数据
- 错误注入：设置 `store.CreateUserErr`、`store.GetUserByIDErr` 等字段
- 测试中使用 `crypto.NewPasswordService(10)`（降低bcrypt cost）

## JWT与安全

- Access Token：RS256签名，包含用户声明
- Refresh Token：32字节随机字符串，不含用户信息
- 生产环境bcrypt cost必须 >= 12（测试可用10）
- 登录锁定：5次失败 → 锁定30分钟
- 限流：默认100请求/分钟
- CORS：生产环境必须设置 `CORS_ALLOWED_ORIGINS`

## 常见问题

- JWT验证失败 → 检查签名算法是否为RS256
- 数据库连接失败 → 检查 `DB_PASSWORD` 环境变量
- CORS错误 → 检查 `CORS_ALLOWED_ORIGINS` 配置
- 密钥错误 → 运行 `make generate-keys` 创建 `./keys/private.pem` 和 `./keys/public.pem`
