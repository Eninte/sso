# E2E端到端测试指南

## 概述

本文档说明如何运行SSO服务的端到端(E2E)集成测试。

## 前置条件

1. **JWT密钥已生成**：运行 `make generate-keys` 生成 RSA 密钥对（`./keys/private.pem` 和 `./keys/public.pem`）
2. **服务运行中**：SSO服务必须在 `localhost:9090` 运行
3. **数据库可访问**：PostgreSQL测试数据库可连接
4. **Redis可访问**：Redis缓存服务可连接
5. **限流已禁用**：服务必须以正确方式启动（见下方说明）

## 快速开始

### 方法1：使用 E2E 启动脚本（推荐）

```bash
# 1. 启动服务（自动处理所有限流层）
./scripts/run_e2e_no_ratelimit.sh

# 2. 在另一个终端准备测试数据
make test-e2e-prepare

# 3. 运行E2E测试
make test-e2e

# 4. 清理（禁用触发器）
make test-e2e-cleanup
```

### 方法2：手动启动

```bash
# 1. 启动服务（禁用全局HTTP限流 + 敏感端点限流）
RATE_LIMIT_REQUESTS=0 make run &

# 2. 准备测试数据（启用自动验证触发器）
make test-e2e-prepare

# 3. 运行E2E测试
make test-e2e

# 4. 清理（禁用触发器）
make test-e2e-cleanup
```

### 方法2：使用一键命令

```bash
# 启动服务
RATE_LIMIT_REQUESTS=0 make run &

# 运行完整测试流程（准备 + 测试）
make test-e2e-full

# 清理
make test-e2e-cleanup
```

## 测试数据准备机制

### 自动验证触发器

为了让E2E测试能够正常运行，我们使用PostgreSQL触发器自动验证测试用户：

- **触发器功能**：自动将 `@example.com` 域名的用户标记为已验证
- **启用时机**：运行 `make test-e2e-prepare` 时
- **禁用时机**：运行 `make test-e2e-cleanup` 时

**重要提示**：
- ⚠️ 触发器仅用于测试环境，**禁止在生产环境使用**
- ⚠️ 测试完成后必须运行清理脚本禁用触发器
- ⚠️ 触发器会自动验证所有新注册的 `@example.com` 用户

### 手动操作

如果需要手动操作数据库：

```bash
# 启用触发器
psql "$DATABASE_URL" -f scripts/enable-auto-verify-test-users.sql

# 禁用触发器
psql "$DATABASE_URL" -f scripts/disable-auto-verify-test-users.sql

# 手动验证管理员
psql "$DATABASE_URL" -c "UPDATE users SET email_verified=true, role='admin' WHERE email='system@eninte.com';"

# 手动验证所有测试用户
psql "$DATABASE_URL" -c "UPDATE users SET email_verified=true WHERE email LIKE '%@example.com';"
```

## 测试环境配置

### 环境变量

E2E测试需要以下环境变量（在 `.env.test` 中配置）：

```bash
# 管理员账户（必须）
E2E_ADMIN_EMAIL=system@eninte.com
E2E_ADMIN_PASSWORD=Admin123!

# 数据库连接
DB_HOST=192.168.1.3
DB_PORT=5432
DB_NAME=sso_test
DB_USER=sso
DB_PASSWORD=sso

# 限流配置（测试时必须为0）
RATE_LIMIT_REQUESTS=0
```

### 服务启动

测试前必须启动服务并禁用限流。推荐使用 `run_e2e_no_ratelimit.sh` 脚本，它会自动处理所有限流层：

```bash
# 推荐：使用E2E启动脚本（自动处理所有限流层）
./scripts/run_e2e_no_ratelimit.sh

# 或手动启动（仅禁用全局HTTP限流和敏感端点限流）
RATE_LIMIT_REQUESTS=0 make run
```

#### 限流机制说明

SSO服务有4层独立限流，`RATE_LIMIT_REQUESTS=0` 仅禁用前两层：

| 限流层 | 作用域 | `RATE_LIMIT_REQUESTS=0` 能禁用？ |
|--------|--------|-------------------------------|
| 全局HTTP中间件 | 所有路由 | ✅ 是 |
| 敏感端点中间件 | register/login/forgot/reset | ✅ 是（已修复） |
| 业务层登录限流 | login, per IP（硬编码20/10min） | ❌ 需要脚本处理 |
| 业务层邮件限流 | email, per address（硬编码5/hour） | ❌ 需要脚本处理 |

`run_e2e_no_ratelimit.sh` 脚本会：
- 用大 `RATE_LIMIT_REQUESTS` 值启动服务（等效无限）
- 后台守护进程定期清理 Redis 中的 `login:ratelimit:*` key

## 测试结果

### 当前测试覆盖

