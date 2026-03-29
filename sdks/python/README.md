# SSO Python SDK

SSO 单点登录服务的 Python 客户端 SDK，零外部依赖（仅使用标准库）。

## 安装

```bash
pip install sso-sdk
```

## 快速开始

```python
from sso_sdk import SSOClient

client = SSOClient("http://localhost:9090")

# 注册
client.register("user@example.com", "P@ssw0rd1")

# 登录（自动保存 Token）
client.login("user@example.com", "P@ssw0rd1")

# 获取用户信息（Token 过期自动刷新）
info = client.user_info()
print(info.email)

# 登出
client.revoke_token()
```

## 客户端配置

```python
# 基本用法
client = SSOClient("http://localhost:9090")

# 预设 Token
client = SSOClient("http://localhost:9090", access_token="eyJhbGci...", refresh_token="dGhpc...")

# 自定义超时（秒）
client = SSOClient("http://localhost:9090", timeout=10)
```

## API 方法

### 认证

| 方法 | 说明 |
|------|------|
| `register(email, password)` | 注册 |
| `login(email, password)` | 登录 |
| `refresh_token()` | 刷新 Token |
| `exchange_code(code, client_id, client_secret, redirect_uri)` | OAuth2 授权码换 Token |
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

### OAuth2

| 方法 | 说明 |
|------|------|
| `authorize(client_id, redirect_uri, scope, state)` | 获取授权码 |
| `authorize_with_pkce(...)` | 获取授权码（PKCE） |

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

```python
from sso_sdk import SSOClient, SSOError

try:
    client.login("user@example.com", "wrong")
except SSOError as e:
    print(e.http_status)  # 401
    print(e.code)         # "INVALID_CREDENTIALS"
    print(e.message)      # "邮箱或密码错误"

    if e.is_unauthorized(): ...
    if e.is_conflict(): ...
    if e.is_rate_limited(): ...
```

## 目录结构

```
sdks/python/
├── sso_sdk/
│   ├── __init__.py     # 导出入口
│   ├── client.py       # SSOClient 类（所有 API 方法）
│   ├── models.py       # 数据模型（dataclass）
│   └── errors.py       # SSOError 类、错误码常量
├── tests/
│   └── test_client.py  # 测试（18 个用例）
├── pyproject.toml
└── README.md
```
