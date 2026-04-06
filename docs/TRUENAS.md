# TrueNAS SCALE 部署指南

本文档介绍如何将SSO服务部署到TrueNAS SCALE。

## 部署方式

TrueNAS SCALE支持两种部署方式：

1. **Custom App（推荐）** - 使用Docker Compose
2. **TrueCharts** - 使用预构建的Helm Chart

---

## 方式一：Custom App部署（推荐）

### 前置要求

- TrueNAS SCALE 22.12+（已在 25.04 版本验证）
- 已启用Apps功能
- 至少4GB可用内存
- 至少20GB可用存储

### 步骤1：准备数据集

在TrueNAS中创建数据集用于持久化存储：

```
/mnt/pool/sso/
├── keys/              # RSA密钥
├── config/            # 配置文件
└── logs/              # 日志文件
```

**创建数据集**：

1. 进入 **Storage** → **Pools**
2. 选择您的存储池
3. 点击 **Add Dataset**
4. 创建以下数据集：
   - `sso-keys`
   - `sso-config`

> **注意**：PostgreSQL 和 Redis 已经部署好了，无需创建相关数据集。

### 步骤2：生成RSA密钥

通过SSH连接到TrueNAS：

```bash
ssh admin@truenas-ip

# 创建密钥目录
mkdir -p /mnt/pool/sso/keys

# 生成RSA密钥对
openssl genrsa -out /mnt/pool/sso/keys/private.pem 2048
openssl rsa -in /mnt/pool/sso/keys/private.pem -pubout -out /mnt/pool/sso/keys/public.pem

# 设置权限
chmod 600 /mnt/pool/sso/keys/private.pem
chmod 644 /mnt/pool/sso/keys/public.pem
```

### 步骤3：创建配置文件

创建环境配置文件：

```bash
cat > /mnt/pool/sso/config/.env << 'EOF'
# 服务器配置
SERVER_HOST=0.0.0.0
SERVER_PORT=9090
SERVER_ENV=production

# 数据库配置（指向已部署的 PostgreSQL）
DB_HOST=your-postgres-host  # 修改为您的 PostgreSQL 主机地址
DB_PORT=5432
DB_NAME=sso
DB_USER=sso
DB_PASSWORD=YourStrongPassword123!
DB_SSL_MODE=disable  # 内网可使用 disable，生产环境建议 require

# 数据库连接池配置
DB_MAX_OPEN_CONNS=100
DB_MAX_IDLE_CONNS=50
DB_CONN_MAX_LIFETIME=5m
DB_CONN_MAX_IDLE_TIME=1m
DB_QUERY_TIMEOUT=10s

# Redis配置（指向已部署的 Redis）
REDIS_ENABLE=true
REDIS_HOST=your-redis-host  # 修改为您的 Redis 主机地址
REDIS_PORT=6379
REDIS_PASSWORD=  # 如有密码请填写
REDIS_DB=0
REDIS_CONN_TIMEOUT=5s
REDIS_POOL_SIZE=10
REDIS_MIN_IDLE_CONNS=5

# JWT配置
JWT_PRIVATE_KEY_PATH=/app/keys/private.pem
JWT_PUBLIC_KEY_PATH=/app/keys/public.pem
JWT_ACCESS_TOKEN_TTL=15m
JWT_REFRESH_TOKEN_TTL=168h
JWT_ISSUER=sso

# 密钥轮换配置
KEY_ROTATION_ENABLED=false
KEY_ROTATION_INTERVAL=2160h
KEY_TRANSITION_PERIOD=24h

# 安全配置
BCRYPT_COST=12
RATE_LIMIT_REQUESTS=100
RATE_LIMIT_WINDOW=1m
MAX_LOGIN_ATTEMPTS=5
LOCKOUT_DURATION=30m

# MFA配置（⚠️ 生产环境必须设置强密钥，否则恢复码可被伪造）
MFA_RECOVERY_HMAC_KEY=your_strong_hmac_key_here

# 邮件配置
SMTP_HOST=smtp.example.com
SMTP_PORT=465
SMTP_USER=your_smtp_username
SMTP_PASSWORD=your_smtp_password
SMTP_FROM=noreply@yourdomain.com

# CORS配置（根据您的域名修改）
CORS_ALLOWED_ORIGINS=http://localhost:3000,https://yourdomain.com

# Metrics配置 (Prometheus指标端点认证)
METRICS_USERNAME=metrics
METRICS_PASSWORD=your_metrics_password

# 优雅关闭配置
SHUTDOWN_TIMEOUT=30s
EOF
```

