# SSO 项目改进计划

**创建日期**: 2026年3月24日  
**最后更新**: 2026年3月24日  
**版本**: 1.0  

---

## 一、改进概述

### 1.1 改进范围

本计划针对代码分析报告中识别的待办项，包括：

| 优先级 | 问题 | 状态 |
|--------|------|------|
| 高 | 命名冲突 | ⏳ 待办 |
| 中 | 注释覆盖不足 | ⏳ 待办 |
| 中 | 密钥路径前缀过于宽泛 | ⏳ 待办 |
| 中 | N+1查询问题 | ⏳ 待办 |
| 高 | 集成测试覆盖率低 | ⏳ 待办 |
| 高 | 端到端测试缺失 | ⏳ 待办 |
| 高 | 配置测试缺失 | ⏳ 待办 |

### 1.2 改进目标

1. **代码质量**: 消除命名冲突，添加必要注释
2. **安全性**: 优化密钥路径验证，减少N+1查询
3. **测试**: 提高测试覆盖率，添加端到端测试
4. **可维护性**: 改善代码结构，便于后续维护

---

## 二、详细改进方案

### 2.1 修复命名冲突

#### 问题描述

| 原名称 | 问题 | 影响范围 |
|--------|------|----------|
| `MetricsService` | 与包名重复，产生 `mock.MetricsService` | ~5个文件 |
| `MockStore` | 与包名重复，产生 `mock.MockStore` | ~10个文件 |

#### 改进方案

**步骤1**: 重命名 `internal/metrics/metrics.go`

```go
// 原代码
type MetricsService struct { ... }

// 新代码
type Service struct { ... }
```

**步骤2**: 重命名 `internal/store/mock/mock.go`

```go
// 原代码
type MockStore struct { ... }

// 新代码
type Store struct { ... }
```

**步骤3**: 更新所有引用

需要更新的文件列表：

| 文件 | 修改内容 |
|------|----------|
| `internal/service/auth.go` | `*metrics.MetricsService` → `*metrics.Service` |
| `cmd/server/main.go` | `metrics.NewMetricsService()` → `metrics.New()` |
| `internal/handler/metrics.go` | `metricsSvc *metrics.MetricsService` → `metricsSvc *metrics.Service` |
| `internal/handler/handler_test.go` | `mock.New()` → `mock.NewMockStore()` |
| `internal/service/auth_test.go` | `mock.New()` → `mock.NewMockStore()` |
| `internal/service/user_test.go` | `mock.New()` → `mock.NewMockStore()` |
| ... | 其他测试文件 |

#### 验证命令

```bash
go build ./...
go test ./...
make lint
```

---

### 2.2 添加导出函数注释

#### 问题描述

`internal/cache/redis.go` 中的多个导出函数缺少注释。

#### 需要添加注释的函数

| 函数名 | 注释内容 |
|--------|----------|
| `TokenKey` | 生成Token缓存键 |
| `UserIDKey` | 生成用户ID缓存键 |
| `UserEmailKey` | 生成用户邮箱缓存键 |
| `ClientKey` | 生成客户端缓存键 |
| `NewMemoryCache` | 创建内存缓存实例 |
| `MemoryCache` | 内存缓存实现 |
| `MemoryCache.Get` | 获取缓存值 |
| `MemoryCache.Set` | 设置缓存值 |
| `MemoryCache.SetWithNilProtection` | 设置缓存值（支持空值防护） |
| `MemoryCache.Delete` | 删除缓存 |
| `MemoryCache.DeletePattern` | 按模式删除缓存 |
| `MemoryCache.Close` | 关闭缓存 |

#### 示例注释格式

```go
// TokenKey 生成Token缓存键
// 格式: "token:" + accessToken
func TokenKey(accessToken string) string {
    return TokenCachePrefix + accessToken
}
```

---

### 2.3 优化密钥路径前缀

#### 问题描述

当前允许 `/tmp/` 路径，在生产环境存在安全风险。

#### 当前代码 (`internal/crypto/keyloader.go`)

```go
// 第171-176行
if !strings.HasPrefix(absPath, "/etc/sso/") &&
    !strings.HasPrefix(absPath, "/keys/") &&
    !strings.HasPrefix(absPath, "/home/") &&
    !strings.HasPrefix(absPath, "/tmp/") {
    return ErrKeyPathInvalid
}
```

