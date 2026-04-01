# ============================================================================
# 代码质量分析目标（追加到主Makefile）
# ============================================================================

# 分析报告目录
REPORTS_DIR=./reports

# ============================================================================
# 工具安装
# ============================================================================
.PHONY: install-analysis-tools
install-analysis-tools: ## 安装所有代码分析工具
	@bash scripts/install-analysis-tools.sh

# ============================================================================
# 完整分析
# ============================================================================
.PHONY: analyze-all
analyze-all: ## 运行完整代码质量分析（约30分钟）
	@bash scripts/run-full-analysis.sh

.PHONY: analyze-quick
analyze-quick: analyze-lint analyze-test-coverage ## 快速分析（lint + 覆盖率）

# ============================================================================
# 静态分析
# ============================================================================
.PHONY: analyze-lint
analyze-lint: ## 运行完整lint检查
	@echo "运行 golangci-lint..."
	@mkdir -p $(REPORTS_DIR)/static
	@golangci-lint run --enable-all --timeout=10m ./... > $(REPORTS_DIR)/static/lint-full.txt 2>&1 || true
	@golangci-lint run --out-format=json ./... > $(REPORTS_DIR)/static/lint.json 2>&1 || true
	@echo "✓ Lint报告: $(REPORTS_DIR)/static/lint-full.txt"

.PHONY: analyze-complexity
analyze-complexity: ## 分析代码复杂度
	@echo "运行复杂度分析..."
	@mkdir -p $(REPORTS_DIR)/static
	@gocyclo -over 15 -avg ./... > $(REPORTS_DIR)/static/complexity.txt 2>&1 || true
	@find internal -name "*.go" -exec gocyclo {} \; 2>/dev/null | sort -nr -k1 > $(REPORTS_DIR)/static/complexity-hotspots.txt || true
	@echo "✓ 复杂度报告: $(REPORTS_DIR)/static/complexity.txt"

.PHONY: analyze-duplication
analyze-duplication: ## 检测代码重复
	@echo "运行重复代码检测..."
	@mkdir -p $(REPORTS_DIR)/static
	@dupl -threshold 50 ./internal/... > $(REPORTS_DIR)/static/duplication.txt 2>&1 || true
	@dupl -threshold 50 -html ./internal/... > $(REPORTS_DIR)/static/duplication.html 2>&1 || true
	@echo "✓ 重复代码报告: $(REPORTS_DIR)/static/duplication.html"

.PHONY: analyze-dependencies
analyze-dependencies: ## 分析依赖关系
	@echo "运行依赖分析..."
	@mkdir -p $(REPORTS_DIR)/static
	@go mod graph > $(REPORTS_DIR)/static/dependencies.txt
	@go list -m all > $(REPORTS_DIR)/static/modules.txt
	@echo "✓ 依赖报告: $(REPORTS_DIR)/static/dependencies.txt"

# ============================================================================
# 安全审计
# ============================================================================
.PHONY: analyze-security
analyze-security: ## 运行完整安全审计
	@echo "运行安全审计..."
	@mkdir -p $(REPORTS_DIR)/security
	@gosec -fmt=json -out=$(REPORTS_DIR)/security/gosec.json ./... 2>&1 || true
	@gosec -fmt=text ./... > $(REPORTS_DIR)/security/gosec.txt 2>&1 || true
	@govulncheck -json ./... > $(REPORTS_DIR)/security/vulncheck.json 2>&1 || true
	@govulncheck ./... > $(REPORTS_DIR)/security/vulncheck.txt 2>&1 || true
	@echo "✓ 安全报告: $(REPORTS_DIR)/security/"

.PHONY: analyze-security-quick
analyze-security-quick: ## 快速安全检查（仅govulncheck）
	@govulncheck ./...

