# 项目调试规则（仅非显眼内容）

## 调试工具
- 使用 `go test -v -run TestName ./path/` 运行单个测试
- 使用 `go test -race ./...` 检测竞态条件
- 使用 `go test -coverprofile=coverage.out ./...` 生成覆盖率报告

## 日志配置
- 使用 `slog` 进行结构化日志记录
- 生产环境日志级别为 `INFO`，开发环境为 `DEBUG`
- 错误日志包含上下文信息（如用户ID、请求ID）

## 数据库调试
- 使用 `make migrate-up` 和 `make migrate-down` 管理数据库迁移
- 数据库连接URL格式：`postgres://user:password@host:port/dbname?sslmode=disable`
- 使用 `store.Ping(ctx)` 检查数据库连接

## 缓存调试
- Redis连接URL格式：`redis://:password@host:port`
- 使用 `cache.Ping(ctx)` 检查Redis连接
- 缓存键命名规范：`{type}:{id}`（如 `user:123`）

## 常见问题
- JWT Token验证失败：检查签名算法是否为RS256
- 密码哈希失败：检查bcrypt cost设置（生产环境必须 >= 12）
- 数据库连接失败：检查 `DB_PASSWORD` 环境变量是否设置
- CORS错误：检查 `CORS_ALLOWED_ORIGINS` 环境变量配置
