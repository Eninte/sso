# SSO Kotlin SDK

SSO 单点登录服务的 Kotlin 客户端 SDK（Android/JVM）。

## 安装（Gradle）

```kotlin
// build.gradle.kts
dependencies {
    implementation("com.sso:sso-sdk:1.0.0")
}
```

## 快速开始

```kotlin
import com.sso.sdk.SSOClient

val client = SSOClient("http://localhost:9090")

// 登录
client.login("user@example.com", "P@ssw0rd1")

// 获取用户信息（Token 过期自动刷新）
val info = client.userInfo()
println(info.email)

// 登出
client.revokeToken()
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

```kotlin
try {
    client.login("user@example.com", "wrong")
} catch (e: SSOError) {
    when {
        e.isUnauthorized() -> println("认证失败")
        e.isConflict() -> println("冲突")
        e.isRateLimited() -> println("限流")
    }
}
```

## 依赖

| 库 | 用途 |
|----|------|
| OkHttp 4.x | HTTP 客户端 |
| Gson | JSON 序列化 |
