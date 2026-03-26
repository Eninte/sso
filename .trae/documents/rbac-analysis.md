# RBAC 权限系统需求分析

## 问题背景

根据代码分析报告 `docs/reports/code-analysis/07-改进建议清单.md`，SEC-03 问题指出：

| ID | 问题 | 位置 | 影响 | 建议 | 状态 |
|----|------|------|------|------|------|
| SEC-03 | 管理员权限检查薄弱 | middleware/auth.go | 基于邮箱的检查可能被绕过 | 实现RBAC系统 | 待修复 |

**当前管理员检查实现** (`middleware/auth.go:108-131`)：
```go
func isAdminUser(email string, adminEmails []string, adminDomains []string) bool {
    // 检查是否在管理员邮箱白名单中
    // 检查邮箱域名是否在管理员域名白名单中
    // 问题：配置文件静态定义，无法动态管理
}
```

---

## 当前权限模型分析

### 现有实现

1. **用户模型** (`model/model.go`)：无角色字段
2. **管理员检查**：基于邮箱白名单 + 域名白名单
3. **OAuth Scopes**：仅用于 API 访问范围，非用户权限
4. **中间件**：`AdminMiddleware` 检查邮箱是否在白名单

### 现有问题

| 问题 | 影响 |
|------|------|
| 静态配置 | 管理员列表在配置文件中，无法动态管理 |
| 邮箱依赖 | 用户邮箱变更后权限可能失效或被绕过 |
| 无细粒度权限 | 只有"管理员/普通用户"两种角色 |
| 无审计追踪 | 无法记录权限变更历史 |
| 无权限继承 | 无法实现权限组合 |

---

## RBAC 需求评估

### 不需要 RBAC 的理由

| 理由 | 说明 |
|------|------|
| 功能简单 | 当前管理功能主要是用户管理 |
| 用户规模小 | SSO 服务的管理员数量通常有限 |
| 开发成本 | RBAC 增加开发和维护复杂度 |
| 当前方案可用 | 邮箱白名单方案在小规模场景下足够 |

### 需要 RBAC 的理由

| 理由 | 说明 |
|------|------|
| 安全性 | 邮箱白名单容易被绕过（如邮箱变更） |
| 灵活性 | 无法动态添加/删除管理员 |
| 细粒度控制 | 无法区分不同类型的管理员权限 |
| 审计需求 | 无法记录权限变更历史 |
| 合规要求 | 某些安全标准要求基于角色的访问控制 |
| 扩展性 | 未来功能增加时难以扩展 |

---

## 方案对比

### 方案一：最小改进（不推荐）

仅将管理员标识存储到数据库，保持二分法。

**优点**：开发量小（约 4 小时）
**缺点**：无法满足细粒度权限需求

### 方案二：简化版 RBAC（推荐）

预定义角色，用户-角色关联，基于角色的权限检查。

**数据模型**：
```go
type Role struct {
    ID          string    `json:"id" db:"id"`
    Name        string    `json:"name" db:"name"`           // admin, user_manager, auditor
    Description string    `json:"description" db:"description"`
    Permissions []string  `json:"permissions" db:"permissions"` // user:read, user:write, system:admin
    CreatedAt   time.Time `json:"created_at" db:"created_at"`
}

type UserRole struct {
    UserID     string    `json:"user_id" db:"user_id"`
    RoleID     string    `json:"role_id" db:"role_id"`
    AssignedAt time.Time `json:"assigned_at" db:"assigned_at"`
    AssignedBy string    `json:"assigned_by" db:"assigned_by"` // 分配者ID
}
```

**预定义角色**：
| 角色 | 权限 |
|------|------|
| admin | 所有权限 |
| user_manager | user:read, user:write |
| auditor | audit:read |
| key_manager | key:read, key:write |

**优点**：
- 满足细粒度权限需求
- 支持动态管理
- 开发成本适中
- 易于扩展

**工作量**：约 18 小时（2-3 个工作日）

