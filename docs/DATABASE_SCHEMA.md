# 数据库 Schema 参考

本文档描述 SSO 服务的 PostgreSQL 数据库结构。

## 概览

| 表名 | 说明 | 记录类型 |
|------|------|----------|
| `users` | 用户账户 | 核心 |
| `oauth_clients` | OAuth 客户端注册 | 核心 |
| `tokens` | Access/Refresh Token 存储 | 核心 |
| `authorization_codes` | OAuth 授权码 | 临时 |
| `verification_tokens` | 邮箱验证令牌 | 临时 |
| `reset_tokens` | 密码重置令牌 | 临时 |
| `audit_logs` | 审计日志 | 只增 |
| `key_versions` | JWT 密钥版本管理 | 核心 |
| `mfa_recovery_codes` | MFA 恢复码 | 核心 |

## 1. users — 用户账户

用户核心信息表。

| 列 | 类型 | 约束 | 默认值 | 说明 |
|---|------|------|--------|------|
| `id` | UUID | PK | `gen_random_uuid()` | 用户唯一标识 |
| `email` | VARCHAR(255) | UNIQUE, NOT NULL | — | 用户邮箱 |
| `password_hash` | VARCHAR(255) | NULLABLE | NULL | bcrypt 哈希；社交登录用户为 NULL |
| `email_verified` | BOOLEAN | NOT NULL | FALSE | 邮箱是否已验证 |
| `mfa_enabled` | BOOLEAN | NOT NULL | FALSE | 是否启用 MFA |
| `mfa_secret` | VARCHAR(255) | NULLABLE | NULL | TOTP 密钥（加密存储） |
| `status` | VARCHAR(20) | NOT NULL | `'active'` | `active` / `locked` / `disabled` |
| `login_attempts` | INTEGER | NOT NULL | 0 | 连续登录失败次数 |
| `locked_until` | TIMESTAMP | NULLABLE | NULL | 账户锁定截止时间 |
| `role` | VARCHAR(20) | NOT NULL | `'user'` | `user` / `admin`（CHECK 约束） |
| `created_at` | TIMESTAMP | NOT NULL | `NOW()` | 创建时间 |
| `updated_at` | TIMESTAMP | NOT NULL | `NOW()` | 更新时间 |

### 索引

| 索引名 | 列 | 类型 | 用途 |
|--------|-----|------|------|
| `idx_users_email` | `email` | UNIQUE | 邮箱唯一性约束 |
| `idx_users_status` | `status` | BTREE | 按状态过滤查询 |
| `idx_users_created_at` | `created_at DESC` | BTREE | 按创建时间排序 |
| `idx_users_role` | `role` | BTREE | 按角色过滤 |

### 状态流转

```
active ──(5次登录失败)──> locked ──(30分钟后)──> active
   │                                              │
   └──────(管理员操作)─────> disabled ──(管理员)───┘
```

## 2. oauth_clients — OAuth 客户端

注册的第三方/内部客户端应用。

| 列 | 类型 | 约束 | 默认值 | 说明 |
|---|------|------|--------|------|
| `id` | UUID | PK | `gen_random_uuid()` | 内部 ID |
| `client_id` | VARCHAR(255) | UNIQUE, NOT NULL | — | OAuth 公开标识符 |
| `client_secret` | VARCHAR(255) | NULLABLE | NULL | 哈希后的密钥；公开客户端为 NULL |
| `name` | VARCHAR(255) | NOT NULL | — | 客户端显示名称 |
| `redirect_uris` | TEXT[] | NOT NULL | `'{}'` | 允许的回调地址白名单 |
| `grant_types` | TEXT[] | NOT NULL | `'{authorization_code}'` | 允许的授权类型 |
| `scopes` | TEXT[] | NOT NULL | `'{openid,profile,email}'` | 允许的权限范围 |
| `public_client` | BOOLEAN | NOT NULL | FALSE | 是否为公开客户端（SPA/移动应用） |
| `created_at` | TIMESTAMP | NOT NULL | `NOW()` | 创建时间 |

### 索引

