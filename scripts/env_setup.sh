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
    echo "   sudo ./scripts/import-collections.sh /path/to/collection-snapshots/"
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

# Check if required ports are available
echo "🔍 Checking port availability..."
PORTS_IN_USE=""
if netstat -tlnp 2>/dev/null | grep -q ":6333 "; then
    PORTS_IN_USE="$PORTS_IN_USE 6333"
fi
if netstat -tlnp 2>/dev/null | grep -q ":6334 "; then
    PORTS_IN_USE="$PORTS_IN_USE 6334"
fi

if [ -n "$PORTS_IN_USE" ]; then
    echo "⚠️  Warning: Required ports are in use:$PORTS_IN_USE"
    echo "📋 Processes using these ports:"
    for port in $PORTS_IN_USE; do
        echo "  Port $port:"
        netstat -tlnp 2>/dev/null | grep ":$port " | head -3
    done
    echo ""
    echo "🛑 Attempting to stop existing Qdrant containers first..."
fi

# Stop any existing Qdrant containers (both possible names)
echo "🛑 Stopping any existing Qdrant containers..."
docker stop qdrant-server scanoss-qdrant 2>/dev/null || true
docker rm qdrant-server scanoss-qdrant 2>/dev/null || true

# Wait a moment for ports to be released
sleep 2

# Recheck ports after stopping containers
if netstat -tlnp 2>/dev/null | grep -q ":6333 \|:6334 "; then
    echo "❌ Ports 6333 or 6334 are still in use after stopping containers"
    echo "📋 Current port usage:"
    netstat -tlnp 2>/dev/null | grep ":6333 \|:6334 "
    echo ""
    echo "💡 Please manually stop the processes using these ports and try again"
    exit 1
fi

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

# Recreate clean data directory with proper permissions
mkdir -p "$QDRANT_PATH/data"
chmod 755 "$QDRANT_PATH/data"

# Set proper ownership for the data directory
if [ "$RUNTIME_USER" != "root" ]; then
    chown -R $RUNTIME_USER:$RUNTIME_USER "$QDRANT_PATH/data"
fi

# Copy docker-compose.yml from package directory
echo "📝 Using Docker configuration from package..."
if [ -f "$ORIGINAL_DIR/docker-compose.yml" ]; then
    if ! cp "$ORIGINAL_DIR/docker-compose.yml" "$QDRANT_PATH/"; then
        echo "Failed to copy docker-compose.yml"
        exit 1
    fi
    echo "✅ Docker Compose configuration copied"
else
    echo "❌ docker-compose.yml not found in package"
    exit 1
fi

# Start clean Qdrant container
echo "🚀 Starting clean Qdrant container..."
cd "$QDRANT_PATH"
docker-compose up -d

# Give the container a moment to initialize
sleep 5

# Check if container is actually running
if ! docker ps --filter name=qdrant-server --filter status=running | grep -q qdrant-server; then
    echo "❌ Container failed to start. Checking logs..."
    docker logs qdrant-server 2>/dev/null || echo "Could not retrieve logs"
    echo "🔍 Container status:"
    docker ps -a --filter name=qdrant-server
    exit 1
fi

# Wait for Qdrant to be ready with improved error handling
echo "⏳ Waiting for Qdrant to be ready..."
timeout=120 # 2 minutes should be enough for clean startup
counter=0
container_failed=false

while [ $counter -lt $timeout ]; do
    # Check if container is still running
    if ! docker ps --filter name=qdrant-server --filter status=running | grep -q qdrant-server; then
        echo "❌ Container stopped running during startup"
        container_failed=true
        break
    fi
    
    # Check if Qdrant API is responding
    if curl -f http://localhost:6333 >/dev/null 2>&1; then
        echo "✅ Qdrant is ready!"
        break
    fi
    
    # Progress reporting
    if [ $((counter % 15)) -eq 0 ]; then
        echo "Still waiting for Qdrant... ($counter/$timeout seconds)"
        # Show container health status
        HEALTH_STATUS=$(docker inspect qdrant-server --format='{{.State.Health.Status}}' 2>/dev/null || echo "unknown")
        if [ "$HEALTH_STATUS" != "unknown" ]; then
            echo "  Container health: $HEALTH_STATUS"
        fi
    fi
    
    sleep 3
    counter=$((counter + 3))
done

if [ "$container_failed" = true ] || [ $counter -ge $timeout ]; then
    echo "❌ Qdrant failed to start properly"
    echo ""
    echo "📋 Container logs (last 50 lines):"
    docker logs --tail 50 qdrant-server 2>/dev/null || echo "Could not retrieve logs"
    echo ""
    echo "🔍 Container status:"
    docker ps -a --filter name=qdrant-server
    echo ""
    echo "🔍 Container inspect (State section):"
    docker inspect qdrant-server --format='{{json .State}}' 2>/dev/null | python3 -m json.tool 2>/dev/null || echo "Could not inspect container"
    echo ""
    echo "💡 Troubleshooting tips:"
    echo "  - Check if port 6333 is already in use: netstat -tlnp | grep 6333"
    echo "  - Check Docker daemon status: systemctl status docker"
    echo "  - Try manual start: cd $QDRANT_PATH && docker-compose up"
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
docker ps --filter name=qdrant-server --format "table {{.Names}}\t{{.Status}}\t{{.Ports}}"
echo ""
echo "📋 Next Steps:"
echo ""
echo "1. 📝 Configure the service:"
echo "   sudo cp $C_PATH/config.example.json $C_PATH/$CONF"
echo "   sudo nano $C_PATH/$CONF"
echo ""
echo "2. 📥 Import your knowledge base data:"
echo "   sudo ./scripts/import-collections.sh /path/to/collection-snapshots/"
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
echo "  - View Qdrant logs: docker logs qdrant-server"
echo ""
