#!/bin/bash

##########################################
#
# Recreates the Qdrant database from local snapshot files.
#
# For every *.snapshot file in the snapshots directory it uploads the file
# to Qdrant, which restores (and creates if missing) the collection from the
# snapshot. The collection name is taken from the file name, so files are
# expected to be named <collection>.snapshot (as produced by
# qdrant-generate-snapshots.sh).
#
# Usage:
#   ./scripts/qdrant-restore-snapshots.sh [snapshots_dir]
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

if [ ! -d "$SNAPSHOTS_DIR" ]; then
  echo "ERROR: snapshots directory '$SNAPSHOTS_DIR' does not exist." >&2
  exit 1
fi

shopt -s nullglob
SNAPSHOTS=("$SNAPSHOTS_DIR"/*.snapshot)
if [ ${#SNAPSHOTS[@]} -eq 0 ]; then
  echo "No *.snapshot files found in '$SNAPSHOTS_DIR'." >&2
  exit 1
fi

echo "Restoring collections into $QDRANT_HTTP from '$SNAPSHOTS_DIR'"
echo

for file in "${SNAPSHOTS[@]}"; do
  col=$(basename "$file" .snapshot)
  printf "%-25s " "$col"

  # priority=snapshot makes the uploaded snapshot win over any existing data,
  # effectively recreating the collection from the snapshot.
  resp=$(curl -s -X POST \
    "$QDRANT_HTTP/collections/$col/snapshots/upload?priority=snapshot" \
    -H 'Content-Type:multipart/form-data' \
    -F "snapshot=@${file}")

  if echo "$resp" | jq -e '.result == true' >/dev/null 2>&1; then
    echo "OK"
  else
    echo "FAILED -> $(echo "$resp" | jq -c '.status // .' 2>/dev/null || echo "$resp")"
  fi
done

echo
echo "Done. Current collections:"
curl -s "$QDRANT_HTTP/collections" | jq -r '.result.collections[].name' | sort | sed 's/^/  - /'
