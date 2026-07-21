# 变更日志

本项目遵循 [Keep a Changelog](https://keepachangelog.com/zh-CN/1.0.0/) 规范，版本遵循 [语义化版本](https://semver.org/lang/zh-CN/)。

## [Unreleased]

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
