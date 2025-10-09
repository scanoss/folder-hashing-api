# SCANOSS Folder Hashing API

[![License](https://img.shields.io/badge/License-GPL%20v2%2B-blue.svg)](LICENSE)
[![Go Version](https://img.shields.io/badge/Go-1.22+-00ADD8.svg)](go.mod)

A high-performance REST and gRPC API service for component fingerprinting and similarity matching using Qdrant vector database. The SCANOSS Folder Hashing API enables efficient code component analysis and similarity detection for software composition analysis.

## Prerequisites

- **Go 1.22+**: For building and running the service
- **Qdrant**: Vector database (must be running and accessible)

## Quick Start

```bash
# 1. Clone the repository
git clone https://github.com/scanoss/folder-hashing-api.git
cd folder-hashing-api

# 2. Set up configuration
cp config.example.json config/app-config.json
# Edit config/app-config.json as needed

# 3. Build the service
make build_amd  # or build_arm for ARM64

# 4. Run the service
./dist/scanoss-hfh-api --json-config config/app-config.json

# 5. Verify it's running
curl -X POST -H "Content-Type: application/json" -d '{"message":"test"}' http://localhost:40061/api/v2/scanning/echo
```

## Service Endpoints

Once running, the service provides:

| Service | Default Endpoint | Description |
|---------|----------|-------------|
| **REST API** | http://localhost:40061 | Main API interface |
| **gRPC API** | localhost:50061 | High-performance gRPC interface |
| **Dynamic Logging** | localhost:60061 | Runtime log level control |

## Configuration

### JSON Configuration (Recommended)

```json
{
  "App": {
    "Name": "SCANOSS HFH Server",
    "GRPCPort": "50061",
    "RESTPort": "40061",
    "Debug": false,
    "Mode": "production"
  },
  "Hfh": {
    "QdrantHost": "localhost",
    "QdrantPort": 6334
  },
  "Logging": {
    "DynamicLogging": true,
    "DynamicPort": "localhost:60061"
  },
  "Telemetry": {
    "Enabled": false,
    "OltpExporter": "0.0.0.0:4317"
  }
}
```

### Environment Variables

```bash
export APP_PORT=50061
export REST_PORT=40061
export QDRANT_HOST=localhost
export QDRANT_PORT=6334
export APP_DEBUG=true
```

### Command Line Flags

```bash
# Using JSON config
./dist/scanoss-hfh-api --json-config config/app-config.json

# Using environment file
./dist/scanoss-hfh-api --env-config .env

# With debug flag
./dist/scanoss-hfh-api --debug --json-config config/app-config.json
```

## Building

```bash
# Build for AMD64
make build_amd

# Build for ARM64
make build_arm

# Run locally (development)
make run_local
```

## Testing

```bash
# Run all tests
make test

# Run with coverage
go test -v -cover ./...

# Run linting
make lint_local

# Auto-fix linting issues
make lint_local_fix
```

## Importing Data

The `cmd/import/main.go` tool allows you to populate the Qdrant vector database with component data from CSV files.

### Basic Usage

```bash
# Build the import tool
go build -o dist/import-tool cmd/import/main.go

# Update database (adds/updates data in existing collections)
./dist/import-tool \
  -dir /path/to/csv/directory \
  -top-purls /path/to/top-purls.json

# Recreate database (deletes existing collections and imports fresh)
./dist/import-tool \
  -dir /path/to/csv/directory \
  -top-purls /path/to/top-purls.json \
  -overwrite

# Specify Qdrant host and port
./dist/import-tool \
  -dir /path/to/csv/directory \
  -top-purls /path/to/top-purls.json \
  -overwrite \
  -qdrant-host my-qdrant-host.example.com \
  -qdrant-port 5555
```

### Command Options

| Flag | Required | Description |
|------|----------|-------------|
| `-dir` | Yes | Directory containing CSV files to import |
| `-top-purls` | Yes | JSON file with PURL rankings for search prioritization |
| `-overwrite` | No | Delete and recreate all collections (use for fresh start) |

### How It Works

The import tool:
- Processes CSV files in parallel using 12 concurrent workers
- Groups components by programming language into separate collections (e.g., `py_collection`, `js_collection`)
- Creates optimized vector indexes with named vectors (`dirs`, `names`, `contents`)
- Handles large datasets with batching (2000 records per batch)

### Example Workflow

```bash
# 1. Ensure Qdrant is running
# (Start your Qdrant instance)

# 2. Build the tool
go build -o dist/import-tool cmd/import/main.go

# 3. Import your data
./dist/import-tool \
  -dir /data/csv/ \
  -top-purls /data/top-purls.json

# 4. Verify collections were created
curl http://localhost:6333/collections
```

## Development

### Local Development Setup

```bash
# Install dependencies
go mod download

# Run tests
make test

# Run linting
make lint

# Build locally
make build_amd64

# Run locally
make run
```

### Available Make Targets

```bash
make help                 # Show all available commands
make build_amd64          # Build for AMD64
make build_arm64          # Build for ARM64
make run                  # Run the service locally
make test                 # Run all unit tests
make lint                 # Run linting
make lint-fix             # Run linting with auto-fix
make clean_testcache      # Clean Go test caches
```

## Troubleshooting

### API not responding

```bash
# Check if service is running
ps aux | grep scanoss-hfh-api

# Check configuration
cat config/app-config.json

# Run with debug logging
./dist/scanoss-hfh-api --debug --json-config config/app-config.json
```

### Qdrant connection issues

```bash
# Verify Qdrant is accessible
curl http://localhost:6333/collections

# Check Qdrant host/port in config
grep -A 3 "Hfh" config/app-config.json
```

### Import tool issues

```bash
# Verify CSV directory exists and contains files
ls -la /path/to/csv/directory/

# Verify top-purls.json is valid JSON
cat /path/to/top-purls.json | jq .

# Run with verbose output
./dist/import-tool -dir /path/to/csv/ -top-purls /path/to/top-purls.json
```

## Documentation

- **API Documentation**: Available at REST endpoints when service is running
- **Configuration Reference**: See `config.example.json` for all available options
- **Scripts**: Check `scripts/` directory for additional utilities

## Contributing

1. Fork the repository
2. Create a feature branch (`git checkout -b feature/amazing-feature`)
3. Make your changes
4. Run tests (`make test`)
5. Run linting (`make lint_local`)
6. Commit your changes (`git commit -m 'Add amazing feature'`)
7. Push to the branch (`git push origin feature/amazing-feature`)
8. Open a Pull Request

## License

This project is licensed under the GPL v2+ License - see the [LICENSE](LICENSE) file for details.

## Links

- **SCANOSS Website**: [https://www.scanoss.com](https://www.scanoss.com)
- **Documentation**: [https://docs.scanoss.com](https://docs.scanoss.com)
- **GitHub**: [https://github.com/scanoss/folder-hashing-api](https://github.com/scanoss/folder-hashing-api)

---

**Built with ❤️ by the SCANOSS Team**
