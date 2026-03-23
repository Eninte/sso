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
DB_SSL_MODE=disable

# Redis配置
REDIS_HOST=redis
REDIS_PORT=6379
REDIS_PASSWORD=your_redis_password

# JWT配置
JWT_PRIVATE_KEY_PATH=/app/keys/private.pem
JWT_PUBLIC_KEY_PATH=/app/keys/public.pem
JWT_ACCESS_TOKEN_TTL=15m
JWT_REFRESH_TOKEN_TTL=168h
JWT_ISSUER=sso

# 安全配置
BCRYPT_COST=14
RATE_LIMIT_REQUESTS=50
RATE_LIMIT_WINDOW=1m
MAX_LOGIN_ATTEMPTS=5
LOCKOUT_DURATION=30m

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

```bash
docker-compose -f docker/docker-compose.yml exec sso \
  migrate -path /app/migrations -database "postgres://sso:your_password@postgres:5432/sso?sslmode=disable" up
```

6. **验证部署**

```bash
curl http://localhost:9090/health
```

### Docker Compose配置说明

```yaml
# docker/docker-compose.yml

services:
  sso:
    build:
      context: ..
      dockerfile: docker/Dockerfile
    ports:
      - "9090:9090"
    environment:
      # 环境变量配置
    volumes:
      - ../keys:/app/keys:ro  # 挂载密钥（只读）
    depends_on:
      postgres:
        condition: service_healthy
      redis:
        condition: service_healthy
    restart: unless-stopped
    healthcheck:
      test: ["CMD", "curl", "-f", "http://localhost:9090/health"]
      interval: 30s
      timeout: 10s
      retries: 3
```

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
# 数据库密码
kubectl create secret generic sso-db-secret \
  --namespace sso \
  --from-literal=password=your_strong_password

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
        ports:
        - containerPort: 6379
        volumeMounts:
        - name: redis-data
          mountPath: /data
      volumes:
      - name: redis-data
        emptyDir: {}
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
        - name: REDIS_HOST
          value: redis
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
            path: /health
            port: 9090
          initialDelaySeconds: 10
          periodSeconds: 30
        readinessProbe:
          httpGet:
            path: /health
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
    kubernetes.io/ingress.class: nginx
    cert-manager.io/cluster-issuer: letsencrypt-prod
spec:
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
go build -o sso cmd/server/main.go
```

4. **创建系统用户**

```bash
sudo useradd -r -s /bin/false sso
sudo mkdir -p /opt/sso/keys
sudo chown -R sso:sso /opt/sso
```

5. **复制文件**

```bash
sudo cp sso /opt/sso/
sudo cp keys/*.pem /opt/sso/keys/
sudo cp .env /opt/sso/
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
export DATABASE_URL='postgres://sso:your_password@localhost:5432/sso?sslmode=disable'
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
    add_header X-XSS-Protection "1; mode=block" always;
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
        X-XSS-Protection "1; mode=block"
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

# 自动备份脚本
#!/bin/bash
BACKUP_DIR="/backup/sso"
mkdir -p $BACKUP_DIR
pg_dump -U sso -h localhost sso | gzip > $BACKUP_DIR/sso_$(date +%Y%m%d_%H%M%S).sql.gz
find $BACKUP_DIR -name "*.sql.gz" -mtime +30 -delete
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
docker-compose logs sso
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
curl -H "Authorization: Bearer <token>" http://localhost:9090/admin/health
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
DB_MAX_OPEN_CONNS=50
DB_MAX_IDLE_CONNS=25
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
