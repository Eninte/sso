# SSO 服务安全审查报告

- **审查日期**：2026-07-21
- **审查对象**：`github.com/example/sso`（commit `c39a54b`，main 分支）
- **审查方式**：人工源码审查（3 个域并行：密码学/MFA、HTTP/OAuth、数据层/配置/日志）+ 自动化扫描（gosec v2.28.0、govulncheck v1.1.4、golangci-lint）
- **审查基准**：AGENTS.md 安全规范、OWASP ASVS 思路、RFC 6749/7009/7636、OIDC Core

## 总体结论

项目安全成熟度**高于同类自研认证服务的平均水平**：参数化查询与标识符白名单完整、OAuth 核心流程（redirect_uri 精确匹配、PKCE 强制 S256、授权码原子一次性、consent_token 绑定）实现正确、Refresh Token 轮换与重放检测为事务原子实现、日志脱敏体系系统完整、生产配置校验严格。**未发现 Critical 级问题。**

最突出的残余风险集中在一个主题——**数据库静态数据中的敏感值明文**（H1/H2/H3）：会话令牌、密码重置令牌、JWT 签名私钥三类凭据在 DB 中明文存储，构成"DB 一旦泄露即全线失守"的纵深防御缺口。三者均非代码缺陷而是设计取舍，但 H1/H2 已有 hash 基础设施，修复成本低、收益大。

自动化扫描：gosec **0 issues**（含 CI 豁免 G118/G201/G202/G706/G710，豁免理由经复核均成立）；govulncheck **代码路径 0 漏洞**（golang.org/x/crypto v0.49.0 存在已知漏洞但代码未触达，建议升级，见 L15）。

## 发现汇总

| # | 级别 | 问题 | 位置 |
|---|------|------|------|
| H1 | High | Access/Refresh Token 明文落库 | `internal/store/postgres/token.go:102-134` |
| H2 | High | 密码重置/邮箱验证令牌明文落库 | `internal/store/postgres/verification.go:44-49` |
| H3 | High | JWT 轮换私钥明文入库 | `internal/crypto/jwt.go:445-458`、`internal/store/postgres/key.go:19-36` |
| M1 | Medium | CORS Origin 反射 + 恒定 `Allow-Credentials: true` | `internal/middleware/cors.go:67-105` |
| M2 | Medium | 社交登录 state 未绑定发起方会话（login CSRF） | `internal/handler/social.go:43`、`internal/service/social.go:232-303` |
| M3 | Medium | MFA 限流与 TOTP 重放记录为单机内存，多副本失效 | `internal/service/mfa.go:127-211` |
| M4 | Medium | 登录/邮件/分布式限流 Redis 故障时 fail-open（含敏感端点） | `internal/middleware/ratelimit_distributed.go:96-165`、`internal/service/login_ratelimit.go:62-65` |
| M5 | Medium | 配置向导泄露原始 DB 错误（日志未脱敏 + 详情入响应） | `internal/handler/setup.go:383-386` |
| M6 | Medium | CI/构建工具链使用 `@latest` 未固定版本 | `.github/workflows/ci.yml`、`docker/Dockerfile:25` |
| L1-L15 | Low | 见下文明细 | — |
| I1-I6 | Info | 见下文明细 | — |

---

## High 级发现

### H1. Access/Refresh Token 明文落库

- **位置**：`internal/store/postgres/token.go:102-134`（StoreToken）、`226-299`（RotateRefreshToken）
- **证据**：INSERT 同时写入 `access_token`/`refresh_token` 明文与 `access_token_hash`/`refresh_token_hash`，注释明确"明文保留用于调试和审计"。迁移 018 已引入 hash 列且查询优先走 hash。
- **利用场景**：数据库泄露（拖库/备份外泄/注入读取）→ 攻击者直接获得全部未过期会话凭据，hash 列的保护被完全抵消；撤销机制无法防御已泄露的有效凭据。
- **修复建议**：新迁移将明文列置 NULL（后续版本 DROP COLUMN），删除明文回退查询路径（`getTokenByField(ctx, "refresh_token", ...)`），仅保留 hash 存储与查询。

