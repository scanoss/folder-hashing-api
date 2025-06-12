# SCANOSS Platform 2.0 High Precision Folder Hashing Service
Welcome to the High Precision Folder Hashing server for SCANOSS Platform 2.0

**Warning** Work In Progress **Warning**

## Repository Structure
This repository is made up of the following components:
* ?

## Configuration

The server supports multiple configuration sources with the following priority order:

1. **Environment variables** (highest priority - overrides everything)
2. **JSON configuration file** (medium priority - base configuration)  
3. **.env file** (lowest priority)
4. **Default values** (fallback)

### Command Line Usage

```bash
# Use JSON config with custom env file
./server --json-config config.json --env-config production.env

# Use JSON config only (environment variables can still override)
./server --json-config config.json

# Use env file only
./server --env-config .env.production

# Enable debug mode
./server --debug

# Show version
./server --version
```

### Key Configuration Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `APP_NAME` | "SCANOSS HFH Server" | Application name |
| `APP_PORT` | "50061" | gRPC server port |
| `REST_PORT` | "40061" | REST server port |
| `APP_MODE` | "dev" | Application mode (dev/production) |
| `APP_DEBUG` | false | Enable debug logging |
| `QDRANT_HOST` | "localhost" | Qdrant database host |
| `QDRANT_PORT` | 6334 | Qdrant database port |

### JSON Configuration Format

```json
{
  "App": {
    "Name": "SCANOSS HFH Server",
    "GRPCPort": "50061",
    "RESTPort": "40061",
    "Mode": "dev"
  },
  "Hfh": {
    "QdrantHost": "localhost",
    "QdrantPort": 6334
  }
}
```

See `config.example.json` for a complete configuration example.

## Docker Environment

### How to build

```bash
make ghcr_build
```

### How to run

```bash
# With JSON config
docker run -it -v "$(pwd)":"$(pwd)" -p 50061:50061 ghcr.io/scanoss/scanoss-hfh-api --json-config $(pwd)/config.json --debug

# With environment variables
docker run -it --env-file .env -p 50061:50061 ghcr.io/scanoss/scanoss-hfh-api --debug
```

## Development

```bash
# Run with JSON config
go run cmd/server/main.go --json-config config.example.json --debug

# Run with environment variables only
go run cmd/server/main.go --debug
```
