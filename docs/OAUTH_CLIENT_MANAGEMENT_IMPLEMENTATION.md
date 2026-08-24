# OAuth 2.0 / OIDC 客户端管理 — AI 执行实施任务文档

> **文档用途**：本文档面向 AI 编码代理，目标是把 SSO 服务的 OAuth 客户端（应用注册）能力从"仅初始化面板创建"补齐为"生产级管理"。执行前先通读本文档与 `AGENTS.md`，遵循项目全部工程规范。
>
> **行号说明**：文中引用的行号基于撰写时的代码快照，执行时以符号名搜索为准，行号仅作参考。

---

## 1. 背景与现状

当前系统（OAuth 2.0 Provider + OIDC）的客户端注册能力存在以下缺口（已核实）：

| 能力 | 现状 |
|------|------|
| 创建客户端 | 仅 `POST /api/v1/init/client`（初始化面板，loopback-only，配置硬编码） |
| 更新客户端（回调地址 / scopes / 名字） | ❌ 无。`ClientStore` 仅有 `CreateClient` / `GetByClientID` / `ValidateRedirectURI` |
| 删除 / 禁用客户端 | ❌ 无 |
| 客户端列表与查询 | ❌ 无。`/api/v1/admin/*` 无任何 client 路由 |
| 密钥轮换（rotate client secret） | ❌ 无 |
| 公共客户端（SPA / 移动端，无 secret + PKCE） | ❌ 创建入口硬编码 `PublicClient: false` |

关键代码锚点：

- 客户端模型：`internal/model/model.go:72`（`Client` 结构体）
- 存储接口：`internal/store/store.go:84`（`ClientStore`）
- Postgres 实现：`internal/store/postgres/client.go`
- Mock 实现：`internal/store/mock/mock.go:403-441`
- 初始化创建逻辑：`internal/service/init.go:134`（`CreateOAuthClient`，注意其 grant types / scopes / public_client 均硬编码）
- 管理员服务：`internal/service/admin.go:44`（`AdminServiceInterface`）、`:79`（`AdminService`，选项模式注入依赖）
- 管理员 Handler：`internal/handler/admin.go`（分页、`mux.Vars`、`middleware.GetUserIDFromContext` 取 operatorID 的样板）
- 管理路由注册：`internal/app/router.go:204-227`（admin 子路由，继承 `AuthMiddleware` + `RequireAdmin`）
- 客户端缓存：`internal/service/oauth.go:99`（`getClient`，缓存键 `cache.ClientKey(clientID)`）——**所有客户端读取的必经点**
- 审计事件常量：`internal/model/audit.go:64-69`（`admin.user_*` 命名模式）
- 表结构：`migrations/002_create_clients.up.sql`（`oauth_clients`）、`migrations/003_create_tokens.up.sql`（`tokens` / `authorization_codes` 对 `oauth_clients(client_id)` 的 FK **无 ON DELETE 动作**，直接删客户端会被外键阻塞）
- 最新迁移版本：`020`（下一个为 `021`）
- OpenAPI 规范：`internal/handler/templates/openapi.json`（embed 于 `internal/handler/api_docs.go:27`）

## 2. 目标与非目标

### 2.1 目标

1. 管理员可通过 `/api/v1/admin/clients` 系列端点完成客户端的**创建、查询单个、分页列表、更新、禁用/启用、删除、密钥轮换**。
2. 创建时支持**公共客户端**（`public_client=true`，不生成 secret；PKCE 已全局强制，见 `internal/service/oauth.go:177`）。
3. 禁用 / 删除 / 轮换密钥时**撤销该客户端全部未撤销 Token** 并**失效客户端缓存**（安全优先）。
4. 被禁用的客户端无法发起新的授权码流程或 Token 交换。
5. 每个操作记录审计日志，遵守 `auditutil.SafeAuditLog` 规范。
6. 单元 / 集成 / E2E 测试齐备，整体覆盖率 ≥ 80%，新增 service 层核心逻辑 ≥ 90%。

### 2.2 非目标（本期不做，防止范围蔓延）

- **RFC 7591 动态客户端注册**（开放注册端点安全面大：需 initial access token、注册速率限制、software statement 校验）。留待后续单独任务。
- **`client_credentials` 授权类型**：Token 端点未实现该流程（`internal/handler/token.go:46-54` 的 switch 仅处理 `refresh_token` 与 `authorization_code`，无 `client_credentials` 分支），注册时必须**拒绝**该 grant，避免注册出不可用的客户端。
- 客户端自有管理门户（自助注册 UI），本期只做管理员 API。
- OIDC Discovery 文档变更（不新增 `registration_endpoint`）。

## 3. 关键设计决策（执行时不得偏离）