### H2. 密码重置/邮箱验证令牌明文落库

- **位置**：`internal/store/postgres/verification.go:44-49`；生成点 `internal/service/user.go:232-245`
- **证据**：`common.GenerateToken()` 生成 32 字节随机令牌后明文写入 `reset_tokens.token` / `verification_tokens.token`，同时明文拼入邮件链接。
- **利用场景**：任何 DB 读权限泄露 → 在 1 小时 TTL 内直接使用泄露令牌重置任意账户密码（重置令牌等效一次性账户接管凭据）。
- **修复建议**：改存 SHA-256 hash（令牌本身高熵无需加盐），校验时 hash 后比对；邮件链接仍用原始令牌。

### H3. JWT 轮换私钥明文入库

- **位置**：`internal/crypto/jwt.go:445-458`（CreateKeyVersion）、`internal/service/keyrotation.go:55`（StoreKey）、`internal/store/postgres/key.go:19-36`
- **证据**：密钥轮换模式下新生成的 RSA 私钥以 PKCS8 PEM 明文写入 `key_versions.private_key`，无信封加密/KMS 保护。与文件模式"私钥权限必须 0600"的要求形成明显安全等级落差。
- **缓解因素**：本服务 `ValidateToken` 验签后还查库确认 token 记录存在（`auth_token.go:391-428`），伪造 token 不在库中会被拒绝——但**仅验签的下游服务/JWKS 信任方会完全沦陷**。
- **利用场景**：DB 读权限泄露 → 攻击者获得活跃私钥，可任意签发合法 JWT。
- **修复建议**：私钥落库前用 KEK（环境变量或 KMS）做 AES-GCM 信封加密；或轮换模式仅公钥入库、私钥走文件/secret 挂载。

---

## Medium 级发现

### M1. CORS Origin 反射 + 恒定 Allow-Credentials

- **位置**：`internal/middleware/cors.go:67-105`
- **证据**：Origin 命中允许列表即反射回显且恒定设置 `Access-Control-Allow-Credentials: true`；`Validate()` 仅在 `env == "production"` 时禁止 `*`；`*.example.com` 子域通配（HasSuffix）与 credentials 组合时，任一（可能被攻陷的）子域均可带凭据跨域。另未设置 `Vary: Origin`。
- **利用场景**：非 production 的 `SERVER_ENV` 暴露公网（配置失误）时，恶意站点可携带用户凭据读取 `/api/v1/userinfo` 等响应。
- **修复建议**：仅精确匹配的 Origin 发送 credentials；任何通配形式（含 `*.` 子域）不发送；补 `Vary: Origin`。

### M2. 社交登录 state 未绑定发起方会话

- **位置**：`internal/handler/social.go:43`、`internal/service/social.go:232-303, 375-427`
- **证据**：state 可由客户端任意传入，服务端仅校验存在性、一次性（GETDEL）、provider 绑定与 TTL，**不校验该 state 是否由当前浏览器发起**。
- **利用场景**：典型 login CSRF——攻击者用自己的 Google/GitHub 账号获得合法 code + 自建 state，诱导受害者访问回调 URL，受害者被登录为攻击者账号。当前 token 以 JSON 返回，实际影响取决于前端接管方式。
- **修复建议**：state 强制服务端生成，与发起方指纹绑定（双重提交 cookie 校验）；文档明确前端集成的 login CSRF 注意事项。

### M3. MFA 防护为单机内存实现

