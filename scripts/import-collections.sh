#!/bin/bash

##########################################
#
# This script restores individual collection snapshots to Qdrant using REST API
# Used for setting up Qdrant from collection-based snapshots
#
################################################################

set -e

if [ "$1" = "-h" ] || [ "$1" = "-help" ]; then
    echo "$0 [-help] [snapshots-dir]"
    echo "   Restore collections from individual snapshots using REST API"
    echo "   [snapshots-dir] directory containing collection snapshots (required)"
    echo ""
    echo "Examples:"
    echo "   $0 collection-snapshots/"
    echo "   $0 /path/to/snapshots/"
    exit 1
fi

SNAPSHOTS_DIR="$1"

# Validate snapshots directory
if [ -z "$SNAPSHOTS_DIR" ]; then
    echo "ERROR: Snapshots directory is required"
    echo "Usage: $0 [snapshots-dir]"
    echo "Example: $0 collection-snapshots/"
    exit 1
fi

if [ ! -d "$SNAPSHOTS_DIR" ]; then
    echo "ERROR: Snapshots directory not found: $SNAPSHOTS_DIR"
    exit 1
fi

echo "🔄 Restoring SCANOSS Collections from Snapshots"
echo "=============================================="
echo "📁 Snapshots directory: $SNAPSHOTS_DIR"

# Check if Qdrant is running
if ! curl -f http://localhost:6333 >/dev/null 2>&1; then
    echo "❌ Qdrant is not running or not accessible"
    echo "Please ensure Qdrant is running on localhost:6333"
    echo "You can start it with: cd /usr/local/etc/scanoss/qdrant && docker-compose up -d"
    exit 1
fi

# Find all snapshot files
SNAPSHOT_FILES=$(find "$SNAPSHOTS_DIR" -name "*.snapshot" -type f 2>/dev/null || true)

if [ -z "$SNAPSHOT_FILES" ]; then
    echo "❌ No snapshot files found in $SNAPSHOTS_DIR"
    echo "Please ensure the directory contains .snapshot files"
    exit 1
fi

SNAPSHOT_COUNT=$(echo "$SNAPSHOT_FILES" | wc -l)
echo "📦 Found $SNAPSHOT_COUNT snapshot files:"

echo "$SNAPSHOT_FILES" | while read -r snapshot_file; do
    if [ -n "$snapshot_file" ]; then
        BASENAME=$(basename "$snapshot_file")
        SIZE=$(du -h "$snapshot_file" 2>/dev/null | cut -f1 || echo "unknown")
        echo "  - $BASENAME ($SIZE)"
    fi
done

echo ""
echo "🚀 Starting collection restoration..."

# Create restoration log files
RESTORE_LOG="$SNAPSHOTS_DIR/.restoration_log"
SUCCESS_LOG="$SNAPSHOTS_DIR/.restored_collections"
FAILED_LOG="$SNAPSHOTS_DIR/.failed_restorations"

# Clear previous logs
> "$RESTORE_LOG"
> "$SUCCESS_LOG"
> "$FAILED_LOG"

RESTORED_COUNT=0
FAILED_COUNT=0

