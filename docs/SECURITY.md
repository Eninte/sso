# 安全策略

## 支持的版本

| 版本 | 支持状态 |
|------|----------|
| 1.0.x | ✅ 支持 |
| < 1.0 | ❌ 不支持 |

## 报告漏洞

如果您发现安全漏洞，请**不要**通过公开Issue报告。

### 报告流程

1. **发送邮件**：ocean@eninte.com
2. **加密通信**：可使用PGP加密（公钥见下方）
3. **提供详情**：
   - 漏洞描述
   - 复现步骤
   - 影响范围
   - 建议修复方案（如有）

### 响应时间

- **确认收到**：24小时内
- **初步评估**：72小时内
- **修复计划**：7天内
- **修复发布**：根据严重程度，30天内

### 漏洞严重程度

| 级别 | 说明 | 响应时间 |
|------|------|----------|
| 严重 | 远程代码执行、SQL注入、认证绕过 | 7天 |
| 高 | 权限提升、敏感数据泄露 | 14天 |
| 中 | XSS、CSRF、信息泄露 | 30天 |
| 低 | 配置问题、日志泄露 | 60天 |

## 安全特性

### 认证安全

- **密码哈希**：bcrypt（cost=12-14）
- **密码策略**：最小8位，包含大小写字母和数字
- **登录锁定**：5次失败后锁定30分钟
- **MFA支持**：TOTP双因素认证
- **MFA防暴力**：恢复码失败限流（5次锁15分钟）与 TOTP 重放记录均存储于 Redis（键 `mfa:recovery:attempts:{userID}`、`mfa:totp:used:{userID}:{timeStep}`），多副本部署下一致生效；每个时间步独立记录，90 秒窗口内旧码不可二次使用；Redis 故障时降级为进程内存并输出 Error 日志
- **社交登录防 Login CSRF**：OAuth state 始终由服务端生成（crypto/rand 32 字节），发起端下发 `HttpOnly + SameSite=Lax`（生产 `Secure`）的 state HMAC 指纹 Cookie，回调端恒定时间比较校验，无 Cookie 或指纹不匹配一律拒绝；纯 API 客户端可通过 `SOCIAL_STATE_COOKIE_BINDING=false` 恢复旧行为
- **Metrics端点**：支持Basic Auth认证保护

### Token安全

- **签名算法**：RS256（RSA + SHA256）
- **Access Token**：有效期15分钟
- **Refresh Token**：有效期7天，支持轮换
- **Token撤销**：支持即时撤销
- **明文不落库**：tokens 表与邮箱验证/密码重置令牌仅存储 SHA-256 哈希，查询与轮换均按哈希匹配，明文不出现在数据库、日志与 Redis 缓存键中（token 缓存键同样使用哈希）
- **轮换私钥信封加密**：启用密钥轮换（DB 密钥存储）时，私钥以 AES-256-GCM 加密落库（密文格式 `v1:gcm:`），KEK 由 `JWT_KEY_ENCRYPTION_KEY`（64 位 hex，生产启用轮换时必填）提供；存量明文私钥在加载时自动懒加密回写，无需迁移脚本
- **角色变更生效时效**：管理员修改/撤销用户角色时，该用户的全部 refresh token 与会话立即撤销；已签发的 access token 无法召回，最迟在其剩余有效期（≤15 分钟）内自然失效后新角色完全生效
- **管理员操作防护**：admin 不能修改或删除自己的角色（403）；当目标用户是系统最后一个 active admin 时，降级/禁用/删除均被拒绝（409），防止管理面锁死

### 传输安全

- **HTTPS**：生产环境必须使用
- **HSTS**：强制HTTPS
- **安全头**：CSP（含随机 nonce）、X-Frame-Options等
- **CORS凭据收紧**：仅当 Origin 精确匹配允许列表中的具体源时才发送 `Access-Control-Allow-Credentials: true`；命中通配形式（`*` 或 `*.suffix`）时允许跨域但不发送凭据头；所有响应携带 `Vary: Origin`；生产环境禁止 `*` 通配

### 配置安全校验

