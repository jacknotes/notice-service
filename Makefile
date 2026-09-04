# Notice Service · 生命周期管理 Makefile
# 用法：make <target>（运行 `make help` 查看所有目标）

SHELL := /bin/bash
BIN    := .dev/notice-service
WEB    := web
PORT   := 8080
GO_ENV := GOCACHE=$(CURDIR)/.dev/go-cache GOMODCACHE=$(CURDIR)/.dev/gomodcache GOPATH=/tmp/dsh-gopath

.PHONY: help deps build run dev dev-backend prod-backend test vet fmt swagger frontend-install frontend-build frontend-dev \
        docker-build docker-up docker-down docker-logs db-clean db-start db-stop db-status clean

help: ## 显示所有命令
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-18s\033[0m %s\n", $$1, $$2}'

## ---------- 后端 ----------

deps: frontend-install ## 安装 Go + 前端依赖
	$(GO_ENV) go mod download

swagger: ## 重新生成 Swagger 文档
	$(GO_ENV) go run github.com/swaggo/swag/cmd/swag@v1.16.6 init -g cmd/server/main.go -o docs/swagger

VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)

version: ## 打印当前构建版本号（git describe，注入 buildVersion）
	@echo $(VERSION)

build: swagger ## 编译后端（静态；ldflags 注入版本号，先生成 swagger 避免缺失编译失败）
	$(GO_ENV) CGO_ENABLED=0 go build -ldflags "-X main.buildVersion=$(VERSION)" -o $(BIN) ./cmd/server

run: build ## 编译并启动后端（:$(PORT)，默认 release 模式）
	PORT=$(PORT) $(BIN)

dev: db-start ## 本地开发：启动本地 MySQL（未运行则拉起）+ 后端 :8080 + 前端 :5173（/api 自动代理）
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
	docker build --build-arg BUILD_VERSION=$(VERSION) -t notice-service .

publish: ## 发布到 Docker Hub（构建 + 版本tag/latest 双tag 推送；FORCE=1 允许 dirty 工作区）
	./deploy/publish.sh $(if $(filter 1,$(FORCE)),--force)

docker-up: ## 启动多实例高可用部署（2 实例 + MySQL 5.7）
	docker compose up -d

docker-down: ## 停止部署
	docker compose down

docker-logs: ## 查看部署日志
	docker compose logs -f

## ---------- 本地 MySQL（.dev 下的裸 MariaDB） ----------

# 本地开发用的 MariaDB 不是 Docker/系统服务，而是 .dev 目录里的独立实例：
#   数据目录 .dev/mysql-data / socket .dev/mysql-run/mysqld.sock / 日志 .dev/mysql-run/mariadb.log
MYSQL_SOCK := $(CURDIR)/.dev/mysql-run/mysqld.sock
MYSQL_DATA := $(CURDIR)/.dev/mysql-data
MYSQL_RUN  := $(CURDIR)/.dev/mysql-run
MYSQL_USER := $(shell id -un)

db-start: ## 启动本地 MySQL（.dev 裸 MariaDB；已在运行则跳过）
	@mkdir -p $(MYSQL_RUN); \
	if mysqladmin --socket=$(MYSQL_SOCK) -u root ping >/dev/null 2>&1; then \
		echo "MySQL 已在运行（socket: $(MYSQL_SOCK)）"; \
		exit 0; \
	fi; \
	if [ ! -d "$(MYSQL_DATA)/mysql" ]; then \
		echo "首次初始化数据目录 $(MYSQL_DATA) ..."; \
		mariadb-install-db --datadir="$(MYSQL_DATA)" --user=$(MYSQL_USER) >/dev/null 2>&1 || true; \
		echo "提示：首次使用请按 README「快速开始」创建数据库与用户（notice_service / notice_service_test / notice）"; \
	fi; \
	echo "启动 mariadbd（127.0.0.1:3306）..."; \
	nohup mariadbd --datadir="$(MYSQL_DATA)" --socket="$(MYSQL_SOCK)" \
		--port=3306 --bind-address=127.0.0.1 \
		--pid-file="$(MYSQL_RUN)/mysqld.pid" \
		--log-error="$(MYSQL_RUN)/mariadb.log" \
		--user=$(MYSQL_USER) >/dev/null 2>&1 & \
	for i in $$(seq 1 30); do \
		if mysqladmin --socket="$(MYSQL_SOCK)" -u root ping >/dev/null 2>&1; then \
			echo "MySQL 已就绪（127.0.0.1:3306）"; \
			exit 0; \
		fi; \
		sleep 1; \
	done; \
	echo "错误：MySQL 启动超时，请查看日志 $(MYSQL_RUN)/mariadb.log"; \
	exit 1

db-stop: ## 停止本地 MySQL（优雅关闭）
	@if mysqladmin --socket=$(MYSQL_SOCK) -u root ping >/dev/null 2>&1; then \
		mysqladmin --socket=$(MYSQL_SOCK) -u root shutdown && echo "MySQL 已停止"; \
	else \
		echo "MySQL 未在运行"; \
	fi

db-status: ## 查看本地 MySQL 状态
	@if mysqladmin --socket=$(MYSQL_SOCK) -u root ping >/dev/null 2>&1; then \
		echo "运行中：127.0.0.1:3306（socket: $(MYSQL_SOCK)）"; \
		mysqladmin --socket=$(MYSQL_SOCK) -u root status 2>/dev/null | sed -n '1p' || true; \
	else \
		echo "未运行（可执行 make db-start 启动）"; \
	fi

## ---------- 运维 ----------

db-clean: ## 清空真实库与测试库数据（危险：会删除所有数据）
	@echo "危险操作：将删除 notice_service 与 notice_service_test 的全部数据，确认请运行: make db-clean FORCE=1"
	@[ "$(FORCE)" = "1" ] && (mysql --socket=$(CURDIR)/.dev/mysql-run/mysqld.sock -u root -e "DROP DATABASE IF EXISTS notice_service; DROP DATABASE IF EXISTS notice_service_test; CREATE DATABASE notice_service CHARACTER SET utf8mb4; CREATE DATABASE notice_service_test CHARACTER SET utf8mb4; GRANT ALL ON notice_service.* TO 'notice'@'%'; GRANT ALL ON notice_service_test.* TO 'notice'@'%';") || echo "已取消"

clean: ## 清理构建产物（含生成的 Swagger 文档）
	rm -rf $(BIN) $(WEB)/dist $(WEB)/node_modules docs/swagger
