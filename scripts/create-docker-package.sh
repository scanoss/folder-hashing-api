#!/bin/bash

##########################################
#
# SCANOSS Folder Hashing API - Docker Package Creation Script
#
# This script creates Docker-based distribution packages for offline deployment
# Replaces the legacy systemd-based packaging approach
#
################################################################

set -e

if [ "$1" = "-h" ] || [ "$1" = "-help" ]; then
    echo "$0 [-help] <platform> [version]"
    echo "   Create a SCANOSS Folder Hashing API Docker distribution package"
    echo ""
    echo "Arguments:"
    echo "   <platform>          Target platform: linux/amd64, linux/arm64, or multi"
    echo "   [version]           Version number (optional, auto-detected from git tags)"
    echo ""
    echo "Version Detection:"
    echo "   - If version not provided, uses latest git tag"
    echo "   - If no git tags found, defaults to 'dev'"
    echo "   - If git not available, defaults to 'dev'"
    echo ""
    echo "Package Contents:"
    echo "   - Docker Compose files for deployment"
    echo "   - Docker images (saved as tar files)"
    echo "   - Configuration templates"
    echo "   - Collection management scripts"
    echo "   - Deployment and management scripts"
    echo ""
    echo "Customer Workflow:"
    echo "   1. Extract package: tar -xzf package.tar.gz"
    echo "   2. Load images: ./scripts/load-images.sh"
    echo "   3. Configure: cp config/app-config.example.json config/app-config.json"
    echo "   4. Deploy: ./scripts/deploy.sh"
    echo "   5. Import data: ./scripts/import-collections.sh /path/to/snapshots/"
    echo ""
    echo "Examples:"
    echo "   $0 linux/amd64           # AMD64 package with git tag version"
    echo "   $0 linux/arm64 1.2.3     # ARM64 package with specified version"
    echo "   $0 multi                 # Multi-architecture package"
    exit 1
fi

if [ -z "$1" ]; then
    echo "ERROR: Please provide a target platform: linux/amd64, linux/arm64, or multi"
    exit 1
fi

# Get version from git tag or use provided version
if [ -z "$2" ]; then
    # Try to get version from git tag
    if command -v git >/dev/null 2>&1 && git rev-parse --git-dir >/dev/null 2>&1; then
        version=$(git tag --sort=-version:refname | head -n 1)
        if [ -z "$version" ]; then
            version="dev"
            echo "⚠️  No git tags found, using default version: $version"
        else
            echo "📋 Using git tag version: $version"
        fi
    else
        version="dev"
        echo "⚠️  Git not available, using default version: $version"
    fi
else
    version="$2"
    echo "📋 Using provided version: $version"
fi

export COPYFILE_DISABLE=true # Required if packaging on OSX
platform=$1

# Validate platform
case "$platform" in
"linux/amd64" | "amd64")
    platform="linux/amd64"
    platform_name="amd64"
    ;;
"linux/arm64" | "arm64")
    platform="linux/arm64"
    platform_name="arm64"
    ;;
"multi")
    platform="multi"
    platform_name="multi"
    ;;
*)
    echo "ERROR: Unsupported platform: $platform"
    echo "Supported platforms: linux/amd64, linux/arm64, multi"
    exit 1
    ;;
esac

build=1
prefix_name="scanoss-hfh-api-docker"
package_name="${prefix_name}-${platform_name}-${version}"
tar_name="${package_name}-${build}.tar.gz"

# Get a unique archive name
while [ -f "$tar_name" ]; do
    ((build++))
    tar_name="${package_name}-${build}.tar.gz"
done

echo "🐳 Creating SCANOSS Folder Hashing API Docker distribution package..."
echo "Platform: $platform"
echo "Version: $version"
echo "Package: $tar_name"
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

echo "✅ Prerequisites check passed"

# Create temporary package directory
temp_dir=$(mktemp -d)
package_dir="$temp_dir/$package_name"
mkdir -p "$package_dir"

echo "📁 Creating package structure..."

# Copy Docker Compose files
echo "  - Docker Compose files..."
cp docker-compose.qdrant.yml "$package_dir/"

