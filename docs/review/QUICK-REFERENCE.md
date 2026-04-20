# 快速参考 - 新增功能使用指南

## 环境变量配置

### KEY_PATH_WHITELIST

**用途**: 自定义密钥文件允许的存储路径

**格式**: 逗号分隔的绝对路径列表

**默认值**: `/app/keys,/keys,/etc/sso/keys`

**示例**:

```bash
# 使用默认白名单
./sso

# 自定义单个路径
KEY_PATH_WHITELIST="/custom/keys" ./sso

# 自定义多个路径
KEY_PATH_WHITELIST="/custom/keys,/opt/sso/keys,/var/lib/sso/keys" ./sso

# Docker 环境
docker run -e KEY_PATH_WHITELIST="/app/keys,/custom/keys" sso:latest

# Kubernetes ConfigMap
apiVersion: v1
kind: ConfigMap
metadata:
  name: sso-config
data:
  KEY_PATH_WHITELIST: "/app/keys,/mnt/secrets/keys"
```

**注意事项**:
- 必须使用绝对路径
- 路径会自动清理（`filepath.Clean`）
- 无效路径会被忽略
- 如果所有自定义路径都无效，会回退到默认值

---

## 配置向导使用

### 首次启动

当服务检测到配置缺失时，会自动启动配置向导：

```bash
$ ./sso
[INFO] 配置文件不存在，启动配置向导...
[INFO] 配置向导运行在: http://localhost:8080
[INFO] 一次性访问令牌: abc123def456...
```

### 访问配置向导

1. 打开浏览器访问 `http://localhost:8080`
2. 输入一次性令牌（从控制台复制）
3. 按照向导步骤配置：
   - 数据库连接
   - Redis 缓存（可选）
   - 服务器设置
   - JWT 密钥
   - 安全配置
   - 邮件配置（可选）
   - Metrics 配置（可选）

### 测试连接

在保存配置前，可以测试各项连接：

- **测试数据库**: 点击"测试连接"按钮
- **测试 Redis**: 点击"测试连接"按钮
- **生成密钥**: 点击"生成 RSA 密钥对"按钮

### 保存配置

点击"保存配置并重启"后：
1. 配置写入 `.env` 文件（权限 0640）
2. 服务自动重启
3. 进入正常运行模式

---

## 初始化面板使用

### 访问初始化面板

服务正常启动后，访问 `/init` 路径：

```bash
http://localhost:9090/init
```

### 系统状态检查

初始化面板会显示：
- ✅ 数据库状态
- ✅ Redis 状态（或"已禁用"）
- ✅ 数据库迁移版本
- ✅ 服务版本和构建时间

### 创建管理员账户

1. 填写邮箱地址
2. 设置密码（至少 8 位，包含大小写字母和数字）
3. 确认密码
4. 点击"创建管理员"

**注意**: 只能创建一个管理员账户，创建后此功能自动禁用。

### 创建 OAuth 客户端

1. 填写客户端名称
2. 填写回调地址（必须是 http/https）
3. 点击"创建客户端"
4. **重要**: 保存显示的 Client ID 和 Client Secret

**注意**: Client Secret 只显示一次，请妥善保存。

---

## API 端点

### 配置向导 API

```
GET  /api/v1/setup/page          - 配置向导页面
POST /api/v1/setup/save          - 保存配置
POST /api/v1/setup/test-db       - 测试数据库连接
POST /api/v1/setup/test-redis    - 测试 Redis 连接
POST /api/v1/setup/generate-keys - 生成 RSA 密钥对
```

### 初始化面板 API

```
GET  /init                       - 初始化页面
GET  /api/v1/init/status         - 系统状态
POST /api/v1/init/admin          - 创建管理员
POST /api/v1/init/client         - 创建 OAuth 客户端
```

---

## 安全注意事项

### 配置向导

- ✅ 使用一次性令牌保护（32 字节随机）
- ✅ 限流保护（10 请求/分钟）
- ✅ 仅在配置缺失时启用
- ⚠️ 建议在内网环境下完成配置

### 初始化面板

- ✅ 检查管理员是否已存在
- ✅ 限流保护
- ✅ 审计日志记录
- ⚠️ 建议配置完成后立即创建管理员

### 密钥文件

- ✅ 私钥权限自动设置为 0600
- ✅ 公钥权限自动设置为 0644
- ✅ 路径白名单验证
- ✅ 符号链接检测
- ⚠️ 确保密钥文件所在目录权限正确

---

## 故障排查

### 配置向导无法访问

**问题**: 浏览器无法打开配置向导

**解决**:
1. 检查防火墙是否允许端口 8080
2. 确认服务正在运行
3. 检查日志中的访问令牌

### 数据库连接测试失败

**问题**: 测试数据库连接时报错

**解决**:
1. 检查数据库是否运行
2. 验证主机名和端口
3. 确认用户名和密码正确
4. 检查 SSL 模式设置

### 密钥生成失败

**问题**: 生成 RSA 密钥对时报错

**解决**:
1. 检查目标目录是否存在
2. 确认目录有写入权限
3. 验证路径在白名单内
4. 检查磁盘空间

### 管理员创建失败

**问题**: 创建管理员账户时报错

**解决**:
1. 检查邮箱格式是否正确
2. 确认密码符合要求（至少 8 位，包含大小写字母和数字）
3. 验证数据库连接正常
4. 检查是否已存在管理员账户

---

## 最佳实践

### 部署流程

1. **准备环境**
   ```bash
   # 创建密钥目录
   mkdir -p /app/keys
   chmod 700 /app/keys
   
   # 准备数据库
   createdb sso
   ```

2. **首次启动**
   ```bash
   # 启动服务（自动进入配置向导）
   ./sso
   ```

3. **完成配置**
   - 通过 Web 界面配置所有参数
   - 测试各项连接
   - 保存配置

4. **初始化系统**
   - 访问 `/init` 页面
   - 创建管理员账户
   - 创建 OAuth 客户端

5. **验证部署**
   ```bash
   # 检查服务状态
   curl http://localhost:9090/health
   
   # 检查 Metrics
   curl http://localhost:9090/metrics
   ```

### 生产环境建议

- ✅ 使用 `SERVER_ENV=production`
- ✅ 设置 `BCRYPT_COST >= 12`
- ✅ 启用 `DB_SSL_MODE=require`
- ✅ 配置 `MFA_RECOVERY_HMAC_KEY`
- ✅ 设置正确的 `CORS_ALLOWED_ORIGINS`
- ✅ 启用 Metrics Basic Auth
- ✅ 定期备份 `.env` 文件和密钥文件

### 安全检查清单

- [ ] 配置向导仅在内网访问
- [ ] 一次性令牌妥善保管
- [ ] 管理员密码足够强
- [ ] OAuth Client Secret 已保存
- [ ] 密钥文件权限正确
- [ ] .env 文件权限为 0640
- [ ] 数据库使用 SSL 连接
- [ ] Redis 设置密码（如果启用）
- [ ] 限流配置合理
- [ ] 审计日志正常记录

---

## 相关文档

- [AGENTS.md](../../AGENTS.md) - 项目开发规范
- [FIX-SUMMARY.md](./FIX-SUMMARY.md) - 修复总结
- [FINAL-REVIEW.md](./FINAL-REVIEW.md) - 最终审查报告
- [COMMIT-MESSAGE.txt](./COMMIT-MESSAGE.txt) - 提交消息

---

**文档版本**: 1.0  
**最后更新**: 2026-04-20  
**维护者**: SSO Team
