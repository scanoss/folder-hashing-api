#!/bin/bash

##########################################
#
# This script is designed to run by Systemd SCANOSS Folder Hashing API service.
# It rotates scanoss log file and starts the Folder Hashing API.
# Install it in /usr/local/bin
#
# Usage: scanoss-hfh-api.sh [environment] [config-method] [config-path]
#   environment:   Environment name (default: prod)
#   config-method: json, env, or auto (default: json)
#   config-path:   Custom config file path (optional)
#
################################################################

DEFAULT_ENV="prod"
DEFAULT_CONFIG_METHOD="json"

ENVIRONMENT="${1:-$DEFAULT_ENV}"
CONFIG_METHOD="${2:-$DEFAULT_CONFIG_METHOD}"
CUSTOM_CONFIG_PATH="$3"

LOGFILE=/var/log/scanoss/hfh/scanoss-hfh-${ENVIRONMENT}.log

# Determine configuration approach
case "$CONFIG_METHOD" in
    "json")
        if [ -n "$CUSTOM_CONFIG_PATH" ]; then
            CONF_FILE="$CUSTOM_CONFIG_PATH"
        else
            CONF_FILE="/usr/local/etc/scanoss/hfh/app-config-${ENVIRONMENT}.json"
        fi
        CONFIG_FLAG="--json-config"
        ;;
    "env")
        if [ -n "$CUSTOM_CONFIG_PATH" ]; then
            CONF_FILE="$CUSTOM_CONFIG_PATH"
        else
            CONF_FILE="/usr/local/etc/scanoss/hfh/.env-${ENVIRONMENT}"
        fi
        CONFIG_FLAG="--env-config"
        ;;
    "auto")
        # Auto-detect: prefer JSON, fallback to .env
        JSON_FILE="/usr/local/etc/scanoss/hfh/app-config-${ENVIRONMENT}.json"
        ENV_FILE="/usr/local/etc/scanoss/hfh/.env-${ENVIRONMENT}"
        
        if [ -f "$JSON_FILE" ]; then
            CONF_FILE="$JSON_FILE"
            CONFIG_FLAG="--json-config"
        elif [ -f "$ENV_FILE" ]; then
            CONF_FILE="$ENV_FILE"
            CONFIG_FLAG="--env-config"
        else
            echo "❌ No configuration file found for environment: $ENVIRONMENT"
            echo "Expected: $JSON_FILE or $ENV_FILE"
            exit 1
        fi
        ;;
    *)
        echo "❌ Invalid config method: $CONFIG_METHOD"
        echo "Valid options: json, env, auto"
        exit 1
        ;;
esac

# Validate configuration file exists (unless using env-only mode)
if [ "$CONFIG_METHOD" != "env-only" ] && [ ! -f "$CONF_FILE" ]; then
    echo "❌ Configuration file not found: $CONF_FILE"
    echo "Please ensure the configuration file exists or use a different config method"
    exit 1
fi

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
echo "Starting SCANOSS Folder Hashing API with $CONFIG_FLAG: $CONF_FILE"

exec /usr/local/bin/scanoss-hfh-api $CONFIG_FLAG "$CONF_FILE" >"$LOGFILE" 2>&1