# Create config directory and copy templates
echo "  - Configuration templates..."
mkdir -p "$package_dir/config"
cp config.example.json "$package_dir/config/app-config.example.json"

if [ -f ".env.example" ]; then
    cp .env.example "$package_dir/config/.env.example"
fi

# Create scripts directory and copy relevant scripts
echo "  - Deployment and management scripts..."
mkdir -p "$package_dir/scripts"

# Copy Docker deployment script
cp scripts/docker-deploy.sh "$package_dir/scripts/deploy.sh"
chmod +x "$package_dir/scripts/deploy.sh"

# Copy collection management scripts (they work great with Docker!)
cp scripts/create-collection-snapshots.sh "$package_dir/scripts/"
cp scripts/import-collections.sh "$package_dir/scripts/"

# Copy TLS setup script
cp scripts/setup-tls.sh "$package_dir/scripts/"

# Create images directory for Docker images
mkdir -p "$package_dir/images"

# Helper function to verify image was built correctly
verify_built_image() {
    local image_name="$1"
    if ! docker image inspect "$image_name" >/dev/null 2>&1; then
        echo "❌ Failed to build image: $image_name"
        exit 1
    fi
    echo "✅ Image built successfully: $image_name"
}

# Helper function to generate deployment-specific Docker Compose files
generate_deployment_compose_files() {
    local version="$1"
    local package_dir="$2"

    echo "  - Generating deployment-specific Docker Compose files..."

    # Process docker-compose.yml - replace build directive with image reference
    sed "s|build: \.|image: scanoss/hfh-api:${version}|g" \
        docker-compose.yml >"${package_dir}/docker-compose.yml"

    echo "  - Docker Compose files configured for pre-built images"
}

# Build and save Docker images
echo "🔨 Building Docker images..."

if [ "$platform" = "multi" ]; then
    echo "  - Building multi-architecture images..."

    # Setup buildx if not available
    if ! docker buildx inspect multiarch-builder >/dev/null 2>&1; then
        echo "  - Setting up Docker buildx for multi-architecture builds..."
        docker buildx create --name multiarch-builder --use --bootstrap
    else
        docker buildx use multiarch-builder
    fi

    # Build and save multi-arch images
    docker buildx build --platform linux/amd64,linux/arm64 \
        --build-arg VERSION="$version" \
        -t scanoss/hfh-api:$version \
        --output type=oci,dest="$package_dir/images/scanoss-hfh-api-${version}.tar" \
        .

    echo "  - Multi-architecture image saved: scanoss-hfh-api-${version}.tar"
else
    echo "  - Building $platform image..."

    # Build single-architecture image with consistent naming
    docker build --platform "$platform" \
        --build-arg VERSION="$version" \
        -t scanoss/hfh-api:$version \
        .

    # Verify the image was built correctly
    verify_built_image "scanoss/hfh-api:$version"

    # Save the image with consistent naming
    docker save scanoss/hfh-api:$version >"$package_dir/images/scanoss-hfh-api-${version}.tar"
    echo "  - Image saved: scanoss-hfh-api-${version}.tar"
fi

# Pull and save Qdrant image for offline deployment
echo "  - Pulling and saving Qdrant image..."
docker pull qdrant/qdrant:latest
docker save qdrant/qdrant:latest >"$package_dir/images/qdrant-latest.tar"
echo "  - Qdrant image saved: qdrant-latest.tar"

# Generate deployment-specific Docker Compose files
generate_deployment_compose_files "$version" "$package_dir"

# Create image loading script
echo "  - Creating image loading script..."
cat >"$package_dir/scripts/load-images.sh" <<EOF
#!/bin/bash

##########################################
#
# Load Docker images for offline deployment
#
################################################################

set -e

echo "🐳 Loading SCANOSS HFH API Docker images..."
echo "========================================"

IMAGES_DIR="\$(dirname "\$0")/../images"

