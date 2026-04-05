# 贡献指南

感谢您对SSO服务项目的关注！本文档将帮助您了解如何参与项目开发。

## 开发流程

### 1. 环境准备

```bash
# 克隆项目
git clone <repo-url>
cd sso

# 安装依赖
go mod download

# 安装开发工具
go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest
go install golang.org/x/vuln/cmd/govulncheck@latest
```

### 2. 创建分支

```bash
# 从main分支创建功能分支
git checkout main
git pull origin main
git checkout -b feature/your-feature-name
```

分支命名规范：
- `feature/` - 新功能
- `fix/` - Bug修复
- `refactor/` - 重构
- `docs/` - 文档更新
- `test/` - 测试相关

### 3. 开发代码

遵循项目代码规范，详见 `AGENTS.md`。

### 4. 运行检查

```bash
# 格式化代码
make fmt

# 运行lint检查
make lint

# 运行测试
make test

# 安全检查
make test-security
```

### 5. 提交代码

```bash
git add .
git commit -m "feat: 添加用户注册功能"
git push origin feature/your-feature-name
```

### 6. 创建Pull Request

在GitHub上创建PR，填写PR模板中的信息。

## 提交消息规范

使用 [Conventional Commits](https://www.conventionalcommits.org/) 规范：

```
<type>(<scope>): <description>

[optional body]

[optional footer]
```

### Type类型

| 类型 | 说明 |
|------|------|
| feat | 新功能 |
| fix | Bug修复 |
| docs | 文档更新 |
| style | 代码格式（不影响功能） |
| refactor | 重构 |
| perf | 性能优化 |
| test | 测试相关 |
| chore | 构建/工具相关 |

### 示例

```
feat(auth): 添加MFA认证支持
fix(token): 修复Token刷新并发问题
docs: 更新API文档
test(service): 添加用户服务单元测试
```

## 代码规范

### Go代码规范

- 使用 `go fmt` 格式化代码
- 使用 `golangci-lint` 检查代码
- 导入顺序：标准库 → 第三方库 → 项目包
- 导出函数必须有注释
- 错误必须处理，不能忽略

### 测试规范

- 测试覆盖率不低于80%
- 使用 `testify` 进行断言
- 测试命名：`TestFunctionName_Scenario`
- 使用Mock隔离外部依赖

### 文档规范

- 包必须有包注释
- 导出函数必须有注释
- 复杂逻辑添加行内注释
- 使用中文注释，英文错误消息

## 代码审查

### 审查清单

- [ ] 代码符合项目规范
- [ ] 测试覆盖充分
- [ ] 无安全漏洞
- [ ] 文档已更新
- [ ] 无破坏性变更

### 审查流程

1. 自动化检查通过（CI/CD）
2. 至少1位审查者批准
3. 解决所有审查意见
4. 合并到main分支

## 报告问题

### Bug报告

使用Bug报告模板，包含：
- 问题描述
- 复现步骤
- 预期行为
- 实际行为
- 环境信息

### 功能请求

使用功能请求模板，包含：
- 功能描述
- 使用场景
- 替代方案

## 发布流程

1. 从main创建release分支
2. 更新版本号和CHANGELOG
3. 测试通过后合并到main
4. 创建Git Tag
5. 自动构建和发布

## 获取帮助

- 提交Issue讨论问题
- 查看项目文档
- 阅读代码注释

## 行为准则

- 尊重所有参与者
- 建设性地提供反馈
- 接受批评和建议
- 关注项目目标
