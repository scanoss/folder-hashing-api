.PHONY: help test lint fmt build clean run
.DEFAULT_GOAL := help

# Version management
VERSION ?= $(shell git tag --sort=-version:refname | head -n 1)
ifeq ($(VERSION),)
VERSION := dev
endif

# Build flags
LDFLAGS := -w -s -X github.com/scanoss/folder-hashing-api/internal/domain/entities.AppVersion=$(VERSION)
BUILD_DIR := ./target

help: ## Show this help message
	@awk 'BEGIN {FS = ":.*?## "} /^[a-zA-Z_-]+:.*?## / {printf "\033[36m%-20s\033[0m %s\n", $$1, $$2}' $(MAKEFILE_LIST)

# Development
run: ## Run the API locally
	@go run cmd/server/main.go

test: ## Run all tests
	@go test -v -race -coverprofile=coverage.out ./...

test-coverage: test ## Run tests with coverage report
	@go tool cover -html=coverage.out

clean-testcache: ## Clear test cache
	@go clean -testcache

# Code quality
lint: ## Run linter
	@golangci-lint run ./...

lint-fix: ## Run linter with auto-fix
	@golangci-lint run --fix ./...

fmt: ## Format code
	@gofumpt -l -w .
	@goimports -w .

vet: ## Run go vet
	@go vet ./...

# Build
build: build-amd64 build-arm64 ## Build binaries for all architectures

build-amd64: ## Build AMD64 binary
	@mkdir -p $(BUILD_DIR)
	@GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -ldflags="$(LDFLAGS)" -o $(BUILD_DIR)/folder-hashing-api-linux-amd64 ./cmd/server

build-arm64: ## Build ARM64 binary
	@mkdir -p $(BUILD_DIR)
	@GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -ldflags="$(LDFLAGS)" -o $(BUILD_DIR)/folder-hashing-api-linux-arm64 ./cmd/server

build-import-amd64: ## Build import tool (AMD64)
	@mkdir -p $(BUILD_DIR)
	@GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -ldflags="-w -s -X main.Version=$(VERSION)" -o $(BUILD_DIR)/hfh-import-linux-amd64 ./cmd/import

build-import-arm64: ## Build import tool (ARM64)
	@mkdir -p $(BUILD_DIR)
	@GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -ldflags="-w -s -X main.Version=$(VERSION)" -o $(BUILD_DIR)/hfh-import-linux-arm64 ./cmd/import

# Maintenance
clean: ## Clean build artifacts
	@rm -rf $(BUILD_DIR) coverage.out
	@go clean -cache -testcache -modcache

tidy: ## Tidy and verify dependencies
	@go mod tidy
	@go mod verify

version: ## Show current version
	@echo $(VERSION)

# Quality checks (CI)
ci: lint test ## Run all CI checks
