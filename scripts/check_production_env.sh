#!/bin/bash

# 生产环境配置检查脚本
# 用于验证所有必需的环境变量是否正确配置

set -e

echo "🔍 检查生产环境配置..."
echo ""

# 颜色定义
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# 错误计数
ERRORS=0
WARNINGS=0

# 检查函数
check_required() {
    local var_name=$1
    local var_value=${!var_name}
    
    if [ -z "$var_value" ]; then
        echo -e "${RED}❌ $var_name 未设置${NC}"
        ((ERRORS++))
        return 1
    else
        echo -e "${GREEN}✅ $var_name 已设置${NC}"
        return 0
    fi
}

check_value() {
    local var_name=$1
    local expected=$2
    local var_value=${!var_name}
    
    if [ "$var_value" != "$expected" ]; then
        echo -e "${RED}❌ $var_name=$var_value (期望: $expected)${NC}"
        ((ERRORS++))
        return 1
    else
        echo -e "${GREEN}✅ $var_name=$var_value${NC}"
        return 0
    fi
}

check_min_value() {
    local var_name=$1
    local min_value=$2
    local var_value=${!var_name}
    
    if [ -z "$var_value" ]; then
        echo -e "${RED}❌ $var_name 未设置${NC}"
        ((ERRORS++))
        return 1
    fi
    
    if [ "$var_value" -lt "$min_value" ]; then
        echo -e "${RED}❌ $var_name=$var_value (最小值: $min_value)${NC}"
        ((ERRORS++))
        return 1
    else
        echo -e "${GREEN}✅ $var_name=$var_value${NC}"
        return 0
    fi
}

check_not_contains() {
    local var_name=$1
    local forbidden=$2
    local var_value=${!var_name}
    
    if [ -z "$var_value" ]; then
        echo -e "${RED}❌ $var_name 未设置${NC}"
        ((ERRORS++))
        return 1
    fi
    
    if [[ "$var_value" == *"$forbidden"* ]]; then
        echo -e "${RED}❌ $var_name 包含禁止的值: $forbidden${NC}"
        ((ERRORS++))
        return 1
    else
        echo -e "${GREEN}✅ $var_name 不包含 $forbidden${NC}"
        return 0
    fi
}

