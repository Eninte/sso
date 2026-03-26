# Changelog

All notable changes to this project will be documented in this file.

## [Unreleased]

### Security
- SEC-09: 实现JWT密钥轮换机制，支持无缝密钥更新
- SEC-10: 完善审计日志，覆盖所有关键安全操作

### Added
- 新增 KeyRotationService 用于密钥轮换管理 (`internal/service/keyrotation.go`)
- 新增多种审计事件类型（密码修改、MFA操作、Token刷新等）
- 新增密钥轮换管理API端点
- 新增 `key_versions` 数据库表存储密钥版本信息
- JWTService 支持多密钥验证，添加 `kid` (Key ID) 到 JWT Header
- 新增 JWKS 端点支持多密钥发布

### Changed
- JWTService 支持多密钥验证
- AuthService、UserService、MFAService 集成审计日志
- 扩展 Store 接口添加 KeyStore 方法

### Configuration
- 新增 `KEY_ROTATION_ENABLED` 配置项（默认 false）
- 新增 `KEY_ROTATION_INTERVAL` 配置项（默认 90天）
- 新增 `KEY_TRANSITION_PERIOD` 配置项（默认 24小时）

### Database Migrations
- 新增 `009_create_key_versions.up.sql` 创建密钥版本表
- 新增 `009_create_key_versions.down.sql` 回滚密钥版本表