| # | 决策 | 理由 |
|---|------|------|
| D1 | 管理路由统一使用**公开 `client_id`**（UUID 字符串）作为路径参数，不用内部 `id` | 管理员在审计日志 / OAuth 流程里看到的就是 `client_id`；且 `GetByClientID` 已存在 |
| D2 | 删除为**硬删除**，通过迁移 021 把 FK 改为：`authorization_codes.client_id → ON DELETE CASCADE`，`tokens.client_id → ON DELETE SET NULL`；删除事务内先撤销该客户端全部未撤销 Token | Token 行保留（`model.Token.ClientID` 本就是 `*string` 可空），授权码是短命数据直接级联 |
| D3 | **轮换密钥 = 生成新 secret + 撤销该客户端全部未撤销 Token + 失效缓存** | OAuth Security BCP：secret 泄露场景下必须假定 Token 已泄露 |
| D4 | **禁用 = `is_active=false` + 撤销全部未撤销 Token + 失效缓存**；启用不恢复已撤销的 Token | 安全优先，与 D3 同理；启用后用户重新走授权流程即可 |
| D5 | `public_client` **创建后不可变**（更新请求中出现该字段且与当前值不同 → 400） | 机密↔公共转换涉及 secret 状态机，复杂且无实际收益 |
| D6 | 运行时（授权 / 换 Token / 刷新）对禁用客户端返回**现有 `ErrInvalidClient`（400）**，不新增"已禁用"专用错误，避免向公网泄露客户端状态 | 与现有 `getClient` 错误语义一致；管理 API 内部才区分 404/状态 |
| D7 | 启用/禁用接口**幂等**（对已禁用客户端再次禁用返回成功） | 与现有用户启用/禁用实现保持一致（执行前阅读 `handleUserStatusChange` 确认语义并与之对齐） |
| D8 | secret 生成复用 `internal/service/init.go` 的 `generateRandomHex(32)` + `passwordSvc.HashPassword`（bcrypt）哈希后存储，明文**仅创建/轮换响应中返回一次** | 与初始化流程完全一致 |
| D9 | 校验复用：redirect URI 校验复用 `service` 包内 `validateRedirectURI`（`internal/service/init.go:189`）；scope 校验复用 `model.IsSupportedScope`（全局白名单） | 不重复造轮子 |
| D10 | `getClient`（`internal/service/oauth.go:99`）是客户端读取唯一入口，`is_active` 检查加在此处（缓存命中与未命中都要检查） | 单一咽喉点，覆盖授权码签发与交换两条路径（`CreateAuthorizationCode` / `CreateAuthorizationCodeWithConsent` / `validateClientSecret` 均经 `getClient`）；刷新路径（`auth_token.go` 的 `RefreshTokenWithAudit`）不做客户端读取、仅比对 client_id 归属，由禁用/轮换/删除时先行的 Token 撤销兜底（见 D3/D4） |

## 4. 变更总览

| 层 | 新建文件 | 修改文件 |
|----|---------|---------|
| Model | — | `internal/model/model.go`、`internal/model/model_test.go`、`internal/model/audit.go` |
| 迁移 | `migrations/021_add_client_management.up.sql` / `.down.sql` | — |
| Store 接口 | — | `internal/store/store.go` |
| Postgres | `internal/store/postgres/client_admin_test.go`（integration tag） | `internal/store/postgres/client.go` |
| Mock | — | `internal/store/mock/mock.go` |
| Service | `internal/service/admin_client.go`、`internal/service/admin_client_test.go` | `internal/service/admin.go`（接口）、`internal/service/oauth.go`（getClient 检查）、`internal/util/serviceutil/`（缓存失效 helper）、`internal/cache/redis.go`（ClientCachePrefix 版本化，见 Phase 4.4） |
| Errors | — | `internal/errors/errors.go` |
| Handler | `internal/handler/admin_client.go`、`internal/handler/admin_client_test.go` | `internal/app/router.go`、`internal/handler/templates/openapi.json` |
| E2E | `test/e2e/admin_client_test.go` | — |
| 文档 | — | `AGENTS.md`（第 10 节路由表）、`README.md`、`docs/ARCHITECTURE.md`、`docs/DATABASE_SCHEMA.md`、`docs/CHANGELOG.md` |

## 5. API 规格

所有端点挂在现有 admin 子路由下（继承 JWT 认证 + `RequireAdmin`，无需额外中间件）。

### 5.1 创建客户端

```
POST /api/v1/admin/clients
```

请求：

```json
{
  "name": "内部 Wiki",
  "redirect_uris": ["https://wiki.example.com/oauth/callback"],
  "grant_types": ["authorization_code", "refresh_token"],
  "scopes": ["openid", "profile", "email"],
  "public_client": false
}
```

- `name`：必填，1-255 字符。
- `redirect_uris`：必填，≥1 条，每条须通过 `validateRedirectURI`；去重后 ≤ 20 条。
- `grant_types`：可选，默认 `["authorization_code","refresh_token"]`；必须是 `{authorization_code, refresh_token}` 子集，且**必须包含 `authorization_code`**；含 `client_credentials` → 400。
- `scopes`：可选，默认 `["openid","profile","email"]`；非空、每项须通过 `model.IsSupportedScope`、去重。
- `public_client`：可选，默认 `false`。

响应 `201`（机密客户端，secret 明文仅此一次返回）：

```json
{
  "client_id": "<uuid>",
  "client_secret": "<64位hex>",
  "name": "内部 Wiki",
  "redirect_uris": ["https://wiki.example.com/oauth/callback"],
  "grant_types": ["authorization_code", "refresh_token"],
  "scopes": ["openid", "profile", "email"],
  "public_client": false,
  "is_active": true,
  "created_at": "2026-08-22T00:00:00Z"
}
```

公共客户端（`public_client=true`）响应中**无 `client_secret` 字段**（`omitempty`）。

### 5.2 客户端列表

```
GET /api/v1/admin/clients?page=1&pageSize=20
```

分页语义与 `GET /api/v1/admin/users` 完全一致（`DefaultPageSize=20`、`MaxPageSize=100`，见 `internal/handler/admin.go:21-27`）。响应包含 `clients`（数组，**绝不包含 secret**）、`total`、`page`、`page_size`、`total_pages`。

