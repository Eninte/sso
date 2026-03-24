# SSO项目架构重构计划

**制定时间**: 2026-03-23 17:00  
**制定人**: AI Agent  
**目标**: 修复分层架构问题，提升代码质量和可维护性

---

## 一、现状分析

### 1.1 发现的核心问题

#### 问题1: 分层架构违反
```
AdminHandler ──直接依赖──> store.Store ❌
RegisterHandler ──直接依赖──> store.ErrDuplicateEmail ❌
```

**正确架构应该是**:
```
Handler ──依赖──> Service ──依赖──> Store
```

#### 问题2: 接口定义不完整
| Service | 是否有接口 | 影响 |
|---------|------------|------|
| AuthService | ✅ 有 | 可测试 |
| OAuthService | ✅ 有 | 可测试 |
| EmailService | ✅ 有 | 可测试 |
| AuditService | ✅ 有 | 可测试 |
| MFAService | ❌ 无 | 难测试 |
| UserService | ❌ 无 | 难测试 |
| SocialLoginService | ❌ 无 | 难测试 |

#### 问题3: Handler依赖具体类型
```go
// register.go
type RegisterHandler struct {
    authSvc *service.AuthService  // 依赖具体类型 ❌
}

// 应该是
type RegisterHandler struct {
    authSvc service.AuthServiceInterface  // 依赖接口 ✅
}
```

---

## 二、重构方案

### 2.1 架构原则

```
┌─────────────────────────────────────────────────────────────┐
│                         Handler层                           │
│  - 只依赖Service接口                                        │
│  - 处理HTTP请求/响应                                        │
│  - 不直接访问Store                                          │
└─────────────────────────────────────────────────────────────┘
                              │
                              ▼
┌─────────────────────────────────────────────────────────────┐
│                         Service层                           │
│  - 业务逻辑处理                                             │
│  - 依赖Store接口                                            │
│  - 错误转换（Store错误 → Service错误）                      │
└─────────────────────────────────────────────────────────────┘
                              │
                              ▼
┌─────────────────────────────────────────────────────────────┐
│                          Store层                            │
│  - 数据访问                                                 │
│  - 数据库操作                                               │
│  - 返回Store级别错误                                        │
└─────────────────────────────────────────────────────────────┘
```

### 2.2 重构步骤

#### 阶段1: 补充Service接口（1小时）

**目标**: 为所有Service创建接口

**任务清单**:
1. 创建 `MFAServiceInterface`
2. 创建 `UserServiceInterface`
3. 创建 `SocialLoginServiceInterface`
4. 创建 `AdminServiceInterface`（新增）

#### 阶段2: 创建AdminService（1小时）

**目标**: 将AdminHandler的业务逻辑移至Service层

**任务清单**:
1. 创建 `AdminService` 结构体
2. 实现用户管理方法
3. 实现系统管理方法

#### 阶段3: 重构AdminHandler（30分钟）

**目标**: AdminHandler改为依赖AdminService接口

**任务清单**:
1. 修改 `AdminHandler` 结构体
2. 更新所有处理方法
3. 移除对store的直接依赖

#### 阶段4: 修复错误处理耦合（30分钟）

**目标**: Handler不再直接使用Store错误

**任务清单**:
1. 在Service层添加错误转换
2. 更新RegisterHandler
3. 移除对store包的直接导入

#### 防段5: 更新Handler依赖（30分钟）

**目标**: 所有Handler改为依赖接口

**任务清单**:
1. 更新所有Handler结构体
2. 更新构造函数
3. 更新main.go的依赖注入

---

## 三、详细实现方案

### 3.1 阶段1: 补充Service接口

#### 3.1.1 MFAServiceInterface

```go
// internal/service/interfaces.go

// MFAServiceInterface 多因素认证服务接口
type MFAServiceInterface interface {
    // SetupMFA 设置MFA
    SetupMFA(ctx context.Context, userID string) (*model.MFASetupResponse, error)
    
    // VerifyAndEnableMFA 验证并启用MFA
    VerifyAndEnableMFA(ctx context.Context, userID, code string) error
    
    // DisableMFA 禁用MFA
    DisableMFA(ctx context.Context, userID, code string) error
    
    // GetMFAStatus 获取MFA状态
    GetMFAStatus(ctx context.Context, userID string) (*model.MFAStatusResponse, error)
}
```

#### 3.1.2 UserServiceInterface

```go
// UserServiceInterface 用户服务接口
type UserServiceInterface interface {
    // SendVerificationEmail 发送验证邮件
    SendVerificationEmail(ctx context.Context, userID string) error
    
    // VerifyEmail 验证邮箱
    VerifyEmail(ctx context.Context, userID, token string) error
    
    // ForgotPassword 忘记密码
    ForgotPassword(ctx context.Context, email string) error
    
    // ResetPassword 重置密码
    ResetPassword(ctx context.Context, userID, token, newPassword string) error
    
    // ChangePassword 修改密码
    ChangePassword(ctx context.Context, userID, oldPassword, newPassword string) error
}
```

