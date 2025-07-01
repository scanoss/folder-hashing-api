# SCANOSS Folder Hashing API - Docker Deployment Guide

This directory contains Docker-based deployment scripts and utilities for the SCANOSS Folder Hashing API. The new approach uses **Docker Compose** for simplified deployment and eliminates the complexity of systemd-based installations.

## 🐳 Docker-First Architecture

The SCANOSS HFH API now uses a modern containerized approach:

- **Zero System Dependencies**: No systemd, no user management, no complex permissions
- **Single Command Deployment**: Start everything with one command
- **Environment Isolation**: Clean separation between development and production
- **Consistent Deployments**: Same environment everywhere
- **Easy Maintenance**: Standard Docker tooling for management

## 📁 Scripts Overview

### Core Deployment Scripts

- **`docker-deploy.sh`** - Main deployment script for all environments
- **`create-docker-package.sh`** - Creates Docker-based distribution packages
- **`create-collection-snapshots.sh`** - Exports collections for backup/distribution
- **`import-collections.sh`** - Imports collection snapshots into Qdrant
- **`setup-tls.sh`** - Configures TLS/SSL encryption for secure communications

### Legacy Scripts (Removed)

The following systemd-based scripts have been removed in favor of Docker deployment:

- ~~`env_setup.sh`~~ - Replaced by Docker Compose
- ~~`scanoss-hfh-api.service`~~ - Replaced by Docker Compose services
- ~~`scanoss-hfh-api.sh`~~ - Replaced by container orchestration
- ~~`create-package.sh`~~ - Replaced by `create-docker-package.sh`

## 🚀 Quick Start

### 1. Development Environment

```bash
# Start development environment
./scripts/docker-deploy.sh dev

# Or using make
make dev
```

### 2. Production Environment

```bash
# Start production environment
./scripts/docker-deploy.sh prod

# Or using make
make prod
```

### 3. Import Collections

```bash
# Import collection snapshots
./scripts/import-collections.sh /path/to/snapshots/
```

## 📋 Detailed Usage

### docker-deploy.sh

Main deployment script with support for multiple environments and actions:

```bash
./scripts/docker-deploy.sh [environment] [action]

# Environments:
#   dev, development  - Development environment with debug logging
#   prod, production  - Production environment with resource limits

# Actions:
#   up       - Start services (default)
#   down     - Stop services
#   logs     - View service logs
#   status   - Check service status
#   restart  - Restart services
#   pull     - Update images

# Examples:
./scripts/docker-deploy.sh prod up       # Start production
./scripts/docker-deploy.sh dev logs     # View development logs
./scripts/docker-deploy.sh prod status  # Check production status
```

**Features:**
- ✅ Automatic prerequisite checking (Docker, Docker Compose)
- ✅ Health monitoring with timeout handling
- ✅ Configuration file validation and auto-creation
- ✅ Service dependency management
- ✅ Comprehensive error handling and logging

### create-docker-package.sh

Creates distribution packages for offline deployment:

```bash
./scripts/create-docker-package.sh <platform> [version]

# Platforms:
#   linux/amd64  - AMD64 architecture
#   linux/arm64  - ARM64 architecture  
#   multi        - Multi-architecture (buildx required)

# Examples:
./scripts/create-docker-package.sh linux/amd64        # AMD64 package
./scripts/create-docker-package.sh linux/arm64 1.2.3 # ARM64 with version
./scripts/create-docker-package.sh multi             # Multi-arch package
```

**Package Contents:**
- Docker Compose configuration files
- Pre-built Docker images (saved as tar files)
- Configuration templates
- Deployment and management scripts
- Collection management utilities
- Complete documentation

### create-collection-snapshots.sh

Exports Qdrant collections for backup or distribution:

```bash
./scripts/create-collection-snapshots.sh [output-dir]

# Examples:
./scripts/create-collection-snapshots.sh                # Default: collection-snapshots/
./scripts/create-collection-snapshots.sh backups/      # Custom directory
```

**Features:**
- ✅ Individual collection snapshots (not monolithic)
- ✅ REST API based (reliable)
- ✅ Progress monitoring with timeouts
- ✅ Automatic cleanup of temporary files
- ✅ Detailed logging and error handling

### import-collections.sh

Imports collection snapshots into Qdrant:

```bash
./scripts/import-collections.sh <snapshots-dir>

# Examples:
./scripts/import-collections.sh collection-snapshots/
./scripts/import-collections.sh /path/to/customer/data/
```

**Features:**
- ✅ Docker-aware (works with containers)
- ✅ No sudo required (uses Docker group permissions)
- ✅ Individual collection restoration
- ✅ Progress monitoring and health checks
- ✅ Automatic retry logic for failed imports

### setup-tls.sh

Configures TLS/SSL encryption for secure communications:

```bash
./scripts/setup-tls.sh <cert-file> <key-file>

# Example:
./scripts/setup-tls.sh /path/to/cert.pem /path/to/key.pem
```

