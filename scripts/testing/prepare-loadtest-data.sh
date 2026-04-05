#!/bin/bash
# ============================================================================
# SSO 压测数据准备脚本
# ============================================================================
# 用途: 在压测前生成测试数据并写入 loadtest/data/ 目录
# 用法: ./scripts/prepare-loadtest-data.sh
# 环境变量:
#   BASE_URL              - SSO 服务地址 (默认 http://localhost:9090)
#   USER_POOL_SIZE        - 普通用户池规模 (默认 1000)
#   TOKEN_POOL_SIZE       - Access/Refresh Token 池规模 (默认 2000)
#   ADMIN_TOKEN_POOL_SIZE - 管理员 Token 池规模 (默认 50)
#   MALICIOUS_POOL_SIZE   - 恶意账号池规模 (默认 100)
#   ADMIN_EMAIL           - 管理员邮箱 (必需，从 E2E_ADMIN_EMAIL 读取)
#   ADMIN_PASSWORD        - 管理员密码 (必需，从 E2E_ADMIN_PASSWORD 读取)
#   TEST_PASSWORD         - 测试密码 (默认 TestPassword123!)
# ============================================================================

set -euo pipefail

BASE_URL="${BASE_URL:-http://localhost:9090}"
USER_POOL_SIZE="${USER_POOL_SIZE:-1000}"
TOKEN_POOL_SIZE="${TOKEN_POOL_SIZE:-2000}"
ADMIN_TOKEN_POOL_SIZE="${ADMIN_TOKEN_POOL_SIZE:-50}"
MALICIOUS_POOL_SIZE="${MALICIOUS_POOL_SIZE:-100}"
ADMIN_EMAIL="${E2E_ADMIN_EMAIL:-}"
ADMIN_PASSWORD="${E2E_ADMIN_PASSWORD:-}"
TEST_PASSWORD="${TEST_PASSWORD:-}"

DATA_DIR="loadtest/data"
TIMESTAMP=$(date +%s)

mkdir -p "$DATA_DIR"

# 验证必填环境变量
if [ -z "$ADMIN_EMAIL" ] || [ -z "$ADMIN_PASSWORD" ]; then
  echo "错误: 必须设置 E2E_ADMIN_EMAIL 和 E2E_ADMIN_PASSWORD 环境变量"
  echo "示例: E2E_ADMIN_EMAIL=admin@example.com E2E_ADMIN_PASSWORD=xxx $0"
  exit 1
fi

if [ -z "$TEST_PASSWORD" ]; then
  echo "错误: 必须设置 TEST_PASSWORD 环境变量"
  exit 1
fi

echo "=== SSO 压测数据准备 ==="
echo "目标服务: $BASE_URL"
echo "数据目录: $DATA_DIR"
echo ""

# ============================================================================
# 幂等性检查 (#13)
# ============================================================================
if [ -f "$DATA_DIR/users.json" ] && [ -f "$DATA_DIR/access_tokens.json" ]; then
  USER_COUNT=$(grep -c '"email"' "$DATA_DIR/users.json" 2>/dev/null || echo "0")
  if [ "$USER_COUNT" -gt 0 ]; then
    echo "警告: 数据目录已存在有效数据 ($USER_COUNT 个用户)"
    read -rp "是否重新生成? [y/N] " confirm
    if [[ "$confirm" != [yY] ]]; then
      echo "使用现有数据，跳过数据生成"
      echo ""
      echo "文件清单:"
      ls -la "$DATA_DIR/"
      exit 0
    fi
    echo "清理现有数据..."
    rm -f "$DATA_DIR"/*.json
  fi
fi

# ============================================================================
# 工具函数
# ============================================================================

# http_post: 使用 curl 直接发送 POST 请求，避免 eval 命令注入 (#2)
# 输出: 响应体 + HTTP 状态码（最后一行）
http_post() {
  local path="$1"
  local body="$2"
  local token="${3:-}"

  local -a curl_args=(-s -w '\n%{http_code}' -H 'Content-Type: application/json' -d "$body")
  if [ -n "$token" ]; then
    curl_args+=(-H "Authorization: Bearer $token")
  fi
  curl_args+=("${BASE_URL}${path}")

  curl "${curl_args[@]}"
}

# http_get: 使用 curl 直接发送 GET 请求
http_get() {
  local path="$1"
  local token="${2:-}"

  local -a curl_args=(-s -w '\n%{http_code}')
  if [ -n "$token" ]; then
    curl_args+=(-H "Authorization: Bearer $token")
  fi
  curl_args+=("${BASE_URL}${path}")

  curl "${curl_args[@]}"
}

# extract_field: 从 JSON 响应中提取字符串字段值
# 优先使用 jq（如果可用），否则回退到 sed (#4)
extract_field() {
  local json="$1"
  local key="$2"
  if command -v jq &>/dev/null; then
    echo "$json" | jq -r ".${key} // empty" 2>/dev/null | head -1
  else
    echo "$json" | sed -n "s/.*\"${key}\"[[:space:]]*:[[:space:]]*\"\([^\"]*\)\".*/\1/p" | head -1
  fi
}

