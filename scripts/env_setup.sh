#!/bin/bash

##########################################
#
# This script sets up the SCANOSS Folder Hashing API infrastructure:
# - Creates system folders and permissions
# - Installs service files and binaries
# - Starts a clean Qdrant container (no data import)
# 
# Data import is handled separately with: ./scripts/import-collections.sh
#
# Config goes into: /usr/local/etc/scanoss/hfh
# Logs go into: /var/log/scanoss/hfh
# Service definition goes into: /etc/systemd/system
# Binary & startup go into: /usr/local/bin
# Qdrant setup in: /usr/local/etc/scanoss/qdrant
#
################################################################

if [ "$1" = "-h" ] || [ "$1" = "-help" ]; then
    echo "$0 [-help] [environment]"
    echo "   Setup SCANOSS Folder Hashing API infrastructure (no data import)"
    echo ""
    echo "Arguments:"
    echo "   [environment]   optional environment suffix (default: prod)"
    echo ""
    echo "Examples:"
    echo "   $0 prod"
    echo "   $0 dev"
    echo ""
    echo "After infrastructure setup, import data separately:"
    echo "   ./scripts/import-collections.sh /path/to/collection-snapshots/"
    exit 1
fi

DEFAULT_ENV="prod"
ENVIRONMENT="${1:-$DEFAULT_ENV}"

# Store the original working directory for later use
ORIGINAL_DIR=$(pwd)

export C_PATH=/usr/local/etc/scanoss/hfh
export LOG_DIR=/var/log/scanoss
export L_PATH="${LOG_DIR}/hfh"
export QDRANT_PATH=/usr/local/etc/scanoss/qdrant
export RUNTIME_USER=scanoss

echo "🚀 SCANOSS Folder Hashing API Infrastructure Setup"
echo "=================================================="
echo "Environment: $ENVIRONMENT"
echo

# Check prerequisites
echo "Checking prerequisites..."

# Check for Docker
if ! command -v docker &>/dev/null; then
    echo "❌ Docker is required but not installed."
    echo "Please install Docker first: https://docs.docker.com/engine/install/"
    exit 1
fi

# Check for Docker Compose
if ! command -v docker-compose &>/dev/null; then
    echo "❌ Docker Compose is required but not installed."
    echo "Please install Docker Compose first."
    exit 1
fi

# Check if Docker daemon is running
if ! docker info &>/dev/null; then
    echo "❌ Docker daemon is not running."
    echo "Please start Docker service: systemctl start docker"
    exit 1
fi

echo "✅ Prerequisites check passed"

# Makes sure the scanoss user exists
if ! getent passwd $RUNTIME_USER >/dev/null; then
    echo "Runtime user does not exist: $RUNTIME_USER"
    echo "Please create using: useradd --system $RUNTIME_USER"
    exit 1
fi

# Also, make sure we're running as root
if [ "$EUID" -ne 0 ]; then
    echo "Please run as root"
    exit 1
fi

read -p "Install SCANOSS Folder Hashing API $ENVIRONMENT infrastructure (y/n) [n]? " -n 1 -r
echo
if [[ $REPLY =~ ^[Yy]$ ]]; then
    echo "Starting infrastructure setup..."
else
    echo "Stopping."
    exit 1
fi

# Setup all the required folders and ownership
echo "📁 Setting up system folders..."
if ! mkdir -p $C_PATH; then
    echo "mkdir failed for $C_PATH"
    exit 1
fi
if ! mkdir -p $L_PATH; then
    echo "mkdir failed for $L_PATH"
    exit 1
fi
if ! mkdir -p $QDRANT_PATH; then
    echo "mkdir failed for $QDRANT_PATH"
    exit 1
fi

# Create Qdrant subdirectories
mkdir -p "$QDRANT_PATH/data"
mkdir -p "$QDRANT_PATH/snapshots"

if [ "$RUNTIME_USER" != "root" ]; then
    echo "Changing ownership of $LOG_DIR to $RUNTIME_USER ..."
    if ! chown -R $RUNTIME_USER $LOG_DIR; then
        echo "chown of $LOG_DIR to $RUNTIME_USER failed"
        exit 1
    fi
fi

# Setup clean Qdrant container (no data import)
echo "🐳 Setting up clean Qdrant container..."