**Features:**
- ✅ Automatic certificate directory creation
- ✅ Secure file permissions (600 for private key)
- ✅ TLS configuration template generation
- ✅ Health check auto-detection of HTTPS

## 🔐 TLS/SSL Configuration

### Quick TLS Setup

1. **Prepare your certificate files**:
   ```bash
   # You need:
   # - A certificate file (e.g., cert.pem)
   # - A private key file (e.g., key.pem)
   ```

2. **Run the TLS setup script**:
   ```bash
   ./scripts/setup-tls.sh /path/to/cert.pem /path/to/key.pem
   ```

3. **Use the TLS configuration**:
   ```bash
   cp ./config/app-config-tls.json ./config/app-config.json
   ```

4. **Deploy with TLS enabled**:
   ```bash
   ./scripts/docker-deploy.sh prod
   ```

### TLS Configuration Details

Add the TLS section to your `app-config.json`:

```json
{
  "TLS": {
    "CertFile": "/app/certs/cert.pem",
    "KeyFile": "/app/certs/key.pem",
    "CN": "your-domain.com"
  }
}
```

Or use environment variables:
- `COMP_TLS_CERT`: Path to certificate file
- `COMP_TLS_KEY`: Path to private key file
- `COMP_TLS_CN`: Common Name (optional)

### Testing TLS

```bash
# Test REST API over HTTPS
curl -v https://localhost:40061/api/v2/scanning/echo \
  -X POST -H "Content-Type: application/json" \
  -d '{"message":"TLS test"}'

# For self-signed certificates
curl -vk https://localhost:40061/api/v2/scanning/echo \
  -X POST -H "Content-Type: application/json" \
  -d '{"message":"TLS test"}'
```

For more detailed TLS configuration and troubleshooting, see [docs/TLS-SETUP.md](../docs/TLS-SETUP.md).

## 🏗️ Architecture Overview

### Docker Compose Structure

```yaml
services:
  qdrant:      # Vector database
    - Data persistence via named volumes
    - Health checks and auto-restart
    - Optimized for production performance
    
  hfh-api:     # SCANOSS API service
    - Depends on Qdrant health
    - Multi-stage Docker build
    - Non-root container security
    - Environment-specific configurations
```

### Volume Management

```yaml
volumes:
  qdrant_data:       # Database storage
  qdrant_snapshots:  # Snapshot storage  
  hfh_logs:          # Application logs
```

### Network Configuration

```yaml
networks:
  scanoss-network:   # Isolated network for services
```

## 🔧 Configuration Management

### Environment-Specific Configurations

**Development (`docker-compose.dev.yml`)**:
- Debug logging enabled
- Source code mounting for development
- Faster restart policies
- Enhanced telemetry

**Production (`docker-compose.prod.yml`)**:
- Security-optimized settings
- Production logging levels
- Auto-restart policies

### Configuration Files

```
config/
├── app-config.example.json    # JSON configuration template
├── app-config-tls.json       # TLS-enabled config (created by setup-tls.sh)
├── .env.example              # Environment variables template
└── certs/                    # TLS certificates directory (created by setup-tls.sh)
    ├── cert.pem           # TLS certificate
    └── key.pem           # Private key
```

## 📦 Distribution Workflow

### For SCANOSS (Package Creation)

```bash
# 1. Create collection snapshots from your knowledge base
./scripts/create-collection-snapshots.sh distribution-snapshots/

# 2. Create distribution packages
./scripts/create-docker-package.sh linux/amd64 1.0.0
./scripts/create-docker-package.sh linux/arm64 1.0.0

# 3. Distribute packages + snapshots to customers
```

### For Customers (Package Deployment)

```bash
# 1. Extract package
tar -xzf scanoss-hfh-api-docker-amd64-1.0.0-1.tar.gz
cd scanoss-hfh-api-docker-amd64-1.0.0-1

# 2. Load Docker images (offline support)
./scripts/load-images.sh

# 3. Configure service
cp config/app-config.example.json config/app-config.json
# Edit configuration as needed

# 4. Deploy services
./scripts/deploy.sh prod

# 5. Import knowledge base
./scripts/import-collections.sh /path/to/collection-snapshots/

# 6. Verify deployment
./scripts/verify-installation.sh
```

## 🛠️ Management Commands

### Service Management

```bash
# Start services
./scripts/docker-deploy.sh prod up

# Stop services  
./scripts/docker-deploy.sh prod down

# Restart services
./scripts/docker-deploy.sh prod restart

# View logs
./scripts/docker-deploy.sh prod logs

# Check status
./scripts/docker-deploy.sh prod status
```

### Data Management

```bash
# Export collections
./scripts/create-collection-snapshots.sh snapshots/

# Import collections
./scripts/import-collections.sh snapshots/

# Verify collections
curl http://localhost:6333/collections
```

### Maintenance

```bash
# Update images
./scripts/docker-deploy.sh prod pull

# Clean up resources
docker-compose down -v
docker system prune -f

# Reset environment
make env_reset
```

