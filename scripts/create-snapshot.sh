#!/bin/bash

##########################################
#
# This script creates a new Qdrant snapshot from the current knowledge base
# Used for creating monthly distribution snapshots
#
################################################################

set -e

if [ "$1" = "-h" ] || [ "$1" = "-help" ]; then
    echo "$0 [-help] [output-dir]"
    echo "   Create a new Qdrant snapshot for distribution"
    echo "   [output-dir] directory to save snapshot (default: snapshots/)"
    exit 1
fi

OUTPUT_DIR="${1:-snapshots}"
DATE=$(date +%Y-%m-%d)
SNAPSHOT_NAME="scanoss-kb-${DATE}"

echo "📸 Creating SCANOSS Knowledge Base Snapshot"
echo "==========================================="

# Check if Qdrant is running
if ! curl -f http://localhost:6333 >/dev/null 2>&1; then
    echo "❌ Qdrant is not running or not accessible"
    echo "Please ensure Qdrant is running on localhost:6333"
    exit 1
fi

# Create output directory
mkdir -p "$OUTPUT_DIR"

echo "📊 Checking current knowledge base status..."

# Get collection information
COLLECTIONS_RESPONSE=$(curl -s http://localhost:6333/collections 2>/dev/null)
if ! echo "$COLLECTIONS_RESPONSE" | grep -q '"status":"ok"'; then
    echo "❌ Failed to get collection information"
    echo "Response: $COLLECTIONS_RESPONSE"
    exit 1
fi

COLLECTION_NAMES=$(echo "$COLLECTIONS_RESPONSE" | grep -o '"name":"[^"]*"' | cut -d'"' -f4)
COLLECTION_COUNT=$(echo "$COLLECTION_NAMES" | wc -l)

echo "✅ Found $COLLECTION_COUNT collections:"
echo "$COLLECTION_NAMES" | while read -r collection; do
    if [ -n "$collection" ]; then
        echo "  - $collection"
    fi
done

# Create full storage snapshot
echo ""
echo "📸 Creating full storage snapshot..."
echo "This may take several minutes for large datasets..."

CREATE_RESPONSE=$(curl -s -X POST "http://localhost:6333/snapshots" 2>/dev/null)

if echo "$CREATE_RESPONSE" | grep -q '"status":"ok"'; then
    echo "✅ Snapshot creation initiated successfully!"

    # Extract snapshot name from response
    CREATED_SNAPSHOT=$(echo "$CREATE_RESPONSE" | grep -o '"name":"[^"]*"' | cut -d'"' -f4)
    if [ -n "$CREATED_SNAPSHOT" ]; then
        echo "📦 Snapshot name: $CREATED_SNAPSHOT"
    else
        echo "⚠️  Could not extract snapshot name from response"
        CREATED_SNAPSHOT="$SNAPSHOT_NAME"
    fi
else
    echo "❌ Snapshot creation failed!"
    echo "Response: $CREATE_RESPONSE"
    exit 1
fi

# Wait for snapshot creation to complete
echo "⏳ Waiting for snapshot creation to complete..."
timeout=1800 # 30 minutes timeout
counter=0
while [ $counter -lt $timeout ]; do
    # List snapshots to check if creation is complete
    LIST_RESPONSE=$(curl -s http://localhost:6333/snapshots 2>/dev/null)
    if echo "$LIST_RESPONSE" | grep -q "\"$CREATED_SNAPSHOT\""; then
        echo "✅ Snapshot creation complete!"
        break
    fi

    if [ $((counter % 60)) -eq 0 ]; then
        echo "Still creating snapshot... ($counter/$timeout seconds)"
    fi
    sleep 10
    counter=$((counter + 10))
done

if [ $counter -ge $timeout ]; then
    echo "❌ Timeout waiting for snapshot creation"
    exit 1
fi

# Download the snapshot
echo "📥 Downloading snapshot..."
OUTPUT_FILE="$OUTPUT_DIR/${SNAPSHOT_NAME}.snapshot"

if curl -f "http://localhost:6333/snapshots/${CREATED_SNAPSHOT}" --output "$OUTPUT_FILE" 2>/dev/null; then
    echo "✅ Snapshot downloaded successfully!"
else
    echo "❌ Failed to download snapshot"
    exit 1
fi

# Verify the downloaded file
if [ -f "$OUTPUT_FILE" ]; then
    SNAPSHOT_SIZE=$(du -h "$OUTPUT_FILE" | cut -f1)
    echo "📁 Snapshot file: $OUTPUT_FILE"
    echo "📏 File size: $SNAPSHOT_SIZE"
else
    echo "❌ Snapshot file not found after download"
    exit 1
fi

# Clean up the snapshot from Qdrant (optional)
read -p "Do you want to delete the snapshot from Qdrant server? (y/n) [y]? " -n 1 -r
echo
if [[ $REPLY =~ ^[Yy]$|^$ ]]; then
    echo "🗑️  Cleaning up server snapshot..."
    DELETE_RESPONSE=$(curl -s -X DELETE "http://localhost:6333/snapshots/${CREATED_SNAPSHOT}" 2>/dev/null)
    if echo "$DELETE_RESPONSE" | grep -q '"status":"ok"'; then
        echo "✅ Server snapshot cleaned up"
    else
        echo "⚠️  Failed to clean up server snapshot (this is not critical)"
    fi
else
    echo "ℹ️  Server snapshot retained"
fi

echo ""
echo "🎉 Snapshot creation complete!"
echo ""
echo "📦 Snapshot file: $OUTPUT_FILE"
echo "📏 Size: $SNAPSHOT_SIZE"
echo "📅 Date: $DATE"
echo ""
echo "This snapshot can now be used for offline distribution packages."
echo "Use with: ./create-package.sh linux_amd64 1.0.0 $DATE"
echo ""
