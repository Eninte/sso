#!/usr/bin/env bash
# run_e2e_no_ratelimit.sh — 以"等效无限流"方式启动 SSO 服务，供 E2E 测试使用。
#
# 背景：login 端点有两层独立限流，register 端点有一层，均需处理：
#
#   1. HTTP 中间件限流（router.go:124 sensitiveLimiter）
#      - 覆盖 register/login/forgot-password/reset-password
#      - 限额 = RATE_LIMIT_REQUESTS / 10，窗口 1 分钟
#      - limit<=0 时回退到默认 10（router.go:206-208），故 RATE_LIMIT_REQUESTS=0 无效
#      - 进程内内存实现，随服务进程生命周期累积
#      → 方案：用大 RATE_LIMIT_REQUESTS（默认 1000000，敏感端点=100000/分钟）+ 重启清零
#
#   2. 业务层登录限流（service/login_ratelimit.go LoginRateLimiter）
#      - 仅覆盖 login，基于 IP（key=login:ratelimit:<IP>）
#      - 硬编码上限 20 次/10 分钟（LoginRateLimitMax，不可配置）
#      - 存 Redis，跨服务重启持久化
#      → 方案：后台守护进程定期清 Redis 的 login:ratelimit:* key
#
# 用法：
#   ./scripts/run_e2e_no_ratelimit.sh            # 默认参数启动
#   ./scripts/run_e2e_no_ratelimit.sh -p 9091    # 指定端口
#   PORT=9090 ./scripts/run_e2e_no_ratelimit.sh  # 或用环境变量
#
# 启动后执行：
#   make test-e2e-prepare && make test-e2e

set -euo pipefail

# ==== 默认参数（可被环境变量或命令行参数覆盖）====
PORT="${PORT:-9090}"
RATE_LIMIT_REQUESTS="${RATE_LIMIT_REQUESTS:-1000000}"
LOG_FILE="${LOG_FILE:-/tmp/sso-e2e.log}"
HEALTH_TIMEOUT="${HEALTH_TIMEOUT:-30}"  # 健康检查最大等待秒数
LOGIN_RL_CLEAN_INTERVAL="${LOGIN_RL_CLEAN_INTERVAL:-0.5}"  # 业务层登录限流清理间隔（秒）

# Redis 配置：优先环境变量，其次从 .env.test 读取，最后默认 localhost:6379
REDIS_HOST="${REDIS_HOST:-}"
REDIS_PORT="${REDIS_PORT:-}"
REDIS_PASSWORD="${REDIS_PASSWORD:-}"
REDIS_DB="${REDIS_DB:-0}"
ENV_TEST_FILE="${ENV_TEST_FILE:-.env.test}"

# 从 .env.test 读取 Redis 配置（若环境变量未显式设置）
load_redis_config_from_envtest() {
  if [ -f "$ENV_TEST_FILE" ]; then
    [ -z "$REDIS_HOST" ]     && REDIS_HOST=$(grep -E '^REDIS_HOST=' "$ENV_TEST_FILE" | cut -d= -f2 | tr -d '[:space:]' || true)
    [ -z "$REDIS_PORT" ]     && REDIS_PORT=$(grep -E '^REDIS_PORT=' "$ENV_TEST_FILE" | cut -d= -f2 | tr -d '[:space:]' || true)
    [ -z "$REDIS_PASSWORD" ] && REDIS_PASSWORD=$(grep -E '^REDIS_PASSWORD=' "$ENV_TEST_FILE" | cut -d= -f2 | tr -d '[:space:]' || true)
    [ -z "$REDIS_DB" ]       && REDIS_DB=$(grep -E '^REDIS_DB=' "$ENV_TEST_FILE" | cut -d= -f2 | tr -d '[:space:]' || true)
  fi
  REDIS_HOST="${REDIS_HOST:-localhost}"
  REDIS_PORT="${REDIS_PORT:-6379}"
  REDIS_DB="${REDIS_DB:-0}"
}

# ==== 解析命令行参数 ====
while getopts ":p:r:l:h" opt; do
  case "$opt" in
    p) PORT="$OPTARG" ;;
    r) RATE_LIMIT_REQUESTS="$OPTARG" ;;
    l) LOG_FILE="$OPTARG" ;;
    h)
      sed -n '2,30p' "$0"
      exit 0
      ;;
    \?) echo "未知参数: -$OPTARG" >&2; exit 1 ;;
    :)  echo "选项 -$OPTARG 需要参数" >&2; exit 1 ;;
  esac
done

load_redis_config_from_envtest

# 构建 redis-cli 公共参数
REDIS_CLI_ARGS=(-h "$REDIS_HOST" -p "$REDIS_PORT" -n "$REDIS_DB")
if [ -n "$REDIS_PASSWORD" ]; then
  REDIS_CLI_ARGS+=(-a "$REDIS_PASSWORD" --no-auth-warning)
fi

# ==== 子进程 PID 跟踪（用于 trap 清理）====
SERVER_PID=""
CLEANUP_PID=""

cleanup_all() {
  echo
  echo "==> 停止后台进程"
  [ -n "$CLEANUP_PID" ] && kill "$CLEANUP_PID" 2>/dev/null || true
  [ -n "$SERVER_PID" ]  && kill "$SERVER_PID"  2>/dev/null || true
  exit 130
}
trap cleanup_all INT TERM