#### 3.1.3 SocialLoginServiceInterface

```go
// SocialLoginServiceInterface 社交登录服务接口
type SocialLoginServiceInterface interface {
    // GetProviders 获取支持的提供商列表
    GetProviders() []string
    
    // GetAuthorizationURL 获取授权URL
    GetAuthorizationURL(provider, redirectURI, state string) (string, error)
    
    // HandleCallback 处理回调
    HandleCallback(ctx context.Context, provider, code, redirectURI string) (*model.LoginResponse, error)
}
```

### 3.2 阶段2: 创建AdminService

```go
// internal/service/admin.go

package service

import (
    "context"
    "log/slog"
    "time"
    
    "github.com/your-org/sso/internal/model"
    "github.com/your-org/sso/internal/store"
)

// AdminServiceInterface 管理员服务接口
type AdminServiceInterface interface {
    // 用户管理
    ListUsers(ctx context.Context, offset, limit int) ([]*model.User, int, error)
    GetUser(ctx context.Context, userID string) (*model.User, error)
    DisableUser(ctx context.Context, userID string) error
    EnableUser(ctx context.Context, userID string) error
    
    // 系统管理
    SystemHealth(ctx context.Context) (*SystemHealthInfo, error)
    CleanupExpired(ctx context.Context) error
}

// SystemHealthInfo 系统健康信息
type SystemHealthInfo struct {
    Status    string    `json:"status"`
    Timestamp time.Time `json:"timestamp"`
    Database  string    `json:"database"`
    Version   string    `json:"version"`
}

// AdminService 管理员服务实现
type AdminService struct {
    store store.Store
}

// NewAdminService 创建管理员服务
func NewAdminService(store store.Store) *AdminService {
    return &AdminService{store: store}
}

// ListUsers 列出用户
func (s *AdminService) ListUsers(ctx context.Context, offset, limit int) ([]*model.User, int, error) {
    return s.store.ListUsers(ctx, offset, limit)
}

// GetUser 获取用户
func (s *AdminService) GetUser(ctx context.Context, userID string) (*model.User, error) {
    return s.store.GetByID(ctx, userID)
}

// DisableUser 禁用用户
func (s *AdminService) DisableUser(ctx context.Context, userID string) error {
    user, err := s.store.GetByID(ctx, userID)
    if err != nil {
        return err
    }
    
    user.Status = "disabled"
    user.UpdatedAt = time.Now()
    
    if err := s.store.Update(ctx, user); err != nil {
        return err
    }
    
    // 撤销所有Token（失败不影响主流程）
    if err := s.store.RevokeAllUserTokens(ctx, userID); err != nil {
        slog.Warn("撤销用户Token失败", "error", err, "user_id", userID)
    }
    
    return nil
}

// EnableUser 启用用户
func (s *AdminService) EnableUser(ctx context.Context, userID string) error {
    user, err := s.store.GetByID(ctx, userID)
    if err != nil {
        return err
    }
    
    user.Status = "active"
    user.LoginAttempts = 0
    user.LockedUntil = nil
    user.UpdatedAt = time.Now()
    
    return s.store.Update(ctx, user)
}

// SystemHealth 系统健康检查
func (s *AdminService) SystemHealth(ctx context.Context) (*SystemHealthInfo, error) {
    dbStatus := "ok"
    if err := s.store.Ping(ctx); err != nil {
        dbStatus = "error"
    }
    
    return &SystemHealthInfo{
        Status:    "ok",
        Timestamp: time.Now(),
        Database:  dbStatus,
        Version:   "1.0.0",
    }, nil
}

// CleanupExpired 清理过期数据
func (s *AdminService) CleanupExpired(ctx context.Context) error {
    return s.store.CleanupExpired(ctx)
}
```

### 3.3 阶段3: 重构AdminHandler

```go
// internal/handler/admin.go

package handler

import (
    "encoding/json"
    "net/http"
    "strconv"
    
    "github.com/gorilla/mux"
    
    "github.com/your-org/sso/internal/service"
)

// AdminHandler 管理员处理器
type AdminHandler struct {
    adminSvc service.AdminServiceInterface  // 改为依赖接口
}

// NewAdminHandler 创建管理员处理器
func NewAdminHandler(adminSvc service.AdminServiceInterface) *AdminHandler {
    return &AdminHandler{adminSvc: adminSvc}
}

// HandleListUsers 处理用户列表请求
func (h *AdminHandler) HandleListUsers(w http.ResponseWriter, r *http.Request) {
    page := 1
    pageSize := DefaultPageSize
    
    // 解析分页参数...
    offset := (page - 1) * pageSize
    
    // 通过Service获取数据
    users, total, err := h.adminSvc.ListUsers(r.Context(), offset, pageSize)
    if err != nil {
        writeError(w, http.StatusInternalServerError, "获取用户列表失败")
        return
    }
    
    // 构建响应...
}
```

