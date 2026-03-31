# 测试环境检查清单

**目的：确保所有测试能够正常运行，禁止因环境问题跳过测试！**

## ✅ 必需环境检查

### 1. 数据库连接（PostgreSQL）

**状态：** ✅ 已配置

```bash
# 测试连接
nc -zv 192.168.1.3 5432

# 或使用psql
psql "postgres://sso:sso@192.168.1.3:5432/sso_test?sslmode=disable" -c "SELECT 1"
```

**配置信息：**
- 主机：`192.168.1.3`
- 端口：`5432`
- 数据库：`sso_test`
- 用户：`sso`
- 密码：`sso`
- SSL模式：`disable`（测试环境）

**环境变量：**
```bash
export DATABASE_URL="postgres://sso:sso@192.168.1.3:5432/sso_test?sslmode=disable"
```

### 2. Redis连接

**状态：** ✅ 已配置

```bash
# 测试连接
nc -zv 192.168.1.3 30059

# 或使用redis-cli
redis-cli -h 192.168.1.3 -p 30059 ping
```

**配置信息：**
- 主机：`192.168.1.3`
- 端口：`30059`
- 密码：无

### 3. JWT密钥文件

**状态：** ✅ 已存在

```bash
# 检查密钥文件
ls -la keys/private.pem keys/public.pem

# 如果不存在，生成密钥
make generate-keys
```

**文件位置：**
- 私钥：`./keys/private.pem`
- 公钥：`./keys/public.pem`

### 4. 必需工具

#### gotestsum（测试运行器）

**状态：** ✅ 已安装

```bash
# 检查
which gotestsum

# 安装
go install gotest.tools/gotestsum@latest
```

#### golangci-lint（代码检查）

**状态：** ✅ 已安装

```bash
# 检查
which golangci-lint

# 安装
curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh | sh -s -- -b $(go env GOPATH)/bin
```

#### migrate（数据库迁移）

**状态：** ❌ 未安装

```bash
# 检查
which migrate

# 安装
go install -tags 'postgres' github.com/golang-migrate/migrate/v4/cmd/migrate@latest
```

#### govulncheck（安全扫描）

```bash
# 检查
which govulncheck

# 安装
go install golang.org/x/vuln/cmd/govulncheck@latest
```

### 5. SMTP配置（邮件测试）

**状态：** ✅ 已配置（.env.test）

```bash
SMTP_HOST=smtp.qiye.aliyun.com
SMTP_PORT=465
SMTP_USER=system@eninte.com
SMTP_PASSWORD=123system,./
SMTP_FROM=system@eninte.com
```

**注意：** 如果SMTP密码已更改，请更新`.env.test`文件。

### 6. E2E测试配置

**状态：** ✅ 已配置

```bash
export E2E_ADMIN_EMAIL="system@eninte.com"
export E2E_ADMIN_PASSWORD="Admin123!"
```

## 🔍 当前发现的问题

### 问题1：集成测试会跳过（如果未设置DATABASE_URL）

**位置：** `internal/store/postgres/postgres_test.go`

**代码：**
```go
dbURL := os.Getenv("DATABASE_URL")
if dbURL == "" {
    t.Skip("跳过集成测试：未设置DATABASE_URL环境变量")
}
```

**解决方案：**
```bash
# 方案1：设置环境变量
export DATABASE_URL="postgres://sso:sso@192.168.1.3:5432/sso_test?sslmode=disable"

# 方案2：使用Makefile（已自动设置）
make test

# 方案3：直接运行测试时设置
DATABASE_URL="postgres://sso:sso@192.168.1.3:5432/sso_test?sslmode=disable" go test ./...
```

**验证：**
```bash
# 确认环境变量已设置
echo $DATABASE_URL

# 运行集成测试
make test
```

## 📋 测试前检查清单

运行测试前，请确认以下所有项：

- [ ] 数据库连接正常（`nc -zv 192.168.1.3 5432`）
- [ ] Redis连接正常（`nc -zv 192.168.1.3 30059`）
- [ ] JWT密钥文件存在（`ls keys/*.pem`）
- [ ] DATABASE_URL环境变量已设置
- [ ] gotestsum已安装
- [ ] golangci-lint已安装
- [ ] migrate已安装（如需运行迁移）
- [ ] .env.test文件存在且配置正确