echo "$SNAPSHOT_FILES" | while read -r snapshot_file; do
    if [ -z "$snapshot_file" ]; then
        continue
    fi
    
    # Extract collection name from filename
    BASENAME=$(basename "$snapshot_file" .snapshot)
    # Remove date suffix if present (e.g., collection-2025-06-18 -> collection)
    COLLECTION_NAME=$(echo "$BASENAME" | sed 's/-[0-9]\{4\}-[0-9]\{2\}-[0-9]\{2\}$//')
    
    echo ""
    echo "📥 Restoring collection: $COLLECTION_NAME"
    echo "   From: $(basename "$snapshot_file")"
    
    # Copy snapshot to a location accessible by Qdrant container
    CONTAINER_SNAPSHOT_PATH="/tmp/$(basename "$snapshot_file")"
    
    # For Docker setup, we need to copy the file into the container
    echo "📋 Copying snapshot to container..."
    if docker cp "$snapshot_file" scanoss-qdrant:"$CONTAINER_SNAPSHOT_PATH" 2>/dev/null; then
        echo "✅ Snapshot copied to container"
    else
        echo "❌ Failed to copy snapshot to container"
        echo "$COLLECTION_NAME:copy_failed" >> "$FAILED_LOG"
        FAILED_COUNT=$((FAILED_COUNT + 1))
        continue
    fi
    
    # Restore collection using REST API
    echo "🔄 Initiating restoration via REST API..."
    
    RESTORE_PAYLOAD=$(cat <<EOF
{
  "location": "file://$CONTAINER_SNAPSHOT_PATH",
  "priority": "snapshot"
}
EOF
)
    
    RESTORE_RESPONSE=$(curl -s -X PUT \
        -H "Content-Type: application/json" \
        -d "$RESTORE_PAYLOAD" \
        "http://localhost:6333/collections/$COLLECTION_NAME/snapshots/recover" 2>/dev/null)
    
    if echo "$RESTORE_RESPONSE" | grep -q '"status":"ok"'; then
        echo "✅ Restoration initiated successfully for $COLLECTION_NAME"
        
        # Wait for restoration to complete
        echo "⏳ Waiting for restoration to complete..."
        timeout=1800 # 30 minutes per collection
        counter=0
        restoration_complete=false
        
        while [ $counter -lt $timeout ]; do
            # Check if collection exists and has data
            COLLECTION_INFO=$(curl -s "http://localhost:6333/collections/$COLLECTION_NAME" 2>/dev/null)
            
            if echo "$COLLECTION_INFO" | grep -q '"status":"ok"'; then
                # Check if collection has points
                POINTS_COUNT=$(echo "$COLLECTION_INFO" | grep -o '"points_count":[0-9]*' | cut -d':' -f2 || echo "0")
                if [ "$POINTS_COUNT" -gt 0 ]; then
                    echo "✅ Collection $COLLECTION_NAME restored successfully ($POINTS_COUNT points)"
                    echo "$COLLECTION_NAME:$POINTS_COUNT" >> "$SUCCESS_LOG"
                    restoration_complete=true
                    RESTORED_COUNT=$((RESTORED_COUNT + 1))
                    break
                fi
            fi
            
            if [ $((counter % 60)) -eq 0 ]; then
                echo "Still restoring $COLLECTION_NAME... ($counter/$timeout seconds)"
            fi
            sleep 10
            counter=$((counter + 10))
        done
        
        if [ "$restoration_complete" = false ]; then
            echo "❌ Timeout waiting for $COLLECTION_NAME restoration"
            echo "$COLLECTION_NAME:timeout" >> "$FAILED_LOG"
            FAILED_COUNT=$((FAILED_COUNT + 1))
        fi
        
    else
        echo "❌ Restoration failed for $COLLECTION_NAME"
        echo "Response: $RESTORE_RESPONSE"
        echo "$COLLECTION_NAME:api_failed" >> "$FAILED_LOG"
        FAILED_COUNT=$((FAILED_COUNT + 1))
    fi
    
    # Clean up temporary snapshot file from container
    echo "🗑️  Cleaning up temporary files..."
    docker exec scanoss-qdrant rm -f "$CONTAINER_SNAPSHOT_PATH" 2>/dev/null || true
    
    # Log the restoration attempt
    echo "$(date): $COLLECTION_NAME - $(if [ "$restoration_complete" = true ]; then echo "SUCCESS"; else echo "FAILED"; fi)" >> "$RESTORE_LOG"
done

# Final summary
echo ""
echo "🎉 Collection restoration complete!"
echo ""

# Read final counts from log files
FINAL_RESTORED=0
FINAL_FAILED=0

if [ -f "$SUCCESS_LOG" ]; then
    FINAL_RESTORED=$(wc -l < "$SUCCESS_LOG" 2>/dev/null || echo "0")
fi

if [ -f "$FAILED_LOG" ]; then
    FINAL_FAILED=$(wc -l < "$FAILED_LOG" 2>/dev/null || echo "0")
fi

echo "📊 Final Summary:"
echo "  ✅ Successfully restored: $FINAL_RESTORED collections"
echo "  ❌ Failed: $FINAL_FAILED collections"
echo ""

if [ -f "$SUCCESS_LOG" ] && [ "$FINAL_RESTORED" -gt 0 ]; then
    echo "✅ Successfully restored collections:"
    while IFS=: read -r collection points; do
        if [ -n "$collection" ] && [ -n "$points" ]; then
            echo "  - $collection ($points points)"
        fi
    done < "$SUCCESS_LOG"
fi

if [ -f "$FAILED_LOG" ] && [ "$FINAL_FAILED" -gt 0 ]; then
    echo ""
    echo "❌ Failed collections:"
    while IFS=: read -r collection reason; do
        if [ -n "$collection" ] && [ -n "$reason" ]; then
            echo "  - $collection (reason: $reason)"
        fi
    done < "$FAILED_LOG"
fi

echo ""
echo "📋 Restoration logs saved in:"
echo "  - Full log: $RESTORE_LOG"
echo "  - Success log: $SUCCESS_LOG"
echo "  - Failed log: $FAILED_LOG"

# Verify final state
echo ""
echo "🔍 Verifying final Qdrant state..."
FINAL_COLLECTIONS=$(curl -s http://localhost:6333/collections 2>/dev/null)
if echo "$FINAL_COLLECTIONS" | grep -q '"status":"ok"'; then
    TOTAL_COLLECTIONS=$(echo "$FINAL_COLLECTIONS" | grep -o '"name":"[^"]*"' | wc -l)
    echo "✅ Qdrant is running with $TOTAL_COLLECTIONS collections"
    echo "🌐 Qdrant dashboard: http://localhost:6333/dashboard"
else
    echo "⚠️  Could not verify final Qdrant state"
fi

echo ""
if [ "$FINAL_FAILED" -eq 0 ]; then
    echo "🎉 All collections restored successfully!"
    exit 0
else
    echo "⚠️  Some collections failed to restore. Check the logs above."
    exit 1
fi
