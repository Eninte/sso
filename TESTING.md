# SSO服务 - 测试指南

本文档详细说明SSO服务的测试规范、最佳实践和常见问题。

## ⚠️ 测试前必读

**在运行任何测试之前，请先运行环境检查：**

```bash
bash scripts/test-env-check.sh
```

详细的环境要求和配置请参考：[TEST_ENVIRONMENT_CHECKLIST.md](./TEST_ENVIRONMENT_CHECKLIST.md)

**重要原则：**
- ❌ 禁止因环境问题跳过测试
- ❌ 禁止因功能未实现跳过测试
- ✅ 必须修复环境问题后再运行测试
- ✅ 必须实现功能后再运行测试

## 测试命令

### 基础测试

```bash
make test                         # 全部测试（含-race检测）
make test-unit                    # 仅单元测试（短测试）
make test-integration             # 集成测试（-tags=integration）
make test-coverage                # 生成覆盖率报告
make bench                        # 全部基准测试
```

### 单个测试运行

```bash
# 运行特定测试函数
go test -v -run TestAuthService_Login ./internal/service/

# 运行特定子测试
go test -v -run TestAuthService_Register/邮箱已存在 ./internal/service/

# 运行特定包的所有测试（含竞态检测）
go test -v -race -count=1 ./internal/handler/...

# 运行单个测试文件
go test -v ./internal/service/auth_test.go
```

### 测试选项

```bash
# 竞态检测
go test -race ./...

# 多次运行（检测不稳定测试）
go test -count=10 ./internal/service/

# 显示详细输出
go test -v ./...

# 超时控制
go test -timeout 30s ./...

# 并行控制
go test -parallel 4 ./...
```

## 测试规范

### 基本原则

1. **黑盒测试**：使用`package service_test`（非`package service`）
2. **测试框架**：`testify/assert` + `testify/require`
3. **测试风格**：优先使用表驱动测试
4. **命名规范**：`TestFunctionName_场景`

### 测试命名示例

```go
// ✅ 好的命名
func TestAuthService_Register_邮箱已存在(t *testing.T)
func TestAuthService_Login_密码错误(t *testing.T)
func TestUserStore_GetByID_用户不存在(t *testing.T)

// ❌ 不好的命名
func TestRegister(t *testing.T)
func TestLogin1(t *testing.T)
func TestGetUser(t *testing.T)
```

### 表驱动测试模板

```go
func TestAuthService_Register(t *testing.T) {
    tests := []struct {
        name    string
        email   string
        wantErr error
        setup   func(*mock.Store)
    }{
        {
            name:    "成功注册",
            email:   "test@example.com",
            wantErr: nil,
            setup: func(m *mock.Store) {
                m.GetUserByEmailErr = store.ErrNotFound
            },
        },
        {
            name:    "邮箱已存在",
            email:   "existing@example.com",
            wantErr: store.ErrDuplicateEmail,
            setup: func(m *mock.Store) {
                m.GetUserByEmailErr = nil
            },
        },
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            mockStore := mock.New()
            if tt.setup != nil {
                tt.setup(mockStore)
            }
            
            svc := service.NewAuthService(mockStore)
            err := svc.Register(tt.email)
            
            if tt.wantErr != nil {
                assert.ErrorIs(t, err, tt.wantErr)
            } else {
                assert.NoError(t, err)
            }
        })
    }
}
```

## Mock使用

### Mock Store基础

```go
// 创建Mock实例
mockStore := mock.New()

// 重置Mock数据（每个测试前调用）
mockStore.Reset()

// 设置返回数据
mockStore.Users = []model.User{
    {ID: 1, Email: "test@example.com"},
}

// 注入错误
mockStore.CreateUserErr = store.ErrDuplicateEmail
mockStore.GetUserByIDErr = store.ErrNotFound
```

### 常用错误注入字段

```go
// User相关
mockStore.CreateUserErr = store.ErrDuplicateEmail
mockStore.GetUserByIDErr = store.ErrNotFound
mockStore.GetUserByEmailErr = store.ErrNotFound
mockStore.UpdateUserErr = errors.New("update failed")

// Session相关
mockStore.CreateSessionErr = errors.New("session failed")
mockStore.GetSessionErr = store.ErrNotFound
```

### 测试隔离

```go
func TestMultipleScenarios(t *testing.T) {
    t.Run("场景1", func(t *testing.T) {
        mockStore := mock.New()
        // 场景1的测试逻辑
    })
    
    t.Run("场景2", func(t *testing.T) {
        mockStore := mock.New() // 新实例，完全隔离
        // 场景2的测试逻辑
    })
}
```

## 性能优化

### Bcrypt Cost

测试中使用较低的bcrypt cost以加快速度：

