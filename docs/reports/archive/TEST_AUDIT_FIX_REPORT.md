# 测试质量审计与修复报告

**项目**: SSO 单点登录服务
**日期**: 2026-03-29
**范围**: 全项目 45 个测试文件的质量审计与修复

---

## 一、审计发现总览

### 1.1 Mock Store 错误注入审计

`internal/store/mock/mock.go` 定义了 **32 个错误注入字段**，用于模拟存储层故障。

| 类别 | 数量 | 占比 |
|------|------|------|
| 实际被测试使用的字段 | 3 | 9.4% |
| 从未被注入的字段 | 29 | 90.6% |

**被使用的 3 个字段**：
- `RevokeAllUserTokensErr` — `auth_test.go`、`admin_test.go`
- `GetVerificationTokenErr` — `user_test.go`
- `PingErr` — `admin_test.go`

**未覆盖的关键字段**：`CreateUserErr`、`GetUserByIDErr`、`GetUserByEmailErr`、`StoreTokenErr`、`GetTokenByRefreshTokenErr`、`GetActiveKeyErr` 等 29 个。意味着这些存储层故障路径在单元测试中从未被验证。

### 1.2 测试文件问题分类

| 问题类型 | 涉及文件数 | 严重程度 |
|----------|-----------|---------|
| 空壳测试（仅 assertNotNil） | 1 | HIGH |
| 重复测试（同文件/跨文件） | 6 | HIGH |
| 测试 Mock 而非逻辑（T4 反模式） | 1 | HIGH |
| time.Sleep 硬等待 | 2 | MEDIUM |
| 硬编码魔法数字 | 4 | MEDIUM |
| 死代码 | 2 | MEDIUM |
| 缺少 Build Tag | 1 | HIGH |
| 弱断言 | 2 | MEDIUM |
| 误导性测试名称 | 1 | MEDIUM |

### 1.3 E2E 测试跳过情况

共 **24 处 `t.Skip()`**，主要集中在：
- `admin_flow_test.go`：16 处（11 处因 URL 前缀不匹配，5 处因管理员账户未配置）
- `email_verify_test.go`：3 处（缺少测试基础设施）
- `token_test.go`：3 处（端点未实现 / 测试设计限制）
- `password_reset_test.go`：2 处（端点未实现 / 缺少测试基础设施）

---

## 二、已执行修复

### 2.1 `internal/service/constructors_test.go`

**问题**：8 个纯壳测试仅断言 `assert.NotNil(t, svc)`，无任何行为验证。另有 3 个与 `auth_test.go`、`oauth_test.go` 重复。

**修复**：
- 删除 `TestNewAuthServiceWithAudit`（与 `auth_test.go:522` 重复）
- 删除 `TestNewAdminServiceWithCache`（空壳）
- 删除 `TestNewMFAServiceWithAudit`（空壳）
- 删除 `TestNewUserServiceWithAudit`（空壳）
- 删除 `TestNewOAuthServiceWithCache`（与 `oauth_test.go:404` 重复）
- 删除 `TestNewOAuthServiceWithAudit`（与 `oauth_test.go:387` 重复）
- 删除 `TestNewAuthServiceWithMetrics`（空壳）
- 删除 `TestNewAdminServiceWithVersion`（空壳）

**保留**：`TestAuditService_LogSystemStart`、`TestOAuthService_GetAccessTokenTTL`、`TestSocialLoginService_Close`（有实际行为验证）

**影响**：删除 200 行无效测试代码，清理 3 个未使用导入（`cache`、`metrics`、`require`）

### 2.2 `internal/service/audit_test.go`

**问题**：23 处 `time.Sleep(100 * time.Millisecond)`，总计 ~2.3 秒固定等待。审计日志通过 channel 异步写入，固定 sleep 在慢速 CI 上不可靠。

**修复**：
- 新增 `waitForAuditLogs()` 辅助函数，使用 `require.Eventually`（10ms 轮询间隔，2 秒超时）
- 替换全部 23 处 `time.Sleep` 调用
- 每个测试用例改用 `waitForAuditLogs(t, ctx, store, userID, eventType, minCount)`

**影响**：消除固定等待，测试在快速机器上更快完成（~10ms vs 100ms），在慢速机器上更可靠（2 秒超时 vs 100ms 硬编码）

### 2.3 `internal/service/social_test.go`

