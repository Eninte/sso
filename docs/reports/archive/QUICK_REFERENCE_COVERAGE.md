# 测试覆盖率快速参考

**更新日期**: 2026 年 4 月 3 日

---

## 📊 当前覆盖率（正确数据）

```
整体覆盖率: 74.2%
目标覆盖率: 80%+
差距: -5.8%
```

### 各包覆盖率

| 包 | 覆盖率 | 状态 |
|---|--------|------|
| metrics | 100.0% | ⭐ 完美 |
| validator | 100.0% | ⭐ 完美 |
| handlerutil | 100.0% | ⭐ 完美 |
| logging | 96.6% | ✅ 优秀 |
| auditutil | 96.0% | ✅ 优秀 |
| errors | 93.6% | ✅ 优秀 |
| config | 90.0% | ✅ 优秀 |
| common | 88.9% | ✅ 优秀 |
| middleware | 83.9% | ✅ 优秀 |
| crypto | 81.8% | ✅ 达标 |
| postgres | 74.9% | ⚠️ 接近达标 |
| service | 74.5% | ⚠️ 需提升 |
| model | 72.7% | ⚠️ 需提升 |
| handler | 72.0% | ⚠️ 需提升 |
| cache | 67.2% | ⚠️ 需提升 |
| serviceutil | 0.0% | 🔴 无测试 |

---

## ⚠️ 重要提示

### postgres 包覆盖率问题

如果看到 postgres 覆盖率显示为 **1.7%**，说明：
- ❌ 测试时未设置 `DATABASE_URL` 环境变量
- ❌ 所有集成测试被跳过
- ❌ 覆盖率统计不准确

**正确的覆盖率**: 74.9%

---

## ✅ 正确的测试方法

### 方法 1: 使用 Makefile（推荐）

```bash
make test              # 运行所有测试
make test-coverage     # 生成覆盖率报告
make test-verbose      # 详细输出
```

### 方法 2: 手动设置环境变量

```bash
# 导出环境变量
export DATABASE_URL="postgres://sso:sso@192.168.1.3:5432/sso_test?sslmode=disable"
go test -cover ./...

# 或内联环境变量
DATABASE_URL="postgres://sso:sso@192.168.1.3:5432/sso_test?sslmode=disable" \
  go test -cover ./...
```

### 方法 3: 检查特定包

```bash
# 检查 postgres 包
DATABASE_URL="postgres://sso:sso@192.168.1.3:5432/sso_test?sslmode=disable" \
  go test -cover ./internal/store/postgres/...

# 应该显示: coverage: 74.9% of statements
```

---

## 🎯 改进目标

### 高优先级（1 周）

| 包 | 当前 | 目标 | 差距 | 工作量 |
|---|------|------|------|--------|
| serviceutil | 0.0% | 80%+ | -80.0% | 2-3 天 |

### 中优先级（2-4 周）

| 包 | 当前 | 目标 | 差距 | 工作量 |
|---|------|------|------|--------|
| postgres | 74.9% | 80%+ | -5.1% | 2-3 天 |
| cache | 67.2% | 80%+ | -12.8% | 2-3 天 |
| handler | 72.0% | 80%+ | -8.0% | 3-4 天 |
| service | 74.5% | 80%+ | -5.5% | 3-4 天 |
| model | 72.7% | 80%+ | -7.3% | 1-2 天 |

### 预期成果

完成所有改进后：
- 整体覆盖率: 74.2% → 82%+
- 所有核心包: >= 80%
- 测试质量评分: 7.8/10 → 8.5/10 (A-)

---

## 🔍 验证覆盖率

### 检查是否正确运行

```bash
# 运行测试并查看详细输出
make test-verbose | grep -E "SKIP|postgres"

# 如果看到 "跳过集成测试：未设置DATABASE_URL环境变量"
# 说明环境变量未设置，覆盖率会显示为 1.7%

# 正确运行时不应该看到 SKIP 消息
```

### 生成覆盖率报告

```bash
# 生成 HTML 报告
make test-coverage

# 查看报告
open coverage.html  # macOS
xdg-open coverage.html  # Linux
```

### 检查特定包

```bash
# 检查 postgres 包
DATABASE_URL="postgres://sso:sso@192.168.1.3:5432/sso_test?sslmode=disable" \
  go test -v -cover ./internal/store/postgres/... 2>&1 | head -20

# 应该看到测试运行，而不是 SKIP
```

---

## 📚 相关文档

- [POSTGRES_TEST_COVERAGE_ISSUE.md](./POSTGRES_TEST_COVERAGE_ISSUE.md) - 问题详细说明
- [COVERAGE_UPDATE_SUMMARY.md](./COVERAGE_UPDATE_SUMMARY.md) - 更新总结
- [05-测试质量报告.md](./code-analysis/05-测试质量报告.md) - 完整测试质量报告
- [TESTING.md](../../TESTING.md) - 测试指南
- [Makefile](../../Makefile) - 测试命令配置

---

## 🚀 快速开始

```bash
# 1. 克隆项目
git clone <repo-url>
cd sso

# 2. 配置测试环境
cp .env.example .env.test
# 编辑 .env.test（如果需要）

# 3. 运行测试
make test

# 4. 查看覆盖率
make test-coverage
open coverage.html

# 5. 验证 postgres 覆盖率
DATABASE_URL="postgres://sso:sso@192.168.1.3:5432/sso_test?sslmode=disable" \
  go test -cover ./internal/store/postgres/...
# 应该显示: coverage: 74.9% of statements
```

---

**最后更新**: 2026 年 4 月 3 日  
**维护者**: SSO 开发团队
