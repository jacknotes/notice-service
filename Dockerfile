# 阶段1：构建前端（npm 走镜像 + 缓存）
FROM node:20-alpine AS web
ARG NPM_REGISTRY=https://registry.npmmirror.com
ENV NPM_CONFIG_REGISTRY=${NPM_REGISTRY}
WORKDIR /app
COPY web/package.json web/package-lock.json ./
RUN --mount=type=cache,target=/root/.npm npm install
COPY web/ ./
RUN npm run build

# 阶段2：构建后端（Go 走镜像 + 缓存）
FROM golang:1.25-alpine AS build
ARG GOPROXY=https://goproxy.cn,direct
ENV GOPROXY=${GOPROXY} GOFLAGS=-mod=mod
WORKDIR /src
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    go mod download
COPY . .
COPY --from=web /app/dist ./web/dist
RUN go install github.com/swaggo/swag/cmd/swag@v1.16.6
RUN swag init -g cmd/server/main.go -o docs/swagger
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /notice-service ./cmd/server

# 阶段3：运行
FROM alpine:3.18
RUN apk add --no-cache ca-certificates tzdata
WORKDIR /app
COPY --from=build /notice-service /app/notice-service
COPY --from=web /app/dist /app/web/dist
COPY migrations/ /app/migrations/
EXPOSE 8080
CMD ["/app/notice-service"]