# ============================================================================
# 测试分析
# ============================================================================
.PHONY: analyze-test-coverage
analyze-test-coverage: ## 生成详细测试覆盖率报告
	@echo "运行测试覆盖率分析..."
	@mkdir -p $(REPORTS_DIR)/testing
	@go test -coverprofile=$(REPORTS_DIR)/testing/coverage.out ./... 2>&1 || true
	@go tool cover -func=$(REPORTS_DIR)/testing/coverage.out > $(REPORTS_DIR)/testing/coverage-func.txt
	@go tool cover -html=$(REPORTS_DIR)/testing/coverage.out -o $(REPORTS_DIR)/testing/coverage.html
	@echo "✓ 覆盖率报告: $(REPORTS_DIR)/testing/coverage.html"
	@echo ""
	@echo "总体覆盖率:"
	@go tool cover -func=$(REPORTS_DIR)/testing/coverage.out | grep "total:"

.PHONY: analyze-race
analyze-race: ## 运行竞态条件检测
	@echo "运行竞态检测（可能需要几分钟）..."
	@mkdir -p $(REPORTS_DIR)/testing
	@go test -race -count=5 ./... > $(REPORTS_DIR)/testing/race-detection.txt 2>&1 || true
	@echo "✓ 竞态检测报告: $(REPORTS_DIR)/testing/race-detection.txt"

.PHONY: analyze-test-stats
analyze-test-stats: ## 生成测试统计信息
	@echo "生成测试统计..."
	@mkdir -p $(REPORTS_DIR)/testing
	@echo "=== 测试函数统计 ===" > $(REPORTS_DIR)/testing/test-statistics.txt
	@find internal -name "*_test.go" -exec grep -c "^func Test" {} \; 2>/dev/null | awk '{s+=$$1} END {print "总测试函数数: " s}' >> $(REPORTS_DIR)/testing/test-statistics.txt
	@echo "" >> $(REPORTS_DIR)/testing/test-statistics.txt
	@echo "=== 表驱动测试统计 ===" >> $(REPORTS_DIR)/testing/test-statistics.txt
	@grep -r "tests := \[\]struct" internal/ 2>/dev/null | wc -l | awk '{print "表驱动测试数: " $$1}' >> $(REPORTS_DIR)/testing/test-statistics.txt
	@cat $(REPORTS_DIR)/testing/test-statistics.txt

# ============================================================================
# 性能分析
# ============================================================================
.PHONY: analyze-benchmark
analyze-benchmark: ## 运行基准测试
	@echo "运行基准测试..."
	@mkdir -p $(REPORTS_DIR)/performance
	@go test -bench=. -benchmem ./... > $(REPORTS_DIR)/performance/benchmark.txt 2>&1 || true
	@echo "✓ 基准测试报告: $(REPORTS_DIR)/performance/benchmark.txt"

.PHONY: analyze-cpu
analyze-cpu: ## CPU性能剖析
	@echo "运行CPU剖析..."
	@mkdir -p $(REPORTS_DIR)/performance
	@go test -cpuprofile=$(REPORTS_DIR)/performance/cpu.prof -bench=. ./internal/service/ 2>&1 || true
	@go tool pprof -top $(REPORTS_DIR)/performance/cpu.prof > $(REPORTS_DIR)/performance/cpu-top.txt 2>&1 || true
	@echo "✓ CPU剖析: $(REPORTS_DIR)/performance/cpu-top.txt"

.PHONY: analyze-memory
analyze-memory: ## 内存性能剖析
	@echo "运行内存剖析..."
	@mkdir -p $(REPORTS_DIR)/performance
	@go test -memprofile=$(REPORTS_DIR)/performance/mem.prof -bench=. ./internal/service/ 2>&1 || true
	@go tool pprof -top $(REPORTS_DIR)/performance/mem.prof > $(REPORTS_DIR)/performance/mem-top.txt 2>&1 || true
	@echo "✓ 内存剖析: $(REPORTS_DIR)/performance/mem-top.txt"

