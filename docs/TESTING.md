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

## E2E端到端测试

### 概述

E2E测试验证完整的用户流程，包括注册、登录、Token管理、OAuth授权等。测试需要真实的服务运行环境。

**当前测试状态**：
- 总测试数：156个
- 通过率：94.9% (148/156)
- 执行时间：约75秒

### 快速开始

```bash
# 1. 启动服务（禁用限流）
RATE_LIMIT_REQUESTS=0 make run &

# 2. 准备测试数据（启用自动验证触发器）
make test-e2e-prepare

# 3. 运行E2E测试
make test-e2e

# 4. 清理测试环境（禁用触发器）
make test-e2e-cleanup
```

### 一键测试

```bash
# 完整测试流程（准备 + 测试）
make test-e2e-full

# 测试完成后清理
make test-e2e-cleanup
```

### 环境要求

1. **服务运行中**：SSO服务必须在 `localhost:9090` 运行
2. **数据库可访问**：PostgreSQL测试数据库可连接
3. **Redis可访问**：Redis缓存服务可连接
4. **限流已禁用**：服务必须以 `RATE_LIMIT_REQUESTS=0` 启动

### 测试数据准备机制

E2E测试使用**PostgreSQL触发器**自动验证测试用户：

```sql
-- 自动验证 @example.com 域名的测试用户
CREATE TRIGGER trigger_auto_verify_test_users
    BEFORE INSERT ON users
    FOR EACH ROW
    EXECUTE FUNCTION auto_verify_test_users();
```

**优点**：
- ✅ 不污染生产代码
- ✅ 测试时自动生效
- ✅ 测试后可完全移除
- ✅ 仅影响测试环境

**重要提示**：
- ⚠️ 触发器仅用于测试环境，**禁止在生产环境使用**
- ⚠️ 测试完成后必须运行清理脚本禁用触发器
- ⚠️ 触发器会自动验证所有新注册的 `@example.com` 用户

### 手动操作

如果需要手动操作数据库：

```bash
# 启用触发器
psql "$DATABASE_URL" -f scripts/enable-auto-verify-test-users.sql

# 禁用触发器
psql "$DATABASE_URL" -f scripts/disable-auto-verify-test-users.sql

# 手动验证管理员
psql "$DATABASE_URL" -c "UPDATE users SET email_verified=true, role='admin' WHERE email='system@eninte.com';"

# 手动验证所有测试用户
psql "$DATABASE_URL" -c "UPDATE users SET email_verified=true WHERE email LIKE '%@example.com';"
```

### 测试覆盖范围

| 测试类别 | 测试数 | 说明 |
|---------|--------|------|
| 健康检查 | 2 | 服务健康状态检查 |
| 注册流程 | 8 | 用户注册、验证 |
| 登录流程 | 12 | 登录、多设备登录 |
| Token管理 | 18 | Token验证、刷新、撤销 |
| 邮箱验证 | 10 | 邮箱验证流程 |
| 密码重置 | 12 | 忘记密码、重置密码 |
| 管理员功能 | 15 | 用户管理、审计日志 |
| OAuth流程 | 20 | OAuth授权流程 |
| 并发测试 | 25 | 并发注册、登录、刷新 |
| 安全测试 | 18 | SQL注入、XSS、边界值 |
| 错误处理 | 16 | 异常情况处理 |

### 常见问题

**Q: 测试失败提示"connection refused"**

A: 服务未启动或端口不是9090。请确保：
```bash
# 检查服务是否运行
curl http://localhost:9090/health

# 如果未运行，启动服务
RATE_LIMIT_REQUESTS=0 make run
```

**Q: 测试失败提示"429 Too Many Requests"**

A: 限流未禁用。必须以 `RATE_LIMIT_REQUESTS=0` 启动服务。

**Q: 测试失败提示"401 Unauthorized"**

A: 用户邮箱未验证。运行准备脚本：
```bash
make test-e2e-prepare
```

**Q: 如何清理测试数据？**

A: 运行清理脚本并选择清理数据：
```bash
make test-e2e-cleanup
# 提示时输入 'y' 确认清理
```

### 详细文档

完整的E2E测试说明、故障排查和最佳实践请参考：[E2E测试指南](./E2E_TESTING.md)

## E2E测试

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

## 压力测试

详细的执行清单、场景顺序、数据准备和结果记录模板请参考：[PRESSURE_TESTING_RUNBOOK.md](./PRESSURE_TESTING_RUNBOOK.md)

### 目标

压力测试用于验证SSO服务在真实混合流量下的容量上限、延迟表现、保护机制和长时间运行稳定性。