- **位置**：`internal/service/mfa.go:127-171`（recoveryAttempts）、`181-211`（totpUsage）
- **证据**：恢复码失败限流（5 次锁 15 分钟）与 TOTP 重放记录存进程内 map，而登录限流已用 Redis 分布式实现。多副本部署时尝试次数随副本数线性放大；TOTP 重放记录不跨实例，重放请求打到另一实例即绕过。
- **修复建议**：迁移至 Redis（`INCR`+TTL / `SET NX EX 90`），与 LoginRateLimiter 保持一致。

### M4. 限流 Redis 故障时 fail-open（含敏感端点）

- **位置**：`internal/middleware/ratelimit_distributed.go:96-165`、`internal/service/login_ratelimit.go:62-65`、`email_ratelimit.go:56-59`
- **证据**：Redis 错误时直接放行（分布式中间件有日志+指标，登录/邮件限流器静默放行）。登录/注册/忘记密码/重置密码/MFA 验证等敏感端点同样 fail-open。
- **利用场景**：Redis 故障窗口内（或攻击者诱发故障）对登录/重置密码无限制爆破。账户锁定（DB 层）是唯一兜底。
- **修复建议**：敏感端点 Redis 故障时 fail-closed（503）或降级为进程内限流；所有 fail-open 路径必须有 Error 级日志 + metrics。

### M5. 配置向导泄露原始 DB 错误

- **位置**：`internal/handler/setup.go:383-386`
- **证据**：`slog.Error("setup test-db 数据库连接失败", "error", err)` 未走 `logging.SanitizeDBURL`（项目其他 20+ 处已做脱敏，此处遗漏）；且 `apperrors.ErrInternal.WithDetails(err.Error())` 把内部错误详情返回 HTTP 客户端，违反项目错误处理规范。
- **利用场景**：向导开启期间，本地调用者可借连接测试接口探测内网 DB 拓扑；若驱动错误串含 DSN 片段则泄露凭据。
- **修复建议**：日志经 `SanitizeDBURL` 脱敏，响应返回通用错误消息。

### M6. CI/构建工具链未固定版本

- **位置**：`.github/workflows/ci.yml:360-369`、`docker/Dockerfile:25`
- **证据**：`go install gotest.tools/gotestsum@latest`、`migrate@latest` 完全不固定版本；`actions/checkout@v5` 等仅固定 major tag（gosec@v2.28.0、govulncheck@v1.1.4 已固定，值得肯定）。
- **利用场景**：上游仓库/模块被投毒时，CI 在带 secrets（docker job）的环境执行恶意代码。
- **修复建议**：action 固定到 commit SHA（配 Dependabot）；工具安装固定具体版本。

---

## Low 级发现

