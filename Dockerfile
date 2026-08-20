# 镜像前缀：默认空 = 官方 Docker Hub；国内网络可设为
# docker.m.daocloud.io/library/ 等镜像源（docker build --build-arg IMAGE_PREFIX=...）
ARG IMAGE_PREFIX=""

# 阶段1：构建前端（npm 走镜像 + 缓存）
FROM ${IMAGE_PREFIX}node:20-alpine AS web
ARG NPM_REGISTRY=https://registry.npmmirror.com
ENV NPM_CONFIG_REGISTRY=${NPM_REGISTRY}
WORKDIR /app
COPY web/package.json web/package-lock.json ./
RUN npm install
COPY web/ ./
RUN npm run build

# 阶段2：构建后端（Go 走镜像 + 缓存）
FROM ${IMAGE_PREFIX}golang:1.25-alpine AS build
ARG GOPROXY=https://goproxy.cn,direct
ENV GOPROXY=${GOPROXY} GOFLAGS=-mod=mod
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
COPY --from=web /app/dist ./web/dist
RUN go install github.com/swaggo/swag/cmd/swag@v1.16.6
RUN swag init -g cmd/server/main.go -o docs/swagger
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /notice-service ./cmd/server

# 阶段3：运行
FROM ${IMAGE_PREFIX}alpine:3.18
RUN apk add --no-cache ca-certificates tzdata
WORKDIR /app
COPY --from=build /notice-service /app/notice-service
COPY --from=web /app/dist /app/web/dist
EXPOSE 8080
# 健康检查：依赖 /api/health（含 DB 探测），容器/编排据此摘除故障实例。
# busybox 自带 wget，无需额外安装。
HEALTHCHECK --interval=30s --timeout=5s --start-period=15s --retries=3 \
  CMD wget -q -O /dev/null http://127.0.0.1:8080/api/health || exit 1
CMD ["/app/notice-service"]