```go
// 测试中使用
passwordService := crypto.NewPasswordService(10)

// 生产环境使用
passwordService := crypto.NewPasswordService(12) // 或更高
```

### 并行测试

```go
func TestParallel(t *testing.T) {
    t.Parallel() // 标记为可并行运行
    
    // 测试逻辑
}
```

## E2E测试

### 环境准备

1. 启动服务：`make dev`（服务运行在`http://localhost:9090`）
2. 配置环境变量：
   ```bash
   export E2E_ADMIN_EMAIL="system@eninte.com"
   export E2E_ADMIN_PASSWORD="Admin123!"
   ```
3. 确保数据库连接可用

### 运行E2E测试

```bash
# 使用Makefile
make test-e2e

# 手动运行
E2E_ADMIN_EMAIL="system@eninte.com" \
E2E_ADMIN_PASSWORD="Admin123!" \
go test -v -tags=e2e ./test/e2e/...

# 运行特定E2E测试
go test -v -tags=e2e -run TestE2E_UserRegistration ./test/e2e/
```

### E2E测试结构

```go
//go:build e2e

package e2e_test

import (
    "testing"
    "net/http"
)

func TestE2E_UserFlow(t *testing.T) {
    baseURL := "http://localhost:9090"
    
    // 1. 注册用户
    resp := registerUser(t, baseURL, "test@example.com")
    assert.Equal(t, http.StatusCreated, resp.StatusCode)
    
    // 2. 登录
    token := login(t, baseURL, "test@example.com", "password")
    assert.NotEmpty(t, token)
    
    // 3. 访问受保护资源
    profile := getProfile(t, baseURL, token)
    assert.Equal(t, "test@example.com", profile.Email)
}
```

## 基准测试

### 运行基准测试

```bash
# 运行所有基准测试
make bench

# 运行特定包的基准测试
go test -bench=. -benchmem ./internal/cache/

# 运行特定基准测试
go test -bench=BenchmarkPasswordHash -benchmem ./internal/crypto/

# 控制运行时间
go test -bench=. -benchtime=10s ./internal/cache/

# 控制运行次数
go test -bench=. -count=5 ./internal/cache/
```

### 基准测试对比

```bash
# 保存基准测试结果
go test -bench=. -benchmem ./internal/cache/ > old.txt

# 修改代码后重新测试
go test -bench=. -benchmem ./internal/cache/ > new.txt

# 对比结果（需要安装benchcmp）
benchcmp old.txt new.txt

# 或使用benchstat（更推荐）
go install golang.org/x/perf/cmd/benchstat@latest
benchstat old.txt new.txt
```

### 基准测试示例

```go
func BenchmarkPasswordHash(b *testing.B) {
    ps := crypto.NewPasswordService(10)
    password := "TestPassword123!"
    
    b.ResetTimer()
    for i := 0; i < b.N; i++ {
        _, _ = ps.HashPassword(password)
    }
}

func BenchmarkCacheGet(b *testing.B) {
    cache := setupCache(b)
    cache.Set("key", "value")
    
    b.ResetTimer()
    b.RunParallel(func(pb *testing.PB) {
        for pb.Next() {
            _, _ = cache.Get("key")
        }
    })
}
```

## 测试覆盖率

### 生成覆盖率报告

```bash
# 生成覆盖率报告
make test-coverage

# 手动生成
go test -coverprofile=coverage.out ./...
go tool cover -html=coverage.out -o coverage.html

# 查看覆盖率统计
go tool cover -func=coverage.out

# 按包查看覆盖率
go test -cover ./...
```

### 覆盖率要求

- 整体覆盖率：>= 80%
- 核心业务逻辑：>= 90%
- Handler层：>= 85%
- Service层：>= 90%
- Store层：>= 85%

## ⛔ 严禁事项

### 禁止跳过测试

**绝对禁止使用 `t.Skip()` 跳过测试！这是严重的代码质量问题。**

```go
// ❌ 绝对禁止
func TestSomething(t *testing.T) {
    t.Skip("功能未实现")
    t.Skip("环境问题")
    t.Skip("端点未实现")
}

// ✅ 正确做法：实现功能后再运行测试
```

### 禁止宽松断言

```go
// ❌ 过于宽松
assert.True(t, code >= 400)
assert.NotNil(t, err)

// ✅ 精确断言
assert.Equal(t, http.StatusBadRequest, code)
assert.ErrorIs(t, err, store.ErrNotFound)
```

### 禁止测试污染

```go
// ❌ 共享状态导致测试污染
var globalMock = mock.New()

func TestA(t *testing.T) {
    globalMock.Users = []model.User{{ID: 1}}
}

func TestB(t *testing.T) {
    // TestA的数据会影响TestB
}

// ✅ 每个测试独立创建
func TestA(t *testing.T) {
    mockStore := mock.New()
    mockStore.Users = []model.User{{ID: 1}}
}

func TestB(t *testing.T) {
    mockStore := mock.New()
    // 完全独立
}
```

