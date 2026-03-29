# SSO 单点登录服务

[![Go Version](https://img.shields.io/badge/Go-1.26+-00ADD8?style=flat&logo=go)](https://golang.org/)
[![License](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)
[![CI](https://img.shields.io/badge/CI-Passing-brightgreen.svg)](.github/workflows/ci.yml)

生产级单点登录(SSO)微服务，基于Go 1.26+构建，支持OAuth 2.0和OpenID Connect协议。为内部和外部应用提供统一、安全的认证服务。

## 功能特性

### 核心功能
- **用户认证**：注册、登录、登出、Token刷新
- **JWT Token**：基于RS256签名的Access/Refresh Token
- **OAuth 2.0**：授权码流程、Token端点、客户端管理
- **OpenID Connect**：Discovery端点、JWKS公钥发布

### 高级功能
- **多因素认证(MFA)**：TOTP支持、备用恢复码
- **第三方登录**：Google、GitHub社交登录集成
- **账户安全**：登录失败锁定、密码强度验证
- **审计日志**：关键操作审计追踪
- **管理员功能**：用户管理、系统健康检查

### 运维特性
- **指标监控**：Prometheus指标端点
- **健康检查**：服务状态监控
- **优雅关闭**：支持信号优雅停止
- **数据库迁移**：版本化数据库迁移管理

## 技术栈

| 组件 | 技术 | 版本 |
|------|------|------|
| 语言 | Go | 1.26+ |
| HTTP路由 | gorilla/mux | 1.8+ |
| 数据库 | PostgreSQL | 15+ |
| 缓存 | Redis | 7+ |
| JWT | golang-jwt/jwt/v5 | 5.3+ |
| 部署 | Docker Compose | - |

## 快速开始

### 前置要求

- Go 1.26+
- Docker & Docker Compose
- PostgreSQL 15+
- Redis 7+

### 安装步骤

1. **克隆项目**
```bash
git clone <repo-url>
cd sso
```

2. **配置环境变量**
```bash
cp .env.example .env
# 编辑 .env 文件，修改数据库密码等配置
```

3. **生成RSA密钥**
```bash
make generate-keys
```

4. **启动依赖服务**
```bash
make docker-up
```

5. **运行数据库迁移**
```bash
export DATABASE_URL='postgres://sso:changeme@localhost:5432/sso?sslmode=disable'
make migrate-up
```

6. **启动服务**
```bash
make run
```

服务将在 http://localhost:9090 启动

### 验证安装

```bash
# 检查健康状态
curl http://localhost:9090/health
```

预期响应：
```json
{
  "status": "ok",
  "service": "sso",
  "timestamp": "2024-01-01T12:00:00Z"
}
```

## 开发指南

### 构建命令

```bash
# 构建二进制文件
make build

# 运行应用
make run

# 开发模式（启动依赖服务并运行）
make dev

# 格式化代码
make fmt

# 代码检查
make lint
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

# 生成测试覆盖率报告
make test-coverage
```

### Docker部署

```bash
# 构建Docker镜像
make docker-build

# 启动所有服务
make docker-up

# 停止服务
make docker-down

# 查看日志
make docker-logs
```

### 其他部署方式

- **TrueNAS SCALE**: 查看 [TrueNAS部署指南](docs/TRUENAS.md)
- **Kubernetes**: 查看 [部署文档](docs/DEPLOYMENT.md#kubernetes部署)
- **裸机部署**: 查看 [部署文档](docs/DEPLOYMENT.md#裸机部署)

## API端点

### 系统端点

| 方法 | 路径 | 描述 | 认证 |
|------|------|------|------|
| GET | /health | 健康检查 | 否 |
| GET | /metrics | Prometheus指标 | Basic Auth（可选） |

### OIDC Discovery端点

| 方法 | 路径 | 描述 | 认证 |
|------|------|------|------|
| GET | /.well-known/openid-configuration | OIDC配置 | 否 |
| GET | /.well-known/jwks.json | JWKS公钥 | 否 |

### 认证端点

| 方法 | 路径 | 描述 | 认证 |
|------|------|------|------|
| POST | /api/v1/register | 用户注册 | 否 |
| POST | /api/v1/login | 用户登录 | 否 |
| POST | /api/v1/token | Token签发/刷新 | 否 |
| POST | /api/v1/token/revoke | Token撤销 | 否 |
| POST | /api/v1/forgot-password | 忘记密码 | 否 |
| POST | /api/v1/reset-password | 重置密码 | 否 |
| GET | /api/v1/verify-email | 验证邮箱 | 否 |

### 用户端点（需认证）

| 方法 | 路径 | 描述 | 认证 |
|------|------|------|------|
| GET | /api/v1/userinfo | 获取用户信息（含email_verified） | 是 |
| POST | /api/v1/verify-email/send | 发送验证邮件 | 是 |
| POST | /api/v1/change-password | 修改密码 | 是 |
| POST | /api/v1/logout-all | 登出所有设备 | 是 |

### MFA端点（需认证）

| 方法 | 路径 | 描述 | 认证 |
|------|------|------|------|
| POST | /api/v1/mfa/setup | 设置MFA | 是 |
| POST | /api/v1/mfa/verify | 验证MFA | 是 |
| POST | /api/v1/mfa/disable | 禁用MFA | 是 |
| GET | /api/v1/mfa/status | MFA状态 | 是 |

### OAuth端点（需认证）

| 方法 | 路径 | 描述 | 认证 |
|------|------|------|------|
| GET | /api/v1/authorize | OAuth2授权端点 | 是 |
| POST | /api/v1/authorize/approve | 批准授权 | 是 |

### 第三方登录端点

| 方法 | 路径 | 描述 | 认证 |
|------|------|------|------|
| GET | /auth/providers | 获取支持的提供商 | 否 |
| GET | /auth/{provider} | 第三方登录入口 | 否 |
| GET | /auth/{provider}/callback | 第三方回调 | 否 |

### 管理员端点（需认证+管理员权限）

| 方法 | 路径 | 描述 | 认证 |
|------|------|------|------|
| GET | /admin/health | 系统健康检查 | 是 |
| POST | /admin/cleanup | 清理过期数据 | 是 |
| GET | /admin/users | 用户列表（分页） | 是 |
| GET | /admin/users/{id} | 用户详情 | 是 |
| POST | /admin/users/{id}/disable | 禁用用户 | 是 |
| POST | /admin/users/{id}/enable | 启用用户 | 是 |
| DELETE | /admin/users/{id} | 删除用户 | 是 |
| GET | /admin/audit-logs | 审计日志（分页） | 是 |

## 目录结构

```
sso/
├── cmd/
│   └── server/
│       └── main.go              # 服务入口
├── internal/
│   ├── cache/                   # Redis缓存层
│   ├── config/                  # 配置管理
│   ├── crypto/                  # 加密工具（JWT、密码哈希）
│   ├── errors/                  # 统一错误定义
│   ├── handler/                 # HTTP处理器
│   ├── logging/                 # 日志工具
│   ├── metrics/                 # Prometheus指标
│   ├── middleware/              # HTTP中间件
│   ├── model/                   # 数据模型
│   ├── service/                 # 业务逻辑层
│   ├── store/                   # 数据存储层
│   └── validator/               # 输入验证
├── migrations/                  # 数据库迁移脚本
├── scripts/                     # 工具脚本
├── docker/                      # Docker配置
├── keys/                        # RSA密钥（不提交）
├── static/                      # 静态资源
├── templates/                   # 模板文件
└── docs/                        # 项目文档
```

## 安全特性

### 密码安全
- bcrypt哈希（cost>=12，测试环境可用10）
- 最小密码长度要求
- 密码强度验证（大小写字母、数字、特殊字符）

### Token安全
- JWT使用RS256签名
- Access Token有效期15分钟
- Refresh Token有效期7天
- Token轮换机制（每次刷新生成新Token）
- Token撤销支持
- Token黑名单检查（已撤销Token无法使用）

### 账户安全
- 登录失败锁定机制（默认5次失败锁定30分钟）
- 账户状态管理（活跃/锁定/禁用）

### 网络安全
- Rate Limiting防护（默认100请求/分钟）
- 安全HTTP头（CSP、HSTS、X-Frame-Options等）
- CORS白名单配置
- 生产环境数据库必须启用SSL（`DB_SSL_MODE=require`）

## 环境变量配置

参考 `.env.example` 文件，主要配置项：

| 变量 | 描述 | 默认值 | 生产要求 |
|------|------|--------|----------|
| SERVER_HOST | 服务器监听地址 | 0.0.0.0 | - |
| SERVER_PORT | 服务器端口 | 9090 | - |
| DB_HOST | 数据库主机 | localhost | - |
| DB_PORT | 数据库端口 | 5432 | - |
| DB_NAME | 数据库名称 | sso | - |
| DB_PASSWORD | 数据库密码 | - | **必填** |
| DB_SSL_MODE | 数据库SSL模式 | require | **必须为require** |
| JWT_PRIVATE_KEY_PATH | JWT私钥路径 | ./keys/private.pem | - |
| JWT_PUBLIC_KEY_PATH | JWT公钥路径 | ./keys/public.pem | - |
| BCRYPT_COST | bcrypt成本因子 | 12 | **必须>=12** |
| CORS_ALLOWED_ORIGINS | 允许的跨域源 | http://localhost:3000 | **必填** |
| MAX_LOGIN_ATTEMPTS | 最大登录尝试次数 | 5 | - |
| METRICS_USERNAME | Metrics Basic Auth用户名 | - | 生产环境建议设置 |
| METRICS_PASSWORD | Metrics Basic Auth密码 | - | 生产环境建议设置 |

## 常见问题

### 如何重置管理员密码？

使用数据库直接更新或通过忘记密码流程。

### 如何添加新的OAuth提供商？

1. 在 `internal/service/social.go` 添加提供商实现
2. 在 `internal/handler/social.go` 添加处理器
3. 更新配置文件

### 如何备份数据库？

```bash
# 备份
./scripts/backup.sh

# 恢复
./scripts/restore.sh /backup/sso/sso_20240101_120000.sql.gz
```

## 贡献指南

1. Fork项目
2. 创建功能分支（`git checkout -b feature/AmazingFeature`）
3. 提交更改（`git commit -m 'Add some AmazingFeature'`）
4. 推送到分支（`git push origin feature/AmazingFeature`）
5. 创建Pull Request

### 开发规范

- 遵循Go代码规范
- 运行 `make lint` 检查代码风格
- 确保所有测试通过
- 添加必要的测试覆盖

## 许可证

MIT License

## 联系方式

如有问题或建议，请提交Issue。
