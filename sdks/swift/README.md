# SSO Swift SDK

SSO 单点登录服务的 Swift 客户端 SDK（iOS/macOS）。

## 安装（Swift Package Manager）

在 `Package.swift` 中添加：

```swift
dependencies: [
    .package(url: "https://github.com/your-org/sso.git", from: "1.0.0"),
]
```

或在 Xcode 中：File → Add Package Dependencies → 输入仓库 URL。

## 快速开始

```swift
import SSOSDK

let client = SSOClient(baseURL: "http://localhost:9090")

// 登录
let tokens = try await client.login(email: "user@example.com", password: "P@ssw0rd1")

// 获取用户信息（Token 过期自动刷新）
let info = try await client.userInfo()
print(info.email)

// 登出
try await client.revokeToken()
```

## API 方法

### 认证

| 方法 | 说明 |
|------|------|
| `register(email, password)` | 注册 |
| `login(email, password)` | 登录 |
| `refreshToken()` | 刷新 Token |
| `revokeToken()` | 登出 |
| `forgotPassword(email)` | 发送重置邮件 |
| `resetPassword(token, userId, newPassword)` | 重置密码 |

### 用户

| 方法 | 说明 |
|------|------|
| `userInfo()` | 获取用户信息 |
| `changePassword(oldPassword, newPassword)` | 修改密码 |

### MFA

| 方法 | 说明 |
|------|------|
| `mfaSetup()` | 初始化 MFA |
| `mfaVerify(code)` | 验证并启用 MFA |
| `mfaDisable(code)` | 禁用 MFA |
| `mfaStatus()` | 获取 MFA 状态 |

### 管理员

| 方法 | 说明 |
|------|------|
| `adminHealth()` | 健康检查 |
| `listUsers(page, pageSize)` | 用户列表 |
| `disableUser(userId)` | 禁用用户 |
| `enableUser(userId)` | 启用用户 |

### OIDC

| 方法 | 说明 |
|------|------|
| `discovery()` | OIDC Discovery 配置 |
| `jwks()` | JWKS 公钥 |

## 错误处理

```swift
do {
    try await client.login(email: "user@example.com", password: "wrong")
} catch let e as SSOError {
    if e.isUnauthorized() { print("认证失败") }
    if e.isConflict() { print("冲突") }
    if e.isRateLimited() { print("限流") }
}
```

## 并发模型

`SSOClient` 使用 Swift `actor`，所有方法都是 `async`，Token 状态线程安全。
