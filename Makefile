# ============================================================================
# SSO服务构建配置
# ============================================================================

# Shell configuration for proper error propagation
.ONESHELL:
SHELL := /bin/bash
.SHELLFLAGS := -e -u -o pipefail -c

# 变量定义
APP_NAME=sso
BUILD_DIR=./bin
MIGRATION_DIR=./migrations
TEST_DATABASE_URL ?= postgres://sso:sso@192.168.1.3:5432/sso_test?sslmode=disable

# 版本信息（通过git自动获取）
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
BUILD_TIME ?= $(shell date -u '+%Y-%m-%dT%H:%M:%SZ')
LDFLAGS = -ldflags "-X main.Version=$(VERSION) -X main.BuildTime=$(BUILD_TIME)"

# ============================================================================
# 默认目标
# ============================================================================
.PHONY: all
all: test build  ## 运行测试并构建

# ============================================================================
# 构建相关
# ============================================================================
.PHONY: build
build: ## 构建应用二进制文件（自动注入版本信息）
	@echo "构建 $(APP_NAME) $(VERSION)..."
	@mkdir -p $(BUILD_DIR) || { echo "❌ 创建构建目录失败"; exit 1; }
	go build $(LDFLAGS) -o $(BUILD_DIR)/$(APP_NAME) cmd/server/main.go || { \
		echo "❌ 构建失败"; \
		exit 1; \
	}
	@echo "✅ 构建完成: $(BUILD_DIR)/$(APP_NAME) (版本: $(VERSION), 构建时间: $(BUILD_TIME))"

.PHONY: release
release: ## 构建发布版本（清理后构建，带版本标签）
	@echo "构建发布版本..."
	$(MAKE) clean
	$(MAKE) build
	@echo "发布版本构建完成"

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
	@test -f .env.test || { echo "❌ .env.test 不存在，请参照 .env.example 创建"; exit 1; }; set -a; source .env.test; set +a; \
	DATABASE_URL="$(TEST_DATABASE_URL)" gotestsum --format pkgname -- -race -timeout 120s ./... || { \
		echo "❌ 测试失败"; \
		exit 1; \
	}
	@echo "✅ 所有测试通过"

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
	@E2E_ADMIN_EMAIL="system@eninte.com" E2E_ADMIN_PASSWORD="Admin1234!" RATE_LIMIT_REQUESTS=0 CAPTCHA_ENABLED=false DATABASE_URL="$(TEST_DATABASE_URL)" gotestsum --format pkgname -- -race -tags=e2e ./test/e2e/... || { \
		echo "❌ E2E测试失败"; \
		exit 1; \
	}
	@echo "✅ E2E测试通过"

.PHONY: test-e2e-prepare
test-e2e-prepare: ## 准备E2E测试数据（验证测试用户邮箱）
	@DATABASE_URL="$(TEST_DATABASE_URL)" bash scripts/prepare-e2e-test.sh

.PHONY: test-e2e-cleanup
test-e2e-cleanup: ## 清理E2E测试数据（CI安全，非交互模式）
	@DATABASE_URL="$(TEST_DATABASE_URL)" bash scripts/cleanup-e2e-test.sh --force

.PHONY: test-e2e-full
test-e2e-full: test-e2e-prepare test-e2e test-e2e-cleanup ## 完整E2E测试流程（准备数据 + 运行测试 + 清理）

.PHONY: test-coverage
test-coverage: ## 生成测试覆盖率报告（HTML）并执行阈值检查（>=80%）
	@test -f .env.test || { echo "❌ .env.test 不存在，请参照 .env.example 创建"; exit 1; }; set -a; source .env.test; set +a; \
	DATABASE_URL="$(TEST_DATABASE_URL)" go test -coverprofile=coverage.out $$(go list ./internal/... | grep -v '/store/mock' | grep -v '/internal/app$$' | grep -v '/internal/testing/') || { \
		echo "❌ 覆盖率测试失败"; \
		exit 1; \
	}
	@go tool cover -func=coverage.out | grep "total:" || { \
		echo "❌ 覆盖率报告生成失败"; \
		exit 1; \
	}
	@echo "---"
	@echo "生成 HTML 覆盖率报告..."
	@go tool cover -html=coverage.out -o coverage.html
	@COVERAGE=$$(go tool cover -func=coverage.out | grep total | awk '{print $$3}' | sed 's/%//'); \
	echo "当前覆盖率: $$COVERAGE% (阈值: 80.0%)"; \
	awk -v c=$$COVERAGE 'BEGIN { exit !(c >= 80.0) }' || { \
		echo "❌ 覆盖率 $$COVERAGE% 低于 80% 阈值"; \
		exit 1; \
	}
	@echo "✅ 覆盖率报告: coverage.html"
	@echo "✅ 覆盖率检查通过"

