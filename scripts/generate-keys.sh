#!/bin/bash
# ============================================================================
# 生成RSA密钥对脚本
# 用于JWT签名和验证
# ============================================================================

set -e  # 遇到错误立即退出

KEYS_DIR="./keys"

echo "=========================================="
echo "  SSO服务 - RSA密钥对生成工具"
echo "=========================================="
echo ""

# 创建keys目录 (如果不存在)
mkdir -p $KEYS_DIR

# 检查是否已存在密钥
if [ -f "$KEYS_DIR/private.pem" ]; then
    read -p "密钥文件已存在，是否覆盖? (y/N): " confirm
    if [[ $confirm != [yY] ]]; then
        echo "已取消操作"
        exit 0
    fi
fi

echo "正在生成2048位RSA私钥..."
openssl genrsa -out $KEYS_DIR/private.pem 2048

echo "正在从私钥提取公钥..."
openssl rsa -in $KEYS_DIR/private.pem -pubout -out $KEYS_DIR/public.pem

# 设置适当的文件权限（仅所有者可读写）
chmod 600 $KEYS_DIR/private.pem
chmod 644 $KEYS_DIR/public.pem

echo ""
echo "=========================================="
echo "  密钥对生成成功!"
echo "=========================================="
echo ""
echo "  私钥: $KEYS_DIR/private.pem"
echo "  公钥: $KEYS_DIR/public.pem"
echo ""
echo "  请妥善保管私钥，切勿提交到版本控制系统!"
echo "  建议将 /keys/ 目录添加到 .gitignore"
echo ""
