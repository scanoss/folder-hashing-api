# SCANOSS Folder Hashing API Deployment Support

This folder contains convenience utilities for deploying, configuring and running the SCANOSS Folder Hashing API service with its Qdrant knowledge base.

## Overview

The Folder Hashing API uses a hybrid deployment approach:
- **Qdrant Database**: Runs in Docker container with customer-provided knowledge base snapshots
- **HFH API Service**: Runs as traditional systemd service
- **Knowledge Base**: Customer provides their own Qdrant snapshots containing component fingerprints

## Distribution Process

### For SCANOSS (Distribution Creation):

1. **Package**: `./create-package.sh linux_amd64 1.0.0` or `make package_amd 1.0.0`
   - Automatically builds binary for specified platform
   - Creates lightweight package with scripts + binary only
   - No snapshot or config files included
2. **Distribute**: Send `.tar.gz` package to customer

### For Customer (Installation):

1. **Extract**: `tar -xzf scanoss-hfh-api-linux_amd64-1.0.0-1.tar.gz`
2. **Install with snapshot**: `sudo ./scripts/env_setup.sh prod /path/to/your-snapshot.snapshot`
3. **Configure**: Customize `config.example.json` or provide your own config

### For Monthly Updates:

1. **Receive new snapshot** from SCANOSS
2. **Update**: `sudo ./scripts/update-snapshot.sh new-snapshot.snapshot`
3. **Automatic backup/rollback** on failure

## Setup

The scripts folder contains an `env_setup.sh` script which attempts to do the following:

1. Set up the default folders and permissions
2. Install and configure Qdrant with knowledge base snapshot
3. Set up service registration (`scanoss-hfh-api.service`)
4. Copy in binaries (if `scanoss-hfh-api` exists in the binaries folder)
5. Copy in preferred configuration (if `app-config-prod.json` exists in the config folder)

Logs are written by default to `/var/log/scanoss/hfh/scanoss-hfh-prod.log`.

Configuration is written by default to: `/usr/local/etc/scanoss/hfh`.

Qdrant data and snapshots are stored in: `/usr/local/etc/scanoss/qdrant`.

## Prerequisites

- Docker and Docker Compose installed
- Minimum 32GB RAM // TODO: Update this
- 90GB+ disk space for knowledge base
- `scanoss` system user created (`useradd --system scanoss`)

## Installation

Running the `env_setup.sh` on the target server takes care of installation. You must provide the path to your snapshot file:

```bash
./env_setup.sh [environment] [snapshot_path]
```

**Examples:**
```bash
# Production installation
sudo ./env_setup.sh prod /path/to/scanoss-kb-2025-01-15.snapshot

# Development installation  
sudo ./env_setup.sh dev /home/user/snapshots/latest.snapshot
```

This will:
- Copy configuration template to `/usr/local/etc/scanoss/hfh`
- Copy binaries to `/usr/local/bin`
- Copy service registration to `/etc/systemd/system`
- Set up Qdrant with your provided knowledge base snapshot
- Redirect logging to `/var/log/scanoss/hfh`

## Configuration

After installation, you need to configure the service. The installation copies `config.example.json` as a template. You have several options:

**Option A: Edit the example config**
```bash
sudo cp /usr/local/etc/scanoss/hfh/config.example.json /usr/local/etc/scanoss/hfh/app-config-prod.json
sudo nano /usr/local/etc/scanoss/hfh/app-config-prod.json
```

**Option B: Use your own JSON config**
```bash
sudo cp your-config.json /usr/local/etc/scanoss/hfh/app-config-prod.json
```

**Option C: Use .env file**
```bash
sudo cp your-app.env /usr/local/etc/scanoss/hfh/.env
# Modify /usr/local/bin/scanoss-hfh-api.sh to use --env-config flag
```

**Option D: Use environment variables**
```bash
# Set environment variables in the systemd service file
```

The API supports multiple configuration methods (in priority order):
1. **Environment variables** (highest priority)
2. **JSON config file** via `--json-config` flag
3. **.env file** via `--env-config` flag
4. **Default values** (lowest priority)

## Knowledge Base Updates

Monthly knowledge base updates are supported through the snapshot update mechanism:

1. Receive new snapshot file from SCANOSS
2. Run the update script: `./update-snapshot.sh`
3. The script will backup current data and restore the new snapshot

## Service Management

After installation, manage the service using standard systemd commands:

```bash
# Start the service
systemctl start scanoss-hfh-api

# Stop the service
systemctl stop scanoss-hfh-api

# Check status
systemctl status scanoss-hfh-api

# View logs
journalctl -u scanoss-hfh-api -f
```

## API Endpoints

After successful installation:
- **HFH API**: http://localhost:40061 (REST) and localhost:50061 (gRPC)
- **Qdrant Dashboard**: http://localhost:6333/dashboard

## Troubleshooting

1. **Qdrant not starting**: Check Docker service and available disk space
2. **API can't connect to Qdrant**: Verify Qdrant is running on localhost:6334
3. **Low memory errors**: Ensure system has minimum 32GB RAM
4. **Snapshot restore fails**: Check snapshot file integrity and available disk space

## File Structure

```
/usr/local/etc/scanoss/hfh/          # API configuration
/usr/local/etc/scanoss/qdrant/       # Qdrant data and snapshots
/var/log/scanoss/hfh/                # API logs
/usr/local/bin/scanoss-hfh-api       # API binary
/etc/systemd/system/scanoss-hfh-api.service  # Service definition
