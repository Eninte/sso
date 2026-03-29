# SSO SDK

各语言客户端 SDK，方便第三方应用集成 SSO 认证服务。

## 可用 SDK

| 语言 | 目录 | 导入路径 | 状态 |
|------|------|---------|------|
| Go | [`golang/`](golang/) | `github.com/your-org/sso/sdks/golang` | ✅ 可用 |
| JavaScript/TypeScript | [`js/`](js/) | `@sso/sdk` | ✅ 可用 |
| Python | [`python/`](python/) | `sso-sdk` | ✅ 可用 |
| Rust | [`rust/`](rust/) | `sso-sdk` | ✅ 可用 |

## 快速开始（以 Go 为例）

```go
import sdk "github.com/your-org/sso/sdks/golang"

client := sdk.NewClient("http://localhost:9090")

// 登录
tokens, _ := client.Login(ctx, "user@example.com", "P@ssw0rd1")

// 获取用户信息（Token 过期自动刷新）
info, _ := client.UserInfo(ctx)
```

## 通用特性

所有 SDK 均提供以下能力：

- 完整覆盖 SSO 服务 29 个 API 端点
- Token 自动管理（过期检测 + 自动刷新）
- 类型安全的请求/响应结构
- 统一的错误处理

## 添加新语言 SDK

在 `sdks/` 下创建对应目录：

```
sdks/
├── golang/     # Go
├── js/         # JavaScript/TypeScript
├── python/     # Python
├── rust/       # Rust
└── <lang>/     # 其他语言
```
