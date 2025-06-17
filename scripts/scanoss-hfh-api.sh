#!/bin/bash

##########################################
#
# This script is designed to run by Systemd SCANOSS Folder Hashing API service.
# It rotates scanoss log file and starts the Folder Hashing API.
# Install it in /usr/local/bin
#
################################################################

DEFAULT_ENV="prod"
ENVIRONMENT="${1:-$DEFAULT_ENV}"
LOGFILE=/var/log/scanoss/hfh/scanoss-hfh-${ENVIRONMENT}.log
CONF_FILE=/usr/local/etc/scanoss/hfh/app-config-${ENVIRONMENT}.json

# Rotate log
if [ -f "$LOGFILE" ]; then
    echo "rotating logfile..."
    TIMESTAMP=$(date '+%Y%m%d-%H%M%S')
    BACKUP_FILE=$LOGFILE.$TIMESTAMP
    cp "$LOGFILE" "$BACKUP_FILE"
    gzip -f "$BACKUP_FILE"
fi
echo >"$LOGFILE"

# Wait for Qdrant to be ready
echo "Checking Qdrant availability..."
timeout=300 # 5 minutes timeout
counter=0
while [ $counter -lt $timeout ]; do
    if curl -f http://localhost:6333/health >/dev/null 2>&1; then
        echo "✅ Qdrant is ready!"
        break
    fi
    if [ $((counter % 30)) -eq 0 ]; then
        echo "Waiting for Qdrant... ($counter/$timeout seconds)"
    fi
    sleep 2
    counter=$((counter + 2))
done

if [ $counter -ge $timeout ]; then
    echo "❌ Timeout waiting for Qdrant to be ready"
    echo "Please check Qdrant status: docker ps | grep scanoss-qdrant"
    exit 1
fi

# Start API
echo "Starting SCANOSS Folder Hashing API with config: $CONF_FILE"

exec /usr/local/bin/scanoss-hfh-api --json-config "$CONF_FILE" >"$LOGFILE" 2>&1
