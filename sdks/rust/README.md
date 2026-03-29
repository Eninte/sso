# SSO Rust SDK

SSO 单点登录服务的 Rust 客户端 SDK。

## 安装

```toml
[dependencies]
sso-sdk = "1.0.0"
```

## 快速开始

```rust
use sso_sdk::SSOClient;

#[tokio::main]
async fn main() -> Result<(), SSOError> {
    let client = SSOClient::new("http://localhost:9090");

    // 登录（自动保存 Token）
    client.login("user@example.com", "P@ssw0rd1").await?;

    // 获取用户信息（Token 过期自动刷新）
    let info = client.user_info().await?;
    println!("{}", info.email);

    // 登出
    client.revoke_token().await?;
    Ok(())
}
```

## 客户端配置

```rust
// 基本用法
let client = SSOClient::new("http://localhost:9090");

// 预设 Token
let client = SSOClient::new("http://localhost:9090")
    .with_tokens("access_token", "refresh_token", 900);

// 自定义 HTTP 客户端
let http = reqwest::Client::builder()
    .timeout(Duration::from_secs(10))
    .build()?;
let client = SSOClient::new("http://localhost:9090")
    .with_http_client(http);
```

## API 方法

### 认证

| 方法 | 说明 |
|------|------|
| `register(email, password)` | 注册 |
| `login(email, password)` | 登录 |
| `refresh_token()` | 刷新 Token |
| `exchange_code(code, client_id, client_secret, redirect_uri, code_verifier)` | OAuth2 授权码换 Token |
| `revoke_token()` | 登出 |
| `forgot_password(email)` | 发送重置邮件 |
| `reset_password(token, user_id, new_password)` | 重置密码 |
| `verify_email(token, user_id)` | 验证邮箱 |
| `send_verification_email()` | 发送验证邮件 |

### 用户

| 方法 | 说明 |
|------|------|
| `user_info()` | 获取用户信息 |
| `change_password(old_password, new_password)` | 修改密码 |

### MFA

| 方法 | 说明 |
|------|------|
| `mfa_setup()` | 初始化 MFA |
| `mfa_verify(code)` | 验证并启用 MFA |
| `mfa_disable(code)` | 禁用 MFA |
| `mfa_status()` | 获取 MFA 状态 |

### 管理员

| 方法 | 说明 |
|------|------|
| `admin_health()` | 健康检查 |
| `admin_cleanup()` | 清理过期数据 |
| `list_users(page, page_size)` | 用户列表 |
| `get_user(user_id)` | 用户详情 |
| `disable_user(user_id)` | 禁用用户 |
| `enable_user(user_id)` | 启用用户 |

### OIDC

| 方法 | 说明 |
|------|------|
| `discovery()` | OIDC Discovery 配置 |
| `jwks()` | JWKS 公钥 |

## 错误处理

```rust
use sso_sdk::{SSOClient, SSOError, ErrorCode};

match client.login("user@example.com", "wrong").await {
    Err(e) if e.is_unauthorized() => {
        println!("认证失败: {}", e.code);
    }
    Err(e) if e.is_conflict() => { /* ... */ }
    Err(e) if e.is_rate_limited() => { /* ... */ }
    Err(e) => eprintln!("错误: {e}"),
    Ok(_) => {}
}
```

## 目录结构

```
sdks/rust/
├── src/
│   ├── lib.rs        # 导出入口
│   ├── client.rs     # SSOClient（所有 API 方法）
│   ├── models.rs     # 数据模型（serde 序列化）
│   └── errors.rs     # SSOError、ErrorCode
├── tests/
│   └── client_test.rs
├── Cargo.toml
└── README.md
```

## 依赖

| Crate | 用途 |
|-------|------|
| `reqwest` | HTTP 客户端 |
| `serde` / `serde_json` | JSON 序列化 |
| `thiserror` | 错误类型 |
| `tokio` | 异步运行时 |
| `mockito` | 测试 mock（dev） |