| 索引名 | 列 | 类型 | 用途 |
|--------|-----|------|------|
| `idx_oauth_clients_client_id` | `client_id` | UNIQUE | 按 client_id 查找 |

## 3. tokens — Token 存储

Access Token 和 Refresh Token 的持久化存储，支持撤销和过期管理。

| 列 | 类型 | 约束 | 默认值 | 说明 |
|---|------|------|--------|------|
| `id` | UUID | PK | `gen_random_uuid()` | 内部 ID |
| `access_token` | VARCHAR(500) | UNIQUE, NOT NULL | — | JWT Access Token |
| `refresh_token` | VARCHAR(500) | UNIQUE, NULLABLE | NULL | 32 字节随机 Refresh Token |
| `user_id` | UUID | FK → users(id) ON DELETE CASCADE | — | 所属用户 |
| `client_id` | VARCHAR(255) | FK → oauth_clients(client_id), NULLABLE | NULL | 所属客户端 |
| `scopes` | TEXT[] | NULLABLE | `'{}'` | 已授予的权限范围 |
| `expires_at` | TIMESTAMP | NOT NULL | — | 过期时间 |
| `created_at` | TIMESTAMP | NOT NULL | `NOW()` | 创建时间 |
| `revoked_at` | TIMESTAMP | NULLABLE | NULL | 撤销时间；NULL 表示未撤销 |

### 索引

| 索引名 | 列 | 类型 | 用途 |
|--------|-----|------|------|
| `idx_tokens_user_id` | `user_id` | BTREE | 按用户查找 Token |
| `idx_tokens_access_token` | `access_token` | UNIQUE | 按 Access Token 查找 |
| `idx_tokens_refresh_token` | `refresh_token` | UNIQUE | 按 Refresh Token 查找 |
| `idx_tokens_expires_at` | `expires_at` | BTREE | 过期清理查询 |
| `idx_tokens_user_active` | `user_id` WHERE `revoked_at IS NULL` | PARTIAL | 活跃 Token 查询 |
| `idx_tokens_expired` | `expires_at` WHERE `expires_at < NOW()` | PARTIAL | 过期 Token 查询 |
| `idx_tokens_client_id` | `client_id` | BTREE | 按客户端查找 |

### 级联删除

当用户被删除时，其所有 Token 自动级联删除。

## 4. authorization_codes — 授权码

OAuth 授权码的一次性存储。

| 列 | 类型 | 约束 | 默认值 | 说明 |
|---|------|------|--------|------|
| `code` | VARCHAR(255) | PK | — | 授权码（一次性使用） |
| `client_id` | VARCHAR(255) | FK → oauth_clients(client_id) | — | 客户端 ID |
| `user_id` | UUID | FK → users(id) ON DELETE CASCADE | — | 用户 ID |
| `redirect_uri` | VARCHAR(500) | NOT NULL | — | 注册的回调地址 |
| `scopes` | TEXT[] | NULLABLE | `'{}'` | 已授予的权限范围 |
| `code_challenge` | VARCHAR(255) | NULLABLE | NULL | PKCE 挑战码 |
| `code_challenge_method` | VARCHAR(10) | NULLABLE | NULL | PKCE 方法（`S256` / `plain`） |
| `expires_at` | TIMESTAMP | NOT NULL | — | 过期时间（短有效期，通常 10 分钟） |
| `created_at` | TIMESTAMP | NOT NULL | `NOW()` | 创建时间 |
| `used_at` | TIMESTAMP | NULLABLE | NULL | 使用时间；NULL 表示未使用 |

### 索引

| 索引名 | 列 | 类型 | 用途 |
|--------|-----|------|------|
| `idx_authorization_codes_client_id` | `client_id` | BTREE | 按客户端查找 |
| `idx_authorization_codes_expires_at` | `expires_at` | BTREE | 过期清理查询 |
| `idx_authorization_codes_unused` | `code` WHERE `used_at IS NULL` | PARTIAL | 未使用授权码查询 |
| `idx_authorization_codes_user_id` | `user_id` | BTREE | 按用户查找 |

## 5. verification_tokens — 邮箱验证令牌

每个用户最多一个验证令牌。

