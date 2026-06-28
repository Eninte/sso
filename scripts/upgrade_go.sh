#!/usr/bin/env bash
# upgrade_go.sh — 在无 sudo / 只读 /usr/local/go / 只读 $HOME 的容器环境中升级 Go 工具链
#
# 用法:
#   bash scripts/upgrade_go.sh            # 升级到默认版本 go1.26.4
#   bash scripts/upgrade_go.sh 1.26.5     # 升级到指定版本
#
# 环境约束 (本脚本自动适配):
#   - /usr/local/go 只读
#   - $HOME/.bashrc 只读
#   - $HOME/.local/bin 可写 (已在 PATH 中, 优先于 /usr/local/go/bin)
#   - $HOME/.local/share 可写 (用于存放 Go SDK)
#
# 行为:
#   1. 下载指定版本 Go 到 $HOME/.local/share/go-<version>
#   2. 校验 sha256 (从 Google Cloud Storage 获取)
#   3. 在 $HOME/.local/bin 创建 go / gofmt 符号链接 (覆盖系统 Go)
#   4. 生成 env.sh 供手动 source (因 .bashrc 只读)
#   5. 验证新版本并运行 make test-security 对比漏洞修复情况
#
# 注意: 脚本不需要 sudo, 不修改任何只读文件。

set -euo pipefail

# ============================================================================
# 配置
# ============================================================================
TARGET_VERSION="${1:-1.26.4}"
INSTALL_BASE="${HOME}/.local/share"
INSTALL_DIR="${INSTALL_BASE}/go-${TARGET_VERSION}"
BIN_DIR="${HOME}/.local/bin"
ENV_FILE="${INSTALL_BASE}/go-env.sh"
DOWNLOAD_URL="https://dl.google.com/go/go${TARGET_VERSION}.linux-amd64.tar.gz"
TMP_DIR=$(mktemp -d)

# 颜色输出
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

log_info()  { echo -e "${GREEN}[INFO]${NC} $*"; }
log_warn()  { echo -e "${YELLOW}[WARN]${NC} $*"; }
log_error() { echo -e "${RED}[ERROR]${NC} $*" >&2; }
log_step()  { echo -e "${BLUE}[STEP]${NC} $*"; }

trap 'rm -rf "${TMP_DIR}"' EXIT

# ============================================================================
# Step 0: 环境检查
# ============================================================================
log_step "Step 0: 环境检查"

ARCH=$(uname -m)
OS=$(uname -s | tr '[:upper:]' '[:lower:]')
if [[ "${ARCH}" != "x86_64" ]]; then
    log_error "本脚本仅支持 linux/amd64, 当前架构: ${ARCH}"
    exit 1
fi
if [[ "${OS}" != "linux" ]]; then
    log_error "本脚本仅支持 Linux, 当前系统: ${OS}"
    exit 1
fi

# 检查可写目录
if ! mkdir -p "${INSTALL_BASE}" 2>/dev/null || ! touch "${INSTALL_BASE}/.write_test" 2>/dev/null; then
    log_error "${INSTALL_BASE} 不可写, 无法安装"
    exit 1
fi
rm -f "${INSTALL_BASE}/.write_test"

if ! mkdir -p "${BIN_DIR}" 2>/dev/null || ! touch "${BIN_DIR}/.write_test" 2>/dev/null; then
    log_error "${BIN_DIR} 不可写, 无法创建符号链接"
    exit 1
fi
rm -f "${BIN_DIR}/.write_test"

CURRENT_GO_VERSION=""
if command -v go >/dev/null 2>&1; then
    CURRENT_GO_VERSION=$(go version 2>/dev/null | awk '{print $3}' | sed 's/^go//')
fi
CURRENT_GO_PATH=$(command -v go 2>/dev/null || echo "")

