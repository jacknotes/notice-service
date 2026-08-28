ARG IMAGE_PREFIX=""

# 阶段1：构建前端
FROM ${IMAGE_PREFIX}node:20-alpine AS web
ARG NPM_REGISTRY="https://registry.npmmirror.com"
ENV NPM_CONFIG_REGISTRY=${NPM_REGISTRY}
WORKDIR /app
COPY web/package.json web/package-lock.json ./
RUN npm ci
COPY web/ ./
RUN npm run build

# 阶段2：构建后端
FROM ${IMAGE_PREFIX}golang:1.25-alpine AS build
ARG BUILD_VERSION=dev
ARG GOPROXY="https://goproxy.cn,direct"
ENV GOPROXY=${GOPROXY} GOFLAGS=-mod=mod
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
# swag 不依赖源码，放在 COPY . . 之前，改代码时不再重复安装
RUN go install github.com/swaggo/swag/cmd/swag@v1.16.6
COPY . .
COPY --from=web /app/dist ./web/dist
RUN swag init -g cmd/server/main.go -o docs/swagger
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w -X main.buildVersion=${BUILD_VERSION}" -o /notice-service ./cmd/server

# 阶段3：运行（3.18 已 EOL，升级到 3.21）
FROM ${IMAGE_PREFIX}alpine:3.21
# 替换 apk 源为阿里云镜像（仅 main + community，与官方源结构一致）
RUN sed -i 's#https://dl-cdn.alpinelinux.org#https://mirrors.aliyun.com#g' /etc/apk/repositories \
    && apk add --no-cache ca-certificates tzdata
WORKDIR /app
COPY --from=build /notice-service /app/notice-service
COPY --from=web /app/dist /app/web/dist
EXPOSE 8080
HEALTHCHECK --interval=30s --timeout=5s --start-period=15s --retries=3 \
  CMD wget -q -O /dev/null http://127.0.0.1:8080/api/health || exit 1
CMD ["/app/notice-service"]
