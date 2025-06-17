#!/bin/bash

##########################################
#
# This script will copy all the required files into the correct locations on the server
# and set up Qdrant with the knowledge base snapshot
# Config goes into: /usr/local/etc/scanoss/hfh
# Logs go into: /var/log/scanoss/hfh
# Service definition goes into: /etc/systemd/system
# Binary & startup go into: /usr/local/bin
# Qdrant setup in: /usr/local/etc/scanoss/qdrant
#
################################################################

if [ "$1" = "-h" ] || [ "$1" = "-help" ]; then
    echo "$0 [-help] [environment] [snapshot_path]"
    echo "   Setup and copy the relevant files into place on a server to run the SCANOSS Folder Hashing API"
    echo ""
    echo "Arguments:"
    echo "   [environment]   optional environment suffix (default: prod)"
    echo "   [snapshot_path] path to SCANOSS knowledge base snapshot file (required)"
    echo ""
    echo "Examples:"
    echo "   $0 prod /path/to/scanoss-kb-2025-01-15.snapshot"
    echo "   $0 dev /home/user/snapshots/latest.snapshot"
    exit 1
fi

DEFAULT_ENV="prod"
ENVIRONMENT="${1:-$DEFAULT_ENV}"
SNAPSHOT_PATH="$2"

# Validate snapshot path
if [ -z "$SNAPSHOT_PATH" ]; then
    echo "ERROR: Snapshot path is required"
    echo "Usage: $0 [environment] [snapshot_path]"
    echo "Example: $0 prod /path/to/scanoss-kb-2025-01-15.snapshot"
    exit 1
fi

if [ ! -f "$SNAPSHOT_PATH" ]; then
    echo "ERROR: Snapshot file not found: $SNAPSHOT_PATH"
    echo "Please ensure the snapshot file exists and is accessible"
    exit 1
fi

echo "Using snapshot: $SNAPSHOT_PATH"

export C_PATH=/usr/local/etc/scanoss/hfh
export LOG_DIR=/var/log/scanoss
export L_PATH="${LOG_DIR}/hfh"
export QDRANT_PATH=/usr/local/etc/scanoss/qdrant
export RUNTIME_USER=scanoss

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

read -p "Install SCANOSS Folder Hashing API $ENVIRONMENT (y/n) [n]? " -n 1 -r
echo
if [[ $REPLY =~ ^[Yy]$ ]]; then
    echo "Starting installation..."
else
    echo "Stopping."
    exit 1
fi

# Setup all the required folders and ownership
echo "Setting up Folder Hashing API system folders..."
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

if [ "$RUNTIME_USER" != "root" ]; then
    echo "Changing ownership of $LOG_DIR to $RUNTIME_USER ..."
    if ! chown -R $RUNTIME_USER $LOG_DIR; then
        echo "chown of $LOG_DIR to $RUNTIME_USER failed"
        exit 1
    fi
fi

# Setup Qdrant with snapshot first
echo "Setting up Qdrant with knowledge base..."
if ! ./scripts/setup-qdrant.sh "$ENVIRONMENT" "$SNAPSHOT_PATH"; then
    echo "❌ Qdrant setup failed"
    exit 1
fi

# Setup the service on the system (defaulting to service name without environment)
SC_SERVICE_FILE="scanoss-hfh-api.service"
SC_SERVICE_NAME="scanoss-hfh-api"
if [ -n "$ENVIRONMENT" ]; then
    SC_SERVICE_FILE="scanoss-hfh-api-${ENVIRONMENT}.service"
    SC_SERVICE_NAME="scanoss-hfh-api-${ENVIRONMENT}"
fi

export service_stopped=""
if [ -f "/etc/systemd/system/$SC_SERVICE_FILE" ]; then
    echo "Stopping $SC_SERVICE_NAME service first..."
    if ! systemctl stop "$SC_SERVICE_NAME"; then
        echo "service stop failed"
        exit 1
    fi
    export service_stopped="true"
fi

echo "Copying service startup config..."
if [ -f "scripts/$SC_SERVICE_FILE" ]; then
    if ! cp "scripts/$SC_SERVICE_FILE" /etc/systemd/system; then
        echo "service copy failed"
        exit 1
    fi
else
    echo "Service file scripts/$SC_SERVICE_FILE not found"
    exit 1
fi

if ! cp scripts/scanoss-hfh-api.sh /usr/local/bin; then
    echo "hfh api startup script copy failed"
    exit 1
fi
chmod +x /usr/local/bin/scanoss-hfh-api.sh

# Copy configuration template for customer to customize
CONF=app-config-prod.json
if [ -n "$ENVIRONMENT" ] && [ "$ENVIRONMENT" != "prod" ]; then
    CONF="app-config-${ENVIRONMENT}.json"
fi

echo "Copying configuration template to $C_PATH ..."
if [ -f "config.example.json" ]; then
    if ! cp "config.example.json" "$C_PATH/"; then
        echo "copy config.example.json failed"
        exit 1
    fi
    echo "✅ Configuration template copied to $C_PATH/config.example.json"
    echo "📝 Please customize and rename to $CONF before starting the service"
else
    echo "⚠️  config.example.json not found in package"
    echo "📝 Please create your config file at: $C_PATH/$CONF"
fi

# Copy the binary if requested
BINARY=scanoss-hfh-api
if [ -f "dist/$BINARY" ]; then
    echo "Copying app binary to /usr/local/bin ..."
    if ! cp "dist/$BINARY" /usr/local/bin; then
        echo "copy dist/$BINARY failed"
        echo "Please make sure the service is stopped: systemctl stop $SC_SERVICE_NAME"
        exit 1
    fi
    chmod +x /usr/local/bin/$BINARY
else
    echo "Please copy the API binary file into: /usr/local/bin/$BINARY"
fi

# Reload systemd and enable service
systemctl daemon-reload
systemctl enable "$SC_SERVICE_NAME"

echo "Installation complete."

# Start the service if it was previously stopped
if [ "$service_stopped" == "true" ]; then
    echo "Restarting service after install..."
    if ! systemctl start "$SC_SERVICE_NAME"; then
        echo "failed to restart service"
        exit 1
    fi
    systemctl status "$SC_SERVICE_NAME"
else
    echo "Starting service for the first time..."
    if ! systemctl start "$SC_SERVICE_NAME"; then
        echo "failed to start service - check configuration"
        echo "View logs with: journalctl -u $SC_SERVICE_NAME -f"
        exit 1
    fi
    systemctl status "$SC_SERVICE_NAME"
fi

echo
echo "🎉 SCANOSS Folder Hashing API installation complete!"
echo
echo "📁 Review service config in: $C_PATH/$CONF"
echo "📝 Review service logs in: $L_PATH"
echo "🔧 Start the service using: systemctl start $SC_SERVICE_NAME"
echo "🛑 Stop the service using: systemctl stop $SC_SERVICE_NAME"
echo "📊 Get service status using: systemctl status $SC_SERVICE_NAME"
echo "🌐 REST API endpoint: http://localhost:40061"
echo "🌐 gRPC endpoint: http://localhost:50061"
echo "🔍 Qdrant dashboard: http://localhost:6333/dashboard"
echo
