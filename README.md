# SCANOSS Folder Hashing API

[![License](https://img.shields.io/badge/License-GPL%20v2%2B-blue.svg)](LICENSE)
[![Docker](https://img.shields.io/badge/Docker-Supported-blue.svg)](docker-compose.yml)
[![Go Version](https://img.shields.io/badge/Go-1.22+-00ADD8.svg)](go.mod)

A high-performance REST and gRPC API service for component fingerprinting and similarity matching using Qdrant vector database. The SCANOSS Folder Hashing API enables efficient code component analysis and similarity detection for software composition analysis.

## 🚀 Quick Start with Docker

The easiest way to deploy SCANOSS HFH API is using Docker Compose:

```bash
# 1. Clone the repository
git clone https://github.com/scanoss/folder-hashing-api.git
cd folder-hashing-api

# 2. Set up configuration
mkdir -p config
cp config.example.json config/app-config.json
# Edit config/app-config.json as needed

# 3. Deploy with Docker Compose
make prod
# or alternatively: ./scripts/docker-deploy.sh prod

# 4. Verify deployment
curl -X POST -H "Content-Type: application/json" -d '{"message":"test"}' http://localhost:40061/api/v2/scanning/echo
```

🎉 Your SCANOSS HFH API is now running!

## 📦 Service Endpoints

| Service | Endpoint | Description |
|---------|----------|-------------|
| **REST API** | http://localhost:40061 | Main API interface |
| **gRPC API** | localhost:50061 | High-performance gRPC interface |
| **Dynamic Logging** | localhost:60061 | Runtime log level control |
| **Qdrant API** | http://localhost:6333 | Vector database API |
| **Qdrant Dashboard** | http://localhost:6333/dashboard | Database management UI |

## 🐳 Docker-First Architecture

This project uses a **Docker-first approach** for simplified deployment and management:

- **Single Command Deployment**: `make prod` or `make dev`
- **Environment Isolation**: Development and production configurations
- **Automated Health Checks**: Built-in service monitoring
- **Volume Persistence**: Data survives container restarts
- **Resource Management**: Production-optimized resource limits

### Development Environment

```bash
# Start development environment with debug logging
make dev

# View logs
make docker_logs

# Check status
make docker_status

# Stop services
make docker_down
```

### Production Environment

```bash
# Start production environment
make prod

# Check health
make docker_health

# View production logs
./scripts/docker-deploy.sh prod logs

# Stop services
./scripts/docker-deploy.sh prod down
```

## 📊 Collection Management

The API supports importing and exporting vector collections for knowledge base management:

### Export Collections

```bash
# Export all collections to snapshots
make collections_export

# Or directly:
./scripts/create-collection-snapshots.sh snapshots/
```

### Import Collections

```bash
# Import collection snapshots
make collections_import

# Or directly:
./scripts/import-collections.sh /path/to/snapshots/
```

### Import from CSV Files

The `cmd/import/main.go` tool allows you to import component data from CSV files directly into Qdrant collections:

```bash
# Build the import tool
go build -o dist/import-tool cmd/import/main.go

# Import CSV files from a directory
./dist/import-tool \
  -dir /path/to/csv/directory \
  -top-purls /path/to/top-purls.json \
  [-overwrite]

# Options:
#   -dir        Directory containing CSV files to import (required)
#   -top-purls  JSON file with top-rated PURLs for ranking (required)
#   -overwrite  Delete existing collections before import (optional)
```

**CSV File Format:**
Each CSV file should contain component data with the following columns:
- Columns 0-2: Hash values (dirs, names, contents)
- Column 3: URL hash
- Columns 4-10: Component metadata (vendor, component, version, etc.)
- Columns 11-16: File metrics and category
- Column 17: Language extensions (JSON format)

**Top PURLs File:**
A JSON file mapping PURLs to ranking scores for prioritizing search results:
```json
{
  "pkg:github/apache/commons-lang": 1,
  "pkg:npm/react": 2,
  "pkg:pypi/requests": 3
}
```

The import tool will:
- Process CSV files in parallel using multiple workers
- Group components by programming language into separate collections
- Create optimized vector indexes for fast similarity search
- Handle large datasets with configurable batch processing

### Verify Collections

```bash
# Check collection status
make collections_verify

# Manual verification
curl http://localhost:6333/collections
```

## ⚙️ Configuration

The API supports multiple configuration methods:

### 1. JSON Configuration (Recommended)

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
    "QdrantHost": "qdrant",
    "QdrantPort": 6334
  },
  "Telemetry": {
    "Enabled": false,
    "OltpExporter": "0.0.0.0:4317"
  }
}
```

### 2. Environment Variables

```bash
# Core settings
export APP_PORT=50061
export REST_PORT=40061
export QDRANT_HOST=qdrant
export QDRANT_PORT=6334

# Debug settings
export APP_DEBUG=true
export APP_TRACE=true
```

### 3. Command Line Flags

```bash
# Using JSON config
./dist/scanoss-hfh-api --json-config config/app-config.json

# Using environment file
./dist/scanoss-hfh-api --env-config .env

# With debug flag
./dist/scanoss-hfh-api --debug --json-config config/app-config.json
```

## 🔧 Development

### Prerequisites

- **Docker & Docker Compose**: Container runtime
- **Go 1.22+**: For local development
- **Make**: Build automation

### Local Development Setup

```bash
# Set up development environment
make env_setup

# Start development services
make dev

# Run tests
make test

# Run linting
make lint_local

# Build locally
make build_amd
```

### Building and Testing

```bash
# Run all tests
make test

# Run unit tests locally
make unit_test

# Build Docker image
make docker_build

# Build binary
make build_amd  # for AMD64
make build_arm  # for ARM64
```

## 📦 Distribution Packages

Create Docker-based distribution packages for offline deployment:

```bash
# Create AMD64 package
make docker_package_amd

# Create ARM64 package
make docker_package_arm

# Create multi-architecture package
make docker_package_multi

# Create all packages
make docker_package_all
```

### Customer Deployment

Distribution packages include everything needed for offline deployment:

```bash
# Extract package
tar -xzf scanoss-hfh-api-docker-amd64-1.0.0-1.tar.gz
cd scanoss-hfh-api-docker-amd64-1.0.0-1

# Load Docker images
./scripts/load-images.sh

# Configure service
cp config/app-config.example.json config/app-config.json
# Edit configuration as needed

# Deploy
./scripts/deploy.sh prod

# Import knowledge base (if available)
./scripts/import-collections.sh /path/to/snapshots/
```

## 🛠️ Available Make Targets

<details>
<summary>View all available commands</summary>

```bash
# Development Workflow
make help                 # Show this help
make dev                  # Quick start development environment
make prod                 # Quick start production environment
make test                 # Run tests in Docker environment
make lint_local           # Run local linting
make clean_testcache      # Clean Go test caches

# Docker Operations
make docker_build         # Build Docker image
make docker_up            # Start production services
make docker_up_dev        # Start development services
make docker_down          # Stop all services
make docker_logs          # View service logs
make docker_status        # Check service status
make docker_clean         # Clean up Docker resources

# Package Creation
make package              # Create Docker distribution package
make docker_package_amd   # Create AMD64 package
make docker_package_arm   # Create ARM64 package
make docker_package_multi # Create multi-arch package

# Collection Management
make collections_export   # Export collections
make collections_import   # Import collections
make collections_verify   # Verify collections

# Environment Management
make env_setup            # Setup development environment
make env_clean            # Clean up Docker resources
make env_reset            # Reset entire environment

# Debugging
make docker_health        # Check service health
make docker_debug_qdrant  # Debug Qdrant issues
make docker_debug_api     # Debug API issues
```

</details>

## 🔍 Troubleshooting

### Common Issues

**Services won't start:**
```bash
# Check Docker status
docker info

# Check service logs
make docker_logs

# Reset environment
make env_reset
```

**Qdrant connection issues:**
```bash
# Debug Qdrant container
make docker_debug_qdrant

# Check Qdrant health
curl http://localhost:6333/collections
```

**API not responding:**
```bash
# Debug API container
make docker_debug_api

# Check API health
curl -X POST -H "Content-Type: application/json" -d '{"message":"test"}' http://localhost:40061/api/v2/scanning/echo
```

## 📚 Documentation

- **API Documentation**: Available at REST endpoints when service is running
- **Docker Deployment**: See `docker-compose.yml` files for detailed configuration
- **Collection Management**: Check `scripts/` directory for data management tools
- **Configuration Reference**: See `config.example.json` for all available options

## 🤝 Contributing

1. Fork the repository
2. Create a feature branch (`git checkout -b feature/amazing-feature`)
3. Make your changes
4. Commit your changes (`git commit -m 'Add amazing feature'`)
5. Push to the branch (`git push origin feature/amazing-feature`)
6. Open a Pull Request

## 📄 License

This project is licensed under the GPL v2+ License - see the [LICENSE](LICENSE) file for details.

## 🔗 Links

- **SCANOSS Website**: [https://www.scanoss.com](https://www.scanoss.com)
- **Documentation**: [https://docs.scanoss.com](https://docs.scanoss.com)
- **Docker Hub**: [https://hub.docker.com/r/scanoss/folder-hashing-api](https://hub.docker.com/r/scanoss/folder-hashing-api)
- **GitHub Container Registry**: [ghcr.io/scanoss/folder-hashing-api](https://ghcr.io/scanoss/folder-hashing-api)

---

**Built with ❤️ by the SCANOSS Team**
