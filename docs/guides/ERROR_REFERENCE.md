# 错误码参考

本文档列出 SSO 服务所有错误码、消息和触发场景。

## 国际化支持

错误消息支持多语言：

| 语言 | 代码 | 说明 |
|------|------|------|
| 简体中文 | `zh-CN` | 默认语言 |
| 英语 | `en-US` | 备选语言 |

**语言检测优先级**：
1. 查询参数 `?lang=<code>`
2. `Accept-Language` 请求头
3. 默认 `zh-CN`

**回退链**：请求语言 → `zh-CN` → 错误码字符串本身

## 通用错误

| ErrorCode | HTTP | zh-CN 消息 | 触发场景 |
|-----------|------|------------|----------|
| `INTERNAL_ERROR` | 500 | 内部服务器错误 | 服务器内部异常 |
| `BAD_REQUEST` | 400 | 请求参数错误 | 请求格式错误或参数无效 |
| `NOT_FOUND` | 404 | 资源不存在 | 请求的资源不存在 |
| `CONFLICT` | 409 | 资源冲突 | 如邮箱已注册 |
| `UNAUTHORIZED` | 401 | 未授权 | 未提供认证凭据 |
| `FORBIDDEN` | 403 | 禁止访问 | 无权限访问 |
| `TOO_MANY_REQUESTS` | 429 | 请求过多，请稍后重试 | 超过限流阈值 |
| `REQUEST_BODY_TOO_LARGE` | 413 | 请求体过大 | 请求体超过大小限制 |
| `REQUEST_BODY_EXTRA_DATA` | 400 | 请求体包含多余数据 | JSON 请求体包含未定义字段 |

## 认证错误

| ErrorCode | HTTP | zh-CN 消息 | 触发场景 |
|-----------|------|------------|----------|
| `INVALID_CREDENTIALS` | 401 | 邮箱或密码错误 | 登录时密码错误 |
| `ACCOUNT_LOCKED` | 403 | 账户已锁定 | 登录失败次数过多，账户被临时锁定 |
| `ACCOUNT_DISABLED` | 403 | 账户已被禁用 | 账户被管理员禁用 |
| `INVALID_TOKEN` | 401 | 无效的Token | Token 格式错误或无法解析 |
| `TOKEN_EXPIRED` | 401 | Token已过期 | Token 超过有效期 |
| `EMAIL_NOT_VERIFIED` | 401 | 请先验证邮箱后再登录 | 未验证邮箱尝试登录 |

## 用户/注册错误

| ErrorCode | HTTP | zh-CN 消息 | 触发场景 |
|-----------|------|------------|----------|
| `EMAIL_EXISTS` | 409 | 邮箱已注册 | 注册时邮箱已被使用 |
| `EMAIL_INVALID` | 400 | 邮箱地址格式无效 | 邮箱格式不符合 RFC 5322 |
| `EMAIL_REQUIRED` | 400 | 邮箱地址不能为空 | 请求中缺少邮箱字段 |
| `PASSWORD_TOO_SHORT` | 400 | 密码长度不能少于8个字符 | 密码少于 8 字符 |
| `PASSWORD_TOO_LONG` | 400 | 密码长度不能超过72个字符 | 密码超过 72 字符（bcrypt 限制） |
| `PASSWORD_REQUIRED` | 400 | 密码不能为空 | 请求中缺少密码字段 |
| `PASSWORD_MISMATCH` | 400 | 密码不匹配 | 验证密码时不匹配 |
| `PASSWORD_NO_UPPERCASE` | 400 | 密码必须包含至少一个大写字母 | 密码缺少大写字母 |
| `PASSWORD_NO_LOWERCASE` | 400 | 密码必须包含至少一个小写字母 | 密码缺少小写字母 |
| `PASSWORD_NO_DIGIT` | 400 | 密码必须包含至少一个数字 | 密码缺少数字 |
| `PASSWORD_NO_SPECIAL` | 400 | 密码必须包含至少一个特殊字符 | 密码缺少特殊字符 |

## 邮箱验证错误

| ErrorCode | HTTP | zh-CN 消息 | 触发场景 |
|-----------|------|------------|----------|
| `EMAIL_ALREADY_VERIFIED` | 409 | 邮箱已验证 | 对已验证邮箱重复发送验证请求 |
| `VERIFICATION_CODE_INVALID` | 400 | 验证码无效 | 验证令牌不正确 |
| `VERIFICATION_CODE_EXPIRED` | 400 | 验证码已过期 | 验证令牌超过有效期 |
| `RESET_TOKEN_INVALID` | 400 | 重置令牌无效 | 密码重置令牌不正确 |
| `RESET_TOKEN_EXPIRED` | 400 | 重置令牌已过期 | 密码重置令牌超过有效期 |

