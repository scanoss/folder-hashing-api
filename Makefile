export PATH := /usr/bin/go/bin:$(PATH)
#vars
IMAGE_NAME=folder-hashing-api
REPO=scanoss
DOCKER_FULLNAME=${REPO}/${IMAGE_NAME}
GHCR_FULLNAME=ghcr.io/${REPO}/${IMAGE_NAME}
VERSION=$(shell git tag --sort=-version:refname | head -n 1)

# Set default version if no git tags
ifeq ($(VERSION),)
VERSION=dev
endif

# HELP
# This will output the help for each task
# thanks to https://marmelab.com/blog/2016/02/29/auto-documented-makefile.html
.PHONY: help

help: ## This help
	@awk 'BEGIN {FS = ":.*?## "} /^[a-zA-Z_-]+:.*?## / {printf "\033[36m%-20s\033[0m %s\n", $$1, $$2}' $(MAKEFILE_LIST)

.DEFAULT_GOAL := help

# Development Workflow
clean_testcache:  ## Expire all Go test caches
	@echo "Cleaning test caches..."
	go clean -testcache ./...

test: ## Run tests in Docker environment
	@echo "Running tests in Docker..."
	docker-compose -f docker-compose.yml -f docker-compose.dev.yml run --rm hfh-api go test -v ./...

unit_test:  ## Run all unit tests locally
	@echo "Running unit test framework..."
	go test -v ./...

lint_local: ## Run local instance of linting across the code base
	golangci-lint run ./...

lint_local_fix: ## Run local instance of linting across the code base including auto-fixing
	golangci-lint run --fix ./...

lint_docker: ## Run docker instance of linting across the code base
	docker run --rm -v $(pwd):/app -v ~/.cache/golangci-lint/v1.50.1:/root/.cache -w /app golangci/golangci-lint:v1.50.1 golangci-lint run ./...

run_local:  ## Launch the API locally for test
	@echo "Launching API locally..."
	go run cmd/server/main.go -ldflags "-X github.com/scanoss/folder-hashing-api/internal/domain/entities.AppVersion=$(VERSION)"

# Docker Operations
docker_build: ## Build Docker image
	@echo "Building Docker image $(VERSION)..."
	docker build --build-arg VERSION=$(VERSION) -t $(DOCKER_FULLNAME):$(VERSION) -t $(DOCKER_FULLNAME):latest .

docker_build_dev: ## Build Docker image for development
	@echo "Building development Docker image..."
	docker build --build-arg VERSION=$(VERSION)-dev -t $(DOCKER_FULLNAME):dev .

docker_build_multiarch: ## Build multi-architecture Docker image
	@echo "Building multi-architecture Docker image..."
	docker buildx build --platform linux/amd64,linux/arm64 --build-arg VERSION=$(VERSION) -t $(DOCKER_FULLNAME):$(VERSION) -t $(DOCKER_FULLNAME):latest --push .

docker_up: ## Start services with docker-compose (production)
	@echo "Starting production services..."
	docker-compose -f docker-compose.yml -f docker-compose.prod.yml up -d

docker_up_dev: ## Start development environment
	@echo "Starting development services..."
	docker-compose -f docker-compose.yml -f docker-compose.dev.yml up -d

docker_down: ## Stop all services
	@echo "Stopping all services..."
	docker-compose down

docker_logs: ## View service logs
	@echo "Viewing service logs..."
	docker-compose logs -f

docker_status: ## Check service status
	@echo "Checking service status..."
	docker-compose ps

docker_clean: ## Clean up Docker resources
	@echo "Cleaning up Docker resources..."
	docker-compose down -v
	docker system prune -f

# Development shortcuts
dev: docker_up_dev ## Quick start development environment
prod: docker_up ## Quick start production environment

# GitHub Container Registry Operations
ghcr_build: ## Build GitHub container image
	@echo "Building GHCR container image..."
	docker build --no-cache --build-arg VERSION=$(VERSION) -t $(GHCR_FULLNAME):$(VERSION) -t $(GHCR_FULLNAME):latest --platform linux/amd64 .

ghcr_push:  ## Push the GH container image to GH Packages
	@echo "Publishing GHCR container $(VERSION)..."
	docker push $(GHCR_FULLNAME):$(VERSION)
	docker push $(GHCR_FULLNAME):latest

ghcr_multiarch: ## Build and push multi-architecture images to GHCR
	@echo "Building and pushing multi-architecture images to GHCR..."
	docker buildx build --platform linux/amd64,linux/arm64 --build-arg VERSION=$(VERSION) -t $(GHCR_FULLNAME):$(VERSION) -t $(GHCR_FULLNAME):latest --push .

ghcr_all: ghcr_build ghcr_push  ## Execute all GitHub Package container actions

# Binary Building (for local development)
build_amd:  ## Build an AMD 64 binary
	@echo "Building AMD binary $(VERSION)..."
	@mkdir -p ./dist
	GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -ldflags="-w -s -X github.com/scanoss/folder-hashing-api/internal/domain/entities.AppVersion=$(VERSION)" -o ./dist/scanoss-hfh-api ./cmd/server