.PHONY: test-e2e-coverage
test-e2e-coverage: ## 生成 E2E 测试覆盖率报告（需要服务运行中）
	@echo ">>> 运行 E2E 测试并收集覆盖率..."
	@E2E_ADMIN_EMAIL="system@eninte.com" E2E_ADMIN_PASSWORD="Admin1234!" \
		RATE_LIMIT_REQUESTS=0 CAPTCHA_ENABLED=false \
		DATABASE_URL="$(TEST_DATABASE_URL)" \
		gotestsum --format pkgname -- \
			-race -tags=e2e -timeout 300s \
			-coverprofile=e2e-coverage.out \
			-coverpkg=./internal/... \
			./test/e2e/... || { \
		echo "❌ E2E 覆盖率测试失败"; \
		exit 1; \
	}
	@echo ">>> E2E 覆盖率函数级报告："
	@go tool cover -func=e2e-coverage.out | grep "total:" || echo "(无覆盖率数据)"
	@echo ">>> 生成 HTML 报告：e2e-coverage.html"
	@go tool cover -html=e2e-coverage.out -o e2e-coverage.html
	@echo "✅ E2E 覆盖率报告: e2e-coverage.html"

.PHONY: test-coverage-full
test-coverage-full: ## 合并单元/集成/E2E 覆盖率报告
	@test -f .env.test || { echo "❌ .env.test 不存在，请参照 .env.example 创建"; exit 1; }
	@echo ">>> [1/3] 运行单元+集成测试覆盖率..."
	@set -a; source .env.test; set +a; \
		DATABASE_URL="$(TEST_DATABASE_URL)" go test -race -tags=integration -timeout 180s \
			-coverprofile=unit-coverage.out \
			-coverpkg=./internal/... \
			$$(go list ./internal/... | grep -v '/store/mock' | grep -v '/internal/app$$' | grep -v '/internal/testing/') || { \
		echo "❌ 单元+集成覆盖率测试失败"; \
		exit 1; \
	}
	@echo ">>> [2/3] 运行 E2E 测试覆盖率..."
	@E2E_ADMIN_EMAIL="system@eninte.com" E2E_ADMIN_PASSWORD="Admin1234!" \
		RATE_LIMIT_REQUESTS=0 CAPTCHA_ENABLED=false \
		DATABASE_URL="$(TEST_DATABASE_URL)" \
		go test -race -tags=e2e -timeout 300s \
			-coverprofile=e2e-coverage.out \
			-coverpkg=./internal/... \
			./test/e2e/... || { \
		echo "❌ E2E 覆盖率测试失败"; \
		exit 1; \
	}
	@echo ">>> [3/3] 合并覆盖率报告..."
	@go tool cover -merge unit-coverage.out e2e-coverage.out -o full-coverage.out
	@echo ">>> 合并后函数级覆盖率："
	@go tool cover -func=full-coverage.out | grep "total:" || echo "(无覆盖率数据)"
	@go tool cover -html=full-coverage.out -o full-coverage.html
	@echo "✅ 合并覆盖率报告: full-coverage.html"

.PHONY: test-report
test-report: ## 生成JUnit XML测试报告
	gotestsum --junitfile test-results.xml --format pkgname -- -race ./...
	@echo "测试报告: test-results.xml"

.PHONY: test-failed
test-failed: ## 仅重跑失败的测试
	gotestsum --rerun-fails --format pkgname -- -race ./...