# Create docker-compose.yml for clean Qdrant startup (no CLI arguments)
echo "📝 Creating Qdrant Docker configuration..."
cat > "$QDRANT_PATH/docker-compose.yml" << EOF
version: '3.8'

services:
  qdrant:
    image: qdrant/qdrant:v1.11.0
    container_name: scanoss-qdrant
    ports:
      - "6333:6333"  # HTTP API port
      - "6334:6334"  # gRPC API port
    volumes:
      - ./data:/qdrant/storage
    restart: unless-stopped
    environment:
      - QDRANT__SERVICE__HTTP_PORT=6333
      - QDRANT__SERVICE__GRPC_PORT=6334
      - QDRANT__LOG_LEVEL=INFO
      - QDRANT__SERVICE__MAX_REQUEST_SIZE_MB=32
      - QDRANT__SERVICE__GRPC_TIMEOUT_MS=60000
    healthcheck:
      test: ["CMD", "curl", "-f", "http://localhost:6333"]
      interval: 30s
      timeout: 10s
      retries: 5
      start_period: 30s
    # Clean startup - no CLI arguments, no data import
    command: ["./qdrant"]

networks:
  default:
    name: qdrant_default
EOF

# Stop any existing Qdrant containers
echo "🛑 Stopping any existing Qdrant containers..."
docker stop scanoss-qdrant 2>/dev/null || true
docker rm scanoss-qdrant 2>/dev/null || true

# Clean up any existing networks
docker network rm qdrant_default 2>/dev/null || true

# Clean up any Docker volumes that might persist data
echo "🧹 Cleaning up Docker volumes..."
docker volume ls -q --filter name=qdrant | xargs -r docker volume rm 2>/dev/null || true

# Clean up existing data directory for fresh start
echo "🧹 Cleaning existing Qdrant data for fresh start..."
if [ -d "$QDRANT_PATH/data" ]; then
    find "$QDRANT_PATH/data" -mindepth 1 -delete 2>/dev/null || true
    rm -rf "$QDRANT_PATH/data" 2>/dev/null || true
fi

# Recreate clean data directory
mkdir -p "$QDRANT_PATH/data"
chmod 755 "$QDRANT_PATH/data"

# Start clean Qdrant container
echo "🚀 Starting clean Qdrant container..."
cd "$QDRANT_PATH"
docker-compose up -d

# Wait for Qdrant to be ready
echo "⏳ Waiting for Qdrant to be ready..."
timeout=120 # 2 minutes should be enough for clean startup
counter=0
while [ $counter -lt $timeout ]; do
    if curl -f http://localhost:6333 >/dev/null 2>&1; then
        echo "✅ Qdrant is ready!"
        break
    fi
    if [ $((counter % 15)) -eq 0 ]; then
        echo "Still waiting for Qdrant... ($counter/$timeout seconds)"
    fi
    sleep 3
    counter=$((counter + 3))
done

if [ $counter -ge $timeout ]; then
    echo "❌ Timeout waiting for Qdrant to be ready"
    echo "📋 Container logs:"
    docker logs scanoss-qdrant 2>/dev/null || echo "Could not retrieve logs"
    echo "🔍 Container status:"
    docker ps -a --filter name=scanoss-qdrant
    exit 1
fi

