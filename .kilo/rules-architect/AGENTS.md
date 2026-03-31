# 项目架构规则

## 核心原则：生产代码与测试的关系

**生产代码服务于真实业务场景，不为测试开后门。**

- 生产代码独立运行，不依赖测试
- 测试用于验证生产代码的行为
- 测试断言与生产行为不一致时，修改测试代码，不改生产代码
- 禁止在生产代码中添加仅用于测试的API、开关或后门

## 测试修复原则

当测试失败时，先判断原因：

1. **测试断言错误** — 生产行为正确，测试期望值错误 → 修改测试
2. **生产代码Bug** — 生产行为确实错误 → 修复生产代码
3. **测试环境问题** — 依赖缺失或配置错误 → 修复测试环境

判断方法：对照API文档和业务需求，确认生产代码的实际行为是否符合预期。

## 分层架构

- **Handler**（`internal/handler/`）— HTTP路由、输入验证、错误响应
- **Service**（`internal/service/`）— 业务逻辑、事务管理
- **Store**（`internal/store/`）— 数据访问接口；Postgres实现在`store/postgres/`
- **Model**（`internal/model/`）— 数据结构定义（含JSON标签）

依赖注入通过接口实现（`store.Store`、`service.AuthServiceInterface`）。测试使用 `internal/store/mock` 包。

## 统一错误处理

- 使用 `apperrors.Err*` 预定义错误变量
- Store层返回统一错误（`store.ErrNotFound`、`store.ErrDuplicateEmail`）
- Service层用 `fmt.Errorf("context: %w", err)` 包装
- Handler层映射为HTTP状态码，响应消息使用 `ErrCode*` 常量

## 数据库

- 测试服务已在远程主机运行，禁止安装 PostgreSQL 或 Redis
- 测试环境连接：`DB_HOST=192.168.1.3`，`REDIS_HOST=192.168.1.3`
- 生产环境必须设置 `DB_SSL_MODE=require`
