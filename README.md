# SCANOSS Folder Hashing API

[![License](https://img.shields.io/badge/License-GPL%20v2%2B-blue.svg)](LICENSE)
[![Go Version](https://img.shields.io/badge/Go-1.23+-00ADD8.svg)](go.mod)

A high-performance REST and gRPC API service for component fingerprinting and similarity matching using Qdrant vector database. The SCANOSS Folder Hashing API enables efficient code component analysis and similarity detection for software composition analysis.

## Prerequisites

- **Go 1.23+**: For building and running the service
- **Docker + Docker Compose**: For running the Qdrant vector database
- **jq**: Required by the Qdrant snapshot scripts (optional otherwise)

## Setting up Qdrant

The service requires a running Qdrant instance. The repository ships a ready-to-use Docker Compose file ([`docker-compose-qdrant.yml`](docker-compose-qdrant.yml)) tuned for large-scale imports.

```bash
# 1. Start Qdrant (from the repository root)
docker compose -f docker-compose-qdrant.yml up -d

# 2. Verify it's up
curl http://localhost:6333/collections
docker logs scanoss-qdrant
```

This starts a `scanoss-qdrant` container with:

| Port | Protocol | Used by |
|------|----------|---------|
| 6333 | HTTP | Snapshot scripts, manual inspection (`curl http://localhost:6333/...`) |
| 6334 | gRPC | The API service and the import tool |

Data is persisted on the host in `./qdrant_data` (bind mount), so the database survives container restarts. To stop Qdrant: `docker compose -f docker-compose-qdrant.yml down` (data is kept).

> **Debian note:** Debian's own repositories ship neither the `docker compose` v2 plugin nor (on minimal installs) the AppArmor userspace tools that Docker needs. If containers fail to start with an `apparmor_parser: executable file not found` error, run `sudo apt-get install -y apparmor`. If `docker compose` is unavailable, either install Docker CE from Docker's official apt repository, or start Qdrant with a plain `docker run` using the same image, ports (`-p 6333:6333 -p 6334:6334`), volume (`-v $(pwd)/qdrant_data:/qdrant/storage`) and environment variables as the compose file.

Once Qdrant is running, populate it with data — see [Importing Data](#importing-data).

## Quick Start

```bash
# 1. Clone the repository
git clone https://github.com/scanoss/folder-hashing-api.git
cd folder-hashing-api

# 2. Start Qdrant (see "Setting up Qdrant" above)
docker compose -f docker-compose-qdrant.yml up -d

# 3. Set up configuration
cp config.example.json app-config.json
# Edit app-config.json as needed (Qdrant host/port, ports, etc.)

# 4. Build the service (binary goes into ./target)
make build_amd64   # or: make build_arm64

# 5. Run the service
./target/scanoss-folder-hashing-api-linux-amd64 --json-config app-config.json

# Alternatively, run directly from source during development:
make run

# 6. Verify it's running
curl -X POST -H "Content-Type: application/json" -d '{"message":"test"}' http://localhost:40061/v2/scanning/echo
```

## Service Endpoints

Once running, the service provides:

| Service | Default Endpoint | Description |
|---------|----------|-------------|
| **REST API** | http://localhost:40061 | Main API interface (grpc-gateway) |
| **gRPC API** | localhost:50061 | High-performance gRPC interface |
| **Dynamic Logging** | localhost:60061 | Runtime log level control |

REST routes (from the `scanoss/papi` scanning v2 definition):

| Method | Path | Description |
|--------|------|-------------|
| POST | `/v2/scanning/echo` | Echo test endpoint |
| POST | `/v2/scanning/hfh/scan` | Folder hash scan (similarity search) |

### Scan request example

```bash
curl -s -X POST -H "Content-Type: application/json" -d '{
  "rank_threshold": 10,
  "root": {
    "path_id": "/",
    "sim_hash_dir_names": "cfedb95e5bfdefab",
    "sim_hash_names":     "e8ab6d7fcbce5fe9",
    "sim_hash_content":   "827dc8fd1c6b0c57",
    "lang_extensions": {"py": 10}
  }
}' http://localhost:40061/v2/scanning/hfh/scan
```

Two parameters trip people up:

- **`rank_threshold` is required in practice.** Only components with `rank <= rank_threshold` are returned, and it defaults to `0` — which filters out every component with rank 1 or higher, i.e. **everything**, so an omitted threshold produces empty results even on a fully populated database. Set it to cover the ranks you want (lower rank = higher priority component).
- **`lang_extensions` selects which collection is searched.** The dominant extension routes the query (e.g. `{"py": 10}` searches `python_collection`, `{"class": 156}` searches `java_collection`), so it must be consistent with the folder being scanned — a mismatch searches the wrong collection and returns weak or empty matches. Folders with no mapped extensions go to `misc_collection`.

## Configuration

Configuration priority: **environment variables > JSON config file > `.env` file > built-in defaults**. If a `.env` file exists in the working directory it is picked up automatically.

### JSON Configuration (Recommended)

See [`config.example.json`](config.example.json) for a full example:

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

> **Note:** `Hfh.QdrantPort` must be the **gRPC** port of Qdrant (default `6334`), not the HTTP port (`6333`).

### Environment Variables

See [`.env.example`](.env.example) for the full list:

```bash
export APP_PORT=50061        # gRPC port
export REST_PORT=40061       # REST port
export QDRANT_HOST=localhost
export QDRANT_PORT=6334
export APP_DEBUG=true
```

### Command Line Flags

```bash
# Using JSON config
./target/scanoss-folder-hashing-api-linux-amd64 --json-config app-config.json

# Using environment file
./target/scanoss-folder-hashing-api-linux-amd64 --env-config .env

# With debug flag
./target/scanoss-folder-hashing-api-linux-amd64 --debug --json-config app-config.json

# Display version
./target/scanoss-folder-hashing-api-linux-amd64 --version
```

## Building

Binaries are written to the `./target` directory:

```bash
# Build the API for AMD64 -> target/scanoss-folder-hashing-api-linux-amd64
make build_amd64

# Build the API for ARM64 -> target/scanoss-folder-hashing-api-linux-arm64
make build_arm64

# Build both architectures
make build

# Build the import tool -> target/scanoss-folder-hashing-import-linux-amd64
make build_import_amd64   # or: make build_import_arm64

# Run locally from source (development)
make run
```

## Testing

```bash
# Run all tests (race detector + coverage profile)
make test

# Open the HTML coverage report
make test-coverage

# Run linting
make lint

# Auto-fix linting issues
make lint-fix

# Clear the Go test cache
make clean-testcache
```

## Importing Data

There are two ways to populate the Qdrant vector database:

1. **Import from CSV files** — build the database from raw component data using the import tool (`cmd/import`). Use this to create or update the data from scratch.
2. **Restore from snapshots** — recreate the database from previously generated Qdrant snapshots. This is much faster than a full CSV import and is the recommended way to provision a new environment from an existing dataset. See [Restoring from Snapshots](#restoring-from-snapshots).

### Basic Usage

```bash
# Build the import tool
make build_import_amd64

# Update database (adds/updates data in existing collections)
# -top-purls is optional; when omitted, the rank from the CSV is used
./target/scanoss-folder-hashing-import-linux-amd64 \
  -dir /path/to/csv/directory

# Update database with an optional PURL ranking file to prioritize results
./target/scanoss-folder-hashing-import-linux-amd64 \
  -dir /path/to/csv/directory \
  -top-purls /path/to/top-purls.json

# Recreate database (deletes existing collections and imports fresh)
./target/scanoss-folder-hashing-import-linux-amd64 \
  -dir /path/to/csv/directory \
  -overwrite

# Specify Qdrant host and port (defaults: localhost:6334)
./target/scanoss-folder-hashing-import-linux-amd64 \
  -dir /path/to/csv/directory \
  -top-purls /path/to/top-purls.json \
  -overwrite \
  -qdrant-host my-qdrant-host.example.com \
  -qdrant-port 6334
```

### Command Options

| Flag | Required | Description |
|------|----------|-------------|
| `-dir` | Yes | Directory containing CSV files to import |
| `-top-purls` | **No (optional)** | JSON file with PURL rankings for search prioritization. **When omitted, the `rank` column from the CSV is used as-is.** |
| `-overwrite` | No | Delete and recreate all collections (use for fresh start) |
| `-qdrant-host` | No | Qdrant server host (default `localhost`) |
| `-qdrant-port` | No | Qdrant server gRPC port (default `6334`) |

> **Note:** The `-top-purls` file is **optional**. It only overrides the `rank` of the matching PURLs to prioritize them in search results; if you don't provide it, the import relies entirely on the `rank` column already present in the CSV. The file is a JSON object mapping purl to rank, e.g. `{"pkg:github/torvalds/linux": 1, "pkg:npm/react": 1}`.

### CSV Format

Each CSV file is read as **headerless** and must contain exactly **13 columns** per row, in this order:

| Idx | Column | Type | Description |
|-----|--------|------|-------------|
| 0 | `hfh_dirs` | hex (16 chars / 64 bits) | Hash of directories, used for the `dirs` vector |
| 1 | `hfh_names` | hex | Hash of file names, used for the `names` vector |
| 2 | `hfh_contents` | hex | Hash of file contents, used for the `contents` vector |
| 3 | `url_hash` | hex (16 chars / 64 bits) | Internal source identifier — used in the Qdrant point ID, not exposed |
| 4 | `url_md5` | hex (32 chars / MD5) | MD5 of the source URL — exposed in the API response (as `url_hash` per version) |
| 5 | `purl` | string | Package URL — primary component key |
| 6 | `vendor` | string | Component vendor — exposed in the API response |
| 7 | `component` | string | Component name — exposed in the API response |
| 8 | `version` | string | Component version — exposed in the API response |
| 9 | `release_date` | string (`YYYY-MM-DD`) | Component release date — exposed per version in the API response |
| 10 | `license` | string (SPDX id, e.g. `ISC`, `MIT`) | License — exposed per version as a `License` object with `name` and `spdx_id` set to this value. Empty produces an empty list |
| 11 | `language_extensions` | JSON object `{ext: count}` or empty | Determines the target collection |
| 12 | `rank` | int | Selection priority — lower is better. Overridden by the `top-purls` file when matched |

Example row:

```csv
165fda3c6cc3bf1a,c4ed1d7ce8549a19,f57a5525acdaae94,854139ed027322d9,c4ac4ad84052612271d5995cd1553d6b,pkg:github/scanoss/scanoss.py,scanoss,scanoss.py,v1.19.0,2024-12-20,MIT,"{""py"":70,""json"":14,""md"":9}",1
```

Notes:
- Rows with fewer than 13 fields are skipped with a warning.
- Empty `language_extensions` routes the record to `misc_collection`.
- Invalid or empty `rank` defaults to `0`, which ranks higher than any positive value in the current sort — make sure the generator emits sanitized values.

### How It Works

The import tool:
- Processes CSV files in parallel; the number of workers (2–32) is calculated automatically from available CPU cores and memory
- Groups components by programming language into separate collections (e.g., `python_collection`, `javascript_collection`, `misc_collection`)
- Creates optimized vector indexes with named vectors (`dirs`, `names`, `contents`)
- Handles large datasets with batching (2000 records per batch, 1000 when running with more than 16 workers)
- Disables HNSW indexing during the bulk load and re-enables it at the end; Qdrant then builds the indexes in the background

### Example Workflow

```bash
# 1. Ensure Qdrant is running
docker compose -f docker-compose-qdrant.yml up -d

# 2. Build the tool
make build_import_amd64

# 3. Import your data (-top-purls is optional)
./target/scanoss-folder-hashing-import-linux-amd64 \
  -dir /data/csv/ \
  -top-purls /data/top-purls.json

# 4. Verify collections were created
curl http://localhost:6333/collections
```

### Restoring from Snapshots

Instead of importing from CSV, you can recreate the database from Qdrant snapshots. This is the fastest way to provision a new environment from an existing dataset, since it skips vector indexing and bulk loading.

Two helper scripts are provided in [`scripts/`](scripts/):

- `scripts/qdrant-generate-snapshots.sh` — creates a snapshot of every collection through the Qdrant HTTP API and downloads each one to a local `snapshots/<collection>.snapshot` file. The server-side snapshot is removed afterwards so it does not accumulate disk usage.
- `scripts/qdrant-restore-snapshots.sh` — uploads every `*.snapshot` file from the snapshots directory back to Qdrant, recreating (or overwriting) each collection from its file.

```bash
# 1. Ensure Qdrant is running
curl http://localhost:6333/collections

# 2. Generate snapshots of all collections (default output: ./snapshots)
./scripts/qdrant-generate-snapshots.sh

# 3. Restore / recreate the database from those snapshots
./scripts/qdrant-restore-snapshots.sh
```

Both scripts accept an optional snapshots directory as the first argument and honor the `QDRANT_HTTP` environment variable to target a different endpoint (default `http://localhost:6333`):

```bash
# Custom snapshots directory and remote Qdrant endpoint
./scripts/qdrant-generate-snapshots.sh /data/backups
QDRANT_HTTP=http://my-qdrant-host.example.com:6333 ./scripts/qdrant-restore-snapshots.sh /data/backups
```

Notes:
- Snapshot files are named `<collection>.snapshot`; the restore script derives the collection name from the file name, so keep that naming convention.
- The restore uses `priority=snapshot`, so the uploaded snapshot wins over any existing data in the collection.
- `jq` is required by both scripts.

## Production Deployment (systemd)

The service is deployed as a systemd unit on Linux servers using the scripts in [`scripts/`](scripts/). See [`scripts/README.md`](scripts/README.md) for full details.

```bash
# 1. Build the binary and package it with the deployment scripts
#    (produces scanoss-folder-hashing-api_linux-amd64_<version>-1.tgz;
#    the binary itself is placed into scripts/)
make package_amd64   # or: make package_arm64

# 2. Copy the archive to the target server and extract it
tar xzvf scanoss-folder-hashing-api_linux-amd64_<version>-1.tgz
cd scripts

# 3. Create the runtime user (once per server)
sudo useradd --system scanoss

# 4. Run the environment setup script
#    (installs binary + startup script to /usr/local/bin, systemd unit to
#    /etc/systemd/system, config to /usr/local/etc/scanoss/folder-hashing-api)
sudo ./env-setup.sh          # interactive
sudo ./env-setup.sh --force  # automated, no prompts

# 5. Review the configuration (Qdrant host/port, ports, telemetry)
sudo vi /usr/local/etc/scanoss/folder-hashing-api/app-config-prod.json

# 6. Start and enable the service
sudo systemctl start scanoss-folder-hashing-api
sudo systemctl enable scanoss-folder-hashing-api

# 7. Check status and logs
sudo systemctl status scanoss-folder-hashing-api
sudo journalctl -u scanoss-folder-hashing-api -f
sudo tail -f /var/log/scanoss/folder-hashing-api/scanoss-folder-hashing-api-prod.log
```

The target server also needs a running Qdrant instance (see [Setting up Qdrant](#setting-up-qdrant)) populated via CSV import or snapshot restore.

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
make run                  # Run the API locally from source
make test                 # Run all unit tests
make test-coverage        # Run tests and open HTML coverage report
make lint                 # Run linting
make lint-fix             # Run linting with auto-fix
make fmt                  # Format code (gofumpt + goimports)
make vet                  # Run go vet
make build                # Build API binaries for all architectures
make build_amd64          # Build API for AMD64
make build_arm64          # Build API for ARM64
make build_import_amd64   # Build import tool for AMD64
make build_import_arm64   # Build import tool for ARM64
make package_amd64        # Build & package AMD64 binary + deploy scripts
make package_arm64        # Build & package ARM64 binary + deploy scripts
make clean                # Clean build artifacts
make clean-testcache      # Clean Go test caches
make tidy                 # Tidy and verify dependencies
make version              # Display current version
```

## Troubleshooting

### API not responding

```bash
# Check if service is running
ps aux | grep scanoss-folder-hashing-api

# Check configuration
cat app-config.json

# Run with debug logging
./target/scanoss-folder-hashing-api-linux-amd64 --debug --json-config app-config.json
```

### Qdrant connection issues

```bash
# Verify the container is running
docker ps | grep qdrant

# Verify Qdrant is accessible
curl http://localhost:6333/collections

# Review Qdrant logs
docker logs scanoss-qdrant

# Check Qdrant host/port in config (must be the gRPC port, default 6334)
grep -A 3 "Hfh" app-config.json
```

### Import tool issues

```bash
# Verify CSV directory exists and contains files
ls -la /path/to/csv/directory/

# Verify top-purls.json is valid JSON
cat /path/to/top-purls.json | jq .

# Run the import again (progress and per-collection stats are printed)
./target/scanoss-folder-hashing-import-linux-amd64 -dir /path/to/csv/ -top-purls /path/to/top-purls.json
```

## Documentation

- **API Definition**: [scanoss/papi](https://github.com/scanoss/papi) (scanning v2 service)
- **Configuration Reference**: See `config.example.json` and `.env.example` for all available options
- **Deployment**: See [`scripts/README.md`](scripts/README.md) for deployment and management scripts

## Contributing

1. Fork the repository
2. Create a feature branch (`git checkout -b feature/amazing-feature`)
3. Make your changes
4. Run tests (`make test`)
5. Run linting (`make lint`)
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
