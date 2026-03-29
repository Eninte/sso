# 变更日志

本项目遵循 [Keep a Changelog](https://keepachangelog.com/zh-CN/1.0.0/) 规范，版本遵循 [语义化版本](https://semver.org/lang/zh-CN/)。

## [Unreleased]

### Added
- 多语言支持（i18n）
- 更多第三方登录提供商支持
- **Mock Store 错误注入测试**：新增 5 个顶层测试函数（7 个子测试）覆盖 `GetUserByEmailErr`、`CreateUserErr`、`GetTokenByRefreshTokenErr`、`GetUserByIDErr`、`RevokeTokenErr` 等关键存储故障路径
- **审计日志写入验证测试**：新增 3 个顶层测试函数（4 个子测试）验证 `LoginWithAudit`、`LogoutWithAudit`、`RefreshTokenWithAudit` 实际写入审计日志并验证事件类型、IP 地址、Success 标志

### Security

- **JWT密钥轮换并发安全**：`JWTService` 的 map 操作添加 `sync.RWMutex`，修复密钥轮换时与 token 验证的 data race
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

- 修复服务器启动失败时 goroutine 中调用 `os.Exit(1)` 导致 `db.Close()` / `cacheSvc.Close()` 不执行的问题，改用 error channel 通知主 goroutine
- 修复 `ForgotPassword` 所有错误分支返回 `nil` 导致生产环境无法排查数据库故障、token 生成失败等问题，添加 `slog` 日志
- 修复 `gracefulShutdown` 未调用 `auditSvc.Close()` 导致 worker goroutine 丢失未写入审计日志的问题
- 修复 token 响应中 `expires_in` 硬编码为 `900` 未从 JWT 配置读取的问题
- 修复 `AdminService` 结构体中未使用的 `client` 字段
- 修复 `admin.go` 状态字符串硬编码问题，改用 `model.UserStatusDisabled` / `model.UserStatusActive` 常量
- 修复 `admin.go` 版本号硬编码 `1.0.0`，改用 `Version` 变量（支持 `-ldflags` 注入）
- **修复管理员路由前缀**：管理员端点从 `/admin/...` 移至 `/api/v1/admin/...`，与其他 API 端点保持一致，修复 E2E 测试中 11 个因 URL 不匹配导致的假阳性跳过

### Changed

- `AuditServiceInterface.Log()` 移除 `error` 返回值，与 `AuditService.Log()` 实现保持一致（审计日志异步写入，失败不阻塞主流程）
- `gracefulShutdown` 函数签名变更，新增 `auditSvc`、`socialSvc` 和 `timeout` 参数用于优雅关闭
- 清理 `.golangci.yml` 中 10 个已废弃 linter 配置（deadcode, exhaustivestruct, golint 等）
- `auth.go` 提取 `revokeRetryBaseDelay` 常量替代内联魔法数字 `100 * time.Millisecond`
- `config.go` 新增 `ShutdownTimeout` 配置项（环境变量 `SHUTDOWN_TIMEOUT`，默认 30s）
- CI 测试步骤添加覆盖率阈值检查（≥70%）
- Makefile 新增 `test-coverage-check` 目标
- **测试质量审计**：删除 15 个空壳/重复测试函数，新增 11 个有价值的测试函数
- **审计测试可靠性**：`audit_test.go` 中 23 处 `time.Sleep(100ms)` 替换为 `require.Eventually` 轮询（10ms 间隔，2s 超时）
- **Redis 测试隔离**：`cache/redis_test.go` 添加 `//go:build integration` 标签，避免无 Redis 环境下测试失败
- **Mock Store 错误注入覆盖率**：从 3/32 (9.4%) 提升至 8/32 (25.0%)

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
