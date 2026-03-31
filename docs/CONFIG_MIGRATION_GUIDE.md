# 配置迁移指南

## 变更说明

为了提高安全性和可维护性，我们对配置管理进行了重构：

### 主要变更

1. **配置文件分离**
   - `.env.example` - 可安全分发的配置模板（不含真实凭据）
   - `.env.test` - 真实测试环境配置（从 `.env.example` 复制并修改，不再提交到Git）

2. **AGENTS.md 简化**
   - 移除了具体的配置值和凭据信息
   - 改为引用配置文件
   - 保留了配置说明和环境差异对照

3. **新增文档**
   - `docs/CONFIGURATION.md` - 完整的配置管理指南
   - `docs/CONFIG_MIGRATION_GUIDE.md` - 本迁移指南（仅供参考）

4. **Git忽略规则更新**
   - `.env.test` 已添加到 `.gitignore`
   - `.env.test` 已从Git跟踪中移除

## 迁移步骤

### 对于开发者

如果你已经有本地的 `.env.test` 文件：

1. **备份现有配置**
   ```bash
   cp .env.test .env.test.backup
   ```

2. **验证配置完整性**
   ```bash
   # 检查是否包含所有必需的配置项
   ./scripts/test-env-check.sh
   ```

3. **确认Git状态**
   ```bash
   # .env.test 应该显示为已删除（从Git跟踪中移除）
   git status
   ```

4. **继续使用现有配置**
   - 你的本地 `.env.test` 文件不会被删除
   - 它将继续工作，但不再被Git跟踪
   - 不要提交这个文件到Git

### 对于新加入的开发者

1. **创建测试配置**
   ```bash
   cp .env.example .env.test
   ```

2. **填写真实凭据**
   编辑 `.env.test` 文件，修改以下配置项为真实的测试环境值：
   ```bash
   # 数据库配置
   DB_HOST=192.168.1.3
   DB_PORT=5432
   DB_NAME=sso_test
   DB_USER=sso
   DB_PASSWORD=sso
   DB_SSL_MODE=disable
   
   # Redis配置
   REDIS_HOST=192.168.1.3
   REDIS_PORT=30059
   REDIS_PASSWORD=
   
   # 安全配置
   BCRYPT_COST=10  # 测试环境使用10加快速度
   
   # SMTP配置
   SMTP_HOST=smtp.qiye.aliyun.com
   SMTP_PORT=465
   SMTP_USER=system@eninte.com
   SMTP_PASSWORD=<真实密码>
   SMTP_FROM=system@eninte.com
   
   # E2E测试配置
   E2E_ADMIN_EMAIL=system@eninte.com
   E2E_ADMIN_PASSWORD=Admin123!
   
   # Metrics配置
   METRICS_PASSWORD=test123
   ```

3. **验证配置**
   ```bash
   make test
   ```

### 对于CI/CD

如果你的CI/CD流程依赖 `.env.test`：

1. **使用环境变量**
   在CI/CD平台配置环境变量，而不是依赖文件

2. **或使用密钥管理**
   ```bash
   # 从密钥管理系统获取配置
   ./scripts/fetch-test-config.sh
   ```

3. **或在CI中动态生成**
   ```yaml
   # GitHub Actions 示例
   - name: Create test config
     run: |
       cat > .env.test << EOF
       DB_HOST=${{ secrets.TEST_DB_HOST }}
       DB_PASSWORD=${{ secrets.TEST_DB_PASSWORD }}
       # ... 其他配置
       EOF
   ```

## 配置项对照表

### 从 AGENTS.md 迁移到配置文件

| 原位置 | 新位置 | 说明 |
|--------|--------|------|
| AGENTS.md 数据库配置表 | .env.test | 真实值在配置文件中 |
| AGENTS.md Redis配置表 | .env.test | 真实值在配置文件中 |
| AGENTS.md SMTP配置表 | .env.test | 真实值在配置文件中 |
| AGENTS.md E2E测试配置表 | .env.test | 真实值在配置文件中 |
| AGENTS.md 安全配置表 | .env.test | 真实值在配置文件中 |

### 配置文件对照

| 配置项 | .env.example | .env.test (真实) |
|--------|--------------|------------------|
| DB_HOST | localhost | 192.168.1.3 |
| DB_PASSWORD | changeme | sso |
| DB_SSL_MODE | require | disable |
| BCRYPT_COST | 12 | 10 |
| SMTP_PASSWORD | (空) | 真实密码 |
| E2E_ADMIN_EMAIL | admin@example.com | system@eninte.com |

## 安全检查清单

迁移完成后，请确认：

- [ ] `.env.test` 不在Git跟踪中
  ```bash
  git ls-files .env.test  # 应该无输出
  ```

- [ ] `.env.test` 在 `.gitignore` 中
  ```bash
  grep "\.env\.test" .gitignore  # 应该有输出
  ```

- [ ] `.env.test` 包含所有必需的配置项
  ```bash
  ./scripts/test-env-check.sh  # 应该通过
  ```

- [ ] `.env.example` 不包含真实凭据
  ```bash
  grep -E "(192\.168|system@eninte|123system)" .env.example  # 应该无输出
  ```

- [ ] `AGENTS.md` 不包含真实凭据
  ```bash
  grep -E "(192\.168|system@eninte|123system)" AGENTS.md  # 应该无输出
  ```

## 常见问题

### Q: 我的 .env.test 文件被删除了？

A: 不会。`git rm --cached` 只是从Git跟踪中移除，本地文件仍然存在。

### Q: 如何获取测试环境的真实凭据？

A: 联系团队管理员或查看团队内部文档。

### Q: 为什么要做这个变更？

A: 主要原因：
1. 安全性：避免将真实凭据提交到Git
2. 可维护性：配置集中管理，易于更新
3. 合规性：符合安全最佳实践

### Q: 生产环境配置如何管理？

A: 生产环境配置应该：
1. 使用环境变量或密钥管理系统
2. 不提交到Git
3. 使用强密码和安全配置

### Q: 如何验证配置是否正确？

A: 运行测试：
```bash
make test
```

如果测试通过，说明配置正确。

## 回滚指南

如果需要回滚到旧的配置方式：

```bash
# 1. 恢复 .env.test 到Git跟踪
git checkout HEAD -- .env.test
git add .env.test

# 2. 恢复 AGENTS.md
git checkout HEAD -- AGENTS.md

# 3. 恢复 .gitignore
git checkout HEAD -- .gitignore

# 4. 删除新文件
rm docs/CONFIGURATION.md
rm docs/CONFIG_MIGRATION_GUIDE.md
```

## 相关文档

- [配置管理指南](./CONFIGURATION.md)
- [AGENTS.md](../AGENTS.md)
- [测试指南](../TESTING.md)
- [部署指南](./DEPLOYMENT.md)

## 支持

如有问题，请：
1. 查看本迁移指南
2. 查看 [配置管理指南](./CONFIGURATION.md)
3. 联系团队管理员
4. 提交Issue