# Verify clean state
echo "🔍 Verifying clean Qdrant state..."
COLLECTIONS_RESPONSE=$(curl -s http://localhost:6333/collections 2>/dev/null || echo '{"result":{"collections":[]}}')
if echo "$COLLECTIONS_RESPONSE" | grep -q '"status":"ok"'; then
    COLLECTION_COUNT=$(echo "$COLLECTIONS_RESPONSE" | grep -o '"name":"[^"]*"' | wc -l)
    echo "✅ Qdrant started with $COLLECTION_COUNT collections (should be 0 for clean start)"
else
    echo "⚠️  Could not verify Qdrant state, but it's responding"
fi

# Setup the service on the system
SC_SERVICE_FILE="scanoss-hfh-api.service"
SC_SERVICE_NAME="scanoss-hfh-api"
if [ -n "$ENVIRONMENT" ] && [ "$ENVIRONMENT" != "prod" ]; then
    SC_SERVICE_FILE="scanoss-hfh-api-${ENVIRONMENT}.service"
    SC_SERVICE_NAME="scanoss-hfh-api-${ENVIRONMENT}"
fi

export service_stopped=""
if [ -f "/etc/systemd/system/$SC_SERVICE_FILE" ]; then
    echo "🛑 Stopping $SC_SERVICE_NAME service first..."
    if ! systemctl stop "$SC_SERVICE_NAME"; then
        echo "service stop failed"
        exit 1
    fi
    export service_stopped="true"
fi

echo "📋 Copying service startup config..."
if [ -f "$ORIGINAL_DIR/scripts/$SC_SERVICE_FILE" ]; then
    if ! cp "$ORIGINAL_DIR/scripts/$SC_SERVICE_FILE" /etc/systemd/system; then
        echo "service copy failed"
        exit 1
    fi
else
    echo "Service file $ORIGINAL_DIR/scripts/$SC_SERVICE_FILE not found"
    exit 1
fi

if ! cp "$ORIGINAL_DIR/scripts/scanoss-hfh-api.sh" /usr/local/bin; then
    echo "hfh api startup script copy failed"
    exit 1
fi
chmod +x /usr/local/bin/scanoss-hfh-api.sh

# Copy configuration template for customer to customize
CONF=app-config-prod.json
if [ -n "$ENVIRONMENT" ] && [ "$ENVIRONMENT" != "prod" ]; then
    CONF="app-config-${ENVIRONMENT}.json"
fi

echo "📝 Copying configuration template to $C_PATH ..."
if [ -f "$ORIGINAL_DIR/config.example.json" ]; then
    if ! cp "$ORIGINAL_DIR/config.example.json" "$C_PATH/"; then
        echo "copy config.example.json failed"
        exit 1
    fi
    echo "✅ Configuration template copied to $C_PATH/config.example.json"
    echo "📝 Please customize and rename to $CONF before starting the service"
else
    echo "⚠️  config.example.json not found in package"
    echo "📝 Please create your config file at: $C_PATH/$CONF"
fi

# Copy the binary if available
BINARY=scanoss-hfh-api
if [ -f "$ORIGINAL_DIR/dist/$BINARY" ]; then
    echo "📦 Copying app binary to /usr/local/bin ..."
    if ! cp "$ORIGINAL_DIR/dist/$BINARY" /usr/local/bin; then
        echo "copy dist/$BINARY failed"
        echo "Please make sure the service is stopped: systemctl stop $SC_SERVICE_NAME"
        exit 1
    fi
    chmod +x /usr/local/bin/$BINARY
    echo "✅ Binary installed successfully"
else
    echo "⚠️  Binary not found in dist/ directory"
    echo "📝 Please copy the API binary file to: /usr/local/bin/$BINARY"
fi

# Reload systemd and enable service
systemctl daemon-reload
systemctl enable "$SC_SERVICE_NAME"

echo ""
echo "🎉 SCANOSS Folder Hashing API infrastructure setup complete!"
echo ""
echo "📊 Infrastructure Status:"
echo "  ✅ System folders created"
echo "  ✅ Clean Qdrant container running"
echo "  ✅ Service files installed"
echo "  ✅ Configuration template available"
if [ -f "/usr/local/bin/$BINARY" ]; then
    echo "  ✅ API binary installed"
else
    echo "  ⚠️  API binary needs to be installed manually"
fi
echo ""
echo "🔧 Container Status:"
docker ps --filter name=scanoss-qdrant --format "table {{.Names}}\t{{.Status}}\t{{.Ports}}"
echo ""
echo "📋 Next Steps:"
echo ""
echo "1. 📝 Configure the service:"
echo "   sudo cp $C_PATH/config.example.json $C_PATH/$CONF"
echo "   sudo nano $C_PATH/$CONF"
echo ""
echo "2. 📥 Import your knowledge base data:"
echo "   ./scripts/import-collections.sh /path/to/collection-snapshots/"
echo ""
echo "3. 🚀 Start the API service:"
echo "   sudo systemctl start $SC_SERVICE_NAME"
echo ""
echo "🌐 Endpoints (after data import and service start):"
echo "  - REST API: http://localhost:40061"
echo "  - gRPC API: http://localhost:50061"
echo "  - Qdrant Dashboard: http://localhost:6333/dashboard"
echo ""
echo "📋 Management Commands:"
echo "  - View service status: systemctl status $SC_SERVICE_NAME"
echo "  - View service logs: journalctl -u $SC_SERVICE_NAME -f"
echo "  - View Qdrant logs: docker logs scanoss-qdrant"
echo ""
