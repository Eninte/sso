#!/bin/bash
# SSO服务 - 安装代码分析工具脚本

set -e

GREEN='\033[0;32m'
BLUE='\033[0;34m'
YELLOW='\033[1;33m'
NC='\033[0m'

log_info() {
    echo -e "${BLUE}[INFO]${NC} $1"
}

log_success() {
    echo -e "${GREEN}[SUCCESS]${NC} $1"
}

log_warning() {
    echo -e "${YELLOW}[WARNING]${NC} $1"
}

echo "========================================="
echo "  安装代码分析工具"
echo "========================================="
echo ""

# golangci-lint
log_info "安装 golangci-lint..."
if command -v golangci-lint &> /dev/null; then
    log_warning "golangci-lint 已安装，跳过"
else
    go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest
    log_success "golangci-lint 安装完成"
fi

# gocyclo
log_info "安装 gocyclo..."
if command -v gocyclo &> /dev/null; then
    log_warning "gocyclo 已安装，跳过"
else
    go install github.com/fzipp/gocyclo/cmd/gocyclo@latest
    log_success "gocyclo 安装完成"
fi

# dupl
log_info "安装 dupl..."
if command -v dupl &> /dev/null; then
    log_warning "dupl 已安装，跳过"
else
    go install github.com/mibk/dupl@latest
    log_success "dupl 安装完成"
fi

# gosec
log_info "安装 gosec..."
if command -v gosec &> /dev/null; then
    log_warning "gosec 已安装，跳过"
else
    go install github.com/securego/gosec/v2/cmd/gosec@latest
    log_success "gosec 安装完成"
fi

# govulncheck
log_info "安装 govulncheck..."
if command -v govulncheck &> /dev/null; then
    log_warning "govulncheck 已安装，跳过"
else
    go install golang.org/x/vuln/cmd/govulncheck@latest
    log_success "govulncheck 安装完成"
fi

# gotestsum (更好的测试输出)
log_info "安装 gotestsum..."
if command -v gotestsum &> /dev/null; then
    log_warning "gotestsum 已安装，跳过"
else
    go install gotest.tools/gotestsum@latest
    log_success "gotestsum 安装完成"
fi

# benchstat (基准测试对比)
log_info "安装 benchstat..."
if command -v benchstat &> /dev/null; then
    log_warning "benchstat 已安装，跳过"
else
    go install golang.org/x/perf/cmd/benchstat@latest
    log_success "benchstat 安装完成"
fi

echo ""
echo "========================================="
echo "  所有工具安装完成！"
echo "========================================="
echo ""

# 验证安装
log_info "验证工具安装..."
echo ""
echo "已安装的工具："
command -v golangci-lint &> /dev/null && echo "  ✓ golangci-lint: $(golangci-lint version --format short 2>&1 | head -1)"
command -v gocyclo &> /dev/null && echo "  ✓ gocyclo"
command -v dupl &> /dev/null && echo "  ✓ dupl"
command -v gosec &> /dev/null && echo "  ✓ gosec: $(gosec --version 2>&1 | head -1)"
command -v govulncheck &> /dev/null && echo "  ✓ govulncheck"
command -v gotestsum &> /dev/null && echo "  ✓ gotestsum"
command -v benchstat &> /dev/null && echo "  ✓ benchstat"

echo ""
log_success "准备就绪！运行 'bash scripts/run-full-analysis.sh' 开始分析"