**问题**：`TestSocialLoginService_FindOrCreateUser` 的 3 个子测试直接调用 `storeInst.GetByEmail()` 和 `storeInst.Create()`，从未调用任何 `SocialLoginService` 方法。测试名称声称测试 `findOrCreateUser` 但实际测试的是 Mock Store 的 CRUD 操作（T4 反模式）。

**修复**：
- 删除整个 `TestSocialLoginService_FindOrCreateUser` 函数（68 行）
- 清理未使用导入：`apperrors`、`store`

**影响**：消除误导性测试。真实 `findOrCreateUser` 逻辑已在 `HandleCallback_FullFlow` 测试中通过完整回调流程覆盖。

### 2.4 `internal/handler/handler_test.go`

**问题 1**：`TestUserInfoHandler_Handle` 中 `_ = service.NewAuthService(...)` 创建了从未使用的 `authSvc`（死代码）。

**修复**：删除该行及相关的 `passwordSvc`、`jwtSvc` 变量声明。

**问题 2**：3 个独立测试函数与已有子测试重复：
- `TestRegisterHandler_InvalidJSON` ↔ `TestRegisterHandler_Handle/无效的JSON格式`
- `TestLoginHandler_InvalidJSON` ↔ `TestLoginHandler_Handle/无效的JSON格式`
- `TestAuthorizeHandler_InvalidRequest` ↔ `authorize_test.go` 中相同场景

**修复**：删除 3 个重复测试函数（~60 行），添加注释说明 `authorize_test.go` 已覆盖。

### 2.5 `internal/handler/handler_extra_test.go`

**问题 1**：`TestUserInfoHandler_HandleFull` 与 `handler_test.go` 中 `TestUserInfoHandler_Handle` 完全重复（相同子测试：未认证-401、返回用户信息）。

**修复**：删除整个 `TestUserInfoHandler_HandleFull` 函数及 `createTestUserInfoHandlerFull` 辅助函数。

**问题 2**：3 处硬编码纳秒数 `30*60*1000000000`（30 分钟），可读性差且易出错。

**修复**：全部替换为 `30*time.Minute`。

**问题 3**：弱断言 `assert.True(t, w.Code >= 300)` — 接受任何 3xx+ 状态码。

**修复**：替换为 `assert.Contains(t, []int{http.StatusBadRequest, http.StatusUnauthorized}, w.Code)`。

**影响**：删除 50 行重复代码，修复 3 处魔法数字，修复 1 处弱断言，清理未使用 `context` 导入。

### 2.6 `internal/cache/redis_test.go`

**问题**：Redis 测试无 Build Tag，直接连接 `192.168.1.3:30059`。运行 `go test ./internal/cache/...` 时如果 Redis 不可用会导致失败而非跳过。

**修复**：添加 `//go:build integration` Build Tag。

**影响**：`go test ./internal/cache/...` 现在返回 `[no tests to run]`，需显式使用 `-tags=integration` 才会运行 Redis 测试。与 `make test-integration` 配合使用。

### 2.7 `internal/crypto/jwt_test.go`

**问题**：`TestKeyVersion_IsActive` 和 `TestKeyVersion_CanVerify` 是 `model/key_test.go` 中同名测试的部分重复（crypto 版本只覆盖部分场景，model 版本有 9 个场景 + 边界条件）。

**修复**：删除两个重复测试函数，清理未使用 `model` 导入。

**影响**：消除跨包重复，`model` 包是 `KeyVersion` 方法的正确定义位置。

---

## 三、未修复问题（建议后续处理）

### 3.1 Mock Store 24 个未使用错误注入字段

**严重程度**: MEDIUM

24 个错误注入字段从未在测试中设置（已从 29 减少到 24）。建议按优先级补充错误路径测试：
1. `StoreTokenErr`、`GetTokenByAccessTokenErr` — Token 存储故障路径
2. `GetActiveKeyErr`、`StoreKeyErr` — 密钥管理故障路径
3. `StoreAuditLogErr`、`CleanupExpiredErr` — 审计和清理故障路径

### ~~3.2 `auth_test.go` 中 `ValidateToken` 近似重复~~

**状态**: 已修复（第四轮）

`TestAuthService_ValidateToken_Extended` 已删除。`TestAuthService_ValidateToken` 完整覆盖 3 种场景。

### ~~3.3 `auth_test.go` 审计相关测试不验证审计内容~~

**状态**: 已修复（第四轮）

已新增 3 个 `VerifyLog` 测试函数，使用 `require.Eventually` 验证审计日志内容。