# ==== 1. 杀掉占用 PORT 的旧进程（清零 HTTP 中间件内存限流计数器）====
echo "==> [1/4] 释放端口 $PORT（清零 HTTP 中间件内存限流计数器）"
if command -v fuser >/dev/null 2>&1; then
  fuser -k "${PORT}/tcp" 2>/dev/null || true
elif command -v lsof >/dev/null 2>&1; then
  PID=$(lsof -ti :"${PORT}" 2>/dev/null || true)
  [ -n "$PID" ] && kill -9 $PID 2>/dev/null || true
else
  echo "WARN: 既无 fuser 也无 lsof，无法自动释放端口，请手动确保 $PORT 空闲" >&2
fi
sleep 2

# ==== 2. 清理 Redis 中的业务层登录限流计数器 ====
echo "==> [2/4] 清理 Redis 业务层登录限流（login:ratelimit:*）"
if command -v redis-cli >/dev/null 2>&1; then
  # 测试 Redis 连通性
  if redis-cli "${REDIS_CLI_ARGS[@]}" PING >/dev/null 2>&1; then
    DELETED=$(redis-cli "${REDIS_CLI_ARGS[@]}" --scan --pattern 'login:ratelimit:*' 2>/dev/null | wc -l || echo 0)
    redis-cli "${REDIS_CLI_ARGS[@]}" --scan --pattern 'login:ratelimit:*' 2>/dev/null | \
      xargs -r redis-cli "${REDIS_CLI_ARGS[@]}" DEL >/dev/null 2>&1 || true
    echo "         Redis=${REDIS_HOST}:${REDIS_PORT} DB=${REDIS_DB}，已清理 ${DELETED} 个登录限流 key"
  else
    echo "WARN: Redis ${REDIS_HOST}:${REDIS_PORT} 不可达，跳过登录限流清理（业务层限流将持续生效）" >&2
  fi
else
  echo "WARN: 未安装 redis-cli，跳过登录限流清理（业务层限流将持续生效）" >&2
fi

# ==== 3. 启动服务（后台，大限流值等效禁用 HTTP 中间件限流）====
echo "==> [3/4] 启动 SSO 服务（RATE_LIMIT_REQUESTS=${RATE_LIMIT_REQUESTS}）"
echo "         HTTP 中间件限流 = $((RATE_LIMIT_REQUESTS / 10))/分钟（等效无限）"
echo "         日志: ${LOG_FILE}"

RATE_LIMIT_REQUESTS="${RATE_LIMIT_REQUESTS}" \
CAPTCHA_ENABLED=false \
  go run cmd/server/main.go > "${LOG_FILE}" 2>&1 &
SERVER_PID=$!
echo "         服务 PID: ${SERVER_PID}"

# ==== 4. 启动业务层登录限流守护清理（测试期间持续清 Redis login:ratelimit:*）====
# 业务层限流硬编码 20 次/10 分钟，134 个 E2E 测试必然超限，必须持续清理。
echo "==> [4/4] 启动业务层登录限流守护清理（每 ${LOGIN_RL_CLEAN_INTERVAL}s 清一次）"
(
  while true; do
    if command -v redis-cli >/dev/null 2>&1; then
      redis-cli "${REDIS_CLI_ARGS[@]}" --scan --pattern 'login:ratelimit:*' 2>/dev/null | \
        xargs -r redis-cli "${REDIS_CLI_ARGS[@]}" DEL >/dev/null 2>&1 || true
    fi
    sleep "$LOGIN_RL_CLEAN_INTERVAL"
  done
) &
CLEANUP_PID=$!
echo "         守护清理 PID: ${CLEANUP_PID}"

# ==== 等待健康检查通过 ====
echo "==> 等待健康检查（最长 ${HEALTH_TIMEOUT}s）"
for i in $(seq 1 "${HEALTH_TIMEOUT}"); do
  if curl -sf "http://localhost:${PORT}/health" >/dev/null 2>&1; then
    echo
    echo "✓ 服务就绪: http://localhost:${PORT}"
    echo "✓ HTTP 中间件限流已等效禁用（敏感端点 $((RATE_LIMIT_REQUESTS / 10))/分钟）"
    echo "✓ 业务层登录限流守护清理已启动（每 ${LOGIN_RL_CLEAN_INTERVAL}s 清 Redis login:ratelimit:*）"
    echo
    echo "下一步："
    echo "  make test-e2e-prepare && make test-e2e"
    echo
    echo "停止服务与守护: Ctrl-C 本脚本  （或 kill ${SERVER_PID} ${CLEANUP_PID}）"
    # 保持脚本前台运行，服务在后台；用户 Ctrl-C 时 trap 会杀服务+守护
    wait "${SERVER_PID}"
    exit 0
  fi

  # 检查服务进程是否已退出（启动失败）
  if ! kill -0 "${SERVER_PID}" 2>/dev/null; then
    echo "✗ 服务进程已退出，启动失败。日志尾部：" >&2
    tail -n 20 "${LOG_FILE}" >&2 || true
    cleanup_all
  fi

  printf '.'
  sleep 1
done

echo
echo "✗ 服务 ${HEALTH_TIMEOUT}s 内未就绪，查看日志: ${LOG_FILE}" >&2
tail -n 20 "${LOG_FILE}" >&2 || true
cleanup_all