#### 改进方案

```go
// 在validateKeyPath函数中添加环境检查
func validateKeyPath(path string, env string) error {
    // ... 其他验证 ...
    
    // 生产环境不允许 /tmp/ 路径
    if env == "production" && strings.HasPrefix(absPath, "/tmp/") {
        return fmt.Errorf("%w: /tmp/ not allowed in production", ErrKeyPathInvalid)
    }
    
    // ... 其他验证 ...
}
```

#### 调用方式修改

```go
// LoadPrivateKeyFromFile
if err := validateKeyPath(path, env); err != nil { ... }

// 需要传递环境变量
func validateKeyPath(path, env string) error { ... }
```

---

### 2.4 优化N+1查询

#### 问题描述

`ValidateRedirectURI` 每次调用都加载整个客户端对象。

#### 当前代码 (`internal/store/postgres/postgres.go`)

```go
func (s *Store) ValidateRedirectURI(ctx context.Context, clientID string, redirectURI string) bool {
    client, err := s.GetByClientID(ctx, clientID)  // 加载整个对象
    if err != nil {
        return false
    }
    for _, uri := range client.RedirectURIs {
        if uri == redirectURI {
            return true
        }
    }
    return false
}
```

#### 优化后代码

```go
func (s *Store) ValidateRedirectURI(ctx context.Context, clientID string, redirectURI string) bool {
    query := `
        SELECT EXISTS(
            SELECT 1 FROM oauth_clients 
            WHERE client_id = $1 
            AND $2 = ANY(redirect_uris)
        )
    `
    var exists bool
    err := s.db.QueryRowContext(ctx, query, clientID, redirectURI).Scan(&exists)
    if err != nil {
        slog.Warn("验证重定向URI失败", "error", err)
        return false
    }
    return exists
}
```

#### 优点

1. 减少数据库数据传输量
2. 只返回布尔值，不加载完整对象
3. 使用 PostgreSQL `ANY()` 数组函数

---

### 2.5 添加配置测试

#### 新建文件

`internal/config/config_test.go`

#### 测试用例

| 测试函数 | 测试场景 | 预期结果 |
|----------|----------|----------|
| `TestLoad_MissingDBPassword` | 未设置DB_PASSWORD环境变量 | 返回错误 |
| `TestLoad_WithDBPassword` | 设置DB_PASSWORD环境变量 | 成功加载 |
| `TestValidate_InvalidPort_TooLow` | 端口=0 | 返回警告 |
| `TestValidate_InvalidPort_TooHigh` | 端口=70000 | 返回警告 |
| `TestValidate_InvalidPort_NonNumeric` | 端口="abc" | 返回警告 |
| `TestValidate_BcryptCostTooLow` | production环境 cost=10 | 返回错误 |
| `TestValidate_BcryptCostTooHigh` | cost=35 | 返回警告 |
| `TestValidate_ProductionDefaults` | production + 默认CORS | 返回错误 |
| `TestValidate_AccessTokenTTLTooShort` | TTL=30s | 返回警告 |
| `TestValidate_RefreshTokenTTLSmaller` | Refresh < Access | 返回警告 |
| `TestDatabaseURL` | - | 生成正确URL |
| `TestRedisURL_WithPassword` | 有Redis密码 | 生成正确URL |
| `TestRedisURL_WithoutPassword` | 无Redis密码 | 生成正确URL |
| `TestGetAdminEmails_Multiple` | 多邮箱逗号分隔 | 正确解析 |
| `TestGetAdminEmails_WithSpaces` | 带空格的邮箱 | 正确解析并trim |
| `TestGetAdminEmails_Empty` | 空字符串 | 返回nil |

#### 示例测试代码

