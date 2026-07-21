#!/bin/sh
set -e

# .env 文件路径
ENV_FILE="${ENV_FILE_PATH:-/app/.env}"

# 如果 .env 不存在，设置标记让服务启动配置向导
if [ ! -f "$ENV_FILE" ]; then
    echo "No .env file found, starting setup wizard on :9090..."
    export SERVER_ENV=setup
fi

# T17（安全修复，审计 L9）：数据库迁移的 DSN 不再拼接明文密码。
# 旧实现把含密码的 DATABASE_URL 放进 migrate 命令行参数（/proc/<pid>/cmdline 可读），
# 并 export 到容器进程环境（docker inspect 可见），存在凭据泄露面。
# 现改为：
#   - DSN 不拼接密码，仅含 user@host:port/db 与 sslmode
#   - 密码仅通过 migrate 子进程环境的 PGPASSWORD 传递（不 export，主服务不继承）
#     lib/pq 在无密码 DSN 下回退读取 PGPASSWORD（已实证验证）
#   - DSN 不再 export（服务端自身使用 DB_* 配置，不消费 DATABASE_URL）
if [ -z "$DATABASE_URL" ] && [ -n "$DB_HOST" ] && [ "$SERVER_ENV" != "setup" ]; then
    MIGRATE_DSN="postgres://${DB_USER}@${DB_HOST}:${DB_PORT}/${DB_NAME}?sslmode=${DB_SSL_MODE:-prefer}"
    MIGRATE_PASSWORD="${DB_PASSWORD}"
else
    # 用户显式提供 DATABASE_URL 时按原样使用（其中是否含密码由用户自行决定），
    # 不再补注 PGPASSWORD
    MIGRATE_DSN="$DATABASE_URL"
    MIGRATE_PASSWORD=""
fi

# 自动迁移（可通过 AUTO_MIGRATE=false 跳过）
if [ "${AUTO_MIGRATE:-true}" = "true" ] && [ -n "$MIGRATE_DSN" ]; then
    echo "Running database migrations..."
    if [ -n "$MIGRATE_PASSWORD" ]; then
        # PGPASSWORD 仅注入 migrate 子进程环境
        PGPASSWORD="$MIGRATE_PASSWORD" migrate -path /app/migrations -database "$MIGRATE_DSN" up || {
            echo "Migration failed!"
            exit 1
        }
    else
        migrate -path /app/migrations -database "$MIGRATE_DSN" up || {
            echo "Migration failed!"
            exit 1
        }
    fi
    echo "Migrations complete."
fi

# 启动主应用
exec /app/sso "$@"
