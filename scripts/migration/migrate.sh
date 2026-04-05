#!/bin/bash
# ============================================================================
# SSO服务数据库迁移脚本
# 使用golang-migrate执行数据库迁移
# ============================================================================

set -e

# 配置
MIGRATION_DIR="./migrations"
DATABASE_URL="${DATABASE_URL}"

# 检查DATABASE_URL
if [ -z "$DATABASE_URL" ]; then
    echo "错误: 未设置DATABASE_URL环境变量"
    echo ""
    echo "示例: export DATABASE_URL='postgres://sso:changeme@localhost:5432/sso?sslmode=disable'"
    exit 1
fi

# 检查migrate命令
if ! command -v migrate &> /dev/null; then
    echo "错误: 未找到migrate命令"
    echo ""
    echo "请安装golang-migrate:"
    echo "  go install -tags 'postgres' github.com/golang-migrate/migrate/v4/cmd/migrate@latest"
    exit 1
fi

# 获取操作类型
ACTION=${1:-up}

echo "=========================================="
echo "  SSO服务 - 数据库迁移工具"
echo "=========================================="
echo ""
echo "  操作: $ACTION"
echo "  迁移目录: $MIGRATION_DIR"
echo ""

case $ACTION in
    up)
        echo "执行迁移 (向上)..."
        migrate -path $MIGRATION_DIR -database $DATABASE_URL up
        ;;
    down)
        echo "执行迁移 (向下)..."
        migrate -path $MIGRATION_DIR -database $DATABASE_URL down
        ;;
    force)
        if [ -z "$2" ]; then
            echo "错误: 请提供版本号"
            echo "用法: $0 force <版本号>"
            exit 1
        fi
        echo "强制设置版本为: $2"
        migrate -path $MIGRATION_DIR -database $DATABASE_URL force $2
        ;;
    version)
        echo "当前数据库版本:"
        migrate -path $MIGRATION_DIR -database $DATABASE_URL version
        ;;
    *)
        echo "未知操作: $ACTION"
        echo ""
        echo "可用操作:"
        echo "  up      - 执行所有待执行的迁移"
        echo "  down    - 回滚最后一次迁移"
        echo "  force   - 强制设置版本号"
        echo "  version - 显示当前版本"
        exit 1
        ;;
esac

echo ""
echo "=========================================="
echo "  迁移完成!"
echo "=========================================="
