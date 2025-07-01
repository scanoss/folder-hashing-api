#!/bin/bash

##########################################
#
# SCANOSS Folder Hashing API - Docker Deployment Script
#
# This script deploys the SCANOSS HFH API and Qdrant using Docker Compose
# Supports development and production environments
#
################################################################

set -e

if [ "$1" = "-h" ] || [ "$1" = "-help" ]; then
    echo "$0 [-help] [environment] [action]"
    echo "   Deploy SCANOSS Folder Hashing API using Docker Compose"
    echo ""
    echo "Arguments:"
    echo "   [environment]   Environment to deploy: dev, prod (default: prod)"
    echo "   [action]        Action to perform: up, down, logs, status (default: up)"
    echo ""
    echo "Examples:"
    echo "   $0                    # Deploy production environment"
    echo "   $0 dev                # Deploy development environment"
    echo "   $0 prod up            # Deploy production environment"
    echo "   $0 dev down           # Stop development environment"
    echo "   $0 prod logs          # View production logs"
    echo "   $0 prod status        # Check production status"
    echo ""
    echo "Prerequisites:"
    echo "   - Docker and Docker Compose installed"
    echo "   - Configuration file in ./config/app-config.json"
    echo "   - Snapshots directory ./snapshots/ (for collection import/export)"
    exit 1
fi

ENVIRONMENT="${1:-prod}"
ACTION="${2:-up}"

# Validate environment
case "$ENVIRONMENT" in
"dev" | "development")
    ENVIRONMENT="dev"
    COMPOSE_FILES="-f docker-compose.yml -f docker-compose.dev.yml"
    echo "🚀 SCANOSS HFH API - Development Deployment"
    ;;
"prod" | "production")
    ENVIRONMENT="prod"
    COMPOSE_FILES="-f docker-compose.yml -f docker-compose.prod.yml"
    echo "🚀 SCANOSS HFH API - Production Deployment"
    ;;
*)
    echo "❌ Invalid environment: $ENVIRONMENT"
    echo "Valid environments: dev, prod"
    exit 1
    ;;
esac

echo "=================================="
echo "Environment: $ENVIRONMENT"
echo "Action: $ACTION"
echo ""

# Check prerequisites
echo "🔍 Checking prerequisites..."

# Check Docker
if ! command -v docker &>/dev/null; then
    echo "❌ Docker is required but not installed"
    echo "Please install Docker: https://docs.docker.com/engine/install/"
    exit 1
fi

# Check Docker Compose
if ! command -v docker-compose &>/dev/null; then
    echo "❌ Docker Compose is required but not installed"
    echo "Please install Docker Compose"
    exit 1
fi

# Check if Docker daemon is running
if ! docker info &>/dev/null; then
    echo "❌ Docker daemon is not running"
    echo "Please start Docker service"
    exit 1
fi

echo "✅ Docker prerequisites check passed"

# Helper function to check required images are available
check_required_images() {
    echo "🐳 Checking required Docker images..."
    
    # Get version from package metadata or use 'latest' as fallback
    local version="latest"
    if [ -f "./package-info.json" ]; then
        version=$(grep '"version"' package-info.json | cut -d'"' -f4 2>/dev/null || echo "latest")
    fi
    
    local required_images=("scanoss/hfh-api:${version}" "qdrant/qdrant:latest")
    local missing_images=()
    
    for image in "${required_images[@]}"; do
        if docker image inspect "$image" >/dev/null 2>&1; then
            echo "✅ Image available: $image"
        else
            echo "❌ Image missing: $image"
            missing_images+=("$image")
        fi
    done
    
    if [ "${#missing_images[@]}" -gt 0 ]; then
        echo ""
        echo "❌ Missing required Docker images!"
        echo "📥 Load images first with:"
        echo "   ./scripts/load-images.sh"
        echo ""
        echo "💡 If you don't have the load-images.sh script, you may need to:"
        echo "   1. Build the images locally, or"
        echo "   2. Pull them from a registry"
        echo ""
        exit 1
    fi
    
    echo "✅ All required images are available"
}

# Create necessary directories
echo "📁 Creating necessary directories..."
mkdir -p ./config
mkdir -p ./config/certs
mkdir -p ./snapshots

