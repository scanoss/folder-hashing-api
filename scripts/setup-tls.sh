#!/bin/bash

##########################################
#
# SCANOSS HFH API - TLS Setup Script
#
# This script helps set up TLS certificates for Docker deployment
# Ensures proper permissions for the scanoss user
#
################################################################

set -e

if [ "$1" = "-h" ] || [ "$1" = "-help" ]; then
  echo "$0 [-help] <cert-file> <key-file>"
  echo "   Set up TLS certificates for SCANOSS HFH API"
  echo ""
  echo "Arguments:"
  echo "   <cert-file>   Path to TLS certificate file (required)"
  echo "   <key-file>    Path to TLS private key file (required)"
  echo ""
  echo "When to use sudo:"
  echo "   - Use sudo if source certificates are only readable by root"
  echo "   - Use sudo if you want files owned by UID 1000 (container user)"
  echo "   - Without sudo: files will be world-readable (less secure)"
  echo ""
  echo "Examples:"
  echo "   $0 /path/to/cert.pem /path/to/key.pem"
  echo "   sudo $0 /opt/scanoss/ssl/cert.pem /opt/scanoss/ssl/key.pem"
  echo ""
  exit 1
fi

if [ -z "$1" ] || [ -z "$2" ]; then
  echo "❌ Error: Certificate and key files are required"
  echo "Usage: $0 <cert-file> <key-file>"
  exit 1
fi

# Certificate directory (mounted into Docker container)
CERT_DIR="./config/certs"
CERT_FILE="$CERT_DIR/cert.pem"
KEY_FILE="$CERT_DIR/key.pem"

echo "🔐 SCANOSS HFH API - TLS Certificate Setup"
echo "=========================================="

# Check if we might need sudo
NEED_SUDO=""
if [ ! -r "$1" ] || [ ! -r "$2" ]; then
  echo "⚠️  Warning: Cannot read one or both certificate files"
  echo "   Certificate: $1 (readable: $([ -r "$1" ] && echo "yes" || echo "no"))"
  echo "   Key: $2 (readable: $([ -r "$2" ] && echo "yes" || echo "no"))"
  if [ "$EUID" -ne 0 ]; then
    echo ""
    echo "   Try running with sudo: sudo $0 $@"
    exit 1
  fi
fi

# Create certificate directory
echo "📁 Creating certificate directory..."
mkdir -p "$CERT_DIR"

# Copy provided certificate files
if [ ! -f "$1" ]; then
  echo "❌ Certificate file not found: $1"
  exit 1
fi

if [ ! -f "$2" ]; then
  echo "❌ Key file not found: $2"
  exit 1
fi

echo "📋 Copying provided certificate files..."
cp "$1" "$CERT_FILE"
cp "$2" "$KEY_FILE"
echo "✅ Certificate files copied"

# Set appropriate permissions
echo "🔒 Setting secure permissions..."

# The container scanoss user has UID/GID 1000 (defined in Dockerfile)
CONTAINER_UID=1000
CONTAINER_GID=1000

# Make certificate readable by all (644), key readable by owner+group (640)
chmod 644 "$CERT_FILE"
chmod 640 "$KEY_FILE"

# Try to set ownership to match container user
echo "👤 Setting ownership for container user (UID/GID $CONTAINER_UID)..."

# Check if we're running as root or can set ownership
if [ "$EUID" -eq 0 ]; then
  # Running as root, can set any ownership
  chown "$CONTAINER_UID:$CONTAINER_GID" "$CERT_FILE" "$KEY_FILE"
  echo "✅ Ownership set to $CONTAINER_UID:$CONTAINER_GID (running as root)"
elif [ "$(id -u)" -eq "$CONTAINER_UID" ]; then
  # Current user matches container UID, ownership is already correct
  echo "✅ Current user UID matches container UID ($CONTAINER_UID)"
elif chown "$CONTAINER_UID:$CONTAINER_GID" "$CERT_FILE" "$KEY_FILE" 2>/dev/null; then
  # Non-root but chown succeeded (unlikely)
  echo "✅ Ownership set to $CONTAINER_UID:$CONTAINER_GID"
else
  # Cannot set ownership, use permission fallback
  echo "⚠️  Cannot set ownership to $CONTAINER_UID:$CONTAINER_GID"
  echo "   Using permission-based solution instead..."
  
  # Make files world-readable as fallback
  chmod 644 "$CERT_FILE"
  chmod 644 "$KEY_FILE"
  
  # Check if user is in docker group
  if groups | grep -q docker; then
    echo "✅ You're in the docker group - files should be accessible to container"
  else
    echo "⚠️  You're not in the docker group - container might have issues"
    echo "   Run: sudo usermod -aG docker $USER && newgrp docker"
  fi
fi

# Verify files are readable
if [ -r "$CERT_FILE" ] && [ -r "$KEY_FILE" ]; then
  echo "✅ Certificate files are readable"
else
  echo "❌ Error: Certificate files may not be readable in container"
  echo "   Try running with sudo: sudo $0 $@"
  exit 1
fi

# Create TLS-enabled config if it doesn't exist
TLS_CONFIG="./config/app-config-tls.json"
if [ ! -f "$TLS_CONFIG" ]; then
  echo "📝 Creating TLS-enabled configuration template..."
  cat >"$TLS_CONFIG" <<EOF
{
  "App": {
    "Name": "SCANOSS HFH Server",
    "GRPCPort": "50061",
    "RESTPort": "40061",
    "Debug": false,
    "Trace": false,
    "Mode": "production"
  },
  "Logging": {
    "DynamicLogging": true,
    "DynamicPort": "localhost:60061"
  },
  "Telemetry": {
    "Enabled": false,
    "OltpExporter": "0.0.0.0:4317"
  },
  "TLS": {
    "CertFile": "/app/certs/cert.pem",
    "KeyFile": "/app/certs/key.pem",
    "CN": "localhost"
  },
  "Hfh": {
    "QdrantHost": "qdrant",
    "QdrantPort": 6334
  }
}
EOF
  echo "✅ TLS configuration template created: $TLS_CONFIG"
fi

echo ""
echo "🎉 TLS setup complete!"
echo ""
echo "📋 Next steps:"
echo "  1. Review/edit the TLS config: $TLS_CONFIG"
echo "  2. Use it for deployment: cp $TLS_CONFIG ./config/app-config.json"
echo "  3. Deploy with TLS: ./scripts/docker-deploy.sh prod"
echo ""
echo "🌐 Your API will be available at:"
echo "  - HTTPS REST API: https://localhost:40061"
echo "  - TLS gRPC API: localhost:50061"
echo ""
echo "⚠️  Security Notes:"
echo "  - Health checks will automatically use HTTPS when TLS is configured"
echo "  - Ensure firewall allows HTTPS traffic on port 40061"
echo "  - The certificate CN field should match your server hostname"
echo ""
echo "📋 Permission Summary:"
echo "  - Container runs as UID/GID 1000 (scanoss user)"
echo "  - Certificate files in ./config/certs/ are:"
ls -la "$CERT_FILE" "$KEY_FILE" | sed 's/^/    /'
echo ""
echo "  - If container fails to read certificates:"
echo "    1. Check docker logs: docker logs <container-name>"
echo "    2. Re-run with sudo: sudo $0 <cert> <key>"
echo ""
