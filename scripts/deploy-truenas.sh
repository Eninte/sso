#!/bin/bash
set -e

# ============================================================================
# SSO TrueNAS 部署脚本
# 流程: 本地构建 → docker save | gzip → scp → docker load → docker compose up
# 支持部署失败自动回滚
# ============================================================================

TRUENAS_HOST="${TRUENAS_HOST:-192.168.1.3}"
TRUENAS_USER="${TRUENAS_USER:-root}"
IMAGE_NAME="${IMAGE_NAME:-sso}"
IMAGE_TAG="${IMAGE_TAG:-$(git describe --tags --always --dirty 2>/dev/null || echo latest)}"
REMOTE_PATH="${REMOTE_PATH:-/mnt/pool/sso}"
HEALTH_CHECK_RETRIES="${HEALTH_CHECK_RETRIES:-10}"
HEALTH_CHECK_INTERVAL="${HEALTH_CHECK_INTERVAL:-5}"

echo "========================================="
echo "  SSO Deploy to TrueNAS"
echo "========================================="
echo "  Host: $TRUENAS_HOST"
echo "  Image: $IMAGE_NAME:$IMAGE_TAG"
echo ""

# Step 1: 本地构建镜像
echo "[1/5] Building Docker image..."
docker build -f docker/Dockerfile -t "$IMAGE_NAME:$IMAGE_TAG" -t "$IMAGE_NAME:latest" .

# Step 2: 导出镜像
echo "[2/5] Saving image to tarball..."
docker save "$IMAGE_NAME:$IMAGE_TAG" | gzip > /tmp/sso-image.tar.gz
echo "  Size: $(du -h /tmp/sso-image.tar.gz | cut -f1)"

# Step 3: 传输到 TrueNAS 并加载
echo "[3/5] Transferring to TrueNAS ($TRUENAS_HOST)..."
scp /tmp/sso-image.tar.gz "$TRUENAS_USER@$TRUENAS_HOST:/tmp/sso-image.tar.gz"
echo "  Loading image on TrueNAS..."
ssh "$TRUENAS_USER@$TRUENAS_HOST" "docker load < /tmp/sso-image.tar.gz && rm /tmp/sso-image.tar.gz"

# Step 4: 记录当前版本用于回滚，然后重启服务
echo "[4/5] Restarting service..."
PREV_IMAGE=$(ssh "$TRUENAS_USER@$TRUENAS_HOST" "cd $REMOTE_PATH && docker compose ps -q sso | xargs -r docker inspect --format='{{.Config.Image}}'" || echo "")
ssh "$TRUENAS_USER@$TRUENAS_HOST" "cd $REMOTE_PATH && docker compose up -d --no-deps sso"

# Step 5: 健康检查
echo "[5/5] Health checking..."
DEPLOY_FAILED=0
for i in $(seq 1 "$HEALTH_CHECK_RETRIES"); do
    if ssh "$TRUENAS_USER@$TRUENAS_HOST" "curl -sf http://localhost:9090/health > /dev/null 2>&1"; then
        echo "  ✓ Service is healthy (attempt $i/$HEALTH_CHECK_RETRIES)"
        DEPLOY_FAILED=0
        break
    fi
    echo "  Waiting for service... ($i/$HEALTH_CHECK_RETRIES)"
    DEPLOY_FAILED=1
    sleep "$HEALTH_CHECK_INTERVAL"
done

if [ "$DEPLOY_FAILED" -eq 1 ]; then
    echo ""
    echo "  ✗ Health check failed after $HEALTH_CHECK_RETRIES attempts!"
    if [ -n "$PREV_IMAGE" ] && [ "$PREV_IMAGE" != "" ] && [ "$PREV_IMAGE" != "$IMAGE_NAME:$IMAGE_TAG" ]; then
        echo "  Rolling back to previous image: $PREV_IMAGE"
        ssh "$TRUENAS_USER@$TRUENAS_HOST" "cd $REMOTE_PATH && IMAGE_NAME=${PREV_IMAGE%%:*} IMAGE_TAG=${PREV_IMAGE##*:} docker compose up -d --no-deps sso" || true
        echo "  Rollback completed. Please investigate the failure."
    fi
    rm -f /tmp/sso-image.tar.gz
    exit 1
fi

# 清理本地临时文件
rm -f /tmp/sso-image.tar.gz

echo ""
echo "========================================="
echo "  Deployment complete!"
echo "  Version: $IMAGE_TAG"
echo "  Health:  curl http://$TRUENAS_HOST:9090/health"
echo "========================================="