### 步骤4：创建Docker Compose文件

```bash
cat > /mnt/pool/sso/docker-compose.yml << 'EOF'
version: '3.8'

services:
  # SSO服务
  sso:
    image: your-registry/sso:latest  # 替换为您的镜像地址
    container_name: sso-app
    restart: unless-stopped
    ports:
      - "9090:9090"
    environment:
      - TZ=Asia/Shanghai
    env_file:
      - /mnt/pool/sso/config/.env
    volumes:
      - /mnt/pool/sso/keys:/app/keys:ro
    healthcheck:
      test: ["CMD", "curl", "-f", "http://localhost:9090/health"]
      interval: 30s
      timeout: 10s
      retries: 3
      start_period: 10s
    networks:
      - sso-network

networks:
  sso-network:
    driver: bridge
EOF
```

### 步骤5：通过TrueNAS界面部署

> **TrueNAS 25.04 版本界面说明**：
> - 菜单名称为 **Applications**（不是 "Apps"）
> - 需要先配置应用目录才能安装 Custom App

1. **进入Applications界面**
   - 登录TrueNAS SCALE
   - 点击左侧菜单 **Applications**
   - 如果是首次使用，需要先配置 **Apps** → **Configuration** → **Choose Pool**

2. **安装Custom App**
   - 点击 **Discover Apps**
   - 在右上角搜索框输入 "Custom App"
   - 或者直接点击 **Custom App** 按钮
   - 点击 **Install**

3. **配置Custom App**

   **Application Name**: `sso`
   
   **Image Repository**: `your-registry/sso`（或您构建的镜像）
   
   **Image Tag**: `latest`
   
   **Port Forwarding**:
   - Container Port: `9090`
   - Node Port: `9090`
   
   **Environment Variables**:
   添加所有环境变量（从.env文件）
   
   > **提示**：可以点击 "Add from YAML" 批量导入环境变量

   **Storage**:
   - Host Path: `/mnt/pool/sso/keys`
   - Mount Path: `/app/keys`
   - Read Only: ✅

4. **点击Install**

### 步骤6：使用Docker Compose（替代方案）

如果Custom App界面不够灵活，可以使用Docker Compose：

```bash
# SSH连接到TrueNAS
ssh admin@truenas-ip

# 进入sso目录
cd /mnt/pool/sso

# 启动服务
docker compose up -d

# 查看状态
docker compose ps

# 查看日志
docker compose logs -f
```

### 步骤7：运行数据库迁移

```bash
# SSH连接到TrueNAS
ssh admin@truenas-ip

# 运行迁移
# 使用环境变量 DATABASE_URL（推荐）
docker exec sso-app migrate -path /app/migrations -database "postgres://sso:YourStrongPassword123!@your-postgres-host:5432/sso?sslmode=disable" up
```

### 步骤8：验证部署

```bash
# 检查健康状态
curl http://localhost:9090/health

# 预期响应
{"status":"ok","service":"sso","timestamp":"2024-01-15T10:30:00Z"}
```

---

## 方式二：分别部署各个服务（可选）

> **注意**：您的 TrueNAS 25.04 已经部署了 PostgreSQL 和 Redis，可以跳过此部分。
> 此部分仅作为参考，如果需要重新部署或在其他环境部署时使用。

如果需要更精细的控制，可以分别通过TrueNAS Apps部署各个服务。

### 部署PostgreSQL

1. 进入 **Applications** → **Discover Apps**
2. 搜索 "PostgreSQL"
3. 点击 **Install**
4. 配置：
   - App Name: `sso-postgres`
   - Postgres Password: `YourStrongPassword123!`
   - Postgres User: `sso`
   - Postgres Database: `sso`
   - Storage: 选择数据集 `/mnt/pool/sso/postgres`
   - Port: `5432`

### 部署Redis

1. 进入 **Applications** → **Discover Apps**
2. 搜索 "Redis"
3. 点击 **Install**
4. 配置：
   - App Name: `sso-redis`
   - Storage: 选择数据集 `/mnt/pool/sso/redis`
   - Port: `6379`

### 部署SSO应用

1. 进入 **Applications** → **Discover Apps**
2. 搜索 "Custom App"
3. 点击 **Install**
4. 配置SSO容器，指向PostgreSQL和Redis服务

---

## 网络配置

### 端口映射