.PHONY: test-coverage-check
test-coverage-check: ## 运行测试并检查覆盖率阈值 (>=80%，排除app组合根/store/mock/testing基础设施)
	@test -f .env.test || { echo "❌ .env.test 不存在，请参照 .env.example 创建"; exit 1; }; set -a; source .env.test; set +a; \
	DATABASE_URL="$(TEST_DATABASE_URL)" go test -coverprofile=coverage.out $$(go list ./internal/... | grep -v '/store/mock' | grep -v '/internal/app$$' | grep -v '/internal/testing/') > /dev/null 2>&1 || { \
		echo "❌ 测试失败"; \
		exit 1; \
	}
	@COVERAGE=$$(go tool cover -func=coverage.out | grep total | awk '{print $$3}' | sed 's/%//'); \
	echo "当前覆盖率: $$COVERAGE% (阈值: 80.0%)"; \
	awk -v c=$$COVERAGE 'BEGIN { exit !(c >= 80.0) }' || { \
		echo "❌ 覆盖率 $$COVERAGE% 低于 80% 阈值"; \
		exit 1; \
	}
	@echo "✅ 覆盖率检查通过"

.PHONY: test-security
test-security: ## 运行安全检查
	@go vet ./... || { echo "❌ go vet 安全检查失败"; exit 1; }
	@which govulncheck > /dev/null || go install golang.org/x/vuln/cmd/govulncheck@v1.1.4 || { \
		echo "❌ govulncheck 安装失败"; \
		exit 1; \
	}
	@govulncheck ./... || { echo "❌ 漏洞检查失败"; exit 1; }
	@echo "✅ 安全检查通过"

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
# Docker镜像推送 (Docker Hub)
# ============================================================================
DOCKERHUB_USER ?= your-dockerhub-user
IMAGE_NAME = $(DOCKERHUB_USER)/$(APP_NAME)

.PHONY: docker-push
docker-push: ## 构建并推送镜像到Docker Hub (latest标签)
	docker build -f docker/Dockerfile -t $(IMAGE_NAME):latest .
	docker push $(IMAGE_NAME):latest

.PHONY: docker-push-tag
docker-push-tag: ## 构建并推送带版本标签的镜像到Docker Hub
	docker build -f docker/Dockerfile -t $(IMAGE_NAME):$(VERSION) -t $(IMAGE_NAME):latest .
	docker push $(IMAGE_NAME):$(VERSION)
	docker push $(IMAGE_NAME):latest

# ============================================================================
# TrueNAS 部署
# ============================================================================
TRUENAS_HOST ?= 192.168.1.3
TRUENAS_USER ?= root

.PHONY: deploy
deploy: ## 部署到TrueNAS (用法: make deploy [TRUENAS_HOST=192.168.1.3])
	TRUENAS_HOST=$(TRUENAS_HOST) TRUENAS_USER=$(TRUENAS_USER) bash scripts/deploy-truenas.sh

# ============================================================================
# 代码质量
# ============================================================================
.PHONY: lint
lint: ## 运行代码检查
	@go vet ./... || { echo "❌ go vet 检查失败"; exit 1; }
	@which golangci-lint > /dev/null || { echo "❌ 未安装 golangci-lint，请安装后再运行 lint"; exit 1; }
	@golangci-lint run ./... || { echo "❌ golangci-lint 检查失败"; exit 1; }
	@echo "✅ 代码检查通过"

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
# 错误处理验证
# ============================================================================
.PHONY: test-error-handling
test-error-handling: ## 验证 Makefile 错误处理机制
	@echo "=========================================="
	@echo "  Makefile 错误处理验证测试"
	@echo "=========================================="
	@echo ""
	@echo "测试 1: 构建失败错误传播"
	@echo "-------------------------------------------"
	@bash -c 'set -e; false; echo "这行不应该被执行"' 2>/dev/null && { \
		echo "❌ 测试失败：错误未正确传播"; \
		exit 1; \
	} || echo "✅ 通过：false 命令正确导致脚本终止"
	@echo ""
	@echo "测试 2: 命令链错误中断（管道）"
	@echo "-------------------------------------------"
	@bash -c 'set -e -o pipefail; false | true' 2>/dev/null && { \
		echo "❌ 测试失败：管道错误未正确传播"; \
		exit 1; \
	} || echo "✅ 通过：管道中的失败命令正确导致脚本终止"
	@echo ""
	@echo "测试 3: 未定义变量检测"
	@echo "-------------------------------------------"
	@bash -c 'set -u; echo $$UNDEFINED_VAR' 2>/dev/null && { \
		echo "❌ 测试失败：未定义变量未被检测"; \
		exit 1; \
	} || echo "✅ 通过：未定义变量正确触发错误"
	@echo ""
	@echo "=========================================="
	@echo "✅ 所有 Makefile 错误处理测试通过"
	@echo "=========================================="

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
# 压力测试 (k6)
# ============================================================================
K6_LOADTEST_DIR=./loadtest
K6_DATA_DIR=$(K6_LOADTEST_DIR)/data
K6_RESULTS_DIR=$(K6_LOADTEST_DIR)/results
K6 ?= k6
BASE_URL ?= http://localhost:9090