| 列 | 类型 | 约束 | 默认值 | 说明 |
|---|------|------|--------|------|
| `user_id` | UUID | PK, FK → users(id) ON DELETE CASCADE | — | 用户 ID（一对一） |
| `token` | VARCHAR(255) | NOT NULL | — | 验证令牌 |
| `expires_at` | TIMESTAMP | NOT NULL | — | 过期时间 |
| `created_at` | TIMESTAMP | NOT NULL | `CURRENT_TIMESTAMP` | 创建时间 |

### 索引

| 索引名 | 列 | 类型 | 用途 |
|--------|-----|------|------|
| `idx_verification_tokens_expires_at` | `expires_at` | BTREE | 过期清理查询 |
| `idx_verification_tokens_user_id` | `user_id` | BTREE | 按用户查找 |
| `idx_verification_token` | `token` | UNIQUE | 按令牌查找 |

## 6. reset_tokens — 密码重置令牌

每个用户最多一个重置令牌。

| 列 | 类型 | 约束 | 默认值 | 说明 |
|---|------|------|--------|------|
| `user_id` | UUID | PK, FK → users(id) ON DELETE CASCADE | — | 用户 ID（一对一） |
| `token` | VARCHAR(255) | NOT NULL | — | 重置令牌 |
| `expires_at` | TIMESTAMP | NOT NULL | — | 过期时间 |
| `used_at` | TIMESTAMP | NULLABLE | NULL | 使用时间；NULL 表示未使用 |
| `created_at` | TIMESTAMP | NOT NULL | `CURRENT_TIMESTAMP` | 创建时间 |

### 索引

| 索引名 | 列 | 类型 | 用途 |
|--------|-----|------|------|
| `idx_reset_tokens_expires_at` | `expires_at` | BTREE | 过期清理查询 |
| `idx_reset_token` | `token` | UNIQUE | 按令牌查找 |
| `idx_reset_tokens_unused` | `user_id` WHERE `used_at IS NULL` | PARTIAL | 未使用令牌查询 |

### 安全设计

- 令牌只能使用一次，使用后立即标记 `used_at`
- 防止令牌在有效期内被重复使用
- 使用部分索引加速未使用令牌查询

## 7. audit_logs — 审计日志

只增不改的审计记录。

| 列 | 类型 | 约束 | 默认值 | 说明 |
|---|------|------|--------|------|
| `id` | VARCHAR(64) | PK | — | 日志 ID（时间戳 + 随机字符串） |
| `event_type` | VARCHAR(50) | NOT NULL | — | 事件类型（如 `user.login`、`token.issued`） |
| `user_id` | VARCHAR(36) | NULLABLE | NULL | 用户 ID |
| `client_id` | VARCHAR(255) | NULLABLE | NULL | 客户端 ID |
| `ip_address` | VARCHAR(45) | NULLABLE | NULL | 客户端 IP（IPv6 最大 45 字符） |
| `user_agent` | TEXT | NULLABLE | NULL | 用户代理字符串 |
| `details` | JSONB | NULLABLE | NULL | 事件详情（JSON 格式） |
| `success` | BOOLEAN | NOT NULL | TRUE | 操作是否成功 |
| `timestamp` | TIMESTAMP | NOT NULL | `NOW()` | 事件时间 |

### 索引

| 索引名 | 列 | 类型 | 用途 |
|--------|-----|------|------|
| `idx_audit_logs_event_type` | `event_type` | BTREE | 按事件类型过滤 |
| `idx_audit_logs_user_id` | `user_id` | BTREE | 按用户过滤 |
| `idx_audit_logs_timestamp` | `timestamp` | BTREE | 按时间范围查询 |
| `idx_audit_logs_user_timestamp` | `user_id, timestamp DESC` | COMPOSITE | 用户时间线查询 |
| `idx_audit_logs_user_event_timestamp` | `user_id, event_type, timestamp DESC` | COMPOSITE | 用户事件时间线查询 |

### 常见事件类型

