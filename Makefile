# mindbridge Makefile
# 国内网络优先走 goproxy.cn
GOPROXY ?= https://goproxy.cn,direct
PKG := github.com/JexKnox/mindbridge
BIN := mindbridge
ADDR ?= :8080
WEB ?= frontend

export GOPROXY

.PHONY: tidy build run test vet fmt clean migrate web-install web-dev web-build

tidy: ## 拉取并整理依赖
	go mod tidy

build: ## 编译二进制到 ./bin/
	mkdir -p bin
	go build -o bin/$(BIN) ./cmd/mindbridge

run: ## 本地运行
	go run ./cmd/mindbridge

test: ## 跑全部测试
	go test ./...

vet: ## 静态检查
	go vet ./...

fmt: ## 格式化
	gofmt -s -w .

migrate: ## 执行 DB 迁移(占位:实际由程序启动时自动跑)
	@echo "migrations 在程序启动时自动执行"

web-install: ## 安装前端依赖
	cd $(WEB) && pnpm install

web-dev: ## 前端开发服务器(:5173,/api 反代到 :8080)
	cd $(WEB) && pnpm dev

web-build: ## 构建前端到 frontend/dist(Go 启动时自动托管)
	cd $(WEB) && pnpm build

clean:
	rm -rf bin/ $(WEB)/dist
