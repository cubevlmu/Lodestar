# Lodestar — build & release helpers
#
#   make build    构建 WebUI + 编译二进制
#   make dev      前端热更新（/api 代理到 :8080）
#   make run      构建并启动服务端
#   make test     go vet + go test
#   make clean    清理产物
#
# 没有 make 的机器（比如 Windows）用 scripts/build.sh 或 scripts/build.ps1。

BINARY  := lodestar
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
COMMIT  ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo dev)
LDFLAGS := -s -w -X lodestar/internal/buildinfo.Version=$(VERSION) -X lodestar/internal/buildinfo.Commit=$(COMMIT)

.PHONY: all build webui dev run test vet fmt tidy clean docker

all: build

webui:
	cd webui && npm install --no-audit --no-fund && npm run build

build: webui
	CGO_ENABLED=0 go build -trimpath -ldflags "$(LDFLAGS)" -o $(BINARY) ./cmd/lodestar

dev:
	cd webui && npm run dev

run: build
	./$(BINARY) serve

test:
	go vet ./...
	go test ./...

vet:
	go vet ./...

fmt:
	gofmt -w ./cmd ./internal

tidy:
	go mod tidy

clean:
	rm -f $(BINARY) $(BINARY).exe
	rm -rf internal/web/dist/assets internal/web/dist/index.html

docker:
	docker build -t lodestar:$(VERSION) .
