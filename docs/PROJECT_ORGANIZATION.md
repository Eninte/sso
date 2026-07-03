# 项目目录组织说明

## 已完成的清理工作

### 1. 移除临时文件
- ✅ 删除所有覆盖率测试文件 (`coverage*.out`, `coverage.html`)
- ✅ 这些文件在每次运行 `make test-coverage` 时会重新生成

### 2. 整理文档文件
已将以下文档从根目录移至 `docs/` 目录：
- ✅ `CHANGELOG_EMAIL.md` → `docs/CHANGELOG_EMAIL.md`
- ✅ `code-review-fixes-summary.md` → `docs/code-review-fixes-summary.md`
- ✅ `code-review-report.md` → `docs/code-review-report.md`
- ✅ `DOCUMENTATION_UPDATE_SUMMARY.md` → `docs/DOCUMENTATION_UPDATE_SUMMARY.md`
- ✅ `email-optimization-final-report.md` → `docs/email-optimization-final-report.md`
- ✅ `email-template-test-report.md` → `docs/email-template-test-report.md`
- ✅ `FIXES_APPLIED.md` → `docs/FIXES_APPLIED.md`
- ✅ `SECURITY_FIXES_SUMMARY.md` → `docs/SECURITY_FIXES_SUMMARY.md`

### 3. 保留的文件
以下文件保留在根目录（符合 `.gitignore` 规则）：
- `.env.backup_for_test` - 测试环境备份（已在 .gitignore 中）
- `.env.backup_manual` - 手动备份（已在 .gitignore 中）
- AI 工具配置目录（`.claude/`, `.kilo/`, `.kilocode/`, `.lingma/`, `.opencode/`, `.qoder/`, `.trae/`）

## 当前项目结构

```
/home/dev/SSO/
├── .github/              # GitHub 配置（CI/CD、Issue模板）
├── bin/                  # 构建产物目录
├── cmd/                  # 应用入口
│   └── server/          # 服务器主程序
├── docker/              # Docker 配置
├── docs/                # 📚 所有文档（已整理）
│   ├── guides/         # 指南文档
│   ├── reports/        # 测试报告
│   ├── review/         # 代码审查
│   └── superpowers/    # 高级功能文档
├── internal/            # 内部代码（不可导入）
│   ├── app/            # 组合根（依赖装配、路由注册、服务器管理）
│   ├── audit/          # 审计子系统（合规检查、漏洞扫描、报告生成）
│   ├── cache/          # 缓存层（Redis实现、内存回退、LRU淘汰）
│   ├── captcha/        # 验证码服务
│   ├── common/         # 公共工具（语言检测、随机数生成）
│   ├── config/         # 配置管理
│   ├── crypto/         # 加密工具（JWT、密码哈希、密钥加载）
│   ├── errors/         # 统一错误定义
│   ├── handler/        # HTTP 处理器
│   ├── logging/        # 日志工具（结构化日志、敏感信息脱敏）
│   ├── metrics/        # Prometheus 指标收集
│   ├── middleware/      # HTTP 中间件（认证、限流、CORS、安全头）
│   ├── model/          # 数据模型
│   ├── service/        # 业务逻辑层
│   ├── store/          # 数据访问层
│   │   ├── postgres/   # PostgreSQL 实现
│   │   ├── memory/     # 内存存储实现（开发/测试）
│   │   └── mock/       # Mock 实现（单元测试）
│   ├── util/           # 工具模块
│   │   ├── auditutil/  # 审计日志工具
│   │   ├── handlerutil/# Handler 响应工具
│   │   ├── serviceutil/# Service 错误处理工具
│   │   ├── retryutil/  # 重试工具（指数退避）
│   │   └── testutil/   # 测试辅助工具（DB/Redis 连接）
│   └── validator/      # 输入验证
├── keys/                # JWT 密钥（.pem 文件在 .gitignore）
├── loadtest/            # 压力测试（k6 脚本）
├── migrations/          # 数据库迁移
├── scripts/             # 工具脚本
│   ├── prepare-e2e-test.sh     # E2E 测试数据准备
│   ├── cleanup-e2e-test.sh     # E2E 测试数据清理
│   └── run_e2e_no_ratelimit.sh # E2E 服务启动（处理限流）
├── sdks/                # SDK 客户端（Go, JS, Python, Rust）
├── test/                # 测试文件
│   └── e2e/            # E2E 端到端测试（//go:build e2e）
├── testdata/            # 测试数据
├── .env.example         # 环境配置模板
├── .gitignore           # Git 忽略规则
├── .golangci.yml        # Linter 配置
├── AGENTS.md            # AI 代理协作指南
├── go.mod               # Go 模块定义
├── LICENSE              # 许可证
├── Makefile             # 构建命令
└── README.md            # 项目说明
```

## 维护建议

### 定期清理
```bash
# 清理构建产物和临时文件
make clean

# 清理覆盖率文件
rm -f coverage*.out coverage.html
```

### 文档管理
- 所有新文档应放在 `docs/` 目录
- 报告类文档放在 `docs/reports/`
- 指南类文档放在 `docs/guides/`
- 代码审查放在 `docs/review/`

### 不应提交的文件（已在 .gitignore）
- 构建产物：`/bin/`, `/server`
- 环境文件：`.env`, `.env.test`, `.env.backup*`
- 密钥文件：`/keys/*.pem`, `/keys/*.key`
- 覆盖率文件：`coverage*.out`, `coverage.html`
- AI 工具配置：`.kilo/`, `.kilocode/`, `.lingma/` 等

## 快速命令

```bash
# 查看根目录文件（排除隐藏目录）
ls -1 | grep -v "^\."

# 查看所有文档
ls -1 docs/

# 检查未跟踪的文件
git status --short

# 清理所有临时文件
git clean -fdx --dry-run  # 预览
git clean -fdx            # 执行（谨慎使用）
```