# Helper function to check Docker daemon
check_docker_daemon() {
    if ! command -v docker &>/dev/null; then
        echo "❌ Docker is not installed"
        echo "Please install Docker: https://docs.docker.com/engine/install/"
        exit 1
    fi

    if ! docker info &>/dev/null; then
        echo "❌ Docker daemon is not running"
        echo "Please start Docker service"
        exit 1
    fi
    echo "✅ Docker daemon is running"
}

# Helper function to verify tar files exist
verify_tar_files_exist() {
    if [ ! -d "\$IMAGES_DIR" ]; then
        echo "❌ Images directory not found: \$IMAGES_DIR"
        exit 1
    fi

    local tar_count=\$(find "\$IMAGES_DIR" -name "*.tar" | wc -l)
    if [ "\$tar_count" -eq 0 ]; then
        echo "❌ No tar files found in \$IMAGES_DIR"
        exit 1
    fi
    echo "✅ Found \$tar_count image files to load"
}

# Helper function to verify image loaded correctly
verify_image_loaded() {
    local expected_image="\$1"
    if ! docker image inspect "\$expected_image" >/dev/null 2>&1; then
        echo "❌ Failed to load image: \$expected_image"
        return 1
    fi
    echo "✅ Image loaded: \$expected_image"
    return 0
}

# Helper function to check available disk space
check_available_disk_space() {
    local required_gb=10
    local available_gb=\$(df . | awk 'NR==2 {printf "%.0f", \$4/1024/1024}')
    
    if [ "\$available_gb" -lt "\$required_gb" ]; then
        echo "⚠️  Warning: Low disk space. Available: \${available_gb}GB, Recommended: \${required_gb}GB+"
    else
        echo "✅ Sufficient disk space available: \${available_gb}GB"
    fi
}

# Pre-flight checks
echo "🔍 Running pre-flight checks..."
check_docker_daemon
verify_tar_files_exist
check_available_disk_space

echo ""
echo "📥 Loading Docker images..."

# Expected images for verification
EXPECTED_IMAGES=("scanoss/hfh-api:$version" "qdrant/qdrant:latest")
LOADED_IMAGES=()

