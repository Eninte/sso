#!/bin/sh
set -e

# .env 文件路径
ENV_FILE="${ENV_FILE_PATH:-/app/.env}"

# 如果 .env 不存在，设置标记让服务启动配置向导
if [ ! -f "$ENV_FILE" ]; then
    echo "No .env file found, starting setup wizard on :9090..."
    export SERVER_ENV=setup
fi

# URL编码密码中的特殊字符（@ : / ? # 等）
# 使用 awk 实现 POSIX 兼容，不依赖 bash 的 C 风格 for 循环与 ${str:$i:1} 子字符串切片
urlencode() {
    printf '%s' "$1" | awk '
    BEGIN {
        for (i = 0; i <= 255; i++) ord[sprintf("%c", i)] = i
    }
    {
        for (i = 1; i <= length($0); i++) {
            c = substr($0, i, 1)
            if (c ~ /[a-zA-Z0-9.~_-]/) {
                printf "%s", c
            } else {
                printf "%%%02X", ord[c]
            }
        }
    }
    '
}

# 如果未设置 DATABASE_URL 且有 DB_HOST，自动构建
if [ -z "$DATABASE_URL" ] && [ -n "$DB_HOST" ] && [ "$SERVER_ENV" != "setup" ]; then
    ENCODED_PASSWORD="$(urlencode "${DB_PASSWORD}")"
    export DATABASE_URL="postgres://${DB_USER}:${ENCODED_PASSWORD}@${DB_HOST}:${DB_PORT}/${DB_NAME}?sslmode=${DB_SSL_MODE:-prefer}"
fi

# 自动迁移（可通过 AUTO_MIGRATE=false 跳过）
if [ "${AUTO_MIGRATE:-true}" = "true" ] && [ -n "$DATABASE_URL" ]; then
    echo "Running database migrations..."
    migrate -path /app/migrations -database "$DATABASE_URL" up || {
        echo "Migration failed!"
        exit 1
    }
    echo "Migrations complete."
fi

# 启动主应用
exec /app/sso "$@"