# 压测环境变量默认值（与 .env.test 保持一致，可通过 shell 环境变量或命令行覆盖）
E2E_ADMIN_EMAIL ?= system@eninte.com
E2E_ADMIN_PASSWORD ?= Admin1234!
TEST_PASSWORD ?= TestPassword123!
OAUTH_CLIENT_SECRET ?=
OAUTH_PUBLIC_CLIENT_ID ?= public-test-client
OAUTH_CONFIDENTIAL_CLIENT_ID ?= confidential-test-client
OAUTH_REDIRECT_URI ?= http://localhost:3000/callback

# 压测通用环境变量（从 .env.test 读取，可通过命令行覆盖）
K6_ENV_COMMON = \
	-e BASE_URL=$(BASE_URL) \
	-e E2E_ADMIN_EMAIL=$(E2E_ADMIN_EMAIL) \
	-e E2E_ADMIN_PASSWORD=$(E2E_ADMIN_PASSWORD) \
	-e TEST_PASSWORD=$(TEST_PASSWORD) \
	-e OAUTH_CLIENT_SECRET=$(OAUTH_CLIENT_SECRET) \
	-e OAUTH_PUBLIC_CLIENT_ID=$(OAUTH_PUBLIC_CLIENT_ID) \
	-e OAUTH_CONFIDENTIAL_CLIENT_ID=$(OAUTH_CONFIDENTIAL_CLIENT_ID) \
	-e OAUTH_REDIRECT_URI=$(OAUTH_REDIRECT_URI)

.PHONY: loadtest-prepare
loadtest-prepare: ## 准备压测数据 (生成用户池、Token池等)
	@E2E_ADMIN_EMAIL=$(E2E_ADMIN_EMAIL) E2E_ADMIN_PASSWORD=$(E2E_ADMIN_PASSWORD) TEST_PASSWORD=$(TEST_PASSWORD) BASE_URL=$(BASE_URL) \
	bash scripts/prepare-loadtest-data.sh

.PHONY: loadtest-s1
loadtest-s1: ## S1: 公开读接口基线
	@mkdir -p $(K6_RESULTS_DIR)
	$(K6) run --out json=$(K6_RESULTS_DIR)/s1_$(shell date +%Y%m%d_%H%M%S).json \
		$(K6_ENV_COMMON) \
		$(K6_LOADTEST_DIR)/scenarios/s1_public_read.js

.PHONY: loadtest-s2
loadtest-s2: ## S2: 登录单接口 (需要 loadtest/data/users.json)
	@mkdir -p $(K6_RESULTS_DIR)
	@test -f $(K6_DATA_DIR)/users.json || { echo "错误: 数据文件不存在，请先运行 make loadtest-prepare"; exit 1; }
	$(K6) run --out json=$(K6_RESULTS_DIR)/s2_$(shell date +%Y%m%d_%H%M%S).json \
		$(K6_ENV_COMMON) \
		-e USER_POOL_FILE=$(K6_DATA_DIR)/users.json \
		$(K6_LOADTEST_DIR)/scenarios/s2_login.js

.PHONY: loadtest-s3
loadtest-s3: ## S3: 注册单接口
	@mkdir -p $(K6_RESULTS_DIR)
	$(K6) run --out json=$(K6_RESULTS_DIR)/s3_$(shell date +%Y%m%d_%H%M%S).json \
		$(K6_ENV_COMMON) \
		$(K6_LOADTEST_DIR)/scenarios/s3_register.js

