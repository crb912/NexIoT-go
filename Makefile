.PHONY: build test clean run unitest tidy lint

SERVICE_NAME 		:= better-iot-edge
SERVICE_VERSION     := "dev-81b77be" # AUTO_GENERATED
BINARY        		:= ./bin/$(SERVICE_NAME)
ARCH				:=	$(shell uname -m)

## build: 编译服务二进制
build: tidy
	@mkdir -p bin
	CGO_ENABLED=1 go build -ldflags="-s -w -X main.serviceVersion=${Version})" \
            -o ./bin/edge-gateway ./cmd/
# Setup local environment and git hooks
init:
	git config core.hooksPath .githooks
	@echo "Git hooks configured successfully."

tidy:
	go mod tidy

unittest:
	GO111MODULE=on go test $(GOTESTFLAGS) -coverprofile=coverage.out ./...

## test-cover: 测试并生成覆盖率报告
test-cover:
	go test -race -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out -o coverage.html

## lint: 静态分析（需要安装 golangci-lint）
lint:
	@which golangci-lint >/dev/null || echo "WARNING: go linter not installed. To install, run\n  curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh | sh -s -- -b \$$(go env GOPATH)/bin v1.46.2"
	@if [ "z${ARCH}" = "zx86_64" ] && which golangci-lint >/dev/null ; then golangci-lint run --config .golangci.yml ; else echo "WARNING: Linting skipped (not on x86_64 or linter not installed)"; fi


test: unittest lint
	GO111MODULE=on go vet ./...
	gofmt -l $$(find . -type f -name '*.go'| grep -v "/vendor/")
	[ "`gofmt -l $$(find . -type f -name '*.go'| grep -v "/vendor/")`" = "" ]
	./bin/test-attribution-txt.sh


## run: 本地运行（依赖 EdgeX 核心服务已通过 docker-compose 启动）
run: build
	$(BINARY) \
		--configDir=./res \
		--configFile=configuration.yaml \
		--overwriteProfiles \
		--overwriteDevices

## clean: 清理编译产物
clean:
	rm -rf bin/ coverage.out coverage.html

vendor:
	$(GO) mod vendor
