# ============================================================================
# SSO服务 Dockerfile (生产环境)
# 使用多阶段构建减小镜像体积
# ============================================================================

# --------------------------------------------------------------------------
# 阶段1: 构建
# --------------------------------------------------------------------------
FROM golang:1.26.6-alpine AS builder

# Alpine 镜像源（默认阿里云，海外构建可通过 --build-arg APK_MIRROR=dl-cdn.alpinelinux.org 覆盖）
ARG APK_MIRROR=mirrors.aliyun.com
RUN sed -i "s/dl-cdn.alpinelinux.org/${APK_MIRROR}/g" /etc/apk/repositories

# 安装必要的构建工具
RUN apk add --no-cache git gcc musl-dev

# 设置工作目录
WORKDIR /build

# 先复制依赖文件，利用Docker缓存
COPY go.mod go.sum ./
RUN go mod download

# 安装数据库迁移工具（T12：固定版本，供应链安全）
RUN go install -tags 'postgres' github.com/golang-migrate/migrate/v4/cmd/migrate@v4.19.1

# 复制源代码
COPY . .

# 构建二进制文件
# CGO_ENABLED=0: 静态链接，减小依赖
# GOOS=linux: 目标平台Linux
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /build/sso cmd/server/main.go

# --------------------------------------------------------------------------
# 阶段2: 运行
# --------------------------------------------------------------------------
# alpine:3.19 已停止维护，升级至当前稳定版（与镜像源参数保持一致）
FROM alpine:3.22

ARG APK_MIRROR=mirrors.aliyun.com
RUN sed -i "s/dl-cdn.alpinelinux.org/${APK_MIRROR}/g" /etc/apk/repositories

# 安装运行时依赖
RUN apk add --no-cache ca-certificates curl tzdata

# 设置时区
ENV TZ=Asia/Shanghai

# 创建非root用户
RUN addgroup -g 1000 sso && \
    adduser -u 1000 -G sso -s /bin/sh -D sso

# 创建应用目录
WORKDIR /app

# 从构建阶段复制二进制文件和迁移工具
COPY --from=builder /build/sso /app/sso
COPY --from=builder /go/bin/migrate /usr/local/bin/migrate
COPY --from=builder /build/migrations /app/migrations

# 创建必要的目录
RUN mkdir -p /app/keys && chown -R sso:sso /app

# 复制并设置entrypoint
COPY docker/entrypoint.sh /app/entrypoint.sh
USER root
RUN chmod +x /app/entrypoint.sh
USER sso

# 暴露端口
EXPOSE 9090

# 健康检查
HEALTHCHECK --interval=30s --timeout=10s --start-period=10s --retries=3 \
    CMD curl -f http://localhost:9090/health || exit 1

# 启动命令（通过entrypoint自动迁移后启动）
ENTRYPOINT ["/app/entrypoint.sh"]
