#!/bin/bash

##########################################
#
# Generates a Qdrant snapshot for every collection and downloads each one
# into a local snapshots directory.
#
# Snapshots are created through the Qdrant HTTP API and downloaded to the
# host, so this works regardless of how the container storage is mounted.
# Pair this with qdrant-restore-snapshots.sh to recreate the database.
#
# Usage:
#   ./scripts/qdrant-generate-snapshots.sh [snapshots_dir]
#
# Environment variables:
#   QDRANT_HTTP   Qdrant HTTP endpoint (default: http://localhost:6333)
#
################################################################
set -euo pipefail

QDRANT_HTTP="${QDRANT_HTTP:-http://localhost:6333}"
SNAPSHOTS_DIR="${1:-snapshots}"

command -v jq >/dev/null 2>&1 || { echo "ERROR: jq is required but not installed." >&2; exit 1; }

# Verify Qdrant is reachable
if ! curl -sf -o /dev/null "$QDRANT_HTTP/collections"; then
  echo "ERROR: cannot reach Qdrant at $QDRANT_HTTP" >&2
  exit 1
fi

mkdir -p "$SNAPSHOTS_DIR"

COLLECTIONS=$(curl -s "$QDRANT_HTTP/collections" | jq -r '.result.collections[].name' | sort)

if [ -z "$COLLECTIONS" ]; then
  echo "No collections found at $QDRANT_HTTP"
  exit 0
fi

echo "Generating snapshots into '$SNAPSHOTS_DIR' from $QDRANT_HTTP"
echo

for col in $COLLECTIONS; do
  printf "%-25s " "$col"

  # Create the snapshot on the server
  snap=$(curl -s -X POST "$QDRANT_HTTP/collections/$col/snapshots" | jq -r '.result.name // empty')
  if [ -z "$snap" ]; then
    echo "FAILED to create snapshot"
    continue
  fi

  # Download it to the local snapshots directory as <collection>.snapshot
  out="$SNAPSHOTS_DIR/$col.snapshot"
  if curl -sf "$QDRANT_HTTP/collections/$col/snapshots/$snap" --output "$out"; then
    echo "OK -> $out"
  else
    echo "FAILED to download $snap"
  fi

  # Remove the server-side snapshot to avoid accumulating disk usage
  curl -s -X DELETE "$QDRANT_HTTP/collections/$col/snapshots/$snap" >/dev/null
done

echo
echo "Done. Snapshots saved in '$SNAPSHOTS_DIR'."
