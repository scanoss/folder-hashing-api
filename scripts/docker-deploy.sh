#!/bin/bash

##########################################
#
# SCANOSS Folder Hashing API - Docker Deployment Script
#
# This script deploys the SCANOSS HFH API and optionally Qdrant using Docker Compose
# Supports independent service deployment
#
################################################################

set -e

if [ "$1" = "-h" ] || [ "$1" = "-help" ]; then
    echo "$0 [-help] [service] [action]"
    echo "   Deploy SCANOSS Folder Hashing API services using Docker Compose"
    echo ""
    echo "Arguments:"
    echo "   [service]       Service to deploy: api, qdrant, all (default: api)"
    echo "   [action]        Action to perform: up, down, logs, status (default: up)"
    echo ""
    echo "Examples:"
    echo "   $0                    # Deploy API service only"
    echo "   $0 qdrant             # Deploy Qdrant only"
    echo "   $0 all                # Deploy both API and Qdrant"
    echo "   $0 api down           # Stop API service"
    echo "   $0 qdrant logs        # View Qdrant logs"
    echo "   $0 all status         # Check status of all services"
    echo ""
    echo "Prerequisites:"
    echo "   - Docker and Docker Compose installed"
    echo "   - Configuration file in ./config/app-config.json"
    echo "   - Snapshots directory ./snapshots/ (for collection import/export)"
    echo ""
    echo "Note: Qdrant should be started before the API service"
    exit 1
fi

SERVICE="${1:-api}"
ACTION="${2:-up}"

# Validate service
case "$SERVICE" in
"api")
    COMPOSE_FILES="-f docker-compose.yml"
    SERVICE_NAME="HFH API"
    ;;
"qdrant")
    COMPOSE_FILES="-f docker-compose.qdrant.yml"
    SERVICE_NAME="Qdrant"
    ;;
"all")
    COMPOSE_FILES="-f docker-compose.qdrant.yml -f docker-compose.yml"
    SERVICE_NAME="All services"
    ;;
*)
    echo "❌ Invalid service: $SERVICE"
    echo "Valid services: api, qdrant, all"
    exit 1
    ;;
esac

echo "🚀 SCANOSS Folder Hashing API - Docker Deployment"
echo "============================================="
echo "Service: $SERVICE_NAME"
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
    local required_images=()
    if [[ "$SERVICE" == "api" ]] || [[ "$SERVICE" == "all" ]]; then
        required_images+=("scanoss/hfh-api:${version}")
    fi
    if [[ "$SERVICE" == "qdrant" ]] || [[ "$SERVICE" == "all" ]]; then
        required_images+=("qdrant/qdrant:latest")
    fi

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
    echo "🚀 Starting $SERVICE_NAME..."

    # Check required images are available
    check_required_images
    echo ""

    # Check for configuration file (only for API service)
    if [[ "$SERVICE" == "api" ]] || [[ "$SERVICE" == "all" ]]; then
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
    fi

    # Start services
    docker-compose $COMPOSE_FILES up -d

    echo ""
    echo "⏳ Waiting for services to start..."
    sleep 5

    echo ""
    echo "🎉 $SERVICE_NAME deployment complete!"
    echo ""

    # Show relevant endpoints based on service
    echo "🌐 Service endpoints:"
    if [[ "$SERVICE" == "api" ]] || [[ "$SERVICE" == "all" ]]; then
        # Check if TLS is configured
        TLS_CONFIGURED="no"
        if [ -f "./config/certs/cert.pem" ] && [ -f "./config/certs/key.pem" ]; then
            TLS_CONFIGURED="yes"
        fi

        if [ "$TLS_CONFIGURED" = "yes" ]; then
            echo "  - REST API:        https://localhost:40061 (TLS enabled)"
            echo "  - gRPC API:        localhost:50061 (TLS enabled)"
        else
            echo "  - REST API:        http://localhost:40061"
            echo "  - gRPC API:        localhost:50061"
        fi
        echo "  - Dynamic Logging: localhost:60061"
    fi

    if [[ "$SERVICE" == "qdrant" ]] || [[ "$SERVICE" == "all" ]]; then
        echo "  - Qdrant API:      http://localhost:6333"
        echo "  - Qdrant Dashboard: http://localhost:6333/dashboard"
    fi

    echo ""

    # Show next steps based on service
    echo "📋 Next steps:"
    if [[ "$SERVICE" == "qdrant" ]]; then
        echo "  - Start API service: $0 api"
        echo "  - Import collections: ./scripts/import-collections.sh /path/to/snapshots/"
    elif [[ "$SERVICE" == "api" ]]; then
        echo "  - Import collections: ./scripts/import-collections.sh /path/to/snapshots/"
        echo "  - View logs: $0 $SERVICE logs"
        echo "  - Check status: $0 $SERVICE status"
    else
        echo "  - Import collections: ./scripts/import-collections.sh /path/to/snapshots/"
        echo "  - View logs: $0 all logs"
        echo "  - Check status: $0 all status"
    fi

    if [[ "$SERVICE" == "api" ]] || [[ "$SERVICE" == "all" ]]; then
        if [ "$TLS_CONFIGURED" = "no" ]; then
            echo ""
            echo "🔐 TLS Setup (optional):"
            echo "  - Run: ./scripts/setup-tls.sh /path/to/cert.crt /path/to/cert.key"
        fi
    fi
    ;;

"down")
    echo "🛑 Stopping $SERVICE_NAME..."
    docker-compose $COMPOSE_FILES down
    echo "✅ $SERVICE_NAME stopped"
    ;;

"logs")
    echo "📋 Viewing $SERVICE_NAME logs..."
    docker-compose $COMPOSE_FILES logs -f
    ;;

"status")
    echo "📊 $SERVICE_NAME status:"
    echo ""
    docker-compose $COMPOSE_FILES ps
    echo ""
    ;;

"restart")
    echo "🔄 Restarting $SERVICE_NAME..."
    docker-compose $COMPOSE_FILES restart
    echo "✅ $SERVICE_NAME restarted"
    ;;

"pull")
    echo "📥 Pulling latest images for $SERVICE_NAME..."
    docker-compose $COMPOSE_FILES pull
    echo "✅ Images updated"
    ;;

*)
    echo "❌ Invalid action: $ACTION"
    echo "Valid actions: up, down, logs, status, restart, pull"
    exit 1
    ;;
esac
