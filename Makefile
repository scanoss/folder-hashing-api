
export PATH := /usr/bin/go/bin:$(PATH)
#vars
IMAGE_NAME=folder-hashing-api
REPO=scanoss
DOCKER_FULLNAME=${REPO}/${IMAGE_NAME}
GHCR_FULLNAME=ghcr.io/${REPO}/${IMAGE_NAME}
VERSION=$(shell git tag --sort=-version:refname | head -n 1)

# HELP
# This will output the help for each task
# thanks to https://marmelab.com/blog/2016/02/29/auto-documented-makefile.html
.PHONY: help

help: ## This help
	@awk 'BEGIN {FS = ":.*?## "} /^[a-zA-Z_-]+:.*?## / {printf "\033[36m%-20s\033[0m %s\n", $$1, $$2}' $(MAKEFILE_LIST)

.DEFAULT_GOAL := help

clean:  ## Clean all dev data
	@echo "Removing dev data..."
	@rm -f pkg/cmd/version.txt version.txt

clean_testcache:  ## Expire all Go test caches
	@echo "Cleaning test caches..."
	go clean -testcache ./...

version:  ## Produce hfh version text file
	@echo "Writing version file..."
	echo $(VERSION) > pkg/cmd/version.txt
test: ## Deploy milvus and run tests
	@echo "Deploying Milvus..."
	@CURRENT_DIR=$$(pwd) && \
	cd pkg/usecase/milvus && \
	bash milvus_deploy.sh && \
	cd $$CURRENT_DIR && \
	echo "Running tests..." && \
	$(MAKE) unit_test

unit_test:  ## Run all unit tests in the pkg folder
	@echo "Running unit test framework..."
	go test -v ./pkg/...

lint_local: ## Run local instance of linting across the code base
	golangci-lint run ./...

lint_local_fix: ## Run local instance of linting across the code base including auto-fixing
	golangci-lint run --fix ./...

lint_docker: ## Run docker instance of linting across the code base
	docker run --rm -v $(pwd):/app -v ~/.cache/golangci-lint/v1.50.1:/root/.cache -w /app golangci/golangci-lint:v1.50.1 golangci-lint run ./...

run_local:  ## Launch the API locally for test
	@echo "Launching API locally..."
	go run cmd/server/main.go -ldflags "-X github.com/scanoss/folder-hashing-api/entities.AppVersion=$(VERSION)"

ghcr_build: version  ## Build GitHub container image
	@echo "Building GHCR container image..."
	docker build --no-cache -t $(GHCR_FULLNAME) --platform linux/amd64 .

ghcr_tag:  ## Tag the latest GH container image with the version from Git tag
	@echo "Tagging GHCR latest image with $(VERSION)..."
	docker tag $(GHCR_FULLNAME):latest $(GHCR_FULLNAME):$(VERSION)

ghcr_push:  ## Push the GH container image to GH Packages
	@echo "Publishing GHCR container $(VERSION)..."
	docker push $(GHCR_FULLNAME):$(VERSION)
	docker push $(GHCR_FULLNAME):latest

ghcr_all: ghcr_build ghcr_tag ghcr_push  ## Execute all GitHub Package container actions

build_amd: version  ## Build an AMD 64 binary
	@echo "Building AMD binary $(VERSION)..."
	GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -ldflags="-w -s -X github.com/scanoss/folder-hashing-api/entities.AppVersion=$(VERSION)" -o ./target/scanoss-hfh-api-linux-amd64 ./cmd/server

build_arm: version  ## Build an ARM 64 binary
	@echo "Building ARM binary $(VERSION)..."
	GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -ldflags="-w -s -X github.com/scanoss/folder-hashing-api/entities.AppVersion=$(VERSION)" -o ./target/scanoss-hfh-api-linux-arm64 ./cmd/server

package: package_amd  ## Build & Package an AMD 64 binary

package_amd: version  ## Build & Package an AMD 64 binary
	@echo "Building AMD binary $(VERSION) and placing into scripts..."
	GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -ldflags="-w -s -X github.com/scanoss/folder-hashing-api/entities.AppVersion=$(VERSION)" -o ./scripts/scanoss-hfh-api ./cmd/server
	bash ./package-scripts.sh linux-amd64 $(VERSION)

package_arm: version  ## Build & Package an ARM 64 binary
	@echo "Building ARM binary $(VERSION) and placing into scripts..."
	GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -ldflags="-w -s -X github.com/scanoss/folder-hashing-api/entities.AppVersion=$(VERSION)" -o ./scripts/scanoss-hfh-api ./cmd/server
	bash ./package-scripts.sh linux-arm64 $(VERSION)
