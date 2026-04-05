#!/bin/bash
# SSO服务 - 完整代码质量分析脚本
# 执行所有分析任务并生成报告

set -e

# 颜色定义
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# 日志函数
log_info() {
    echo -e "${BLUE}[INFO]${NC} $1"
}

log_success() {
    echo -e "${GREEN}[SUCCESS]${NC} $1"
}

log_warning() {
    echo -e "${YELLOW}[WARNING]${NC} $1"
}

log_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

# 检查工具是否安装
check_tool() {
    if ! command -v "$1" &> /dev/null; then
        log_warning "$1 未安装，跳过相关检查"
        return 1
    fi
    return 0
}

# 创建报告目录
log_info "创建报告目录..."
mkdir -p reports/{static,security,testing,performance,architecture,documentation}

# 记录开始时间
START_TIME=$(date +%s)

echo ""
echo "========================================="
echo "  SSO 服务代码质量全面分析"
echo "  开始时间: $(date '+%Y-%m-%d %H:%M:%S')"
echo "========================================="
echo ""

# ============================================================================
# 阶段1: 静态代码分析
# ============================================================================
log_info "阶段1: 静态代码分析"
echo ""

# 1.1 Lint检查
if check_tool golangci-lint; then
    log_info "运行 golangci-lint..."
    golangci-lint run --enable-all --timeout=10m ./... > reports/static/lint-full.txt 2>&1 || true
    golangci-lint run --out-format=json ./... > reports/static/lint.json 2>&1 || true
    log_success "Lint检查完成"
else
    log_warning "跳过 golangci-lint 检查"
fi

# 1.2 复杂度分析
if check_tool gocyclo; then
    log_info "运行复杂度分析..."
    gocyclo -over 15 -avg ./... > reports/static/complexity.txt 2>&1 || true
    find internal -name "*.go" -exec gocyclo {} \; 2>/dev/null | sort -nr -k1 > reports/static/complexity-hotspots.txt || true
    log_success "复杂度分析完成"
else
    log_warning "跳过复杂度分析（需要安装 gocyclo）"
fi

# 1.3 重复代码检测
if check_tool dupl; then
    log_info "运行重复代码检测..."
    dupl -threshold 50 ./internal/... > reports/static/duplication.txt 2>&1 || true
    dupl -threshold 50 -html ./internal/... > reports/static/duplication.html 2>&1 || true
    log_success "重复代码检测完成"
else
    log_warning "跳过重复代码检测（需要安装 dupl）"
fi

# 1.4 依赖分析
log_info "运行依赖分析..."
go mod graph > reports/static/dependencies.txt
go list -m all > reports/static/modules.txt
log_success "依赖分析完成"

echo ""

# ============================================================================
# 阶段2: 安全审计
# ============================================================================
log_info "阶段2: 安全审计"
echo ""

# 2.1 gosec扫描
if check_tool gosec; then
    log_info "运行 gosec 安全扫描..."
    gosec -fmt=json -out=reports/security/gosec.json ./... 2>&1 || true
    gosec -fmt=text ./... > reports/security/gosec.txt 2>&1 || true
    log_success "gosec 扫描完成"
else
    log_warning "跳过 gosec 扫描"
fi

# 2.2 govulncheck
if check_tool govulncheck; then
    log_info "运行 govulncheck 漏洞检查..."
    govulncheck -json ./... > reports/security/vulncheck.json 2>&1 || true
    govulncheck ./... > reports/security/vulncheck.txt 2>&1 || true
    log_success "漏洞检查完成"
else
    log_warning "跳过漏洞检查（需要安装 govulncheck）"
fi

# 2.3 手动安全检查
log_info "运行手动安全检查..."
{
    echo "=== JWT 签名算法检查 ==="
    grep -r "SigningMethod" internal/crypto/ || echo "未找到"
    echo ""
    echo "=== 密码哈希检查 ==="
    grep -r "bcrypt.GenerateFromPassword\|bcrypt.Cost" internal/ || echo "未找到"
    echo ""
    echo "=== SQL注入风险检查 ==="
    grep -r "fmt.Sprintf.*SELECT\|INSERT\|UPDATE\|DELETE" internal/store/ || echo "未找到"
    echo ""
} > reports/security/manual-checks.txt
log_success "手动安全检查完成"

echo ""

# ============================================================================
# 阶段3: 测试质量分析
# ============================================================================
log_info "阶段3: 测试质量分析"
echo ""

# 3.1 测试覆盖率
log_info "运行测试覆盖率分析..."
go test -coverprofile=reports/testing/coverage.out ./... 2>&1 || true
if [ -f reports/testing/coverage.out ]; then
    go tool cover -func=reports/testing/coverage.out > reports/testing/coverage-func.txt
    go tool cover -html=reports/testing/coverage.out -o reports/testing/coverage.html
    log_success "覆盖率分析完成"
else
    log_warning "覆盖率文件未生成"
fi

# 3.2 竞态检测
log_info "运行竞态条件检测（可能需要几分钟）..."
go test -race -count=5 ./... > reports/testing/race-detection.txt 2>&1 || true
log_success "竞态检测完成"

# 3.3 测试统计
log_info "生成测试统计..."
{
    echo "=== 测试函数统计 ==="
    find internal -name "*_test.go" -exec grep -c "^func Test" {} \; 2>/dev/null | awk '{s+=$1} END {print "总测试函数数: " s}'
    echo ""
    echo "=== 表驱动测试统计 ==="
    grep -r "tests := \[\]struct" internal/ 2>/dev/null | wc -l | awk '{print "表驱动测试数: " $1}'
    echo ""
    echo "=== 并行测试统计 ==="
    grep -r "t.Parallel()" internal/ 2>/dev/null | wc -l | awk '{print "并行测试数: " $1}'
} > reports/testing/test-statistics.txt
log_success "测试统计完成"