| # | 问题 | 位置 | 要点 |
|---|------|------|------|
| L1 | TOTP 重放记录每用户仅存一条 | `internal/service/mfa_setup.go:163-177`、`mfa.go:202-211` | 90 秒窗口内两次验证后旧码可被重放一次；改为小集合或 Redis SET |
| L2 | `MFA_RECOVERY_HMAC_KEY` 无强度校验 | `internal/config/config.go:489-501` | 仅查非空；弱值使恢复码 HMAC 形同虚设；建议 ≥32 字节校验 |
| L3 | 文件模式 kid 每次重启随机生成 | `internal/crypto/keyloader.go:269-299` | 重启后全部在线用户强制登出，轮换公钥机制失效；建议 kid 从密钥内容派生（RFC 7638 thumbprint）。fail-closed，属可用性缺陷 |
| L4 | `/api/v1/token` 未纳入敏感限流 | `internal/app/router.go:164` | client_secret 可以 100 次/分/IP 在线猜测；建议纳入 sensitive 子路由 |
| L5 | 无自锁/末位管理员防护；禁用用户 token 撤销失败仅告警 | `internal/handler/admin.go:183-213`、`internal/service/admin.go:167-187` | 可将全部 admin 禁用致管理面锁死；RevokeAllUserTokens 失败窗口内被禁用户 token 可用至过期（≤15 分钟） |
| L6 | `RequireAdmin` 仅信任 JWT role claim | `internal/middleware/auth.go:179-222` | 角色降级在 token 剩余有效期内（≤15 分钟）不生效；可接受但需文档声明 |
| L7 | 验证码为数学题且按 IP 计数 | `internal/captcha/captcha.go:126`、`internal/handler/helpers.go:50-77` | 脚本可解析算术题；IP 轮换可永久低于触发阈值；建议账号维度计数并行 |
| L8 | 邮件日志记录完整收件人邮箱 | `internal/service/email.go:138,142` | 与全项目 PII 脱敏策略不一致；改用 `SanitizeEmail` |
| L9 | 容器内密码出现在进程命令行/环境 | `docker/entrypoint.sh:36,42`、`docker-compose.yml:167` | migrate -database 参数与 redis healthcheck 密码对 `ps`/`docker inspect` 可见 |
| L10 | Compose `DB_PASSWORD` 无强制校验 | `docker-compose.yml:30` | 未设置时静默传空串；改用 `${DB_PASSWORD:?...}` |
| L11 | 注册邮箱接受 display-name 形式 | `internal/validator/validator.go:42` | `mail.ParseAddress` 接受 `"Name" <a@b.c>`；建议限制为纯 addr-spec |
| L12 | TOTP secret 明文落库 | `internal/store/postgres/user.go:81,105` | 业界常见做法但 DB 泄露即可绕过 MFA；可选信封加密加固 |
| L13 | 未知 `SERVER_ENV` 仅警告 | `internal/config/config.go:574-576` | `Production`（大小写拼错）静默跳过全部生产校验；建议非白名单值拒绝启动 |
| L14 | `storeToken` 忽略 DELETE 错误 | `internal/store/postgres/verification.go:44` | 删除旧令牌失败被静默吞掉，错误语义不清晰 |
| L15 | `golang.org/x/crypto v0.49.0` 存在已知漏洞 | `go.mod` | govulncheck：15 个漏洞（ssh/agent 等）均未触达；建议升级 v0.52.0 消除噪音并保持卫生 |

---

## Info 级观察

| # | 问题 | 位置 | 要点 |
|---|------|------|------|
| I1 | OIDC Discovery 与实际不符 | `internal/handler/wellknown.go:50` | `authorization_endpoint` 声明 `/authorize` 实为 `/api/v1/authorize`；token 端点仅接受 JSON body 而非 RFC 6749 惯例的 form-urlencoded；标准 OIDC 客户端集成会失败 |
| I2 | consent_token 验证忽略 kid | `internal/service/oauth_security.go:248-258` | 固定用当前公钥，轮换后 5 分钟窗口内旧 consent_token 失效；fail-closed，建议统一走 kid 查找 |
| I3 | JWT 未设置/校验 audience | `internal/crypto/jwt.go:211-298` | 单服务自用可接受；多资源服务器场景建议引入 aud 防跨服务重放 |
| I4 | RequestID 原样信任客户端 `X-Request-ID` | `internal/middleware/requestid.go:22-27` | 可用于日志关联伪造；建议限制字符集/长度 |
| I5 | `GetClientIP` 部署注意 | `internal/middleware/ratelimit.go:209-232` | 不读 X-Forwarded-For 是正确选择；但反代后漏配 TRUSTED_PROXIES 时全局限流汇聚为代理 IP 导致单点锁死；建议启动告警 |
| I6 | 限流中间件位于 CORS 之前 | `internal/app/router.go:69-90` | OPTIONS 预检消耗配额；429 响应不带 CORS 头，浏览器表现为网络错误 |

---

## 确认安全的要点（抽查通过）

