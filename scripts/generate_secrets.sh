#!/bin/bash

# 生成生产环境所需的密钥和秘密
# 用于初始化生产环境配置

set -e

echo "🔐 生成生产环境密钥..."
echo ""

# 颜色定义
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# 1. 生成MFA恢复码HMAC密钥（32字节）
echo "1️⃣  生成MFA恢复码HMAC密钥（32字节）"
MFA_KEY=$(openssl rand -base64 32)
echo -e "${GREEN}MFA_RECOVERY_HMAC_KEY=$MFA_KEY${NC}"
echo ""

# 2. 生成JWT密钥对（RSA 2048位）
echo "2️⃣  生成JWT密钥对（RSA 2048位）"
JWT_DIR="./keys/jwt"
mkdir -p $JWT_DIR

# 生成私钥
openssl genrsa -out $JWT_DIR/private.pem 2048 2>/dev/null
echo -e "${GREEN}✅ JWT私钥已生成: $JWT_DIR/private.pem${NC}"

# 生成公钥
openssl rsa -in $JWT_DIR/private.pem -pubout -out $JWT_DIR/public.pem 2>/dev/null
echo -e "${GREEN}✅ JWT公钥已生成: $JWT_DIR/public.pem${NC}"

echo -e "${GREEN}JWT_PRIVATE_KEY_PATH=$JWT_DIR/private.pem${NC}"
echo -e "${GREEN}JWT_PUBLIC_KEY_PATH=$JWT_DIR/public.pem${NC}"
echo ""

# 3. 生成数据库密码（32字符）
echo "3️⃣  生成数据库密码（32字符）"
DB_PASSWORD=$(openssl rand -base64 24 | tr -d "=+/" | cut -c1-32)
echo -e "${GREEN}DB_PASSWORD=$DB_PASSWORD${NC}"
echo ""

# 4. 生成Redis密码（32字符）
echo "4️⃣  生成Redis密码（32字符）"
REDIS_PASSWORD=$(openssl rand -base64 24 | tr -d "=+/" | cut -c1-32)
echo -e "${GREEN}REDIS_PASSWORD=$REDIS_PASSWORD${NC}"
echo ""

# 5. 生成会话密钥（64字节）
echo "5️⃣  生成会话密钥（64字节）"
SESSION_KEY=$(openssl rand -base64 64 | tr -d "\n")
echo -e "${GREEN}SESSION_SECRET=$SESSION_KEY${NC}"
echo ""

# 6. 生成.env.production模板
echo "6️⃣  生成.env.production模板"
cat > .env.production.template << EOF
# 生产环境配置模板
# 生成时间: $(date)
# ⚠️  请勿将此文件提交到版本控制系统

# ============================================================================
# 环境标识
# ============================================================================
SERVER_ENV=production
SERVER_PORT=8080

# ============================================================================
# 安全配置（必须）
# ============================================================================
MFA_RECOVERY_HMAC_KEY=$MFA_KEY
BCRYPT_COST=14
DB_SSL_MODE=require

# ============================================================================
# CORS配置（必须）
# ============================================================================
# 替换为你的生产域名
CORS_ALLOWED_ORIGINS=https://your-domain.com,https://app.your-domain.com

# ============================================================================
# 数据库配置（必须）
# ============================================================================
DB_HOST=your-db-host
DB_PORT=5432
DB_USER=sso
DB_PASSWORD=$DB_PASSWORD
DB_NAME=sso
DB_SSL_MODE=require
DATABASE_URL=postgres://sso:$DB_PASSWORD@your-db-host:5432/sso?sslmode=require

# ============================================================================
# JWT配置（必须）
# ============================================================================
JWT_PRIVATE_KEY_PATH=$JWT_DIR/private.pem
JWT_PUBLIC_KEY_PATH=$JWT_DIR/public.pem
JWT_ISSUER=https://your-domain.com
ACCESS_TOKEN_TTL=15m
REFRESH_TOKEN_TTL=168h

# ============================================================================
# SMTP配置（必须）
# ============================================================================
SMTP_HOST=smtp.your-provider.com
SMTP_PORT=587
SMTP_USERNAME=your-smtp-username
SMTP_PASSWORD=your-smtp-password
SMTP_FROM=noreply@your-domain.com
SMTP_FROM_NAME=Your App Name

# ============================================================================
# Redis配置（推荐）
# ============================================================================
REDIS_ENABLE=true
REDIS_HOST=your-redis-host
REDIS_PORT=6379
REDIS_PASSWORD=$REDIS_PASSWORD
REDIS_DB=0

# ============================================================================
# 限流配置（推荐）
# ============================================================================
RATE_LIMIT_REQUESTS=100
RATE_LIMIT_WINDOW=60s
MAX_LOGIN_ATTEMPTS=5
ACCOUNT_LOCKOUT_DURATION=30m

# ============================================================================
# 日志配置
# ============================================================================
LOG_LEVEL=info
LOG_FORMAT=json

# ============================================================================
# 会话配置
# ============================================================================
SESSION_SECRET=$SESSION_KEY

# ============================================================================
# 邮件限流配置
# ============================================================================
EMAIL_RATE_LIMIT=5
EMAIL_RATE_LIMIT_WINDOW=1h
EOF

echo -e "${GREEN}✅ .env.production.template 已生成${NC}"
echo ""

# 7. 设置文件权限
echo "7️⃣  设置文件权限"
chmod 600 $JWT_DIR/private.pem
chmod 644 $JWT_DIR/public.pem
chmod 600 .env.production.template
echo -e "${GREEN}✅ 文件权限已设置${NC}"
echo ""

# 总结
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo "✅ 密钥生成完成！"
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo ""
echo "📝 下一步操作："
echo "1. 复制 .env.production.template 为 .env.production"
echo "   cp .env.production.template .env.production"
echo ""
echo "2. 编辑 .env.production，替换以下占位符："
echo "   - your-domain.com"
echo "   - your-db-host"
echo "   - your-redis-host"
echo "   - your-smtp-*"
echo ""
echo "3. 验证配置："
echo "   ./scripts/check_production_env.sh"
echo ""
echo "4. 部署到生产环境"
echo ""
echo -e "${YELLOW}⚠️  重要提示：${NC}"
echo "   - 请妥善保管生成的密钥文件"
echo "   - 不要将 .env.production 提交到版本控制系统"
echo "   - 定期轮换密钥（建议每季度一次）"
echo "   - 使用密钥管理服务（如AWS KMS、HashiCorp Vault）存储密钥"
echo ""
