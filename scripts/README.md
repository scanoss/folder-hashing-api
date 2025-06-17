# SCANOSS Folder Hashing API Deployment Support

This folder contains convenience utilities for deploying, configuring and running the SCANOSS Folder Hashing API service with its Qdrant knowledge base.

## Overview

The Folder Hashing API uses a hybrid deployment approach:
- **Qdrant Database**: Runs in Docker container with pre-loaded knowledge base snapshots
- **HFH API Service**: Runs as traditional systemd service
- **Knowledge Base**: Distributed as Qdrant snapshots containing millions of component fingerprints

## Distribution Process

### For SCANOSS (Distribution Creation):

1. **Create snapshot**: `./scripts/create-snapshot.sh`
2. **Build binary**: Place in `dist/scanoss-hfh-api`
3. **Package**: `./package-scripts.sh linux_amd64 1.0.0`
4. **Distribute**: Send `.tar.gz` package to customer

### For Customer (Installation):

1. **Extract**: `tar -xzf scanoss-hfh-offline-linux_amd64-1.0.0-1.tar.gz`
2. **Install**: `sudo ./scripts/env_setup.sh`
3. **Verify**: `curl http://localhost:40061/health`

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
- Minimum 32GB RAM (for handling millions of component records)
- 100GB+ disk space for knowledge base
- `scanoss` system user created (`useradd --system scanoss`)

## Installation

Running the `env_setup.sh` on the target server takes care of installation. Simply run:

```bash
./env_setup.sh
```

This will:
- Copy configuration files to `/usr/local/etc/scanoss/hfh`
- Copy binaries to `/usr/local/bin`
- Copy service registration to `/etc/systemd/system`
- Set up Qdrant with knowledge base snapshot
- Redirect logging to `/var/log/scanoss/hfh`

## Multi-service Registration

If there is a need to deploy more than one HFH API service on the same server, this can be achieved by using a different ENVIRONMENT name.

Create a copy of the `scanoss-hfh-api.service` using the following command:

```bash
cp scanoss-hfh-api.service scanoss-hfh-api-<env>.service
```

Where `<env>` is the name of this edition of the service (i.e. dev).

The `app-config-prod.json` file will also need to be copied:

```bash
cp app-config-prod.json app-config-<env>.json
```

Note: Please remember to use different port numbers and Qdrant configurations.

Finally, run the environment setup script using:

```bash
./env_setup.sh <env>
```

This will search for these specific service & config files and place them in the correct location.

Details for starting/stopping the service will be displayed in the console at the end of installation.

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
- **Health Check**: http://localhost:40061/health

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