**密码学与令牌**
- JWT 强制 RS256 算法白名单，拒绝 alg=none/HS256 混淆；kid 仅用于服务端控制的公钥表查表，不可注入
- exp/iss/nbf 完整校验；token 验证后查库确认未撤销（fail-secure）；access/refresh/consent token 三类验证路径完全分离、issuer 互斥
- Refresh Token 事务原子轮换（`UPDATE ... WHERE rotated_at IS NULL AND revoked_at IS NULL`），重放触发全量撤销 + CriticalAuditLog
- RSA 密钥 < 2048 位拒绝加载；私钥文件权限/路径白名单校验；生产启动二次校验
- 恢复码 HMAC-SHA256 + O(1) 查找 + ConstantTimeCompare 且找到不 break；用后即时失效
- 全部安全随机来自 crypto/rand；bcrypt 生产强制 cost ≥ 12、上限 31；登录对不存在用户执行 dummy bcrypt 消除时序
- MFA Challenge 绑定 IP+UA、一次性、5 次尝试上限、上下文不匹配即销毁

**HTTP 与 OAuth**
- redirect_uri 数据库级精确匹配 + 交换时二次等值比较；授权码 10 分钟过期、UPDATE 原子一次性
- PKCE 强制 S256、plain 全局禁用、恒定时间比较；scope 全局白名单 + 客户端范围双重校验
- consent_token RS256 + 5 分钟 TTL + user_id 与 state 双重绑定
- 水平越权防护：userinfo/change-password/mfa 一律从 context 取 userID；token/revoke 属主校验且一律 204
- 用户枚举防护：注册/forgot-password 响应一致、统一 ErrInvalidCredentials
- 社交登录以 (provider, provider_user_id) 绑定，拒绝按 email 自动合并；email_verified=false 拒绝新建
- 初始化面板三层防御（loopback + isLocalRequest + INIT_ENABLED）+ advisory lock 防并发初始化
- 安全头齐全（CSP 每请求 nonce、HSTS、X-Frame-Options 等）；decodeJSON 1MB 上限 + DisallowUnknownFields
- BasicAuth 恒定时间比较；认证中间件 DB 错误 fail-closed

**数据层与基础设施**
- 全部查询参数化；动态标识符（表名/字段名/ORDER BY）全部走白名单；未发现 SQL 注入面
- client_secret bcrypt 落库；审计事件类型为服务端常量不可伪造；关键事件 CriticalAuditLog 同步落库
- 日志脱敏体系（SanitizeValue 递归/SanitizeDBURL/审计 metadata）覆盖完整；中间件只记 path 不记 query
- 邮件 html/template 自动转义；465 强制 TLS≥1.2；SMTP 错误不外泄
- 生产校验严格：DB_SSL_MODE ≥ require、CORS 非 localhost、PUBLIC_BASE_URL 必须 HTTPS、metrics Basic Auth 等
- Docker 非 root + read_only + cap_drop ALL + no-new-privileges；端口绑 127.0.0.1
- CI 全局 `permissions: contents: read`；secrets 仅 main 分支 docker job 可用，fork PR 隔离

## 修复路线图建议

**第一梯队（改动小、收益大）**
1. H1/H2：token 与重置/验证令牌去明文存储（迁移 019 + store 层改造，hash 基础设施已就绪）
2. M5：setup.go 错误脱敏（两行修复，与项目既有规范对齐）
3. L2/L13：配置校验补强（HMAC 密钥强度、SERVER_ENV 白名单）
4. L15：升级 `golang.org/x/crypto` 至 v0.52.0

**第二梯队（设计决策）**

5. H3：轮换私钥信封加密（需引入 KEK 管理策略）
6. M1：CORS credentials 策略收紧
7. M3/M4：限流与 MFA 防护 Redis 化 + 敏感端点 fail-closed
8. M2：社交登录 state 会话绑定

**第三梯队（纵深加固）**

9. M6：CI 供应链固定
10. L1/L4/L5/L7：TOTP 重放集合、token 端点敏感限流、管理员防护、验证码升级
11. I1：OIDC Discovery 合规性修正（影响标准客户端集成）