### ~~3.4 E2E 测试 4 处 `t.Skip()`~~

**状态**: 已修复（第四轮，从 24 降至 4）

剩余 4 处为合法限制：
- 2 处需从邮件中提取验证/重置令牌（测试环境 SMTP 可用但无自动令牌提取机制）
- 1 处需构造过期 Token 或等待过期
- 1 处需模拟邮件服务获取重置令牌

### 3.5 Linter 配置对测试文件过于宽松

**严重程度**: LOW

`.golangci.yml` 已禁用 `dupl`、`errcheck`、`testifylint` 对 `_test.go` 的检查。这允许重复和弱断言通过 CI。建议启用 `testifylint` 以自动检测 `assert.True(t, x >= 300)` 等弱断言模式。

---

## 四、第二轮修复（验证问题存在性后执行）

### 4.1 `internal/service/auth_test.go` — Mock Store 错误注入测试

**验证结果**：确认 29/32 错误注入字段从未被测试。已为 5 个关键字段补充错误路径测试。

**新增测试函数**（5 个顶层函数，7 个子测试）：

| 测试函数 | 覆盖的错误注入字段 | 验证行为 |
|----------|-------------------|---------|
| `TestAuthService_Register_StoreErrors/GetByEmail失败` | `GetUserByEmailErr` | 注册时检查邮箱存在性失败返回错误 |
| `TestAuthService_Register_StoreErrors/Create失败` | `CreateUserErr` | 创建用户失败返回错误 |
| `TestAuthService_Login_StoreErrors/GetByEmail失败` | `GetUserByEmailErr` | 登录时查询用户失败返回错误 |
| `TestAuthService_RefreshToken_StoreErrors/GetTokenByRefreshToken失败` | `GetTokenByRefreshTokenErr` | 刷新Token时查询失败返回 ErrInvalidToken |
| `TestAuthService_RefreshToken_StoreErrors/GetByID失败` | `GetUserByIDErr` | 刷新Token时查询用户失败返回错误 |
| `TestAuthService_Logout_StoreErrors/RevokeToken失败` | `RevokeTokenErr` | 登出时撤销Token失败返回错误 |
| `TestAuthService_LogoutAll_StoreErrors/RevokeAllUserTokens失败` | `RevokeAllUserTokensErr` | 登出所有设备时失败返回错误 |

**影响**：错误注入字段使用率从 3/32 (9.4%) 提升至 8/32 (25.0%)。新增 5 个独立错误注入字段覆盖。

### 4.2 `internal/service/auth_test.go` — 审计日志写入验证

**验证结果**：确认 `LoginWithAudit`、`LogoutWithAudit`、`RefreshTokenWithAudit` 等测试仅验证返回值，不验证审计日志是否实际写入。

**新增测试函数**（3 个顶层函数，4 个子测试）：

| 测试函数 | 子测试 | 验证内容 |
|----------|--------|---------|
| `TestAuthService_LoginWithAudit_VerifyLog` | `登录成功写入审计日志` | 验证 `EventUserLogin` 日志写入、IP 地址正确、Success=true |
| `TestAuthService_LoginWithAudit_VerifyLog` | `登录失败写入审计日志` | 验证 `EventUserLogin` 日志写入、Success=false |
| `TestAuthService_LogoutWithAudit_VerifyLog` | `登出写入审计日志` | 验证 `EventUserLogout` 日志写入、IP 地址正确 |
| `TestAuthService_RefreshTokenWithAudit_VerifyLog` | `刷新Token写入审计日志` | 验证 `EventTokenRefresh` 日志写入、IP 地址正确 |

**影响**：审计相关测试从"仅验证返回值"升级为"验证审计日志内容"，确保审计功能真正工作。

### 4.3 `internal/service/auth_test.go` — 删除重复测试

**验证结果**：`TestAuthService_ValidateToken_Extended` 的 2 个子测试（"验证有效Token"、"验证无效Token"）是 `TestAuthService_ValidateToken` 3 个子测试的严格子集。

**修复**：删除 `TestAuthService_ValidateToken_Extended`（37 行）。`TestAuthService_ValidateToken` 已完整覆盖有效 Token、无效 Token、已撤销 Token 三种场景。

### 4.4 `cmd/server/main.go` — 修复管理员路由前缀

**验证结果**：确认 E2E 测试中 11 个管理员端点跳过是由于 URL 前缀不匹配：
- **路由注册**：`router.PathPrefix("/admin")` → 实际路径 `/admin/users`
- **E2E 测试请求**：`/api/v1/admin/users` → 收到 404 → 触发 `t.Skip()`