| 服务 | 容器端口 | 主机端口 | 说明 |
|------|----------|----------|------|
| SSO | 9090 | 9090 | 主服务端口 |
| PostgreSQL | 5432 | - | 仅内部访问 |
| Redis | 6379 | - | 仅内部访问 |

### 内部网络

服务间通过Docker网络 `sso-network` 通信，无需暴露数据库端口到主机。

### 外部访问

通过TrueNAS的反向代理或路由器端口转发实现外部访问。

---

## SSL/HTTPS配置

### 方案1：使用TrueNAS内置反向代理

1. 进入 **Apps** → **Discover Apps**
2. 搜索并安装 "Nginx Proxy Manager" 或 "Traefik"
3. 配置反向代理指向 `localhost:9090`
4. 配置SSL证书

### 方案2：使用Cloudflare Tunnel

```bash
# 安装Cloudflare Tunnel
docker run -d \
  --name cloudflare-tunnel \
  --network host \
  cloudflare/cloudflared:latest tunnel \
  --no-autoupdate run \
  --token YOUR_CLOUDFLARE_TOKEN
```

### 方案3：在SSO容器前添加Nginx

```yaml
# 在docker-compose.yml中添加
services:
  nginx:
    image: nginx:alpine
    container_name: sso-nginx
    restart: unless-stopped
    ports:
      - "443:443"
      - "80:80"
    volumes:
      - /mnt/pool/sso/nginx/nginx.conf:/etc/nginx/nginx.conf:ro
      - /mnt/pool/sso/nginx/certs:/etc/nginx/certs:ro
    depends_on:
      - sso
    networks:
      - sso-network
```

Nginx配置文件：

```nginx
# /mnt/pool/sso/nginx/nginx.conf
events {
    worker_connections 1024;
}

http {
    upstream sso_backend {
        server sso:9090;
    }

    server {
        listen 80;
        server_name sso.yourdomain.com;
        return 301 https://$server_name$request_uri;
    }

    server {
        listen 443 ssl http2;
        server_name sso.yourdomain.com;

        ssl_certificate /etc/nginx/certs/fullchain.pem;
        ssl_certificate_key /etc/nginx/certs/privkey.pem;
        ssl_protocols TLSv1.2 TLSv1.3;

        location / {
            proxy_pass http://sso_backend;
            proxy_set_header Host $host;
            proxy_set_header X-Real-IP $remote_addr;
            proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
            proxy_set_header X-Forwarded-Proto $scheme;
        }
    }
}
```

---

## 备份与恢复

### 自动备份脚本

创建备份脚本：

```bash
cat > /mnt/pool/sso/scripts/backup.sh << 'EOF'
#!/bin/bash
BACKUP_DIR="/mnt/pool/sso/backups"
DATE=$(date +%Y%m%d_%H%M%S)

mkdir -p $BACKUP_DIR

# 备份PostgreSQL
docker exec sso-postgres pg_dump -U sso sso | gzip > $BACKUP_DIR/sso_db_$DATE.sql.gz

# 备份配置文件
tar -czf $BACKUP_DIR/sso_config_$DATE.tar.gz /mnt/pool/sso/config /mnt/pool/sso/keys

# 删除30天前的备份
find $BACKUP_DIR -name "*.sql.gz" -mtime +30 -delete
find $BACKUP_DIR -name "*.tar.gz" -mtime +30 -delete

echo "备份完成: $DATE"
EOF

chmod +x /mnt/pool/sso/scripts/backup.sh
```

### 设置定时备份

在TrueNAS中设置定时任务：

1. 进入 **System Settings** → **Advanced** → **Cron Jobs**
2. 点击 **Add**
3. 配置：
   - Description: `SSO Backup`
   - Command: `/mnt/pool/sso/scripts/backup.sh`
   - Schedule: 每天凌晨2点 (`0 2 * * *`)
   - Enabled: ✅

### 恢复数据

```bash
# 恢复数据库
gunzip -c /mnt/pool/sso/backups/sso_db_20240115_020000.sql.gz | docker exec -i sso-postgres psql -U sso sso

# 恢复配置
tar -xzf /mnt/pool/sso/backups/sso_config_20240115_020000.tar.gz -C /
```

---

## 监控

### 查看容器状态

```bash
# 查看所有容器
docker ps | grep sso

# 查看资源使用
docker stats sso-app sso-postgres sso-redis
```

### 查看日志

```bash
# 查看SSO日志
docker logs sso-app -f

# 查看PostgreSQL日志
docker logs sso-postgres -f

# 查看Redis日志
docker logs sso-redis -f
```