# Perform the requested action
case "$ACTION" in
"up")
    echo "🚀 Starting SCANOSS HFH API services..."

    # Check required images are available
    check_required_images
    echo ""

    # Check for configuration file
    if [ ! -f "./config/app-config.json" ]; then
        echo "⚠️  Configuration file not found: ./config/app-config.json"
        if [ -f "./config.example.json" ]; then
            echo "📝 Copying example configuration..."
            cp "./config.example.json" "./config/app-config.json"
            echo "✅ Configuration template copied to ./config/app-config.json"
            echo "📝 Please review and customize the configuration before proceeding"
        elif [ -f "./config/app-config.example.json" ]; then
            echo "📝 Copying example configuration..."
            cp "./config/app-config.example.json" "./config/app-config.json"
            echo "✅ Configuration template copied to ./config/app-config.json"
            echo "📝 Please review and customize the configuration before proceeding"
        else
            echo "❌ No configuration template found"
            echo "Please create ./config/app-config.json with your configuration"
            exit 1
        fi
    fi

    # Start services
    docker-compose $COMPOSE_FILES up -d

    echo ""
    echo "⏳ Waiting for services to be ready..."

    # Wait for Qdrant to be healthy
    timeout=120
    counter=0
    while [ $counter -lt $timeout ]; do
        if docker-compose $COMPOSE_FILES ps qdrant | grep -q "healthy"; then
            echo "✅ Qdrant is healthy"
            break
        fi
        if [ $((counter % 15)) -eq 0 ]; then
            echo "Still waiting for Qdrant... ($counter/$timeout seconds)"
        fi
        sleep 3
        counter=$((counter + 3))
    done

    if [ $counter -ge $timeout ]; then
        echo "❌ Timeout waiting for Qdrant to be healthy"
        echo "📋 Check logs: docker-compose $COMPOSE_FILES logs qdrant"
        exit 1
    fi

    # Wait for HFH API to be healthy
    timeout=180
    counter=0
    while [ $counter -lt $timeout ]; do
        if docker-compose $COMPOSE_FILES ps hfh-api | grep -q "healthy"; then
            echo "✅ HFH API is healthy"
            break
        fi
        if [ $((counter % 15)) -eq 0 ]; then
            echo "Still waiting for HFH API... ($counter/$timeout seconds)"
        fi
        sleep 3
        counter=$((counter + 3))
    done

    if [ $counter -ge $timeout ]; then
        echo "❌ Timeout waiting for HFH API to be healthy"
        echo "📋 Check logs: docker-compose $COMPOSE_FILES logs hfh-api"
        exit 1
    fi

    echo ""
    echo "🎉 SCANOSS HFH API deployment complete!"
    echo ""
    
    # Check if TLS is configured
    TLS_CONFIGURED="no"
    if [ -f "./config/certs/server.crt" ] && [ -f "./config/certs/server.key" ]; then
        TLS_CONFIGURED="yes"
    fi
    
    echo "🌐 Service endpoints:"
    if [ "$TLS_CONFIGURED" = "yes" ]; then
        echo "  - REST API:        https://localhost:40061 (TLS enabled)"
        echo "  - gRPC API:        localhost:50061 (TLS enabled)"
    else
        echo "  - REST API:        http://localhost:40061"
        echo "  - gRPC API:        localhost:50061"
    fi
    echo "  - Dynamic Logging: localhost:60061"
    echo "  - Qdrant API:      http://localhost:6333"
    echo "  - Qdrant Dashboard: http://localhost:6333/dashboard"
    echo ""
    if [ "$TLS_CONFIGURED" = "no" ]; then
        echo "🔐 TLS Setup (optional):"
        echo "  - Run: ./scripts/setup-tls.sh /path/to/cert.crt /path/to/cert.key"
        echo ""
    fi
    echo "📋 Next steps:"
    echo "  - Import collections: ./scripts/import-collections.sh /path/to/snapshots/"
    echo "  - View logs: $0 $ENVIRONMENT logs"
    echo "  - Check status: $0 $ENVIRONMENT status"
    ;;

"down")
    echo "🛑 Stopping SCANOSS HFH API services..."
    docker-compose $COMPOSE_FILES down
    echo "✅ Services stopped"
    ;;

"logs")
    echo "📋 Viewing service logs..."
    docker-compose $COMPOSE_FILES logs -f
    ;;

"status")
    echo "📊 Service status:"
    echo ""
    docker-compose $COMPOSE_FILES ps
    echo ""

    # Check service health
    echo "🔍 Health status:"

    # Check Qdrant
    if curl -f http://localhost:6333/collections >/dev/null 2>&1; then
        COLLECTIONS=$(curl -s http://localhost:6333/collections | grep -o '"name":"[^"]*"' | wc -l || echo "0")
        echo "  ✅ Qdrant: Healthy ($COLLECTIONS collections)"
    else
        echo "  ❌ Qdrant: Not responding"
    fi

    # Check HFH API
    if curl -f -X POST -H "Content-Type: application/json" -d '{"message":"health-check"}' http://localhost:40061/api/v2/scanning/echo >/dev/null 2>&1; then
        echo "  ✅ HFH API: Healthy"
    else
        echo "  ❌ HFH API: Not responding"
    fi
    ;;

"restart")
    echo "🔄 Restarting SCANOSS HFH API services..."
    docker-compose $COMPOSE_FILES restart
    echo "✅ Services restarted"
    ;;

"pull")
    echo "📥 Pulling latest images..."
    docker-compose $COMPOSE_FILES pull
    echo "✅ Images updated"
    ;;

*)
    echo "❌ Invalid action: $ACTION"
    echo "Valid actions: up, down, logs, status, restart, pull"
    exit 1
    ;;
esac
