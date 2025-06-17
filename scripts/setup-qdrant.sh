#!/bin/bash

##########################################
#
# This script sets up Qdrant with the SCANOSS knowledge base snapshot
# It handles Docker container setup and snapshot restoration
#
################################################################

set -e

if [ "$1" = "-h" ] || [ "$1" = "-help" ]; then
    echo "$0 [-help] [environment] [snapshot_path]"
    echo "   Set up Qdrant with SCANOSS knowledge base snapshot"
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

ENVIRONMENT="${1:-prod}"
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
QDRANT_PATH="/usr/local/etc/scanoss/qdrant"
SNAPSHOT_DIR="$QDRANT_PATH/snapshots"
QDRANT_DATA="$QDRANT_PATH/data"

echo "🚀 Setting up Qdrant with SCANOSS knowledge base..."

# Create directories
echo "📁 Creating Qdrant directories..."
mkdir -p "$SNAPSHOT_DIR"
mkdir -p "$QDRANT_DATA"

# Copy provided snapshot to Qdrant directory
echo "📦 Copying snapshot to Qdrant directory..."
SNAPSHOT_NAME=$(basename "$SNAPSHOT_PATH")
cp "$SNAPSHOT_PATH" "$SNAPSHOT_DIR/"

if [ ! -f "$SNAPSHOT_DIR/$SNAPSHOT_NAME" ]; then
    echo "❌ Failed to copy snapshot to $SNAPSHOT_DIR/"
    exit 1
fi

echo "✅ Snapshot copied: $SNAPSHOT_NAME"

# Create Qdrant docker-compose configuration
echo "📝 Creating Qdrant Docker configuration..."
cat >"$QDRANT_PATH/docker-compose.yml" <<'EOF'
version: '3.8'
services:
  qdrant:
    image: qdrant/qdrant:v1.11.0
    container_name: scanoss-qdrant
    ports:
      - "6333:6333"
      - "6334:6334"
    volumes:
      - ./data:/qdrant/storage
      - ./snapshots:/snapshots:ro
    restart: unless-stopped
    environment:
      - QDRANT__SERVICE__HTTP_PORT=6333
      - QDRANT__SERVICE__GRPC_PORT=6334
    healthcheck:
      test: ["CMD", "curl", "-f", "http://localhost:6333/health"]
      interval: 30s
      timeout: 10s
      retries: 5
      start_period: 30s
    command: ["./qdrant"]
EOF

# Stop any existing Qdrant container
echo "🛑 Stopping any existing Qdrant containers..."
docker stop scanoss-qdrant 2>/dev/null || true
docker rm scanoss-qdrant 2>/dev/null || true

# Start Qdrant
echo "🚀 Starting Qdrant container..."
cd "$QDRANT_PATH"
docker-compose up -d

# Wait for Qdrant to be ready with better error handling
echo "⏳ Waiting for Qdrant to be ready..."
timeout=300 # 5 minutes timeout
counter=0
while [ $counter -lt $timeout ]; do
    if curl -f http://localhost:6333/health >/dev/null 2>&1; then
        echo "✅ Qdrant is ready!"
        break
    fi
    if [ $((counter % 30)) -eq 0 ]; then
        echo "Still waiting for Qdrant... ($counter/$timeout seconds)"
    fi
    sleep 2
    counter=$((counter + 2))
done

if [ $counter -ge $timeout ]; then
    echo "❌ Timeout waiting for Qdrant to be ready"
    echo "Check Qdrant logs: docker logs scanoss-qdrant"
    exit 1
fi

# Check if collections already exist (for re-installations)
echo "🔍 Checking existing collections..."
COLLECTIONS_RESPONSE=$(curl -s http://localhost:6333/collections 2>/dev/null || echo '{"result":{"collections":[]}}')
COLLECTIONS_COUNT=$(echo "$COLLECTIONS_RESPONSE" | grep -o '"collections":\[[^]]*\]' | grep -o '\[.*\]' | grep -o ',' | wc -l)
COLLECTIONS_COUNT=$((COLLECTIONS_COUNT + 1))

# If collections exist, ask user what to do
if [ "$COLLECTIONS_COUNT" -gt 1 ]; then
    echo "⚠️  Found $COLLECTIONS_COUNT existing collections in Qdrant"
    read -p "Do you want to restore snapshot anyway? This will overwrite existing data (y/n) [n]? " -n 1 -r
    echo
    if [[ ! $REPLY =~ ^[Yy]$ ]]; then
        echo "Skipping snapshot restoration. Using existing data."
        echo "✅ Qdrant setup complete with existing data!"
        return 0
    fi
fi

# Restore snapshot
echo "📥 Restoring knowledge base snapshot..."
echo "This may take several minutes for large datasets..."

RESTORE_RESPONSE=$(curl -s -X PUT "http://localhost:6333/snapshots/recover" \
    -H "Content-Type: application/json" \
    -d "{\"location\": \"file:///snapshots/$SNAPSHOT_NAME\", \"priority\": \"snapshot\"}" \
    2>/dev/null)

# Check if restore was successful
if echo "$RESTORE_RESPONSE" | grep -q '"status":"ok"'; then
    echo "✅ Snapshot restoration initiated successfully!"
else
    echo "❌ Snapshot restoration failed!"
    echo "Response: $RESTORE_RESPONSE"
    echo "Check Qdrant logs: docker logs scanoss-qdrant"
    exit 1
fi

# Wait for restoration to complete
echo "⏳ Waiting for snapshot restoration to complete..."
timeout=1800 # 30 minutes timeout for large datasets
counter=0
while [ $counter -lt $timeout ]; do
    # Check collections count to see if restoration is complete
    COLLECTIONS_RESPONSE=$(curl -s http://localhost:6333/collections 2>/dev/null || echo '{"result":{"collections":[]}}')
    CURRENT_COLLECTIONS=$(echo "$COLLECTIONS_RESPONSE" | grep -o '"name":"[^"]*"' | wc -l)

    if [ "$CURRENT_COLLECTIONS" -gt 0 ]; then
        echo "✅ Snapshot restoration complete! Found $CURRENT_COLLECTIONS collections."
        break
    fi

    if [ $((counter % 60)) -eq 0 ]; then
        echo "Still restoring snapshot... ($counter/$timeout seconds)"
    fi
    sleep 10
    counter=$((counter + 10))
done

if [ $counter -ge $timeout ]; then
    echo "❌ Timeout waiting for snapshot restoration"
    echo "Check Qdrant logs: docker logs scanoss-qdrant"
    exit 1
fi

# Verify the setup by checking collection stats
echo "📊 Verifying knowledge base setup..."
COLLECTIONS_RESPONSE=$(curl -s http://localhost:6333/collections 2>/dev/null)
if echo "$COLLECTIONS_RESPONSE" | grep -q '"status":"ok"'; then
    COLLECTION_NAMES=$(echo "$COLLECTIONS_RESPONSE" | grep -o '"name":"[^"]*"' | cut -d'"' -f4)
    echo "✅ Knowledge base loaded successfully!"
    echo "Available collections:"
    echo "$COLLECTION_NAMES" | while read -r collection; do
        if [ -n "$collection" ]; then
            echo "  - $collection"
        fi
    done
else
    echo "⚠️  Could not verify collections, but Qdrant is running"
fi

echo
echo "🎉 Qdrant setup complete!"
echo "🌐 Qdrant dashboard: http://localhost:6333/dashboard"
echo "🔧 Qdrant API: http://localhost:6333"
echo "📊 gRPC endpoint: localhost:6334"
echo
