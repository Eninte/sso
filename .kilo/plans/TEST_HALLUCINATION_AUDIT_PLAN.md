# 测试代码幻觉审计计划

## 目标

全面审查所有 45 个测试文件，识别以下类型的"测试幻觉"：

| 类型 | 说明 | 严重性 |
|------|------|--------|
| **T1: 重复测试** | 同一逻辑在多处测试，制造覆盖率假象 | HIGH |
| **T2: 空壳测试** | 仅检查 `assert.NotNil` 或"不panic"，无业务逻辑验证 | HIGH |
| **T3: 弱断言** | 断言过于宽泛（如 `w.Code >= 300`），不验证具体行为 | HIGH |
| **T4: 测试Mock而非逻辑** | 验证的是Mock行为而非Service/Handler实际逻辑 | HIGH |
| **T5: 跳过的关键测试** | `t.Skip()` 掩盖了未实现或失败的功能 | MEDIUM |
| **T6: 隐式集成测试** | 无build tag连接真实外部服务 | MEDIUM |
| **T7: 脆弱异步测试** | 使用 `time.Sleep` 等待异步操作 | MEDIUM |
| **T8: 死代码/幽灵依赖** | 测试中创建但未使用的对象 | LOW |
| **T9: 错误路径缺失** | 仅测试成功路径，未注入错误验证失败处理 | HIGH |
| **T10: 值不匹配** | 硬编码值与实际业务逻辑不符 | HIGH |

---

## 已发现的高优先级问题（预审计发现）

### HIGH — 重复测试

| 文件A | 文件B | 重复内容 |
|-------|-------|---------|
| `handler/handler_test.go` | `handler/handler_extra_test.go` | `TestUserInfoHandler_Handle` 与 `TestUserInfoHandler_HandleFull` 子测试完全重复（未认证-返回401, 返回用户信息） |
| `service/auth_test.go` | `service/auth_test.go` | `TestAuthService_ValidateToken` 与 `TestAuthService_ValidateToken_Extended` 几乎相同 |
| `service/admin_test.go` | `service/admin_test.go` | `TestAdminService_DisableUser_NotFound` 与 `DisableUser` 子测试"禁用不存在的用户"重复 |
| `model/key_test.go` | `crypto/jwt_test.go` | `TestKeyVersion_IsActive`、`TestKeyVersion_CanVerify` 重复 |

### HIGH — 空壳测试

- `service/constructors_test.go` — **全部 11 个测试**仅断言 `assert.NotNil(t, svc)` 或"不应该panic"，零业务逻辑验证
- `TestNewAdminService`、`TestNewMFAService`、`TestOAuthService_NewOAuthServiceWithAudit`、`TestOAuthService_NewOAuthServiceWithCache`

### HIGH — 弱断言

- `handler/handler_extra_test.go` `TestHandlerErrorFunctions` 子测试 "decodeJSON" 仅断言 `assert.True(t, w.Code >= 300)` — 不验证具体错误码
- `logging/logger_test.go` `TestInitForEnv_*` 仅断言"不应该panic"

### HIGH — 测试Mock而非真实逻辑

- `service/social_test.go` `TestSocialLoginService_FindOrCreateUser` 直接调用 `store.GetByEmail`/`store.Create` 测试Mock行为，而非Service的 `findOrCreateUser` 逻辑

### MEDIUM — 跳过的关键测试

- `test/e2e/token_test.go` `TestTokenExpired` — 整个测试跳过
- `test/e2e/email_verify_test.go` — 2个子测试跳过
- `test/e2e/password_reset_test.go` — 1个子测试跳过
- 多个管理员端点测试跳过"端点未实现"

### MEDIUM — 隐式集成测试

- `cache/redis_test.go` Redis测试连接真实Redis（`192.168.1.3:30059`）但无 `-tags=integration`

### MEDIUM — 脆弱异步测试

- `service/audit_test.go` 在 13+ 处使用 `time.Sleep(100ms)` 等待异步写入

### LOW — 死代码

- `handler/handler_test.go` 第350行：`_ = service.NewAuthService(...)` 创建但未使用
- `handler/handler_extra_test.go`：硬编码 `30*60*1000000000` 而非 `30*time.Minute`

---

## 审计阶段

### 阶段 1: 自动化静态扫描（2h）

使用脚本自动检测高概率幻觉模式。

#### 1.1 重复测试检测

```bash
# 查找重复的测试函数名
grep -h "^func Test" --include="*_test.go" -r . | sort | uniq -d

# 查找重复子测试名
grep -h "t.Run(" --include="*_test.go" -r . | sort | uniq -d
```

#### 1.2 空壳/弱断言检测