# ============================================================================
# 架构分析
# ============================================================================
.PHONY: analyze-architecture
analyze-architecture: ## 分析架构和分层
	@echo "运行架构分析..."
	@mkdir -p $(REPORTS_DIR)/architecture
	@echo "=== 包大小统计 ===" > $(REPORTS_DIR)/architecture/layering-analysis.txt
	@go list -f '{{.ImportPath}} {{.Dir}}' ./... | while read pkg dir; do \
		if [ -d "$$dir" ]; then \
			size=$$(find "$$dir" -name "*.go" 2>/dev/null | xargs wc -l 2>/dev/null | tail -1 | awk '{print $$1}'); \
			echo "$$pkg: $$size lines"; \
		fi \
	done | sort -t: -k2 -nr >> $(REPORTS_DIR)/architecture/layering-analysis.txt
	@echo "" >> $(REPORTS_DIR)/architecture/layering-analysis.txt
	@echo "=== Handler层依赖检查 ===" >> $(REPORTS_DIR)/architecture/layering-analysis.txt
	@grep -r "store\." internal/handler/ 2>/dev/null | grep -v "mock" | grep -v "_test.go" >> $(REPORTS_DIR)/architecture/layering-analysis.txt || echo "✅ 无直接依赖" >> $(REPORTS_DIR)/architecture/layering-analysis.txt
	@echo "✓ 架构分析: $(REPORTS_DIR)/architecture/layering-analysis.txt"

# ============================================================================
# 报告管理
# ============================================================================
.PHONY: analyze-report
analyze-report: ## 查看分析摘要报告
	@if [ -f $(REPORTS_DIR)/EXECUTIVE_SUMMARY.md ]; then \
		cat $(REPORTS_DIR)/EXECUTIVE_SUMMARY.md; \
	else \
		echo "报告未生成，请先运行: make analyze-all"; \
	fi

.PHONY: analyze-clean
analyze-clean: ## 清理所有分析报告
	@echo "清理分析报告..."
	@rm -rf $(REPORTS_DIR)
	@echo "✓ 报告已清理"

.PHONY: analyze-open
analyze-open: ## 在浏览器中打开HTML报告
	@echo "打开HTML报告..."
	@if [ -f $(REPORTS_DIR)/testing/coverage.html ]; then \
		open $(REPORTS_DIR)/testing/coverage.html 2>/dev/null || xdg-open $(REPORTS_DIR)/testing/coverage.html 2>/dev/null || echo "请手动打开: $(REPORTS_DIR)/testing/coverage.html"; \
	fi
	@if [ -f $(REPORTS_DIR)/static/duplication.html ]; then \
		open $(REPORTS_DIR)/static/duplication.html 2>/dev/null || xdg-open $(REPORTS_DIR)/static/duplication.html 2>/dev/null || echo "请手动打开: $(REPORTS_DIR)/static/duplication.html"; \
	fi

# ============================================================================
# 帮助信息
# ============================================================================
.PHONY: analyze-help
analyze-help: ## 显示分析命令帮助
	@echo "代码质量分析命令:"
	@echo ""
	@echo "  完整分析:"
	@echo "    make analyze-all              - 运行所有分析（约30分钟）"
	@echo "    make analyze-quick            - 快速分析（lint + 覆盖率）"
	@echo ""
	@echo "  静态分析:"
	@echo "    make analyze-lint             - Lint检查"
	@echo "    make analyze-complexity       - 复杂度分析"
	@echo "    make analyze-duplication      - 重复代码检测"
	@echo "    make analyze-dependencies     - 依赖分析"
	@echo ""
	@echo "  安全审计:"
	@echo "    make analyze-security         - 完整安全审计"
	@echo "    make analyze-security-quick   - 快速安全检查"
	@echo ""
	@echo "  测试分析:"
	@echo "    make analyze-test-coverage    - 测试覆盖率"
	@echo "    make analyze-race             - 竞态检测"
	@echo "    make analyze-test-stats       - 测试统计"
	@echo ""
	@echo "  性能分析:"
	@echo "    make analyze-benchmark        - 基准测试"
	@echo "    make analyze-cpu              - CPU剖析"
	@echo "    make analyze-memory           - 内存剖析"
	@echo ""
	@echo "  架构分析:"
	@echo "    make analyze-architecture     - 架构和分层分析"
	@echo ""
	@echo "  报告管理:"
	@echo "    make analyze-report           - 查看摘要报告"
	@echo "    make analyze-open             - 打开HTML报告"
	@echo "    make analyze-clean            - 清理报告"
	@echo ""
	@echo "  工具安装:"
	@echo "    make install-analysis-tools   - 安装分析工具"
	@echo ""
