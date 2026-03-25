# 项目文档规则（仅非显眼内容）

## 文档结构
- `docs/` 目录包含所有项目文档
- `docs/API.md` 包含完整的API端点文档
- `docs/ARCHITECTURE.md` 包含系统架构设计
- `docs/DEPLOYMENT.md` 包含部署指南

## 代码文档
- 每个包必须有包注释（`package xxx` 上方）
- 导出函数必须有注释，描述功能和参数
- 使用 `// ====` 分隔符组织代码块
- 允许使用中文注释，但错误消息使用英文

## 配置文档
- `.env.example` 包含所有环境变量配置示例
- 生产环境必须设置的变量：`DB_PASSWORD`、`CORS_ALLOWED_ORIGINS`、`ADMIN_EMAILS`
- JWT密钥路径默认为 `./keys/private.pem` 和 `./keys/public.pem`

## API文档
- API端点路径：`/api/v1/`
- 认证端点：`/api/v1/login`、`/api/v1/register`、`/api/v1/token`
- 用户端点：`/api/v1/user/info`、`/api/v1/user/password`
- 管理员端点：`/api/v1/admin/users`、`/api/v1/admin/health`

## 测试文档
- 测试文件命名：`*_test.go`
- 测试包命名：`package_test`（黑盒测试）
- 测试函数命名：`TestFunctionName_Scenario`
- 使用 `testify/assert` 和 `testify/require` 进行断言