### 5.3 查询单个客户端

```
GET /api/v1/admin/clients/{client_id}
```

响应同 5.1 的对象结构（无 secret）。不存在 → 404 `CLIENT_NOT_FOUND`。

### 5.4 更新客户端

```
PUT /api/v1/admin/clients/{client_id}
```

请求（全量替换语义，字段校验规则同创建）：

```json
{
  "name": "内部 Wiki v2",
  "redirect_uris": ["https://wiki.example.com/oauth/callback", "https://wiki2.example.com/cb"],
  "grant_types": ["authorization_code", "refresh_token"],
  "scopes": ["openid", "email"]
}
```

- `public_client` **不接受修改**（出现且与当前值不同 → 400，错误码 `PUBLIC_CLIENT_IMMUTABLE`）。
- 更新成功后**失效该客户端缓存**。
- 响应为更新后的客户端对象（无 secret）。

### 5.5 禁用 / 启用

```
POST /api/v1/admin/clients/{client_id}/disable
POST /api/v1/admin/clients/{client_id}/enable
```

- 禁用：`is_active=false` + 撤销该客户端全部未撤销 Token + 失效缓存（见 D4）。
- 启用：`is_active=true` + 失效缓存。幂等。
- 响应：`{"message": "客户端已禁用"}` / `{"message": "客户端已启用"}`（与用户启用/禁用的响应格式对齐）。

### 5.6 轮换密钥

```
POST /api/v1/admin/clients/{client_id}/rotate-secret
```

- 仅机密客户端可用；公共客户端 → 400（错误码 `SECRET_NOT_SUPPORTED_FOR_PUBLIC_CLIENT`）。
- 行为：生成新 secret → bcrypt 哈希入库 → 撤销该客户端全部未撤销 Token → 失效缓存（见 D3）。
- 响 `200`：`{"client_id": "...", "client_secret": "<新明文，仅此一次>"}`。

### 5.7 删除客户端

```
DELETE /api/v1/admin/clients/{client_id}
```

行为（单事务，见 D2）：撤销该客户端全部未撤销 Token → 删除其授权码（FK CASCADE 自动完成）→ 删除客户端行 → 失效缓存。响应 `200`：`{"message": "客户端已删除"}`。

## 6. 分阶段任务

> 执行顺序必须按 Phase 0 → 8。每个 Phase 内遵循 TDD：先写测试（红）→ 实现（绿）。**每完成一个 Phase 运行该 Phase 的验证命令，全部通过后才进入下一 Phase。**

### Phase 0：基线准备

1. 从 `main` 创建分支 `feat/client-management`。
2. 运行 `make test-unit` 确认基线全绿。
3. 通读以下文件（执行各 Phase 的前置依赖）：
   - `internal/handler/admin.go`（Handler 样板：分页、`mux.Vars`、`middleware.GetUserIDFromContext`、`service.WithClientIP`、`writeError/writeJSON/getMessage`）
   - `internal/service/admin.go`（选项模式构造、`serviceutil.HandleStoreError` 用法、`auditutil.SafeAuditLog` 用法）
   - `internal/service/init.go:134-210`（客户端创建与校验的现有实现）
   - `internal/store/postgres/client.go` + `internal/store/mock/mock.go:403-441`
   - `AGENTS.md` 第 7、8、16 节

**验收**：分支创建、基线测试通过。

### Phase 1：Model 与数据库迁移

**1.1 Model 扩展** — `internal/model/model.go`

为 `Client` 结构体追加两个字段（放在 `PublicClient` 之后）：

```go
IsActive  bool      `json:"is_active" db:"is_active"`
UpdatedAt time.Time `json:"updated_at" db:"updated_at"`
```

> 注意：`UpdatedAt` 不加 `omitempty`——该标签对非指针 `time.Time` 无效，且与 `CreatedAt` 风格一致。保证响应中该值非零的方式见 Phase 4.3 的"写后重读"规则。

新增请求/响应模型（放在 `Client` 之后）：

```go
// ClientCreateRequest 管理员创建客户端请求
type ClientCreateRequest struct {
    Name         string   `json:"name"`
    RedirectURIs []string `json:"redirect_uris"`
    GrantTypes   []string `json:"grant_types"`
    Scopes       []string `json:"scopes"`
    PublicClient bool     `json:"public_client"`
}

// ClientUpdateRequest 管理员更新客户端请求（public_client 不可变）
type ClientUpdateRequest struct {
    Name         string   `json:"name"`
    RedirectURIs []string `json:"redirect_uris"`
    GrantTypes   []string `json:"grant_types"`
    Scopes       []string `json:"scopes"`
    PublicClient *bool    `json:"public_client,omitempty"` // 非空时必须与当前值一致
}
```

新增辅助函数：

```go
// IsSupportedGrantType 判断授权类型是否被服务端支持（client_credentials 未实现，不属于支持集合）
func IsSupportedGrantType(g string) bool  // 支持 authorization_code、refresh_token
```

同步更新 `internal/model/model_test.go`：覆盖 `IsSupportedGrantType` 全部分支。

**1.2 迁移** — 用 `make migrate-create NAME=add_client_management` 生成 `021_*` 对（确认序号为 021，若已被占用则顺延），内容：