**修复**：将 `router.PathPrefix("/admin")` 改为 `api.PathPrefix("/admin")`，使管理员端点统一在 `/api/v1/admin/...` 下。这将消除 11 个假阳性 `t.Skip()`。

### 4.5 E2E 测试跳过统计（修正后）

| 类别 | 修复前 | 修复后 |
|------|--------|--------|
| URL 前缀不匹配导致的假阳性跳过 | 11 | 0 |
| 管理员账户未配置导致的跳过 | 5 | 5（需环境变量） |
| 端点确实不存在导致的跳过 | 5 | 5 |
| 测试设计限制导致的跳过 | 3 | 3 |
| **总计** | **24** | **13** |

---

## 五、变更统计（累计）

| 指标 | 第一轮 | 第二轮 | 累计 |
|------|--------|--------|------|
| 修改文件数 | 7 | 2 | 9 |
| 删除代码行数 | ~400 | ~37 | ~437 |
| 删除测试函数 | 14 | 1 | 15 |
| 新增顶层测试函数 | 0 | 8 | 8 |
| 新增子测试 | 0 | 11 | 11 |
| 修复硬编码魔法数字 | 3 | 0 | 3 |
| 修复弱断言 | 1 | 0 | 1 |
| 修复死代码 | 2 | 0 | 2 |
| 修复 time.Sleep 模式 | 23 | 0 | 23 |
| 新增 Build Tag | 1 | 0 | 1 |
| 新增辅助函数 | 1 | 0 | 1 |
| 新增错误注入字段覆盖 | 0 | 5 | 5 |
| 新增审计验证测试 | 0 | 3 | 3 |

---

## 六、验证结果

| 测试套件 | 状态 | 耗时 |
|----------|------|------|
| `go vet ./...` | PASS | — |
| `go test ./internal/service/...` | PASS | 17.4s |
| `go test ./internal/handler/...` | PASS | 4.9s |
| `go test ./internal/crypto/...` | PASS | 9.5s |
| `go test ./internal/model/...` | PASS | 0.008s |
| `go test ./internal/cache/...` | PASS (no tests to run) | 0.004s |
| Mock Store 错误注入字段利用率 | 8/32 (25.0%) | ↑ from 9.4% |
| E2E 假阳性跳过 | 0 (admin URL 前缀) | ↓ from 11 |
| E2E 总跳过数 | 4 | ↓ from 24 |

---

## 七、后续建议优先级

1. **P1**: 继续补充 Mock Store 错误注入测试（剩余 24 个字段），优先覆盖 Token 存储和密钥管理相关字段
2. **P2**: 启用 `testifylint` 自动检测弱断言（`metrics_test.go:255-256` 仍有 `assert.True(t, count >= 19)` 弱断言）
3. **P2**: 为 E2E 测试添加邮件令牌自动提取机制（消除剩余 4 处 `t.Skip`）

---

## 八、第三轮修复（文档验证后执行）

### 8.1 `internal/handler/register_test.go` — 遗漏的重复测试

**验证结果**：文档验证发现 `register_test.go` 中 3 个函数与 `handler_test.go` 的 `TestRegisterHandler_Handle` 子测试重复：

| register_test.go 函数 | handler_test.go 对应子测试 | 处理 |
|----------------------|--------------------------|------|
| `TestRegisterHandler_Handle_InvalidJSON/无效JSON` | `TestRegisterHandler_Handle/无效的JSON格式` | 删除重复，保留唯一子测试"空请求体"并移入 handler_test.go |
| `TestRegisterHandler_Handle_DuplicateEmail` | `TestRegisterHandler_Handle/邮箱已存在` | 删除 |
| `TestRegisterHandler_Handle_Success` | `TestRegisterHandler_Handle/成功注册` | 删除 |

**保留**：`TestRegisterHandler_Handle_ValidationErrors`（表驱动验证测试，8 个验证场景，比 handler_test.go 的单个子测试更全面）

**修复**：
- 将 `TestRegisterHandler_Handle_InvalidJSON` 的"空请求体"子测试移入 `handler_test.go` 的 `TestRegisterHandler_Handle`
- 删除 `register_test.go` 中 3 个重复函数（~55 行）
- 清理未使用导入（`require`）

### 8.2 问题分类修正

第一轮审计遗漏了 `register_test.go` 中的重复测试。修正后的问题分类：