### Prometheus监控

如果需要更详细的监控，可以添加Prometheus和Grafana：

```yaml
# 添加到docker-compose.yml
services:
  prometheus:
    image: prom/prometheus:latest
    container_name: sso-prometheus
    restart: unless-stopped
    ports:
      - "9091:9090"
    volumes:
      - /mnt/pool/sso/prometheus/prometheus.yml:/etc/prometheus/prometheus.yml:ro
      - /mnt/pool/sso/prometheus/data:/prometheus
    networks:
      - sso-network

  grafana:
    image: grafana/grafana:latest
    container_name: sso-grafana
    restart: unless-stopped
    ports:
      - "3000:3000"
    volumes:
      - /mnt/pool/sso/grafana:/var/lib/grafana
    environment:
      - GF_SECURITY_ADMIN_PASSWORD=admin
    networks:
      - sso-network
```

Prometheus配置：

```yaml
# /mnt/pool/sso/prometheus/prometheus.yml
global:
  scrape_interval: 15s

scrape_configs:
  - job_name: 'sso'
    static_configs:
      - targets: ['sso:9090']
    metrics_path: '/metrics'
```

---

## 故障排查

### 常见问题

**1. 容器无法启动**

```bash
# 查看容器日志
docker logs sso-app

# 检查容器状态
docker inspect sso-app
```

**2. 数据库连接失败**

```bash
# 检查PostgreSQL是否运行
docker ps | grep postgres

# 测试数据库连接
docker exec sso-postgres pg_isready -U sso -d sso

# 查看PostgreSQL日志
docker logs sso-postgres
```

**3. Redis连接失败**

```bash
# 检查Redis是否运行
docker ps | grep redis

# 测试Redis连接
docker exec sso-redis redis-cli ping
```

**4. 端口冲突**

```bash
# 检查端口占用
netstat -tlnp | grep 9090

# 修改docker-compose.yml中的端口映射
ports:
  - "9091:9090"  # 改用其他端口
```

**5. 权限问题**

```bash
# 检查文件权限
ls -la /mnt/pool/sso/keys/

# 修复权限
chmod 600 /mnt/pool/sso/keys/private.pem
chmod 644 /mnt/pool/sso/keys/public.pem
```

### 重置部署

```bash
# 停止并删除所有容器
cd /mnt/pool/sso
docker compose down

# 删除 SSO 应用数据（谨慎操作！）
rm -rf /mnt/pool/sso/config/*

# 重新启动
docker compose up -d

# 重新运行迁移
docker exec sso-app migrate -path /app/migrations -database "postgres://sso:YourStrongPassword123!@your-postgres-host:5432/sso?sslmode=disable" up
```

---

## 安全建议

1. **修改默认密码**
   - 数据库密码
   - Redis密码（如启用）

2. **限制网络访问**
   - 仅暴露必要的端口
   - 使用防火墙规则限制访问

3. **启用HTTPS**
   - 配置SSL证书
   - 强制HTTPS重定向

4. **定期更新**
   - 更新容器镜像
   - 更新TrueNAS系统

5. **备份数据**
   - 定期备份数据库
   - 备份配置文件和密钥

---

## 快速命令参考

```bash
# 启动服务
docker compose -f /mnt/pool/sso/docker-compose.yml up -d

# 停止服务
docker compose -f /mnt/pool/sso/docker-compose.yml down

# 查看状态
docker compose -f /mnt/pool/sso/docker-compose.yml ps

# 查看日志
docker compose -f /mnt/pool/sso/docker-compose.yml logs -f

# 重启服务
docker compose -f /mnt/pool/sso/docker-compose.yml restart

# 更新镜像
docker compose -f /mnt/pool/sso/docker-compose.yml pull
docker compose -f /mnt/pool/sso/docker-compose.yml up -d

# 运行数据库迁移
docker exec sso-app migrate -path /app/migrations -database "postgres://sso:PASSWORD@your-postgres-host:5432/sso?sslmode=disable" up

# 备份数据库（替换 your-postgres-container 为您的 PostgreSQL 容器名称）
docker exec your-postgres-container pg_dump -U sso sso | gzip > backup.sql.gz

# 进入容器
docker exec -it sso-app sh
# 进入 PostgreSQL（替换 your-postgres-container 为您的 PostgreSQL 容器名称）
docker exec -it your-postgres-container psql -U sso sso
# 进入 Redis（替换 your-redis-container 为您的 Redis 容器名称）
docker exec -it your-redis-container redis-cli
```