```sql
-- 021_add_client_management.up.sql
-- 注：不加显式 BEGIN/COMMIT，与现有迁移（002/003）风格一致；golang-migrate 单文件批量执行具备原子性
ALTER TABLE oauth_clients ADD COLUMN updated_at TIMESTAMP DEFAULT NOW();
ALTER TABLE oauth_clients ADD COLUMN is_active BOOLEAN NOT NULL DEFAULT TRUE;

-- 注意：003 迁移未命名约束，默认名为 <表>_<列>_fkey。
-- 执行前先在测试库确认：SELECT conname FROM pg_constraint WHERE conrelid = 'authorization_codes'::regclass AND contype='f';
ALTER TABLE authorization_codes DROP CONSTRAINT IF EXISTS authorization_codes_client_id_fkey;
ALTER TABLE authorization_codes ADD CONSTRAINT authorization_codes_client_id_fkey
    FOREIGN KEY (client_id) REFERENCES oauth_clients(client_id) ON DELETE CASCADE;

ALTER TABLE tokens DROP CONSTRAINT IF EXISTS tokens_client_id_fkey;
ALTER TABLE tokens ADD CONSTRAINT tokens_client_id_fkey
    FOREIGN KEY (client_id) REFERENCES oauth_clients(client_id) ON DELETE SET NULL;
```

```sql
-- 021_add_client_management.down.sql
ALTER TABLE tokens DROP CONSTRAINT IF EXISTS tokens_client_id_fkey;
ALTER TABLE tokens ADD CONSTRAINT tokens_client_id_fkey
    FOREIGN KEY (client_id) REFERENCES oauth_clients(client_id);
ALTER TABLE authorization_codes DROP CONSTRAINT IF EXISTS authorization_codes_client_id_fkey;
ALTER TABLE authorization_codes ADD CONSTRAINT authorization_codes_client_id_fkey
    FOREIGN KEY (client_id) REFERENCES oauth_clients(client_id);
ALTER TABLE oauth_clients DROP COLUMN IF EXISTS is_active;
ALTER TABLE oauth_clients DROP COLUMN IF EXISTS updated_at;
```

> ⚠️ down 迁移在存在历史 token 行时可能因 FK 校验失败，属预期（down 仅用于开发环境回滚）。

**1.3 Postgres 已有查询同步**：`internal/store/postgres/client.go` 的 `GetByClientID` / `CreateClient` 的 SELECT/INSERT 列表补上 `is_active, updated_at`（Scan 与参数同步）。CreateClient 入参的 `UpdatedAt` 若为零值由数据库 DEFAULT 填充，INSERT 不必包含该列。

**验证**：`make test-unit`；加载 `.env.test` 后 `make migrate-up` + `make migrate-down` + 再 `make migrate-up` 验证可逆性（在测试库执行）。

**验收**：迁移可上可下；既有测试全绿（Postgres 集成测试在 Phase 2 验证）。

### Phase 2：Store 层（接口 + Postgres + Mock）

**2.1 接口** — `internal/store/store.go` 的 `ClientStore` 追加：

```go
type ClientStore interface {
    CreateClient(ctx context.Context, client *model.Client) error
    GetByClientID(ctx context.Context, clientID string) (*model.Client, error)
    ValidateRedirectURI(ctx context.Context, clientID, redirectURI string) bool
    // ↓ 新增
    ListClients(ctx context.Context, offset, limit int) ([]*model.Client, int, error)
    UpdateClient(ctx context.Context, client *model.Client) error
    DeleteClient(ctx context.Context, clientID string) error      // 单事务：撤销token→删授权码→删客户端
    RotateClientSecret(ctx context.Context, clientID, secretHash string) error
    SetClientActive(ctx context.Context, clientID string, active bool) error
    RevokeAllClientTokens(ctx context.Context, clientID string) error // 供禁用/轮换复用（非事务场景）
}
```

**2.2 Postgres 实现** — `internal/store/postgres/client.go` 追加，遵循现有风格（`withTimeout`、`store.ErrNotFound` 映射、`scanTextArray`）：

- `ListClients`：`SELECT ... FROM oauth_clients ORDER BY created_at DESC LIMIT $1 OFFSET $2` + `SELECT COUNT(*)`。
- `UpdateClient`：`UPDATE oauth_clients SET name=$2, redirect_uris=$3, grant_types=$4, scopes=$5, updated_at=NOW() WHERE client_id=$1`，`RowsAffected==0` → `store.ErrNotFound`。
- `RotateClientSecret`：`UPDATE ... SET client_secret=$2, updated_at=NOW() WHERE client_id=$1`，同样处理 NotFound。
- `SetClientActive`：`UPDATE ... SET is_active=$2, updated_at=NOW() WHERE client_id=$1`。
- `RevokeAllClientTokens`：`UPDATE tokens SET revoked_at=NOW() WHERE client_id=$1 AND revoked_at IS NULL`。
- `DeleteClient`：**单事务**（事务写法参照 `internal/store/postgres/token.go:217` 的 `RotateRefreshToken` 或 `mfa_recovery.go:65`，均使用 `s.db.BeginTx(ctx, nil)` + defer rollback）：
  1. `UPDATE tokens SET revoked_at=NOW() WHERE client_id=$1 AND revoked_at IS NULL`
  2. `DELETE FROM authorization_codes WHERE client_id=$1`（显式删除，FK CASCADE 兜底）
  3. `DELETE FROM oauth_clients WHERE client_id=$1`，`RowsAffected==0` 回滚并返回 `store.ErrNotFound`
