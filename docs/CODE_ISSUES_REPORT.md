# SSO 项目代码问题分析报告

> 分析日期: 2026-03-28
> 修复日期: 2026-03-28
> 工具: go vet, golangci-lint, 手动代码审查

## 测试与 Lint 状态

- **单元测试**: 全部通过（validator, config, crypto, service, handler 等）
- **go vet**: 无错误
- **golangci-lint**: 2 个警告（ireturn），废弃 linter 已清理

---

## HIGH 严重度问题

### 1. JWTService Map 并发安全问题 ✅ 已修复

- **位置**: `internal/crypto/jwt.go:29-93`
- **类别**: 并发安全
- **问题**: `JWTService` 的 `keys`/`publicKeys` map 无 mutex 保护。`SetActiveKey`、`AddVerificationKey`、`RemoveKey` 写入 map，而 `ValidateAccessToken` 读取 map。密钥轮换时与 token 验证并发读写会触发 data race。
- **修复方案**: 为 `JWTService` 添加 `sync.RWMutex` 字段，写操作（`SetActiveKey`、`AddVerificationKey`、`RemoveKey`、`LoadKeysFromStore`）加写锁，读操作（`ValidateAccessToken`、`GetAccessTokenTTL`、`GetJWKS` 等）加读锁。

### 2. Goroutine 中调用 os.Exit 导致资源泄漏 ✅ 已修复

- **位置**: `cmd/server/main.go:269-275`
- **类别**: 资源泄漏
- **问题**: 在 goroutine 中调用 `os.Exit(1)` 处理启动失败。`os.Exit` 不会执行 defer 函数（如 `db.Close()`、`cacheSvc.Close()`），启动失败时资源泄漏。
- **修复方案**: 使用 error channel 通知主 goroutine 启动失败，由主 goroutine 负责清理和退出。`gracefulShutdown` 函数简化为纯关闭逻辑，信号处理移至 main 函数的 `select` 语句。

### 3. ForgotPassword 静默吞没所有错误 ✅ 已修复

- **位置**: `internal/service/user.go:158-181`
- **类别**: 错误处理
- **问题**: `ForgotPassword` 方法所有错误分支都返回 `nil`。虽为安全设计（不泄露用户是否存在），但同时吞没了数据库故障、token 生成失败等基础设施错误，生产环境无法排查问题。
- **修复方案**: 保持返回值为 `nil`（安全需要），但在每个错误分支添加 `slog.Error` 或 `slog.Debug` 日志记录，便于生产环境排查问题。

### 4. SQL 注入隐患 — fmt.Sprintf 拼接表名 ✅ 已修复

- **位置**: `internal/store/postgres/postgres.go:657-665`
- **类别**: SQL 安全
- **问题**: `cleanupExpiredBatch` 使用 `fmt.Sprintf` 将 `tableName` 参数拼接进 SQL。当前调用方传硬编码字符串（`"tokens"`、`"authorization_codes"`），但函数签名接受任意 string。若未来开发者传入用户输入，将成为 SQL 注入漏洞。
- **修复方案**: 添加 `allowedCleanupTables` 白名单 map，在 `cleanupExpiredBatch` 函数开头校验表名是否在白名单中，不在则返回错误。

---

## MEDIUM 严重度问题

### 5. AuditServiceInterface 接口不匹配 ✅ 已修复

- **位置**: `internal/service/interfaces.go:78-85` vs `internal/service/audit.go:81`
- **类别**: 设计缺陷
- **问题**: `AuditServiceInterface.Log()` 声明返回 `error`，但 `AuditService.Log()` 实现无返回值。`AuditService` 不满足该接口，编译器不会报错但接口无实际用途。
- **修复方案**: 修改接口定义，移除 `Log()` 方法的 `error` 返回值，使其与实现一致。审计日志是异步操作，失败不应阻塞主流程。

### 6. ResetPassword 跳过密码强度验证 ✅ 已修复

