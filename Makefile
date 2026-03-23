# ============================================================================
# SSO服务构建配置
# ============================================================================

# 变量定义
APP_NAME=sso
BUILD_DIR=./bin
MIGRATION_DIR=./migrations

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
	go test -v -race ./...

.PHONY: test-unit
test-unit: ## 运行单元测试 (短测试)
	go test -v -race -short ./...

.PHONY: test-integration
test-integration: ## 运行集成测试
	go test -v -tags=integration ./...

.PHONY: test-coverage
test-coverage: ## 生成测试覆盖率报告
	go test -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out -o coverage.html
	@echo "覆盖率报告: coverage.html"

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
	rm -f coverage.out coverage.html

# ============================================================================
# 帮助
# ============================================================================
.PHONY: help
help: ## 显示帮助信息
	@echo "SSO服务构建命令:"
	@echo ""
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-20s\033[0m %s\n", $$1, $$2}'
	@echo ""
