#!/bin/bash

##########################################
#
# This script creates individual snapshots for each collection in Qdrant
# Used for creating collection-based distribution snapshots
#
################################################################

set -e

if [ "$1" = "-h" ] || [ "$1" = "-help" ]; then
    echo "$0 [-help] [output-dir]"
    echo "   Create individual collection snapshots for distribution"
    echo "   [output-dir] directory to save snapshots (default: collection-snapshots/)"
    exit 1
fi

OUTPUT_DIR="${1:-collection-snapshots}"
DATE=$(date +%Y-%m-%d)

echo "📸 Creating SCANOSS Collection Snapshots"
echo "========================================"

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

if [ "$COLLECTION_COUNT" -eq 0 ]; then
    echo "❌ No collections found in Qdrant"
    exit 1
fi

echo "✅ Found $COLLECTION_COUNT collections:"
echo "$COLLECTION_NAMES" | while read -r collection; do
    if [ -n "$collection" ]; then
        echo "  - $collection"
    fi
done

echo ""
echo "📸 Creating individual collection snapshots..."

# Create snapshots for each collection
CREATED_SNAPSHOTS=()
FAILED_COLLECTIONS=()

echo "$COLLECTION_NAMES" | while read -r collection; do
    if [ -z "$collection" ]; then
        continue
    fi
    
    echo ""
    echo "📦 Creating snapshot for collection: $collection"
    
    # Create collection snapshot
    CREATE_RESPONSE=$(curl -s -X POST "http://localhost:6333/collections/$collection/snapshots" 2>/dev/null)
    
    if echo "$CREATE_RESPONSE" | grep -q '"status":"ok"'; then
        echo "✅ Snapshot creation initiated for $collection"
        
        # Extract snapshot name from response
        CREATED_SNAPSHOT=$(echo "$CREATE_RESPONSE" | grep -o '"name":"[^"]*"' | cut -d'"' -f4)
        if [ -n "$CREATED_SNAPSHOT" ]; then
            echo "📦 Snapshot name: $CREATED_SNAPSHOT"
            echo "$collection:$CREATED_SNAPSHOT" >> "$OUTPUT_DIR/.snapshot_mapping"
        else
            echo "⚠️  Could not extract snapshot name for $collection"
            echo "$collection" >> "$OUTPUT_DIR/.failed_collections"
            continue
        fi
    else
        echo "❌ Snapshot creation failed for $collection!"
        echo "Response: $CREATE_RESPONSE"
        echo "$collection" >> "$OUTPUT_DIR/.failed_collections"
        continue
    fi
    
    # Wait for this collection's snapshot to be ready
    echo "⏳ Waiting for $collection snapshot to complete..."
    timeout=600 # 10 minutes per collection
    counter=0
    snapshot_ready=false
    
    while [ $counter -lt $timeout ]; do
        # List snapshots for this collection
        LIST_RESPONSE=$(curl -s "http://localhost:6333/collections/$collection/snapshots" 2>/dev/null)
        if echo "$LIST_RESPONSE" | grep -q "\"$CREATED_SNAPSHOT\""; then
            echo "✅ Snapshot ready for $collection"
            snapshot_ready=true
            break
        fi
        
        if [ $((counter % 30)) -eq 0 ]; then
            echo "Still creating snapshot for $collection... ($counter/$timeout seconds)"
        fi
        sleep 5
        counter=$((counter + 5))
    done
    
    if [ "$snapshot_ready" = false ]; then
        echo "❌ Timeout waiting for $collection snapshot"
        echo "$collection" >> "$OUTPUT_DIR/.failed_collections"
        continue
    fi
    
    # Download the snapshot
    echo "📥 Downloading snapshot for $collection..."
    OUTPUT_FILE="$OUTPUT_DIR/${collection}-${DATE}.snapshot"
    
    if curl -f "http://localhost:6333/collections/$collection/snapshots/$CREATED_SNAPSHOT" --output "$OUTPUT_FILE" 2>/dev/null; then
        SNAPSHOT_SIZE=$(du -h "$OUTPUT_FILE" | cut -f1)
        echo "✅ Downloaded: $OUTPUT_FILE ($SNAPSHOT_SIZE)"
        echo "$collection:$OUTPUT_FILE" >> "$OUTPUT_DIR/.downloaded_snapshots"
    else
        echo "❌ Failed to download snapshot for $collection"
        echo "$collection" >> "$OUTPUT_DIR/.failed_collections"
        continue
    fi
    
    # Clean up the snapshot from Qdrant server
    echo "🗑️  Cleaning up server snapshot for $collection..."
    DELETE_RESPONSE=$(curl -s -X DELETE "http://localhost:6333/collections/$collection/snapshots/$CREATED_SNAPSHOT" 2>/dev/null)
    if echo "$DELETE_RESPONSE" | grep -q '"status":"ok"'; then
        echo "✅ Server snapshot cleaned up for $collection"
    else
        echo "⚠️  Failed to clean up server snapshot for $collection (not critical)"
    fi
done

# Summary
echo ""
echo "🎉 Collection snapshot creation complete!"
echo ""

# Count results
DOWNLOADED_COUNT=0
FAILED_COUNT=0

if [ -f "$OUTPUT_DIR/.downloaded_snapshots" ]; then
    DOWNLOADED_COUNT=$(wc -l < "$OUTPUT_DIR/.downloaded_snapshots")
fi

if [ -f "$OUTPUT_DIR/.failed_collections" ]; then
    FAILED_COUNT=$(wc -l < "$OUTPUT_DIR/.failed_collections")
fi

echo "📊 Summary:"
echo "  ✅ Successfully created: $DOWNLOADED_COUNT snapshots"
echo "  ❌ Failed: $FAILED_COUNT collections"
echo ""

if [ -f "$OUTPUT_DIR/.downloaded_snapshots" ]; then
    echo "📦 Created snapshots:"
    while IFS=: read -r collection file; do
        if [ -n "$collection" ] && [ -n "$file" ]; then
            SIZE=$(du -h "$file" 2>/dev/null | cut -f1 || echo "unknown")
            echo "  - $collection: $(basename "$file") ($SIZE)"
        fi
    done < "$OUTPUT_DIR/.downloaded_snapshots"
fi

if [ -f "$OUTPUT_DIR/.failed_collections" ]; then
    echo ""
    echo "❌ Failed collections:"
    cat "$OUTPUT_DIR/.failed_collections" | while read -r collection; do
        if [ -n "$collection" ]; then
            echo "  - $collection"
        fi
    done
fi

echo ""
echo "📁 Snapshots directory: $OUTPUT_DIR"
echo "📅 Date: $DATE"
echo ""
echo "Use these snapshots with the collection restoration script:"
echo "  ./scripts/import-collections.sh $OUTPUT_DIR"
echo ""