## OAuth 错误

| ErrorCode | HTTP | zh-CN 消息 | 触发场景 |
|-----------|------|------------|----------|
| `INVALID_CLIENT` | 400 | 无效的客户端 | client_id 不存在或无效 |
| `INVALID_REDIRECT_URI` | 400 | 无效的重定向URI | redirect_uri 与注册的不匹配 |
| `INVALID_GRANT_TYPE` | 400 | 无效的授权类型 | 不支持的 grant_type |
| `INVALID_CODE` | 400 | 无效的授权码 | 授权码不正确 |
| `CODE_EXPIRED` | 400 | 授权码已过期 | 授权码超过有效期 |
| `CODE_USED` | 400 | 授权码已被使用 | 授权码已被消费（只能使用一次） |
| `INVALID_CODE_VERIFIER` | 400 | 无效的PKCE验证器 | code_verifier 与 code_challenge 不匹配 |
| `INVALID_PKCE_CHALLENGE` | 400 | 无效的PKCE挑战码 | code_challenge 格式或方法无效 |

## MFA 错误

| ErrorCode | HTTP | zh-CN 消息 | 触发场景 |
|-----------|------|------------|----------|
| `MFA_ALREADY_ENABLED` | 409 | MFA已启用 | 对已启用 MFA 的用户再次 setup |
| `MFA_NOT_ENABLED` | 400 | MFA未启用 | 对未启用 MFA 的用户执行 MFA 操作 |
| `INVALID_TOTP_CODE` | 400 | 验证码错误 | TOTP 代码不正确 |
| `TOTP_CODE_EXPIRED` | 400 | 验证码已过期 | TOTP 代码超过时间窗口 |
| `INVALID_MFA_SECRET` | 400 | MFA密钥无效 | MFA secret 格式无效 |
| `RECOVERY_CODE_INVALID` | 400 | 恢复码无效 | 恢复码不正确 |
| `RECOVERY_CODE_USED` | 400 | 恢复码已使用 | 恢复码已被消费（只能使用一次） |
| `RECOVERY_CODE_GENERATION` | 500 | 恢复码生成失败 | 生成恢复码时内部错误 |
| `TOO_MANY_RECOVERY_ATTEMPTS` | 429 | 恢复码尝试次数过多，请稍后再试 | 恢复码验证失败次数过多 |

## 第三方登录错误

| ErrorCode | HTTP | zh-CN 消息 | 触发场景 |
|-----------|------|------------|----------|
| `PROVIDER_NOT_SUPPORTED` | 400 | 不支持的登录提供商 | 请求的 provider 不在支持列表中 |
| `OAUTH_CODE_EXCHANGE_FAILED` | 400 | OAuth授权码交换失败 | 与第三方提供商交换 code 时失败 |
| `SOCIAL_LOGIN_FAILED` | 400 | 社交登录失败 | 第三方登录流程内部错误 |
| `OAUTH_STATE_INVALID` | 400 | OAuth状态无效 | state 参数不匹配 |
| `OAUTH_STATE_EXPIRED` | 400 | OAuth状态已过期，请重新发起登录 | state 超过有效期 |

## 密钥/加密错误

| ErrorCode | HTTP | zh-CN 消息 | 触发场景 |
|-----------|------|------------|----------|
| `KEY_NOT_FOUND` | 500 | 密钥未找到 | 请求的密钥 ID 不存在 |
| `KEY_PATH_INVALID` | 500 | 密钥路径无效 | 密钥文件路径无效或包含路径遍历 |
| `KEY_PARSE_FAILED` | 500 | 密钥解析失败 | 无法解析 PEM 格式的密钥 |
| `KEY_ID_EMPTY` | 400 | 密钥ID不能为空 | 密钥轮换时未提供 key_id |
| `PRIVATE_KEY_NIL` | 400 | 私钥不能为空 | JWT 初始化时缺少私钥 |
| `PUBLIC_KEY_NIL` | 400 | 公钥不能为空 | JWT 初始化时缺少公钥 |
| `NO_ACTIVE_KEY` | 500 | 无活跃密钥可用 | 所有密钥都被标记为 deprecated 或 revoked |
| `KEY_PERMISSION_OPEN` | 500 | 密钥文件权限不安全 | 私钥文件权限超过 600 |
| `KEY_TOO_SHORT` | 500 | RSA密钥长度必须至少为2048位 | 密钥长度不足 2048 位 |

## 配置错误

