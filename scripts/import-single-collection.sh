#!/bin/bash

set -e

if [ "$1" = "-h" ] || [ "$1" = "-help" ] || [ -z "$1" ]; then
    echo "Usage: $0 <snapshot-file>"
    echo "   Import a single collection snapshot to Qdrant"
    echo ""
    echo "Example:"
    echo "   $0 javascript_collection-2025-07-01.snapshot"
    exit 1
fi

SNAPSHOT_FILE="$1"

# Validate snapshot file
if [ ! -f "$SNAPSHOT_FILE" ]; then
    echo "❌ Snapshot file not found: $SNAPSHOT_FILE"
    exit 1
fi

# Extract collection name from filename
BASENAME=$(basename "$SNAPSHOT_FILE" .snapshot)
COLLECTION_NAME=$(echo "$BASENAME" | sed 's/-[0-9]\{4\}-[0-9]\{2\}-[0-9]\{2\}$//')

echo "🔄 Importing collection: $COLLECTION_NAME"
echo "📁 From snapshot: $(basename "$SNAPSHOT_FILE")"

# Check if Qdrant is running
if ! curl -f http://localhost:6333 >/dev/null 2>&1; then
    echo "❌ Qdrant is not running or not accessible"
    exit 1
fi

# Copy snapshot to container
CONTAINER_SNAPSHOT_PATH="/tmp/$(basename "$SNAPSHOT_FILE")"
echo "📋 Copying snapshot to container..."
if ! docker cp "$SNAPSHOT_FILE" scanoss-qdrant:"$CONTAINER_SNAPSHOT_PATH" 2>/dev/null; then
    echo "❌ Failed to copy snapshot to container"
    exit 1
fi

# Restore collection
echo "🔄 Restoring collection..."
RESTORE_RESPONSE=$(curl -s -X PUT \
    -H "Content-Type: application/json" \
    -d "{\"location\": \"file://$CONTAINER_SNAPSHOT_PATH\", \"priority\": \"snapshot\"}" \
    "http://localhost:6333/collections/$COLLECTION_NAME/snapshots/recover")

if echo "$RESTORE_RESPONSE" | grep -q '"status":"ok"'; then
    echo "✅ Restoration initiated"
    
    # Wait for completion
    echo "⏳ Waiting for restoration to complete..."
    for i in {1..180}; do
        COLLECTION_INFO=$(curl -s "http://localhost:6333/collections/$COLLECTION_NAME" 2>/dev/null)
        POINTS_COUNT=$(echo "$COLLECTION_INFO" | grep -o '"points_count":[0-9]*' | cut -d':' -f2 || echo "0")
        
        if [ "$POINTS_COUNT" -gt 0 ]; then
            echo "✅ Collection restored successfully ($POINTS_COUNT points)"
            docker exec scanoss-qdrant rm -f "$CONTAINER_SNAPSHOT_PATH" 2>/dev/null || true
            exit 0
        fi
        
        if [ $((i % 6)) -eq 0 ]; then
            echo "Still restoring... ($((i*10))s)"
        fi
        sleep 10
    done
    
    echo "❌ Timeout waiting for restoration"
else
    echo "❌ Restoration failed"
    echo "Response: $RESTORE_RESPONSE"
fi

# Cleanup
docker exec scanoss-qdrant rm -f "$CONTAINER_SNAPSHOT_PATH" 2>/dev/null || true
exit 1