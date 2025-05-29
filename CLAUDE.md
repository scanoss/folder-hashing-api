# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Development Commands

### Local Development
```bash
make run_local          # Launch API locally with JSON config
make run_local_env      # Launch API locally with .env config
go run cmd/server/main.go -json-config config/app-config-dev.json -debug
```

### Testing
```bash
make test               # Deploy Milvus and run full test suite
make unit_test          # Run unit tests only (go test -v ./pkg/...)
make clean_testcache    # Clear Go test cache
```

### Code Quality
```bash
make lint_local         # Run golangci-lint
make lint_local_fix     # Run golangci-lint with auto-fix
```

### Building & Packaging
```bash
make build_amd          # Build AMD64 binary
make build_arm          # Build ARM64 binary
make package            # Build and package binary
make ghcr_build         # Build GitHub container
```

## Architecture Overview

This is a **High Precision Folder Hashing Service** for SCANOSS Platform 2.0 that performs hierarchical folder analysis for open source component identification using SimHash algorithms and vector similarity search.

### Core Components

- **Entry Points**: `cmd/server/main.go` (gRPC server), `cmd/cli/main.go` (CLI tool)
- **Service Layer**: `pkg/service/hfh_service.go` (gRPC service implementation)
- **Business Logic**: `pkg/usecase/hfh.go` (three-stage scanning algorithm)
- **Data Layer**: `pkg/hfh/qdrant.go` (Qdrant vector DB), legacy Milvus support

### Three-Stage Analysis Algorithm

The core scanning logic in `pkg/usecase/hfh.go` implements a multi-stage approach:
1. **Stage 1**: Directory structure analysis (70% threshold)
2. **Stage 2**: File name pattern matching (60% threshold) 
3. **Stage 3**: Content-based hashing (51% threshold)

Each stage uses SimHash fingerprinting with configurable distance thresholds (HFH_TH1/TH2/TH3).

### Vector Database Migration

Currently migrating from Milvus to Qdrant (branch: `mdaloia/qdrant-implementation`):
- **Qdrant**: New implementation with named vectors (dirs, names, contents)
- **Milvus**: Legacy support in `pkg/usecase/milvus/`

### Configuration

Environment variables in priority order: `.env` → `env.json` → system env

Key settings:
- `APP_PORT=50061` (gRPC), `REST_PORT=40061` (REST gateway)
- `HFH_DMAX=30` (max distance threshold)
- `HFH_TH1=70`, `HFH_TH2=60`, `HFH_TH3=51` (stage thresholds)
- `HFH_URL_LIM=10000` (max URLs processed)

### Testing Requirements

Full testing requires Milvus deployment:
```bash
cd pkg/usecase/milvus && bash milvus_deploy.sh
```

Mock scripts available in `pkg/service/test/` for local testing.

### Dependencies

- Go 1.23.4+
- Qdrant client (`github.com/qdrant/go-client`)
- Milvus SDK (`github.com/milvus-io/milvus-sdk-go/v2`)
- SimHash (`github.com/mfonda/simhash`)
- gRPC with middleware, OpenTelemetry, Zap logging