.PHONY: loadtest-s4
loadtest-s4: ## S4: Refresh Token 单接口 (需要 loadtest/data/refresh_tokens.json)
	@mkdir -p $(K6_RESULTS_DIR)
	@test -f $(K6_DATA_DIR)/refresh_tokens.json || { echo "错误: 数据文件不存在，请先运行 make loadtest-prepare"; exit 1; }
	$(K6) run --out json=$(K6_RESULTS_DIR)/s4_$(shell date +%Y%m%d_%H%M%S).json \
		$(K6_ENV_COMMON) \
		-e REFRESH_TOKEN_POOL_FILE=$(K6_DATA_DIR)/refresh_tokens.json \
		$(K6_LOADTEST_DIR)/scenarios/s4_refresh_token.js

.PHONY: loadtest-s5
loadtest-s5: ## S5: UserInfo 高频读取 (需要 loadtest/data/access_tokens.json)
	@mkdir -p $(K6_RESULTS_DIR)
	@test -f $(K6_DATA_DIR)/access_tokens.json || { echo "错误: 数据文件不存在，请先运行 make loadtest-prepare"; exit 1; }
	$(K6) run --out json=$(K6_RESULTS_DIR)/s5_$(shell date +%Y%m%d_%H%M%S).json \
		$(K6_ENV_COMMON) \
		-e ACCESS_TOKEN_POOL_FILE=$(K6_DATA_DIR)/access_tokens.json \
		$(K6_LOADTEST_DIR)/scenarios/s5_userinfo.js

.PHONY: loadtest-s6
loadtest-s6: ## S6: OAuth 公共客户端完整流程 (需要 loadtest/data/users.json)
	@mkdir -p $(K6_RESULTS_DIR)
	@test -f $(K6_DATA_DIR)/users.json || { echo "错误: 数据文件不存在，请先运行 make loadtest-prepare"; exit 1; }
	$(K6) run --out json=$(K6_RESULTS_DIR)/s6_$(shell date +%Y%m%d_%H%M%S).json \
		$(K6_ENV_COMMON) \
		-e USER_POOL_FILE=$(K6_DATA_DIR)/users.json \
		$(K6_LOADTEST_DIR)/scenarios/s6_oauth_public.js

.PHONY: loadtest-s7
loadtest-s7: ## S7: OAuth 机密客户端完整流程 (需要 loadtest/data/users.json)
	@mkdir -p $(K6_RESULTS_DIR)
	@test -f $(K6_DATA_DIR)/users.json || { echo "错误: 数据文件不存在，请先运行 make loadtest-prepare"; exit 1; }
	$(K6) run --out json=$(K6_RESULTS_DIR)/s7_$(shell date +%Y%m%d_%H%M%S).json \
		$(K6_ENV_COMMON) \
		-e USER_POOL_FILE=$(K6_DATA_DIR)/users.json \
		$(K6_LOADTEST_DIR)/scenarios/s7_oauth_confidential.js

.PHONY: loadtest-s8
loadtest-s8: ## S8: 混合流量 (需要 loadtest/data/{users,access_tokens,refresh_tokens,admin_tokens}.json)
	@mkdir -p $(K6_RESULTS_DIR)
	@test -f $(K6_DATA_DIR)/users.json || { echo "错误: 数据文件不存在，请先运行 make loadtest-prepare"; exit 1; }
	@test -f $(K6_DATA_DIR)/access_tokens.json || { echo "错误: 数据文件不存在，请先运行 make loadtest-prepare"; exit 1; }
	@test -f $(K6_DATA_DIR)/refresh_tokens.json || { echo "错误: 数据文件不存在，请先运行 make loadtest-prepare"; exit 1; }
	$(K6) run --out json=$(K6_RESULTS_DIR)/s8_$(shell date +%Y%m%d_%H%M%S).json \
		$(K6_ENV_COMMON) \
		-e USER_POOL_FILE=$(K6_DATA_DIR)/users.json \
		-e ACCESS_TOKEN_POOL_FILE=$(K6_DATA_DIR)/access_tokens.json \
		-e REFRESH_TOKEN_POOL_FILE=$(K6_DATA_DIR)/refresh_tokens.json \
		-e ADMIN_TOKEN_POOL_FILE=$(K6_DATA_DIR)/admin_tokens.json \
		$(K6_LOADTEST_DIR)/scenarios/s8_mixed_traffic.js

