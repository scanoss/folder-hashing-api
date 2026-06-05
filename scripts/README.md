# SCANOSS Folder Hashing API - Deployment Scripts

This directory contains deployment and management scripts for the SCANOSS Folder Hashing API. The service is deployed using systemd on Linux servers, with AWX for automated orchestration.

## 📁 Scripts Overview

### Deployment Scripts

- **`env-setup.sh`** - System setup script that prepares the server environment for deployment
- **`scanoss-folder-hashing-api.sh`** - Service startup script that manages log rotation and launches the API
- **`scanoss-folder-hashing-api.service`** - Systemd service definition file

### Qdrant Snapshot Scripts

- **`qdrant-generate-snapshots.sh`** - Creates a Qdrant snapshot of every collection and downloads each one to a local `snapshots/` directory
- **`qdrant-restore-snapshots.sh`** - Recreates the database by uploading local snapshot files back to Qdrant

### Supporting Infrastructure

The project includes `docker-compose.qdrant.yml` in the root directory for running the Qdrant vector database:

```bash
# Start Qdrant vector database
docker-compose -f docker-compose-qdrant.yml up -d
```

## 🚀 Installation

### Prerequisites

- Linux server with systemd
- Go 1.22+ for building the binary
- Docker for Qdrant database
- `scanoss` system user

### Manual Installation Steps

1. **Create the runtime user**:
   ```bash
   sudo useradd --system scanoss
   ```

2. **Build the API binary**:
   ```bash
   go build -o scanoss-folder-hashing-api
   ```

3. **Run the setup script**:
   ```bash
   # Interactive mode (prompts for confirmations)
   sudo ./scripts/env-setup.sh

   # Force mode (automated, no prompts)
   sudo ./scripts/env-setup.sh --force
   ```

4. **Configure the service**:
   Edit the configuration file at `/usr/local/etc/scanoss/folder-hashing-api/app-config-prod.json`

5. **Start the service**:
   ```bash
   sudo systemctl start scanoss-folder-hashing-api
   sudo systemctl enable scanoss-folder-hashing-api
   ```

## 📋 Script Details

### env-setup.sh

Prepares the server environment by setting up directories, copying files, and configuring permissions.

**Usage:**
```bash
sudo ./scripts/env-setup.sh [-h|--help] [-f|--force] [environment]

Options:
  -h, --help    Show help message
  -f, --force   Run without interactive prompts (for automation)
  [environment] Optional environment suffix (e.g., "dev", "staging")
```

**What it does:**
- Creates required directories:
  - `/usr/local/etc/scanoss/folder-hashing-api` - Configuration
  - `/var/log/scanoss/folder-hashing-api` - Logs
  - `/var/lib/scanoss/db/sqlite/folder-hashing-api` - Database (if needed)
- Copies systemd service file to `/etc/systemd/system/`
- Copies startup script to `/usr/local/bin/`
- Copies binary to `/usr/local/bin/scanoss-folder-hashing-api`
- Sets proper ownership and permissions for the `scanoss` user
- Optionally downloads default configuration if not present

**Directory Structure:**
```
/usr/local/etc/scanoss/folder-hashing-api/
  └── app-config-prod.json

/var/log/scanoss/folder-hashing-api/
  └── scanoss-folder-hashing-api-prod.log

/usr/local/bin/
  ├── scanoss-folder-hashing-api
  └── scanoss-folder-hashing-api.sh

/etc/systemd/system/
  └── scanoss-folder-hashing-api.service
```

### scanoss-folder-hashing-api.sh

Startup script executed by systemd that handles log rotation and launches the API.

**Features:**
- Rotates logs on startup (compresses old logs with gzip)
- Launches the API with the correct configuration file
- Redirects output to log file

**Default Paths:**
- Log: `/var/log/scanoss/folder-hashing-api/scanoss-folder-hashing-api-prod.log`
- Config: `/usr/local/etc/scanoss/folder-hashing-api/app-config-prod.json`

### scanoss-folder-hashing-api.service

Systemd service unit file for managing the API as a system service.

**Service Configuration:**
- Runs as the `scanoss` user
- Automatically restarts on failure (5 second delay)
- Starts after network is available

