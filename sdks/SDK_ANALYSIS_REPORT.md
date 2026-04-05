# SDK 代码分析报告

> 生成时间: 2026-04-05
> 审查范围: sdks/ 目录下全部 6 个语言 SDK

---

## 概览

| SDK | 语言 | 端点覆盖 | 测试用例 | 状态 |
|-----|------|----------|----------|------|
| [Go](golang/) | Go 1.26+ | 29/29 | 27 | 有严重 typo |
| [JS/TS](js/) | TypeScript | 29/29 | 22 | 良好 |
| [Python](python/) | Python 3.10+ | 29/29 | 18 | 有严重 bug |
| [Rust](rust/) | Rust 2021 | 29/29 | ~17 | 有类型错误 |
| [Kotlin](kotlin/) | Kotlin 1.9+ | 17/29 | 7 | 覆盖不全 |
| [Swift](swift/) | Swift 5.9+ | 17/29 | 7 | 覆盖不全 |

---

## 严重 Bug (P0/P1)

### Bug #1 — Python `register` 参数错乱

**文件**: `python/sso_sdk/client.py:106`

```python
# 当前（错误）
resp = self._request("POST", "/api/v1/register", {"email": password})

# 应为
resp = self._request("POST", "/api/v1/register", {"email": email, "password": password})
```

**影响**: 注册功能完全损坏。密码被当作邮箱发送，真实邮箱和真实密码均未发送到服务端。

---

### Bug #2 — Rust `enable_user` 使用错误请求类型

**文件**: `rust/src/client.rs:495-499`

```rust
// 当前（错误）
pub async fn enable_user(&self, user_id: &str) -> Result<MessageResponse, SSOError> {
    self.request(
        Method::POST,
        "/admin/users/enable",
        Some(&DisableUserRequest {  // <-- 应为 EnableUserRequest
            user_id: user_id.to_string(),
        }),
        true,
    )
    .await
}
```

**影响**: 虽然 `DisableUserRequest` 和 `EnableUserRequest` 字段结构相同（都是 `user_id`），序列化结果一致，但类型语义错误，且如果未来两个结构体字段分叉会立刻出 bug。此外 `EnableUserRequest` 在 `models.rs` 中未定义。

---

### Bug #3 — Go 错误消息前缀 typo

**文件**: `golang/client.go:112`

```go
// 当前
return nil, fmt.Errorf("sdo: no access token available, please login first")

// 应为
return nil, fmt.Errorf("sso: no access token available, please login first")
```

**影响**: 错误消息前缀不一致，其他所有 SDK 和 Go SDK 其余位置均使用 `"sso:"` 前缀。

---

## 中等问题 (P2)

### #4 — Kotlin 竞态条件

**文件**: `kotlin/src/main/kotlin/com/sso/sdk/SSOClient.kt:148-204`

使用 `@Volatile` 修饰三个独立字段，但 `ensureToken()` 中对 `_accessToken` 和 `_refreshToken` 的读取-刷新-写入不是原子操作。多线程并发调用时可能出现重复刷新或 Token 丢失。

**建议**: 使用 `AtomicReference` 封装 TokenState，或在 `ensureToken` 方法上加 `synchronized`。

---

### #5 — Swift `tokenExpiry` 初始化值

**文件**: `swift/SSOSDK/Client.swift:29`

```swift
self.tokenExpiry = Date()  // 初始化为"现在"
```

`ensureToken()` 中判断逻辑为 `Date() > tokenExpiry.addingTimeInterval(-30)`。初始化为 `Date()` 意味着 `Date() > Date().addingTimeInterval(-30)` 为 true，导致首次调用时就误判为需要刷新（即使没有 refresh token 也会走到错误分支）。

**建议**: 初始化为 `Date.distantPast` 或 `Date(timeIntervalSince1970: 0)`。

---

### #6 — Rust `url_encode` 不完整

**文件**: `rust/src/client.rs:526-533`

手工实现的 URL 编码仅处理了 `%`, ` `, `?`, `&`, `=`, `+`, `#` 七个字符，遗漏了 `/`, `~`, `!`, `$`, `'`, `(`, `)`, `*`, `,`, `;`, `:` 等。

**建议**: 使用 `percent-encoding` crate 的 `utf8_percent_encode`。

---

## 轻微问题 (P3)

### #7 — 错误码常量不一致

各 SDK 定义的错误码数量差异较大：

