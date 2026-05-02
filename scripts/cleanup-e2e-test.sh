#!/bin/bash
# ============================================================================
# E2E测试数据清理脚本
# 用途：禁用自动验证触发器，并可选清理测试数据
#
# 配置来源（优先级从高到低）：
#   1. 单独的环境变量：DB_HOST, DB_PORT, DB_USER, DB_PASSWORD, DB_NAME
#   2. DATABASE_URL（Makefile 通过 TEST_DATABASE_URL 传入）
#   3. 各项默认值
#
# 用法:
#   bash scripts/cleanup-e2e-test.sh              # 交互模式
#   bash scripts/cleanup-e2e-test.sh --force      # 非交互模式（仅禁用触发器）
#   bash scripts/cleanup-e2e-test.sh --cleanup    # 非交互模式（禁用触发器+清理数据）
# ============================================================================

set -e

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m'

FORCE_MODE=false
CLEANUP_DATA=false

for arg in "$@"; do
    case $arg in
        --force|--non-interactive) FORCE_MODE=true ;;
        --cleanup|--cleanup-data)  CLEANUP_DATA=true ; FORCE_MODE=true ;;
        --help) echo "用法: $0 [--force] [--cleanup]"; exit 0 ;;
    esac
done

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

PSQL_ARGS=(-h "${DB_HOST}" -p "${DB_PORT}" -U "${DB_USER}" -d "${DB_NAME}")

echo -e "${GREEN}=== E2E测试清理 ===${NC}"
echo ""

echo -e "${YELLOW}[1/2] 检查数据库连接...${NC}"
if ! psql "${PSQL_ARGS[@]}" -c "SELECT 1" > /dev/null 2>&1; then
    echo -e "${RED}✗ 数据库连接失败${NC}"
    unset PGPASSWORD
    exit 1
fi
echo -e "${GREEN}✓ 数据库连接成功${NC}"
echo ""

echo -e "${YELLOW}[2/2] 禁用测试用户自动验证触发器...${NC}"
psql "${PSQL_ARGS[@]}" -f scripts/disable-auto-verify-test-users.sql > /dev/null 2>&1
echo -e "${GREEN}✓ 触发器已禁用${NC}"
echo ""

do_cleanup=$CLEANUP_DATA
if ! $FORCE_MODE; then
    echo -e "${YELLOW}是否清理测试数据？${NC}"
    echo "  这将删除所有 @example.com 域名的测试用户"
    read -p "确认清理？(y/N) " -n 1 -r
    echo
    if [[ $REPLY =~ ^[Yy]$ ]]; then
        do_cleanup=true
    fi
fi

if $do_cleanup; then
    echo ""
    echo -e "${YELLOW}清理测试数据...${NC}"

    DELETED_USERS=$(psql "${PSQL_ARGS[@]}" -t -c "DELETE FROM users WHERE email LIKE '%@example.com' RETURNING id;" | wc -l | xargs)
    echo -e "${GREEN}✓ 已删除 ${DELETED_USERS} 个测试用户${NC}"

    psql "${PSQL_ARGS[@]}" -c "DELETE FROM verification_tokens WHERE expires_at < NOW();" > /dev/null
    psql "${PSQL_ARGS[@]}" -c "DELETE FROM reset_tokens WHERE expires_at < NOW();" > /dev/null
    psql "${PSQL_ARGS[@]}" -c "DELETE FROM tokens WHERE expires_at < NOW();" > /dev/null
    echo -e "${GREEN}✓ 已清理过期令牌${NC}"
    echo ""
fi

unset PGPASSWORD

echo -e "${GREEN}=== 清理完成 ===${NC}"
