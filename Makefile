# ============================================================================
# SSO服务构建配置
# ============================================================================

# 变量定义
APP_NAME=sso
BUILD_DIR=./bin
MIGRATION_DIR=./migrations
TEST_DATABASE_URL ?= postgres://sso:sso@192.168.1.3:5432/sso_test?sslmode=disable

# ============================================================================
# 默认目标
# ============================================================================
.PHONY: all
all: test build  ## 运行测试并构建

# ============================================================================
# 构建相关
# ============================================================================
.PHONY: build
build: ## 构建应用二进制文件
	@echo "构建 $(APP_NAME)..."
	@mkdir -p $(BUILD_DIR)
	go build -o $(BUILD_DIR)/$(APP_NAME) cmd/server/main.go
	@echo "构建完成: $(BUILD_DIR)/$(APP_NAME)"

.PHONY: run
run: ## 运行应用
	go run cmd/server/main.go

.PHONY: dev
dev: ## 开发模式: 启动依赖服务并运行应用
	@echo "启动开发环境..."
	docker-compose -f docker/docker-compose.yml up -d postgres redis
	@echo "等待数据库就绪..."
	@sleep 3
	go run cmd/server/main.go

# ============================================================================
# 测试相关
# ============================================================================
.PHONY: test
test: ## 运行所有测试
	DATABASE_URL="$(TEST_DATABASE_URL)" gotestsum --format pkgname -- -race -timeout 120s ./...

.PHONY: test-verbose
test-verbose: ## 运行测试（详细输出）
	DATABASE_URL="$(TEST_DATABASE_URL)" gotestsum --format standard-verbose -- -race -count=1 -timeout 120s ./...

.PHONY: test-unit
test-unit: ## 运行单元测试 (短测试)
	DATABASE_URL="$(TEST_DATABASE_URL)" gotestsum --format pkgname -- -race -short -timeout 60s ./...

.PHONY: test-integration
test-integration: ## 运行集成测试
	gotestsum --format pkgname -- -race -tags=integration ./...

.PHONY: test-e2e
test-e2e: ## 运行端到端测试（需要服务运行中）
	E2E_ADMIN_EMAIL="system@eninte.com" E2E_ADMIN_PASSWORD="Admin123!" RATE_LIMIT_REQUESTS=0 DATABASE_URL="$(TEST_DATABASE_URL)" gotestsum --format pkgname -- -race -tags=e2e ./test/e2e/...

.PHONY: test-coverage
test-coverage: ## 生成测试覆盖率报告（HTML）
	DATABASE_URL="$(TEST_DATABASE_URL)" go test -coverprofile=coverage.out $(shell go list ./internal/... | grep -v '/store/mock')
	go tool cover -func=coverage.out | grep "total:"
	@echo "---"
	go tool cover -html=coverage.out -o coverage.html
	@echo "覆盖率报告: coverage.html"

.PHONY: test-report
test-report: ## 生成JUnit XML测试报告
	gotestsum --junitfile test-results.xml --format pkgname -- -race ./...
	@echo "测试报告: test-results.xml"

.PHONY: test-failed
test-failed: ## 仅重跑失败的测试
	gotestsum --rerun-fails --format pkgname -- -race ./...

.PHONY: test-coverage-check
test-coverage-check: ## 运行测试并检查覆盖率阈值 (>=70%)
	@go test -coverprofile=coverage.out ./... > /dev/null 2>&1
	@COVERAGE=$$(go tool cover -func=coverage.out | grep total | awk '{print $$3}' | sed 's/%//'); \
	if [ $$(echo "$$COVERAGE < 70" | bc) -eq 1 ]; then \
		echo "❌ Coverage $$COVERAGE% is below threshold 70%"; \
		exit 1; \
	fi; \
	echo "✅ Coverage: $$COVERAGE%"

.PHONY: test-security
test-security: ## 运行安全检查
	go vet ./...
	@which govulncheck > /dev/null || go install golang.org/x/vuln/cmd/govulncheck@latest
	govulncheck ./...