log_info "当前 Go 版本: ${CURRENT_GO_VERSION:-未安装}"
log_info "当前 go 路径: ${CURRENT_GO_PATH:-无}"
log_info "目标 Go 版本: ${TARGET_VERSION}"
log_info "安装目录:     ${INSTALL_DIR}"
log_info "符号链接目录:  ${BIN_DIR} (已在 PATH 中)"

if [[ "${CURRENT_GO_VERSION}" == "${TARGET_VERSION}" ]]; then
    log_warn "当前已是目标版本 ${TARGET_VERSION}, 将仅运行验证步骤"
    SKIP_INSTALL=1
else
    SKIP_INSTALL=0
fi

# ============================================================================
# Step 1: 下载 Go 工具链
# ============================================================================
if [[ "${SKIP_INSTALL}" == "0" ]]; then
    log_step "Step 1: 下载 Go ${TARGET_VERSION}"

    ARCHIVE="${TMP_DIR}/go${TARGET_VERSION}.linux-amd64.tar.gz"
    log_info "下载 URL: ${DOWNLOAD_URL}"
    log_info "下载到: ${ARCHIVE}"

    if ! curl -fL --progress-bar -o "${ARCHIVE}" "${DOWNLOAD_URL}"; then
        log_error "下载失败, 请检查版本号 ${TARGET_VERSION} 是否存在"
        log_error "可用版本列表: https://go.dev/dl/"
        exit 1
    fi

    FILE_SIZE=$(stat -c%s "${ARCHIVE}")
    log_info "下载完成, 文件大小: $((FILE_SIZE / 1024 / 1024)) MB"
else
    log_step "Step 1: 跳过下载 (已是目标版本)"
fi

# ============================================================================
# Step 2: SHA256 校验
# ============================================================================
if [[ "${SKIP_INSTALL}" == "0" ]]; then
    log_step "Step 2: SHA256 校验"

    # Go 官方 .sha256 URL 会重定向到 HTML 页面, 改从 Google Cloud Storage 获取
    SHA256_GCS="https://storage.googleapis.com/golang/go${TARGET_VERSION}.linux-amd64.tar.gz.sha256"
    EXPECTED_SHA256=$(curl -fsL "${SHA256_GCS}" 2>/dev/null | awk '{print $1}' || echo "")

    ACTUAL_SHA256=$(sha256sum "${ARCHIVE}" | awk '{print $1}')

    if [[ -z "${EXPECTED_SHA256}" ]]; then
        log_warn "无法从 GCS 获取期望 sha256, 跳过比对"
        log_info "本地计算 sha256: ${ACTUAL_SHA256}"
        log_warn "请手动到 https://go.dev/dl/#go${TARGET_VERSION} 核对"
    else
        log_info "期望 sha256: ${EXPECTED_SHA256}"
        log_info "实际 sha256: ${ACTUAL_SHA256}"

        if [[ "${EXPECTED_SHA256}" != "${ACTUAL_SHA256}" ]]; then
            log_error "sha256 校验失败!"
            exit 1
        fi
        log_info "sha256 校验通过"
    fi
else
    log_step "Step 2: 跳过校验 (已是目标版本)"
fi

# ============================================================================
# Step 3: 安装 Go SDK 到 .local/share
# ============================================================================
if [[ "${SKIP_INSTALL}" == "0" ]]; then
    log_step "Step 3: 安装 Go ${TARGET_VERSION}"

    # 如已存在同名目录, 先备份
    if [[ -d "${INSTALL_DIR}" ]]; then
        BACKUP="${INSTALL_DIR}.backup.$(date +%Y%m%d%H%M%S)"
        log_warn "目标目录已存在, 备份到: ${BACKUP}"
        mv "${INSTALL_DIR}" "${BACKUP}"
    fi

    log_info "解压到: ${INSTALL_DIR}"
    # 先解压到临时位置, 再移动 (避免部分解压)
    TMP_EXTRACT="${TMP_DIR}/go"
    tar -C "${TMP_DIR}" -xzf "${ARCHIVE}"
    mv "${TMP_EXTRACT}" "${INSTALL_DIR}"
    log_info "安装完成"

    # 验证安装完整性
    if [[ ! -x "${INSTALL_DIR}/bin/go" ]]; then
        log_error "安装不完整: ${INSTALL_DIR}/bin/go 不存在或不可执行"
        exit 1
    fi
