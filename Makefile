.PHONY: build test clean run unitest tidy lint

SERVICE_NAME       := octopus-edge
SERVICE_VERSION    := "dev-d9d13b7" # AUTO_GENERATED
BINARY             := ./bin/$(SERVICE_NAME)
ARCH               := $(shell uname -m)

# Find config directory by priority: 
# 1. current dir, 2. bin dir, 3. parent dir
CONFIG_DIR := $(shell \
	if [ -d "./res" ]; then \
		echo "./res"; \
	elif [ -d "./bin/res" ]; then \
		echo "./bin/res"; \
	elif [ -d "../res" ]; then \
		echo "../res"; \
	fi \
)

## build: Compile the binary
build: tidy
	@mkdir -p bin
	CGO_ENABLED=1 go build -ldflags='-s -w -X device.Version=${SERVICE_VERSION} -X main.serviceName=${SERVICE_NAME}' \
			-o $(BINARY) ./cmd/main.go

# Setup local environment and git hooks
init:
	git config core.hooksPath .githooks
	@echo "Git hooks configured successfully."

tidy:
	go mod tidy

unittest:
	GO111MODULE=on go test $(GOTESTFLAGS) -coverprofile=coverage.out ./...

## test-cover: Test and generate coverage report
test-cover:
	go test -race -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out -o coverage.html

## lint: Static analysis (requires golangci-lint)
lint:
	@which golangci-lint >/dev/null || echo "WARNING: go linter not installed. To install, run\n  curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh | sh -s -- -b \$$(go env GOPATH)/bin v1.46.2"
	@if [ "z${ARCH}" = "zx86_64" ] && which golangci-lint >/dev/null ; then golangci-lint run --config .golangci.yml ; else echo "WARNING: Linting skipped (not on x86_64 or linter not installed)"; fi

test: unittest lint
	GO111MODULE=on go vet ./...
	gofmt -l $$(find . -type f -name '*.go'| grep -v "/vendor/")
	[ "`gofmt -l $$(find . -type f -name '*.go'| grep -v "/vendor/")`" = "" ]
	./bin/test-attribution-txt.sh

## run: Export env var and run locally
run: build
	export EDGEX_SECURITY_SECRET_STORE=false && $(BINARY) \
	   --confdir=$(CONFIG_DIR) \
	   --file=configuration.toml \
	   --overwrite

## clean: Clean build artifacts
clean:
	rm -rf bin/ coverage.out coverage.html

vendor:
	go mod vendor