```go
func TestLoad_MissingDBPassword(t *testing.T) {
    // 清除环境变量
    oldValue := os.Getenv("DB_PASSWORD")
    defer os.Setenv("DB_PASSWORD", oldValue)
    os.Unsetenv("DB_PASSWORD")

    _, err := Load()
    require.Error(t, err)
    assert.Equal(t, ErrDBPasswordRequired, err)
}

func TestValidate_BcryptCostTooLow(t *testing.T) {
    cfg := &Config{
        DBPassword:     "test",
        BcryptCost:     10,
        Env:            "production",
        CORSAllowedOrigins: "https://example.com",
        AdminEmails:    "admin@example.com",
    }

    err := cfg.validate()
    require.Error(t, err)
    assert.Equal(t, ErrBcryptCostTooLow, err)
}
```

---

### 2.6 提高集成测试覆盖率

#### 新增测试用例 (`internal/store/postgres/postgres_test.go`)

| 测试函数 | 测试场景 |
|----------|----------|
| `TestStore_GetUserByField_ValidFields` | 白名单字段（id, email） |
| `TestStore_GetUserByField_InvalidField` | 非白名单字段（password） |
| `TestStore_GetTokenByField_ValidFields` | 白名单字段 |
| `TestStore_GetTokenByField_InvalidField` | 非白名单字段 |
| `TestStore_ListUsers_PaginationFirst` | 第一页 |
| `TestStore_ListUsers_PaginationMiddle` | 中间页 |
| `TestStore_ListUsers_PaginationBeyond` | 超过总页数 |
| `TestStore_ListAuditLogs_WithUserID` | 按用户ID过滤 |
| `TestStore_ListAuditLogs_WithEventType` | 按事件类型过滤 |
| `TestStore_ListAuditLogs_WithBoth` | 联合过滤 |
| `TestStore_CleanupExpired_Tokens` | 清理过期Token |
| `TestStore_CleanupExpired_AuthCodes` | 清理过期授权码 |
| `TestStore_CleanupExpired_Mixed` | 混合清理 |

#### 示例测试代码

```go
func TestStore_GetUserByField_InvalidField(t *testing.T) {
    skipIfNoDB(t)

    store := newTestStore(t)
    defer store.Close()

    _, err := store.getUserByField(context.Background(), "password", "test")
    require.Error(t, err)
    assert.Contains(t, err.Error(), "invalid field name")
}

func TestStore_ListUsers_PaginationBeyond(t *testing.T) {
    skipIfNoDB(t)

    store := newTestStore(t)
    defer store.Close()

    // 创建测试用户
    for i := 0; i < 5; i++ {
        createTestUser(t, store, fmt.Sprintf("user%d@test.com", i))
    }

    // 请求超过总页数
    users, total, err := store.ListUsers(context.Background(), 100, 10)
    require.NoError(t, err)
    assert.Equal(t, 5, total)
    assert.Len(t, users, 0)
}
```

---

### 2.7 添加端到端测试

#### 新建目录

`test/e2e/`

#### 文件结构

```
test/
└── e2e/
    ├── go.mod
    ├── auth_flow_test.go
    ├── oauth_flow_test.go
    └── admin_flow_test.go
```

#### 测试文件内容

##### 1. auth_flow_test.go

```go
package e2e

import (
    "context"
    "net/http"
    "net/http/httptest"
    "testing"

    "github.com/stretchr/testify/require"
)

func TestAuthFlow_RegisterLoginRefreshLogout(t *testing.T) {
    // 跳过测试如果没有启用e2e标签
    if !testing.Short() {
        t.Skip("跳过端到端测试，使用 -short 运行")
    }

    baseURL := getBaseURL(t)

    t.Run("注册", func(t *testing.T) {
        // 1. 注册用户
        req := httptest.NewRequest("POST", baseURL+"/api/v1/register", nil)
        // ... 设置请求体
        resp := httptest.NewRecorder()
        client.Do(req, resp)

        require.Equal(t, http.StatusCreated, resp.Code)
    })

    t.Run("登录", func(t *testing.T) {
        // 2. 登录获取token
        // ...
    })

    t.Run("刷新Token", func(t *testing.T) {
        // 3. 使用refresh_token刷新
        // ...
    })

    t.Run("登出", func(t *testing.T) {
        // 4. 登出撤销token
        // ...
    })
}
```

##### 2. oauth_flow_test.go

```go
func TestOAuthFlow_AuthorizationCode(t *testing.T) {
    // 1. 获取授权码
    // 2. 使用授权码换取token
    // 3. 使用access_token访问资源
    // 4. 验证token
}
```

