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
  echo "Examples:"
  echo "   $0 /path/to/cert.pem /path/to/key.pem"
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

# Make certificate readable by all, key readable only by owner
# The container runs as UID 1000 (scanoss user)
chmod 644 "$CERT_FILE"
chmod 600 "$KEY_FILE"

echo "✅ Permissions set successfully"

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
echo "⚠️  Notes:"
echo "  - Health checks will automatically use HTTPS when TLS is configured"
echo "  - Ensure firewall allows HTTPS traffic on port 40061"
echo "  - The certificate CN field should match your server hostname"
echo ""