# ============================================================================
# 1. 检查服务健康
# ============================================================================
echo "[1/6] 检查服务健康..."
RESPONSE=$(http_get "/health")
HTTP_CODE=$(echo "$RESPONSE" | tail -1)

if [ "$HTTP_CODE" != "200" ]; then
  echo "错误: 服务不可用 (HTTP $HTTP_CODE)"
  exit 1
fi
echo "服务正常"

# ============================================================================
# 2. 准备管理员账号
# ============================================================================
echo ""
echo "[2/6] 准备管理员账号..."
ADMIN_TOKEN=""

RESPONSE=$(http_post "/api/v1/login" "{\"email\":\"${ADMIN_EMAIL}\",\"password\":\"${ADMIN_PASSWORD}\"}")
HTTP_CODE=$(echo "$RESPONSE" | tail -1)
BODY=$(echo "$RESPONSE" | sed '$d')

if [ "$HTTP_CODE" = "200" ]; then
  ADMIN_TOKEN=$(extract_field "$BODY" "access_token")
  echo "管理员登录成功"
else
  echo "管理员账号不存在，尝试注册..."
  RESPONSE=$(http_post "/api/v1/register" "{\"email\":\"${ADMIN_EMAIL}\",\"password\":\"${ADMIN_PASSWORD}\"}")
  HTTP_CODE=$(echo "$RESPONSE" | tail -1)

  if [ "$HTTP_CODE" = "201" ] || [ "$HTTP_CODE" = "409" ]; then
    echo "管理员账号已就绪"
    echo "请执行以下 SQL 后重新运行此脚本:"
    echo "  UPDATE users SET email_verified=true, role='admin' WHERE email='${ADMIN_EMAIL}';"
    echo "跳过管理员 Token 生成"
  else
    echo "管理员注册失败 (HTTP $HTTP_CODE)"
  fi
fi

# ============================================================================
# 3. 生成普通用户池
# ============================================================================
echo ""
echo "[3/6] 生成 $USER_POOL_SIZE 个普通用户..."

{
  echo "["
  FIRST=true
  for i in $(seq 1 "$USER_POOL_SIZE"); do
    EMAIL="loadtest-user-${i}-${TIMESTAMP}@example.com"

    RESPONSE=$(http_post "/api/v1/register" "{\"email\":\"${EMAIL}\",\"password\":\"${TEST_PASSWORD}\"}")
    HTTP_CODE=$(echo "$RESPONSE" | tail -1)
    BODY=$(echo "$RESPONSE" | sed '$d')

    if [ "$HTTP_CODE" = "201" ]; then
      USER_ID=$(extract_field "$BODY" "user_id")

      # 验证邮箱 (使用测试端点)
      if [ -n "$USER_ID" ]; then
        http_post "/api/v1/test/verify-email" "{\"token\":\"${USER_ID}\"}" > /dev/null 2>&1 || true
      fi

      if [ "$FIRST" = true ]; then
        FIRST=false
      else
        echo ","
      fi
      printf '{"id":"%s","email":"%s","password":"%s"}' "$USER_ID" "$EMAIL" "$TEST_PASSWORD"
    fi

    if [ $((i % 100)) -eq 0 ]; then
      echo "  已生成 $i/$USER_POOL_SIZE 个用户" >&2
    fi
  done
  echo ""
  echo "]"
} > "$DATA_DIR/users.json"

echo "已写入 $DATA_DIR/users.json"

# ============================================================================
# 4. 生成 Token 池
# ============================================================================
echo ""
echo "[4/6] 生成 $TOKEN_POOL_SIZE 个 Access/Refresh Token..."

AT_FILE="$DATA_DIR/access_tokens.json"
RT_FILE="$DATA_DIR/refresh_tokens.json"

# 使用临时数组收集token数据，最后一次性写入文件
AT_ITEMS=()
RT_ITEMS=()
COUNT=0

