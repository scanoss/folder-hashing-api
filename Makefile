# Version management
VERSION ?= $(shell git tag --sort=-version:refname | head -n 1)
ifeq ($(VERSION),)
VERSION := dev
endif

# Build flags
LDFLAGS := -w -s -X github.com/scanoss/folder-hashing-api/internal/domain/entities.AppVersion=$(VERSION)
BUILD_DIR := ./target

# HELP
# This will output the help for each task
# thanks to https://marmelab.com/blog/2016/02/29/auto-documented-makefile.html
.PHONY: help

help: ## This help
	@awk 'BEGIN {FS = ":.*?## "} /^[0-9a-zA-Z_-]+:.*?## / {printf "\033[36m%-20s\033[0m %s\n", $$1, $$2}' $(MAKEFILE_LIST)

.DEFAULT_GOAL := help

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
build: build_amd64 build_arm64 ## Build binaries for all architectures

build_amd64: ## Build AMD64 binary
	@mkdir -p $(BUILD_DIR)
	@GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -ldflags="$(LDFLAGS)" -o $(BUILD_DIR)/scanoss-folder-hashing-api-linux-amd64 ./cmd/server

build_arm64: ## Build ARM64 binary
	@mkdir -p $(BUILD_DIR)
	@GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -ldflags="$(LDFLAGS)" -o $(BUILD_DIR)/scanoss-folder-hashing-api-linux-arm64 ./cmd/server

build_import_amd64: ## Build import tool (AMD64)
	@mkdir -p $(BUILD_DIR)
	@GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -ldflags="$(LDFLAGS)" -o $(BUILD_DIR)/scanoss-folder-hashing-import-linux-amd64 ./cmd/import

build_import_arm64: ## Build import tool (ARM64)
	@mkdir -p $(BUILD_DIR)
	@GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -ldflags="$(LDFLAGS)" -o $(BUILD_DIR)/scanoss-folder-hashing-import-linux-arm64 ./cmd/import

# Maintenance
clean: ## Clean build artifacts
	@rm -rf $(BUILD_DIR) coverage.out
	@go clean -cache -testcache -modcache

tidy: ## Tidy and verify dependencies
	@go mod tidy
	@go mod verify

version: ## Display current version
	@echo "Current version: $(VERSION)"

# Quality checks (CI)
ci: lint test ## Run all CI checks

# Packaging
package_amd64: version  ## Build & Package an AMD 64 binary
	@echo "Building AMD binary $(VERSION) and placing into scripts..."
	go generate ./cmd/server/main.go
	GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -ldflags="$(LDFLAGS)" -o ./scripts/scanoss-folder-hashing-api ./cmd/server/main.go
	bash ./package-scripts.sh linux-amd64 $(VERSION)

package_arm64: version  ## Build & Package an ARM 64 binary
	@echo "Building ARM binary $(VERSION) and placing into scripts..."
	go generate ./cmd/server/main.go
	GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -ldflags="$(LDFLAGS)" -o ./scripts/scanoss-folder-hashing-api ./cmd/server/main.go
	bash ./package-scripts.sh linux-arm64 $(VERSION)
