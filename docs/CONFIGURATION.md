# 配置管理指南

## 配置文件说明

### .env.example
- 配置模板文件，可安全分发
- 包含所有配置项的说明和示例值
- 不包含真实凭据
- 提交到Git仓库

### .env.test
- 测试环境配置文件（从 `.env.example` 复制并修改）
- 包含真实的测试环境凭据（数据库密码、SMTP密码等）
- **不可分发，不提交到Git**
- 已添加到 `.gitignore`

### .env
- 本地开发配置文件
- 开发者自行创建和维护
- **不提交到Git**
- 已添加到 `.gitignore`

## 配置项分类

### 1. 服务器配置
```bash
SERVER_HOST=0.0.0.0         # 服务器监听地址
SERVER_PORT=9090            # 服务器监听端口
SERVER_ENV=development      # 运行环境: development / production
```

### 2. 数据库配置 (PostgreSQL)
```bash
DB_HOST=localhost           # 数据库主机
DB_PORT=5432                # 数据库端口
DB_NAME=sso                 # 数据库名称
DB_USER=sso                 # 数据库用户
DB_PASSWORD=changeme        # 数据库密码
DB_SSL_MODE=require         # SSL模式
DB_MAX_OPEN_CONNS=50        # 最大打开连接数
DB_MAX_IDLE_CONNS=25        # 最大空闲连接数
DB_CONN_MAX_LIFETIME=5m     # 连接最大生命周期
DB_QUERY_TIMEOUT=10s        # 查询超时时间
```

### 3. Redis配置
```bash
REDIS_HOST=localhost        # Redis主机
REDIS_PORT=6379             # Redis端口
REDIS_PASSWORD=             # Redis密码
```

### 4. JWT配置
```bash
JWT_PRIVATE_KEY_PATH=./keys/private.pem  # JWT私钥路径
JWT_PUBLIC_KEY_PATH=./keys/public.pem    # JWT公钥路径
JWT_ACCESS_TOKEN_TTL=15m   # Access Token有效期
JWT_REFRESH_TOKEN_TTL=168h # Refresh Token有效期
JWT_ISSUER=sso              # Token签发者标识
KEY_ROTATION_ENABLED=false  # 是否启用密钥轮换
KEY_ROTATION_INTERVAL=2160h # 密钥轮换周期
KEY_TRANSITION_PERIOD=24h   # 密钥过渡期时长
```

### 5. 安全配置
```bash
BCRYPT_COST=12              # bcrypt成本因子
RATE_LIMIT_REQUESTS=100     # 限流: 每个时间窗口的请求数
RATE_LIMIT_WINDOW=1m        # 限流时间窗口
MAX_LOGIN_ATTEMPTS=5        # 最大登录失败次数
LOCKOUT_DURATION=30m        # 账户锁定时长
```

### 6. SMTP配置
```bash
SMTP_HOST=smtp.example.com  # SMTP服务器地址
SMTP_PORT=465               # SMTP端口
SMTP_USER=                  # SMTP用户名
SMTP_PASSWORD=              # SMTP密码
SMTP_FROM=noreply@example.com  # 发件人地址
```

### 7. CORS配置
```bash
CORS_ALLOWED_ORIGINS=http://localhost:3000,http://localhost:8080
```

### 8. Metrics配置
```bash
METRICS_USERNAME=metrics    # Metrics Basic Auth用户名
METRICS_PASSWORD=changeme   # Metrics Basic Auth密码
```

### 9. E2E测试配置
```bash
E2E_ADMIN_EMAIL=admin@example.com    # 测试管理员邮箱
E2E_ADMIN_PASSWORD=Admin123!         # 测试管理员密码
```

## 环境差异对照表

