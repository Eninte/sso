# 生产环境部署检查清单

**版本**: 1.1
**更新日期**: 2026-08-02
**适用版本**: v1.x（包含所有安全修复）

> 本文档中的环境变量名、端口与 API 路径均已与 `internal/config/config.go` 和 `internal/app/router.go` 核对。
> 服务默认端口为 **9090**（`SERVER_PORT`）。

## 📋 部署前检查

### 1. 环境变量配置

#### 必须配置项 ✅

- [ ] `SERVER_ENV=production` - 生产环境标识
- [ ] `MFA_RECOVERY_HMAC_KEY` - MFA恢复码HMAC密钥（>= 32 字节，`LAN_DEPLOYMENT=true` 除外，未设置时生产环境拒绝启动）
- [ ] `BCRYPT_COST>=12` - 密码哈希成本（推荐14）
- [ ] `DB_SSL_MODE=require` - 数据库SSL连接（`LAN_DEPLOYMENT=true` 除外）
- [ ] `CORS_ALLOWED_ORIGINS` - CORS允许的域名（不能使用 `*`，不能包含 `localhost`，不能为默认值）
- [ ] `JWT_ISSUER` - JWT签发者标识（不能使用默认值 `sso`）
- [ ] `SMTP_HOST` - 生产SMTP服务器（不能为 `localhost`）
- [ ] `SMTP_PASSWORD` - 生产SMTP密码/授权码
- [ ] `JWT_PRIVATE_KEY_PATH` - JWT私钥路径
- [ ] `JWT_PUBLIC_KEY_PATH` - JWT公钥路径

#### 推荐配置项 ⚠️

- [ ] `RATE_LIMIT_REQUESTS=100` - 限流请求数（默认100/分钟）
- [ ] `RATE_LIMIT_WINDOW=1m` - 限流时间窗口
- [ ] `MAX_LOGIN_ATTEMPTS=5` - 最大登录尝试次数
- [ ] `LOCKOUT_DURATION=30m` - 账户锁定时长
- [ ] `JWT_ACCESS_TOKEN_TTL=15m` - Access Token有效期
- [ ] `JWT_REFRESH_TOKEN_TTL=168h` - Refresh Token有效期（7天）
- [ ] `REDIS_ENABLE=true` - 启用Redis缓存
- [ ] `REDIS_HOST` - Redis主机地址
- [ ] `REDIS_PORT=6379` - Redis端口
- [ ] `REDIS_PASSWORD` - Redis密码
- [ ] `METRICS_USERNAME` / `METRICS_PASSWORD` - Metrics端点Basic Auth凭据

> 密钥轮换（`KEY_ROTATION_ENABLED=true`）启用时，生产环境还必须设置
> `JWT_KEY_ENCRYPTION_KEY`（64 位 hex，`openssl rand -hex 32` 生成）。

### 2. 数据库检查

- [ ] 数据库已备份
- [ ] 数据库迁移已在测试环境验证
- [ ] 数据库连接池配置合理（应用侧 `DB_MAX_OPEN_CONNS`，参考值 25-100；PostgreSQL 服务端 `max_connections` 必须大于应用连接池上限 × 实例数）
- [ ] 数据库SSL证书已配置
- [ ] 数据库用户权限最小化

### 3. 密钥管理

- [ ] JWT密钥对已生成（RSA 2048位或更高）
- [ ] MFA恢复码HMAC密钥已生成（32字节随机）
- [ ] 所有密钥已安全存储（不在代码仓库中）
- [ ] 密钥轮换计划已制定

### 4. 网络配置

- [ ] HTTPS已启用（TLS 1.2+）
- [ ] 防火墙规则已配置
- [ ] 负载均衡器已配置
- [ ] CDN已配置（如需要）
- [ ] DNS记录已配置

### 5. 监控和日志

- [ ] 应用日志已配置
- [ ] 审计日志已启用
- [ ] 错误监控已配置（如Sentry）
- [ ] 性能监控已配置（如Prometheus）
- [ ] 告警规则已配置

## 🚀 部署步骤

### 步骤1: 数据库迁移

```bash
# 1. 备份数据库
pg_dump -U sso -d sso > backup_$(date +%Y%m%d_%H%M%S).sql

# 2. 执行迁移
export DATABASE_URL='postgres://sso:YOUR_PASSWORD@DB_HOST:5432/sso?sslmode=require'
make migrate-up

# 3. 验证迁移版本（以 migrations/ 目录中最大序号为准）
psql -U sso -d sso -c "SELECT version FROM schema_migrations ORDER BY version DESC LIMIT 1;"
```

> Docker 部署时容器 entrypoint 默认自动执行迁移（`AUTO_MIGRATE=true`），
> 通常无需手动执行本步骤；如需关闭自动迁移，设置 `AUTO_MIGRATE=false`。

### 步骤2: 配置验证

```bash
# 1. 运行生产环境检查脚本（校验 MFA密钥/BCRYPT_COST/DB_SSL_MODE/CORS 等必检项）
./scripts/check_production_env.sh

# 2. 测试数据库连接
psql "$DATABASE_URL" -c "SELECT 1;"

# 3. 测试Redis连接（如启用）
redis-cli -h "$REDIS_HOST" -p "$REDIS_PORT" -a "$REDIS_PASSWORD" --no-auth-warning ping
```

### 步骤3: 构建和部署

```bash
# 1. 构建生产版本
make build

# 2. 运行测试
make test

# 3. 运行安全测试
make test-security

# 4. 部署到生产环境
# （根据你的部署方式：Docker、K8s、直接部署等）
```

