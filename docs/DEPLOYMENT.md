# 部署指南

本文档介绍如何将SSO服务部署到生产环境。

## 部署方式

- [Docker Compose部署](#docker-compose部署)（推荐）
- [Kubernetes部署](#kubernetes部署)
- [裸机部署](#裸机部署)

---

## Docker Compose部署

### 前置要求

- Docker 20.10+
- Docker Compose 2.0+
- 2GB+ 可用内存
- 10GB+ 可用磁盘空间

### 快速部署

1. **克隆代码**

```bash
git clone <repo-url>
cd sso
```

2. **配置环境变量**

```bash
cp .env.example .env
```

编辑 `.env` 文件：

```bash
# 生产环境配置
SERVER_ENV=production
SERVER_HOST=0.0.0.0
SERVER_PORT=9090

# 数据库配置（使用强密码）
DB_HOST=postgres
DB_PORT=5432
DB_NAME=sso
DB_USER=sso
DB_PASSWORD=your_strong_password_here
DB_SSL_MODE=require

# 数据库连接池配置
DB_MAX_OPEN_CONNS=100
DB_MAX_IDLE_CONNS=50
DB_CONN_MAX_LIFETIME=5m
DB_CONN_MAX_IDLE_TIME=1m
DB_QUERY_TIMEOUT=10s

# Redis配置
REDIS_ENABLE=true
REDIS_HOST=redis
REDIS_PORT=6379
REDIS_PASSWORD=your_redis_password
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

# 邮件配置（代码默认 587/STARTTLS，如使用 SSL 改为 465）
SMTP_HOST=smtp.example.com
SMTP_PORT=587
SMTP_USER=your_smtp_username
SMTP_PASSWORD=your_smtp_password
SMTP_FROM=noreply@yourdomain.com

# 优雅关闭配置
SHUTDOWN_TIMEOUT=30s

# Metrics配置 (Prometheus指标端点认证)
METRICS_USERNAME=your_metrics_username
METRICS_PASSWORD=your_strong_metrics_password

# CORS配置
CORS_ALLOWED_ORIGINS=https://yourdomain.com
```

3. **生成RSA密钥**

```bash
make generate-keys
```

或手动生成：

```bash
mkdir -p keys
openssl genrsa -out keys/private.pem 2048
openssl rsa -in keys/private.pem -pubout -out keys/public.pem
chmod 600 keys/private.pem
```

4. **启动服务**

```bash
docker-compose -f docker/docker-compose.yml up -d
```

5. **运行数据库迁移**

> 容器 entrypoint 默认已自动执行迁移（`AUTO_MIGRATE=true`），正常启动后此步骤可跳过。
> 仅在设置了 `AUTO_MIGRATE=false` 或需要手动控制迁移时机时执行以下命令：

```bash
# 使用环境变量 DATABASE_URL
export DATABASE_URL='postgres://sso:your_strong_password_here@postgres:5432/sso?sslmode=require'
docker compose -f docker/docker-compose.yml exec sso \
  migrate -path /app/migrations -database "$DATABASE_URL" up
```

或使用 Makefile（需要在宿主机安装 migrate 工具，且数据库端口映射到宿主机）：
```bash
export DATABASE_URL='postgres://sso:your_strong_password_here@localhost:5432/sso?sslmode=require'
make migrate-up
```

6. **验证部署**

```bash
curl http://localhost:9090/health
```

### Docker Compose配置说明

完整且唯一权威的配置请直接参考仓库中的 `docker/docker-compose.yml`（本文不复制全文，避免与源文件脱节）。需要注意的关键点：

- **端口仅绑定回环地址**：`127.0.0.1:9090/5432/6379`，外部访问必须经反向代理
- **强制密码**：`DB_PASSWORD` 与 `REDIS_PASSWORD` 必须作为环境变量提供，未设置时 compose 直接报错，不会以空密码启动
- **`DB_SSL_MODE=prefer`**：容器间内网通信默认 prefer；生产环境请在 `.env` 中改为 `require`
- **最小权限运行**：SSO 容器启用 `read_only`、`cap_drop: ALL`、`no-new-privileges`，并限制内存/CPU 与日志大小
- **自动迁移**：容器 entrypoint 默认自动执行数据库迁移（`AUTO_MIGRATE=true`），密码仅通过 `PGPASSWORD` 注入 migrate 子进程，不出现在命令行或容器环境中
- Redis 密码通过 `$$REDIS_PASSWORD` 在容器内运行时展开，不明文固化在容器配置中

生产部署时请在 `.env` 中覆盖 compose 里的开发默认值（`SERVER_ENV=production`、`DB_SSL_MODE=require`、`JWT_ISSUER`、
`MFA_RECOVERY_HMAC_KEY`、`CORS_ALLOWED_ORIGINS`、`SMTP_*`、`METRICS_*` 等，见上文快速部署一节）。

---

## Kubernetes部署

### 前置要求

- Kubernetes 1.24+
- kubectl 已配置
- Helm 3.0+（可选）

### 部署步骤

1. **创建Namespace**

```bash
kubectl create namespace sso
```

2. **创建Secret**

```bash
# 数据库 / Redis 密码与 MFA HMAC 密钥（一个 Secret 集中管理）
kubectl create secret generic sso-db-secret \
  --namespace sso \
  --from-literal=password=your_strong_password \
  --from-literal=redis-password=your_redis_password \
  --from-literal=mfa-hmac-key=$(openssl rand -hex 32)

# JWT密钥
kubectl create secret generic sso-jwt-secret \
  --namespace sso \
  --from-file=private.pem=keys/private.pem \
  --from-file=public.pem=keys/public.pem
```

3. **部署PostgreSQL**

```yaml
# k8s/postgres.yaml
apiVersion: apps/v1
kind: StatefulSet
metadata:
  name: postgres
  namespace: sso
spec:
  serviceName: postgres
  replicas: 1
  selector:
    matchLabels:
      app: postgres
  template:
    metadata:
      labels:
        app: postgres
    spec:
      containers:
      - name: postgres
        image: postgres:15-alpine
        env:
        - name: POSTGRES_DB
          value: sso
        - name: POSTGRES_USER
          value: sso
        - name: POSTGRES_PASSWORD
          valueFrom:
            secretKeyRef:
              name: sso-db-secret
              key: password
        ports:
        - containerPort: 5432
        volumeMounts:
        - name: postgres-data
          mountPath: /var/lib/postgresql/data
  volumeClaimTemplates:
  - metadata:
      name: postgres-data
    spec:
      accessModes: ["ReadWriteOnce"]
      resources:
        requests:
          storage: 10Gi
---
apiVersion: v1
kind: Service
metadata:
  name: postgres
  namespace: sso
spec:
  selector:
    app: postgres
  ports:
  - port: 5432
  clusterIP: None
```

4. **部署Redis**

```yaml
# k8s/redis.yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: redis
  namespace: sso
spec:
  replicas: 1
  selector:
    matchLabels:
      app: redis
  template:
    metadata:
      labels:
        app: redis
    spec:
      containers:
      - name: redis
        image: redis:7-alpine
        # 密码经环境变量传入，与 Docker Compose 部署保持一致的安全基线
        args: ["sh", "-c", "exec redis-server --appendonly yes --requirepass \"$REDIS_PASSWORD\""]
        env:
        - name: REDIS_PASSWORD
          valueFrom:
            secretKeyRef:
              name: sso-db-secret
              key: redis-password
        ports:
        - containerPort: 6379
        volumeMounts:
        - name: redis-data
          mountPath: /data
      volumes:
      # 持久化存储（emptyDir 会在 Pod 重建时丢失全部缓存数据，禁止使用）
      # 需另行创建 redis-data PVC，或改用 StatefulSet + volumeClaimTemplates
      - name: redis-data
        persistentVolumeClaim:
          claimName: redis-data
---
apiVersion: v1
kind: Service
metadata:
  name: redis
  namespace: sso
spec:
  selector:
    app: redis
  ports:
  - port: 6379
```

5. **部署SSO服务**

```yaml
# k8s/sso.yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: sso
  namespace: sso
spec:
  replicas: 3
  selector:
    matchLabels:
      app: sso
  template:
    metadata:
      labels:
        app: sso
    spec:
      containers:
      - name: sso
        image: your-registry/sso:latest
        ports:
        - containerPort: 9090
        env:
        - name: SERVER_ENV
          value: production
        - name: DB_HOST
          value: postgres
        - name: DB_PASSWORD
          valueFrom:
            secretKeyRef:
              name: sso-db-secret
              key: password
        # 生产环境配置校验要求以下项，缺失时服务拒绝启动
        - name: DB_SSL_MODE
          value: require
        - name: JWT_ISSUER
          value: sso.yourdomain.com  # 不能使用默认值 sso
        - name: CORS_ALLOWED_ORIGINS
          value: https://yourdomain.com  # 不能为 * 或 localhost
        - name: MFA_RECOVERY_HMAC_KEY  # >= 32 字节强随机密钥
          valueFrom:
            secretKeyRef:
              name: sso-db-secret
              key: mfa-hmac-key
        - name: SMTP_HOST  # 不能为 localhost
          value: smtp.example.com
        - name: REDIS_HOST
          value: redis
        - name: REDIS_PASSWORD
          valueFrom:
            secretKeyRef:
              name: sso-db-secret
              key: redis-password
        - name: JWT_PRIVATE_KEY_PATH
          value: /app/keys/private.pem
        - name: JWT_PUBLIC_KEY_PATH
          value: /app/keys/public.pem
        volumeMounts:
        - name: jwt-keys
          mountPath: /app/keys
          readOnly: true
        livenessProbe:
          httpGet:
            path: /healthz
            port: 9090
          initialDelaySeconds: 10
          periodSeconds: 30
        readinessProbe:
          httpGet:
            path: /readyz
            port: 9090
          initialDelaySeconds: 5
          periodSeconds: 10
        resources:
          requests:
            memory: "128Mi"
            cpu: "100m"
          limits:
            memory: "256Mi"
            cpu: "500m"
      volumes:
      - name: jwt-keys
        secret:
          secretName: sso-jwt-secret
---
apiVersion: v1
kind: Service
metadata:
  name: sso
  namespace: sso
spec:
  selector:
    app: sso
  ports:
  - port: 9090
    targetPort: 9090
  type: ClusterIP
---
apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  name: sso-ingress
  namespace: sso
  annotations:
    cert-manager.io/cluster-issuer: letsencrypt-prod
spec:
  ingressClassName: nginx
  tls:
  - hosts:
    - sso.yourdomain.com
    secretName: sso-tls
  rules:
  - host: sso.yourdomain.com
    http:
      paths:
      - path: /
        pathType: Prefix
        backend:
          service:
            name: sso
            port:
              number: 9090
```

6. **应用配置**

> 仓库中不包含 `k8s/` 目录，请先将上述 YAML 示例分别保存为
> `k8s/postgres.yaml`、`k8s/redis.yaml`、`k8s/sso.yaml`，并为 Redis 创建
> `redis-data` PVC 后再执行：

```bash
kubectl apply -f k8s/postgres.yaml
kubectl apply -f k8s/redis.yaml
kubectl apply -f k8s/sso.yaml
```

7. **验证部署**

```bash
kubectl get pods -n sso
kubectl get svc -n sso
```

---

## 裸机部署

### 前置要求

- Linux服务器（Ubuntu 20.04+ / CentOS 8+）
- PostgreSQL 15+
- Redis 7+
- systemd（用于服务管理）

### 部署步骤

1. **安装依赖**

```bash
# Ubuntu/Debian
sudo apt update
sudo apt install -y postgresql redis-server

# CentOS/RHEL
sudo yum install -y postgresql-server redis
```

2. **配置PostgreSQL**

```bash
sudo -u postgres createuser sso
sudo -u postgres createdb sso -O sso
sudo -u postgres psql -c "ALTER USER sso WITH PASSWORD 'your_password';"
```

3. **构建应用**

```bash
# 使用 Makefile 构建（推荐，自动注入版本信息）
make build

# 或手动构建（不包含版本信息）
go build -o ./bin/sso cmd/server/main.go
```

4. **创建系统用户**

```bash
sudo useradd -r -s /bin/false sso
sudo mkdir -p /opt/sso/keys
sudo chown -R sso:sso /opt/sso
```

5. **复制文件**

```bash
sudo cp ./bin/sso /opt/sso/
sudo cp keys/*.pem /opt/sso/keys/
sudo cp .env /opt/sso/
# 数据库迁移所需的工具与脚本（步骤8会用到）
sudo cp -r migrations /opt/sso/
sudo cp "$(command -v migrate)" /opt/sso/migrate 2>/dev/null || true
sudo chmod 600 /opt/sso/keys/private.pem
```

6. **创建systemd服务**

```ini
# /etc/systemd/system/sso.service
[Unit]
Description=SSO Service
After=network.target postgresql.service redis.service

[Service]
Type=simple
User=sso
Group=sso
WorkingDirectory=/opt/sso
ExecStart=/opt/sso/sso
Restart=always
RestartSec=5
EnvironmentFile=/opt/sso/.env

# 安全配置
NoNewPrivileges=true
PrivateTmp=true
ProtectSystem=strict
ReadWritePaths=/opt/sso

[Install]
WantedBy=multi-user.target
```

7. **启动服务**

```bash
sudo systemctl daemon-reload
sudo systemctl enable sso
sudo systemctl start sso
sudo systemctl status sso
```

8. **运行数据库迁移**

```bash
cd /opt/sso
# 若步骤5未能复制 migrate（宿主机未安装），请先安装 golang-migrate
sudo chmod +x /opt/sso/migrate 2>/dev/null || true
export DATABASE_URL='postgres://sso:your_password@localhost:5432/sso?sslmode=require'
./migrate -path ./migrations -database "$DATABASE_URL" up
```

---

## 反向代理配置

### Nginx

```nginx
# /etc/nginx/sites-available/sso
upstream sso_backend {
    server 127.0.0.1:9090;
    # 如果有多个实例
    # server 127.0.0.1:9091;
}

server {
    listen 80;
    server_name sso.yourdomain.com;
    return 301 https://$server_name$request_uri;
}

server {
    listen 443 ssl http2;
    server_name sso.yourdomain.com;

    ssl_certificate /etc/letsencrypt/live/sso.yourdomain.com/fullchain.pem;
    ssl_certificate_key /etc/letsencrypt/live/sso.yourdomain.com/privkey.pem;
    ssl_protocols TLSv1.2 TLSv1.3;
    ssl_ciphers HIGH:!aNULL:!MD5;

    # 安全头
    add_header X-Frame-Options "SAMEORIGIN" always;
    add_header X-Content-Type-Options "nosniff" always;
    add_header Strict-Transport-Security "max-age=31536000; includeSubDomains" always;

    # 请求大小限制
    client_max_body_size 1m;

    location / {
        proxy_pass http://sso_backend;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;

        # 超时配置
        proxy_connect_timeout 60s;
        proxy_send_timeout 60s;
        proxy_read_timeout 60s;
    }

    # 健康检查（不记录日志）
    location /health {
        proxy_pass http://sso_backend;
        access_log off;
    }

    # 指标端点（限制访问）
    location /metrics {
        allow 10.0.0.0/8;
        allow 172.16.0.0/12;
        allow 192.168.0.0/16;
        deny all;
        proxy_pass http://sso_backend;
    }
}
```

### Caddy

```caddyfile
# Caddyfile
sso.yourdomain.com {
    reverse_proxy localhost:9090

    header {
        X-Frame-Options "SAMEORIGIN"
        X-Content-Type-Options "nosniff"
        Strict-Transport-Security "max-age=31536000; includeSubDomains"
    }

    @metrics path /metrics
    handle @metrics {
        @blocked not remote_ip 10.0.0.0/8 172.16.0.0/12 192.168.0.0/16
        respond @blocked 403
        reverse_proxy localhost:9090
    }
}
```

---

## 监控配置

### Prometheus

```yaml
# prometheus.yml
scrape_configs:
  - job_name: 'sso'
    static_configs:
      - targets: ['sso:9090']
    metrics_path: '/metrics'
    scrape_interval: 15s
    # 如果配置了Metrics Basic Auth，需要添加以下配置
    basic_auth:
      username: '${METRICS_USERNAME}'
      password: '${METRICS_PASSWORD}'
```

### Grafana Dashboard

导入JSON dashboard或创建自定义面板，监控以下指标：

- `http_requests_total` - 请求总数
- `http_request_duration_seconds` - 请求延迟
- `auth_login_total` - 登录次数
- `auth_login_failed_total` - 登录失败次数
- `auth_register_total` - 注册次数

---

## 备份与恢复

### 数据库备份

```bash
# 全量备份
pg_dump -U sso -h localhost sso > backup_$(date +%Y%m%d_%H%M%S).sql

# 压缩备份
pg_dump -U sso -h localhost sso | gzip > backup_$(date +%Y%m%d_%H%M%S).sql.gz

# 自动备份脚本（写入 cron 所引用的路径 /opt/sso/scripts/backup.sh）
sudo mkdir -p /opt/sso/scripts
sudo tee /opt/sso/scripts/backup.sh > /dev/null << 'EOF'
#!/bin/bash
BACKUP_DIR="/backup/sso"
mkdir -p $BACKUP_DIR
pg_dump -U sso -h localhost sso | gzip > $BACKUP_DIR/sso_$(date +%Y%m%d_%H%M%S).sql.gz
find $BACKUP_DIR -name "*.sql.gz" -mtime +30 -delete
EOF
sudo chmod +x /opt/sso/scripts/backup.sh
```

### 数据库恢复

```bash
# 从SQL文件恢复
psql -U sso -h localhost sso < backup.sql

# 从压缩文件恢复
gunzip -c backup.sql.gz | psql -U sso -h localhost sso
```

### 自动备份（Cron）

```bash
# 添加到crontab
0 2 * * * /opt/sso/scripts/backup.sh
```

---

## 故障排查

### 常见问题

**服务无法启动**

```bash
# 查看日志
docker compose -f docker/docker-compose.yml logs sso
journalctl -u sso -f

# 检查配置
cat /opt/sso/.env
```

**数据库连接失败**

```bash
# 检查PostgreSQL状态
systemctl status postgresql

# 测试连接
psql -U sso -h localhost -d sso
```

**Redis连接失败**

```bash
# 检查Redis状态
systemctl status redis

# 测试连接
redis-cli ping
```

### 健康检查

```bash
# 服务健康
curl http://localhost:9090/health

# 管理员健康检查（需要认证）
curl -H "Authorization: Bearer <token>" http://localhost:9090/api/v1/admin/health
```

---

## 性能调优

### 数据库优化

```sql
-- PostgreSQL配置优化
ALTER SYSTEM SET shared_buffers = '256MB';
ALTER SYSTEM SET effective_cache_size = '768MB';
ALTER SYSTEM SET work_mem = '4MB';
ALTER SYSTEM SET maintenance_work_mem = '64MB';
SELECT pg_reload_conf();
```

### 应用配置

```bash
# 连接池配置
DB_MAX_OPEN_CONNS=100
DB_MAX_IDLE_CONNS=50
DB_CONN_MAX_LIFETIME=5m

# bcrypt成本（生产环境建议12-14）
BCRYPT_COST=12
```

### 系统配置

```bash
# 增加文件描述符限制
echo "* soft nofile 65535" >> /etc/security/limits.conf
echo "* hard nofile 65535" >> /etc/security/limits.conf
```
