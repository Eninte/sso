# SSO服务 - AI代理协作指南

本指南为AI代理提供在本Go项目中工作的标准规范。

## 项目概述

这是一个基于Go 1.26+的单点登录(SSO)服务，提供OAuth 2.0/OpenID Connect认证功能。

## 构建与测试命令

### 构建命令
```bash
# 构建二进制文件
make build

# 运行应用
make run

# 开发模式（启动依赖服务并运行）
make dev
```

### 测试命令
```bash
# 运行所有测试
make test

# 运行单元测试（短测试）
make test-unit

# 运行集成测试
make test-integration

# 运行单个测试
go test -v -run TestAuthService_Login ./internal/service/

# 运行测试并生成覆盖率报告
make test-coverage
```

### 代码质量
```bash
# 运行代码检查
make lint

# 格式化代码
make fmt

# 安全检查
make test-security
```

### Docker相关
```bash
# 构建Docker镜像
make docker-build

# 启动所有服务
make docker-up

# 停止服务
make docker-down
```

## 代码风格指南

### 导入规范
1. **分组顺序**：标准库 → 第三方库 → 项目内部包
2. **使用别名**：仅在必要时使用（如 `apperrors "github.com/your-org/sso/internal/errors"`）
3. **禁止点导入**：不允许 `import . "package"` 语法

### 命名约定
1. **包名**：小写单词，不使用下划线或混合大小写
2. **接口**：单方法接口以 `er` 结尾（如 `Reader`, `Writer`）
3. **变量**：驼峰式命名，导出变量以大写字母开头
4. **常量**：使用驼峰式或全大写+下划线（根据上下文）
5. **错误变量**：以 `Err` 开头（如 `ErrInvalidCredentials`）

### 类型定义
1. **结构体**：使用描述性名称，字段使用JSON标签
2. **接口**：保持小而专注，避免过大的接口
3. **错误类型**：定义在专门的 `errors` 包中

### 错误处理
1. **立即检查**：错误返回后立即检查，不要延迟
2. **错误包装**：使用 `fmt.Errorf("context: %w", err)` 包装错误
3. **统一错误**：使用预定义的错误变量（如 `apperrors.ErrInvalidCredentials`）
4. **日志记录**：使用 `slog` 记录错误上下文

### 注释规范
1. **包注释**：每个包必须有包注释
2. **函数注释**：导出函数必须有注释，描述功能和参数
3. **代码分隔**：使用注释分隔符组织代码块（如 `// ====`）
4. **中文注释**：允许使用中文注释，但错误消息使用英文

### 测试规范
1. **测试包**：使用 `package_test` 包进行黑盒测试
2. **测试框架**：使用 `testify/assert` 和 `testify/require`
3. **表驱动测试**：优先使用表驱动测试模式
4. **测试命名**：`TestFunctionName_Scenario` 格式
5. **Mock对象**：使用专门的mock包（如 `internal/store/mock`）

### 项目结构
```
SSO/
├── cmd/           # 主要应用程序入口
├── internal/      # 私有应用程序代码
│   ├── cache/     # 缓存相关
│   ├── config/    # 配置管理
│   ├── crypto/    # 加密工具
│   ├── errors/    # 统一错误定义
│   ├── handler/   # HTTP处理器
│   ├── logging/   # 日志工具
│   ├── metrics/   # 指标收集
│   ├── middleware/ # HTTP中间件
│   ├── model/     # 数据模型
│   ├── service/   # 业务逻辑
│   ├── store/     # 数据存储层
│   └── validator/ # 输入验证
├── migrations/    # 数据库迁移
├── scripts/       # 工具脚本
├── static/        # 静态资源
├── templates/     # 模板文件
└── testdata/      # 测试数据
```

## 开发工作流

1. **创建分支**：从 `develop` 分支创建功能分支
2. **编写代码**：遵循上述代码风格
3. **运行测试**：确保所有测试通过
4. **代码检查**：运行 `make lint` 和 `make test-security`
5. **提交代码**：使用清晰的提交消息

## 注意事项

1. **数据库迁移**：使用 `make migrate-create NAME=description` 创建迁移
2. **密钥管理**：使用 `make generate-keys` 生成RSA密钥
3. **环境变量**：参考 `.env.example` 配置环境变量
4. **依赖管理**：使用 `go mod tidy` 管理依赖
5. **并发安全**：注意并发访问共享数据时的线程安全
