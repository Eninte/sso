# 安全问题修复计划

## 问题概述

| ID | 问题 | 位置 | 影响 | 建议 |
|----|------|------|------|------|
| SEC-09 | 缺少密钥轮换机制 | crypto/jwt.go | 密钥泄露风险 | 实现密钥轮换 |
| SEC-10 | 审计日志不完整 | service/*.go | 难以追踪安全事件 | 完善审计日志 |

---

## SEC-09: 密钥轮换机制

### 问题分析

当前 `JWTService` 只支持单一密钥对：
- 无法在不中断服务的情况下轮换密钥
- 密钥泄露后无法快速切换到新密钥
- 缺少密钥版本标识，无法追踪Token使用的密钥

### 实施方案

#### 1. 新增密钥管理模型 (`internal/model/key.go`)

```go
type KeyVersion struct {
    ID         string    // 密钥版本ID (kid)
    PublicKey  []byte    // 公钥PEM
    PrivateKey []byte    // 私钥PEM (仅主密钥存储)
    CreatedAt  time.Time
    ExpiresAt  *time.Time // 过期时间（用于轮换过渡期）
    IsActive   bool      // 是否为当前活跃密钥
    Status     KeyStatus // active, deprecated, revoked
}

type KeyStatus string

const (
    KeyStatusActive     KeyStatus = "active"     // 当前使用
    KeyStatusDeprecated KeyStatus = "deprecated" // 过渡期，仅验证
    KeyStatusRevoked    KeyStatus = "revoked"    // 已撤销
)
```

#### 2. 扩展 Store 接口 (`internal/store/store.go`)

```go
type KeyStore interface {
    StoreKey(ctx context.Context, key *model.KeyVersion) error
    GetActiveKey(ctx context.Context) (*model.KeyVersion, error)
    GetKeyByID(ctx context.Context, keyID string) (*model.KeyVersion, error)
    ListActiveKeys(ctx context.Context) ([]*model.KeyVersion, error)
    DeprecateKey(ctx context.Context, keyID string, expiresAt time.Time) error
    RevokeKey(ctx context.Context, keyID string) error
}
```

#### 3. 重构 JWTService (`internal/crypto/jwt.go`)

- 添加 `kid` (Key ID) 到 JWT Header
- 支持多密钥验证（根据 kid 选择对应公钥）
- 使用当前活跃密钥签名新Token
- 支持过渡期内验证旧密钥签发的Token

```go
type JWTService struct {
    keys           map[string]*rsa.PrivateKey // kid -> private key
    publicKeys     map[string]*rsa.PublicKey  // kid -> public key
    activeKeyID    string                     // 当前活跃密钥ID
    keyStore       store.KeyStore             // 密钥存储
    // ... 其他字段
}

// Token Claims 增加 KeyID
type AccessTokenClaims struct {
    jwt.RegisteredClaims
    KeyID  string   `json:"kid,omitempty"`
    Email  string   `json:"email"`
    Scopes []string `json:"scope"`
}
```

#### 4. 新增密钥轮换服务 (`internal/service/keyrotation.go`)

```go
type KeyRotationService struct {
    keyStore    store.KeyStore
    jwtSvc      *crypto.JWTService
    rotationTTL time.Duration // 过渡期时长
}

// RotateKey 执行密钥轮换
// 1. 生成新密钥对
// 2. 将旧密钥标记为 deprecated
// 3. 设置过渡期
// 4. 过渡期结束后撤销旧密钥
func (s *KeyRotationService) RotateKey(ctx context.Context) error

// CleanupExpiredKeys 清理过期密钥
func (s *KeyRotationService) CleanupExpiredKeys(ctx context.Context) error
```

#### 5. 数据库迁移

```sql
CREATE TABLE key_versions (
    id VARCHAR(64) PRIMARY KEY,
    public_key TEXT NOT NULL,
    private_key TEXT,  -- 仅主密钥存储
    status VARCHAR(20) NOT NULL DEFAULT 'active',
    created_at TIMESTAMP NOT NULL,
    expires_at TIMESTAMP,
    INDEX idx_status (status),
    INDEX idx_created (created_at)
);
```

#### 6. 配置扩展 (`internal/config/config.go`)

```go
type Config struct {
    // ... 现有配置
    KeyRotationEnabled  bool          // 是否启用密钥轮换
    KeyRotationInterval time.Duration // 轮换周期
    KeyTransitionPeriod time.Duration // 过渡期时长
}
```

---

## SEC-10: 审计日志完善

### 问题分析

当前审计日志覆盖不完整：
- `auth.go`: Login/Logout/RefreshToken 未记录审计日志
- `user.go`: 密码修改未记录审计日志
- `mfa.go`: MFA操作未记录审计日志
- 缺少关键安全事件的审计记录

### 实施方案

#### 1. 扩展审计事件类型 (`internal/model/audit.go`)

```go
const (
    // 现有事件...
    
    // 新增安全相关事件
    EventPasswordChanged   AuditEventType = "security.password_changed"   // 密码修改
    EventPasswordReset     AuditEventType = "security.password_reset"     // 密码重置
    EventMFAEnabled        AuditEventType = "mfa.enabled"                 // MFA启用
    EventMFADisabled       AuditEventType = "mfa.disabled"                // MFA禁用
    EventMFASetup          AuditEventType = "mfa.setup"                   // MFA设置
    EventKeyRotated        AuditEventType = "key.rotated"                 // 密钥轮换
    EventKeyRevoked        AuditEventType = "key.revoked"                 // 密钥撤销
    EventTokenRefresh      AuditEventType = "token.refresh"               // Token刷新
    EventTokenRevoke       AuditEventType = "token.revoke"                // Token撤销
    EventLogoutAll         AuditEventType = "user.logout_all"             // 登出所有设备
    EventAccountLocked     AuditEventType = "user.account_locked"         // 账户锁定
    EventAccountUnlocked   AuditEventType = "user.account_unlocked"       // 账户解锁
)
```

#### 2. 修改 AuthService (`internal/service/auth.go`)

注入 `AuditService` 依赖，并在关键操作处添加审计日志：

```go
type AuthService struct {
    // ... 现有字段
    auditSvc *AuditService  // 新增审计服务
}

// Login 方法添加审计日志
func (s *AuthService) Login(ctx context.Context, req *model.LoginRequest) (*model.LoginResponse, error) {
    // ... 现有逻辑
    
    // 登录成功
    s.auditSvc.LogUserLogin(ctx, user.ID, user.Email, ipAddress, userAgent, true)
    
    // 登录失败
    s.auditSvc.LogUserLogin(ctx, user.ID, user.Email, ipAddress, userAgent, false)
    
    // 账户锁定
    s.auditSvc.LogAccountLocked(ctx, user.ID, ipAddress)
}

// Logout 方法添加审计日志
func (s *AuthService) Logout(ctx context.Context, accessToken string) error {
    // ... 现有逻辑
    s.auditSvc.LogUserLogout(ctx, userID, ipAddress)
}

// RefreshToken 方法添加审计日志
func (s *AuthService) RefreshToken(ctx context.Context, refreshToken string) (*model.LoginResponse, error) {
    // ... 现有逻辑
    s.auditSvc.LogTokenRefresh(ctx, userID, clientID, ipAddress)
}
```

#### 3. 修改 UserService (`internal/service/user.go`)

```go
type UserService struct {
    // ... 现有字段
    auditSvc *AuditService
}

// ChangePassword 添加审计日志
func (s *UserService) ChangePassword(ctx context.Context, userID, oldPassword, newPassword string) error {
    // ... 现有逻辑
    s.auditSvc.LogPasswordChanged(ctx, userID, ipAddress, true)
}

// ResetPassword 添加审计日志
func (s *UserService) ResetPassword(ctx context.Context, userID, token, newPassword string) error {
    // ... 现有逻辑
    s.auditSvc.LogPasswordReset(ctx, userID, ipAddress)
}
```

#### 4. 修改 MFAService (`internal/service/mfa.go`)

```go
type MFAService struct {
    store    store.Store
    auditSvc *AuditService  // 新增
}

// SetupMFA 添加审计日志
func (s *MFAService) SetupMFA(ctx context.Context, userID string) (*model.MFASetupResponse, error) {
    // ... 现有逻辑
    s.auditSvc.LogMFASetup(ctx, userID, ipAddress)
}

// VerifyAndEnableMFA 添加审计日志
func (s *MFAService) VerifyAndEnableMFA(ctx context.Context, userID, code string) error {
    // ... 现有逻辑
    s.auditSvc.LogMFAEnabled(ctx, userID, ipAddress)
}

// DisableMFA 添加审计日志
func (s *MFAService) DisableMFA(ctx context.Context, userID, code string) error {
    // ... 现有逻辑
    s.auditSvc.LogMFADisabled(ctx, userID, ipAddress)
}
```

#### 5. 扩展 AuditService 方法 (`internal/service/audit.go`)

```go
// 新增审计日志方法
func (s *AuditService) LogTokenRefresh(ctx context.Context, userID, clientID, ipAddress string)
func (s *AuditService) LogUserLogout(ctx context.Context, userID, ipAddress string)
func (s *AuditService) LogPasswordChanged(ctx context.Context, userID, ipAddress string, success bool)
func (s *AuditService) LogPasswordReset(ctx context.Context, userID, ipAddress string)
func (s *AuditService) LogAccountLocked(ctx context.Context, userID, ipAddress string)
func (s *AuditService) LogAccountUnlocked(ctx context.Context, userID, ipAddress string)
func (s *AuditService) LogMFASetup(ctx context.Context, userID, ipAddress string)
func (s *AuditService) LogMFAEnabled(ctx context.Context, userID, ipAddress string)
func (s *AuditService) LogMFADisabled(ctx context.Context, userID, ipAddress string)
func (s *AuditService) LogKeyRotated(ctx context.Context, keyID string)
func (s *AuditService) LogKeyRevoked(ctx context.Context, keyID string)
func (s *AuditService) LogLogoutAll(ctx context.Context, userID, ipAddress string)
```

#### 6. Handler 层传递上下文信息

需要在 HTTP Handler 中提取并传递 `IPAddress` 和 `UserAgent`：

```go
// 从请求中提取审计信息
func extractAuditInfo(r *http.Request) (ipAddress, userAgent string) {
    ipAddress = r.Header.Get("X-Forwarded-For")
    if ipAddress == "" {
        ipAddress = r.Header.Get("X-Real-IP")
    }
    if ipAddress == "" {
        ipAddress = r.RemoteAddr
    }
    userAgent = r.UserAgent()
    return
}
```

---

## 实施步骤

### 阶段一：SEC-10 审计日志完善（优先级高）

1. 扩展审计事件类型定义
2. 扩展 AuditService 方法
3. 修改 AuthService 添加审计日志
4. 修改 UserService 添加审计日志
5. 修改 MFAService 添加审计日志
6. 更新 Handler 层传递审计上下文
7. 更新单元测试

### 阶段二：SEC-09 密钥轮换机制

1. 创建密钥模型
2. 扩展 Store 接口和实现
3. 创建数据库迁移
4. 重构 JWTService 支持多密钥
5. 创建 KeyRotationService
6. 扩展配置
7. 添加密钥轮换审计日志
8. 更新单元测试

---

## 测试计划

### SEC-09 测试

1. 密钥生成和存储测试
2. 多密钥验证测试
3. 密钥轮换流程测试
4. 过渡期Token验证测试
5. 密钥撤销测试

### SEC-10 测试

1. 各事件审计日志记录测试
2. 审计日志完整性测试
3. 审计日志查询测试
4. 异步写入可靠性测试

---

## 风险评估

| 风险 | 影响 | 缓解措施 |
|------|------|----------|
| 密钥轮换导致Token失效 | 中 | 设置足够长的过渡期，通知客户端刷新Token |
| 审计日志性能影响 | 低 | 使用异步写入，channel缓冲 |
| 数据库迁移失败 | 高 | 提前备份，支持回滚 |

---

## 文档更新计划

修复完成后需要更新以下文档：

### 1. API 文档更新

**位置**: `docs/api.md` 或 `README.md`

- 新增密钥轮换 API 端点文档
  - `POST /admin/keys/rotate` - 执行密钥轮换
  - `GET /admin/keys` - 列出所有密钥版本
  - `DELETE /admin/keys/{keyID}` - 撤销指定密钥
- 审计日志查询 API 文档
  - `GET /audit/logs` - 查询审计日志
  - 新增的事件类型说明

### 2. 架构文档更新

**位置**: `docs/architecture.md` 或 `README.md`

- 密钥轮换机制说明
  - 密钥生命周期（active → deprecated → revoked）
  - 过渡期处理逻辑
  - JWKS 端点多密钥支持
- 审计日志架构说明
  - 异步写入机制
  - 事件类型分类
  - 日志保留策略

### 3. 运维文档更新

**位置**: `docs/operations.md` 或 `README.md`

- 密钥轮换操作指南
  - 定期轮换建议（如每90天）
  - 紧急轮换流程（密钥泄露场景）
  - 轮换前检查清单
- 审计日志运维指南
  - 日志存储容量规划
  - 日志备份策略
  - 安全事件排查流程

### 4. 配置说明更新

**位置**: `.env.example` 和 `README.md`

新增配置项说明：
```bash
# 密钥轮换配置
KEY_ROTATION_ENABLED=true
KEY_ROTATION_INTERVAL=2160h      # 90天
KEY_TRANSITION_PERIOD=24h        # 过渡期24小时

# 审计日志配置
AUDIT_LOG_RETENTION_DAYS=90      # 日志保留天数
AUDIT_LOG_CHANNEL_SIZE=1000      # 异步写入缓冲区大小
```

### 5. CHANGELOG 更新

**位置**: `CHANGELOG.md`

```markdown
## [Unreleased]

### Security
- SEC-09: 实现JWT密钥轮换机制，支持无缝密钥更新
- SEC-10: 完善审计日志，覆盖所有关键安全操作

### Added
- 新增 KeyRotationService 用于密钥轮换管理
- 新增多种审计事件类型（密码修改、MFA操作、Token刷新等）
- 新增密钥轮换管理API端点

### Changed
- JWTService 支持多密钥验证
- AuthService、UserService、MFAService 集成审计日志
```

### 6. 数据库迁移文档

**位置**: `migrations/README.md`

- 记录 key_versions 表的迁移说明
- 迁移回滚步骤

---

## 预估工作量

- SEC-10 审计日志完善：约 4-6 小时
- SEC-09 密钥轮换机制：约 8-10 小时
- 测试编写：约 4 小时
- 文档更新：约 2 小时
- 总计：约 18-22 小时
