#!/bin/bash
# ============================================================================
# SSO服务数据库恢复脚本
# 从备份文件恢复PostgreSQL数据库
# ============================================================================

set -e

# 数据库连接参数
DB_HOST="${DB_HOST:-localhost}"
DB_PORT="${DB_PORT:-5432}"
DB_NAME="${DB_NAME:-sso}"
DB_USER="${DB_USER:-sso}"

# 检查参数
if [ -z "$1" ]; then
    echo "用法: $0 <备份文件路径>"
    echo ""
    echo "示例: $0 /backup/sso/sso_20240101_120000.sql.gz"
    exit 1
fi

BACKUP_FILE=$1

echo "=========================================="
echo "  SSO服务 - 数据库恢复工具"
echo "=========================================="
echo ""

# 检查备份文件是否存在
if [ ! -f "$BACKUP_FILE" ]; then
    echo "错误: 备份文件不存在: $BACKUP_FILE"
    exit 1
fi

echo "  数据库: $DB_NAME"
echo "  备份文件: $BACKUP_FILE"
echo ""

# 确认恢复操作
read -p "警告: 此操作将覆盖现有数据，是否继续? (y/N): " confirm
if [[ $confirm != [yY] ]]; then
    echo "已取消操作"
    exit 0
fi

echo ""
echo "开始恢复数据库..."

# 恢复数据库
if [[ $BACKUP_FILE == *.gz ]]; then
    # 压缩文件
    gunzip -c $BACKUP_FILE | psql -h $DB_HOST -p $DB_PORT -U $DB_USER -d $DB_NAME
else
    # 未压缩文件
    psql -h $DB_HOST -p $DB_PORT -U $DB_USER -d $DB_NAME < $BACKUP_FILE
fi

echo ""
echo "=========================================="
echo "  恢复完成!"
echo "=========================================="
echo ""
echo "建议执行以下操作:"
echo "  1. 验证数据完整性"
echo "  2. 重启SSO服务: docker-compose restart sso"
echo "  3. 检查服务健康状态: curl http://localhost:9090/health"
