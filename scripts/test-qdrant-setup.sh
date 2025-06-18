#!/bin/bash

##########################################
#
# Test script to verify Qdrant container setup
# This script tests the fixes made to env_setup.sh
#
##########################################

echo "🧪 Testing Qdrant Container Setup"
echo "================================="

# Test 1: Check if container is running
echo "Test 1: Checking if Qdrant container is running..."
if docker ps --filter name=qdrant-server --filter status=running | grep -q qdrant-server; then
    echo "✅ Container is running"
else
    echo "❌ Container is not running"
    echo "📋 Container status:"
    docker ps -a --filter name=qdrant-server
    exit 1
fi

# Test 2: Check if API is responding
echo ""
echo "Test 2: Checking if Qdrant API is responding..."
if curl -f http://localhost:6333 >/dev/null 2>&1; then
    echo "✅ API is responding"
else
    echo "❌ API is not responding"
    echo "📋 Container logs:"
    docker logs --tail 20 qdrant-server
    exit 1
fi

# Test 3: Check collections endpoint
echo ""
echo "Test 3: Checking collections endpoint..."
COLLECTIONS_RESPONSE=$(curl -s http://localhost:6333/collections 2>/dev/null)
if echo "$COLLECTIONS_RESPONSE" | grep -q '"status":"ok"'; then
    COLLECTION_COUNT=$(echo "$COLLECTIONS_RESPONSE" | grep -o '"name":"[^"]*"' | wc -l)
    echo "✅ Collections endpoint working ($COLLECTION_COUNT collections)"
else
    echo "❌ Collections endpoint not working"
    echo "Response: $COLLECTIONS_RESPONSE"
    exit 1
fi

# Test 4: Check container health
echo ""
echo "Test 4: Checking container health..."
HEALTH_STATUS=$(docker inspect qdrant-server --format='{{.State.Health.Status}}' 2>/dev/null || echo "unknown")
if [ "$HEALTH_STATUS" = "healthy" ]; then
    echo "✅ Container is healthy"
elif [ "$HEALTH_STATUS" = "starting" ]; then
    echo "⏳ Container is still starting up"
else
    echo "⚠️  Container health: $HEALTH_STATUS"
fi

# Test 5: Check restart policy
echo ""
echo "Test 5: Checking restart policy..."
RESTART_POLICY=$(docker inspect qdrant-server --format='{{.HostConfig.RestartPolicy.Name}}' 2>/dev/null || echo "unknown")
if [ "$RESTART_POLICY" = "unless-stopped" ]; then
    echo "✅ Restart policy is correct: $RESTART_POLICY"
else
    echo "⚠️  Restart policy: $RESTART_POLICY"
fi

# Test 6: Check volume mounting
echo ""
echo "Test 6: Checking volume mounting..."
VOLUME_INFO=$(docker inspect qdrant-server --format='{{range .Mounts}}{{.Type}}:{{.Source}}->{{.Destination}} {{end}}' 2>/dev/null || echo "unknown")
if echo "$VOLUME_INFO" | grep -q "volume.*qdrant_data"; then
    echo "✅ Volume is properly mounted"
    echo "   Volume info: $VOLUME_INFO"
else
    echo "⚠️  Volume mounting may have issues"
    echo "   Volume info: $VOLUME_INFO"
fi

# Test 7: Port accessibility
echo ""
echo "Test 7: Checking port accessibility..."
HTTP_PORT_OK=false
GRPC_PORT_OK=false

if netstat -tlnp 2>/dev/null | grep -q ":6333.*docker-proxy"; then
    HTTP_PORT_OK=true
    echo "✅ HTTP port 6333 is accessible"
else
    echo "❌ HTTP port 6333 is not accessible"
fi

if netstat -tlnp 2>/dev/null | grep -q ":6334.*docker-proxy"; then
    GRPC_PORT_OK=true
    echo "✅ gRPC port 6334 is accessible"
else
    echo "❌ gRPC port 6334 is not accessible"
fi

# Final summary
echo ""
echo "🎯 Test Summary"
echo "==============="
echo "Container Status: $(if docker ps --filter name=qdrant-server --filter status=running | grep -q qdrant-server; then echo "✅ Running"; else echo "❌ Not Running"; fi)"
echo "API Response: $(if curl -f http://localhost:6333 >/dev/null 2>&1; then echo "✅ OK"; else echo "❌ Failed"; fi)"
echo "Health Status: $HEALTH_STATUS"
echo "HTTP Port: $(if [ "$HTTP_PORT_OK" = true ]; then echo "✅ OK"; else echo "❌ Failed"; fi)"
echo "gRPC Port: $(if [ "$GRPC_PORT_OK" = true ]; then echo "✅ OK"; else echo "❌ Failed"; fi)"

# Check if all critical tests passed
if docker ps --filter name=qdrant-server --filter status=running | grep -q qdrant-server && \
   curl -f http://localhost:6333 >/dev/null 2>&1 && \
   [ "$HTTP_PORT_OK" = true ] && [ "$GRPC_PORT_OK" = true ]; then
    echo ""
    echo "🎉 All critical tests passed! Qdrant container is working properly."
    exit 0
else
    echo ""
    echo "❌ Some critical tests failed. Please check the issues above."
    exit 1
fi