| 配置项 | 开发环境 | 测试环境 | 生产环境 | 说明 |
|--------|---------|---------|---------|------|
| `BCRYPT_COST` | `10` | `10` | `>=12` | 生产环境必须≥12 |
| `DB_SSL_MODE` | `disable` | `disable` | `require` | 生产环境必须require |
| `DB_PASSWORD` | 简单密码 | 测试密码 | 强密码 | 生产环境必须强密码 |
| `REDIS_PASSWORD` | 可选 | 无 | 必须 | 生产环境必须设置 |
| `CORS_ALLOWED_ORIGINS` | `localhost` | `localhost` | 生产域名 | 生产环境必须配置 |
| `METRICS_PASSWORD` | 可选 | 测试密码 | 强密码 | 生产环境必须强密码 |
| `SMTP_PASSWORD` | 可选 | 真实密码 | 真实密码 | 必须配置才能发送邮件 |

## 快速开始

### 1. 本地开发环境
```bash
# 复制配置模板
cp .env.example .env

# 编辑配置文件，修改数据库和Redis连接信息
vim .env

# 生成JWT密钥
make generate-keys

# 启动服务
make dev
```

### 2. 测试环境
```bash
# 复制配置模板
cp .env.example .env.test

# 编辑配置文件，填写真实的测试环境凭据
vim .env.test

# 运行测试
make test

# 或指定配置文件运行
go run ./cmd/server -env .env.test
```

### 3. 生产环境
```bash
# 复制配置模板
cp .env.example .env.production

# 编辑配置文件，确保以下配置正确：
# - DB_PASSWORD: 强密码
# - DB_SSL_MODE: require
# - BCRYPT_COST: >=12
# - REDIS_PASSWORD: 强密码
# - CORS_ALLOWED_ORIGINS: 生产域名
# - METRICS_PASSWORD: 强密码
# - SMTP配置: 完整配置

# 生成生产环境JWT密钥
make generate-keys

# 启动服务
go run ./cmd/server -env .env.production
```

## 安全注意事项

1. **不要提交敏感信息到Git**
   - `.env.test` 已添加到 `.gitignore`
   - `.env` 已添加到 `.gitignore`
   - 确保不要提交包含真实凭据的文件

2. **生产环境必须修改的配置**
   - `DB_PASSWORD`: 使用强密码
   - `DB_SSL_MODE`: 必须为 `require`
   - `BCRYPT_COST`: 必须 >= 12
   - `REDIS_PASSWORD`: 必须设置强密码
   - `CORS_ALLOWED_ORIGINS`: 配置生产域名
   - `METRICS_PASSWORD`: 使用强密码
   - `SMTP_PASSWORD`: 配置真实SMTP密码

3. **密钥管理**
   - JWT密钥文件 (`./keys/*.pem`) 已添加到 `.gitignore`
   - 生产环境必须使用独立的密钥对
   - 定期轮换密钥（建议90天）

4. **配置文件权限**
   ```bash
   # 限制配置文件权限
   chmod 600 .env
   chmod 600 .env.test
   chmod 600 .env.production
   ```

## 故障排查

### 配置文件未找到
```bash
# 检查配置文件是否存在
ls -la .env*

# 如果不存在，从模板创建
cp .env.example .env
```

### 数据库连接失败
```bash
# 检查数据库配置
grep "^DB_" .env

# 测试数据库连接
psql "postgres://$DB_USER:$DB_PASSWORD@$DB_HOST:$DB_PORT/$DB_NAME?sslmode=$DB_SSL_MODE"
```

### Redis连接失败
```bash
# 检查Redis配置
grep "^REDIS_" .env

# 测试Redis连接
redis-cli -h $REDIS_HOST -p $REDIS_PORT -a $REDIS_PASSWORD ping
```

### JWT密钥错误
```bash
# 检查密钥文件是否存在
ls -la keys/

# 重新生成密钥
make generate-keys
```

## 配置验证

运行配置验证脚本：
```bash
# 验证测试环境配置
./scripts/test-env-check.sh

# 验证生产环境配置
./scripts/prod-env-check.sh  # 待实现
```

## 参考文档

- [AGENTS.md](../AGENTS.md) - AI代理协作指南
- [TESTING.md](../TESTING.md) - 测试指南
- [DEPLOYMENT.md](./DEPLOYMENT.md) - 部署指南