echo ""

# ============================================================================
# 阶段4: 性能剖析
# ============================================================================
log_info "阶段4: 性能剖析"
echo ""

# 4.1 基准测试
log_info "运行基准测试..."
go test -bench=. -benchmem ./... > reports/performance/benchmark.txt 2>&1 || true
log_success "基准测试完成"

# 4.2 CPU剖析
log_info "运行CPU剖析..."
go test -cpuprofile=reports/performance/cpu.prof -bench=. ./internal/service/ 2>&1 || true
if [ -f reports/performance/cpu.prof ]; then
    go tool pprof -top reports/performance/cpu.prof > reports/performance/cpu-top.txt 2>&1 || true
    log_success "CPU剖析完成"
else
    log_warning "CPU profile未生成"
fi

# 4.3 内存剖析
log_info "运行内存剖析..."
go test -memprofile=reports/performance/mem.prof -bench=. ./internal/service/ 2>&1 || true
if [ -f reports/performance/mem.prof ]; then
    go tool pprof -top reports/performance/mem.prof > reports/performance/mem-top.txt 2>&1 || true
    log_success "内存剖析完成"
else
    log_warning "内存profile未生成"
fi

echo ""

# ============================================================================
# 阶段5: 架构分析
# ============================================================================
log_info "阶段5: 架构分析"
echo ""

log_info "运行架构分析..."
{
    echo "=== 包大小统计 ==="
    go list -f '{{.ImportPath}} {{.Dir}}' ./... | while read pkg dir; do
        if [ -d "$dir" ]; then
            size=$(find "$dir" -name "*.go" 2>/dev/null | xargs wc -l 2>/dev/null | tail -1 | awk '{print $1}')
            echo "$pkg: $size lines"
        fi
    done | sort -t: -k2 -nr
    echo ""
    echo "=== Handler层依赖检查 ==="
    grep -r "store\." internal/handler/ 2>/dev/null | grep -v "mock" | grep -v "_test.go" || echo "✅ 无直接依赖"
    echo ""
    echo "=== Service层HTTP依赖检查 ==="
    grep -r "http\." internal/service/ 2>/dev/null | grep -v "_test.go" || echo "✅ 无HTTP依赖"
} > reports/architecture/layering-analysis.txt
log_success "架构分析完成"

echo ""

# ============================================================================
# 阶段6: 文档分析
# ============================================================================
log_info "阶段6: 文档分析"
echo ""

log_info "运行文档分析..."
{
    echo "=== 导出函数统计 ==="
    total_exported=$(grep -r "^func [A-Z]" internal/ 2>/dev/null | wc -l)
    echo "导出函数总数: $total_exported"
    echo ""
    echo "=== 包文档检查 ==="
    find internal -name "*.go" -exec head -5 {} \; 2>/dev/null | grep "^// Package" | wc -l | awk '{print "有包文档的包数: " $1}'
    echo ""
    echo "=== API文档检查 ==="
    ls -lh docs/*.md 2>/dev/null || echo "文档目录: docs/"
} > reports/documentation/documentation-analysis.txt
log_success "文档分析完成"

echo ""

# ============================================================================
# 生成摘要报告
# ============================================================================
log_info "生成执行摘要..."

END_TIME=$(date +%s)
DURATION=$((END_TIME - START_TIME))

cat > reports/EXECUTIVE_SUMMARY.md << EOF
# SSO 服务代码质量分析 - 执行摘要

> **分析日期**: $(date '+%Y-%m-%d %H:%M:%S')  
> **分析耗时**: ${DURATION}秒  
> **分析工具**: golangci-lint, gocyclo, dupl, gosec, govulncheck, go test

---

## 📊 关键指标

### 测试覆盖率
\`\`\`
$(go tool cover -func=reports/testing/coverage.out 2>/dev/null | grep "total:" || echo "未生成覆盖率数据")
\`\`\`

### Lint问题统计
\`\`\`
$(grep -c "^internal/" reports/static/lint-full.txt 2>/dev/null || echo "0") 个问题
\`\`\`

### 安全漏洞
\`\`\`
$(grep -c "Severity:" reports/security/gosec.txt 2>/dev/null || echo "0") 个潜在问题
\`\`\`

### 复杂度热点
\`\`\`
$(head -10 reports/static/complexity-hotspots.txt 2>/dev/null || echo "未生成复杂度数据")
\`\`\`

---

## 📁 详细报告

- [静态分析](static/) - 代码规范、复杂度、重复度
- [安全审计](security/) - 漏洞扫描、安全检查
- [测试质量](testing/) - 覆盖率、竞态检测
- [性能剖析](performance/) - 基准测试、CPU/内存分析
- [架构分析](architecture/) - 分层验证、依赖检查
- [文档分析](documentation/) - 文档完整性

---

## 🎯 下一步行动

1. 查看各子报告了解详细问题
2. 按优先级修复发现的问题
3. 定期重新运行分析验证改进

---

**生成命令**: \`bash scripts/run-full-analysis.sh\`
EOF

log_success "执行摘要已生成: reports/EXECUTIVE_SUMMARY.md"

echo ""
echo "========================================="
echo "  分析完成！"
echo "  总耗时: ${DURATION}秒"
echo "  报告位置: ./reports/"
echo "========================================="
echo ""
log_info "查看摘要: cat reports/EXECUTIVE_SUMMARY.md"
log_info "查看覆盖率: open reports/testing/coverage.html"
log_info "查看重复代码: open reports/static/duplication.html"