- 错误一律原样返回或映射 `store.ErrNotFound`，由 Service 层经 `serviceutil.HandleStoreError` 统一处理。

**2.3 Mock 实现** — `internal/store/mock/mock.go`：

- `clients map[string]*model.Client` 语义不变；新增方法全部实现。
- **严格遵守 AGENTS.md 7.5**：写操作必须"浅拷贝→修改副本→替换 map 指针"，禁止原地修改已存对象（`-race` 会检出）。
- `tokens` map 中的 token 同理：`RevokeAllClientTokens` / `DeleteClient` 对 token 的修改必须拷贝替换。
- 测试可复用现有 `AddClient` helper（mock.go:1111）造数据。
- `DeleteClient` 从 `clients` map 删除并按上述语义撤销相关 token。

**2.4 集成测试** — 新建 `internal/store/postgres/client_admin_test.go`（`//go:build integration`），连接用 `testutil.ConnectTestDB(t)`，覆盖：

| 用例 | 断言 |
|------|------|
| Create→Get 往返 | 新字段 `is_active=true`、`updated_at` 非零 |
| ListClients 分页 | total 正确、按 created_at DESC、offset/limit 生效 |
| UpdateClient | 字段变更生效；不存在 → `store.ErrNotFound` |
| RotateClientSecret | 哈希变更；不存在 → `store.ErrNotFound` |
| SetClientActive | 状态翻转 |
| DeleteClient | 客户端行删除；其授权码被删；其历史 token 行**仍在且 `client_id IS NULL`、`revoked_at` 非空；有未撤销 token 时先撤销再删** |
| RevokeAllClientTokens | 仅撤销该 client 的未撤销 token，其他 client 的 token 不受影响 |

**验证**：

```bash
make test-unit
set -a && source .env.test && set +a && make test-integration
```

**验收**：全部通过；`go build ./...` 无错（Store 接口新增方法后 Mock 与 Postgres 均已实现，编译即验证完整性）。

### Phase 3：Errors + 审计事件

**3.1 错误** — `internal/errors/errors.go`。

**先复用（已存在，勿重复定义）**：

- `ErrCodeInvalidRedirectURI` / `ErrInvalidRedirectURI`（errors.go:66、255，400）——redirect URI 校验失败
- `ErrCodeInvalidScope` / `ErrInvalidScope`（errors.go:74、263，400）——scope 不在白名单
- `ErrCodeUnsupportedGrantType`（errors.go:384，Handler 消息区常量，值 `UNSUPPORTED_GRANT_TYPE`，token.go:53 的 default 分支已用它返回 400）——**已存在，勿重复定义**；新增 `ErrUnsupportedGrant` 错误值时直接复用该常量

**新增（已核实不存在）**，按现有 `ErrCode*` / `Err*` 成对模式（错误消息中文、HTTP 状态码见括号）：

```go
ErrCodeClientNotFound    ErrorCode = "CLIENT_NOT_FOUND"      // 404
ErrCodePublicImmutable   ErrorCode = "PUBLIC_CLIENT_IMMUTABLE" // 400
ErrCodeNoSecretForPublic ErrorCode = "SECRET_NOT_SUPPORTED_FOR_PUBLIC_CLIENT" // 400
// 另补错误值：ErrUnsupportedGrant = New(ErrCodeUnsupportedGrantType, "...", 400)（常量已存在，勿新建同名常量）
```

> 新增前仍需 grep 一遍确认无重名（`grep -n "CLIENT_NOT_FOUND\|PUBLIC_CLIENT_IMMUTABLE\|SECRET_NOT_SUPPORTED" internal/errors/errors.go` 应无结果；`UNSUPPORTED_GRANT` 会命中已有的 `ErrCodeUnsupportedGrantType`，属预期）。

**3.2 审计事件** — `internal/model/audit.go`，在 `admin.user_*` 之后按同模式新增：

```go
EventClientCreated       AuditEventType = "admin.client_created"        // 管理员创建客户端
EventClientUpdated       AuditEventType = "admin.client_updated"        // 管理员更新客户端
EventClientDeleted       AuditEventType = "admin.client_deleted"        // 管理员删除客户端
EventClientSecretRotated AuditEventType = "admin.client_secret_rotated" // 管理员轮换客户端密钥
EventClientEnabled       AuditEventType = "admin.client_enabled"        // 管理员启用客户端
EventClientDisabled      AuditEventType = "admin.client_disabled"       // 管理员禁用客户端
```

**验收**：`make test-unit` 通过；grep 确认新错误码无重名。

### Phase 4：Service 层

**4.1 缓存失效 helper** — 在 `internal/util/serviceutil/` 中参照 `InvalidateUserTokenCache` 新增：

```go
// InvalidateClientCache 失效客户端缓存（客户端配置变更后必须调用）
func InvalidateClientCache(ctx context.Context, c cache.Cache, clientID string)
```

内部调用 `c.Delete(ctx, cache.ClientKey(clientID))`（`Cache` 接口已有 `Delete` 方法，见 `internal/cache/redis.go:38`；与 `InvalidateUserTokenCache` 同文件 `internal/util/serviceutil/token_cache.go`）。

**4.2 接口扩展** — `internal/service/admin.go` 的 `AdminServiceInterface` 新增"客户端管理"方法组：