build_arm:  ## Build an ARM 64 binary
	@echo "Building ARM binary $(VERSION)..."
	@mkdir -p ./dist
	GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -ldflags="-w -s -X github.com/scanoss/folder-hashing-api/internal/domain/entities.AppVersion=$(VERSION)" -o ./dist/scanoss-hfh-api ./cmd/server

# Docker Package Creation
package: docker_package_amd  ## Create Docker distribution package (AMD64)

docker_package_amd:  ## Create AMD64 Docker distribution package
	@echo "Creating AMD64 Docker distribution package..."
	bash ./scripts/create-docker-package.sh linux/amd64 $(VERSION)

docker_package_arm:  ## Create ARM64 Docker distribution package
	@echo "Creating ARM64 Docker distribution package..."
	bash ./scripts/create-docker-package.sh linux/arm64 $(VERSION)

docker_package_multi:  ## Create multi-architecture Docker distribution package
	@echo "Creating multi-architecture Docker distribution package..."
	bash ./scripts/create-docker-package.sh multi $(VERSION)

docker_package_all: docker_package_amd docker_package_arm docker_package_multi  ## Create all Docker distribution packages

# Collection Management
collections_export: ## Export collections using Docker (requires running services)
	@echo "Exporting collections..."
	bash ./scripts/create-collection-snapshots.sh snapshots/

collections_import: ## Import collections using Docker (requires snapshots directory)
	@echo "Importing collections..."
	@if [ ! -d "snapshots" ]; then echo "❌ snapshots/ directory not found. Please provide collection snapshots."; exit 1; fi
	bash ./scripts/import-collections.sh snapshots/

collections_verify: ## Verify collections in Docker environment
	@echo "Verifying collections..."
	@if curl -f http://localhost:6333/collections >/dev/null 2>&1; then \
		collections=$$(curl -s http://localhost:6333/collections | grep -o '"name":"[^"]*"' | wc -l || echo "0"); \
		echo "✅ Qdrant is running with $$collections collections"; \
	else \
		echo "❌ Qdrant is not responding. Please start services with 'make dev' or 'make prod'"; \
		exit 1; \
	fi

# Environment Management
env_setup: ## Setup local development environment
	@echo "Setting up development environment..."
	@mkdir -p ./config ./snapshots
	@if [ ! -f "./config/app-config.json" ]; then \
		cp config.example.json ./config/app-config.json; \
		echo "✅ Configuration template copied to ./config/app-config.json"; \
	fi

env_clean: ## Clean up all Docker resources (⚠️ Removes data!)
	@echo "Cleaning up environment..."
	docker-compose down -v
	docker system prune -f
	docker volume prune -f

env_reset: env_clean env_setup ## Reset entire development environment

# Monitoring & Debugging
docker_stats: ## Show container resource usage
	@echo "Container resource usage:"
	docker stats --no-stream

docker_health: ## Check health of all services
	@echo "Checking service health..."
	@./scripts/docker-deploy.sh prod status

docker_debug_qdrant: ## Debug Qdrant container issues
	@echo "Qdrant container debugging info:"
	@echo "=== Container Status ==="
	docker ps --filter name=scanoss-qdrant
	@echo "=== Container Logs (last 50 lines) ==="
	docker logs --tail 50 scanoss-qdrant
	@echo "=== Container Inspect ==="
	docker inspect scanoss-qdrant --format='{{json .State}}' | python3 -m json.tool 2>/dev/null || echo "Could not format JSON"

docker_debug_api: ## Debug HFH API container issues
	@echo "HFH API container debugging info:"
	@echo "=== Container Status ==="
	docker ps --filter name=scanoss-hfh-api
	@echo "=== Container Logs (last 50 lines) ==="
	docker logs --tail 50 scanoss-hfh-api
	@echo "=== Container Inspect ==="
	docker inspect scanoss-hfh-api --format='{{json .State}}' | python3 -m json.tool 2>/dev/null || echo "Could not format JSON"

# Deployment Management
deploy_prod: ## Deploy production environment
	@echo "Deploying production environment..."
	bash ./scripts/docker-deploy.sh prod up

deploy_dev: ## Deploy development environment
	@echo "Deploying development environment..."
	bash ./scripts/docker-deploy.sh dev up

deploy_stop: ## Stop deployment
	@echo "Stopping deployment..."
	bash ./scripts/docker-deploy.sh prod down

deploy_logs: ## View deployment logs
	bash ./scripts/docker-deploy.sh prod logs

deploy_status: ## Check deployment status
	bash ./scripts/docker-deploy.sh prod status

# Version Management
version: ## Display current version
	@echo "Current version: $(VERSION)"

version_tag: ## Create a new version tag
	@read -p "Enter new version (current: $(VERSION)): " new_version; \
	git tag $$new_version && \
	echo "Created tag: $$new_version"

# Release Management
release: docker_package_all ## Build and package everything for release
	@echo "🎉 Release $(VERSION) packages created!"
	@ls -la scanoss-hfh-api-docker-*-$(VERSION)-*.tar.gz
