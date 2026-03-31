#!/bin/bash
# ============================================================================
# SSO测试环境检查脚本
# 用途：验证所有测试环境依赖是否就绪
# ============================================================================

set -e

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

echo "=========================================="
echo "  SSO测试环境检查"
echo "=========================================="
echo ""

ERRORS=0
WARNINGS=0

# 检查数据库连接
echo "1. 检查数据库连接..."
if nc -zv 192.168.1.3 5432 2>&1 | grep -q "succeeded\|open"; then
    echo -e "   ${GREEN}✅ 数据库连接正常${NC}"
else
    echo -e "   ${RED}❌ 数据库连接失败${NC}"
    echo "      请确认: 192.168.1.3:5432 可访问"
    ERRORS=$((ERRORS + 1))
fi

# 检查Redis连接
echo "2. 检查Redis连接..."
if nc -zv 192.168.1.3 30059 2>&1 | grep -q "succeeded\|open"; then
    echo -e "   ${GREEN}✅ Redis连接正常${NC}"
else
    echo -e "   ${RED}❌ Redis连接失败${NC}"
    echo "      请确认: 192.168.1.3:30059 可访问"
    ERRORS=$((ERRORS + 1))
fi

# 检查JWT密钥
echo "3. 检查JWT密钥..."
if [ -f "keys/private.pem" ] && [ -f "keys/public.pem" ]; then
    echo -e "   ${GREEN}✅ JWT密钥存在${NC}"
    # 检查密钥权限
    PRIV_PERM=$(stat -c "%a" keys/private.pem 2>/dev/null || stat -f "%A" keys/private.pem 2>/dev/null)
    if [ "$PRIV_PERM" != "600" ]; then
        echo -e "   ${YELLOW}⚠️  私钥权限建议设置为600${NC}"
        WARNINGS=$((WARNINGS + 1))
    fi
else
    echo -e "   ${RED}❌ JWT密钥不存在${NC}"
    echo "      运行: make generate-keys"
    ERRORS=$((ERRORS + 1))
fi

# 检查DATABASE_URL
echo "4. 检查DATABASE_URL环境变量..."
if [ -n "$DATABASE_URL" ]; then
    echo -e "   ${GREEN}✅ DATABASE_URL已设置${NC}"
    echo "      $DATABASE_URL"
else
    echo -e "   ${YELLOW}⚠️  DATABASE_URL未设置${NC}"
    echo "      Makefile会自动设置，或手动设置:"
    echo "      export DATABASE_URL=\"postgres://sso:sso@192.168.1.3:5432/sso_test?sslmode=disable\""
    WARNINGS=$((WARNINGS + 1))
fi

# 检查工具
echo "5. 检查必需工具..."

if which gotestsum > /dev/null 2>&1; then
    echo -e "   ${GREEN}✅ gotestsum已安装${NC}"
else
    echo -e "   ${RED}❌ gotestsum未安装${NC}"
    echo "      运行: go install gotest.tools/gotestsum@latest"
    ERRORS=$((ERRORS + 1))
fi

if which golangci-lint > /dev/null 2>&1; then
    echo -e "   ${GREEN}✅ golangci-lint已安装${NC}"
else
    echo -e "   ${YELLOW}⚠️  golangci-lint未安装（建议安装）${NC}"
    echo "      运行: curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh | sh -s -- -b \$(go env GOPATH)/bin"
    WARNINGS=$((WARNINGS + 1))
fi

if which migrate > /dev/null 2>&1; then
    echo -e "   ${GREEN}✅ migrate已安装${NC}"
else
    echo -e "   ${YELLOW}⚠️  migrate未安装（如需迁移请安装）${NC}"
    echo "      运行: go install -tags 'postgres' github.com/golang-migrate/migrate/v4/cmd/migrate@latest"
    WARNINGS=$((WARNINGS + 1))
fi

if which govulncheck > /dev/null 2>&1; then
    echo -e "   ${GREEN}✅ govulncheck已安装${NC}"
else
    echo -e "   ${YELLOW}⚠️  govulncheck未安装（建议安装）${NC}"
    echo "      运行: go install golang.org/x/vuln/cmd/govulncheck@latest"
    WARNINGS=$((WARNINGS + 1))
fi

# 检查配置文件
echo "6. 检查配置文件..."
if [ -f ".env.test" ]; then
    echo -e "   ${GREEN}✅ .env.test存在${NC}"
else
    echo -e "   ${YELLOW}⚠️  .env.test不存在${NC}"
    echo "      运行: cp .env.example .env.test"
    WARNINGS=$((WARNINGS + 1))
fi

# 检查Go版本
echo "7. 检查Go版本..."
GO_VERSION=$(go version | awk '{print $3}' | sed 's/go//')
REQUIRED_VERSION="1.23"
if [ "$(printf '%s\n' "$REQUIRED_VERSION" "$GO_VERSION" | sort -V | head -n1)" = "$REQUIRED_VERSION" ]; then
    echo -e "   ${GREEN}✅ Go版本: $GO_VERSION${NC}"
else
    echo -e "   ${RED}❌ Go版本过低: $GO_VERSION (需要 >= $REQUIRED_VERSION)${NC}"
    ERRORS=$((ERRORS + 1))
fi

# 测试数据库连接（如果DATABASE_URL已设置）
if [ -n "$DATABASE_URL" ]; then
    echo "8. 测试数据库查询..."
    if psql "$DATABASE_URL" -c "SELECT 1" > /dev/null 2>&1; then
        echo -e "   ${GREEN}✅ 数据库查询成功${NC}"
    else
        echo -e "   ${YELLOW}⚠️  数据库查询失败（可能是psql未安装）${NC}"
        WARNINGS=$((WARNINGS + 1))
    fi
fi

echo ""
echo "=========================================="
echo "  检查结果"
echo "=========================================="
echo -e "错误: ${RED}$ERRORS${NC}"
echo -e "警告: ${YELLOW}$WARNINGS${NC}"
echo ""

if [ $ERRORS -gt 0 ]; then
    echo -e "${RED}❌ 环境检查失败！请修复上述错误后再运行测试。${NC}"
    exit 1
elif [ $WARNINGS -gt 0 ]; then
    echo -e "${YELLOW}⚠️  环境检查通过，但有警告。建议修复警告项。${NC}"
    exit 0
else
    echo -e "${GREEN}✅ 环境检查完全通过！可以运行测试。${NC}"
    echo ""
    echo "运行测试："
    echo "  make test              # 所有测试"
    echo "  make test-unit         # 单元测试"
    echo "  make test-integration  # 集成测试"
    echo "  make test-coverage     # 覆盖率报告"
    exit 0
fi