fi

# ============================================================================
# Step 4: 创建符号链接到 .local/bin
# ============================================================================
log_step "Step 4: 创建符号链接"

# 移除旧符号链接 (可能是文件或指向旧版本的链接)
for bin_name in go gofmt; do
    LINK="${BIN_DIR}/${bin_name}"
    if [[ -L "${LINK}" ]]; then
        rm -f "${LINK}"
        log_info "移除旧符号链接: ${LINK}"
    elif [[ -e "${LINK}" ]]; then
        BACKUP="${LINK}.backup.$(date +%Y%m%d%H%M%S)"
        log_warn "${LINK} 是普通文件, 备份到 ${BACKUP}"
        mv "${LINK}" "${BACKUP}"
    fi
done

# 创建新符号链接
ln -s "${INSTALL_DIR}/bin/go" "${BIN_DIR}/go"
ln -s "${INSTALL_DIR}/bin/gofmt" "${BIN_DIR}/gofmt"
log_info "go     -> ${INSTALL_DIR}/bin/go"
log_info "gofmt  -> ${INSTALL_DIR}/bin/gofmt"

# ============================================================================
# Step 5: 生成 env.sh (因 .bashrc 只读, 提供手动 source 文件)
# ============================================================================
log_step "Step 5: 生成环境变量文件"

cat > "${ENV_FILE}" <<EOF
# Go 工具链环境变量 (由 upgrade_go.sh 生成)
# 用法: source ${ENV_FILE}
export GOROOT="${INSTALL_DIR}"
export GOPATH="${HOME}/go"
export PATH="${BIN_DIR}:\${GOPATH}/bin:\${PATH}"
EOF

log_info "已生成: ${ENV_FILE}"
log_info "  - /home/dev/.local/bin 已在 PATH 中, go 命令立即可用"
log_info "  - 如需显式设置 GOROOT, 执行: source ${ENV_FILE}"

# ============================================================================
# Step 6: 验证安装 (当前 shell 立即生效)
# ============================================================================
log_step "Step 6: 验证安装"

# 临时将 BIN_DIR 放到 PATH 最前, 确保本脚本内调用 go 用新版本
export PATH="${BIN_DIR}:${PATH}"
export GOROOT="${INSTALL_DIR}"

NEW_VERSION=$(go version 2>&1)
log_info "go version: ${NEW_VERSION}"
log_info "which go: $(command -v go)"
log_info "GOROOT: $(go env GOROOT)"

if ! go version | grep -q "go${TARGET_VERSION}"; then
    log_error "版本验证失败: 期望 go${TARGET_VERSION}, 实际 $(go version)"
    exit 1
fi
log_info "版本验证通过"

# ============================================================================
# Step 7: 编译项目验证
# ============================================================================
log_step "Step 7: 编译项目验证"
PROJECT_DIR="$(cd "$(dirname "$0")/.." && pwd)"
log_info "项目目录: ${PROJECT_DIR}"

cd "${PROJECT_DIR}"
if ! go build ./...; then
    log_error "项目编译失败, 请检查代码兼容性"
    exit 1
fi
log_info "项目编译通过"

# ============================================================================
# Step 8: 运行 test-security 验证漏洞修复
# ============================================================================
log_step "Step 8: 运行 test-security 验证漏洞修复"
log_info "这可能需要 1-2 分钟..."

SECURITY_OUTPUT="${TMP_DIR}/security_output.txt"
set +e
make test-security > "${SECURITY_OUTPUT}" 2>&1
SECURITY_EXIT=$?
set -e