# 逐行读取 users.json，跳过 [ ] 和空行
while IFS= read -r line; do
  # 跳过 JSON 结构字符
  [[ "$line" =~ ^[\[\],[:space:]]*$ ]] && continue
  [[ -z "$line" ]] && continue

  EMAIL=$(echo "$line" | sed -n 's/.*"email"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p')
  PASSWORD=$(echo "$line" | sed -n 's/.*"password"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p')

  [ -z "$EMAIL" ] && continue

  RESPONSE=$(http_post "/api/v1/login" "{\"email\":\"${EMAIL}\",\"password\":\"${PASSWORD}\"}")
  HTTP_CODE=$(echo "$RESPONSE" | tail -1)
  BODY=$(echo "$RESPONSE" | sed '$d')

  if [ "$HTTP_CODE" = "200" ]; then
    ACCESS_TOKEN=$(extract_field "$BODY" "access_token")
    REFRESH_TOKEN=$(extract_field "$BODY" "refresh_token")

    if [ -n "$ACCESS_TOKEN" ]; then
      AT_ITEMS+=("$ACCESS_TOKEN")
      COUNT=$((COUNT + 1))
    fi

    if [ -n "$REFRESH_TOKEN" ]; then
      RT_ITEMS+=("$REFRESH_TOKEN")
    fi
  fi

  if [ "$COUNT" -ge "$TOKEN_POOL_SIZE" ]; then
    break
  fi

  if [ $((COUNT % 100)) -eq 0 ] && [ "$COUNT" -gt 0 ]; then
    echo "  已生成 $COUNT/$TOKEN_POOL_SIZE 个 Token"
  fi
done < "$DATA_DIR/users.json"

# 一次性写入 access_tokens.json
{
  echo "["
  for i in "${!AT_ITEMS[@]}"; do
    if [ "$i" -gt 0 ]; then
      printf ','
    fi
    printf '{"access_token":"%s"}' "${AT_ITEMS[$i]}"
  done
  echo ""
  echo "]"
} > "$AT_FILE"

# 一次性写入 refresh_tokens.json
{
  echo "["
  for i in "${!RT_ITEMS[@]}"; do
    if [ "$i" -gt 0 ]; then
      printf ','
    fi
    printf '{"refresh_token":"%s"}' "${RT_ITEMS[$i]}"
  done
  echo ""
  echo "]"
} > "$RT_FILE"

echo "已写入 $DATA_DIR/access_tokens.json ($COUNT 个)"
echo "已写入 $DATA_DIR/refresh_tokens.json"

# ============================================================================
# 5. 生成管理员 Token 池
# ============================================================================
echo ""
echo "[5/6] 生成 $ADMIN_TOKEN_POOL_SIZE 个管理员 Token..."

if [ -n "$ADMIN_TOKEN" ]; then
  {
    echo "["
    for i in $(seq 1 "$ADMIN_TOKEN_POOL_SIZE"); do
      if [ "$i" -gt 1 ]; then
        printf ','
      fi
      printf '{"access_token":"%s"}' "$ADMIN_TOKEN"
    done
    echo ""
    echo "]"
  } > "$DATA_DIR/admin_tokens.json"
  echo "已写入 $DATA_DIR/admin_tokens.json ($ADMIN_TOKEN_POOL_SIZE 个)"
else
  echo '[]' > "$DATA_DIR/admin_tokens.json"
  echo "跳过: 无管理员 Token"
fi

# ============================================================================
# 6. 生成恶意账号池
# ============================================================================
echo ""
echo "[6/6] 生成 $MALICIOUS_POOL_SIZE 个恶意账号..."

{
  echo "["
  FIRST=true
  for i in $(seq 1 "$MALICIOUS_POOL_SIZE"); do
    EMAIL="malicious-${i}-${TIMESTAMP}@example.com"

    RESPONSE=$(http_post "/api/v1/register" "{\"email\":\"${EMAIL}\",\"password\":\"${TEST_PASSWORD}\"}")
    HTTP_CODE=$(echo "$RESPONSE" | tail -1)
    BODY=$(echo "$RESPONSE" | sed '$d')

    if [ "$HTTP_CODE" = "201" ]; then
      USER_ID=$(extract_field "$BODY" "user_id")

      if [ -n "$USER_ID" ]; then
        http_post "/api/v1/test/verify-email" "{\"token\":\"${USER_ID}\"}" > /dev/null 2>&1 || true
      fi

      if [ "$FIRST" = true ]; then FIRST=false; else echo ","; fi
      printf '{"id":"%s","email":"%s","password":"%s"}' "$USER_ID" "$EMAIL" "$TEST_PASSWORD"
    fi
  done
  echo ""
  echo "]"
} > "$DATA_DIR/malicious_users.json"

echo "已写入 $DATA_DIR/malicious_users.json"

# ============================================================================
# 完成
# ============================================================================
echo ""
echo "=== 数据准备完成 ==="
echo "数据目录: $DATA_DIR/"
echo ""
echo "文件清单:"
ls -la "$DATA_DIR/"
echo ""
echo "可开始执行压测场景:"
echo "  make loadtest-s1   # 公开读接口基线"
echo "  make loadtest-s2   # 登录单接口"
echo "  make loadtest-s5   # UserInfo 高频读取"
echo "  make loadtest-s8   # 混合流量"
echo "  make loadtest-s9   # 安全保护专项"
echo "  make loadtest-s10  # 突刺与恢复"
echo "  make loadtest-soak # 长稳态测试"