# ============================================================================
# 数据库迁移
# ============================================================================
.PHONY: migrate-up
migrate-up: ## 执行数据库迁移 (向上)
	migrate -path $(MIGRATION_DIR) -database $$DATABASE_URL up

.PHONY: migrate-down
migrate-down: ## 回滚数据库迁移 (向下)
	migrate -path $(MIGRATION_DIR) -database $$DATABASE_URL down

.PHONY: migrate-create
migrate-create: ## 创建新的迁移文件 (用法: make migrate-create NAME=create_xxx)
	@read -p "输入迁移名称: " name; \
	migrate create -ext sql -dir $(MIGRATION_DIR) -seq $$name

# ============================================================================
# 密钥管理
# ============================================================================
.PHONY: generate-keys
generate-keys: ## 生成RSA密钥对
	@bash scripts/generate-keys.sh

# ============================================================================
# Docker相关
# ============================================================================
.PHONY: docker-build
docker-build: ## 构建Docker镜像
	docker build -f docker/Dockerfile -t $(APP_NAME):latest .

.PHONY: docker-up
docker-up: ## 启动Docker服务 (所有服务)
	docker-compose -f docker/docker-compose.yml up -d

.PHONY: docker-down
docker-down: ## 停止Docker服务
	docker-compose -f docker/docker-compose.yml down

.PHONY: docker-logs
docker-logs: ## 查看Docker日志
	docker-compose -f docker/docker-compose.yml logs -f

# ============================================================================
# 代码质量
# ============================================================================
.PHONY: lint
lint: ## 运行代码检查
	go vet ./...
	@which golangci-lint > /dev/null || echo "建议安装 golangci-lint"
	@golangci-lint run ./... 2>/dev/null || true

.PHONY: fmt
fmt: ## 格式化代码
	go fmt ./...

# ============================================================================
# 清理
# ============================================================================
.PHONY: clean
clean: ## 清理构建文件
	rm -rf $(BUILD_DIR)
	rm -f coverage.out coverage.html test-results.xml