.PHONY: loadtest-s9
loadtest-s9: ## S9: 安全保护专项 (需要 loadtest/data/{users,malicious_users}.json)
	@mkdir -p $(K6_RESULTS_DIR)
	@test -f $(K6_DATA_DIR)/users.json || { echo "错误: 数据文件不存在，请先运行 make loadtest-prepare"; exit 1; }
	@test -f $(K6_DATA_DIR)/malicious_users.json || { echo "错误: 数据文件不存在，请先运行 make loadtest-prepare"; exit 1; }
	$(K6) run --out json=$(K6_RESULTS_DIR)/s9_$(shell date +%Y%m%d_%H%M%S).json \
		$(K6_ENV_COMMON) \
		-e USER_POOL_FILE=$(K6_DATA_DIR)/users.json \
		-e MALICIOUS_POOL_FILE=$(K6_DATA_DIR)/malicious_users.json \
		$(K6_LOADTEST_DIR)/scenarios/s9_security.js

.PHONY: loadtest-s10
loadtest-s10: ## S10: 突刺与恢复 (需要 loadtest/data/access_tokens.json)
	@mkdir -p $(K6_RESULTS_DIR)
	@test -f $(K6_DATA_DIR)/access_tokens.json || { echo "错误: 数据文件不存在，请先运行 make loadtest-prepare"; exit 1; }
	$(K6) run --out json=$(K6_RESULTS_DIR)/s10_$(shell date +%Y%m%d_%H%M%S).json \
		$(K6_ENV_COMMON) \
		-e ACCESS_TOKEN_POOL_FILE=$(K6_DATA_DIR)/access_tokens.json \
		$(K6_LOADTEST_DIR)/scenarios/s10_spike.js

.PHONY: loadtest-soak
loadtest-soak: ## Soak Test: 长稳态测试 (需要 loadtest/data/{users,access_tokens,refresh_tokens}.json)
	@mkdir -p $(K6_RESULTS_DIR)
	@test -f $(K6_DATA_DIR)/users.json || { echo "错误: 数据文件不存在，请先运行 make loadtest-prepare"; exit 1; }
	@test -f $(K6_DATA_DIR)/access_tokens.json || { echo "错误: 数据文件不存在，请先运行 make loadtest-prepare"; exit 1; }
	@test -f $(K6_DATA_DIR)/refresh_tokens.json || { echo "错误: 数据文件不存在，请先运行 make loadtest-prepare"; exit 1; }
	$(K6) run --out json=$(K6_RESULTS_DIR)/soak_$(shell date +%Y%m%d_%H%M%S).json \
		$(K6_ENV_COMMON) \
		-e USER_POOL_FILE=$(K6_DATA_DIR)/users.json \
		-e ACCESS_TOKEN_POOL_FILE=$(K6_DATA_DIR)/access_tokens.json \
		-e REFRESH_TOKEN_POOL_FILE=$(K6_DATA_DIR)/refresh_tokens.json \
		$(K6_LOADTEST_DIR)/scenarios/soak_test.js

.PHONY: loadtest-clean
loadtest-clean: ## 清理压测数据和结果
	@rm -rf $(K6_DATA_DIR) $(K6_RESULTS_DIR)
	@echo "✓ 压测数据已清理"

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
	@gocyclo -over 20 -avg ./... | tee $(REPORTS_DIR)/static/complexity.txt
	@echo "✓ 复杂度报告: $(REPORTS_DIR)/static/complexity.txt"

.PHONY: analyze-duplication
analyze-duplication: ## 检测代码重复
	@echo "运行重复代码检测..."
	@mkdir -p $(REPORTS_DIR)/static
	@which dupl > /dev/null || (echo "请先安装: make install-analysis-tools" && exit 1)
	@dupl -threshold 150 -html ./internal/... > $(REPORTS_DIR)/static/duplication.html 2>&1 || true
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