check_min_length() {
    local var_name=$1
    local min_length=$2
    local var_value=${!var_name}
    
    if [ -z "$var_value" ]; then
        echo -e "${RED}❌ $var_name 未设置${NC}"
        ((ERRORS++))
        return 1
    fi
    
    if [ ${#var_value} -lt $min_length ]; then
        echo -e "${RED}❌ $var_name 长度不足 (当前: ${#var_value}, 最小: $min_length)${NC}"
        ((ERRORS++))
        return 1
    else
        echo -e "${GREEN}✅ $var_name 长度: ${#var_value}${NC}"
        return 0
    fi
}

# 1. 检查环境标识
echo "📋 1. 环境标识"
check_value SERVER_ENV "production"
echo ""

# 2. 检查安全配置
echo "🔒 2. 安全配置"
check_required MFA_RECOVERY_HMAC_KEY
check_min_length MFA_RECOVERY_HMAC_KEY 32
check_min_value BCRYPT_COST 12
check_value DB_SSL_MODE "require"
echo ""

# 3. 检查CORS配置
echo "🌐 3. CORS配置"
check_required CORS_ALLOWED_ORIGINS
check_not_contains CORS_ALLOWED_ORIGINS "*"
check_not_contains CORS_ALLOWED_ORIGINS "localhost"
check_not_contains CORS_ALLOWED_ORIGINS "127.0.0.1"
echo ""

# 4. 检查数据库配置
echo "🗄️  4. 数据库配置"
check_required DATABASE_URL
check_required DB_HOST
check_required DB_PORT
check_required DB_USER
check_required DB_PASSWORD
check_required DB_NAME
echo ""

# 5. 检查JWT配置
echo "🔑 5. JWT配置"
check_required JWT_PRIVATE_KEY_PATH
check_required JWT_PUBLIC_KEY_PATH
check_required JWT_ISSUER

# 检查JWT密钥文件是否存在
if [ -n "$JWT_PRIVATE_KEY_PATH" ] && [ ! -f "$JWT_PRIVATE_KEY_PATH" ]; then
    echo -e "${RED}❌ JWT私钥文件不存在: $JWT_PRIVATE_KEY_PATH${NC}"
    ((ERRORS++))
else
    echo -e "${GREEN}✅ JWT私钥文件存在${NC}"
fi

if [ -n "$JWT_PUBLIC_KEY_PATH" ] && [ ! -f "$JWT_PUBLIC_KEY_PATH" ]; then
    echo -e "${RED}❌ JWT公钥文件不存在: $JWT_PUBLIC_KEY_PATH${NC}"
    ((ERRORS++))
else
    echo -e "${GREEN}✅ JWT公钥文件存在${NC}"
fi
echo ""

# 6. 检查SMTP配置
echo "📧 6. SMTP配置"
check_required SMTP_HOST
check_required SMTP_PORT
check_required SMTP_USERNAME
check_required SMTP_PASSWORD
check_required SMTP_FROM
echo ""

# 7. 检查限流配置（可选但推荐）
echo "⚡ 7. 限流配置（推荐）"
if [ -z "$RATE_LIMIT_REQUESTS" ]; then
    echo -e "${YELLOW}⚠️  RATE_LIMIT_REQUESTS 未设置（将使用默认值）${NC}"
    ((WARNINGS++))
else
    echo -e "${GREEN}✅ RATE_LIMIT_REQUESTS=$RATE_LIMIT_REQUESTS${NC}"
fi

if [ -z "$MAX_LOGIN_ATTEMPTS" ]; then
    echo -e "${YELLOW}⚠️  MAX_LOGIN_ATTEMPTS 未设置（将使用默认值）${NC}"
    ((WARNINGS++))
else
    echo -e "${GREEN}✅ MAX_LOGIN_ATTEMPTS=$MAX_LOGIN_ATTEMPTS${NC}"
fi
echo ""

# 8. 检查Redis配置（可选但推荐）
echo "💾 8. Redis配置（推荐）"
if [ "$REDIS_ENABLE" = "true" ]; then
    check_required REDIS_HOST
    check_required REDIS_PORT
    if [ -n "$REDIS_PASSWORD" ]; then
        echo -e "${GREEN}✅ REDIS_PASSWORD 已设置${NC}"
    else
        echo -e "${YELLOW}⚠️  REDIS_PASSWORD 未设置（如果Redis需要认证，请设置）${NC}"
        ((WARNINGS++))
    fi
else
    echo -e "${YELLOW}⚠️  Redis未启用（推荐启用以提高性能）${NC}"
    ((WARNINGS++))
fi
echo ""

# 9. 检查日志配置
echo "📝 9. 日志配置"
if [ -z "$LOG_LEVEL" ]; then
    echo -e "${YELLOW}⚠️  LOG_LEVEL 未设置（将使用默认值: info）${NC}"
    ((WARNINGS++))
else
    echo -e "${GREEN}✅ LOG_LEVEL=$LOG_LEVEL${NC}"
fi

if [ -z "$LOG_FORMAT" ]; then
    echo -e "${YELLOW}⚠️  LOG_FORMAT 未设置（将使用默认值: json）${NC}"
    ((WARNINGS++))
else
    echo -e "${GREEN}✅ LOG_FORMAT=$LOG_FORMAT${NC}"
fi
echo ""

# 总结
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo "📊 检查结果"
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"

if [ $ERRORS -eq 0 ] && [ $WARNINGS -eq 0 ]; then
    echo -e "${GREEN}✅ 所有检查通过！可以部署到生产环境。${NC}"
    exit 0
elif [ $ERRORS -eq 0 ]; then
    echo -e "${YELLOW}⚠️  发现 $WARNINGS 个警告，建议修复后再部署。${NC}"
    exit 0
else
    echo -e "${RED}❌ 发现 $ERRORS 个错误和 $WARNINGS 个警告，必须修复后才能部署！${NC}"
    exit 1
fi