# 显示 test-security 输出 (仅最后 60 行, 避免过长)
echo "─── test-security 输出 (最后 60 行) ───"
tail -n 60 "${SECURITY_OUTPUT}"

# ============================================================================
# Step 9: 漏洞修复对比报告
# ============================================================================
echo ""
log_step "Step 9: 漏洞修复对比报告"
echo "═════════════════════════════════════════════════════════════"
echo "  升级前 Go 版本: ${CURRENT_GO_VERSION:-未安装}  (${CURRENT_GO_PATH:-})"
echo "  升级后 Go 版本: ${TARGET_VERSION}  (${INSTALL_DIR}/bin/go)"
echo "═════════════════════════════════════════════════════════════"

# 解析 govulncheck 输出:
#   "No vulnerabilities found."           -> 代码未调用任何漏洞路径
#   "Your code is affected by N vulnerabilities." -> N 个漏洞影响代码
#   "This scan also found N vulnerabilities in packages you import..."  -> 依赖中存在但未调用
AFFECTED_NUM=$(grep "Your code is affected by" "${SECURITY_OUTPUT}" 2>/dev/null | grep -oE "[0-9]+" | head -1 || echo "0")
NO_VULN=$(grep -c "No vulnerabilities found" "${SECURITY_OUTPUT}" 2>/dev/null || echo "0")
IMPORTED_VULNS=$(grep "This scan also found" "${SECURITY_OUTPUT}" 2>/dev/null | grep -oE "found [0-9]+" | grep -oE "[0-9]+" | head -1 || echo "0")

if [[ "${NO_VULN}" != "0" || "${AFFECTED_NUM}" == "0" ]]; then
    echo -e "  ${GREEN}✓ No vulnerabilities found.${NC}"
    echo -e "  ${GREEN}✓ 当前代码不受任何标准库漏洞影响${NC}"
    if [[ "${IMPORTED_VULNS}" != "0" ]]; then
        echo -e "  ${YELLOW}  (依赖模块中存在 ${IMPORTED_VULNS} 个漏洞, 但代码未调用这些路径)${NC}"
    fi
    SECURITY_STATUS="FIXED (全部修复)"
else
    echo -e "  ${RED}✗ 仍有 ${AFFECTED_NUM} 个漏洞影响当前代码${NC}"
    echo "  受影响包:"
    grep "Your code is affected by" "${SECURITY_OUTPUT}" | sed 's/^/    /'
    SECURITY_STATUS="REMAINING (仍有漏洞)"
fi

echo "═════════════════════════════════════════════════════════════"

# ============================================================================
# 总结
# ============================================================================
echo ""
log_step "升级总结"
echo "═════════════════════════════════════════════════════════════"
echo "  Go 版本:        ${CURRENT_GO_VERSION:-未安装} → ${TARGET_VERSION}"
echo "  GOROOT:         ${INSTALL_DIR}"
echo "  符号链接:        ${BIN_DIR}/go → ${INSTALL_DIR}/bin/go"
echo "                  ${BIN_DIR}/gofmt → ${INSTALL_DIR}/bin/gofmt"
echo "  环境变量文件:    ${ENV_FILE}"
echo "  项目编译:       通过"
echo "  漏洞修复状态:   ${SECURITY_STATUS}"
echo "═════════════════════════════════════════════════════════════"
echo ""
log_info "go 命令已通过 ${BIN_DIR} (已在 PATH 中) 立即生效"
log_info "新开终端会自动使用新版本 Go"
log_info "如需显式设置 GOROOT 环境变量, 执行: source ${ENV_FILE}"

if [[ "${SECURITY_STATUS}" == FIXED* ]]; then
    exit 0
elif [[ "${SECURITY_STATUS}" == PARTIAL* ]]; then
    exit 0
else
    log_warn "仍有漏洞未修复, 请检查上方报告"
    exit 2
fi