| ErrorCode | HTTP | zh-CN 消息 | 触发场景 |
|-----------|------|------------|----------|
| `DB_PASSWORD_REQUIRED` | 500 | DB_PASSWORD环境变量必须设置 | 启动时未设置数据库密码 |
| `JWT_KEY_REQUIRED` | 500 | JWT密钥路径必须设置 | 启动时未配置 JWT 密钥路径 |
| `BCRYPT_COST_TOO_LOW` | 500 | 生产环境bcrypt cost必须 >= 12 | 生产环境 BCRYPT_COST 低于 12 |

## Handler 层消息码

这些是 Handler 层用于响应消息的 ErrorCode 常量，不对应 AppError 常量：

| ErrorCode | 说明 | 使用场景 |
|-----------|------|----------|
| `INVALID_REQUEST_FORMAT` | 请求格式无效 | JSON 解析失败 |
| `MISSING_REQUIRED_PARAM` | 缺少必填参数 | 通用参数校验失败 |
| `MISSING_USER_ID` | 缺少用户ID | 管理员操作缺少 user_id |
| `MISSING_TOKEN` | 缺少Token | Token 撤销/刷新时缺少 token |
| `MISSING_CODE` | 缺少授权码 | OAuth 回调缺少 code |
| `MISSING_CLIENT_ID` | 缺少客户端ID | OAuth 请求缺少 client_id |
| `MISSING_REDIRECT_URI` | 缺少回调地址 | OAuth 请求缺少 redirect_uri |
| `MISSING_REFRESH_TOKEN` | 缺少Refresh Token | 刷新 Token 时缺少 refresh_token |
| `MISSING_AUTH_CODE` | 缺少授权码 | 授权码交换时缺少 code |
| `MISSING_OLD_PASSWORD` | 缺少旧密码 | 修改密码时缺少 old_password |
| `MISSING_NEW_PASSWORD` | 缺少新密码 | 修改/重置密码时缺少 new_password |
| `MISSING_VERIFICATION_CODE` | 缺少验证码 | MFA 验证时缺少 code |
| `STATE_INVALID` | State 无效 | OAuth state 校验失败 |
| `INVALID_REFRESH_TOKEN` | Refresh Token 无效 | 刷新 Token 时 Token 无效 |
| `UNSUPPORTED_GRANT_TYPE` | 不支持的授权类型 | grant_type 不是 authorization_code 或 refresh_token |
| `UNSUPPORTED_LOGIN_METHOD` | 不支持的登录方式 | 第三方登录 provider 不支持 |

### 操作失败消息

| ErrorCode | 说明 |
|-----------|------|
| `LOGIN_FAILED` | 登录失败 |
| `REGISTER_FAILED` | 注册失败 |
| `LOGOUT_FAILED` | 登出失败 |
| `SEND_VERIFICATION_EMAIL_FAILED` | 发送验证邮件失败 |
| `VERIFY_EMAIL_FAILED` | 验证邮箱失败 |
| `FORGOT_PASSWORD_FAILED` | 忘记密码请求失败 |
| `RESET_PASSWORD_FAILED` | 重置密码失败 |
| `CHANGE_PASSWORD_FAILED` | 修改密码失败 |
| `REFRESH_TOKEN_FAILED` | 刷新 Token 失败 |
| `REVOKE_TOKEN_FAILED` | 撤销 Token 失败 |
| `EXCHANGE_CODE_FAILED` | 交换授权码失败 |

### MFA 操作失败消息

| ErrorCode | 说明 |
|-----------|------|
| `SETUP_MFA_FAILED` | MFA 设置失败 |
| `VERIFY_MFA_FAILED` | MFA 验证失败 |
| `DISABLE_MFA_FAILED` | MFA 禁用失败 |
| `GET_MFA_STATUS_FAILED` | 获取 MFA 状态失败 |

### 管理员操作失败消息

| ErrorCode | 说明 |
|-----------|------|
| `LIST_USERS_FAILED` | 获取用户列表失败 |
| `GET_USER_FAILED` | 获取用户详情失败 |
| `DISABLE_USER_FAILED` | 禁用用户失败 |
| `ENABLE_USER_FAILED` | 启用用户失败 |
| `SYSTEM_HEALTH_FAILED` | 获取系统健康状态失败 |
| `CLEANUP_FAILED` | 清理数据失败 |

## 错误响应格式

### 标准错误响应

```json
{
  "error": "邮箱或密码错误"
}
```

### 带错误码的响应

```json
{
  "error": "邮箱或密码错误",
  "error_code": "INVALID_CREDENTIALS"
}
```

### 验证错误响应

```json
{
  "error": "请求参数错误",
  "field": "email",
  "message": "邮箱地址格式无效"
}
```

### OAuth 错误响应

```json
{
  "error": "invalid_client",
  "error_description": "无效的客户端"
}
```
