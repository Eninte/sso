# TrueNAS SCALE 部署SSO应用

## 环境信息

| 服务 | 地址 | 端口 |
|------|------|------|
| PostgreSQL | 192.168.1.3 | 5432 |
| Redis | 192.168.1.3 | 6379 |
| SSO应用 | 192.168.1.3 | 9090 |

## 部署步骤

### 1. 准备存储目录

SSH连接TrueNAS后执行：

```bash
# 创建目录
mkdir -p /mnt/pool/sso/keys
mkdir -p /mnt/pool/sso/config
```

### 2. 生成RSA密钥

```bash
# 生成密钥
openssl genrsa -out /mnt/pool/sso/keys/private.pem 2048
openssl rsa -in /mnt/pool/sso/keys/private.pem -pubout -out /mnt/pool/sso/keys/public.pem

# 设置权限
chmod 600 /mnt/pool/sso/keys/private.pem
chmod 644 /mnt/pool/sso/keys/public.pem
```

### 3. 创建配置文件

```bash
cat > /mnt/pool/sso/config/.env << 'EOF'
# 服务器配置
SERVER_HOST=0.0.0.0
SERVER_PORT=9090
SERVER_ENV=production

# PostgreSQL配置（请修改密码）
DB_HOST=192.168.1.3
DB_PORT=5432
DB_NAME=sso
DB_USER=sso
DB_PASSWORD=your_postgres_password_here
DB_SSL_MODE=disable

# Redis配置
REDIS_HOST=192.168.1.3
REDIS_PORT=6379
REDIS_PASSWORD=

# JWT配置
JWT_PRIVATE_KEY_PATH=/app/keys/private.pem
JWT_PUBLIC_KEY_PATH=/app/keys/public.pem
JWT_ACCESS_TOKEN_TTL=15m
JWT_REFRESH_TOKEN_TTL=168h
JWT_ISSUER=sso

# 安全配置
BCRYPT_COST=12
MAX_LOGIN_ATTEMPTS=5
LOCKOUT_DURATION=30m

# CORS（根据您的前端地址修改）
CORS_ALLOWED_ORIGINS=http://192.168.1.3:3000,https://yourdomain.com
EOF
```

**请修改**：
- `DB_PASSWORD` - 您的PostgreSQL密码
- `REDIS_PASSWORD` - Redis密码（如有）
- `CORS_ALLOWED_ORIGINS` - 前端应用地址

### 4. 部署SSO应用

#### 方式A：使用TrueNAS Apps界面

1. 登录TrueNAS SCALE
2. 进入 **Apps** → **Discover Apps**
3. 搜索 **Custom App**
4. 点击 **Install**

配置参数：

| 配置项 | 值 |
|--------|-----|
| Application Name | `sso` |
| Image Repository | `your-registry/sso` |
| Image Tag | `latest` |
| Container Port | `9090` |
| Node Port | `9090` |
| Protocol | `TCP` |

**Environment Variables** 添加以下变量：

```
SERVER_HOST=0.0.0.0
SERVER_PORT=9090
SERVER_ENV=production
DB_HOST=192.168.1.3
DB_PORT=5432
DB_NAME=sso
DB_USER=sso
DB_PASSWORD=your_postgres_password_here
DB_SSL_MODE=disable
REDIS_HOST=192.168.1.3
REDIS_PORT=6379
JWT_PRIVATE_KEY_PATH=/app/keys/private.pem
JWT_PUBLIC_KEY_PATH=/app/keys/public.pem
JWT_ACCESS_TOKEN_TTL=15m
JWT_REFRESH_TOKEN_TTL=168h
JWT_ISSUER=sso
BCRYPT_COST=12
MAX_LOGIN_ATTEMPTS=5
LOCKOUT_DURATION=30m
```

**Storage** 添加：

| Host Path | Mount Path | Read Only |
|-----------|------------|-----------|
| `/mnt/pool/sso/keys` | `/app/keys` | ✅ |

5. 点击 **Install**

#### 方式B：使用Docker Compose

创建docker-compose.yml：

```bash
cat > /mnt/pool/sso/docker-compose.yml << 'EOF'
version: '3.8'

services:
  sso:
    image: your-registry/sso:latest
    container_name: sso-app
    restart: unless-stopped
    ports:
      - "9090:9090"
    env_file:
      - /mnt/pool/sso/config/.env
    volumes:
      - /mnt/pool/sso/keys:/app/keys:ro
    healthcheck:
      test: ["CMD", "curl", "-f", "http://localhost:9090/health"]
      interval: 30s
      timeout: 10s
      retries: 3
EOF
```

启动服务：

```bash
cd /mnt/pool/sso
docker compose up -d
```

### 5. 运行数据库迁移

```bash
# SSH执行
docker exec sso-app migrate -path /app/migrations -database "postgres://sso:your_postgres_password_here@192.168.1.3:5432/sso?sslmode=disable" up
```

### 6. 验证部署

```bash
# 检查健康状态
curl http://192.168.1.3:9090/health

# 预期响应
{"status":"ok","service":"sso","timestamp":"2024-01-15T10:30:00Z"}
```

## 常用命令

```bash
# 查看日志
docker logs sso-app -f

# 重启服务
docker restart sso-app

# 停止服务
docker stop sso-app

# 进入容器
docker exec -it sso-app sh
```

## 故障排查

### 数据库连接失败

```bash
# 测试PostgreSQL连接
docker exec sso-app ping 192.168.1.3

# 检查PostgreSQL是否允许远程连接
docker exec sso-postgres psql -U sso -c "SELECT 1"
```

### Redis连接失败

```bash
# 测试Redis连接
docker exec sso-app ping 192.168.1.3

# 检查Redis
docker exec sso-redis redis-cli ping
```

### 查看详细错误

```bash
docker logs sso-app --tail 100
```

## 网络配置

确保TrueNAS防火墙允许以下端口：

| 端口 | 服务 |
|------|------|
| 9090 | SSO应用 |
| 5432 | PostgreSQL（仅内部） |
| 6379 | Redis（仅内部） |

如果PostgreSQL和Redis不需要外部访问，建议不暴露端口，仅通过内部网络通信。

## 下一步

1. 配置HTTPS（使用反向代理）
2. 设置定期备份
3. 配置监控
