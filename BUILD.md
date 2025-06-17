# SCANOSS Folder Hashing API - Build & Distribution Guide

This document describes the build and packaging process for offline distribution of the SCANOSS Folder Hashing API.

## Build Process

### 1. Build API Binary

```bash
# Build for AMD64
make build_amd

# Build for ARM64  
make build_arm

# Binary will be created in ./dist/
```

### 2. Create KB Snapshot

The snapshot file should be obtained from Qdrant instance:

```bash
# From a running Qdrant instance with an updated knowledge base
./scripts/create-snapshot.sh <output-dir>

# Place snapshot file anywhere accessible
# Default output directory is ./snapshots/<snapshot-name>.snapshot
```

### 3. Create Distribution Package

```bash
# Using environment variables for external artifacts
SNAPSHOT_PATH=/path/to/scanoss-kb-2025-01-15.snapshot make package_amd

# Or with custom paths
BINARY_PATH=./dist/scanoss-hfh-api-linux-amd64 \
SNAPSHOT_PATH=/path/to/snapshot.snapshot \
CONFIG_PATH=./config.example.json \
./package-scripts.sh linux_amd64 1.0.0
```

### 4. Result

This creates a complete offline package:
```
scanoss-hfh-offline-linux_amd64-1.0.0-1.tar.gz
```

## Customer Installation

Customers receive the `.tar.gz` file and install with:

```bash
tar -xzf scanoss-hfh-offline-linux_amd64-1.0.0-1.tar.gz
cd scanoss-hfh-offline-linux_amd64-1.0.0-1
sudo ./scripts/env_setup.sh
```

## Monthly Updates

For knowledge base updates:

```bash
sudo ./scripts/update-snapshot.sh new-snapshot.snapshot
```

## Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `BINARY_PATH` | `./dist/scanoss-hfh-api` | Path to compiled binary |
| `SNAPSHOT_PATH` | auto-detect in `./snapshots/` | Path to `.snapshot` file |
| `CONFIG_PATH` | `./config.example.json` | Path to configuration file |

## Development Workflow

1. **Development**: Work with source code, no artifacts committed
2. **Build**: `make build_amd` creates binary in `./dist/`
3. **Test**: Use local snapshot for testing
4. **Package**: `make package_amd` with production snapshot
5. **Distribute**: Send package to customers