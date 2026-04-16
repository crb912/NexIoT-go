.PHONY: build test lint run docker clean tidy

# Example: https://github.com/edgexfoundry/device-sdk-go/blob/v2.3.1/Makefile

SERVICE_NAME  := better-iot-edge
VERSION       := $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
BINARY        := ./bin/$(SERVICE_NAME)
DOCKER_IMAGE  := $(SERVICE_NAME):$(VERSION)

## build: 编译服务二进制
build: tidy
	@mkdir -p bin
	CGO_ENABLED=1 go build -ldflags="-s -w -X main.serviceVersion=266420a" \
            -o ./bin/edge-gateway ./cmd/

## test: 运行单元测试（不含集成测试）
test:
	go test -race -count=1 ./internal/... ./driver/...

## test-cover: 测试并生成覆盖率报告
test-cover:
	go test -race -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out -o coverage.html

## lint: 静态分析（需要安装 golangci-lint）
lint:
	golangci-lint run ./...

## tidy: 整理依赖
tidy:
	go mod tidy

## run: 本地运行（依赖 EdgeX 核心服务已通过 docker-compose 启动）
run: build
	$(BINARY) \
		--configDir=./res \
		--configFile=configuration.yaml \
		--overwriteProfiles \
		--overwriteDevices

## docker: 构建 Docker 镜像
##docker:
##	docker build --build-arg VERSION=$(VERSION) -t $(DOCKER_IMAGE) .

## clean: 清理编译产物
clean:
	rm -rf bin/ coverage.out coverage.html

vendor:
	$(GO) mod vendor