- 容量目标：找出最大稳定吞吐与系统拐点
- 性能目标：观察平均延迟、P95、P99、错误率和恢复时间
- 稳定性目标：识别内存增长、连接池耗尽、goroutine泄漏和缓存抖动
- 安全目标：验证限流、账户锁定、无效Token、重复授权码等保护行为

### 测试范围

本项目建议覆盖以下压力测试范围：

- 公开接口：`/health`、`/.well-known/openid-configuration`、`/.well-known/jwks.json`
- 认证接口：`/api/v1/register`、`/api/v1/login`、`/api/v1/token`、`/api/v1/token/revoke`
- 受保护接口：`/api/v1/userinfo`、`/api/v1/mfa/status`
- OAuth/OIDC流程：`/api/v1/authorize` → `/api/v1/token` → `/api/v1/userinfo`
- 管理员接口：`/api/v1/admin/health`、`/api/v1/admin/users`、`/api/v1/admin/audit-logs`

### 环境模式

#### 1. 容量模式

用于测业务链路极限，不测保护机制。

- 限流：关闭
- 登录保护：可关闭或降低影响
- 适用场景：登录、注册、userinfo、token刷新、OAuth完整流程

#### 2. 保护模式

用于验证系统在恶意流量下是否按预期保护自身。

- 限流：开启
- 登录保护：开启
- 适用场景：错误密码风暴、同邮箱并发注册、无效Token风暴、无效授权码与PKCE校验

#### 3. 稳态模式

用于识别慢性问题和资源泄漏。

- 限流：与容量模式保持一致
- 持续时间：60-120分钟
- 适用场景：混合流量、OAuth完整流程、userinfo高频访问

### 压测前准备清单

执行任何压力测试前，至少完成以下准备：

1. 服务已正常启动，`/health`返回200
2. 明确本次是容量模式还是保护模式
3. 准备普通用户池、管理员用户池、恶意用户池
4. 准备access token池与refresh token池
5. 注册OAuth公共客户端与机密客户端
6. 注册场景使用唯一邮箱，避免把邮箱冲突误判为容量瓶颈
7. 邮件、审计、数据库、Redis依赖状态已确认
8. 明确本次观测面板、日志位置和结果保存目录

### 数据准备建议

#### 账号与Token池

- 登录压测：准备1000-5000个已验证普通账号
- userinfo压测：准备2000-10000个access token
- refresh token压测：准备1000-5000个refresh token
- 注册压测：预生成10000个以上唯一邮箱
- 管理员接口压测：准备5-20个管理员token
- 安全专项：准备100-500个恶意账号样本

#### OAuth客户端

建议预置两类客户端：

| 客户端类型 | 用途 | 核心配置 |
| ----------- | ------ | ---------- |
| 公共客户端 | SPA/移动端压测 | `public_client=true`，使用PKCE |
| 机密客户端 | 服务端应用压测 | `public_client=false`，使用`client_secret` |

建议统一使用以下基础配置：

- `redirect_uri`：`http://localhost:3000/callback`
- `grant_types`：`authorization_code`、`refresh_token`
- `scopes`：`openid`、`profile`、`email`

### 请求模板

#### 登录

```json
{
  "email": "user-0001@example.com",
  "password": "TestPassword123!"
}
```

#### 注册

```json
{
  "email": "register-<unique>@example.com",
  "password": "TestPassword123!"
}
```

#### 刷新Token

```json
{
  "grant_type": "refresh_token",
  "refresh_token": "<refresh_token>"
}
```

#### OAuth授权码交换（PKCE）

```json
{
  "grant_type": "authorization_code",
  "code": "<auth_code>",
  "redirect_uri": "http://localhost:3000/callback",
  "client_id": "public-test-client",
  "code_verifier": "<pkce_verifier>"
}
```

#### OAuth授权码交换（机密客户端）

```json
{
  "grant_type": "authorization_code",
  "code": "<auth_code>",
  "redirect_uri": "http://localhost:3000/callback",
  "client_id": "confidential-test-client",
  "client_secret": "<client_secret>"
}
```

### 场景清单

