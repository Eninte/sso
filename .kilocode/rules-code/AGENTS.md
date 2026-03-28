# 项目编码规则（仅非显眼内容）

## 错误处理
- 必须使用 `internal/errors` 包中预定义的错误变量（如 `apperrors.ErrInvalidCredentials`）
- 不要直接创建新的错误类型，使用 `errors.New()` 或 `errors.Wrap()` 构造函数
- Handler层使用 `ErrCode*` 常量（如 `ErrCodeLoginFailed`）作为消息错误码

## Mock测试
- 使用 `internal/store/mock` 包进行单元测试
- MockStore支持错误注入：设置 `CreateUserErr`、`GetUserByIDErr` 等字段模拟错误场景
- 测试前调用 `mockStore.Reset()` 清空数据（需先创建MockStore实例）

## JWT配置
- Access Token使用RS256算法签名，必须验证签名算法
- Refresh Token是随机字符串（32字节），不包含用户信息
- 生产环境bcrypt cost必须 >= 12，测试环境可使用10加快速度

## 数据库
- 使用 `store.ErrNotFound` 和 `store.ErrDuplicateEmail` 统一错误
- Token存储使用 `model.Token` 结构体，包含 `RevokedAt` 字段标记撤销
- 授权码使用后设置 `UsedAt` 字段，不要删除

## 配置验证
- 生产环境必须设置 `CORS_ALLOWED_ORIGINS`、`DB_PASSWORD`
- 生产环境必须设置 `DB_SSL_MODE=require`（禁止disable）
- 生产环境 `BCRYPT_COST` 必须 >= 12
- JWT密钥路径默认为 `./keys/private.pem` 和 `./keys/public.pem`
- 使用 `make generate-keys` 生成RSA密钥对