## 🚀 快速验证脚本

创建并运行以下脚本来验证环境：

```bash
#!/bin/bash
# test-env-check.sh

echo "=========================================="
echo "  SSO测试环境检查"
echo "=========================================="
echo ""

# 检查数据库连接
echo "1. 检查数据库连接..."
if nc -zv 192.168.1.3 5432 2>&1 | grep -q "succeeded"; then
    echo "   ✅ 数据库连接正常"
else
    echo "   ❌ 数据库连接失败"
    exit 1
fi

# 检查Redis连接
echo "2. 检查Redis连接..."
if nc -zv 192.168.1.3 30059 2>&1 | grep -q "succeeded"; then
    echo "   ✅ Redis连接正常"
else
    echo "   ❌ Redis连接失败"
    exit 1
fi

# 检查JWT密钥
echo "3. 检查JWT密钥..."
if [ -f "keys/private.pem" ] && [ -f "keys/public.pem" ]; then
    echo "   ✅ JWT密钥存在"
else
    echo "   ❌ JWT密钥不存在，运行: make generate-keys"
    exit 1
fi

# 检查DATABASE_URL
echo "4. 检查DATABASE_URL环境变量..."
if [ -n "$DATABASE_URL" ]; then
    echo "   ✅ DATABASE_URL已设置: $DATABASE_URL"
else
    echo "   ⚠️  DATABASE_URL未设置（Makefile会自动设置）"
fi

# 检查工具
echo "5. 检查必需工具..."
if which gotestsum > /dev/null; then
    echo "   ✅ gotestsum已安装"
else
    echo "   ❌ gotestsum未安装，运行: go install gotest.tools/gotestsum@latest"
fi

if which golangci-lint > /dev/null; then
    echo "   ✅ golangci-lint已安装"
else
    echo "   ⚠️  golangci-lint未安装（建议安装）"
fi

if which migrate > /dev/null; then
    echo "   ✅ migrate已安装"
else
    echo "   ⚠️  migrate未安装（如需迁移请安装）"
fi

echo ""
echo "=========================================="
echo "  环境检查完成"
echo "=========================================="
```

## 🎯 运行测试的正确方式

### 方式1：使用Makefile（推荐）

```bash
# 运行所有测试（自动设置DATABASE_URL）
make test

# 运行单元测试
make test-unit

# 运行集成测试
make test-integration

# 生成覆盖率报告
make test-coverage
```

### 方式2：手动设置环境变量

```bash
# 设置环境变量
export DATABASE_URL="postgres://sso:sso@192.168.1.3:5432/sso_test?sslmode=disable"

# 运行测试
go test -v -race ./...
```

### 方式3：一次性设置

```bash
DATABASE_URL="postgres://sso:sso@192.168.1.3:5432/sso_test?sslmode=disable" go test -v ./...
```

## ⚠️ 重要提醒

1. **禁止跳过测试**：如果测试因环境问题失败，必须修复环境，不能跳过测试
2. **DATABASE_URL必须设置**：集成测试需要真实数据库连接
3. **网络连接**：确保能访问`192.168.1.3`的PostgreSQL和Redis
4. **密钥文件**：首次运行前必须生成JWT密钥
5. **工具安装**：确保所有必需工具已安装

## 📞 需要提供的信息

如果测试仍然失败，请提供以下信息：

1. 错误日志：`make test 2>&1 | tee test-error.log`
2. 环境变量：`echo $DATABASE_URL`
3. 网络连接：`nc -zv 192.168.1.3 5432` 和 `nc -zv 192.168.1.3 30059`
4. 密钥文件：`ls -la keys/`
5. 工具版本：`go version` 和 `gotestsum --version`

## ✅ 确认测试环境就绪

运行以下命令确认环境完全就绪：

```bash
# 1. 检查环境
bash test-env-check.sh

# 2. 运行测试
make test

# 3. 检查是否有跳过的测试
make test 2>&1 | grep -i "skip"
```

**如果看到任何"skip"，立即报告！我们必须修复环境，不能跳过测试！**