## 测试发现Bug的处理流程

当测试发现功能缺失或Bug时：

1. **不要跳过测试** - 立即停止
2. **分析问题根因** - 确定是代码问题还是测试问题
3. **实现缺失的功能** - 修复代码
4. **运行测试验证** - 确保测试通过
5. **回归测试** - 运行完整测试套件
6. **更新文档** - 记录新增功能或修复

## 常见测试问题

### 数据库连接问题

```bash
# 问题：测试超时或连接失败
# 解决：检查数据库连接
DATABASE_URL="postgres://sso:sso@192.168.1.3:5432/sso_test?sslmode=disable" make test

# 检查网络连接
ping 192.168.1.3
telnet 192.168.1.3 5432
```

### Mock数据污染

```go
// 问题：测试之间相互影响
// 解决：每个测试前重置Mock
func TestSomething(t *testing.T) {
    mockStore := mock.New()
    mockStore.Reset() // 确保清空数据
    
    // 测试逻辑
}
```

### 竞态条件

```bash
# 问题：测试偶尔失败
# 解决：使用-race检测竞态条件
go test -race ./...

# 多次运行检测不稳定测试
go test -count=100 ./internal/service/
```

### 测试超时

```go
// 问题：测试运行时间过长
// 解决：设置合理的超时时间
func TestWithTimeout(t *testing.T) {
    ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
    defer cancel()
    
    // 使用ctx进行测试
}
```

## 集成测试

### 标签使用

```go
//go:build integration

package service_test

// 集成测试代码
```

### 运行集成测试

```bash
# 仅运行集成测试
make test-integration

# 手动运行
go test -tags=integration ./...

# 排除集成测试
go test -short ./...
```

### 集成测试环境

集成测试需要：
- 数据库连接：`192.168.1.3:5432`
- Redis连接：`192.168.1.3:30059`
- 测试数据库：`sso_test`

## 测试最佳实践

### 1. 测试独立性

每个测试应该独立运行，不依赖其他测试的执行顺序。

```go
func TestIndependent(t *testing.T) {
    // 创建独立的测试环境
    mockStore := mock.New()
    svc := service.NewAuthService(mockStore)
    
    // 测试逻辑
}
```

### 2. 清晰的测试意图

测试名称和结构应该清晰表达测试意图。

```go
func TestAuthService_Login_密码错误时返回错误(t *testing.T) {
    // Arrange
    mockStore := mock.New()
    svc := service.NewAuthService(mockStore)
    
    // Act
    err := svc.Login("test@example.com", "wrong-password")
    
    // Assert
    assert.ErrorIs(t, err, apperrors.ErrInvalidCredentials)
}
```

### 3. 边界条件测试

测试边界条件和异常情况。

```go
func TestValidation(t *testing.T) {
    tests := []struct {
        name  string
        input string
        valid bool
    }{
        {"空字符串", "", false},
        {"最小长度", "a", false},
        {"正常长度", "valid@example.com", true},
        {"超长字符串", strings.Repeat("a", 1000), false},
        {"特殊字符", "test+tag@example.com", true},
    }
    
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            valid := validateEmail(tt.input)
            assert.Equal(t, tt.valid, valid)
        })
    }
}
```

### 4. 使用子测试

使用子测试组织相关测试用例。

```go
func TestAuthService(t *testing.T) {
    t.Run("Register", func(t *testing.T) {
        t.Run("成功注册", func(t *testing.T) { /* ... */ })
        t.Run("邮箱已存在", func(t *testing.T) { /* ... */ })
    })
    
    t.Run("Login", func(t *testing.T) {
        t.Run("成功登录", func(t *testing.T) { /* ... */ })
        t.Run("密码错误", func(t *testing.T) { /* ... */ })
    })
}
```

### 5. 测试辅助函数

提取通用的测试辅助函数。

```go
// 测试辅助函数
func setupTestService(t *testing.T) (*service.AuthService, *mock.Store) {
    t.Helper()
    mockStore := mock.New()
    svc := service.NewAuthService(mockStore)
    return svc, mockStore
}

func TestWithHelper(t *testing.T) {
    svc, mockStore := setupTestService(t)
    // 使用svc和mockStore进行测试
}
```

## 持续集成

### CI测试流程

```yaml
# .github/workflows/test.yml 示例
- name: Run tests
  run: |
    make test
    make test-integration
    make test-coverage
    
- name: Check coverage
  run: |
    go tool cover -func=coverage.out | grep total | awk '{print $3}' | sed 's/%//' | \
    awk '{if ($1 < 80) exit 1}'
```

### 提交前检查

```bash
# 运行完整检查
make test
make lint
make test-security
make test-coverage
```
