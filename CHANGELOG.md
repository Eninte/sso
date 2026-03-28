# Changelog

All notable changes to this project will be documented in this file.

## [Unreleased]

### Security
- SEC-09: 实现JWT密钥轮换机制，支持无缝密钥更新
- SEC-10: 完善审计日志，覆盖所有关键安全操作
- SEC-11: `/metrics`端点新增Basic Auth认证保护
- SEC-12: 提高bcrypt cost下限从10到12，符合生产环境安全要求
- SEC-13: 社交登录新增state参数验证，防止CSRF攻击
- SEC-14: 移除已弃用的X-XSS-Protection安全头

### Added
- 新增 KeyRotationService 用于密钥轮换管理 (`internal/service/keyrotation.go`)
- 新增多种审计事件类型（密码修改、MFA操作、Token刷新等）
- 新增密钥轮换管理API端点
- 新增 `key_versions` 数据库表存储密钥版本信息
- JWTService 支持多密钥验证，添加 `kid` (Key ID) 到 JWT Header
- 新增 JWKS 端点支持多密钥发布
- 新增 `METRICS_USERNAME` 和 `METRICS_PASSWORD` 配置项
- 新增 `EventSystemStart` 审计事件类型
- 新增 `AuditService.LogSystemStart()` 方法
- 新增 `BasicAuth` 中间件用于端点认证保护
- 新增 `stateInfo` 结构体用于OAuth state管理
- 新增 `scanKeyVersions` 函数消除Postgres代码重复

### Changed
- JWTService 支持多密钥验证
- AuthService、UserService、MFAService 集成审计日志
- 扩展 Store 接口添加 KeyStore 方法
- OAuthService 集成 TokenService，修复TODO占位符
- 重构 register.go 错误处理，使用统一的 `writeValidationError` 函数
- MetricsHandler 支持 Basic Auth 认证
- SocialLoginService 的 HandleCallback 方法新增 state 参数
- AuditService 新增降级处理机制，channel满时尝试同步存储

### Fixed
- 修复 oauth.go 中的 TODO 占位符，实现真正的令牌生成
- 修复 auditSvc 未使用的问题，现在记录系统启动事件
- 修复 keyrotation.go 错误处理，使用 apperrors.Is() 替代直接比较
- 修复审计日志channel满时直接丢弃的问题，新增降级处理

### Configuration
- 新增 `KEY_ROTATION_ENABLED` 配置项（默认 false）
- 新增 `KEY_ROTATION_INTERVAL` 配置项（默认 90天）
- 新增 `KEY_TRANSITION_PERIOD` 配置项（默认 24小时）
- 新增 `METRICS_USERNAME` 配置项（Metrics Basic Auth用户名）
- 新增 `METRICS_PASSWORD` 配置项（Metrics Basic Auth密码）

### Database Migrations
- 新增 `009_create_key_versions.up.sql` 创建密钥版本表
- 新增 `009_create_key_versions.down.sql` 回滚密钥版本表
