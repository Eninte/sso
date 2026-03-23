# SSO 单点登录服务

## 简介

这是一个生产级的单点登录(SSO)微服务，支持OAuth 2.0和OpenID Connect协议。可以为内部和外部应用提供统一的认证服务。

## 功能特性

- 用户注册/登录
- JWT Token签发与验证
- OAuth 2.0授权码流程
- OpenID Connect支持
- 多因素认证(MFA) (Phase 4)
- 第三方登录 (Phase 4)

## 技术栈

| 组件 | 技术 |
|------|------|
| 语言 | Go 1.22+ |
| HTTP路由 | gorilla/mux |
| 数据库 | PostgreSQL 15 |
| 缓存 | Redis 7 |
| JWT | golang-jwt/jwt/v5 |
| 部署 | Docker Compose |

## 快速开始

### 前置要求

- Go 1.22+
- Docker & Docker Compose
- PostgreSQL 15+
- Redis 7+

### 安装步骤

1. 克隆项目
```bash
git clone <repo-url>
cd sso
```

2. 配置环境变量
```bash
cp .env.example .env
# 编辑 .env 文件，修改数据库密码等配置
```

3. 生成密钥
```bash
make generate-keys
```

4. 启动依赖服务
```bash
make docker-up
```

5. 运行数据库迁移
```bash
export DATABASE_URL='postgres://sso:changeme@localhost:5432/sso?sslmode=disable'
make migrate-up
```

6. 启动服务
```bash
make run
```

服务将在 http://localhost:9090 启动

### 验证安装

```bash
# 检查健康状态
curl http://localhost:9090/health
```

预期响应:
```json
{"status":"ok","service":"sso","timestamp":"2024-01-01T12:00:00Z"}
```

## 运行测试

```bash
# 运行所有测试
make test

# 运行单元测试
make test-unit

# 运行集成测试
make test-integration

# 生成测试覆盖率报告
make test-coverage
```

## API端点

### 系统端点

| 方法 | 路径 | 描述 |
|------|------|------|
| GET | /health | 健康检查 |

### OIDC Discovery端点

| 方法 | 路径 | 描述 |
|------|------|------|
| GET | /.well-known/openid-configuration | OIDC配置 |
| GET | /.well-known/jwks.json | JWKS公钥 |

### 认证端点 (Phase 1)

| 方法 | 路径 | 描述 |
|------|------|------|
| POST | /register | 用户注册 |
| POST | /login | 用户登录 |
| POST | /token | Token签发/刷新 |
| POST | /token/revoke | Token撤销 |
| GET | /userinfo | 获取用户信息 |
| GET | /authorize | OAuth2授权端点 |

## 目录结构

```
sso/
├── cmd/server/main.go          # 服务入口
├── internal/                   # 私有代码
│   ├── config/                 # 配置管理
│   ├── model/                  # 数据模型
│   ├── store/                  # 数据存储层
│   ├── handler/                # HTTP处理器
│   ├── service/                # 业务逻辑
│   ├── middleware/             # 中间件
│   ├── crypto/                 # 加密工具
│   └── validator/              # 输入验证
├── migrations/                 # 数据库迁移
├── scripts/                    # 工具脚本
├── docker/                     # Docker配置
└── docs/                       # 文档
```

## 安全特性

- 密码使用bcrypt哈希 (cost=12)
- JWT使用RS256签名
- Access Token有效期15分钟
- Refresh Token有效期7天
- 登录失败锁定机制
- Rate Limiting防护
- 安全HTTP头
- CORS白名单

## 备份恢复

```bash
# 备份数据库
./scripts/backup.sh

# 恢复数据库
./scripts/restore.sh /backup/sso/sso_20240101_120000.sql.gz
```

## 许可证

MIT License
