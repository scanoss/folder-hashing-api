#!/bin/bash

##########################################
#
# This script handles monthly knowledge base updates by replacing
# the current Qdrant snapshot with a new one
#
################################################################

set -e

if [ "$1" = "-h" ] || [ "$1" = "-help" ]; then
    echo "$0 [-help] [snapshot-file]"
    echo "   Update SCANOSS knowledge base with new snapshot"
    echo "   [snapshot-file] path to new snapshot file (optional - will look in snapshots/ if not provided)"
    exit 1
fi

SNAPSHOT_FILE="$1"
QDRANT_PATH="/usr/local/etc/scanoss/qdrant"
BACKUP_DIR="/usr/local/etc/scanoss/qdrant/backups"

echo "🔄 SCANOSS Knowledge Base Update Process"
echo "======================================="

# If no snapshot file provided, look in snapshots directory
if [ -z "$SNAPSHOT_FILE" ]; then
    SNAPSHOT_FILE=$(find snapshots/ -name "*.snapshot" 2>/dev/null | head -1)
    if [ -z "$SNAPSHOT_FILE" ]; then
        echo "❌ No snapshot file found!"
        echo "Please provide a snapshot file or place one in the snapshots/ directory"
        exit 1
    fi
fi

# Verify snapshot file exists
if [ ! -f "$SNAPSHOT_FILE" ]; then
    echo "❌ Snapshot file not found: $SNAPSHOT_FILE"
    exit 1
fi

echo "📦 Using new snapshot: $SNAPSHOT_FILE"

# Create backup directory
mkdir -p "$BACKUP_DIR"

# Check if we're running as root
if [ "$EUID" -ne 0 ]; then
    echo "Please run as root for system service management"
    exit 1
fi

# Stop the HFH API service
echo "🛑 Stopping SCANOSS Folder Hashing API service..."
if systemctl is-active --quiet scanoss-hfh-api; then
    systemctl stop scanoss-hfh-api
    echo "✅ Service stopped"
else
    echo "ℹ️  Service was not running"
fi

# Create backup of current Qdrant data
echo "💾 Creating backup of current knowledge base..."
BACKUP_NAME="qdrant-backup-$(date +%Y%m%d-%H%M%S)"

if [ -d "$QDRANT_PATH/data" ] && [ "$(ls -A $QDRANT_PATH/data 2>/dev/null)" ]; then
    cd "$QDRANT_PATH"
    tar -czf "$BACKUP_DIR/$BACKUP_NAME.tar.gz" data/ 2>/dev/null || true
    echo "✅ Backup created: $BACKUP_DIR/$BACKUP_NAME.tar.gz"
else
    echo "ℹ️  No existing data to backup"
fi

# Stop Qdrant container
echo "🛑 Stopping Qdrant container..."
cd "$QDRANT_PATH"
docker-compose down
echo "✅ Qdrant stopped"

# Clear old data
echo "🗑️  Clearing old knowledge base data..."
rm -rf "$QDRANT_PATH/data"/*
echo "✅ Old data cleared"

# Copy new snapshot
echo "📥 Installing new snapshot..."
NEW_SNAPSHOT_NAME=$(basename "$SNAPSHOT_FILE")
cp "$SNAPSHOT_FILE" "$QDRANT_PATH/snapshots/$NEW_SNAPSHOT_NAME"
echo "✅ New snapshot installed"

# Start Qdrant
echo "🚀 Starting Qdrant with new snapshot..."
docker-compose up -d

# Wait for Qdrant to be ready
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
    echo "❌ Update failed - restoring from backup..."

    # Restore backup
    docker-compose down
    rm -rf "$QDRANT_PATH/data"/*
    if [ -f "$BACKUP_DIR/$BACKUP_NAME.tar.gz" ]; then
        cd "$QDRANT_PATH"
        tar -xzf "$BACKUP_DIR/$BACKUP_NAME.tar.gz"
        docker-compose up -d
        echo "🔄 Backup restored"
    fi
    exit 1
fi

# Restore new snapshot
echo "📥 Restoring new knowledge base..."
RESTORE_RESPONSE=$(curl -s -X PUT "http://localhost:6333/snapshots/recover" \
    -H "Content-Type: application/json" \
    -d "{\"location\": \"file:///snapshots/$NEW_SNAPSHOT_NAME\", \"priority\": \"snapshot\"}" \
    2>/dev/null)

if echo "$RESTORE_RESPONSE" | grep -q '"status":"ok"'; then
    echo "✅ Snapshot restoration initiated successfully!"
else
    echo "❌ Snapshot restoration failed!"
    echo "Response: $RESTORE_RESPONSE"
    exit 1
fi

# Wait for restoration to complete
echo "⏳ Waiting for knowledge base restoration..."
timeout=1800 # 30 minutes timeout for large datasets
counter=0
while [ $counter -lt $timeout ]; do
    COLLECTIONS_RESPONSE=$(curl -s http://localhost:6333/collections 2>/dev/null || echo '{"result":{"collections":[]}}')
    CURRENT_COLLECTIONS=$(echo "$COLLECTIONS_RESPONSE" | grep -o '"name":"[^"]*"' | wc -l)

    if [ "$CURRENT_COLLECTIONS" -gt 0 ]; then
        echo "✅ Knowledge base restoration complete! Found $CURRENT_COLLECTIONS collections."
        break
    fi

    if [ $((counter % 60)) -eq 0 ]; then
        echo "Still restoring... ($counter/$timeout seconds)"
    fi
    sleep 10
    counter=$((counter + 10))
done

if [ $counter -ge $timeout ]; then
    echo "❌ Timeout waiting for restoration to complete"
    exit 1
fi

# Start the HFH API service
echo "🚀 Starting SCANOSS Folder Hashing API service..."
systemctl start scanoss-hfh-api

# Wait a moment and check service status
sleep 5
if systemctl is-active --quiet scanoss-hfh-api; then
    echo "✅ Service started successfully"
else
    echo "⚠️  Service may have issues - check status with: systemctl status scanoss-hfh-api"
fi

# Verify the update
echo "🔍 Verifying knowledge base update..."
COLLECTIONS_RESPONSE=$(curl -s http://localhost:6333/collections 2>/dev/null)
if echo "$COLLECTIONS_RESPONSE" | grep -q '"status":"ok"'; then
    COLLECTION_NAMES=$(echo "$COLLECTIONS_RESPONSE" | grep -o '"name":"[^"]*"' | cut -d'"' -f4)
    echo "✅ Knowledge base update successful!"
    echo "Available collections:"
    echo "$COLLECTION_NAMES" | while read -r collection; do
        if [ -n "$collection" ]; then
            echo "  - $collection"
        fi
    done
else
    echo "⚠️  Could not verify collections, but services are running"
fi

# Clean up old backups (keep last 5)
echo "🧹 Cleaning up old backups..."
cd "$BACKUP_DIR"
ls -t qdrant-backup-*.tar.gz 2>/dev/null | tail -n +6 | xargs rm -f 2>/dev/null || true

echo
echo "🎉 Knowledge base update complete!"
echo "🌐 API endpoint: http://localhost:40061"
echo "🔍 Qdrant dashboard: http://localhost:6333/dashboard"
echo "💚 Health check: curl http://localhost:40061/health"
echo "📝 View logs: journalctl -u scanoss-hfh-api -f"
echo