- **位置**: `internal/service/user.go:183-205`
- **类别**: 安全
- **问题**: `ResetPasswordWithAudit` 直接调用 `HashPassword(newPassword)` 但未调用 `validator.ValidatePassword`。而 `ChangePasswordWithAudit`（同文件）正常验证密码强度。密码重置可接受任意密码（甚至空密码）。
- **修复方案**: 在 `ResetPasswordWithAudit` 函数开头添加 `validator.ValidatePassword(newPassword)` 校验，确保密码重置也遵循密码强度要求。

### 7. SMTP TLS 静默回退到 InsecureSkipVerify ✅ 已修复

- **位置**: `internal/service/email.go:201-205`
- **类别**: 安全
- **问题**: `sendEmailSTARTTLS` 在 TLS 连接失败时静默回退到 `InsecureSkipVerify = true`，存在中间人攻击风险。代码中甚至有 `//nolint:gosec` 注释标记已知问题。
- **修复方案**: 移除 TLS 降级逻辑，证书验证失败直接返回错误。同时移除未使用的 `isTLSError` 函数。

### 8. OAuth State 缓存无清理机制 ✅ 已修复

- **位置**: `internal/service/social.go:164-168`
- **类别**: 安全 / 资源泄漏
- **问题**: `stateCache sync.Map` 无后台清理。用户发起 OAuth 授权后放弃流程，state 条目永久驻留内存。攻击者可重复发起未完成的授权流程耗尽内存。
- **修复方案**: 为 `SocialLoginService` 添加 `stopChan` 字段和 `cleanupExpiredStates` 后台 goroutine，每分钟清理超过 5 分钟的过期 state 条目。添加 `Close()` 方法用于优雅关闭。

### 9. OAuth State 验证 TOCTOU 竞争 ✅ 已修复

- **位置**: `internal/service/social.go:193-199`
- **类别**: 并发安全
- **问题**: state 验证分两步：先 `Load` 检查，后 `Delete` 删除。两个并发回调携带相同 state 时可同时通过验证。
- **修复方案**: 使用 `sync.Map.LoadAndDelete` 原子操作替代 `Load` + `Delete`，确保 state 验证和删除的原子性。

### 10. 多步数据库操作无事务保护 ⚠️ 未修复

- **位置**: `internal/service/user.go:117-152`、`postgres.go` 全局
- **类别**: 数据一致性
- **问题**: `VerifyEmail`（更新用户 + 删除 token）、`ResetPasswordWithAudit`（更新用户 + 删除 token + 撤销 token）等操作含多步 DB 写入但无事务保护。任一步骤失败或进程崩溃将导致数据不一致。
- **建议**: 在 Store 层提供事务支持（`BeginTx`），关键多步操作包裹在事务中。
- **备注**: 此问题需要较大范围重构，建议后续版本处理。

### 11. AuditService Worker 优雅关闭缺失 ✅ 已修复

- **位置**: `cmd/server/main.go:141,329-348`
- **类别**: 资源泄漏
- **问题**: `AuditService` 有 `Close()` 方法（`audit.go:76`），但 `gracefulShutdown` 从未调用。服务关闭时 worker goroutine 可能丢失未写入的审计日志。
- **修复方案**: 修改 `gracefulShutdown` 函数签名，添加 `auditSvc` 和 `socialSvc` 参数，在关闭流程中调用它们的 `Close()` 方法。

### 12. Token expires_in 硬编码 ✅ 已修复

- **位置**: `internal/handler/token.go:125`
- **类别**: 配置不一致
- **问题**: token 响应中 `expires_in` 硬编码为 `900`（15 分钟），未从 JWT 配置中读取实际 TTL。修改 JWT TTL 配置后响应值不会同步更新。
- **修复方案**: 在 `OAuthServiceInterface` 接口添加 `GetAccessTokenTTL() time.Duration` 方法，`OAuthService` 实现该方法并委托给 `jwtSvc.GetAccessTokenTTL()`，`TokenHandler` 调用该方法获取实际值。

