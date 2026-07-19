.PHONY: help build dev prod simulator stop-simulator test test-cover lint vet fmt tidy clean vendor init check

SERVICE_NAME := devices-iot-go
BINARY       := ./bin/$(SERVICE_NAME)
ARCH         := $(shell uname -m)

# ==============================================================================
# Variables and configuration
# ==============================================================================

# 1. Get and check version using native Makefile syntax
GIT_BRANCH := $(shell git rev-parse --abbrev-ref HEAD 2>/dev/null)
GIT_COMMIT := $(shell git rev-parse --short HEAD 2>/dev/null)

ifneq ($(GIT_BRANCH),)
ifneq ($(GIT_COMMIT),)
	SERVICE_VERSION := $(GIT_BRANCH)-$(GIT_COMMIT)
else
	SERVICE_VERSION := 1.0.0
endif
else
	SERVICE_VERSION := 1.0.0
endif

# 2. Centralize build flags
LDFLAGS := -s -w -X device.Version=$(SERVICE_VERSION) -X main.serviceName=$(SERVICE_NAME)

# 3. Provide default config dir, allow env var override (e.g., CONFIG_DIR=../res make dev)
CONFIG_DIR ?= ./res

# ==============================================================================
# Targets
# ==============================================================================

default: help

## help: Print this help message
help:
	@echo "Usage: make <target>"
	@echo ""
	@echo "Targets:"
	@sed -n 's/^##//p' ${MAKEFILE_LIST} | column -t -s ':' |  sed -e 's/^/ /'

## build: Compile the binary
build:
	@go mod tidy
	@echo "=> Building $(SERVICE_NAME) v$(SERVICE_VERSION)..."
	@mkdir -p bin
	CGO_ENABLED=1 go build -ldflags="$(LDFLAGS)" -o $(BINARY) ./cmd/main.go

## dev: Build, start background simulator, then run the service
dev:
	@echo "=> Starting $(SERVICE_NAME) (Dev Mode)..."
	EDGEX_SECURITY_SECRET_STORE=false $(BINARY) \
		--confdir=$(CONFIG_DIR) \
		--file=configuration.toml \
		--overwrite

## prod: Build, then run the service
prod: build
	@echo "=> Starting $(SERVICE_NAME) (Prod Mode)..."
	EDGEX_SECURITY_SECRET_STORE=false $(BINARY) \
		--confdir=$(CONFIG_DIR) \
		--file=configuration.toml \
		--overwrite

## simulator-gui: open modbus simulator in a new terminal
simulator-gui:
	@echo "Opening modbus simulator in new terminal..."
	@if command -v gnome-terminal >/dev/null 2>&1; then \
		gnome-terminal -- bash -c "echo '>>> Modbus simulator (port 5020)'; python3 ./simulator/modbus.py; exec bash" & \
	elif command -v xterm >/dev/null 2>&1; then \
		xterm -hold -e "python3 ./simulator/modbus.py" & \
	elif command -v konsole >/dev/null 2>&1; then \
		konsole --hold -e python3 ./simulator/modbus.py & \
	else \
		python3 ./simulator/modbus.py & \
	fi
	@sleep 2

## simulator: Start modbus simulator in the background
simulator:
	@echo "=> Starting modbus simulator..."
	@nohup python3 ./simulator/modbus.py > simulator.log 2>&1 & echo $$! > simulator.pid
	@echo "Simulator running in background (PID: $$(cat simulator.pid)). Logs in simulator.log."
	@sleep 2

## stop-simulator: Stop the background modbus simulator
stop-simulator:
	@if [ -f simulator.pid ]; then \
		kill $$(cat simulator.pid) && rm simulator.pid && echo "Simulator stopped."; \
	else \
		echo "Simulator PID file not found, maybe not running."; \
	fi

## test: Run unit tests with race detector
test:
	@echo "=> Running tests..."
	go test -race -coverprofile=coverage.out ./...

## test-cover: Run tests and generate HTML coverage report
test-cover: test
	go tool cover -html=coverage.out -o coverage.html

## lint: Static analysis (requires golangci-lint)
lint:
	@echo "=> Running golangci-lint..."
	@if ! command -v golangci-lint >/dev/null 2>&1; then \
		echo "WARNING: golangci-lint not installed. Run 'go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest'"; \
		exit 1; \
	fi
	golangci-lint run --config .golangci.yml

## vet: Run go vet
vet:
	@echo "=> Running go vet..."
	go vet ./...

## fmt: Check formatting and fix
fmt:
	@echo "=> Formatting code..."
	@gofmt -w $$(find . -type f -name '*.go' | grep -v "/vendor/")

## tidy: Tidy go modules
tidy:
	@echo "=> Tidy up go.mod..."
	go mod tidy

## clean: Remove build artifacts and stop simulator
clean: stop-simulator
	@echo "=> Cleaning artifacts..."
	rm -rf bin/ coverage.out coverage.html simulator.log simulator.pid

## vendor: Vendor dependencies
vendor:
	go mod vendor

## init: Configure git hooks
init:
	git config core.hooksPath .githooks
	@echo "=> Git hooks configured."

## check: Run all CI checks (fmt, tidy, vet, lint, test)
check: fmt tidy vet lint test
	@echo "=> All checks passed successfully."
