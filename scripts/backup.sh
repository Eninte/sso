#!/bin/bash
# ============================================================================
# SSO服务数据库备份脚本
# 用于定期备份PostgreSQL数据库
# ============================================================================

set -e

# 配置
BACKUP_DIR="${BACKUP_DIR:-/backup/sso}"
RETENTION_DAYS="${RETENTION_DAYS:-30}"
TIMESTAMP=$(date +%Y%m%d_%H%M%S)

# 数据库连接参数
DB_HOST="${DB_HOST:-localhost}"
DB_PORT="${DB_PORT:-5432}"
DB_NAME="${DB_NAME:-sso}"
DB_USER="${DB_USER:-sso}"

echo "=========================================="
echo "  SSO服务 - 数据库备份工具"
echo "=========================================="
echo ""

# 创建备份目录
mkdir -p $BACKUP_DIR

# 备份文件名
BACKUP_FILE="${BACKUP_DIR}/sso_${TIMESTAMP}.sql.gz"

echo "开始备份数据库..."
echo "  数据库: $DB_NAME"
echo "  目标: $BACKUP_FILE"
echo ""

# 执行备份
pg_dump -h $DB_HOST -p $DB_PORT -U $DB_USER -d $DB_NAME \
    --format=plain \
    --no-owner \
    --no-privileges \
    | gzip > $BACKUP_FILE

# 验证备份文件
if [ -f "$BACKUP_FILE" ] && [ -s "$BACKUP_FILE" ]; then
    BACKUP_SIZE=$(du -h $BACKUP_FILE | cut -f1)
    echo "备份成功!"
    echo "  文件大小: $BACKUP_SIZE"
else
    echo "错误: 备份文件创建失败!"
    exit 1
fi

# 清理过期备份
echo ""
echo "清理 ${RETENTION_DAYS} 天前的备份..."
find $BACKUP_DIR -name "sso_*.sql.gz" -mtime +$RETENTION_DAYS -delete

echo ""
echo "=========================================="
echo "  备份完成!"
echo "=========================================="
