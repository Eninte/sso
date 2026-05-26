# 生产环境部署检查清单

**版本**: 1.0  
**更新日期**: 2026-05-26  
**适用版本**: v1.x（包含所有安全修复）

## 📋 部署前检查

### 1. 环境变量配置

#### 必须配置项 ✅

- [ ] `SERVER_ENV=production` - 生产环境标识
- [ ] `MFA_RECOVERY_HMAC_KEY` - MFA恢复码HMAC密钥（32字节）
- [ ] `BCRYPT_COST>=12` - 密码哈希成本（推荐14）
- [ ] `DB_SSL_MODE=require` - 数据库SSL连接
- [ ] `CORS_ALLOWED_ORIGINS` - CORS允许的域名（不能使用*）
- [ ] `SMTP_HOST` - 生产SMTP服务器
- [ ] `SMTP_PASSWORD` - 生产SMTP密码/授权码
- [ ] `JWT_PRIVATE_KEY_PATH` - JWT私钥路径
- [ ] `JWT_PUBLIC_KEY_PATH` - JWT公钥路径

#### 推荐配置项 ⚠️

- [ ] `RATE_LIMIT_REQUESTS=100` - 限流请求数（默认100/分钟）
- [ ] `RATE_LIMIT_WINDOW=60s` - 限流时间窗口
- [ ] `MAX_LOGIN_ATTEMPTS=5` - 最大登录尝试次数
- [ ] `ACCOUNT_LOCKOUT_DURATION=30m` - 账户锁定时长
- [ ] `ACCESS_TOKEN_TTL=15m` - Access Token有效期
- [ ] `REFRESH_TOKEN_TTL=168h` - Refresh Token有效期（7天）
- [ ] `REDIS_ENABLE=true` - 启用Redis缓存
- [ ] `REDIS_HOST` - Redis主机地址
- [ ] `REDIS_PORT=6379` - Redis端口
- [ ] `REDIS_PASSWORD` - Redis密码

### 2. 数据库检查

- [ ] 数据库已备份
- [ ] 数据库迁移已在测试环境验证
- [ ] 数据库连接池配置合理（推荐: max_connections=25）
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
make migrate-up

# 3. 验证迁移
psql -U sso -d sso -c "SELECT version FROM schema_migrations ORDER BY version DESC LIMIT 1;"

# 预期输出: 012
```

### 步骤2: 配置验证

```bash
# 1. 检查环境变量
./scripts/check_production_env.sh

# 2. 验证配置文件
./scripts/validate_config.sh

# 3. 测试数据库连接
./scripts/test_db_connection.sh

# 4. 测试Redis连接（如启用）
./scripts/test_redis_connection.sh
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
curl -f http://localhost:8080/health || exit 1

# 3. 检查日志
tail -f /var/log/sso/app.log
```

### 步骤5: 验证部署

```bash
# 1. 健康检查
curl http://localhost:8080/health

# 2. 测试注册
curl -X POST http://localhost:8080/api/v1/auth/register \
  -H "Content-Type: application/json" \
  -d '{"email":"test@example.com","password":"Test123456!"}'

# 3. 测试登录
curl -X POST http://localhost:8080/api/v1/auth/login \
  -H "Content-Type: application/json" \
  -d '{"email":"test@example.com","password":"Test123456!"}'

# 4. 测试JWT验证
curl -X GET http://localhost:8080/api/v1/user/profile \
  -H "Authorization: Bearer YOUR_ACCESS_TOKEN"
```

## 🔒 安全配置检查

### 1. MFA恢复码配置

```bash
# 验证HMAC密钥已设置
if [ -z "$MFA_RECOVERY_HMAC_KEY" ]; then
  echo "❌ MFA_RECOVERY_HMAC_KEY未设置"
  exit 1
fi

# 验证密钥长度
if [ ${#MFA_RECOVERY_HMAC_KEY} -lt 32 ]; then
  echo "❌ MFA_RECOVERY_HMAC_KEY长度不足32字节"
  exit 1
fi

echo "✅ MFA恢复码配置正确"
```

### 2. CORS配置检查

```bash
# 验证CORS不使用通配符
if [[ "$CORS_ALLOWED_ORIGINS" == "*" ]]; then
  echo "❌ 生产环境禁止使用CORS通配符"
  exit 1
fi

# 验证不包含localhost
if [[ "$CORS_ALLOWED_ORIGINS" == *"localhost"* ]]; then
  echo "❌ 生产环境禁止使用localhost"
  exit 1
fi

echo "✅ CORS配置正确"
```

### 3. bcrypt成本检查

```bash
# 验证bcrypt成本
if [ "$BCRYPT_COST" -lt 12 ]; then
  echo "❌ 生产环境BCRYPT_COST必须>=12"
  exit 1
fi

echo "✅ bcrypt成本配置正确"
```

### 4. 数据库SSL检查

```bash
# 验证数据库SSL
if [ "$DB_SSL_MODE" != "require" ]; then
  echo "❌ 生产环境必须启用数据库SSL"
  exit 1
fi

echo "✅ 数据库SSL配置正确"
```

## 🔧 JWT JTI跟踪配置

### 代码配置

在`cmd/server/main.go`中添加：

```go
// 创建缓存实例
cache, err := cache.NewCache(&cache.Option{
    RedisEnable:   cfg.RedisEnable,
    RedisHost:     cfg.RedisHost,
    RedisPort:     cfg.RedisPort,
    RedisPassword: cfg.RedisPassword,
    RedisDB:       cfg.RedisDB,
})
if err != nil {
    log.Fatal("创建缓存失败:", err)
}

// 创建JWT服务
jwtSvc := crypto.NewJWTService(
    privateKey,
    publicKey,
    cfg.JWTIssuer,
    cfg.AccessTokenTTL,
    cfg.RefreshTokenTTL,
)

// 配置JTI跟踪器（防止JWT重放攻击）
jtiTracker := crypto.NewCacheJTITracker(cache, "jti:")
jwtSvc.SetJTITracker(jtiTracker)
log.Info("JWT JTI跟踪已启用")
```

### 验证JTI跟踪

```bash
# 1. 生成token
TOKEN=$(curl -X POST http://localhost:8080/api/v1/auth/login \
  -H "Content-Type: application/json" \
  -d '{"email":"test@example.com","password":"Test123456!"}' \
  | jq -r '.access_token')

# 2. 第一次使用token（应该成功）
curl -X GET http://localhost:8080/api/v1/user/profile \
  -H "Authorization: Bearer $TOKEN"

# 3. 第二次使用同一个token（应该失败）
curl -X GET http://localhost:8080/api/v1/user/profile \
  -H "Authorization: Bearer $TOKEN"

# 预期: 第二次请求返回401 Unauthorized
```

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
redis-cli -h $REDIS_HOST -p $REDIS_PORT ping

# 5. 检查内存使用
ps aux | grep sso | awk '{print $6}'
```

## 🔄 回滚计划

### 回滚步骤

```bash
# 1. 停止服务
systemctl stop sso

# 2. 回滚数据库迁移
make migrate-down

# 3. 恢复旧版本
cp /backup/sso-old /usr/local/bin/sso

# 4. 启动服务
systemctl start sso

# 5. 验证服务
curl http://localhost:8080/health
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
- `docs/SECURITY_FIX_COMPLETE_REPORT.md` - 安全修复报告
- `docs/TROUBLESHOOTING.md` - 故障排查指南

---

**检查清单版本**: 1.0  
**最后更新**: 2026-05-26  
**下次审查**: 2026-06-26