##### 3. admin_flow_test.go

```go
func TestAdminFlow_UserManagement(t *testing.T) {
    // 1. 管理员登录
    // 2. 获取用户列表
    // 3. 禁用用户
    // 4. 启用用户
    // 5. 查看审计日志
}
```

#### 运行命令

```bash
# 运行所有端到端测试
go test -v -tags=e2e ./test/e2e/...

# 运行单个测试
go test -v -tags=e2e ./test/e2e/... -run TestAuthFlow

# 跳过端到端测试（默认）
go test -short ./...
```

---

## 三、CI/CD改进

### 3.1 增强CI配置

修改文件: `.github/workflows/ci.yml`

#### 新增作业

```yaml
e2e-tests:
    runs-on: ubuntu-latest
    steps:
        - uses: actions/checkout@v4
        
        - name: Set up Go
          uses: actions/setup-go@v5
          with:
              go-version: '1.26'
        
        - name: Run e2e tests
          run: go test -v -tags=e2e ./test/e2e/...
          env:
              BASE_URL: ${{ secrets.E2E_BASE_URL }}
```

### 3.2 安全扫描增强

```yaml
security:
    runs-on: ubuntu-latest
    steps:
        - uses: actions/checkout@v4
        
        - name: Run govulncheck
          run: |
              go install golang.org/x/vuln/cmd/govulncheck@latest
              govulncheck ./...
        
        - name: Run gitleaks
          uses: gitleaks/gitleaks-action@v2
```

---

## 四、执行计划

### 4.1 执行顺序

| 顺序 | 任务 | 优先级 | 预计时间 | 依赖 |
|------|------|--------|----------|------|
| 1 | 修复命名冲突 | 高 | 15分钟 | 无 |
| 2 | 添加导出函数注释 | 中 | 10分钟 | 无 |
| 3 | 优化密钥路径前缀 | 中 | 5分钟 | 无 |
| 4 | 优化N+1查询 | 高 | 10分钟 | 无 |
| 5 | 添加配置测试 | 高 | 30分钟 | 无 |
| 6 | 提高集成测试覆盖率 | 中 | 45分钟 | 任务5 |
| 7 | 添加端到端测试 | 中 | 60分钟 | 任务1 |
| 8 | 增强CI/CD配置 | 低 | 20分钟 | 任务7 |

### 4.2 验证步骤

每个任务完成后执行：

```bash
# 1. 运行单元测试
make test-unit

# 2. 运行lint检查
make lint

# 3. 检查代码覆盖率
make test-coverage

# 4. 查看覆盖率变化
go tool cover -func=coverage.out
```

### 4.3 风险评估

| 任务 | 风险 | 缓解措施 |
|------|------|----------|
| 修复命名冲突 | 可能遗漏引用 | 全面搜索+grep确认 |
| 优化N+1查询 | SQL兼容性 | 使用PostgreSQL特定语法 |
| 添加端到端测试 | 需要测试环境 | 使用 `-short` 跳过 |

---

## 五、验收标准

### 5.1 完成标准

- [ ] 所有命名冲突已修复
- [ ] 导出函数都有注释
- [ ] 密钥路径前缀已优化
- [ ] N+1查询已优化
- [ ] 配置测试覆盖率 > 80%
- [ ] 集成测试覆盖率 > 50%
- [ ] 端到端测试覆盖主要流程
- [ ] CI/CD包含e2e测试和安全扫描

### 5.2 质量标准

```bash
# 所有测试通过
go test ./...  # 必须通过

# lint无错误
golangci-lint run  # 无新警告

# 代码覆盖率不下降
go test -cover ./...  # 覆盖率应保持或提高
```

---

## 六、文档更新

### 6.1 需要更新的文档

| 文档 | 更新内容 |
|------|----------|
| `docs/ARCHITECTURE.md` | 添加缓存优化说明 |
| `docs/guides/CONTRIBUTING.md` | 添加端到端测试说明 |
| `README.md` | 添加测试运行说明 |

### 6.2 新增文档

| 文档 | 内容 |
|------|------|
| `docs/testing.md` | 测试指南 |

---

**计划制定人**: AI Assistant  
**计划审核人**: -  
**计划状态**: 待执行
