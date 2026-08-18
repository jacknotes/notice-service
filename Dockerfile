# 阶段1：构建前端
FROM node:20-alpine AS web
WORKDIR /app
COPY web/package.json web/package-lock.json ./
RUN npm install
COPY web/ ./
RUN npm run build

# 阶段2：构建后端
FROM golang:1.25-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
COPY --from=web /app/dist ./web/dist
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
