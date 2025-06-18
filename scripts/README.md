# SCANOSS Folder Hashing API Deployment Support

This folder contains utilities for deploying, configuring and running the SCANOSS Folder Hashing API service with its Qdrant knowledge base.

## Overview

The Folder Hashing API uses a hybrid deployment approach with **separated infrastructure and data setup**:
- **Infrastructure**: System folders, services, clean Qdrant container
- **Data Import**: Collection-based snapshots imported via REST API
- **HFH API Service**: Runs as traditional systemd service
- **Knowledge Base**: Customer provides collection snapshots containing component fingerprints

## Core Scripts

### Service Setup & Management
- **`env_setup.sh`** - Sets up infrastructure (folders, services, clean Qdrant)
- **`create-package.sh`** - Creates distribution packages for customers
- **`scanoss-hfh-api.service`** - Systemd service definition
- **`scanoss-hfh-api.sh`** - Service startup script

### Knowledge Base Management
- **`create-collection-snapshots.sh`** - Creates individual collection snapshots
- **`import-collections.sh`** - Imports collection snapshots via REST API

### Configuration & Templates
- **`qdrant-docker-compose.yml`** - Docker Compose template for Qdrant
- **`COLLECTION_BASED_SETUP.md`** - Detailed setup documentation

## Installation Workflow

### For Customer (New Installation):

1. **Extract Package**:
   ```bash
   tar -xzf scanoss-hfh-api-linux_amd64-1.0.0-1.tar.gz
   cd scanoss-hfh-api-linux_amd64-1.0.0-1
   ```

2. **Setup Infrastructure**:
   ```bash
   # Create system user
   sudo useradd --system scanoss
   
   # Setup infrastructure (no data import)
   sudo ./scripts/env_setup.sh prod
   ```

3. **Configure Service**:
   ```bash
   # Copy and customize configuration
   sudo cp /usr/local/etc/scanoss/hfh/config.example.json /usr/local/etc/scanoss/hfh/app-config-prod.json
   sudo nano /usr/local/etc/scanoss/hfh/app-config-prod.json
   ```

4. **Import Knowledge Base**:
   ```bash
   # Import collection snapshots
   ./scripts/import-collections.sh /path/to/collection-snapshots/
   ```

5. **Start Service**:
   ```bash
   # Start the API service
   sudo systemctl start scanoss-hfh-api
   
   # Verify installation
   curl http://localhost:40061/health
   ```

### For SCANOSS (Distribution Creation):

1. **Create Collection Snapshots**:
   ```bash
   # From running Qdrant with knowledge base
   ./scripts/create-collection-snapshots.sh collection-snapshots/
   ```

2. **Create Distribution Package**:
   ```bash
   # Build and package
   ./scripts/create-package.sh linux_amd64 1.0.0
   ```

3. **Distribute**:
   - Send package + collection snapshots to customer
   - Customer follows installation workflow above

## Key Advantages of New Approach

### ✅ **Separation of Concerns**
- Infrastructure setup is independent of data import
- Can verify each step works before proceeding
- Easier troubleshooting when issues occur

### ✅ **Reliability**
- No "File exists" errors from CLI restoration
- Uses REST API for all data operations
- Better error handling per collection

### ✅ **Flexibility**
- Can restart infrastructure without losing data setup
- Can import partial collections for testing
- Easy to retry failed imports

## Prerequisites

- **System**: Linux x86_64 or ARM64
- **Docker**: Docker and Docker Compose installed
- **Memory**: Minimum 32GB RAM
- **Storage**: 100GB+ available disk space
- **Access**: Root access for installation
- **User**: `scanoss` system user created

## Directory Structure

After installation:

```
/usr/local/etc/scanoss/hfh/          # API configuration
├── config.example.json              # Configuration template
└── app-config-prod.json             # Your configuration

/usr/local/etc/scanoss/qdrant/       # Qdrant setup
├── docker-compose.yml               # Docker configuration
├── data/                            # Qdrant data
└── snapshots/                       # Temporary import files

/var/log/scanoss/hfh/                # API logs
/usr/local/bin/scanoss-hfh-api       # API binary
/etc/systemd/system/scanoss-hfh-api.service  # Service definition
```

## Configuration Methods

The API supports multiple configuration approaches:

1. **JSON Configuration** (recommended):
   ```bash
   sudo cp config.example.json /usr/local/etc/scanoss/hfh/app-config-prod.json
   sudo nano /usr/local/etc/scanoss/hfh/app-config-prod.json
   ```

2. **Environment Variables**:
   ```bash
   # Set in systemd service or shell environment
   export QDRANT_HOST=localhost
   export QDRANT_PORT=6334
   ```

3. **.env File**:
   ```bash
   # Create .env file and configure service to use it
   sudo cp .env.example /usr/local/etc/scanoss/hfh/.env-prod
   ```

## Service Management

```bash
# Start service
sudo systemctl start scanoss-hfh-api

# Stop service
sudo systemctl stop scanoss-hfh-api

# Check status
sudo systemctl status scanoss-hfh-api

# View logs
sudo journalctl -u scanoss-hfh-api -f

# View Qdrant logs
docker logs scanoss-qdrant
```

## API Endpoints

After successful installation:
- **REST API**: http://localhost:40061
- **gRPC API**: localhost:50061
- **Health Check**: http://localhost:40061/health
- **Qdrant Dashboard**: http://localhost:6333/dashboard

## Troubleshooting

### Infrastructure Issues
```bash
# Check Docker
sudo systemctl status docker

# Check Qdrant container
docker ps --filter name=scanoss-qdrant

# Check system folders
ls -la /usr/local/etc/scanoss/
```

### Data Import Issues
```bash
# Check import logs
cat collection-snapshots/.restoration_log

# Retry failed imports
./scripts/import-collections.sh collection-snapshots/

# Check collections
curl http://localhost:6333/collections
```

### Service Issues
```bash
# Check service status
sudo systemctl status scanoss-hfh-api

# Check configuration
sudo nano /usr/local/etc/scanoss/hfh/app-config-prod.json

# Check API connectivity
curl http://localhost:40061/health
```

## Monthly Updates

For knowledge base updates:

1. **Receive new collection snapshots** from SCANOSS
2. **Import new data**:
   ```bash
   # Stop service
   sudo systemctl stop scanoss-hfh-api
   
   # Import new snapshots
   ./scripts/import-collections.sh new-collection-snapshots/
   
   # Start service
   sudo systemctl start scanoss-hfh-api
   ```

## Migration from Old Approach

If you were using the old full storage snapshot approach:

1. **Create collection snapshots** from your working instance
2. **Clean install** using new infrastructure approach
3. **Import collections** using new REST API method

See `COLLECTION_BASED_SETUP.md` for detailed migration instructions.

## Support

- **Documentation**: See `COLLECTION_BASED_SETUP.md` for detailed setup guide
- **Logs**: Check service and container logs for troubleshooting
- **API Docs**: Visit https://docs.scanoss.com for API documentation