### 方案三：完整 RBAC（过度设计）

动态角色和权限定义，权限继承，资源级别控制。

**优点**：最灵活
**缺点**：开发成本高，维护复杂

---

## 推荐方案

**结论：建议实现简化版 RBAC（方案二）**

理由：
1. SEC-03 已标记为高优先级安全问题
2. 当前方案存在明显的安全和管理缺陷
3. 简化版 RBAC 开发成本可控
4. 为未来扩展预留空间

---

## 实施计划

### 阶段一：数据模型和存储

1. 创建角色模型 (`internal/model/role.go`)
2. 扩展 Store 接口 (`internal/store/store.go`)
3. 创建数据库迁移
4. 实现 PostgreSQL 存储
5. 实现 Mock 存储

### 阶段二：权限服务

1. 创建 RBAC 服务 (`internal/service/rbac.go`)
2. 实现权限检查逻辑
3. 实现角色分配/撤销
4. 集成审计日志

### 阶段三：中间件和 Handler

1. 重构权限中间件 (`internal/middleware/auth.go`)
2. 添加权限检查装饰器
3. 更新 AdminHandler 使用新权限系统

### 阶段四：管理 API

1. 角色管理 API
2. 用户角色分配 API
3. 权限查询 API

### 阶段五：测试和文档

1. 编写单元测试
2. 编写集成测试
3. 更新 CHANGELOG
4. 更新 .env.example

---

## 详细实施步骤

### 1. 创建角色模型

**文件**: `internal/model/role.go`

```go
type Role struct {
    ID          string    `json:"id" db:"id"`
    Name        string    `json:"name" db:"name"`
    Description string    `json:"description" db:"description"`
    Permissions []string  `json:"permissions" db:"permissions"`
    IsSystem    bool      `json:"is_system" db:"is_system"` // 系统内置角色不可删除
    CreatedAt   time.Time `json:"created_at" db:"created_at"`
    UpdatedAt   time.Time `json:"updated_at" db:"updated_at"`
}

type UserRole struct {
    UserID     string    `json:"user_id" db:"user_id"`
    RoleID     string    `json:"role_id" db:"role_id"`
    AssignedAt time.Time `json:"assigned_at" db:"assigned_at"`
    AssignedBy string    `json:"assigned_by" db:"assigned_by"`
}

type Permission string

const (
    PermissionUserRead   Permission = "user:read"
    PermissionUserWrite  Permission = "user:write"
    PermissionUserDelete Permission = "user:delete"
    PermissionAuditRead  Permission = "audit:read"
    PermissionKeyRead    Permission = "key:read"
    PermissionKeyWrite   Permission = "key:write"
    PermissionSystemAdmin Permission = "system:admin"
)
```

### 2. 扩展 Store 接口

**文件**: `internal/store/store.go`

```go
type RoleStore interface {
    CreateRole(ctx context.Context, role *model.Role) error
    GetRoleByID(ctx context.Context, id string) (*model.Role, error)
    GetRoleByName(ctx context.Context, name string) (*model.Role, error)
    ListRoles(ctx context.Context) ([]*model.Role, error)
    UpdateRole(ctx context.Context, role *model.Role) error
    DeleteRole(ctx context.Context, id string) error
    
    AssignRole(ctx context.Context, userID, roleID, assignedBy string) error
    RevokeRole(ctx context.Context, userID, roleID string) error
    GetUserRoles(ctx context.Context, userID string) ([]*model.Role, error)
    GetUserPermissions(ctx context.Context, userID string) ([]string, error)
    HasPermission(ctx context.Context, userID, permission string) (bool, error)
}
```

### 3. 数据库迁移

**文件**: `migrations/010_create_roles.up.sql`

