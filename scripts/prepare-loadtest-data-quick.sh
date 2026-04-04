#!/bin/bash
# 快速压测数据准备脚本（少量数据用于测试）

set -euo pipefail

BASE_URL="http://localhost:9090"
USER_POOL_SIZE=50
TOKEN_POOL_SIZE=50
ADMIN_TOKEN_POOL_SIZE=10
MALICIOUS_POOL_SIZE=10
ADMIN_EMAIL="system@eninte.com"
ADMIN_PASSWORD="Admin123!"
TEST_PASSWORD="TestPassword123!"

DATA_DIR="loadtest/data"
TIMESTAMP=$(date +%s)

mkdir -p "$DATA_DIR"

echo "=== SSO 快速压测数据准备 ==="
echo "目标服务: $BASE_URL"
echo "数据目录: $DATA_DIR"
echo ""

# HTTP辅助函数
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

extract_field() {
  local json="$1"
  local key="$2"
  echo "$json" | sed -n "s/.*\"${key}\"[[:space:]]*:[[:space:]]*\"\([^\"]*\)\".*/\1/p" | head -1
}

# 1. 检查服务健康
echo "[1/5] 检查服务健康..."
RESPONSE=$(http_get "/health")
HTTP_CODE=$(echo "$RESPONSE" | tail -1)
if [ "$HTTP_CODE" != "200" ]; then
  echo "错误: 服务不可用 (HTTP $HTTP_CODE)"
  exit 1
fi
echo "服务正常"

# 2. 管理员登录
echo ""
echo "[2/5] 管理员登录..."
RESPONSE=$(http_post "/api/v1/login" "{\"email\":\"${ADMIN_EMAIL}\",\"password\":\"${ADMIN_PASSWORD}\"}")
HTTP_CODE=$(echo "$RESPONSE" | tail -1)
BODY=$(echo "$RESPONSE" | sed '$d')
ADMIN_TOKEN=""
if [ "$HTTP_CODE" = "200" ]; then
  ADMIN_TOKEN=$(extract_field "$BODY" "access_token")
  echo "管理员登录成功"
else
  echo "管理员登录失败"
fi

# 3. 生成普通用户池
echo ""
echo "[3/5] 生成 $USER_POOL_SIZE 个普通用户..."
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
      if [ -n "$USER_ID" ]; then
        http_post "/api/v1/test/verify-email" "{\"token\":\"${USER_ID}\"}" > /dev/null 2>&1 || true
      fi
      if [ "$FIRST" = true ]; then FIRST=false; else echo ","; fi
      printf '{"id":"%s","email":"%s","password":"%s"}' "$USER_ID" "$EMAIL" "$TEST_PASSWORD"
    fi
  done
  echo ""
  echo "]"
} > "$DATA_DIR/users.json"
echo "已写入 $DATA_DIR/users.json"

# 4. 生成Token池
echo ""
echo "[4/5] 生成Token池..."
AT_FILE="$DATA_DIR/access_tokens.json"
RT_FILE="$DATA_DIR/refresh_tokens.json"
AT_ITEMS=()
RT_ITEMS=()
COUNT=0
while IFS= read -r line; do
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
  if [ "$COUNT" -ge "$TOKEN_POOL_SIZE" ]; then break; fi
done < "$DATA_DIR/users.json"
{
  echo "["
  for i in "${!AT_ITEMS[@]}"; do
    if [ "$i" -gt 0 ]; then printf ','; fi
    printf '{"access_token":"%s"}' "${AT_ITEMS[$i]}"
  done
  echo ""
  echo "]"
} > "$AT_FILE"
{
  echo "["
  for i in "${!RT_ITEMS[@]}"; do
    if [ "$i" -gt 0 ]; then printf ','; fi
    printf '{"refresh_token":"%s"}' "${RT_ITEMS[$i]}"
  done
  echo ""
  echo "]"
} > "$RT_FILE"
echo "已写入 $AT_FILE ($COUNT 个)"
echo "已写入 $RT_FILE"

# 5. 生成管理员Token池
echo ""
echo "[5/5] 生成管理员Token池..."
if [ -n "$ADMIN_TOKEN" ]; then
  {
    echo "["
    for i in $(seq 1 "$ADMIN_TOKEN_POOL_SIZE"); do
      if [ "$i" -gt 1 ]; then printf ','; fi
      printf '{"access_token":"%s"}' "$ADMIN_TOKEN"
    done
    echo ""
    echo "]"
  } > "$DATA_DIR/admin_tokens.json"
  echo "已写入 $DATA_DIR/admin_tokens.json ($ADMIN_TOKEN_POOL_SIZE 个)"
else
  echo '[]' > "$DATA_DIR/admin_tokens.json"
  echo "跳过: 无管理员Token"
fi

# 生成恶意账号池
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

echo ""
echo "=== 数据准备完成 ==="
ls -la "$DATA_DIR/"