- **启动即校验**：生产环境配置缺陷直接拒绝启动（fail-fast），包括 CORS 通配/localhost、JWT Issuer 默认值、SMTP localhost、`BCRYPT_COST < 12`、数据库 SSL 等
- **MFA恢复码密钥**：生产环境 `MFA_RECOVERY_HMAC_KEY` 必填且 ≥32 字节（不足拒绝启动）
- **环境白名单**：`SERVER_ENV` 仅接受 `development` / `production` / `test`，其他值拒绝启动，防止拼写错误导致生产校验被旁路
- **密钥加密密钥**：`JWT_KEY_ENCRYPTION_KEY` 配置即须为 64 位 hex（所有环境 fail-fast）

### 限流与故障降级

- **分布式限流**：登录/注册/忘记密码/重置密码/MFA 验证及 `/api/v1/token` 凭据交换端点使用全局限额 1/10 的独立限流器（Redis 滑动窗口）；敏感限流未启用时回退全局限流，端点始终受保护
- **故障降级**：限流 Redis 故障时当次请求降级为进程内内存限流（同限额同窗口），降级期间限额仍然生效，不做无限放行；每次降级计数 `security_ratelimit_error_total` 指标，Error 日志按限流器实例每分钟最多一条防止刷日志
- **多副本说明**：降级期间内存限额在各副本独立计数，实际限额放宽为副本数倍，属可用性与安全的既定折中
- **验证码双维度触发**：登录失败按来源 IP 与账号（邮箱归一化后 SHA-256 作键）双维度计数，任一达阈值即要求验证码，换 IP 爆破同一账号不再绕过；登录成功双维度清零

### 数据安全

- **敏感数据**：加密存储
- **日志脱敏**：邮箱地址自动脱敏（`u***@example.com`），Token 仅记录前8位
- **SQL注入防护**：参数化查询 + 表名白名单

## 安全配置建议

### 生产环境

```bash
# 必须配置
SERVER_ENV=production
DB_SSL_MODE=require
BCRYPT_COST=14
MFA_RECOVERY_HMAC_KEY=<32字节以上强随机密钥>
JWT_KEY_ENCRYPTION_KEY=<64位hex，启用密钥轮换时必填>

# 建议配置
RATE_LIMIT_REQUESTS=50
RATE_LIMIT_WINDOW=1m
MAX_LOGIN_ATTEMPTS=3
LOCKOUT_DURATION=1h
METRICS_USERNAME=<your-username>   # Metrics Basic Auth
METRICS_PASSWORD=<strong-password>
```

### 密钥管理

- RSA私钥文件权限设置为600
- 定期轮换密钥（建议每年）
- 使用环境变量或密钥管理服务存储敏感配置
- 启用 DB 密钥轮换时，私钥经 AES-256-GCM 信封加密后落库（KEK 为 `JWT_KEY_ENCRYPTION_KEY`，使用 `openssl rand -hex 32` 生成）

### 数据库安全

- 使用强密码
- 限制数据库访问IP
- 启用SSL连接
- 定期备份

## 安全审计

### 自动化检查

```bash
# 运行安全检查
make test-security

# 包含：
# - go vet 静态分析
# - govulncheck 漏洞扫描
# - golangci-lint 安全规则
```

### 手动审计清单

- [ ] 输入验证完整
- [ ] 错误处理不泄露敏感信息
- [ ] 认证和授权正确实现
- [ ] 敏感数据加密存储
- [ ] 日志不包含敏感信息
- [ ] SQL查询参数化
- [ ] CORS配置正确
- [ ] 安全头已设置

## 已知安全考虑

### 风险缓解

| 风险 | 缓解措施 |
|------|----------|
| 暴力破解 | 登录锁定、Rate Limiting |
| Token窃取 | 短有效期、HTTPS、Token轮换 |
| CSRF | SameSite Cookie、CSRF Token |
| XSS | CSP头、输入输出编码 |
| SQL注入 | 参数化查询 |

## 安全更新

安全更新将通过以下渠道发布：

- GitHub Security Advisories
- 项目CHANGELOG
- 邮件通知（订阅者）

## PGP公钥

```
-----BEGIN PGP PUBLIC KEY BLOCK-----
[在此添加PGP公钥]
-----END PGP PUBLIC KEY BLOCK-----
```

## 联系方式

- 安全邮箱：ocean@eninte.com
- 响应时间：工作日24小时内