```go
// 客户端管理
ListClients(ctx context.Context, offset, limit int) ([]*model.Client, int, error)
GetClient(ctx context.Context, clientID string) (*model.Client, error)
CreateClient(ctx context.Context, operatorID string, req model.ClientCreateRequest) (*model.Client, string, error)
UpdateClient(ctx context.Context, operatorID string, clientID string, req model.ClientUpdateRequest) (*model.Client, error)
DeleteClient(ctx context.Context, operatorID, clientID string) error
RotateClientSecret(ctx context.Context, operatorID, clientID string) (string, error)
EnableClient(ctx context.Context, operatorID, clientID string) error
DisableClient(ctx context.Context, operatorID, clientID string) error
```

> 注意：新增接口方法会破坏所有实现方。全局搜索 `AdminServiceInterface` 的实现（含测试中的 stub），逐一补齐。

**4.3 实现** — 新建 `internal/service/admin_client.go`，方法挂在 `AdminService` 上：

- `AdminService` 需要新增依赖 `passwordSvc *crypto.PasswordService`（**具体类型**，与 `InitService` 相同，见 `internal/service/init.go:34`）。按现有选项模式新增 `WithAdminPasswordService(pw *crypto.PasswordService)` 选项；装配点在 `internal/app/services.go:191`（`NewAdminServiceWithOptions(...)` 调用处追加该选项，`passwordSvc` 变量在该文件 79 行已构造好，直接传入）。
- **写后重读规则**：所有写操作（Create/Update/Rotate/SetActive/Delete）完成后，Service 返回前一律重新 `GetByClientID` 取数据库真实值再返回/使用，保证 `updated_at`、`is_active` 非零且准确（INSERT 不写 `updated_at`，依赖 DB DEFAULT，内存对象里是零值）。
- **CreateClient**：
  1. `validateClientRequest`（新建的包内函数，Create/Update 共用）：校验 name / redirect_uris（复用 `validateRedirectURI`，`internal/service/init.go:189`）/ grant_types（`model.IsSupportedGrantType` 子集且含 `authorization_code`）/ scopes（`model.IsSupportedScope` + 非空 + 去重，可复用 `model.NormalizeScopes`）。
  2. 机密客户端：`generateRandomHex(32)`（包级函数，`internal/service/init.go:215`）生成 secret，`passwordSvc.HashPassword` 哈希；公共客户端：secret 留空。
  3. `client_id = uuid.New().String()`，填充默认值（grant types / scopes 缺省值同 5.1 规格）、`IsActive=true`。
  4. `store.CreateClient` → 写后重读 → `auditutil.SafeAuditLog(EventClientCreated)`（metadata 含 client_id、client_name、public_client；**绝不包含 secret**）。
- **UpdateClient**：GetByClientID → 不存在返回 `ErrClientNotFound`（经 `HandleStoreError` 或手动映射）；`req.PublicClient != nil && *req.PublicClient != client.PublicClient` → `ErrPublicClientImmutable`；校验后 `store.UpdateClient` → 写后重读 → 失效缓存 → 审计。
- **RotateClientSecret**：Get → 公共客户端返回 `ErrNoSecretForPublicClient`；生成新 secret 并哈希 → **先** `store.RevokeAllClientTokens` **再** `store.RotateClientSecret` → 失效缓存 → 审计（metadata 不含 secret）。
  > 顺序理由（失败模式分析）：先撤销后换钥，若第二步失败，旧 secret 仍有效、Token 已死，管理员重试即可；反过来（先换钥后撤销）失败时会留下"新 secret 已生效但旧 Token 仍存活"的更危险状态。
- **DisableClient**：**先** `store.RevokeAllClientTokens` **再** `store.SetClientActive(false)` → 失效缓存 → 审计。幂等。顺序理由同上：若 SetActive 失败，最多是 Token 已死但客户端仍可重新授权（与接口报错一致），不会出现"报告已禁用但 refresh token 仍可用"的假安全感。
- **EnableClient**：`store.SetClientActive(true)` → 失效缓存 → 审计。
- **DeleteClient**：`store.DeleteClient`（事务内完成 token 撤销）→ 失效缓存 → 审计。
- 所有 store 错误经 `serviceutil.HandleStoreError(err, apperrors.ErrClientNotFound)` 映射；操作上下文用 `serviceutil.WrapServiceError`。
- 审计日志必须用 `auditutil.SafeAuditLog` / `CriticalAuditLog`，禁止直接 `auditSvc.Log()`；`SafeAuditLog` 的 userID 参数传 **operatorID**，metadata 含 client_id（写法对齐 `DisableUser` 的审计实现，执行前先阅读）。

**4.4 运行时禁用拦截 + 存量缓存兼容（发布关键点）** — `internal/service/oauth.go` 的 `getClient`（约 99 行）：

在缓存命中返回前与 DB 读取后统一检查：

```go
if client != nil && !client.IsActive {
    return nil, ErrInvalidClient
}
```

（缓存命中的 struct 已含 `IsActive`，单点检查即可覆盖两条路径。）

**必须同时做**：把 `internal/cache/redis.go:54` 的 `ClientCachePrefix` 从 `"client:"` 版本化为 `"client:v2:"`。原因：生产 Redis 中存在升级前写入的旧客户端缓存条目，反序列化到新增的 `IsActive` 字段会得到零值 `false`，不做键版本化的话，升级后所有缓存命中的客户端都会被上述检查误判为"已禁用"，**导致全部 OAuth 流程瘫痪**。改前缀后旧键自然失配（过期后自动清理），读写两侧（`oauth.go` 的 get/set）共用 `ClientKey` 函数，无需其他改动。`ClientKey` 的调用方已核实仅有 `getClient` 一处，可安全修改。