# ============================================================================
# 帮助
# ============================================================================
.PHONY: help
help: ## 显示帮助信息
	@echo "SSO服务构建命令:"
	@echo ""
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-20s\033[0m %s\n", $$1, $$2}'
	@echo ""

# ============================================================================
# 性能基准测试
# ============================================================================
.PHONY: bench
bench: ## 运行所有基准测试
	go test -bench=. -benchmem ./...

.PHONY: bench-db
bench-db: ## 运行数据库基准测试 (需要DATABASE_URL)
	@test -n "$$DATABASE_URL" || (echo "错误: 请设置DATABASE_URL环境变量" && exit 1)
	DATABASE_URL=$$DATABASE_URL go test -bench=BenchmarkStore -benchmem -count=3 ./internal/store/postgres/...

.PHONY: bench-cache
bench-cache: ## 运行缓存基准测试
	go test -bench=Benchmark.*Cache -benchmem -count=3 ./internal/cache/...

.PHONY: bench-service
bench-service: ## 运行服务基准测试
	go test -bench=BenchmarkAuthService -benchmem -count=3 ./internal/service/...

.PHONY: bench-password
bench-password: ## 运行密码服务基准测试
	go test -bench=BenchmarkPasswordService -benchmem -count=3 ./internal/service/...

.PHONY: bench-jwt
bench-jwt: ## 运行JWT服务基准测试
	go test -bench=BenchmarkJWTService -benchmem -count=3 ./internal/service/...

.PHONY: bench-report
bench-report: ## 生成基准测试报告
	@echo "# 性能基准测试报告" > docs/reports/performance-benchmark.md
	@echo "" >> docs/reports/performance-benchmark.md
	@echo "生成时间: $$(date)" >> docs/reports/performance-benchmark.md
	@echo "" >> docs/reports/performance-benchmark.md
	@echo "## 缓存性能" >> docs/reports/performance-benchmark.md
	@echo '```' >> docs/reports/performance-benchmark.md
	go test -bench=Benchmark.*Cache -benchmem ./internal/cache/... 2>&1 | tee -a docs/reports/performance-benchmark.md
	@echo '```' >> docs/reports/performance-benchmark.md
	@echo "" >> docs/reports/performance-benchmark.md
	@echo "## 服务性能" >> docs/reports/performance-benchmark.md
	@echo '```' >> docs/reports/performance-benchmark.md
	go test -bench=BenchmarkAuthService -benchmem ./internal/service/... 2>&1 | tee -a docs/reports/performance-benchmark.md
	@echo '```' >> docs/reports/performance-benchmark.md
	@echo "报告已生成: docs/reports/performance-benchmark.md"

# ============================================================================
# 代码质量分析（新增 2026-03-31）
# ============================================================================

# 分析报告目录
REPORTS_DIR=./reports

.PHONY: install-analysis-tools
install-analysis-tools: ## 安装所有代码分析工具
	@bash scripts/install-analysis-tools.sh

.PHONY: analyze-all
analyze-all: ## 运行完整代码质量分析（约30分钟）
	@bash scripts/run-full-analysis.sh

.PHONY: analyze-quick
analyze-quick: lint test-coverage ## 快速分析（lint + 覆盖率）
	@echo "✓ 快速分析完成"

.PHONY: analyze-report
analyze-report: ## 生成详细分析报告
	@bash scripts/generate-detailed-report.sh

.PHONY: analyze-security-scan
analyze-security-scan: ## 运行安全扫描（gosec + govulncheck）
	@echo "运行安全扫描..."
	@mkdir -p $(REPORTS_DIR)/security
	@gosec -fmt=text ./... 2>&1 | tee $(REPORTS_DIR)/security/gosec.txt || true
	@govulncheck ./... 2>&1 | tee $(REPORTS_DIR)/security/vulncheck.txt || true
	@echo "✓ 安全扫描完成: $(REPORTS_DIR)/security/"

.PHONY: analyze-complexity
analyze-complexity: ## 分析代码复杂度
	@echo "运行复杂度分析..."
	@mkdir -p $(REPORTS_DIR)/static
	@which gocyclo > /dev/null || (echo "请先安装: make install-analysis-tools" && exit 1)
	@gocyclo -over 15 -avg ./... | tee $(REPORTS_DIR)/static/complexity.txt
	@echo "✓ 复杂度报告: $(REPORTS_DIR)/static/complexity.txt"

.PHONY: analyze-duplication
analyze-duplication: ## 检测代码重复
	@echo "运行重复代码检测..."
	@mkdir -p $(REPORTS_DIR)/static
	@which dupl > /dev/null || (echo "请先安装: make install-analysis-tools" && exit 1)
	@dupl -threshold 50 -html ./internal/... > $(REPORTS_DIR)/static/duplication.html 2>&1 || true
	@echo "✓ 重复代码报告: $(REPORTS_DIR)/static/duplication.html"

.PHONY: analyze-clean
analyze-clean: ## 清理所有分析报告
	@echo "清理分析报告..."
	@rm -rf $(REPORTS_DIR)
	@echo "✓ 报告已清理"

.PHONY: analyze-help
analyze-help: ## 显示分析命令详细帮助
	@echo "========================================="
	@echo "  代码质量分析命令"
	@echo "========================================="
	@echo ""
	@echo "完整分析:"
	@echo "  make analyze-all              - 运行所有分析（约30分钟）"
	@echo "  make analyze-quick            - 快速分析（lint + 覆盖率）"
	@echo "  make analyze-report           - 生成详细分析报告"
	@echo ""
	@echo "专项分析:"
	@echo "  make analyze-security-scan    - 安全扫描"
	@echo "  make analyze-complexity       - 复杂度分析"
	@echo "  make analyze-duplication      - 重复代码检测"
	@echo ""
	@echo "报告管理:"
	@echo "  make analyze-clean            - 清理报告"
	@echo ""
	@echo "工具安装:"
	@echo "  make install-analysis-tools   - 安装分析工具"
	@echo ""
	@echo "详细文档:"
	@echo "  .kiro/specs/code-quality-analysis-plan.md"
	@echo "  .kiro/specs/analysis-quick-start.md"
	@echo ""
