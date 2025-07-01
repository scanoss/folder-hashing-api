#!/bin/bash

##########################################
#
# SCANOSS HFH API - Docker Health Check Script
#
# This script performs health checks for the HFH API container
# Supports both HTTP and HTTPS endpoints based on TLS configuration
#
################################################################

set -e

# Default values
HEALTH_CHECK_TIMEOUT=5
REST_PORT="${REST_PORT:-40061}"
TLS_CERT_FILE="${COMP_TLS_CERT:-}"
TLS_KEY_FILE="${COMP_TLS_KEY:-}"

# Determine protocol based on TLS configuration
PROTOCOL="http"
CURL_OPTS="-f"

# Check if TLS is configured (both cert and key files must exist)
if [ -n "$TLS_CERT_FILE" ] && [ -n "$TLS_KEY_FILE" ] && [ -f "$TLS_CERT_FILE" ] && [ -f "$TLS_KEY_FILE" ]; then
    PROTOCOL="https"
    # For self-signed certificates, skip verification in health checks
    CURL_OPTS="-f -k"
fi

# Construct health check URL
HEALTH_URL="${PROTOCOL}://localhost:${REST_PORT}/api/v2/scanning/echo"

# Perform health check
echo "Performing health check: $HEALTH_URL"
if curl $CURL_OPTS \
    --connect-timeout $HEALTH_CHECK_TIMEOUT \
    --max-time $HEALTH_CHECK_TIMEOUT \
    -X POST \
    -H "Content-Type: application/json" \
    -d '{"message":"health-check"}' \
    "$HEALTH_URL" >/dev/null 2>&1; then
    echo "Health check passed"
    exit 0
else
    echo "Health check failed"
    exit 1
fi