#!/bin/bash
# ============================================================================
# SSO服务性能测试脚本
# ============================================================================

set -e

# 颜色定义
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# 配置
REPORT_DIR="docs/reports"
REPORT_FILE="$REPORT_DIR/performance-benchmark.md"
COUNT=3

echo -e "${BLUE}======================================${NC}"
echo -e "${BLUE}SSO服务性能测试${NC}"
echo -e "${BLUE}======================================${NC}"
echo ""

# 检查是否在项目根目录
if [ ! -f "go.mod" ]; then
    echo -e "${RED}错误: 请在项目根目录运行此脚本${NC}"
    exit 1
fi

# 创建报告目录
mkdir -p "$REPORT_DIR"

# 初始化报告
cat > "$REPORT_FILE" << HEADER
# 性能基准测试报告

**生成时间**: $(date '+%Y-%m-%d %H:%M:%S')
**测试环境**: $(uname -s) $(uname -m)
**Go版本**: $(go version)

---

HEADER

# ============================================================================
# 缓存性能测试
# ============================================================================
echo -e "${YELLOW}[1/4] 运行缓存性能测试...${NC}"
echo "" >> "$REPORT_FILE"
echo "## 缓存性能测试" >> "$REPORT_FILE"
echo "" >> "$REPORT_FILE"
echo '```' >> "$REPORT_FILE"
go test -bench=Benchmark.*Cache -benchmem -count=$COUNT ./internal/cache/... 2>&1 | tee -a "$REPORT_FILE"
echo '```' >> "$REPORT_FILE"
echo "" >> "$REPORT_FILE"
echo -e "${GREEN}缓存测试完成${NC}"

# ============================================================================
# 服务性能测试
# ============================================================================
echo -e "${YELLOW}[2/4] 运行服务性能测试...${NC}"
echo "" >> "$REPORT_FILE"
echo "## 认证服务性能测试" >> "$REPORT_FILE"
echo "" >> "$REPORT_FILE"
echo '```' >> "$REPORT_FILE"
go test -bench=BenchmarkAuthService -benchmem -count=$COUNT ./internal/service/... 2>&1 | tee -a "$REPORT_FILE"
echo '```' >> "$REPORT_FILE"
echo "" >> "$REPORT_FILE"
echo -e "${GREEN}服务测试完成${NC}"

# ============================================================================
# 密码服务性能测试
# ============================================================================
echo -e "${YELLOW}[3/4] 运行密码服务性能测试...${NC}"
echo "" >> "$REPORT_FILE"
echo "## 密码服务性能测试" >> "$REPORT_FILE"
echo "" >> "$REPORT_FILE"
echo '```' >> "$REPORT_FILE"
go test -bench=BenchmarkPasswordService -benchmem -count=$COUNT ./internal/service/... 2>&1 | tee -a "$REPORT_FILE"
echo '```' >> "$REPORT_FILE"
echo "" >> "$REPORT_FILE"
echo -e "${GREEN}密码服务测试完成${NC}"

# ============================================================================
# JWT服务性能测试
# ============================================================================
echo -e "${YELLOW}[4/4] 运行JWT服务性能测试...${NC}"
echo "" >> "$REPORT_FILE"
echo "## JWT服务性能测试" >> "$REPORT_FILE"
echo "" >> "$REPORT_FILE"
echo '```' >> "$REPORT_FILE"
go test -bench=BenchmarkJWTService -benchmem -count=$COUNT ./internal/service/... 2>&1 | tee -a "$REPORT_FILE"
echo '```' >> "$REPORT_FILE"
echo "" >> "$REPORT_FILE"
echo -e "${GREEN}JWT服务测试完成${NC}"

# ============================================================================
# 数据库性能测试 (可选)
# ============================================================================
if [ -n "$DATABASE_URL" ]; then
    echo -e "${YELLOW}[额外] 运行数据库性能测试...${NC}"
    echo "" >> "$REPORT_FILE"
    echo "## 数据库性能测试" >> "$REPORT_FILE"
    echo "" >> "$REPORT_FILE"
    echo '```' >> "$REPORT_FILE"
    DATABASE_URL=$DATABASE_URL go test -bench=BenchmarkStore -benchmem -count=$COUNT ./internal/store/postgres/... 2>&1 | tee -a "$REPORT_FILE"
    echo '```' >> "$REPORT_FILE"
    echo "" >> "$REPORT_FILE"
    echo -e "${GREEN}数据库测试完成${NC}"
else
    echo -e "${YELLOW}[跳过] 数据库测试 (未设置DATABASE_URL)${NC}"
    echo "" >> "$REPORT_FILE"
    echo "## 数据库性能测试" >> "$REPORT_FILE"
    echo "" >> "$REPORT_FILE"
    echo "> 跳过: 未设置DATABASE_URL环境变量" >> "$REPORT_FILE"
    echo "" >> "$REPORT_FILE"
fi

# ============================================================================
# 生成摘要
# ============================================================================
echo "" >> "$REPORT_FILE"
echo "---" >> "$REPORT_FILE"
echo "" >> "$REPORT_FILE"
echo "## 测试说明" >> "$REPORT_FILE"
echo "" >> "$REPORT_FILE"
echo "- **ns/op**: 每次操作的平均耗时(纳秒)" >> "$REPORT_FILE"
echo "- **B/op**: 每次操作的平均内存分配(字节)" >> "$REPORT_FILE"
echo "- **allocs/op**: 每次操作的平均分配次数" >> "$REPORT_FILE"
echo "- **-N**: GOMAXPROCS值(并行度)" >> "$REPORT_FILE"

echo ""
echo -e "${BLUE}======================================${NC}"
echo -e "${GREEN}性能测试完成!${NC}"
echo -e "${BLUE}报告已生成: ${REPORT_FILE}${NC}"
echo -e "${BLUE}======================================${NC}"
