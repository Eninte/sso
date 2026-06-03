# PostgreSQL 测试覆盖率统计问题说明

**日期**: 2026 年 4 月 3 日  
**问题**: postgres 包测试覆盖率显示不一致

---

## 问题描述

postgres 包的测试覆盖率在不同环境下显示结果差异巨大：

| 环境 | 覆盖率 | 说明 |
|------|--------|------|
| 未设置 `DATABASE_URL` | 1.7% | 所有集成测试被跳过 |
| 设置 `DATABASE_URL` | 74.9% | 正常运行 |

## 根本原因

postgres 包使用集成测试，需要真实数据库连接。测试代码中有以下检查：

```go
// internal/store/postgres/postgres_test.go
func getTestDB(t *testing.T) *sql.DB {
    t.Helper()
    dbURL := os.Getenv("DATABASE_URL")
    if dbURL == "" {
        t.Skip("跳过集成测试：未设置DATABASE_URL环境变量")
    }
    // ...
}
```

当 `DATABASE_URL` 环境变量未设置时，所有测试都会被跳过（`t.Skip()`），导致：
- 测试显示为 SKIP 状态
- 覆盖率统计仅包含少量初始化代码
- 覆盖率显示为 1.7%

## 验证方法

### 错误的运行方式（覆盖率 1.7%）

```bash
# 未设置环境变量
go test -cover ./internal/store/postgres/...
# 输出: coverage: 1.7% of statements
```

### 正确的运行方式（覆盖率 74.9%）

```bash
# 设置环境变量
DATABASE_URL="postgres://sso:sso@192.168.1.3:5432/sso_test?sslmode=disable" \
  go test -cover ./internal/store/postgres/...
# 输出: coverage: 74.9% of statements
```

### 使用 Makefile（推荐）

```bash
# Makefile 已配置环境变量
make test
make test-coverage
```

## 解决方案

### 1. 本地开发

使用 Makefile 命令，已自动配置环境变量：

```bash
make test              # 运行所有测试
make test-coverage     # 生成覆盖率报告
make test-verbose      # 详细输出
```

### 2. CI/CD 配置

确保在 CI/CD 管道中设置环境变量：

```yaml
# GitHub Actions 示例
env:
  DATABASE_URL: postgres://sso:sso@192.168.1.3:5432/sso_test?sslmode=disable

# GitLab CI 示例
variables:
  DATABASE_URL: postgres://sso:sso@192.168.1.3:5432/sso_test?sslmode=disable
```

### 3. 手动运行

如果需要手动运行测试：

```bash
# 方式 1: 导出环境变量
export DATABASE_URL="postgres://sso:sso@192.168.1.3:5432/sso_test?sslmode=disable"
go test -cover ./internal/store/postgres/...

# 方式 2: 内联环境变量
DATABASE_URL="postgres://sso:sso@192.168.1.3:5432/sso_test?sslmode=disable" \
  go test -cover ./internal/store/postgres/...
```

## 测试详情

### 测试文件

- `internal/store/postgres/postgres_test.go` (1151 行)
- `internal/store/postgres/postgres_bench_test.go` (性能测试)

### 测试覆盖范围

当正确运行时（74.9% 覆盖率），测试包括：

- ✅ 用户 CRUD 操作
- ✅ 令牌管理（access/refresh/verification）
- ✅ 授权码管理
- ✅ 审计日志
- ✅ MFA 恢复码
- ✅ 密钥轮换
- ✅ 错误处理
- ✅ 并发操作

### 未覆盖部分（25.1%）

需要补充测试的部分：
- 边界条件测试
- 数据库连接池测试
- 事务回滚测试
- 更多错误路径测试

## 影响范围

### 受影响的报告

如果未设置 `DATABASE_URL`，以下报告会显示错误的覆盖率：

- ❌ 测试质量报告：postgres 显示 1.7%
- ❌ 整体覆盖率：67.7%（实际应为 74.2%）
- ❌ 代码质量评分：7.2/10（实际应为 7.8/10）

### 正确的统计

设置 `DATABASE_URL` 后：

- ✅ postgres 覆盖率：74.9%
- ✅ 整体覆盖率：74.2%
- ✅ 代码质量评分：7.8/10 (B)

## 最佳实践

### 1. 始终使用 Makefile

```bash
# 推荐
make test

# 不推荐
go test ./...
```

### 2. 检查测试是否被跳过

```bash
# 查看详细输出
make test-verbose | grep SKIP

# 如果看到 "跳过集成测试：未设置DATABASE_URL环境变量"
# 说明环境变量未设置
```

### 3. 验证覆盖率

```bash
# 生成覆盖率报告
make test-coverage

# 检查 postgres 包覆盖率
DATABASE_URL="postgres://sso:sso@192.168.1.3:5432/sso_test?sslmode=disable" \
  go test -cover ./internal/store/postgres/...
```

## 总结

- postgres 包实际覆盖率为 **74.9%**，不是 1.7%
- 问题原因：集成测试需要 `DATABASE_URL` 环境变量
- 解决方案：使用 Makefile 或手动设置环境变量
- 整体覆盖率：**74.2%**（接近 80% 目标）
- 仅需补充 serviceutil 测试即可达到 80%+

---

**相关文档**:
- [测试质量报告](./code-analysis/05-测试质量报告.md)
- [TESTING.md](../../TESTING.md)
- [Makefile](../../Makefile)
