# Notice Service · 生命周期管理 Makefile
# 用法：make <target>（运行 `make help` 查看所有目标）

SHELL := /bin/bash
BIN    := .dev/notice-service
WEB    := web
PORT   := 8080
GO_ENV := GOCACHE=$(CURDIR)/.dev/go-cache GOMODCACHE=$(CURDIR)/.dev/gomodcache GOPATH=/tmp/dsh-gopath

.PHONY: help deps build run dev dev-backend prod-backend test vet fmt swagger frontend-install frontend-build frontend-dev \
        docker-build docker-up docker-down docker-logs db-clean clean

help: ## 显示所有命令
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-18s\033[0m %s\n", $$1, $$2}'

## ---------- 后端 ----------

deps: frontend-install ## 安装 Go + 前端依赖
	$(GO_ENV) go mod download

swagger: ## 重新生成 Swagger 文档
	$(GO_ENV) go run github.com/swaggo/swag/cmd/swag@v1.16.6 init -g cmd/server/main.go -o docs/swagger

build: swagger ## 编译后端（静态；先生成 swagger，避免 docs 缺失编译失败）
	$(GO_ENV) CGO_ENABLED=0 go build -o $(BIN) ./cmd/server

run: build ## 编译并启动后端（:$(PORT)，默认 release 模式）
	PORT=$(PORT) $(BIN)

dev: ## 本地开发：后端 + 前端 dev server（:8080 + :5173，/api 自动代理）
	$(MAKE) -j2 run frontend-dev

dev-backend: build ## 开发后端（GIN_MODE=debug，:$(PORT)）
	GIN_MODE=debug PORT=$(PORT) $(BIN)

prod-backend: build ## 生产后端（GIN_MODE=release，:$(PORT)）
	GIN_MODE=release PORT=$(PORT) $(BIN)

test: ## 运行全部 Go 测试（使用独立测试库 notice_service_test；-p 1 串行化包避免共享库跨包干扰）
	$(GO_ENV) go test -p 1 ./... -count=1

vet: ## 静态检查
	$(GO_ENV) go vet ./...

fmt: ## 格式化 Go 代码
	gofmt -w $$(find . -name '*.go' -not -path './.dev/*' -not -path './node_modules/*')

## ---------- 前端 ----------

frontend-install: ## 安装前端依赖
	cd $(WEB) && npm --cache $(CURDIR)/.dev/npm-cache install

frontend-build: ## 构建前端产物到 web/dist
	cd $(WEB) && npm --cache $(CURDIR)/.dev/npm-cache run build

frontend-dev: ## 启动前端 dev server（:5173，热更新）
	cd $(WEB) && npm --cache $(CURDIR)/.dev/npm-cache run dev

## ---------- Docker ----------

docker-build: ## 构建 Docker 镜像
	docker build -t notice-service .

docker-up: ## 启动多实例高可用部署（2 实例 + MySQL 5.7）
	docker compose up -d

docker-down: ## 停止部署
	docker compose down

docker-logs: ## 查看部署日志
	docker compose logs -f

## ---------- 运维 ----------

db-clean: ## 清空真实库与测试库数据（危险：会删除所有数据）
	@echo "危险操作：将删除 notice_service 与 notice_service_test 的全部数据，确认请运行: make db-clean FORCE=1"
	@[ "$(FORCE)" = "1" ] && (mysql --socket=$(CURDIR)/.dev/mysql-run/mysqld.sock -u root -e "DROP DATABASE IF EXISTS notice_service; DROP DATABASE IF EXISTS notice_service_test; CREATE DATABASE notice_service CHARACTER SET utf8mb4; CREATE DATABASE notice_service_test CHARACTER SET utf8mb4; GRANT ALL ON notice_service.* TO 'notice'@'%'; GRANT ALL ON notice_service_test.* TO 'notice'@'%';") || echo "已取消"

clean: ## 清理构建产物（含生成的 Swagger 文档）
	rm -rf $(BIN) $(WEB)/dist $(WEB)/node_modules docs/swagger