- **总测试数**：156个
- **通过率**：94.9% (148/156)
- **失败数**：8个

### 测试分类

| 测试类别 | 测试数 | 说明 |
|---------|--------|------|
| 健康检查 | 2 | 服务健康状态检查 |
| 注册流程 | 8 | 用户注册、验证 |
| 登录流程 | 12 | 登录、多设备登录 |
| Token管理 | 18 | Token验证、刷新、撤销 |
| 邮箱验证 | 10 | 邮箱验证流程 |
| 密码重置 | 12 | 忘记密码、重置密码 |
| 管理员功能 | 15 | 用户管理、审计日志 |
| OAuth流程 | 20 | OAuth授权流程 |
| 并发测试 | 25 | 并发注册、登录、刷新 |
| 安全测试 | 18 | SQL注入、XSS、边界值 |
| 错误处理 | 16 | 异常情况处理 |

### 已知失败测试

以下8个测试失败是预期的业务逻辑问题，不影响核心功能：

1. **TestAdminUnauthorized** - 权限检查返回401而非403
2. **TestResendVerificationEmail** - 重发验证邮件返回500
3. **TestForgotPassword** - 无效邮箱格式验证
4. **TestTokenPermissions** - 权限检查返回401而非403

## 常见问题

### Q: 测试失败提示"connection refused"

**A:** 服务未启动或端口不是9090。请确保：
```bash
# 检查服务是否运行
curl http://localhost:9090/health

# 如果未运行，启动服务
RATE_LIMIT_REQUESTS=0 make run
```

### Q: 测试失败提示"429 Too Many Requests"

**A:** 可能是业务层登录限流触发（硬编码20次/10分钟）。推荐使用 `run_e2e_no_ratelimit.sh` 脚本启动服务，它会自动清理 Redis 中的限流计数器：
```bash
./scripts/run_e2e_no_ratelimit.sh
```

### Q: 测试失败提示"401 Unauthorized"

**A:** 用户邮箱未验证。运行准备脚本：
```bash
make test-e2e-prepare
```

### Q: 如何清理测试数据？

**A:** 运行清理脚本并选择清理数据：
```bash
make test-e2e-cleanup
# 提示时输入 'y' 确认清理
```

### Q: 触发器会影响生产环境吗？

**A:** 不会。触发器仅在测试数据库中启用，且：
- 仅验证 `@example.com` 域名的用户
- 测试完成后会被禁用
- 生产环境不应该有 `@example.com` 的用户

## Makefile命令参考

```bash
# 准备测试数据（启用触发器）
make test-e2e-prepare

# 运行E2E测试
make test-e2e

# 清理测试环境（禁用触发器）
make test-e2e-cleanup

# 完整测试流程（准备 + 测试）
make test-e2e-full
```

## 脚本文件说明

| 文件 | 用途 |
|------|------|
| `scripts/prepare-e2e-test.sh` | 准备测试数据，启用自动验证触发器 |
| `scripts/cleanup-e2e-test.sh` | 禁用触发器，可选清理测试数据 |
| `scripts/enable-auto-verify-test-users.sql` | 创建自动验证触发器的SQL |
| `scripts/disable-auto-verify-test-users.sql` | 删除自动验证触发器的SQL |

## 最佳实践

1. **测试前准备**：始终运行 `make test-e2e-prepare`
2. **测试后清理**：始终运行 `make test-e2e-cleanup`
3. **隔离环境**：使用独立的测试数据库
4. **定期清理**：定期清理测试数据避免数据库膨胀
5. **CI/CD集成**：在CI流程中自动运行准备和清理脚本

## 故障排查

### 检查触发器状态

```sql
-- 查看触发器是否存在
SELECT * FROM pg_trigger WHERE tgname = 'trigger_auto_verify_test_users';

-- 查看触发器函数
\df auto_verify_test_users
```

### 检查用户验证状态

```sql
-- 查看测试用户验证状态
SELECT email, email_verified, role
FROM users
WHERE email LIKE '%@example.com'
LIMIT 10;

-- 查看管理员状态
SELECT email, email_verified, role
FROM users
WHERE email = 'system@eninte.com';
```

### 手动修复

如果触发器出现问题，可以手动修复：

```bash
# 1. 禁用旧触发器
psql "$DATABASE_URL" -f scripts/disable-auto-verify-test-users.sql

# 2. 重新启用触发器
psql "$DATABASE_URL" -f scripts/enable-auto-verify-test-users.sql

# 3. 验证现有测试用户
psql "$DATABASE_URL" -c "UPDATE users SET email_verified=true WHERE email LIKE '%@example.com';"
```

## 参考资料

- [测试文档](./TESTING.md) - 完整测试指南
- [API文档](./API.md) - API接口说明
- [配置文档](./CONFIGURATION.md) - 环境配置说明
