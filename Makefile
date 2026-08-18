# Variables
BINARY_NAME=todo
VERSION=$(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
BUILD_TIME=$(shell date +%FT%T%z)
LDFLAGS=-ldflags "-X main.version=$(VERSION) -X main.buildTime=$(BUILD_TIME)"

# Go related variables
GOBASE=$(shell pwd)
GOBIN=$(GOBASE)/bin
GOFILES=$(wildcard *.go)

# Colors for pretty output
RED=\033[0;31m
GREEN=\033[0;32m
YELLOW=\033[0;33m
BLUE=\033[0;34m
NC=\033[0m # No Color

.PHONY: help build test test-v test-cover run fmt lint vet tidy check install uninstall clean deps

## help: Show this help message
help:
	@echo "$(BLUE)todo CLI - Available targets:$(NC)"
	@echo ""
	@echo "$(GREEN)Development:$(NC)"
	@echo "  build       Build the binary"
	@echo "  test        Run tests"
	@echo "  test-v      Run tests with verbose output"
	@echo "  test-cover  Run tests with coverage"
	@echo "  run         Run the application"
	@echo ""
	@echo "$(GREEN)Code Quality:$(NC)"
	@echo "  fmt         Format Go code"
	@echo "  lint        Run golangci-lint"
	@echo "  vet         Run go vet"
	@echo "  tidy        Tidy Go modules"
	@echo "  check       Run all quality checks (fmt, vet, lint, test)"
	@echo ""
	@echo "$(GREEN)Installation:$(NC)"
	@echo "  install     Install binary to /usr/local/bin"
	@echo "  uninstall   Remove binary from /usr/local/bin"
	@echo ""
	@echo "$(GREEN)Utilities:$(NC)"
	@echo "  clean       Clean build artifacts"
	@echo "  deps        Install development dependencies"

## build: Build the binary
build:
	@echo "$(BLUE)Building $(BINARY_NAME)...$(NC)"
	@go build $(LDFLAGS) -o $(BINARY_NAME) .
	@echo "$(GREEN)Build complete: $(BINARY_NAME)$(NC)"

## test: Run tests
test:
	@echo "$(BLUE)Running tests...$(NC)"
	@go test ./...
	@echo "$(GREEN)Tests passed$(NC)"

## test-v: Run tests with verbose output
test-v:
	@echo "$(BLUE)Running tests (verbose)...$(NC)"
	@go test -v ./...

## test-cover: Run tests with coverage
test-cover:
	@echo "$(BLUE)Running tests with coverage...$(NC)"
	@go test -v -cover ./...
	@go test -coverprofile=coverage.out ./
	@go tool cover -html=coverage.out -o coverage.html
	@echo "$(GREEN)Coverage report generated: coverage.html$(NC)"

## run: Run the application
run: build
	@echo "$(BLUE)Running $(BINARY_NAME)...$(NC)"
	@./$(BINARY_NAME)

## fmt: Format Go code
fmt:
	@echo "$(BLUE)Formatting code...$(NC)"
	@go fmt ./...
	@echo "$(GREEN)Code formatted$(NC)"

## lint: Run golangci-lint
lint:
	@echo "$(BLUE)Running linter...$(NC)"
	@if command -v golangci-lint >/dev/null 2>&1; then \
		golangci-lint run; \
		echo "$(GREEN)Linting complete$(NC)"; \
	else \
		echo "$(YELLOW)golangci-lint not found. Install with: go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest$(NC)"; \
	fi

## vet: Run go vet
vet:
	@echo "$(BLUE)Running go vet...$(NC)"
	@go vet ./...
	@echo "$(GREEN)Vet check passed$(NC)"

## tidy: Tidy Go modules
tidy:
	@echo "$(BLUE)Tidying modules...$(NC)"
	@go mod tidy
	@echo "$(GREEN)Modules tidied$(NC)"

## check: Run all quality checks
check: fmt vet lint test
	@echo "$(GREEN)All quality checks passed$(NC)"

## install: Install binary to /usr/local/bin
install: build
	@echo "$(BLUE)Installing $(BINARY_NAME) to /usr/local/bin...$(NC)"
	@sudo cp $(BINARY_NAME) /usr/local/bin/
	@echo "$(GREEN)$(BINARY_NAME) installed$(NC)"

## uninstall: Remove binary from /usr/local/bin
uninstall:
	@echo "$(BLUE)Uninstalling $(BINARY_NAME)...$(NC)"
	@sudo rm -f /usr/local/bin/$(BINARY_NAME)
	@echo "$(GREEN)$(BINARY_NAME) uninstalled$(NC)"

## clean: Clean build artifacts
clean:
	@echo "$(BLUE)Cleaning...$(NC)"
	@rm -f $(BINARY_NAME)
	@rm -f coverage.out coverage.html
	@echo "$(GREEN)Clean complete$(NC)"

## deps: Install development dependencies
deps:
	@echo "$(BLUE)Installing development dependencies...$(NC)"
	@go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest
	@echo "$(GREEN)Dependencies installed$(NC)"

# Default target
all: check build 