**Service Management:**
```bash
# Start service
sudo systemctl start scanoss-folder-hashing-api

# Stop service
sudo systemctl stop scanoss-folder-hashing-api

# Restart service
sudo systemctl restart scanoss-folder-hashing-api

# Check status
sudo systemctl status scanoss-folder-hashing-api

# View logs
sudo journalctl -u scanoss-folder-hashing-api -f

# Enable on boot
sudo systemctl enable scanoss-folder-hashing-api
```

## 🗄️ Database Setup

### Qdrant Vector Database

Start Qdrant using Docker Compose:

```bash
docker-compose -f docker-compose-qdrant.yml up -d
```

This starts Qdrant with:
- HTTP API on port 6333
- gRPC API on port 6334
- Persistent storage volumes

### Populating the Database

There are two ways to populate the vector database: import raw data from CSV files, or restore previously generated snapshots.

#### Option 1: Import from CSV

Use the `cmd/import` tool to populate the vector database with component data:

```bash
# Build the import tool
go build -o dist/import-tool cmd/import/main.go

# Import CSV data (-top-purls is optional)
./dist/import-tool \
  -dir /path/to/csv/files

# Import CSV data with an optional PURL ranking file to prioritize results
./dist/import-tool \
  -dir /path/to/csv/files \
  -top-purls /path/to/top-purls.json

# Recreate database from scratch
./dist/import-tool \
  -dir /path/to/csv/files \
  -overwrite
```

> **Note:** The `-top-purls` file is **optional**. When omitted, the `rank` column from the CSV is used as-is.

For detailed information about the import process, see the [main README](../README.md#importing-data).

#### Option 2: Restore from Snapshots

Restoring from snapshots is the fastest way to provision a new environment from an existing dataset, since it skips bulk loading and vector indexing.

```bash
# Generate snapshots of every collection (default output: ./snapshots)
./scripts/qdrant-generate-snapshots.sh

# Recreate the database from those snapshots
./scripts/qdrant-restore-snapshots.sh
```

Both scripts accept an optional snapshots directory as the first argument and honor the `QDRANT_HTTP` environment variable to target a different endpoint (default `http://localhost:6333`). `jq` is required.

```bash
# Custom snapshots directory and remote Qdrant endpoint
./scripts/qdrant-generate-snapshots.sh /data/backups
QDRANT_HTTP=http://my-qdrant-host.example.com:6333 ./scripts/qdrant-restore-snapshots.sh /data/backups
```

Snapshot files are named `<collection>.snapshot`; the restore script derives the collection name from the file name, so keep that naming convention. The restore uses `priority=snapshot`, so the uploaded snapshot wins over any existing data in the collection.

For more details, see the [main README](../README.md#restoring-from-snapshots).

## 📦 Packaging

### Creating a Distribution Package

Use the `package-scripts.sh` in the root directory:

```bash
# Create package for AMD64
./package-scripts.sh linux_amd64 1.0.0

# Create package for ARM64
./package-scripts.sh linux_arm64 1.0.0
```

This creates a tar archive containing all scripts for deployment on target servers.

## ⚙️ Configuration

### JSON Configuration

The service uses a JSON configuration file located at:
`/usr/local/etc/scanoss/folder-hashing-api/app-config-prod.json`

**Example configuration:**
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
  "Telemetry": {
    "Enabled": false,
    "OltpExporter": "0.0.0.0:4317"
  }
}
```

### Environment Variables

The service also supports environment-based configuration:

```bash
export APP_PORT=50061
export REST_PORT=40061
export QDRANT_HOST=localhost
export QDRANT_PORT=6334
export APP_DEBUG=false
```

## 🔍 Troubleshooting

### Check Service Status

```bash
# Systemd status
sudo systemctl status scanoss-folder-hashing-api

# View recent logs
sudo journalctl -u scanoss-folder-hashing-api -n 100

# Follow logs in real-time
sudo journalctl -u scanoss-folder-hashing-api -f
```

### Check Log Files

```bash
# View current log
sudo tail -f /var/log/scanoss/folder-hashing-api/scanoss-folder-hashing-api-prod.log
```

### Common Issues

**Qdrant connection errors:**
- Verify Qdrant is running: `docker ps | grep qdrant`
- Check connectivity: `curl http://localhost:6333/collections`
- Review Qdrant logs: `docker logs scanoss-qdrant`