---

## LOW 严重度问题

### 13. golangci.yml 保留废弃 Linter ✅ 已修复

- **位置**: `.golangci.yml:8-18`
- **类别**: 配置维护
- **问题**: 配置中保留了 10 个已废弃 linter（deadcode, exhaustivestruct, golint, ifshort, interfacer, maligned, nosnakecase, scopelint, structcheck, varcheck），产生无意义警告。
- **修复方案**: 从 `.golangci.yml` 的 `disable` 列表中移除这 10 个废弃 linter。

### 14. NewCache 返回接口类型（ireturn）⚠️ 未修复

- **位置**: `internal/cache/redis.go:403,418`、`cmd/server/main.go:288`
- **类别**: Lint 警告
- **问题**: `NewCache`、`NewCacheWithFallback`、`initCache` 返回接口类型 `Cache`，golangci-lint 的 ireturn 规则标记此问题。
- **建议**: 这是依赖注入的设计需要，可在 linter 配置中为这些函数添加例外；否则考虑返回具体类型。
- **备注**: 此为 lint 警告，不影响功能和安全性。

### 15. fallbackLog Goroutine 未传递 Context ⚠️ 未修复

- **位置**: `internal/service/audit.go:128`
- **类别**: Lint 警告
- **问题**: `fallbackLog` 中的 goroutine 创建新 context 而非使用传入的 context，contextcheck linter 标记此问题。
- **建议**: 这里是有意为之（goroutine 生命周期独立于调用方），可添加 linter 忽略注释 `//nolint:contextcheck`。
- **备注**: 此为 lint 警告，设计上是正确的。

### 16. AdminService 未使用的 client 字段 ✅ 已修复

- **位置**: `internal/service/admin.go:52`
- **类别**: 代码清理
- **问题**: `AdminService.client cache.Cache` 字段定义但从未使用。`NewAdminService` 构造函数也未初始化此字段。
- **修复方案**: 从 `AdminService` 结构体中移除未使用的 `client` 字段。

### 17. AuthService 两个构造函数风格不一致 ⚠️ 未修复

- **位置**: `internal/service/auth.go:78-126`
- **类别**: 设计一致性
- **问题**: 存在 `NewAuthService`（variadic metrics 参数）和 `NewAuthServiceWithOptions`（functional options 模式）两个构造函数，风格不一致。
- **建议**: 弃用旧构造函数，统一使用 options 模式。
- **备注**: 此为设计问题，需要评估现有调用方影响后处理。

### 18. DB_SSL_MODE 默认值不安全 ✅ 已修复

- **位置**: `internal/config/config.go:113`
- **类别**: 安全
- **问题**: `DB_SSL_MODE` 默认值为 `"disable"`。虽有生产环境检查，但更安全的默认值（如 `"prefer"`）可降低配置失误风险。
- **修复方案**: 将 `DB_SSL_MODE` 默认值从 `"disable"` 改为 `"prefer"`。

---

## 问题汇总

| 严重度 | 总数 | 已修复 | 未修复 |
|--------|------|--------|--------|
| HIGH   | 4    | 4 ✅   | 0      |
| MEDIUM | 8    | 7 ✅   | 1 ⚠️   |
| LOW    | 6    | 3 ✅   | 3 ⚠️   |
| **合计** | **18** | **14** | **4** |

### 未修复问题说明

| # | 问题 | 原因 |
|---|------|------|
| 10 | 多步数据库操作无事务保护 | 需要 Store 层事务支持，涉及较大范围重构 |
| 14 | NewCache 返回接口类型 | 依赖注入设计需要，lint 警告不影响功能 |
| 15 | fallbackLog Goroutine Context | 有意为之，设计正确 |
| 17 | AuthService 构造函数不一致 | 需评估现有调用方影响 |

### 修复验证

- **编译验证**: `go build ./...` ✅
- **单元测试**: `go test -short ./internal/...` ✅
- **Lint 检查**: 废弃 linter 警告已消除 ✅