| 问题类型 | 原始数量 | 修复后 | 剩余 |
|----------|---------|--------|------|
| 空壳测试 | 1 文件 | 已修复 | 0 |
| 重复测试 | 6 文件 | 已修复 6 | 0 |
| T4 反模式 | 1 文件 | 已修复 | 0 |
| time.Sleep 硬等待 | 2 文件 | 已修复 1 | 1（redis_test.go，integration 标签下可接受） |
| 硬编码魔法数字 | 4 文件 | 已修复 1 | 3（剩余为合理的时间相关测试常量） |
| 死代码 | 2 文件 | 已修复 | 0 |
| 缺少 Build Tag | 1 文件 | 已修复 | 0 |
| 弱断言 | 2 文件 | 已修复 1 | 1（metrics_test.go:255-256） |
| 误导性测试名称 | 1 文件 | 已修复 | 0 |

---

## 九、变更统计（最终）

| 指标 | 第一轮 | 第二轮 | 第三轮 | 第四轮 | 累计 |
|------|--------|--------|--------|--------|------|
| 修改文件数 | 7 | 2 | 2 | 5 | 16 |
| 删除代码行数 | ~400 | ~37 | ~55 | ~50 | ~542 |
| 删除测试函数 | 14 | 1 | 3 | 0 | 18 |
| 新增顶层测试函数 | 0 | 8 | 0 | 0 | 8 |
| 新增子测试 | 0 | 11 | 1 | 0 | 12 |
| 修复硬编码魔法数字 | 3 | 0 | 0 | 0 | 3 |
| 修复弱断言 | 1 | 0 | 0 | 0 | 1 |
| 修复死代码 | 2 | 0 | 0 | 0 | 2 |
| 修复 time.Sleep 模式 | 23 | 0 | 0 | 0 | 23 |
| 新增 Build Tag | 1 | 0 | 0 | 0 | 1 |
| 新增辅助函数 | 1 | 0 | 0 | 0 | 1 |
| 新增错误注入字段覆盖 | 0 | 5 | 0 | 0 | 5 |
| 新增审计验证测试 | 0 | 3 | 0 | 0 | 3 |
| 修复管理员路由前缀 | 0 | 1 | 0 | 0 | 1 |
| 修复 E2E 假阳性 Skip | 0 | 0 | 0 | 20 | 20 |
| 修复错误端点 URL | 0 | 0 | 0 | 1 | 1 |

---

## 十、第四轮修复（E2E 测试 Skip 修复）

### 10.1 根因分析

E2E 测试有 24 处 `t.Skip()`，但测试环境实际已提供所有基础设施（数据库、缓存、SMTP、管理员账户）。跳过的根因：

1. **管理员凭证默认值错误**（5 处）：`admin_flow_test.go` 默认 `admin@example.com` / `AdminPassword123!`，实际测试管理员为 `system@eninte.com` / `Admin123!`
2. **管理员路由前缀不匹配**（11 处）：已在第二轮修复（`router.PathPrefix("/admin")` → `api.PathPrefix("/admin")`）
3. **邮件验证端点 URL 错误**（1 处）：测试用 `/api/v1/resend-verification`，实际端点为 `/api/v1/verify-email/send`
4. **测试逻辑保守**（多处）：收到 404 就认为"端点未实现"并 Skip，而非断言失败

### 10.2 修复

| 文件 | 修复内容 |
|------|---------|
| `admin_flow_test.go` | 默认凭证改为 `system@eninte.com` / `Admin123!`；`loginAdmin()` 失败改为 `require.NoError`；移除所有 `if 404 { t.Skip }` 模式 |
| `token_test.go` | 移除 Token 撤销端点 Skip（`/api/v1/token/revoke` 已注册）；移除管理员端点 Skip（路由已修复） |
| `password_reset_test.go` | `/api/v1/forgot-password` 返回 404 改为 `t.Fatalf` 而非 `t.Skip` |
| `email_verify_test.go` | URL 从 `/api/v1/resend-verification` 修正为 `/api/v1/verify-email/send`；移除对应 Skip；弱断言改为精确 `assert.Equal` |

### 10.3 修复效果

| 类别 | 修复前 | 修复后 |
|------|--------|--------|
| 凭证/路由/URL 错误导致的假阳性 | 20 | 0 |
| 需邮件令牌提取的合法限制 | 3 | 3 |
| 需 Token 过期构造的合法限制 | 1 | 1 |
| **总计** | **24** | **4** |