### 步骤4: 启动服务

```bash
# 1. 启动服务
./bin/sso

# 2. 检查服务状态
curl -f http://localhost:9090/health || exit 1

# 3. 检查日志（systemd 部署）
journalctl -u sso -f
```

### 步骤5: 验证部署

```bash
# 1. 健康检查
curl http://localhost:9090/health

# 2. 测试注册
curl -X POST http://localhost:9090/api/v1/register \
  -H "Content-Type: application/json" \
  -d '{"email":"test@example.com","password":"Test123456!","username":"testuser"}'

# 3. 测试登录
curl -X POST http://localhost:9090/api/v1/login \
  -H "Content-Type: application/json" \
  -d '{"email":"test@example.com","password":"Test123456!"}'

# 4. 测试JWT验证（使用上一步返回的 access_token）
curl -X GET http://localhost:9090/api/v1/userinfo \
  -H "Authorization: Bearer YOUR_ACCESS_TOKEN"
```

## 🔒 安全配置检查

以下检查项已由 `scripts/check_production_env.sh` 自动化覆盖（MFA 密钥、bcrypt 成本、
数据库 SSL、CORS 等），推荐直接运行脚本；如需手动核对，可参考以下要点：

### 1. MFA恢复码配置

- `MFA_RECOVERY_HMAC_KEY` 必须已设置且长度 >= 32 字节
- 生产环境未满足时服务会拒绝启动（`LAN_DEPLOYMENT=true` 除外）

### 2. CORS配置检查

- 生产环境禁止 `CORS_ALLOWED_ORIGINS=*`
- 生产环境禁止包含 `localhost` / `127.0.0.1`

### 3. bcrypt成本检查

- 生产环境 `BCRYPT_COST` 必须 >= 12，且 <= 31（bcrypt 算法上限）

### 4. 数据库SSL检查

- 生产环境 `DB_SSL_MODE` 必须为 `require` 或更高（`LAN_DEPLOYMENT=true` 除外）

## 📊 监控指标

### 关键指标

| 指标 | 阈值 | 告警级别 |
|------|------|---------|
| 登录成功率 | >95% | 警告 |
| JWT验证延迟 | <2ms | 警告 |
| MFA验证延迟 | <1ms | 警告 |
| 数据库连接池使用率 | <80% | 警告 |
| Redis缓存命中率 | >90% | 信息 |
| 邮件发送限流率 | <10% | 信息 |
| 账户锁定率 | <5% | 警告 |
| 审计日志失败率 | <0.1% | 严重 |

### 监控命令

```bash
# 1. 检查服务状态
systemctl status sso

# 2. 检查日志
journalctl -u sso -f

# 3. 检查数据库连接
psql -U sso -d sso -c "SELECT count(*) FROM pg_stat_activity WHERE datname='sso';"

# 4. 检查Redis连接
redis-cli -h $REDIS_HOST -p $REDIS_PORT -a "$REDIS_PASSWORD" --no-auth-warning ping

# 5. 检查内存使用
ps aux | grep sso | awk '{print $6}'
```

## 🔄 回滚计划

### 回滚步骤

```bash
# 1. 停止服务
systemctl stop sso

# 2. 回滚数据库迁移（⚠️ 仅回滚到指定版本，切勿使用 make migrate-down——
#    它会回滚全部迁移并清空业务数据）
migrate -path ./migrations -database "$DATABASE_URL" goto <上一个稳定版本号>

# 3. 恢复旧版本
cp /backup/sso-old /usr/local/bin/sso

# 4. 启动服务
systemctl start sso

# 5. 验证服务
curl http://localhost:9090/health
```

### 回滚决策标准

立即回滚如果：
- [ ] 服务无法启动
- [ ] 数据库迁移失败
- [ ] 登录成功率<50%
- [ ] 严重安全漏洞
- [ ] 数据丢失或损坏

考虑回滚如果：
- [ ] 性能下降>50%
- [ ] 登录成功率<80%
- [ ] 大量用户投诉
- [ ] 关键功能不可用

## ✅ 部署后验证

### 功能验证

- [ ] 用户注册功能正常
- [ ] 用户登录功能正常
- [ ] JWT验证功能正常
- [ ] MFA功能正常
- [ ] 密码重置功能正常
- [ ] 邮件发送功能正常
- [ ] OAuth登录功能正常（如启用）
- [ ] 管理员功能正常

### 安全验证

- [ ] JWT重放攻击防护生效
- [ ] 登录失败计数器正常工作
- [ ] 账户锁定机制正常工作
- [ ] 邮件限流正常工作
- [ ] CORS配置正确
- [ ] SQL注入防护生效
- [ ] 审计日志正常记录

### 性能验证

- [ ] 响应时间<100ms（P95）
- [ ] JWT验证<2ms
- [ ] MFA验证<1ms
- [ ] 数据库查询<10ms
- [ ] 缓存命中率>90%

## 📞 紧急联系方式

- **技术负责人**: [姓名] - [电话]
- **运维负责人**: [姓名] - [电话]
- **安全负责人**: [姓名] - [电话]
- **DBA**: [姓名] - [电话]

## 📚 相关文档

- `docs/DEPLOYMENT.md` - 详细部署指南
- `docs/CONFIGURATION.md` - 配置说明
- `docs/SECURITY.md` - 安全特性说明
- `docs/SECURITY_FIX_PLAN.md` - 安全修复计划
- `scripts/check_production_env.sh` - 生产环境配置自动检查脚本

---

**检查清单版本**: 1.1
**最后更新**: 2026-08-02
**下次审查**: 2026-09-02
