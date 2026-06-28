#!/bin/bash
# ============================================================================
# E2E测试数据准备脚本
# 用途：在运行E2E测试前，启用自动验证测试用户的数据库触发器
#
# 配置来源（优先级从高到低）：
#   1. 单独的环境变量：DB_HOST, DB_PORT, DB_USER, DB_PASSWORD, DB_NAME
#   2. DATABASE_URL（Makefile 通过 TEST_DATABASE_URL 传入）
#   3. 各项默认值
# ============================================================================

set -e

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m'

# 如果提供了 DATABASE_URL，从中解析各组件（与 Makefile 的 TEST_DATABASE_URL 对齐）
if [ -n "$DATABASE_URL" ]; then
    DB_USER=${DB_USER:-$(echo "$DATABASE_URL" | sed -E 's|.*://([^:]+):.*|\1|')}
    DB_PASSWORD=${DB_PASSWORD:-$(echo "$DATABASE_URL" | sed -E 's|.*://[^:]+:([^@]+)@.*|\1|')}
    DB_HOST=${DB_HOST:-$(echo "$DATABASE_URL" | sed -E 's|.*@([^:]+):.*|\1|')}
    DB_PORT=${DB_PORT:-$(echo "$DATABASE_URL" | sed -E 's|.*:([0-9]+)/.*|\1|')}
    DB_NAME=${DB_NAME:-$(echo "$DATABASE_URL" | sed -E 's|.*/([^?]+).*|\1|')}
fi

DB_HOST=${DB_HOST:-localhost}
DB_PORT=${DB_PORT:-5432}
DB_NAME=${DB_NAME:-sso_test}
DB_USER=${DB_USER:-sso}
DB_SSL_MODE=${DB_SSL_MODE:-disable}

if [ -z "$DB_PASSWORD" ]; then
    echo -e "${RED}✗ DB_PASSWORD 未设置${NC}"
    echo "  请通过以下方式之一提供："
    echo "    1. 设置 DATABASE_URL 环境变量（Makefile 自动传入）"
    echo "    2. 设置 DB_PASSWORD 环境变量"
    echo "    3. 通过 .env.test 文件提供（参考 .env.example）"
    exit 1
fi

export PGPASSWORD="${DB_PASSWORD}"

E2E_ADMIN_EMAIL=${E2E_ADMIN_EMAIL:-system@eninte.com}
E2E_ADMIN_PASSWORD=${E2E_ADMIN_PASSWORD:-Admin1234!}

PSQL_ARGS=(-h "${DB_HOST}" -p "${DB_PORT}" -U "${DB_USER}" -d "${DB_NAME}")

echo -e "${GREEN}=== E2E测试数据准备 ===${NC}"
echo ""

# 1. 检查数据库连接
echo -e "${YELLOW}[1/4] 检查数据库连接...${NC}"
if ! psql "${PSQL_ARGS[@]}" -c "SELECT 1" > /dev/null 2>&1; then
    echo -e "${RED}✗ 数据库连接失败${NC}"
    echo "  主机: ${DB_HOST}:${DB_PORT}  用户: ${DB_USER}  数据库: ${DB_NAME}"
    unset PGPASSWORD
    exit 1
fi
echo -e "${GREEN}✓ 数据库连接成功${NC}"
echo ""

# 2. 启用自动验证触发器
echo -e "${YELLOW}[2/4] 启用测试用户自动验证触发器...${NC}"
psql "${PSQL_ARGS[@]}" -f scripts/enable-auto-verify-test-users.sql > /dev/null 2>&1
echo -e "${GREEN}✓ 触发器已启用（所有 @example.com 用户将自动验证）${NC}"
echo ""

# 3. 预建管理员账户
# 管理员邮箱 system@eninte.com 不以 @example.com 结尾，触发器不会自动验证，
# 必须显式预建（email_verified=true, role='admin'），否则 admin_flow_test.go 全部失败。
# 密码用 bcrypt 哈希（cost=10，与测试环境 BCRYPT_COST=10 一致），SQL 用 ON CONFLICT 幂等。
echo -e "${YELLOW}[3/4] 预建管理员账户（${E2E_ADMIN_EMAIL}）...${NC}"
# go run 不支持从 stdin 读取，用项目内临时文件（$$ 保证唯一，trap 确保清理，不新增永久文件）
# 注意：Go 工具忽略以下划线/点开头的文件，文件名必须以字母开头
TMPGO="./genbcrypt_$$.go"
trap 'rm -f "$TMPGO"' EXIT
cat > "$TMPGO" <<'GOEOF'
package main

import (
	"fmt"
	"os"

	"golang.org/x/crypto/bcrypt"
)

func main() {
	pw := os.Getenv("BCRYPT_PASSWORD")
	if pw == "" {
		fmt.Fprintln(os.Stderr, "BCRYPT_PASSWORD 未设置")
		os.Exit(1)
	}
	h, err := bcrypt.GenerateFromPassword([]byte(pw), 10)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Print(string(h))
}
GOEOF
ADMIN_HASH=$(BCRYPT_PASSWORD="$E2E_ADMIN_PASSWORD" go run "$TMPGO" 2>/dev/null)
rm -f "$TMPGO"
trap - EXIT
if [ -z "$ADMIN_HASH" ]; then
    echo -e "${RED}✗ 生成管理员密码哈希失败${NC}"
    unset PGPASSWORD
    exit 1
fi

psql "${PSQL_ARGS[@]}" \
    -v email="$E2E_ADMIN_EMAIL" \
    -v hash="$ADMIN_HASH" \
    <<'SQL' > /dev/null 2>&1
INSERT INTO users (email, password_hash, email_verified, role, status, login_attempts, created_at, updated_at)
VALUES (:'email', :'hash', true, 'admin', 'active', 0, now(), now())
ON CONFLICT (email) DO UPDATE SET
    password_hash = EXCLUDED.password_hash,
    email_verified = true,
    role = 'admin',
    status = 'active',
    login_attempts = 0,
    locked_until = NULL,
    updated_at = now();
SQL
echo -e "${GREEN}✓ 管理员账户已就绪（email_verified=true, role=admin）${NC}"
echo ""

# 4. 显示统计信息
echo -e "${YELLOW}[4/4] 数据库统计信息${NC}"
TOTAL_USERS=$(psql "${PSQL_ARGS[@]}" -t -c "SELECT COUNT(*) FROM users;" | xargs)
VERIFIED_USERS=$(psql "${PSQL_ARGS[@]}" -t -c "SELECT COUNT(*) FROM users WHERE email_verified=true;" | xargs)
ADMIN_USERS=$(psql "${PSQL_ARGS[@]}" -t -c "SELECT COUNT(*) FROM users WHERE role='admin';" | xargs)

echo "  总用户数: ${TOTAL_USERS}"
echo "  已验证用户: ${VERIFIED_USERS}"
echo "  管理员用户: ${ADMIN_USERS}"
echo ""

unset PGPASSWORD

echo -e "${GREEN}=== 数据准备完成，可以运行E2E测试 ===${NC}"
echo ""
echo "运行测试命令："
echo "  make test-e2e"
echo ""
echo -e "${YELLOW}注意：测试完成后请运行 'make test-e2e-cleanup' 禁用触发器${NC}"
echo ""
