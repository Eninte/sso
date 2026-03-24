# 变更日志

本项目遵循 [Keep a Changelog](https://keepachangelog.com/zh-CN/1.0.0/) 规范，版本遵循 [语义化版本](https://semver.org/lang/zh-CN/)。

## [Unreleased]

### Added
- 多语言支持（i18n）
- 更多第三方登录提供商支持

### Changed
- 无

### Fixed
- 无

## [1.0.0] - 2024-01-15

### Added
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

### Security
- bcrypt密码哈希（cost=12）
- JWT RS256签名
- Access Token有效期15分钟
- Refresh Token有效期7天
- 登录失败5次锁定30分钟

## [0.9.0] - 2024-01-01

### Added
- 多因素认证（MFA）TOTP支持
- MFA备用恢复码
- 用户邮箱验证
- 忘记密码/重置密码功能
- 管理员用户管理功能
- 系统健康检查端点
- 审计日志功能

### Changed
- 优化Token刷新机制（Token轮换）
- 改进错误消息国际化

### Fixed
- 修复并发Token刷新问题
- 修复CORS预检请求处理

## [0.8.0] - 2023-12-15

### Added
- Google第三方登录
- GitHub第三方登录
- 社交登录回调处理
- 登录提供商发现端点

### Changed
- 重构认证服务架构
- 优化数据库查询性能

## [0.7.0] - 2023-12-01

### Added
- Redis缓存层
- 会话管理
- Rate Limiting
- 请求限流中间件

### Fixed
- 修复数据库连接池泄漏
- 修复Token过期时间计算错误

## [0.6.0] - 2023-11-15

### Added
- 管理员API端点
- 用户禁用/启用功能
- 批量用户查询
- 过期数据清理

### Changed
- 优化数据库索引
- 改进日志格式

## [0.5.0] - 2023-11-01

### Added
- 邮件服务集成
- 邮箱验证功能
- 忘记密码流程
- 密码重置功能

### Security
- 添加密码强度验证
- 添加邮箱格式验证

## [0.4.0] - 2023-10-15

### Added
- OAuth 2.0授权端点
- 授权码生成和验证
- 客户端管理
- Scope验证

### Changed
- 重构Token生成逻辑
- 优化JWT Claims结构

## [0.3.0] - 2023-10-01

### Added
- 用户注册功能
- 用户登录功能
- Token签发
- Token刷新
- Token撤销

### Security
- bcrypt密码哈希
- JWT签名验证

## [0.2.0] - 2023-09-15

### Added
- PostgreSQL存储层
- 数据库迁移框架
- 用户模型
- Token模型

### Changed
- 从内存存储迁移到PostgreSQL

## [0.1.0] - 2023-09-01

### Added
- 项目初始化
- 基础HTTP服务器
- 路由框架
- 配置管理
- 日志系统
- 健康检查端点

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