## 🔍 Troubleshooting

### Common Issues

**Docker daemon not running:**
```bash
# Check Docker status
docker info

# Start Docker service (varies by system)
sudo systemctl start docker     # Linux systemd
sudo service docker start       # Linux SysV
```

**Port conflicts:**
```bash
# Check port usage
netstat -tlnp | grep -E ':(40061|50061|6333|6334)'

# Stop conflicting services
docker-compose down
```

**Permission issues:**
```bash
# Add user to docker group (logout/login required)
sudo usermod -aG docker $USER

# Or use newgrp to apply immediately
newgrp docker
```

### Debugging Commands

```bash
# View container logs
docker logs scanoss-hfh-api
docker logs scanoss-qdrant

# Check container status
docker ps --filter name=scanoss

# Inspect container configuration
docker inspect scanoss-hfh-api

# Check resource usage
docker stats --no-stream
```

### Service Health Checks

```bash
# Check Qdrant
curl http://localhost:6333/collections

# Check HFH API
curl -X POST -H "Content-Type: application/json" -d '{"message":"test"}' http://localhost:40061/api/v2/scanning/echo

# Check all services
./scripts/docker-deploy.sh prod status
```

### Qdrant Optimization

**For Large Datasets**:
```yaml
environment:
  - QDRANT__SERVICE__MAX_REQUEST_SIZE_MB=64
  - QDRANT__SERVICE__GRPC_TIMEOUT_MS=120000
```

**For High Performance**:
```yaml
volumes:
  - type: tmpfs
    target: /tmp
    tmpfs:
      size: 1G      # Use tmpfs for temporary operations
```

## 📊 Monitoring

### Built-in Health Checks

All services include health checks that monitor:
- Service responsiveness
- Database connectivity
- API endpoint availability
- Resource utilization

### Log Management

```bash
# View real-time logs
./scripts/docker-deploy.sh prod logs

# View specific service logs
docker logs -f scanoss-hfh-api
docker logs -f scanoss-qdrant

# Export logs for analysis
docker logs scanoss-hfh-api > hfh-api.log 2>&1
```

### Metrics Collection

Enable telemetry in configuration for metrics:
```json
{
  "Telemetry": {
    "Enabled": true,
    "OltpExporter": "0.0.0.0:4317"
  }
}
```

## 🔐 Security Considerations

### Container Security

- **Non-root containers**: All services run as non-privileged users
- **Minimal base images**: Debian slim for reduced attack surface
- **Read-only filesystems**: Where applicable
- **Resource limits**: Prevent resource exhaustion

### Network Security

- **Isolated networks**: Services communicate via dedicated Docker network
- **Port exposure**: Only necessary ports exposed to host
- **Internal communication**: Services use container names for resolution
- **TLS encryption**: Optional TLS/SSL support for all endpoints

### Data Security

- **Volume encryption**: Consider encrypting Docker volumes for sensitive data
- **Access controls**: Use proper file permissions on host-mounted volumes
- **Network policies**: Implement firewall rules for exposed ports
- **Certificate security**: Private keys stored with 600 permissions

## 💡 Migration from Legacy Deployment

If you're migrating from the old systemd-based deployment:

### 1. Stop Legacy Services
```bash
sudo systemctl stop scanoss-hfh-api
sudo systemctl disable scanoss-hfh-api
```

### 2. Export Existing Data
```bash
# If you have existing Qdrant data, export it first
./scripts/create-collection-snapshots.sh migration-backup/
```

### 3. Deploy Docker Version
```bash
# Set up Docker deployment
make env_setup
make prod
```

### 4. Import Data
```bash
# Import your existing data
./scripts/import-collections.sh migration-backup/
```

### 5. Verify Migration
```bash
# Verify everything works
./scripts/docker-deploy.sh prod status
curl -X POST -H "Content-Type: application/json" -d '{"message":"test"}' http://localhost:40061/api/v2/scanning/echo
```

## 🤝 Support

For issues with Docker deployment:

1. **Check the logs**: `./scripts/docker-deploy.sh prod logs`
2. **Verify prerequisites**: Docker and Docker Compose installed and running
3. **Check port availability**: Ensure ports 40061, 50061, 6333, 6334 are free
4. **Review configuration**: Validate your `config/app-config.json`
5. **Test connectivity**: Use curl commands to test service endpoints

## 📚 Additional Resources

- **Docker Documentation**: [https://docs.docker.com](https://docs.docker.com)
- **Docker Compose Reference**: [https://docs.docker.com/compose](https://docs.docker.com/compose)
- **Qdrant Documentation**: [https://qdrant.tech/documentation](https://qdrant.tech/documentation)
- **SCANOSS Documentation**: [https://docs.scanoss.com](https://docs.scanoss.com)

---

**The Docker-first approach simplifies deployment, improves reliability, and provides a consistent environment across all deployments. Welcome to the future of SCANOSS HFH API deployment! 🚀**