# Load all tar files in images directory
for image_file in "\$IMAGES_DIR"/*.tar; do
    if [ -f "\$image_file" ]; then
        echo "📥 Loading \$(basename "\$image_file")..."
        if docker load < "\$image_file"; then
            echo "✅ Successfully loaded \$(basename "\$image_file")"
        else
            echo "❌ Failed to load \$(basename "\$image_file")"
            exit 1
        fi
    fi
done

echo ""
echo "🔍 Verifying expected images are available..."

# Verify expected images were loaded
verification_failed=false
for expected_image in "\${EXPECTED_IMAGES[@]}"; do
    if verify_image_loaded "\$expected_image"; then
        LOADED_IMAGES+=("\$expected_image")
    else
        verification_failed=true
    fi
done

if [ "\$verification_failed" = true ]; then
    echo ""
    echo "❌ Some expected images failed to load properly"
    echo "💡 Check the tar files and try again"
    exit 1
fi

echo ""
echo "🎉 All Docker images loaded successfully!"
echo ""
echo "📋 Loaded images:"
docker images --filter reference="scanoss/*" --filter reference="qdrant/*" --format "table {{.Repository}}\t{{.Tag}}\t{{.Size}}"
echo ""
echo "📊 Summary:"
echo "  - Images loaded: \${#LOADED_IMAGES[@]}"
echo "  - HFH API version: $version"
echo "  - Qdrant version: latest"
echo ""
echo "✅ Images are ready for deployment!"
echo ""
echo "📋 Next steps:"
echo "  1. Configure: cp config/app-config.example.json config/app-config.json"
echo "  2. Edit config: nano config/app-config.json (or your preferred editor)"
echo "  3. Deploy: ./scripts/deploy.sh prod"
echo "  4. Verify: ./scripts/verify-installation.sh"
EOF

chmod +x "$package_dir/scripts/load-images.sh"

# Create package documentation
echo "  - Creating package documentation..."
cat >"$package_dir/README.md" <<EOF
# SCANOSS Folder Hashing API - Docker Distribution Package

This package contains the SCANOSS Folder Hashing API and all dependencies for Docker-based deployment.

## Package Information

- **Version**: $version
- **Platform**: $platform
- **Package Date**: $(date '+%Y-%m-%d %H:%M:%S')
- **Type**: Docker containers + deployment scripts

## System Requirements

- **Docker**: Docker Engine 20.10+ and Docker Compose v2.0+
- **System**: Linux x86_64 or ARM64 (depending on package)
- **Memory**: Minimum 4GB RAM, recommended 8GB+
- **Storage**: 20GB+ available disk space
- **Network**: Internet access for initial setup (optional for offline deployment)

## Quick Start

### 1. Extract Package
\`\`\`bash
tar -xzf $tar_name
cd $package_name
\`\`\`

### 2. Load Docker Images
\`\`\`bash
./scripts/load-images.sh
\`\`\`

### 3. Configure Service
\`\`\`bash
# Copy and customize configuration
cp config/app-config.example.json config/app-config.json
# Edit config/app-config.json as needed
\`\`\`

### 4. Deploy Services
\`\`\`bash
# Start Qdrant first (recommended)
./scripts/deploy.sh qdrant

# Then start the API service
./scripts/deploy.sh api

# Or start both services together
./scripts/deploy.sh all
\`\`\`

### 5. Import Knowledge Base (Optional)
\`\`\`bash
# Import collection snapshots if you have them
./scripts/import-collections.sh /path/to/collection-snapshots/
\`\`\`

### 6. Verify Deployment
\`\`\`bash
# Check service status
./scripts/deploy.sh all status

# Test API endpoint
curl -X POST -H "Content-Type: application/json" -d '{"message":"test"}' http://localhost:40061/api/v2/scanning/echo
\`\`\`

## Service Endpoints

After successful deployment:

- **REST API**: http://localhost:40061
- **gRPC API**: localhost:50061
- **Dynamic Logging**: localhost:60061
- **Qdrant API**: http://localhost:6333
- **Qdrant Dashboard**: http://localhost:6333/dashboard

## Configuration Options

### JSON Configuration (Default)
Edit \`config/app-config.json\` with your settings:

\`\`\`json
{
  "App": {
    "Name": "SCANOSS HFH Server",
    "GRPCPort": "50061",
    "RESTPort": "40061",
    "Debug": false,
    "Mode": "production"
  },
  "Hfh": {
    "QdrantHost": "qdrant",
    "QdrantPort": 6334
  }
}
\`\`\`

### Environment Variables
You can also use environment variables by setting them in docker-compose files or using the development mode.

## Management Commands

\`\`\`bash
# Start services
./scripts/deploy.sh qdrant up   # Start Qdrant
./scripts/deploy.sh api up       # Start API
./scripts/deploy.sh all up       # Start both

# Stop services
./scripts/deploy.sh all down     # Stop all services
./scripts/deploy.sh api down     # Stop API only
./scripts/deploy.sh qdrant down  # Stop Qdrant only

# View logs
./scripts/deploy.sh all logs

# Check status
./scripts/deploy.sh all status

# Restart services
./scripts/deploy.sh all restart
\`\`\`

## Collection Management

### Export Collections
\`\`\`bash
# Create snapshots of your collections
./scripts/create-collection-snapshots.sh snapshots/
\`\`\`

### Import Collections
\`\`\`bash
# Import snapshots into Qdrant
./scripts/import-collections.sh snapshots/
\`\`\`

## Troubleshooting

### Check Service Health
\`\`\`bash
./scripts/deploy.sh all status
\`\`\`

### View Service Logs
\`\`\`bash
./scripts/deploy.sh all logs
\`\`\`

### Check Docker Images
\`\`\`bash
docker images --filter reference="scanoss/*" --filter reference="qdrant/*"
\`\`\`

### Reset Environment
\`\`\`bash
# Stop services and remove containers
./scripts/deploy.sh all down

# Remove Docker volumes (⚠️ This will delete data!)
docker volume prune

# Restart deployment
./scripts/deploy.sh qdrant up
./scripts/deploy.sh api up
\`\`\`

## Service Deployment Options

### Independent Deployment
You can deploy Qdrant and the API service independently:

\`\`\`bash
# Deploy Qdrant on one machine
./scripts/deploy.sh qdrant

# Deploy API on another machine (configure Qdrant host in config/app-config.json)
./scripts/deploy.sh api
\`\`\`

### Remote Qdrant
To connect to a remote Qdrant instance, edit \`config/app-config.json\`:

\`\`\`json
{
  "Hfh": {
    "QdrantHost": "remote-qdrant-host",
    "QdrantPort": 6334
  }
}
\`\`\`

## Production Considerations

- **Resource Limits**: Production mode includes memory and CPU limits
- **Restart Policy**: Services automatically restart on failure
- **Logging**: Structured logging with appropriate levels
- **Security**: Non-root containers with minimal attack surface

## Support

- **Documentation**: See docker-compose files for detailed configuration
- **Logs**: All service logs are accessible via Docker commands
- **API Documentation**: Visit REST API endpoints for OpenAPI documentation

---

**Package**: $tar_name  
**Built**: $(date '+%Y-%m-%d %H:%M:%S')  
**Platform**: $platform  
**Version**: $version
EOF

# Create installation verification script
echo "  - Creating verification script..."
cat >"$package_dir/scripts/verify-installation.sh" <<EOF
#!/bin/bash

echo "🔍 SCANOSS Installation Verification"
echo "===================================="

# Helper function to check prerequisites
check_prerequisites() {
    echo "🔧 Checking prerequisites..."
    
    # Check if Docker is available
    if ! command -v docker &>/dev/null; then
        echo "❌ Docker is not installed"
        echo "Please install Docker: https://docs.docker.com/engine/install/"
        exit 1
    fi

    # Check if Docker daemon is running
    if ! docker info &>/dev/null; then
        echo "❌ Docker daemon is not running"
        echo "Please start Docker service"
        exit 1
    fi

    # Check if Docker Compose is available
    if ! command -v docker-compose &>/dev/null; then
        echo "❌ Docker Compose is not installed"
        echo "Please install Docker Compose"
        exit 1
    fi

    echo "✅ All prerequisites are available"
}

# Helper function to check required images
check_required_images() {
    echo "🐳 Checking required Docker images..."
    
    local required_images=("scanoss/hfh-api:$version" "qdrant/qdrant:latest")
    local missing_images=()
    
    for image in "\${required_images[@]}"; do
        if docker image inspect "\$image" >/dev/null 2>&1; then
            echo "✅ Image available: \$image"
        else
            echo "❌ Image missing: \$image"
            missing_images+=("\$image")
        fi
    done
    
    if [ "\${#missing_images[@]}" -gt 0 ]; then
        echo ""
        echo "❌ Missing required images. Load them first:"
        echo "💡 Run: ./scripts/load-images.sh"
        exit 1
    fi
    
    echo "✅ All required images are available"
}

# Helper function to check if services are running
check_services_running() {
    echo "🚀 Checking Docker services..."
    
    if docker-compose ps | grep -q "Up"; then
        echo "✅ Docker services are running"
        
        # Show service status
        echo ""
        echo "📊 Service status:"
        docker-compose ps --format "table {{.Service}}\t{{.State}}\t{{.Ports}}"
    else
        echo "❌ Docker services are not running"
        echo "💡 Start services with: ./scripts/deploy.sh prod"
        exit 1
    fi
}

# Helper function to check service health
check_service_health() {
    echo "🏥 Checking service health..."
    
    # Check Qdrant
    echo "Checking Qdrant..."
    max_attempts=30
    attempt=0
    
    while [ \$attempt -lt \$max_attempts ]; do
        if curl -f http://localhost:6333 >/dev/null 2>&1; then
            echo "✅ Qdrant is responding"
            
            # Check collections
            collections=\$(curl -s http://localhost:6333/collections | grep -o '"name":"[^"]*"' | wc -l 2>/dev/null || echo "0")
            echo "📊 Collections: \$collections"
            break
        else
            attempt=\$((attempt + 1))
            if [ \$attempt -lt \$max_attempts ]; then
                echo "⏳ Waiting for Qdrant... (attempt \$attempt/\$max_attempts)"
                sleep 2
            else
                echo "❌ Qdrant is not responding after \$max_attempts attempts"
                echo "💡 Check logs: docker-compose logs qdrant"
                exit 1
            fi
        fi
    done

    # Check HFH API
    echo "Checking HFH API..."
    attempt=0
    
    while [ \$attempt -lt \$max_attempts ]; do
        if curl -f -X POST -H "Content-Type: application/json" -d '{"message":"health-check"}' http://localhost:40061/api/v2/scanning/echo >/dev/null 2>&1; then
            echo "✅ HFH API is responding"

            # Get API version info
            api_info=\$(curl -s -X POST -H "Content-Type: application/json" -d '{"message":"health-check"}' http://localhost:40061/api/v2/scanning/echo 2>/dev/null || echo "{}")
            echo "📋 API Health: OK"
            break
        else
            attempt=\$((attempt + 1))
            if [ \$attempt -lt \$max_attempts ]; then
                echo "⏳ Waiting for HFH API... (attempt \$attempt/\$max_attempts)"
                sleep 2
            else
                echo "❌ HFH API is not responding after \$max_attempts attempts"
                echo "💡 Check logs: docker-compose logs hfh-api"
                exit 1
            fi
        fi
    done
}

# Main verification flow
main() {
    check_prerequisites
    echo ""
    
    check_required_images
    echo ""
    
    check_services_running
    echo ""
    
    check_service_health
    echo ""
    
    echo "🎉 Installation verification successful!"
    echo ""
    echo "🌐 Service endpoints:"
    echo "  - REST API:          http://localhost:40061"
    echo "  - REST API Echo:     http://localhost:40061/api/v2/scanning/echo"
    echo "  - gRPC API:          localhost:50061"
    echo "  - Dynamic Logging:   localhost:60061"
    echo "  - Qdrant API:        http://localhost:6333"
    echo "  - Qdrant Dashboard:  http://localhost:6333/dashboard"
    echo ""
    echo "📋 Quick tests:"
    echo "  curl -X POST -H 'Content-Type: application/json' -d '{\"message\":\"test\"}' http://localhost:40061/api/v2/scanning/echo"
    echo "  curl http://localhost:6333/collections"
    echo ""
    echo "🔧 Management commands:"
    echo "  ./scripts/deploy.sh prod status    # Check status"
    echo "  ./scripts/deploy.sh prod logs      # View logs"
    echo "  ./scripts/deploy.sh prod down      # Stop services"
    echo "  ./scripts/deploy.sh prod up        # Start services"
}

# Run main function
main
EOF

chmod +x "$package_dir/scripts/verify-installation.sh"

# Calculate package sizes
echo "📊 Calculating package sizes..."
total_size=$(du -sh "$package_dir" | cut -f1)
echo "  - Package directory size: $total_size"

# Create the final package
echo "📦 Creating compressed package..."
cd "$temp_dir"
if ! tar --format=ustar -czf "$(pwd)/../$tar_name" "$package_name"; then
    echo "ERROR: Failed to create package archive"
    rm -rf "$temp_dir"
    exit 1
fi

# Move package to current directory and cleanup
mv "../$tar_name" "$OLDPWD/"
cd "$OLDPWD"
rm -rf "$temp_dir"

# Calculate final package size
if [ -f "$tar_name" ]; then
    package_size=$(du -h "$tar_name" | cut -f1)
else
    echo "ERROR: Package file $tar_name not found after creation"
    exit 1
fi

echo ""
echo "✅ Docker package creation successful!"
echo ""
echo "📦 Package: $tar_name"
echo "📏 Size: $package_size"
echo "🏗️  Build: $build"
echo "🐳 Platform: $platform"
echo ""
echo "🚀 Ready for Docker-based deployment!"
echo ""
echo "📋 Customer deployment commands:"
echo "  tar -xzf $tar_name && cd $package_name"
echo "  ./scripts/load-images.sh"
echo "  cp config/app-config.example.json config/app-config.json"
echo "  ./scripts/deploy.sh prod"
echo ""

exit 0