```sql
CREATE TABLE roles (
    id VARCHAR(36) PRIMARY KEY,
    name VARCHAR(50) UNIQUE NOT NULL,
    description TEXT,
    permissions TEXT[] NOT NULL DEFAULT '{}',
    is_system BOOLEAN NOT NULL DEFAULT false,
    created_at TIMESTAMP NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP NOT NULL DEFAULT NOW()
);

CREATE TABLE user_roles (
    user_id VARCHAR(36) NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    role_id VARCHAR(36) NOT NULL REFERENCES roles(id) ON DELETE CASCADE,
    assigned_at TIMESTAMP NOT NULL DEFAULT NOW(),
    assigned_by VARCHAR(36) REFERENCES users(id),
    PRIMARY KEY (user_id, role_id)
);

CREATE INDEX idx_user_roles_user_id ON user_roles(user_id);
CREATE INDEX idx_user_roles_role_id ON user_roles(role_id);

-- 插入系统内置角色
INSERT INTO roles (id, name, description, permissions, is_system) VALUES
('role-admin', 'admin', '系统管理员', ARRAY['user:read','user:write','user:delete','audit:read','key:read','key:write','system:admin'], true),
('role-user-manager', 'user_manager', '用户管理员', ARRAY['user:read','user:write'], true),
('role-auditor', 'auditor', '审计员', ARRAY['audit:read'], true),
('role-key-manager', 'key_manager', '密钥管理员', ARRAY['key:read','key:write'], true);
```

### 4. RBAC 服务

**文件**: `internal/service/rbac.go`

```go
type RBACService struct {
    store    store.Store
    auditSvc *AuditService
    cache    cache.Cache
}

func (s *RBACService) HasPermission(ctx context.Context, userID, permission string) (bool, error)
func (s *RBACService) AssignRole(ctx context.Context, userID, roleID, assignedBy string) error
func (s *RBACService) RevokeRole(ctx context.Context, userID, roleID string) error
func (s *RBACService) GetUserPermissions(ctx context.Context, userID string) ([]string, error)
```

### 5. 权限中间件重构

**文件**: `internal/middleware/auth.go`

```go
func PermissionMiddleware(rbacSvc *service.RBACService, permission string) func(http.Handler) http.Handler {
    return func(next http.Handler) http.Handler {
        return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
            userID := GetUserIDFromContext(r.Context())
            hasPerm, err := rbacSvc.HasPermission(r.Context(), userID, permission)
            if err != nil || !hasPerm {
                writeAdminError(w, http.StatusForbidden, "权限不足")
                return
            }
            next.ServeHTTP(w, r)
        })
    }
}
```

---

## 配置变更

**文件**: `.env.example`

```bash
# RBAC 配置
RBAC_CACHE_ENABLED=true        # 是否启用权限缓存
RBAC_CACHE_TTL=5m              # 权限缓存时间
```

---

## 工作量估算

| 阶段 | 工作内容 | 预估时间 |
|------|----------|----------|
| 阶段一 | 数据模型和存储 | 4 小时 |
| 阶段二 | 权限服务 | 4 小时 |
| 阶段三 | 中间件和 Handler | 4 小时 |
| 阶段四 | 管理 API | 3 小时 |
| 阶段五 | 测试和文档 | 3 小时 |
| **总计** | | **18 小时** |

---

## 风险评估

| 风险 | 影响 | 缓解措施 |
|------|------|----------|
| 现有管理员迁移 | 中 | 提供迁移脚本，将白名单管理员分配 admin 角色 |
| 性能影响 | 低 | 使用缓存，权限检查结果缓存 5 分钟 |
| 向后兼容 | 低 | 保留 AdminMiddleware 作为兼容层 |

---

## 决策建议

### 如果选择实施 RBAC

- 解决 SEC-03 安全问题
- 提升系统安全性和可管理性
- 为未来扩展预留空间
- 工作量约 18 小时（2-3 个工作日）

### 如果选择不实施 RBAC

- 建议至少实现"管理员数据库存储"改进
- 将管理员标识从配置文件移到数据库
- 添加管理员管理 API
- 工作量约 4 小时