扫描模式：
- `assert.NotNil` 且无其他断言
- `assert.NoError` 且无后续状态验证
- `不应该panic` 注释
- 宽泛断言如 `assert.True(t, w.Code >= 300)`

```bash
# 查找不应该panic
grep -rn "不应该panic" --include="*_test.go" .

# 查找 time.Sleep 在测试中
grep -rn "time.Sleep" --include="*_test.go" .

# 查找弱断言模式
grep -rn "assert.True.*Code.*>=" --include="*_test.go" .
```

#### 1.3 跳过测试扫描

```bash
grep -rn "t.Skip" --include="*_test.go" .
```

#### 1.4 覆盖率零覆盖检测

```bash
go test -coverprofile=coverage.out ./...
go tool cover -func=coverage.out | grep "0.0%"
```

---

### 阶段 2: 逐文件深度审查（8h）

#### 每个测试函数的审查清单

```
□ 测试函数名是否准确描述被测场景？
□ Given-When-Then 结构是否清晰？
□ Mock 设置是否仅限于必要的依赖隔离？
□ 断言是否验证了具体的业务行为（而非仅"不报错"）？
□ 错误路径是否有对应的错误注入测试？
□ 测试数据是否有意义（非空/零值/nil）？
□ 是否存在重复测试同一逻辑的情况？
□ 测试是否验证了状态变化（而非仅返回值）？
□ 测试通过后，是否有可能生产代码实际是错的？
```

#### 审查顺序（按严重性）

**P0 — Service 层（核心业务逻辑，4.5h）：**

| 文件 | 重点审查内容 | 时间 |
|------|-------------|------|
| `service/auth_test.go` | 35个子测试覆盖所有认证分支；RefreshToken验证；锁定机制 | 60min |
| `service/user_test.go` | 错误注入充分性；密码重置流程完整性 | 45min |
| `service/admin_test.go` | `CleanupExpired`是否验证实际清理结果；重复测试 | 30min |
| `service/oauth_test.go` | 授权码交换流程；Token撤销验证 | 45min |
| `service/social_test.go` | `FindOrCreateUser`是否测试mock而非逻辑 | 45min |
| `service/audit_test.go` | `time.Sleep`脆弱性；异步写入验证 | 30min |
| `service/mfa_test.go` | MFA完整流程验证 | 30min |
| `service/constructors_test.go` | **全部11个测试是否为空壳** | 15min |
| `service/email_test.go` | "集成测试"实际测试连接失败 | 20min |
| `service/keyrotation_test.go` | 密钥轮换状态验证 | 20min |

**P1 — Handler 层（3.5h）：**

| 文件 | 重点审查内容 | 时间 |
|------|-------------|------|
| `handler/handler_test.go` | 死代码（未使用service）；重复测试 | 45min |
| `handler/handler_extra_test.go` | 重复UserInfoHandler测试；弱断言；硬编码纳秒 | 45min |
| `handler/admin_test.go` | 权限检查是否被跳过 | 30min |
| `handler/authorize_test.go` | OAuth授权流程完整性 | 30min |
| `handler/register_test.go` | 与handler_test.go重复 | 20min |
| `handler/user_mfa_test.go` | MFA Handler与Service测试衔接 | 30min |
| `handler/wellknown_test.go` | JWKS/discovery验证 | 15min |

**P2 — 基础设施层（2.5h）：**

| 文件 | 重点审查内容 | 时间 |
|------|-------------|------|
| `store/postgres/postgres_test.go` | 隐含假设（数据库状态） | 30min |
| `cache/redis_test.go` | 无build tag的集成测试 | 20min |
| `crypto/jwt_test.go` | RS256验证完整性；重复 | 30min |
| `crypto/password_test.go` | bcrypt cost设置 | 15min |
| `crypto/keyloader_test.go` | 安全测试完整性 | 15min |
| `middleware/*_test.go` | 认证/授权中间件测试完整性 | 30min |
| `errors/*_test.go` | 错误码一致性 | 15min |
| `config/config_test.go` | 生产环境配置验证 | 15min |

**P3 — E2E 和 SDK（1.5h）：**

| 文件 | 重点审查内容 | 时间 |
|------|-------------|------|
| `test/e2e/*.go` (8文件) | 所有 `t.Skip()` 原因和影响 | 60min |
| `sdks/golang/client_test.go` | SDK与服务端行为一致性 | 30min |

---

### 阶段 3: 交叉验证（4h）

#### 3.1 测试-代码分支映射

对每个关键函数，验证实现代码中所有分支是否都有对应测试：