**4.5 单元测试** — 新建 `internal/service/admin_client_test.go`（`package service_test` 黑盒，mock store，表驱动）：

| 组 | 用例要点 |
|----|---------|
| 校验 | name 空/超长；redirect_uris 空/非法 scheme/含 fragment/重复；grant_types 含 `client_credentials`、缺 `authorization_code`、未知值；scopes 空/不在白名单 |
| Create | 机密：返回 secret 为 64 hex、入库的是 bcrypt 哈希（≠明文）；公共：无 secret 入库；审计事件被记录（mock 断言） |
| Update | 不存在→ErrClientNotFound；改 public_client→400；成功后缓存失效被调用（mock cache 断言） |
| Rotate | 新旧 secret 不同；哈希入库；RevokeAllClientTokens 被调用；公共客户端→400 |
| Disable/Enable | 状态翻转 + token 撤销 + 缓存失效；幂等 |
| Delete | store.DeleteClient 被调用；缓存失效；不存在→ErrClientNotFound |
| getClient 拦截 | `is_active=false` → `ErrInvalidClient`（授权码签发与交换两条调用路径各一例） |

**验证**：`make test-unit`（service 层全部）。

**验收**：新增 service 代码行覆盖率 ≥ 90%（`go test ./internal/service/ -coverprofile` 后 `go tool cover -func` 查看）。

### Phase 5：Handler 与路由

**5.1 Handler** — 新建 `internal/handler/admin_client.go`，方法挂在**现有 `AdminHandler`** 上（与用户管理同构，复用 `writeError/writeJSON/getMessage`、`DefaultPageSize/MaxPageSize`、`mux.Vars(r)["client_id"]`、`operatorID := middleware.GetUserIDFromContext(r.Context())`、`ctx := service.WithClientIP(r.Context(), extractClientIP(r))`）：

| 方法 | 路由 | 说明 |
|------|------|------|
| `HandleCreateClient` | POST `/api/v1/admin/clients` | 201；机密客户端响应含一次性 secret |
| `HandleListClients` | GET `/api/v1/admin/clients` | 分页，绝不返回 secret |
| `HandleGetClient` | GET `/api/v1/admin/clients/{client_id}` | 404 映射 |
| `HandleUpdateClient` | PUT `/api/v1/admin/clients/{client_id}` | 400/404 映射 |
| `HandleDisableClient` | POST `/api/v1/admin/clients/{client_id}/disable` | 幂等 |
| `HandleEnableClient` | POST `/api/v1/admin/clients/{client_id}/enable` | 幂等 |
| `HandleRotateClientSecret` | POST `/api/v1/admin/clients/{client_id}/rotate-secret` | 响应一次性新 secret |
| `HandleDeleteClient` | DELETE `/api/v1/admin/clients/{client_id}` | 200 |

要求：

- 请求解析复用包内 `decodeJSON`。
- Service 错误到 HTTP 状态码的映射：优先用 `apperrors` 自带 status（参照 `internal/handler/admin.go` 中 `HandleDeleteUser` 保留 service 错误状态码的写法），禁止一律 500。
- 响应中**任何路径都不得出现 `client_secret` 哈希**（`model.Client.ClientSecret` 已是 `json:"-"`，转换响应对象时也不得手工复制该字段）。

**5.2 路由注册** — `internal/app/router.go` admin 子路由块（约 204-227 行）追加：

```go
admin.HandleFunc("/clients", adminHandler.HandleCreateClient).Methods("POST")
admin.HandleFunc("/clients", adminHandler.HandleListClients).Methods("GET")
admin.HandleFunc("/clients/{client_id}", adminHandler.HandleGetClient).Methods("GET")
admin.HandleFunc("/clients/{client_id}", adminHandler.HandleUpdateClient).Methods("PUT")
admin.HandleFunc("/clients/{client_id}", adminHandler.HandleDeleteClient).Methods("DELETE")
admin.HandleFunc("/clients/{client_id}/rotate-secret", adminHandler.HandleRotateClientSecret).Methods("POST")
admin.HandleFunc("/clients/{client_id}/enable", adminHandler.HandleEnableClient).Methods("POST")
admin.HandleFunc("/clients/{client_id}/disable", adminHandler.HandleDisableClient).Methods("POST")
```

（注意 gorilla/mux 路由顺序：含路径参数的路由与精确路由 `/clients` 无冲突，但注册顺序保持与用户路由块一致的书写风格。）

**5.3 Handler 测试** — 新建 `internal/handler/admin_client_test.go`，参照 `internal/handler/admin_test.go` 的测试装配方式，覆盖：每个端点的成功路径、参数校验 400、NotFound 404、机密/公共客户端差异、secret 只在创建/轮换响应出现一次。

**验证**：`make test-unit`；`make run` 后手工 curl 冒烟（创建→列表→更新→轮换→禁用→删除，用管理员 JWT）。

### Phase 6：E2E 测试

新建 `test/e2e/admin_client_test.go`（`//go:build e2e`），参照 `test/e2e/` 现有测试的服务地址与认证获取方式，编写完整流程：