| 事件类型 | 说明 |
|----------|------|
| `user.login` | 用户登录 |
| `user.logout` | 用户登出 |
| `user.register` | 用户注册 |
| `user.login_failed` | 登录失败 |
| `user.account_locked` | 账户被锁定 |
| `token.refresh` | Token 刷新 |
| `token.revoked` | Token 被撤销 |
| `mfa.setup` | MFA 设置 |
| `mfa.disabled` | MFA 禁用 |
| `password.changed` | 密码修改 |
| `password.reset` | 密码重置 |

## 8. key_versions — JWT 密钥版本

管理 JWT 签名密钥的轮换。

| 列 | 类型 | 约束 | 默认值 | 说明 |
|---|------|------|--------|------|
| `id` | VARCHAR(64) | PK | — | 密钥 ID（kid） |
| `public_key` | TEXT | NOT NULL | — | PEM 格式公钥 |
| `private_key` | TEXT | NULLABLE | NULL | PEM 格式私钥（仅主节点存储） |
| `status` | VARCHAR(20) | NOT NULL | `'active'` | `active` / `deprecated` / `revoked` |
| `created_at` | TIMESTAMP | NOT NULL | `NOW()` | 创建时间 |
| `expires_at` | TIMESTAMP | NULLABLE | NULL | 过期时间（过渡期使用） |

### 索引

| 索引名 | 列 | 类型 | 用途 |
|--------|-----|------|------|
| `idx_key_versions_status` | `status` | BTREE | 按状态过滤 |
| `idx_key_versions_created_at` | `created_at` | BTREE | 按创建时间排序 |

### 密钥状态流转

```
active ──(轮换开始)──> deprecated ──(过渡期结束)──> revoked
   │                     │
   │ 用于签名和验证       │ 仅用于验证
   │                     │
   └─────────────────────┘
```

## 9. mfa_recovery_codes — MFA 恢复码

一次性使用的 MFA 备用码。

| 列 | 类型 | 约束 | 默认值 | 说明 |
|---|------|------|--------|------|
| `id` | UUID | PK | `gen_random_uuid()` | 内部 ID |
| `user_id` | UUID | NOT NULL, FK → users(id) ON DELETE CASCADE | — | 用户 ID |
| `code_hash` | VARCHAR(255) | NOT NULL | — | HMAC-SHA256 哈希（支持 O(1) 查找） |
| `used_at` | TIMESTAMP | NULLABLE | NULL | 使用时间；NULL 表示未使用 |
| `created_at` | TIMESTAMP | NOT NULL | `NOW()` | 创建时间 |

### 索引

| 索引名 | 列 | 类型 | 用途 |
|--------|-----|------|------|
| `idx_mfa_recovery_codes_user_id` | `user_id` | BTREE | 按用户查找 |
| `idx_mfa_recovery_codes_unused` | `id` WHERE `used_at IS NULL` | PARTIAL | 未使用恢复码查询 |
| `idx_mfa_recovery_codes_user_code` | `user_id, code_hash` | UNIQUE | 用户+哈希唯一约束 |

### 安全设计

- 恢复码使用 HMAC-SHA256 哈希存储（非 bcrypt）
- 支持 O(1) 查找，无需遍历所有恢复码
- 使用后立即标记为已使用（`used_at` 设为当前时间）
- 每次生成新的恢复码批次时，旧的批次自动失效

## 表关系图

```
users (1) ────< (N) tokens (N) >──── (1) oauth_clients
  │
  ├──< (N) authorization_codes >─── (1) oauth_clients
  │
  ├──< (1) verification_tokens
  │
  ├──< (1) reset_tokens
  │
  ├──< (N) mfa_recovery_codes
  │
  └──< (N) audit_logs (通过 user_id 字符串关联)
```

## 迁移管理

数据库迁移使用 `migrate` 工具管理，迁移文件位于 `migrations/` 目录。

```bash
make migrate-up         # 执行所有待执行的迁移
make migrate-down       # 回滚最后一次迁移
make migrate-create NAME=xxx  # 创建新的迁移文件
```

### 迁移文件命名

```
<timestamp>_<name>.up.sql
<timestamp>_<name>.down.sql
```

迁移按时间戳顺序执行，确保可重复和可回滚。
