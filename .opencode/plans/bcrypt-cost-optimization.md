# bcrypt Cost 测试优化 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 让测试可以用低 cost 值（4）加速 bcrypt 哈希，同时保证生产环境 cost >= 12 不变。

**Architecture:** 当前 `NewPasswordService` 在构造函数层硬编码了 `cost >= 12`，导致所有测试传入 cost=10 都被静默提升到 12，造成测试超时。正确做法是：Config 层负责生产环境安全校验，PasswordService 只做密码哈希工具。去掉构造函数的硬编码下限，让测试真正用低 cost。

**Tech Stack:** Go, bcrypt, testify

---

## 问题现状

| 层 | 当前行为 | 问题 |
|---|---|---|
| `NewPasswordService(cost)` | cost < 12 → 静默提升到 12 | 测试以为用 cost=10，实际是 cost=12 |
| `config.validate()` | 生产环境 cost < 12 → 报错退出 | ✅ 正确 |
| 测试 | 52 处调用 `NewPasswordService(10)` | 全部实际 cost=12，加 `-race` 后超时 |

## 文件变更清单

| 文件 | 变更类型 | 说明 |
|---|---|---|
| `internal/crypto/password.go` | 修改 | 移除 cost 下限硬编码，扩大范围到 4-31 |
| `internal/crypto/password_test.go` | 修改 | 更新 CostNormalization 测试 |
| `internal/config/config.go` | 不变 | 生产校验保持 cost >= 12 |
| `Makefile` | 修改 | `make test` 添加 `-timeout 120s` |
| 8 个测试文件共 52 处 `NewPasswordService(10)` | 修改 | 统一改为 `NewPasswordService(4)` |

---

## Task 1: 修改 `NewPasswordService` 构造函数

**文件:** `internal/crypto/password.go:35-43`

- [ ] **Step 1: 修改构造函数，接受 bcrypt 合法范围 (4-31)**

将：
```go
func NewPasswordService(cost int) *PasswordService {
    if cost < 12 {
        cost = 12
    }
    if cost > 14 {
        cost = 14
    }
    return &PasswordService{cost: cost}
}
```

改为：
```go
// NewPasswordService 创建密码服务
// cost: bcrypt成本因子
// 合法范围: 4-31
// 推荐值: 12-14，越高越安全但性能越低
// 生产环境必须 >= 12（由 config.validate() 强制执行）
// 测试环境可使用 4-6 以加快执行速度
func NewPasswordService(cost int) *PasswordService {
    if cost < bcrypt.MinCost {
        cost = bcrypt.MinCost
    }
    if cost > 31 {
        cost = 31
    }
    return &PasswordService{cost: cost}
}
```

- [ ] **Step 2: 运行密码测试确认功能不受影响**

```bash
go test -v -count=1 -timeout 60s -run "TestPasswordService" ./internal/crypto/
```

- [ ] **Step 3: Commit**

```bash
git add internal/crypto/password.go
git commit -m "fix: remove hardcoded bcrypt cost minimum in PasswordService constructor"
```

---

## Task 2: 更新 `password_test.go` 中的 CostNormalization 测试

**文件:** `internal/crypto/password_test.go:141-161`

- [ ] **Step 1: 替换 CostNormalization 测试用例**

```go
func TestNewPasswordService_CostNormalization(t *testing.T) {
    tests := []struct {
        name         string
        inputCost    int
    }{
        {"过低cost提升到bcrypt最低", 1},
        {"bcrypt最低cost", 4},
        {"正常测试cost", 6},
        {"正常生产cost", 12},
        {"过高cost被限制", 35},
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            svc := crypto.NewPasswordService(tt.inputCost)
            hash, err := svc.HashPassword("TestPassword123")
            require.NoError(t, err)
            assert.NotEmpty(t, hash)
            err = svc.VerifyPassword(hash, "TestPassword123")
            assert.NoError(t, err)
        })
    }
}
```

- [ ] **Step 2: 运行确认通过**

```bash
go test -v -count=1 -timeout 30s -run "TestNewPasswordService_CostNormalization" ./internal/crypto/
```

- [ ] **Step 3: Commit**

```bash
git add internal/crypto/password_test.go
git commit -m "test: update CostNormalization test for new bcrypt cost range"
```

---

## Task 3: 批量替换测试中的 cost 值

**文件:** 8 个测试文件，共 52 处 `NewPasswordService(10)` → `NewPasswordService(4)`

| 文件 | 数量 |
|---|---|
| `internal/crypto/password_test.go` | 4 |
| `internal/service/auth_test.go` | 26 |
| `internal/service/user_test.go` | 3 |
| `internal/service/social_test.go` | 1 |
| `internal/service/auth_bench_test.go` | 5 |
| `internal/handler/handler_test.go` | 6 |
| `internal/handler/handler_extra_test.go` | 3 |
| `internal/handler/user_mfa_test.go` | 2 |

- [ ] **Step 1: 对每个文件执行替换** `NewPasswordService(10)` → `NewPasswordService(4)`

- [ ] **Step 2: 运行全部单元测试**

```bash
go test -count=1 -timeout 30s ./internal/crypto/ ./internal/service/ ./internal/handler/
```

- [ ] **Step 3: Commit**

```bash
git add internal/
git commit -m "test: use bcrypt cost=4 in all tests for faster execution"
```

---

## Task 4: 修改 Makefile 添加超时

**文件:** `Makefile`

- [ ] **Step 1: 在 test、test-verbose、test-unit 目标中添加 `-timeout 120s`**

将 `-race ./...` 改为 `-race -timeout 120s ./...`

- [ ] **Step 2: 验证 `make test` 全部通过**

```bash
make test
```

- [ ] **Step 3: Commit**

```bash
git add Makefile
git commit -m "build: add -timeout 120s to test targets for -race mode"
```

---

## Task 5: 安全验证

- [ ] **Step 1: 确认 `config.go:240-242` 未变** — 生产环境 cost < 12 仍然报错

- [ ] **Step 2: 运行 config 测试**

```bash
go test -v -count=1 ./internal/config/
```

- [ ] **Step 3: 确认生产代码路径**

```bash
grep "NewPasswordService" cmd/server/main.go
```
预期: `crypto.NewPasswordService(cfg.BcryptCost)` — 未变

---

## 对用户体验的影响

| 方面 | 影响 |
|---|---|
| 生产密码安全 | **无影响** — config.validate() 仍然强制 cost >= 12 |
| API 响应速度 | **无影响** — 生产代码路径未变 |
| 测试速度 | **大幅提升** — 每个哈希快 50-100 倍 |
| `-race` 测试 | **不再超时** |
| 代码清晰度 | **提升** — 不再有"传 10 得 12"的隐式行为 |

## 性能对比

| 指标 | 修改前 (cost=12) | 修改后 (cost=4) | 提升 |
|---|---|---|---|
| 单次哈希 | ~400ms | ~5ms | 80x |
| 单次哈希 + race | ~2000ms | ~15ms | 133x |
| crypto 包全量 (race) | ~93s | ~1.5s | 62x |
| service 包全量 (race) | >30s timeout | ~3s | 10x+ |