```go
// 审查示例：auth.go Login()
// 分支1: 用户不存在 → ErrInvalidCredentials  ← 有测试？
// 分支2: 密码错误 → ErrInvalidCredentials    ← 有测试？
// 分支3: 账户锁定 → ErrAccountLocked          ← 有测试？锁定时间验证？
// 分支4: MFA启用 → 返回mfa_required           ← 有测试？
// 分支5: 登录成功 → 返回tokens                ← 有测试？验证token内容？

// 审查示例：oauth.go ExchangeAuthorizationCode()
// 分支1: 授权码不存在 → 错误                  ← 有测试？
// 分支2: 授权码已使用 → ErrAuthCodeUsed       ← 有测试？
// 分支3: 授权码已过期 → 错误                  ← 有测试？
// 分支4: client_id不匹配 → 错误               ← 有测试？
// 分支5: 交换成功 → 返回tokens                ← 有测试？
```

#### 3.2 Mock行为 vs 真实行为对比

关键检查：
- 测试设置 `mockStore.CreateUserErr = errors.New("db error")` 然后直接 `assert.Error`
- **但未验证** Service 是否正确包装了错误（`fmt.Errorf("创建用户失败: %w", err)`）
- **但未验证** Handler 是否返回了正确的HTTP状态码和错误码（`ErrCodeRegisterFailed`）

#### 3.3 预期值一致性检查

| 值类型 | 审查点 |
|--------|--------|
| HTTP状态码 | 测试期望 400/401/403/404/500 是否与 Handler 实际返回一致 |
| 错误消息 | 测试中的 `ErrCode*` 常量是否与 `apperrors` 包定义一致 |
| 时间常数 | 锁定时间=30min、Token过期时间是否在测试中正确验证 |
| 分页参数 | limit/offset 边界值测试是否充分 |

---

### 阶段 4: 安全和边界条件审查（3h）

#### 4.1 安全关键路径检查

| 场景 | 审查点 | 状态 |
|------|--------|------|
| 登录失败锁定 | 5次失败→锁定30分钟？ | □ |
| Token验证 | RS256算法强制？拒绝none/HS256？ | □ |
| 密码哈希 | 测试cost=10合理，但是否有cost>=12的配置验证测试？ | □ |
| 路径遍历 | keyloader是否测试 `../../etc/passwd`？ | □ |
| CORS | 是否测试跨域请求？ | □ |
| 限流 | 是否测试超限请求？ | □ |
| 并发Token刷新 | 是否有竞态测试？ | □ |

#### 4.2 边界值检查

- 空字符串、nil值、超长输入
- 分页边界（第0页、最后一页、超出范围）
- Token过期的精确时间边界

---

### 阶段 5: 生成审计报告和修复建议（2h）

#### 5.1 幻觉测试清单格式

```markdown
### [文件:行号] 测试函数名

**幻觉类型：** T2 空壳测试
**严重性：** HIGH
**描述：** 测试仅检查 `assert.NotNil(t, svc)`，未验证任何业务行为
**风险：** 构造函数可能返回有效对象但内部状态错误，测试无法发现
**修复建议：** 添加验证内部字段的断言，或测试一个关键业务方法
```

#### 5.2 修复优先级

| 优先级 | 条件 | 处理方式 |
|--------|------|---------|
| P0 紧急 | 核心认证流程的空壳/弱断言 | 立即修复 |
| P1 高 | 重复测试、跳过的安全测试 | 本周修复 |
| P2 中 | 脆弱异步测试、隐式集成测试 | 下个迭代修复 |
| P3 低 | 构造函数测试、死代码 | 技术债务 |

#### 5.3 长期防护

1. **Lint规则：** 启用 `testifylint` 检查弱断言
2. **覆盖率门禁：** 分支覆盖率 >= 80%（当前 >= 70%）
3. **Code Review检查清单：** 将审计清单纳入CR流程
4. **E2E测试补全：** 移除所有 `t.Skip()` 或转换为已知问题
5. **Build Tag规范：** 集成测试必须使用 `-tags=integration`
6. **定期审计：** 每季度执行一次本审计

---

## 预计总时间

| 阶段 | 时间 |
|------|------|
| 阶段1: 自动化扫描 | 2h |
| 阶段2: 逐文件审查 | 8h |
| 阶段3: 交叉验证 | 4h |
| 阶段4: 安全边界审查 | 3h |
| 阶段5: 报告生成 | 2h |
| **合计** | **19h** |

## 预期产出

1. **幻觉测试清单** — 所有问题测试的详细列表（文件、行号、类型、严重性）
2. **修复PR** — 按优先级分批修复
3. **测试指南更新** — 更新 `AGENTS.md` 增加测试质量要求
4. **CI增强** — 添加覆盖率门禁和lint规则