| 场景 | 目标 | 主要接口 | 模式 | 通过标准 |
| ------ | ------ | ---------- | ------ | ---------- |
| S1 | 公开读接口基线 | `/health`、`/.well-known/*` | 容量 | 基本无5xx，延迟稳定 |
| S2 | 登录容量 | `/api/v1/login` | 容量 | 找到稳定吞吐与CPU拐点 |
| S3 | 注册写入压力 | `/api/v1/register` | 容量 | 错误率受控，唯一邮箱策略有效 |
| S4 | 会话续期能力 | `/api/v1/token` | 容量 | refresh成功率稳定 |
| S5 | 受保护读路径 | `/api/v1/userinfo` | 容量/稳态 | 高成功率，P95稳定 |
| S6 | OAuth公共客户端完整流程 | `/api/v1/authorize` → `/api/v1/token` → `/api/v1/userinfo` | 容量/稳态 | 授权与换Token成功率稳定 |
| S7 | OAuth机密客户端完整流程 | `/api/v1/authorize` → `/api/v1/token` → `/api/v1/userinfo` | 容量 | 客户端校验与换Token稳定 |
| S8 | 混合流量 | 登录、注册、userinfo、token、OAuth、admin | 容量/稳态 | 接近真实业务且无系统性退化 |
| S9 | 安全保护专项 | 错误密码、无效Token、重复code | 保护 | 429/401/409符合预期 |
| S10 | 突刺与恢复 | 与S8相同 | 容量 | 峰值后延迟与错误率能回落 |

### 建议负载曲线

每个核心场景建议按照以下阶段推进：

1. 冒烟：1-5并发，确认接口与脚本正确
2. 基线：10、20、50并发，每档2分钟
3. 阶梯加压：50 → 100 → 200 → 300 → 500 → 800并发，每档3-5分钟
4. 峰值保持：选择拐点前一档，持续10-15分钟
5. 突刺：从常态1倍瞬间提升到3-5倍，持续1-3分钟
6. 稳态：以高峰50%-70%的流量运行60-120分钟

### 混合流量建议配比

如果要模拟真实SSO业务流量，推荐先使用以下比例：

- 40% `GET /api/v1/userinfo`
- 20% `POST /api/v1/token`（刷新）
- 15% `POST /api/v1/login`
- 8% `POST /api/v1/register`
- 10% OAuth公共客户端完整流程
- 5% OAuth机密客户端完整流程
- 2% 管理员接口

后续可根据线上真实访问占比再微调。

### 观测指标

#### 压测工具侧

- 实际吞吐（RPS）
- 平均延迟
- P90、P95、P99
- 最大延迟
- 成功率
- 4xx比例
- 5xx比例

#### 应用与系统侧

- `http_requests_total`
- `auth_login_total`
- `auth_login_failed_total`
- `auth_token_refresh_total`
- `security_rate_limit_total`
- `security_invalid_token_total`
- `cache_hits_total`
- `cache_misses_total`
- CPU、内存、goroutine、GC、数据库连接数、Redis响应时间

### 验收标准

建议首版压测采用以下统一门槛：

- 正常容量场景：5xx < 1%
- userinfo场景：成功率接近100%，P95稳定
- OAuth完整流程：授权成功率、换Token成功率、userinfo成功率稳定
- 保护模式：429、401、409、账户锁定等行为符合预期
- 稳态场景：内存、goroutine、连接池不持续增长
- 突刺场景：峰值结束后指标应快速回落

### 执行顺序建议

建议按以下顺序分批执行：

1. 第一天：S1、S2、S3、S4、S5
2. 第二天：S6、S7、S8
3. 第三天：S9、S10、Soak Test

这样可以先拿到单链路容量结论，再进入完整业务场景，最后验证安全与稳定性。

### 结果记录模板

每个场景建议统一记录以下信息：

| 字段 | 说明 |
| ------ | ------ |
| 场景编号 | S1-S10 |
| 场景名称 | 例如“userinfo高频读取” |
| 环境模式 | 容量/保护/稳态 |
| 数据池规模 | 用户数、Token数、客户端数 |
| 目标并发/RPS | 计划负载 |
| 实际吞吐 | 实际测得RPS |
| 平均延迟 | 平均响应时间 |
| P95/P99 | 延迟分位数 |
| 错误率 | 总体失败比例 |
| 4xx/5xx分类 | 便于区分业务失败和系统失败 |
| CPU/内存峰值 | 系统资源消耗 |
| 数据库/Redis异常 | 依赖侧问题 |
| 结论 | 是否达标、拐点位置、瓶颈判断 |

### 注意事项

- 登录和注册结果会显著受bcrypt成本影响，不适合直接代表全系统吞吐
- 容量模式与保护模式不能混跑，否则结果不可比
- 注册接口必须使用唯一邮箱
- userinfo压测必须使用预生成Token池，避免混入登录成本
- OAuth压测必须使用合法客户端和合法redirect URI
- 当前内建HTTP耗时指标不是直方图，P95/P99应以压测工具输出为准

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
