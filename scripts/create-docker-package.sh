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
echo "  - Docker Compose configuration..."
cp docker-compose.yml "$package_dir/"
cp docker-compose.dev.yml "$package_dir/"
cp docker-compose.prod.yml "$package_dir/"

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

# Copy collection management scripts (they work great with Docker!)
cp scripts/create-collection-snapshots.sh "$package_dir/scripts/"
cp scripts/import-collections.sh "$package_dir/scripts/"

# Create snapshots directory
mkdir -p "$package_dir/snapshots"

# Create images directory for Docker images
mkdir -p "$package_dir/images"

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
        --output type=oci,dest="$package_dir/images/scanoss-hfh-api-${version}-multi.tar" \
        .

    echo "  - Multi-architecture image saved: scanoss-hfh-api-${version}-multi.tar"
else
    echo "  - Building $platform image..."

    # Build single-architecture image
    docker build --platform "$platform" \
        --build-arg VERSION="$version" \
        -t scanoss/hfh-api:$version \
        .

    # Save the image
    docker save scanoss/hfh-api:$version >"$package_dir/images/scanoss-hfh-api-${version}-${platform_name}.tar"
    echo "  - Image saved: scanoss-hfh-api-${version}-${platform_name}.tar"
fi

# Pull and save Qdrant image for offline deployment
echo "  - Pulling and saving Qdrant image..."
docker pull qdrant/qdrant:latest
docker save qdrant/qdrant:latest >"$package_dir/images/qdrant-latest.tar"
echo "  - Qdrant image saved: qdrant-latest.tar"

# Create image loading script
echo "  - Creating image loading script..."
cat >"$package_dir/scripts/load-images.sh" <<'EOF'
#!/bin/bash

##########################################
#
# Load Docker images for offline deployment
#
################################################################

set -e

echo "🐳 Loading SCANOSS HFH API Docker images..."
echo "========================================"

IMAGES_DIR="$(dirname "$0")/../images"

if [ ! -d "$IMAGES_DIR" ]; then
    echo "❌ Images directory not found: $IMAGES_DIR"
    exit 1
fi

# Load all tar files in images directory
for image_file in "$IMAGES_DIR"/*.tar; do
    if [ -f "$image_file" ]; then
        echo "📥 Loading $(basename "$image_file")..."
        if docker load < "$image_file"; then
            echo "✅ Successfully loaded $(basename "$image_file")"
        else
            echo "❌ Failed to load $(basename "$image_file")"
            exit 1
        fi
    fi
done

echo ""
echo "🎉 All Docker images loaded successfully!"
echo ""
echo "📋 Loaded images:"
docker images --filter reference="scanoss/*" --filter reference="qdrant/*" --format "table {{.Repository}}\t{{.Tag}}\t{{.Size}}"
echo ""
echo "Next steps:"
echo "  1. Configure: cp config/app-config.example.json config/app-config.json"
echo "  2. Deploy: ./scripts/deploy.sh"
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
# Start production environment
./scripts/deploy.sh prod

# Or start development environment
./scripts/deploy.sh dev
\`\`\`

### 5. Import Knowledge Base (Optional)
\`\`\`bash
# Import collection snapshots if you have them
./scripts/import-collections.sh /path/to/collection-snapshots/
\`\`\`

### 6. Verify Deployment
\`\`\`bash
# Check service status
./scripts/deploy.sh prod status

# Test API endpoint
curl http://localhost:40061/health
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
./scripts/deploy.sh prod up

# Stop services
./scripts/deploy.sh prod down

# View logs
./scripts/deploy.sh prod logs

# Check status
./scripts/deploy.sh prod status

# Restart services
./scripts/deploy.sh prod restart
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
./scripts/deploy.sh prod status
\`\`\`

### View Service Logs
\`\`\`bash
./scripts/deploy.sh prod logs
\`\`\`

### Check Docker Images
\`\`\`bash
docker images --filter reference="scanoss/*" --filter reference="qdrant/*"
\`\`\`

### Reset Environment
\`\`\`bash
# Stop services and remove containers
./scripts/deploy.sh prod down

# Remove Docker volumes (⚠️ This will delete data!)
docker volume prune

# Restart deployment
./scripts/deploy.sh prod up
\`\`\`

## Development Mode

For development with live code reloading:

\`\`\`bash
# Start development environment
./scripts/deploy.sh dev

# This enables debug logging and development features
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
cat >"$package_dir/scripts/verify-installation.sh" <<'EOF'
#!/bin/bash

echo "🔍 SCANOSS Installation Verification"
echo "===================================="

# Check if Docker is available
if ! command -v docker &>/dev/null; then
    echo "❌ Docker is not installed"
    exit 1
fi

# Check if services are running
echo "Checking Docker services..."
if docker-compose ps | grep -q "Up"; then
    echo "✅ Docker services are running"
else
    echo "❌ Docker services are not running"
    echo "💡 Start services with: ./scripts/deploy.sh prod"
    exit 1
fi

# Check Qdrant
echo "Checking Qdrant..."
if curl -f http://localhost:6333 >/dev/null 2>&1; then
    echo "✅ Qdrant is responding"
    
    # Check collections
    collections=$(curl -s http://localhost:6333/collections | grep -o '"name":"[^"]*"' | wc -l || echo "0")
    echo "📊 Collections: $collections"
else
    echo "❌ Qdrant is not responding"
    exit 1
fi

# Check HFH API
echo "Checking HFH API..."
if curl -f http://localhost:40061/health >/dev/null 2>&1; then
    echo "✅ HFH API is responding"
else
    echo "❌ HFH API is not responding"
    exit 1
fi

echo ""
echo "🎉 Installation verification successful!"
echo ""
echo "🌐 Service endpoints:"
echo "  - REST API:         http://localhost:40061"
echo "  - gRPC API:         localhost:50061"
echo "  - Qdrant Dashboard: http://localhost:6333/dashboard"
EOF

chmod +x "$package_dir/scripts/verify-installation.sh"

# Create package metadata
echo "  - Creating package metadata..."
cat >"$package_dir/package-info.json" <<EOF
{
  "name": "scanoss-hfh-api-docker",
  "version": "$version",
  "platform": "$platform",
  "build": $build,
  "created": "$(date -u +%Y-%m-%dT%H:%M:%SZ)",
  "package_type": "docker_containers",
  "components": {
    "hfh_api": "SCANOSS Folder Hashing API Docker Image",
    "qdrant": "Qdrant Vector Database Docker Image",
    "compose": "Docker Compose Configuration Files",
    "scripts": "Deployment and Management Scripts",
    "config_templates": "Configuration Templates"
  },
  "requirements": {
    "docker": "Docker Engine 20.10+",
    "docker_compose": "Docker Compose v2.0+",
    "ram": "4GB minimum, 8GB recommended",
    "disk": "20GB minimum",
    "platforms": "linux/amd64, linux/arm64"
  },
  "deployment_workflow": [
    "Extract package",
    "Load Docker images with load-images.sh",
    "Configure service using config templates",
    "Deploy with deploy.sh script",
    "Import data using import-collections.sh (optional)",
    "Verify with verify-installation.sh"
  ],
  "endpoints": {
    "rest_api": "http://localhost:40061",
    "grpc_api": "localhost:50061",
    "dynamic_logging": "localhost:60061",
    "qdrant_api": "http://localhost:6333",
    "qdrant_dashboard": "http://localhost:6333/dashboard"
  }
}
EOF

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
