# 变更日志

本项目遵循 [Keep a Changelog](https://keepachangelog.com/zh-CN/1.0.0/) 规范，版本遵循 [语义化版本](https://semver.org/lang/zh-CN/)。

## [Unreleased]

### Security

- **`/metrics` 端点空凭据时 fail-open 修复**：`BasicAuth` 中间件在 `METRICS_USERNAME`/`METRICS_PASSWORD` 为空时直接放行请求，非生产环境（开发/测试/误配 staging）的 Prometheus 指标对任何人开放。修复为 fail-closed：凭据未配置一律返回 401（生产校验仍强制非空）
- **refresh_token 刷新强制机密客户端认证**（RFC 6749 §6）：token 端点此前只校验 `client_id` 归属，`ClientSecret` 被解析后直接丢弃——拿到任一 token 对的人仅凭公开的 client_id 即可持续换发新 token。新增 `validateRefreshClientBinding`：OAuth 签发 token 必须携带正确 `client_secret`（bcrypt 比对，缓存缺失时回源 DB）；公共客户端（`public_client=true`）豁免。客户端归属与密钥校验合并为单一收口点，杜绝漏检分支。**破坏性**：带 `client_id` 但不带 secret 的第三方机密客户端刷新将被拒（`INVALID_CLIENT`）；本仓库 6 个 SDK 的 refresh 均不携带 `client_id`/`client_secret`（既有的客户端归属缺口，本次非回归）
- **JWT access token 增加 `aud` 声明并强制校验**：签发时绑定 `aud = issuer`（两条签发路径均已覆盖），验证时用 `jwt.WithAudience` 拒绝缺失/不匹配的 token。**部署影响**：上线瞬间所有存量 access token 失效（≤15 分钟 TTL，影响有界），SDK 均不本地验签不受影响
- **登录路径防账号枚举**：用户不存在/邮箱未验证分支此前不执行 bcrypt 即返回，与"用户存在+密码错误"（cost=12 约 200ms）存在可测量的时序差。两分支补充等成本 dummy bcrypt（`PasswordService.VerifyDummyPassword`）；未验证邮箱的验证邮件补发改异步发送（`context.WithoutCancel` + `safego.Go`），消除慢 SMTP 对登录端点的阻塞与邮件轰炸放大
- **MFA Challenge 尝试计数原子化**：原 `Get→Attempts++→Set` 回写整个 challenge，并发失败请求丢失更新，`MaxMFALoginAttempts`（5 次）上限可被并发成倍击穿。改为独立计数键 + 原子 `INCR`，TTL 与 challenge 对齐；Redis 写失败时删除 challenge（fail-secure），达上限删除 challenge + 计数键并记录审计
- **限流 `INCR`/`SetTTL` 非原子导致永久锁死**：登录/邮件/MFA 恢复码三处限流在 `SetTTL` 失败时未回滚已递增的计数键——无 TTL 的键永久存在，限流从"限流"退化为"持久 DoS"（登录 20 次后永久无法登录、邮箱永久不能收发验证邮件、恢复码永久锁死）。修复为 TTL 失败时补偿删除计数键，并明确可用性优先语义（限流是 DoS 防护而非认证边界）
- **`GetClientIP` 支持 X-Forwarded-For**：此前只解析 `X-Real-IP`，反向代理默认只透传 XFF 时所有客户端 key 塌缩为代理 IP（限流失效 + 审计 IP 全部相同）。改为从右向左遍历 XFF 跳过受信代理链取首个非受信 IP（最左条目完全由客户端伪造，不可作为起点），保留 X-Real-IP 兼容；受信代理列表为空（默认）时只信 RemoteAddr。新增 12 个用例（含"非受信来源伪造 XFF 被忽略""多跳受信链"等）
- **关键审计事件强制同步落库**：密码修改/MFA 启停/账户锁定等关键事件有 6 处误用异步 `SafeAuditLog`，channel 拥塞或服务关闭时审计直接丢失，而 `IsCriticalEvent` 判定器全仓库零调用。修复为在 `SafeAuditLog` 内部按清单强制路由到同步 `LogSync`（单一收口点，调用方无法再搞错）；服务不支持同步记录时明确告警而非静默降级
- **账户锁定不再撤销全部 Token**：锁定是"有人知道邮箱并连续输错密码"的信号而非失窃证据，攻击者可借此每 30 分钟重复锁定任意已知邮箱账号、迫使受害者所有设备反复掉线。改为仅冻结新登录（Refresh token 重放检测与管理员禁用路径仍负责失窃场景的吊销）
- **`LAN_DEPLOYMENT` 一键总开关移除，改为 `INSECURE_PROD_BYPASS` 逐项豁免**：原开关一键放开全部 10 项生产校验（含 metrics 认证、MFA 密钥、CORS、HTTPS base URL），一次误配即让生产在无 CORS 限制、无指标认证、弱 MFA 密钥下静默运行。现每个豁免必须按名称显式声明（13 个具名项），未知项名拒绝启动；旧开关设置后仅记录错误日志。**部署迁移**：使用 `LAN_DEPLOYMENT=true` 的环境（含 `docker/.env.truenas.example`）升级后需按需改为 `INSECURE_PROD_BYPASS=db_ssl` 等，否则校验生效会拒绝启动
- **KEK 生产必填降级为告警**：`JWT_KEY_ENCRYPTION_KEY` 此前在 `KEY_ROTATION_ENABLED=true` 时生产硬必填，但 DB 密钥轮换与信封加密链路（`KeyRotationService`/`LoadKeysFromStore`/`SetKeyEncryptionKey`）在组合根中从未接线，为一个不会执行的路径强制要密钥属于误导。格式校验（64 位 hex）仍硬生效，缺失改为告警，相关代码补充"未接线"标注
- **Go 工具链升级到 1.26.10，消除全部标准库漏洞**：`govulncheck` 在 go1.26.6 下报告 **12 个可达 stdlib 漏洞**（`net/http` 6 项、`html/template` 2 项、`crypto/x509` / `crypto/tls` / `mime/multipart` 各 1 项），修复落在 1.26.9 / 1.26.10，Security Scan 的 govulncheck 步骤失败并连带 skip Build 与 Docker Build。CI `go-version` 升级为 `1.26.10`（5 处）、Docker 构建镜像升级为 `golang:1.26.10-alpine`、`go.mod` go 指令升级为 `1.26.10`。升级后 `govulncheck` 报告 **0 可达漏洞**（仍有 3 个模块级漏洞在依赖中，但代码不可达，不影响退出码）

### Fixed

- **mock store 与 Postgres 三处语义分叉**（单测通过而生产行为不同）：`GetByClientID`/`ListClients`/`StoreAuditLog`/`ListAuditLogs` 返回 map 内共享指针（调用方一写即竞态）改为拷贝语义；`CleanupExpired` 漏清 `verification_tokens`/`reset_tokens` 两张表（Postgres 清 4 张）已对齐；`DeleteUser`/`DeleteKey` 在"行不存在"时的 ErrNotFound 返回值与 Postgres 相反已对齐
- **安全关键错误路径"结构性不可测"**：mock 的 63 个接口方法中 15 个没有错误注入字段，其中 4 个正好承载 fail-secure 逻辑（账户锁定防 DB 错误绕过、MFA 恢复码、重置令牌/授权码重放防护），而 `TestLoginFailureCounter_DatabaseError` 等两个名为 DatabaseError 的测试实际未注入任何错误，制造了"已覆盖"的假象。补齐全部注入字段并重写测试，另新增 fail-secure 回归测试 4 项
- **空目录误导**：删除 `internal/audit`（含 11 个子目录）、`cmd/security-audit`、`cmd/mockserver`、`cmd/coverage-check`（审计功能实际位于 `internal/service/audit.go` + `auditutil`）
- **CI 流水线三处失败（Test / Security Scan / E2E）**：① **迁移版本号撞车**——`022_add_nonce_to_authorization_codes`（`9c9bd97`）与新增的 `022_drop_dead_token_plaintext_indexes`（`e777216`）同为 022，golang-migrate 在打开 source 阶段即报 `duplicate migration file`，Test 与 E2E 两个 job 均跪在 "Run database migrations"（后续步骤全被 skip，Build/Docker 连带 skipped）。死索引迁移重命名为 `023_`（不能反向重命名已推送并已在环境应用的 022）。② **gosec 不识别 `//nolint:gosec`**——那是 golangci-lint 的指令，gosec CLI 只认 `#nosec`，`dummyPasswordHash` 与 `INSECURE_PROD_BYPASS` 枚举常量共 7 处 G101 误报使 Security Scan 失败，改为项目既有惯例的逐行 `#nosec G101 -- 理由`。③ **迁移文件名校验前移**——lint job 新增 `Validate migration filenames` 步骤：按 up/down 方向分别判版本号唯一、并校验每个迁移 up/down 成对，纯文件名检查不依赖 DB，秒级 fail-fast，避免同类撞车再次拖到连库阶段才暴露

### Changed

- **`BasicAuth` 错误消息改英文**（遵循 AGENTS.md §7.3）
- **配置向导"部署模式"卡片**：`LAN_DEPLOYMENT` 开关替换为 `INSECURE_PROD_BYPASS` 输入框（后端 `allowedEnvKeys` 同步接受新变量与 `PUBLIC_BASE_URL`/`TRUSTED_PROXIES`）

### Added

- **管理员客户端管理 API**（OAuth 客户端 CRUD / 禁用启用 / 密钥轮换）：新增 `/api/v1/admin/clients` 系列端点，支持创建机密/公共客户端、查询、分页列表、全量更新、禁用/启用、删除与密钥轮换。安全设计：禁用/删除/轮换先撤销该客户端全部未撤销 Token；`is_active` 在 `getClient`（缓存命中与 DB 路径）统一检查，禁用客户端对公网返回 `INVALID_CLIENT`；公共客户端强制 PKCE（已有）且不支持密钥；`client_credentials` 注册被拒（`UNSUPPORTED_GRANT_TYPE`）。配套迁移 `021_add_client_management`（`oauth_clients` 新增 `is_active`/`updated_at`；`authorization_codes.client_id` → `ON DELETE CASCADE`、`tokens.client_id` → `ON DELETE SET NULL`）
- **标准浏览器 OIDC 授权码登录流**：新增 `GET/POST /ui/login` 浏览器登录页（HTML 表单 + 服务端渲染数学验证码 + 会话 cookie `sso_session`（HttpOnly/SameSite=Lax/生产 Secure）+ 防开放重定向）；`GET /api/v1/authorize` 支持可选认证（`Authorization` 头或 `sso_session` cookie），浏览器导航未登录 302 到登录页、已登录自动授权并 302 回 `redirect_uri?code=...`；`POST /ui/login` 纳入敏感限流（与 `/api/v1/login` 相同）；登录页使用专属 CSP（放行内联样式、不限制 form-action），全局安全策略不变
- **OIDC id_token 签发**：token 交换（authorization_code）响应新增 `id_token`（RS256，claims：iss/sub/aud=client_id/email/nonce），`authorization_codes` 表新增 `nonce` 列（migration `022`）；token 端点兼容 `application/x-www-form-urlencoded`（OIDC 标准）与 `application/json`，响应顶层输出标准 OIDC 字段（`access_token`/`id_token`/`token_type`/`expires_in`/`scope`），`data` 保留兼容旧客户端
- **客户端缓存键版本化**：`ClientCachePrefix` 由 `client:` 改为 `client:v2:`，避免升级后旧缓存条目（无 `is_active` 字段）被误判为禁用导致 OAuth 全流程瘫痪
- **缓存防击穿（singleflight）**：`OAuthService.getClient`、`AdminService.GetUser` 与 `/api/v1/userinfo` 处理器的缓存未命中回源路径接入 `golang.org/x/sync/singleflight`，同一 key 的并发回源只有一个查询 DB，其余等待复用（此前防击穿实现完整但生产零引用，热点 key 过期或 Redis 抖动时会击穿到 DB）
- **质量指标采集器**（`scripts/qualitymetrics`）：解析 `coverage.out` 与 gotestsum JUnit XML，复用 `dashboard.CalculateQualityScore` 打分、`postgres.StoreMetrics` 落库，补上 `quality_metrics` 表缺失的唯一数据生产者（此前质量仪表盘/周报永远空转）；CI test job 在覆盖率阈值检查通过后自动回写，本地可用 `make quality-metrics-store`
- **userinfo 缓存失效路径**：新增 `cache.UserInfoKey` 与 `serviceutil.InvalidateUserInfoCache`（同时清理 `userinfo:{id}` 与 `user:{id}`），邮箱验证成功、用户禁用/启用/删除后立即失效，消除变更后最长 5 分钟的陈旧窗口

### Fixed

- **覆盖率 80% 门槛自 `totalTimeout` 函数引入后被静默绕过**：`go tool cover -func | grep total` 未锚定行首，会同时匹配 `internal/service/email.go` 中名为 `totalTimeout` 的函数（0.0%）与真正的 `total:` 汇总行，`COVERAGE` 变量混入两行后 awk/bc 判定报错或空转——本地 `make test-coverage` 以"0.0% 低于阈值"误报失败，CI 的 bc 检查则静默跳过。修复：Makefile（2 处）与 ci.yml 均改为锚定匹配 `grep '^total:'`
- **机密客户端密钥校验在缓存命中下必然失败**（客户端管理 E2E 首次覆盖暴露）：`model.Client.ClientSecret` 带 `json:"-"` 标签，内存/Redis 缓存经 JSON 序列化后该字段为空，`validateClientSecret` 用空字符串校验 bcrypt 必然失败。修复：缓存命中且 secret 为空时回源 DB 读取哈希再验证
- **Go 工具链升级到 1.26.6，消除全部标准库漏洞**：`govulncheck` 在 go1.26.4 下报告 8 个 stdlib 漏洞（`net/http`/`crypto/tls`/`net/url`/`html/template`/`encoding/xml`/`encoding/asn1`，修复在 1.26.5/1.26.6）。CI `go-version` 升级为 `1.26.6`（5 处）、Docker 构建镜像升级为 `golang:1.26.6-alpine`、`go.mod` go 指令升级为 `1.26.6`。升级后 `govulncheck` 报告 **0 漏洞**
- **public client（`client_secret` 为 NULL）查询崩溃**：`GetByClientID` 对 `client_secret` 用 `sql.NullString` 扫描，此前 `converting NULL to string` 导致 public client 无法通过授权端点
- **客户端级撤销不清 token 缓存，已撤销 access token 最长残留 15 分钟**：认证中间件按 `token:<hash>` 缓存撤销状态（TTL 15 分钟），`DisableClient`/`RotateClientSecret`/`DeleteClient` 只在 DB 撤销 Token 而未失效对应缓存键，缓存窗口内已撤销 token 仍被接受。修复：新增 `TokenStore.ListTokenHashesByClient` 与 `serviceutil.InvalidateClientTokenCache`（以 DB 为索引逐键删除，查询失败回退全量清理），三条撤销路径均已接入；`DeleteClient` 因 FK `SET NULL` 会在删除后置空 `tokens.client_id`，缓存失效在删除前预取执行

### Changed

- **token 缓存按用户精确失效**：`serviceutil.InvalidateUserTokenCache` 不再 `DeletePattern("token:*")` 全量清空（任一用户改密/登出都会清掉所有用户的 token 缓存，放大 Redis 压力），改为以数据库为索引——新增 `TokenStore.ListTokenHashesByUser` 查询该用户全部 `access_token_hash` 后逐键删除，不影响其他用户的缓存命中；查询失败时回退全量清理，保证撤销及时生效
- **Makefile 测试参数与 CI 对齐**：`make test-coverage` / `test-coverage-check` 补齐 `-race -p 1 -tags=integration -timeout 180s`（与 ci.yml 一致，本地覆盖率数字可与 CI 比较）；`make lint` 固定 golangci-lint 版本（`GOLANGCI_LINT_VERSION ?= v1.64.8`，与 CI 相同，版本不匹配直接报错并附安装命令）
- **运行镜像升级 alpine 3.19 → 3.22**（3.19 已停止维护）；Dockerfile apk 镜像源参数化为 `ARG APK_MIRROR`（默认阿里云，海外构建可覆盖）
- **mock 全路径拷贝-替换语义**：`internal/store/mock` 此前仍原地修改共享指针的旧路径（用户 CRUD/登录尝试、授权码更新、StoreToken、重置令牌、社交账号更新、密钥状态变更、测试辅助 Add* 等）全部迁移为"改副本再写回 map"，getter 返回副本——与真实 DB 行更新语义一致，消除并发测试盲区（此前仅 token 撤销/轮换路径合规）

### Security

- **依赖升级消除模块级漏洞**：go-redis v9.18.0→v9.22.0、x/crypto v0.52.0→v0.55.0、testify v1.11.1→v1.12.1、prometheus/client_golang v1.23.2→v1.24.1；升级后 `govulncheck` 调用层面 **0 漏洞**（仅剩 GO-2026-5932 对未使用的 `x/crypto/openpgp` 的公告级条目，官方无修复版本）
- **错误处理收敛**：撤销活跃密钥由裸错误改为语义化 `KEY_STATUS_CONFLICT`（新增错误码，409）；"token service is not initialized" 等 5 处内部不变式错误统一接入 `apperrors.ErrInternal`（对外响应行为不变）；init 客户端审计事件由硬编码字符串 `"oauth_client_created"` 改为 `model.EventClientCreated`，可被按事件类型检索与关键事件归类正确识别

### Test

## [1.1.1] - 2026-08-03

### Fixed

- **JWKS 端点默认模式返回空数组**（commits `ee510c8` / `172d898` / `a01d88b`）：标准（非密钥轮换）模式下 `NewJWTService` 仅将密钥存入结构体字段，`GetJWKS()` 只读取 `publicKeys` 映射，导致 `/.well-known/jwks.json` 始终返回 `{"keys": []}`，依赖 JWKS 的外部验证方（SDK、API 网关）无法验证 token 签名。修复：构造时用 `DeriveKeyID`（RFC 7638 thumbprint，与轮换路径一致）派生 kid 并同步写入 `keys`/`publicKeys`/`activeKeyID`；旧无 kid token 仍可验证，向后兼容
- **SMTP 会话无超时导致潜在生产 hang**（commit `954083f`）：Windows 上 localhost:1 会接受连接但不发 banner，stdlib `smtp.SendMail`/`tls.Dial` 无超时，SMTP 服务端卡 banner 会让 SSO 服务永久挂起。修复：`sendEmailSTARTTLS`/`sendEmailSSL` 改用 `net.DialTimeout` + `tls.DialWithDialer` + `conn.SetDeadline`；`EmailConfig` 新增 `DialTimeout`/`TotalTimeout` 字段（默认 10s/30s）
- **internal/service 包测试超时卡死（280s → ~23s）**（commits `117a474` / `263916f` / `954083f`）：Redis 故障降级测试的 broken cache 未设超时与重试限制，go-redis 默认 `MaxRetries=3` + 5s 拨号超时，`LoginRateLimitMax=20` 循环单测试累计 40s+，4 个 RedisDownFallback 测试串跑使整包频繁超过 120s 包超时。修复：broken cache 加 100ms 短超时，SMTP 集成子测试加 200ms/500ms 短超时

### Test

- **GetJWKS 回归测试加强**（commit `817cfab`）：原测试仅断言 `keys` 字段存在，在 JWKS 空数组缺陷存在时也能通过。补充断言：默认模式下 JWKS 恰含一个主公钥、`kid == GetActiveKeyID()`、`kty=RSA`、`alg=RS256`，防止修复被回退后 JWKS 再次静默返回空数组

### Documentation

- **性能基准测试基线报告**（commit `a51527f`）：首次生成并提交全量基准测试报告（`docs/reports/performance-benchmark.md`），涵盖缓存、JWT、密码哈希、HMAC、AuthService 等性能指标，作为后续性能回归的对比基线
- **部署文档与代码实现对齐**（commit `176a510`）：
  - `PRODUCTION_DEPLOYMENT_CHECKLIST.md` 整体重写（v1.1）——环境变量名更正为代码实际读取的 `LOCKOUT_DURATION` / `JWT_ACCESS_TOKEN_TTL` / `JWT_REFRESH_TOKEN_TTL`，端口统一为 9090，API 路径更正为 `/api/v1/register` 等真实路由，移除不存在的脚本与文档引用，回滚方案由 `make migrate-down`（回滚全部迁移）改为 `migrate goto <版本号>`
  - `DEPLOYMENT.md`——删除与 `docker/docker-compose.yml` 脱节的配置副本；K8s 示例补齐生产校验必需环境变量（`MFA_RECOVERY_HMAC_KEY` / `CORS_ALLOWED_ORIGINS` / `JWT_ISSUER` 等），Redis 由 `emptyDir` 改为 PVC；裸机部署补充迁移工具复制步骤；移除废弃的 `X-XSS-Protection` 头
  - `TRUENAS.md`——密钥挂载点与 `docker-compose.truenas.yml` 对齐（`/keys`）；迁移命令改用 `PGPASSWORD` 传密；方式二模板补充 `LAN_DEPLOYMENT=true`
- **配置/API/迁移文档漂移修正**（commit `98532e1`）：
  - `CONFIGURATION.md`——`-env` 启动参数改为 `ENV_FILE_PATH` 环境变量；替换不存在的环境检查脚本引用；补充 `JWT_KEY_ENCRYPTION_KEY`、`MFA_CHALLENGE_TTL`、`SOCIAL_STATE_COOKIE_BINDING`、`INIT_ENABLED`、`INIT_LISTEN` 配置项文档
  - `API.md`——批准授权端点参数更正为 `consent_token` + `state`（与处理器一致）；补全 `login/mfa/verify`、`authorize/deny`、`mfa/recovery-codes/*` 五个未文档化端点
  - `ARCHITECTURE.md` / `README.md`——迁移版本号 001-015 更新为 001-020
  - `DATABASE_SCHEMA.md`——更正 `migrate-down` 为回滚全部迁移的危险提示，迁移命名说明改为 3 位序号
  - `TESTING.md`——移除不存在的 `test-env-check.sh` 引用

## [1.1.0] - 2026-07-21

### Added

- 多语言支持（i18n）
- 更多第三方登录提供商支持
- **KeyRotationService**：新增密钥轮换管理服务 (`internal/service/keyrotation.go`)
- **密钥轮换管理 API 端点**：支持密钥轮换操作
- **JWTService 多密钥验证**：添加 `kid` (Key ID) 到 JWT Header，支持多密钥验证
- **JWKS 端点**：支持多密钥发布
- **BasicAuth 中间件**：用于端点认证保护
- **stateInfo 结构体**：用于 OAuth state 管理
- **scanKeyVersions 函数**：消除 Postgres 代码重复
- **EventSystemStart 审计事件**：新增系统启动审计事件类型
- **AuditService.LogSystemStart()**：新增系统启动日志记录方法
- **Mock Store 错误注入测试**：新增 5 个顶层测试函数（7 个子测试）覆盖 `GetUserByEmailErr`、`CreateUserErr`、`GetTokenByRefreshTokenErr`、`GetUserByIDErr`、`RevokeTokenErr` 等关键存储故障路径
- **审计日志写入验证测试**：新增 3 个顶层测试函数（4 个子测试）验证 `LoginWithAudit`、`LogoutWithAudit`、`RefreshTokenWithAudit` 实际写入审计日志并验证事件类型、IP 地址、Success 标志

### Security

- SEC-09 **JWT密钥轮换并发安全**：`JWTService` 的 map 操作添加 `sync.RWMutex`，修复密钥轮换时与 token 验证的 data race
- SEC-10 **审计日志完善**：覆盖所有关键安全操作（密码修改、MFA操作、Token刷新等）
- SEC-11 **Metrics 端点认证保护**：`/metrics` 端点新增 Basic Auth 认证保护
- SEC-12 **bcrypt cost 下限提升**：从 10 提高到 12，符合生产环境安全要求
- SEC-13 **OAuth State 参数验证**：社交登录新增 state 参数验证，防止 CSRF 攻击
- SEC-14 **移除废弃安全头**：移除已弃用的 `X-XSS-Protection` 安全头
- **SMTP TLS 不再静默降级**：移除 `sendEmailSTARTTLS` 中 TLS 错误时自动回退到 `InsecureSkipVerify` 的逻辑，证书验证失败直接报错
- **OAuth State 原子验证**：`HandleCallback` 改用 `LoadAndDelete` 替代 `Load` + `Delete`，防止并发回调绕过 state 验证
- **OAuth State 内存泄漏防护**：`SocialLoginService` 添加后台 goroutine 每分钟清理过期 state 条目
- **密码重置强制密码强度校验**：`ResetPasswordWithAudit` 添加 `validator.ValidatePassword` 调用，与 `ChangePasswordWithAudit` 行为一致
- **SQL 注入防护**：`cleanupExpiredBatch` 添加表名白名单，防止未来误传用户输入导致注入
- **数据库连接默认启用 SSL**：`DB_SSL_MODE` 默认值从 `disable` 改为 `prefer`
- **CSP nonce 支持**：`SecurityHeaders` 中间件为每个请求生成随机 CSP nonce，防止 XSS 攻击
- **日志敏感信息脱敏**：新增 `logging.SanitizeEmail()` / `SanitizeToken()` / `SanitizePhone()`，`LogAuth` 自动脱敏邮箱
- **请求追踪 ID**：新增 `middleware.RequestID` 中间件，自动生成 `X-Request-ID`，日志自动关联

### Fixed

- **安全修复（T17 / 报告 L9+L10）**：Docker 凭据卫生——entrypoint.sh 不再 export 含明文密码的 `DATABASE_URL`，密码仅经 `PGPASSWORD` 注入 migrate 子进程，sso 主进程不继承；docker-compose.yml 的 `DB_PASSWORD`/`REDIS_PASSWORD` 改为强制必填（`:?` 语法），Redis 启用 `--requirepass`，command/healthcheck 运行时展开避免 `docker inspect` 固化明文

- **安全修复（T19 / 报告 I1）**：OIDC Discovery 与实际能力对齐——`authorization_endpoint` 修正为真实路由 `/api/v1/authorize`；`token_endpoint_auth_methods_supported` 移除未实现的 `client_secret_basic`（保留 `client_secret_post` 与 `none`）；docs/API.md 示例同步

- **安全修复（T16 / 报告 L3）**：JWT kid 改为按 RFC 7638 JWK Thumbprint 从公钥内容派生（SHA-256 base64url 前 16 字符），同一密钥内容恒定同一 kid；修复原随机 kid 导致重启后旧 token 无匹配公钥、全员强制登出的问题；JWKS 与签发 token header 的 kid 保证一致，重复公钥在 JWKS 中幂等去重

- **安全修复（T18 / 报告 L11）**：注册/登录入口邮箱仅接受纯 addr-spec——拒绝 display-name（`"Name" <a@b.c>`）、尖括号形式、尾部注释与前后空白（原实现 trim 后放行）；纯 addr-spec（含大小写、`+tag`）不受影响

- **安全修复（T15 / 报告 L7）**：图形验证码增加账号维度失败计数——登录凭据错误时同时按来源 IP 与账号（邮箱归一化后 SHA-256 作键，明文不落 Redis 键）累计失败次数，任一维度达阈值即要求验证码，攻击者换 IP 对同一账号爆破不再绕过验证码；登录成功双维度清零；注册/忘记密码等端点同步做账号维度判定；缓存故障时与 IP 维度一致 fail-open 降级

- **安全修复（T14 / 报告 L5+L6）**：管理员接口增加本人/末位管理员防护——禁止 admin 修改或删除自己的角色（`SELF_OPERATION_FORBIDDEN`，403）；当目标用户是系统中最后一个 active admin 时，任何人的降级/禁用/删除均被拒绝（`LAST_ACTIVE_ADMIN`，409），防止管理面永久锁死。`user_roles` 撤销提前到角色写入之前，且写入失败时按差异补撤销，被撤权用户不再能凭旧 JWT 继续调用管理接口。新增 `store.CountActiveAdmins`（含 Postgres 集成测试与 mock 支持）

- **安全修复（T13 / 报告 L4）**：`/api/v1/token` 纳入敏感限流（与登录/注册同一 `SensitiveRateLimiter`），堵住授权码/refresh_token/password 凭据爆破唯一的限流缺口；敏感限流未启用时自动回退到全局限流中间件，保证端点始终受限流保护。`token/revoke` 无爆破面，维持全局限流不变

- **T12 CI 供应链固定**（安全审查 M6）：全部 GitHub Actions 固定到 commit SHA（保留版本注释），gotestsum/migrate 等工具链固定具体版本；新增 Dependabot 周更 github-actions 与 gomod
- **T11 社交登录 state 会话绑定**（安全审查 M2）：state 改为始终由服务端生成（crypto/rand 32 字节），并下发 HMAC 指纹 Cookie（HttpOnly/SameSite=Lax/Path=/auth/5 分钟），回调双重校验防 login CSRF；新配置 `SOCIAL_STATE_COOKIE_BINDING` 默认开启，纯 API 客户端可关闭（生产关闭告警）
- **T10 限流 Redis 故障降级**（安全审查 M4）：全局/敏感端点限流器及登录/邮件限流器在 Redis 故障时降级为进程内内存限流（限额不失效，多副本下限额放宽为 N 倍），替代原静默 fail-open；降级路径补齐 Error 日志（中间件层每分钟节流）与 security_ratelimit_error_total 指标
- **T9 MFA 防护 Redis 化**（安全审查 M3+L1）：恢复码失败限流与 TOTP 重放记录迁入 Redis（`mfa:recovery:attempts:`/`mfa:totp:used:` 键，多副本一致）；TOTP 改按 timeStep 独立记录，修复 90 秒窗口内旧码可二次使用（L1）；Redis 不可用时降级为内存路径并记录 Error 日志
- **T8 CORS credentials 策略收紧**（安全审查 M1）：仅精确匹配的 Origin 发送 `Access-Control-Allow-Credentials`，通配（`*`/`*.suffix`）命中不再发送；所有响应补 `Vary: Origin`；精确匹配优先于通配
- **T7 JWT 轮换私钥信封加密**（安全审查 H3）：`key_versions.private_key` 落库前经 AES-256-GCM 加密（KEK 来自新配置 `JWT_KEY_ENCRYPTION_KEY`，64 位 hex）；读取按 `v1:gcm:` 前缀分派解密，存量明文行读取后自动懒加密回写；生产启用密钥轮换时 KEK 必填，否则拒绝启动
- **T6 升级 golang.org/x/crypto 至 v0.52.0**（安全审查 L15）：消除 15 条已知漏洞的扫描噪音（代码路径本未触达）；govulncheck 复扫 0 项受影响
- **T4 生产环境强制 MFA_RECOVERY_HMAC_KEY ≥32 字节**（安全审查 L2）：弱密钥（如 123456）将拒绝启动，非生产环境降级为警告
- **T5 SERVER_ENV 白名单校验**（安全审查 L13）：非 development/production/test 的值（含 Production 大小写拼错）拒绝启动，避免静默跳过全部生产校验
- **T3 日志与响应脱敏补齐**（安全审查 M5+L8）：配置向导 DB/Redis 连接测试失败不再向响应返回原始错误详情、日志经 SanitizeDBURL 脱敏；邮件发送日志收件人改用 SanitizeEmail
- **T2 重置/验证令牌哈希存储**（安全审查 H2+L14）：`reset_tokens`/`verification_tokens` 改存 SHA-256 hash，校验时 ConstantTimeCompare 比对 hash；修复 storeToken 静默忽略 DELETE 错误。**注意**：迁移 020 执行后已签发未过期的重置/验证令牌失效，用户需重新发起流程
- **T1 tokens 表去除明文存储**（安全审查 H1）：`access_token`/`refresh_token` 明文列改为仅写 NULL，查询/轮换/撤销全部走 SHA-256 hash，删除全部明文回退路径；Redis token 缓存键同步改为 hash。**注意**：迁移 019 执行后，迁移 018 之前签发且仍在有效期内的 refresh token 将失效，用户需重新登录一次
- **修复 mock `RotateRefreshToken` 原地写数据竞争**（CI `-race` 检出）：mock 的 map 存 `*model.Token` 共享指针且 getter 返回同一指针，原地修改 `oldToken` 与 service 层锁外读取 `tokenRecord.RevokedAt` 产生竞争；改为拷贝-替换，语义与真实 DB 行更新一致。防范规范已写入 `AGENTS.md` §7.5
- 修复 `internal/config` 两个测试在 Windows 下因 `t.Cleanup(os.Chdir)` 注册早于 `t.TempDir()` 导致的临时目录清理失败（cleanup LIFO 顺序问题）
- 修复 `Makefile test-coverage-full` 调用不存在的 `go tool cover -merge`：新增 `scripts/merge_coverage.go` 实现 coverprofile 块级并集合并
- 修复服务器启动失败时 goroutine 中调用 `os.Exit(1)` 导致 `db.Close()` / `cacheSvc.Close()` 不执行的问题，改用 error channel 通知主 goroutine
- 修复 `ForgotPassword` 所有错误分支返回 `nil` 导致生产环境无法排查数据库故障、token 生成失败等问题，添加 `slog` 日志
- 修复 `gracefulShutdown` 未调用 `auditSvc.Close()` 导致 worker goroutine 丢失未写入审计日志的问题
- 修复 token 响应中 `expires_in` 硬编码为 `900` 未从 JWT 配置读取的问题
- 修复 `AdminService` 结构体中未使用的 `client` 字段
- 修复 `admin.go` 状态字符串硬编码问题，改用 `model.UserStatusDisabled` / `model.UserStatusActive` 常量
- 修复 `admin.go` 版本号硬编码 `1.0.0`，改用 `Version` 变量（支持 `-ldflags` 注入）
- **修复 oauth.go TODO 占位符**：实现真正的令牌生成
- **修复 auditSvc 未使用问题**：现在记录系统启动事件
- **修复 keyrotation.go 错误处理**：使用 `apperrors.Is()` 替代直接比较
- **修复审计日志丢弃问题**：channel 满时新增降级处理，不再直接丢弃
- **修复管理员路由前缀**：管理员端点从 `/admin/...` 移至 `/api/v1/admin/...`，与其他 API 端点保持一致，修复 E2E 测试中 11 个因 URL 不匹配导致的假阳性跳过

### Changed

- PostgreSQL 驱动由 `lib/pq` 迁移至 `pgx/v5 stdlib`，保留现有 `database/sql` 接口与连接池配置
- `AuditServiceInterface.Log()` 移除 `error` 返回值，与 `AuditService.Log()` 实现保持一致（审计日志异步写入，失败不阻塞主流程）
- `gracefulShutdown` 函数签名变更，新增 `auditSvc`、`socialSvc` 和 `timeout` 参数用于优雅关闭
- 清理 `.golangci.yml` 中 10 个已废弃 linter 配置（deadcode, exhaustivestruct, golint 等）
- `auth.go` 提取 `revokeRetryBaseDelay` 常量替代内联魔法数字 `100 * time.Millisecond`
- `config.go` 新增 `ShutdownTimeout` 配置项（环境变量 `SHUTDOWN_TIMEOUT`，默认 30s）
- CI 测试步骤添加覆盖率阈值检查（≥70%）
- Makefile 新增 `test-coverage-check` 目标
- **AuthService、UserService、MFAService 集成审计日志**
- **扩展 Store 接口**：添加 `KeyStore` 方法
- **OAuthService 集成 TokenService**：修复 TODO 占位符
- **重构 register.go 错误处理**：使用统一的 `writeValidationError` 函数
- **MetricsHandler 支持 Basic Auth 认证**
- **SocialLoginService.HandleCallback**：新增 state 参数
- **AuditService 降级处理**：channel 满时尝试同步存储
- **测试质量审计**：删除 15 个空壳/重复测试函数，新增 11 个有价值的测试函数
- **审计测试可靠性**：`audit_test.go` 中 23 处 `time.Sleep(100ms)` 替换为 `require.Eventually` 轮询（10ms 间隔，2s 超时）
- **Redis 测试隔离**：`cache/redis_test.go` 添加 `//go:build integration` 标签，避免无 Redis 环境下测试失败
- **Mock Store 错误注入覆盖率**：从 3/32 (9.4%) 提升至 8/32 (25.0%)

### Configuration

- 新增 `KEY_ROTATION_ENABLED` 配置项（默认 false）
- 新增 `KEY_ROTATION_INTERVAL` 配置项（默认 90天）
- 新增 `KEY_TRANSITION_PERIOD` 配置项（默认 24小时）
- 新增 `METRICS_USERNAME` 配置项（Metrics Basic Auth用户名）
- 新增 `METRICS_PASSWORD` 配置项（Metrics Basic Auth密码）

### Database Migrations

- 新增 `009_create_key_versions.up.sql` 创建密钥版本表
- 新增 `009_create_key_versions.down.sql` 回滚密钥版本表

---

## [1.0.0] - 2026-03-27

### Added
- 实现RBAC系统并提升测试覆盖率
- 实现基于角色的访问控制(RBAC)
- 实现Redis缓存集成与密钥轮换机制
- JWT密钥轮换机制并完善审计日志
- 添加基准测试工具和性能报告生成功能
- 增加OAuth服务审计和缓存测试用例
- 添加原子登录尝试操作接口和方法

### Fixed
- 修复缓存操作错误被忽略的问题

---

## [0.1.0] - 2026-03-23

### Added
- 项目初始化
- 用户注册/登录功能
- JWT Token签发与验证（RS256）
- Access Token和Refresh Token支持
- Token刷新和撤销
- OAuth 2.0授权码流程
- OpenID Connect Discovery端点
- 密码bcrypt哈希加密
- 登录失败锁定机制
- 安全HTTP头中间件
- CORS配置支持
- Prometheus指标端点
- 健康检查端点
- 数据库迁移脚本
- Docker部署支持
- 完整的单元测试覆盖
- 多因素认证（MFA）TOTP支持
- 用户邮箱验证
- 忘记密码/重置密码功能
- 管理员用户管理功能
- 审计日志功能
- Google/GitHub第三方登录
- Redis缓存层
- Rate Limiting
- 错误消息国际化支持

### Changed
- 重构认证服务架构
- 优化数据库查询性能
- 完成架构重构和Handler依赖更新

### Fixed
- 统一错误处理机制
- 完善错误统一处理

---

## 版本说明

### 版本号格式

`主版本号.次版本号.修订号`

- **主版本号**：不兼容的API变更
- **次版本号**：向下兼容的功能新增
- **修订号**：向下兼容的问题修复

### 变更类型

- **Added**：新功能
- **Changed**：已有功能的变更
- **Deprecated**：即将移除的功能
- **Removed**：已移除的功能
- **Fixed**：Bug修复
- **Security**：安全相关的变更