| SDK | 数量 | 缺失的错误码 |
|-----|------|-------------|
| Go | 20 | — |
| JS | 20 | — |
| Python | 17 | `INVALID_REQUEST_FORMAT`, `REQUEST_BODY_TOO_LARGE`, `EMAIL_REQUIRED` |
| Rust | 14 | `EMAIL_INVALID`, `EMAIL_REQUIRED`, `PASSWORD_TOO_SHORT`, `PASSWORD_TOO_LONG`, `PASSWORD_REQUIRED`, `INVALID_REQUEST_FORMAT`, `REQUEST_BODY_TOO_LARGE` |
| Swift | 10 | 大量缺失 |
| Kotlin | 8 | 大量缺失 |

---

### #8 — Go 未使用的类型

**文件**: `golang/types.go:198-202`

`OAuthProvider` 结构体已定义但未在任何 API 方法中使用。

---

### #9 — JS 未使用的请求类型

**文件**: `js/src/types.ts:5-63`

定义了 `RegisterRequest`, `LoginRequest`, `TokenRequest`, `RevokeRequest`, `ForgotPasswordRequest`, `ResetPasswordRequest`, `ChangePasswordRequest`, `AuthorizeApproveRequest`, `MFAVerifyRequest`, `DisableUserRequest`, `EnableUserRequest` 等接口，但 `client.ts` 中全部使用内联对象字面量，未引用这些类型。

---

### #10 — Swift 请求体类型安全性弱

**文件**: `swift/SSOSDK/Client.swift:69-70`

```swift
if let body = body {
    req.httpBody = try JSONEncoder().encode(body)
}
```

`body` 参数类型为 `Encodable?`，但实际传入的是 `[String: String]` 字典。字典的 `Codable` 实现依赖运行时反射，无法在编译期保证键名正确。

---

### #11 — Kotlin 仅支持 GET/POST

**文件**: `kotlin/src/main/kotlin/com/sso/sdk/SSOClient.kt:176`

```kotlin
when (method) {
    "GET" -> builder.get()
    "POST" -> { ... }
}
```

不支持 PUT/DELETE/PATCH。当前 API 确实只用到 GET 和 POST，但缺乏扩展性。

---

### #12 — Python `get_user` 默认值问题

**文件**: `python/sso_sdk/client.py:268`

```python
return UserItem(**{k: resp.get(k, "") for k in UserItem.__dataclass_fields__})
```

对所有字段使用 `""` 作为默认值，但 `email_verified` 和 `mfa_enabled` 是 bool 类型，`""` 会被传入导致 `TypeError`。

---

## 测试覆盖评估

| SDK | 测试文件 | 用例数 | 覆盖范围 | 备注 |
|-----|---------|--------|----------|------|
| **Go** | `golang/client_test.go` | 27 | 注册、登录、用户信息、撤销、刷新、密码、MFA、管理员、OIDC、错误、Token 管理 | 最完整 |
| **JS/TS** | `js/src/client.test.ts` | 22 | 注册、登录、用户信息、撤销、密码、MFA、管理员、OIDC、错误、Token 管理 | 良好 |
| **Python** | `python/tests/test_client.py` | 18 | 注册、登录、用户信息、撤销、密码、MFA、管理员、OIDC、错误、Token 管理 | 良好，但缺少 register bug 的测试 |
| **Rust** | `rust/tests/client_test.rs` | ~17 | 注册、登录、用户信息、撤销、MFA、管理员、OIDC、错误、Token 管理（含 mockito 集成测试） | 良好 |
| **Swift** | `swift/Tests/SSOSDKTests.swift` | 7 | 创建、Token 管理、无 Token 错误、错误方法、错误解析、模型解码 | 无网络 mock |
| **Kotlin** | `kotlin/src/test/.../SSOClientTest.kt` | 7 | 创建、Token 管理、无 Token 错误、错误方法、错误解析、模型解码 | 无网络 mock |

---

## 修复优先级

| 优先级 | Bug | 影响范围 |
|--------|-----|----------|
| **P0** | Python `register` email/password 错乱 | 注册功能完全不可用 |
| **P1** | Rust `enable_user` 使用 `DisableUserRequest` | 类型错误，维护隐患 |
| **P1** | Go `"sdo"` typo | 错误消息不一致 |
| **P2** | Kotlin 竞态条件 | 多线程下 Token 可能丢失 |
| **P2** | Swift `tokenExpiry` 初始化 | 首次调用可能误判刷新 |
| **P2** | Rust `url_encode` 不完整 | 特殊字符参数会出错 |
| **P3** | 错误码不一致 / 未使用类型 / 类型安全 | 维护性和一致性 |