### 3.4 阶段4: 修复错误处理耦合

#### 3.4.1 在AuthService中添加错误转换

```go
// internal/service/auth.go

// Register 用户注册
func (s *AuthService) Register(ctx context.Context, req *model.RegisterRequest) (*model.User, error) {
    // ... 现有代码 ...
    
    user, err := s.store.CreateUser(ctx, user)
    if err != nil {
        // 转换Store错误为Service错误
        if errors.Is(err, store.ErrDuplicateEmail) {
            return nil, ErrEmailAlreadyExists  // 使用Service级别的错误
        }
        return nil, err
    }
    
    return user, nil
}
```

#### 3.4.2 在errors包中添加Service级别错误

```go
// internal/errors/errors.go

var (
    // ... 现有错误 ...
    
    // 用户相关错误
    ErrEmailAlreadyExists = New(ErrCodeEmailExists, "邮箱已被注册", 409)
)
```

#### 3.4.3 更新RegisterHandler

```go
// internal/handler/register.go

package handler

import (
    "errors"
    "net/http"
    
    "github.com/your-org/sso/internal/model"
    "github.com/your-org/sso/internal/service"
    apperrors "github.com/your-org/sso/internal/errors"
    "github.com/your-org/sso/internal/validator"
)

// 移除对store包的导入

// RegisterHandler 注册处理器
type RegisterHandler struct {
    authSvc service.AuthServiceInterface  // 改为依赖接口
}

// Handle 处理注册请求
func (h *RegisterHandler) Handle(w http.ResponseWriter, r *http.Request) {
    var req model.RegisterRequest
    if err := decodeJSON(r, &req); err != nil {
        writeError(w, http.StatusBadRequest, "无效的请求格式")
        return
    }
    
    user, err := h.authSvc.Register(r.Context(), &req)
    if err != nil {
        // 使用Service级别的错误，而不是Store级别的错误
        if errors.Is(err, service.ErrEmailAlreadyExists) {
            writeError(w, http.StatusConflict, "该邮箱已注册")
            return
        }
        // ... 其他错误处理 ...
    }
    
    // 返回成功响应
}
```

---

## 四、执行计划

### 4.1 时间安排

| 阶段 | 任务 | 预计时间 | 依赖 |
|------|------|----------|------|
| 1 | 补充Service接口 | 1小时 | 无 |
| 2 | 创建AdminService | 1小时 | 阶段1 |
| 3 | 重构AdminHandler | 30分钟 | 阶段2 |
| 4 | 修复错误处理 | 30分钟 | 阶段1 |
| 5 | 更新Handler依赖 | 30分钟 | 阶段1-4 |
| **总计** | | **3.5小时** | |

### 4.2 验证标准

每个阶段完成后需要验证：

1. **单元测试通过**: `make test-unit`
2. **构建成功**: `make build`
3. **无架构违反**: handler层不直接依赖store包

### 4.3 风险控制

1. **保持向后兼容**: 不改变外部API
2. **增量重构**: 每个阶段独立可验证
3. **测试覆盖**: 重构后运行完整测试

---

## 五、重构后架构

```
┌─────────────────────────────────────────────────────────────┐
│                         Handler层                           │
│  AdminHandler ──→ AdminServiceInterface                     │
│  RegisterHandler ──→ AuthServiceInterface                   │
│  LoginHandler ──→ AuthServiceInterface                      │
│  MFAHandler ──→ MFAServiceInterface                         │
│  UserHandler ──→ UserServiceInterface                       │
│  SocialHandler ──→ SocialLoginServiceInterface              │
└─────────────────────────────────────────────────────────────┘
                              │
                              ▼
┌─────────────────────────────────────────────────────────────┐
│                         Service层                           │
│  AdminService ──→ store.Store                               │
│  AuthService ──→ store.Store                                │
│  MFAService ──→ store.Store                                 │
│  UserService ──→ store.Store                                │
│  SocialLoginService ──→ store.Store                         │
│                                                              │
│  错误处理：Store错误 → Service错误（统一转换）              │
└─────────────────────────────────────────────────────────────┘
                              │
                              ▼
┌─────────────────────────────────────────────────────────────┐
│                          Store层                            │
│  PostgreSQL实现                                             │
│  返回store.ErrNotFound, store.ErrDuplicateEmail等           │
└─────────────────────────────────────────────────────────────┘
```

---

## 六、预期收益

### 6.1 架构层面
- ✅ 严格遵循分层架构
- ✅ 所有依赖通过接口
- ✅ 无跨层依赖

### 6.2 可测试性
- ✅ 所有Service有接口定义
- ✅ Handler可通过Mock测试
- ✅ 单元测试覆盖率提升

### 6.3 可维护性
- ✅ 职责分离清晰
- ✅ 错误处理统一
- ✅ 易于扩展新功能

---

**计划制定完成时间**: 2026-03-23 17:15