1. 管理员登录获取 token → 创建机密客户端（断言 secret 存在）；
2. 创建公共客户端 → 用它发起授权码流程（PKCE），断言可以走通到 code 签发（公共客户端接入能力回归）；
3. 更新回调地址 → 用新回调地址发起授权成功、旧地址被拒（`INVALID_REDIRECT_URI`）;
4. 轮换 secret → 旧 secret 换 token 失败（400 `INVALID_CLIENT`，注意 `ErrInvalidClient` 的 HTTP 状态码是 400 而非 401，见 errors.go:254）、新 secret 成功；轮换后旧 refresh token 被撤销；
5. 禁用客户端 → 授权码流程返回 400 `INVALID_CLIENT`（同上，以 `ErrInvalidClient` 既有状态码为准）；启用后恢复；
6. 删除客户端 → `GET /admin/clients/{id}` 返回 404，授权流程 `INVALID_CLIENT`；
7. 越权检查：非管理员 token 访问 → 403。

**验证**：

```bash
RATE_LIMIT_REQUESTS=0 make run &
make test-e2e-prepare && make test-e2e && make test-e2e-cleanup
```

### Phase 7：文档与规范同步

按 `AGENTS.md` 第 17 节要求同步：

1. `AGENTS.md` 第 10 节路由表追加 8 条 `/api/v1/admin/clients*` 路由。
2. `README.md` API 端点列表同步。
3. `docs/ARCHITECTURE.md`：数据模型补充 `oauth_clients` 新字段。
4. `docs/DATABASE_SCHEMA.md`：迁移 021 说明。
5. `docs/CHANGELOG.md`：新增条目。
6. `internal/handler/templates/openapi.json`：补充 5.1-5.7 全部端点的 OpenAPI 定义。认证写法以文件内既有受保护端点为模板（撰写本档时未核实其 securitySchemes 具体命名，执行时先读该文件再动笔）。
7. 检查 `sdks/`：管理端 API 为新增能力，确认 SDK 无破坏性变更；如 SDK 有 admin 模块，追加客户端管理方法为可选增强（不强制）。

### Phase 8：全量验证与收尾

```bash
make test              # 全部测试（-race）
make lint              # go vet + golangci-lint
make test-security     # gosec + govulncheck
make test-coverage     # 覆盖率 ≥ 80% 强制检查
```

逐项核对第 8 节验收清单后提交（提交信息按仓库现有 conventional 风格，如 `feat(oauth): 管理员客户端管理 API（CRUD/密钥轮换/禁用）`，可按 Phase 拆分多次提交）。

## 7. 全局工程约束（违反即返工）

1. **错误处理**：Store 层返回预定义错误；Service 层 `fmt.Errorf("%w")` / `serviceutil.HandleStoreError` / `WrapServiceError`；Handler 层用包内既有响应 helper；禁止 `errors.New` 裸建错误；禁止向响应泄露内部错误详情。
2. **审计**：只用 `auditutil.SafeAuditLog` / `CriticalAuditLog`；metadata 永远不含 secret 明文或哈希。
3. **Mock**：写操作拷贝替换，禁止原地修改共享指针（`-race` 检出即返工）。
4. **测试**：表驱动、黑盒（`package xxx_test`）、每个测试独立 mock；禁止 `t.Skip()`（唯一例外：真实 DB/Redis 未配置）；真实 DB 用 `testutil.ConnectTestDB`，禁止在测试外层自加重试。
5. **注释与错误消息语言**：导出函数必须有文档注释；允许中文注释。错误消息语言存在两层惯例，执行时以相邻代码为准：`apperrors` 预定义错误的 message 为中文（errors.go 现状，如"无效的客户端"）；service 包内校验函数返回的错误为英文（init.go `validateRedirectURI` 现状，如 "redirect URI cannot be empty"）。注意 `AGENTS.md` 7.3 写的是"错误消息使用英文"，与 errors.go 实际现状不一致，本任务跟随相邻代码而非该条字面要求。
6. **导入**：标准库 → 第三方 → 项目包三段分组；禁止点导入。
7. **不提交**：`.env*`、密钥、覆盖率文件；不读取 `.env.test` 内容到文档或代码。
8. **迁移**：只新增 021，不改旧迁移。

## 8. 最终验收清单

- [ ] 管理员可创建机密/公共客户端，公共客户端可完成 PKCE 授权码全流程
- [ ] 列表 / 查询 / 更新可用，更新回调地址后立即生效（缓存已失效）
- [ ] `public_client` 创建后不可变（尝试修改返回 400）
- [ ] 轮换 secret：新 secret 可用、旧 secret 立即失效、该客户端全部 refresh token 被撤销；撤销先于换钥执行
- [ ] 禁用客户端后：授权码流程与 Token 交换均返回 400 `INVALID_CLIENT`；缓存已失效；撤销先于状态翻转执行
- [ ] `cache.ClientCachePrefix` 已版本化（`client:` → `client:v2:`），升级后存量 Redis 缓存条目不会把客户端误判为禁用（Phase 4.4 发布关键点）
- [ ] 删除客户端后：授权码被清、历史 token 行保留但 `client_id` 置空且已撤销、客户端 404
- [ ] `client_credentials` 注册被拒绝（400 `UNSUPPORTED_GRANT_TYPE`）
- [ ] 所有管理操作有审计日志（`admin.client_*`），日志无敏感信息
- [ ] 非管理员访问全部新端点返回 403，未认证返回 401
- [ ] `make test` / `make lint` / `make test-security` / `make test-coverage`（≥80%）全部通过
- [ ] 迁移 021 up/down 可逆
- [ ] `AGENTS.md` 路由表、README、OpenAPI、DATABASE_SCHEMA、CHANGELOG 已同步
