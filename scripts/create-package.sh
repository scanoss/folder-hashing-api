#!/bin/bash
###
# SPDX-License-Identifier: GPL-2.0-or-later
#
# Copyright (C) 2018-2023 SCANOSS.COM
#
# This program is free software: you can redistribute it and/or modify
# it under the terms of the GNU General Public License as published by
# the Free Software Foundation, either version 2 of the License, or
# (at your option) any later version.
# This program is distributed in the hope that it will be useful,
# but WITHOUT ANY WARRANTY; without even the implied warranty of
# MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
# GNU General Public License for more details.
# You should have received a copy of the GNU General Public License
# along with this program.  If not, see <https://www.gnu.org/licenses/>.
###
#
# Create a complete SCANOSS Folder Hashing API offline distribution package
#
if [ "$1" = "-h" ] || [ "$1" = "-help" ]; then
  echo "$0 [-help] <platform> [version]"
  echo "   Create a SCANOSS Folder Hashing API distribution package (binary + scripts only)"
  echo ""
  echo "Arguments:"
  echo "   <platform>          platform the package is destined for (linux_amd64, linux_arm64)"
  echo "   [version]           version number of the package (optional, auto-detected from git tags)"
  echo ""
  echo "Version Detection:"
  echo "   - If version not provided, uses latest git tag"
  echo "   - If no git tags found, defaults to '1.0.0-dev'"
  echo "   - If git not available, defaults to '1.0.0-dev'"
  echo ""
  echo "Package Contents:"
  echo "   scripts/            deployment and service scripts"
  echo "   dist/               compiled binary (built automatically)"
  echo "   config.example.json configuration template for customers"
  echo ""
  echo "Customer Workflow:"
  echo "   1. Extract package: tar -xzf package.tar.gz"
  echo "   2. Setup infrastructure: sudo ./scripts/env_setup.sh [env]"
  echo "   3. Configure service using config.example.json as template"
  echo "   4. Import data: ./scripts/import-collections.sh /path/to/collection-snapshots/"
  echo "   5. Start service: sudo systemctl start scanoss-hfh-api"
  echo ""
  echo "Examples:"
  echo "   $0 linux_amd64              # Uses git tag version"
  echo "   $0 linux_amd64 1.0.0        # Uses specified version"
  echo "   $0 linux_arm64              # Uses git tag version"
  exit 1
fi

if [ -z "$1" ]; then
  echo "ERROR: Please provide a package platform: linux_amd64 or linux_arm64"
  exit 1
fi

# Get version from git tag or use provided version
if [ -z "$2" ]; then
  # Try to get version from git tag
  if command -v git >/dev/null 2>&1 && git rev-parse --git-dir >/dev/null 2>&1; then
    version=$(git tag --sort=-version:refname | head -n 1)
    if [ -z "$version" ]; then
      version="1.0.0-dev"
      echo "⚠️  No git tags found, using default version: $version"
    else
      echo "📋 Using git tag version: $version"
    fi
  else
    version="1.0.0-dev"
    echo "⚠️  Git not available, using default version: $version"
  fi
else
  version="$2"
  echo "📋 Using provided version: $version"
fi

# Check scripts directory exists (always required)
if [ ! -d "scripts" ]; then
  echo "ERROR: Required directory 'scripts' does not exist."
  echo "This directory contains deployment scripts and must be present."
  exit 1
fi

export COPYFILE_DISABLE=true # Required if packaging on OSX
platform=$1
# version is already set above from git tag or provided parameter

# Build binary if it doesn't exist
BINARY_PATH="./dist/scanoss-hfh-api"
if [ ! -f "$BINARY_PATH" ]; then
  echo "🔨 Building binary for $platform..."
  mkdir -p ./dist
  if [ "$platform" = "linux_amd64" ]; then
    GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -ldflags="-w -s -X github.com/scanoss/folder-hashing-api/internal/domain/entities.AppVersion=$version" -o "$BINARY_PATH" ./cmd/server
  elif [ "$platform" = "linux_arm64" ]; then
    GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -ldflags="-w -s -X github.com/scanoss/folder-hashing-api/internal/domain/entities.AppVersion=$version" -o "$BINARY_PATH" ./cmd/server
  else
    echo "ERROR: Unsupported platform: $platform"
    exit 1
  fi

  if [ ! -f "$BINARY_PATH" ]; then
    echo "ERROR: Failed to build binary"
    exit 1
  fi
  echo "✅ Binary built successfully: $BINARY_PATH"
fi

# Verify config.example.json exists
if [ ! -f "config.example.json" ]; then
  echo "ERROR: config.example.json not found in current directory"
  echo "This file is required as a template for customers"
  exit 1
fi

build=1
prefix_name="scanoss-hfh-api"
package_name="${prefix_name}-${platform}-${version}"
tar_name="${package_name}-${build}.tar.gz"

# Get a unique archive name
while [ -f "$tar_name" ]; do
  ((build++))
  tar_name="${package_name}-${build}.tar.gz"
done

echo "🚀 Creating SCANOSS Folder Hashing API distribution package..."
echo "Platform: $platform"
echo "Version: $version"
echo "Binary: $BINARY_PATH"
echo "Package: $tar_name"
echo ""

# Create temporary package directory
temp_dir=$(mktemp -d)
package_dir="$temp_dir/$package_name"
mkdir -p "$package_dir"

# Copy all required components
echo "📁 Copying package components..."

echo "  - Scripts and service files..."
cp -r scripts/ "$package_dir/"

echo "  - Configuration templates..."
cp config.example.json "$package_dir/"
cp .env.example "$package_dir/"

echo "  - Docker configuration..."
cp docker-compose.yml "$package_dir/"

echo "  - Binary file..."
mkdir -p "$package_dir/dist"
cp "$BINARY_PATH" "$package_dir/dist/scanoss-hfh-api"

# Create package documentation
echo "  - Creating package documentation..."
cat >"$package_dir/README.md" <<EOF
# SCANOSS Folder Hashing API - Distribution Package

This package contains the SCANOSS Folder Hashing API binary and deployment scripts.

## Package Contents

- **scripts/**: Installation and management scripts
- **dist/**: Compiled API binary
- **config.example.json**: JSON configuration template
- **.env.example**: Environment variable configuration template
- **docker-compose.yml**: Qdrant container configuration

## System Requirements

- Linux x86_64 or ARM64
- Docker and Docker Compose
- Minimum 32GB RAM
- 100GB+ available disk space
- Root access for installation
- SCANOSS knowledge base snapshot file


Installation Workflow

Extract this package:
$()$(
  bash
  tar -xzf $tar_name
  cd $package_name
)$()
Create scanoss user:
$()$(
  bash
  sudo useradd --system scanoss
)$()
Setup infrastructure (no data import):
$()$(
  bash
  sudo ./scripts/env_setup.sh prod
  This will add scanoss to docker group for permission consistency
  IMPORTANT: Log out and back in for group changes to take effect
  Or run: newgrp docker
)$()
Configure the service (choose one method):
   \`\`\`

4. Configure the service (choose one method):

   **Method A: JSON Configuration (Default)**
   \`\`\`bash
   # Use the provided template
   sudo cp config.example.json /usr/local/etc/scanoss/hfh/app-config-prod.json
   sudo nano /usr/local/etc/scanoss/hfh/app-config-prod.json
   \`\`\`

   **Method B: Custom JSON Configuration**
   \`\`\`bash
   # Use your own JSON config file
   sudo cp your-config.json /usr/local/etc/scanoss/hfh/app-config-prod.json
   
   # Or specify a custom path via systemd override
   sudo systemctl edit scanoss-hfh-api
   # Add these lines:
   # [Service]
   # Environment=HFH_CONFIG_METHOD=json
   # Environment=HFH_CONFIG_PATH=/path/to/your/config.json
   \`\`\`

   **Method C: Environment File Configuration**
   \`\`\`bash
   # Create .env file from template
   sudo cp .env.example /usr/local/etc/scanoss/hfh/.env-prod
   sudo nano /usr/local/etc/scanoss/hfh/.env-prod
   
   # Configure systemd to use .env file
   sudo systemctl edit scanoss-hfh-api
   # Add these lines:
   # [Service]
   # Environment=HFH_CONFIG_METHOD=env
   \`\`\`

   **Method D: Custom Environment File**
   \`\`\`bash
   # Use your own .env file
   sudo systemctl edit scanoss-hfh-api
   # Add these lines:
   # [Service]
   # Environment=HFH_CONFIG_METHOD=env
   # Environment=HFH_CONFIG_PATH=/path/to/your/.env
   \`\`\`

   **Method E: Auto-Detection**
   \`\`\`bash
   # Let the system auto-detect available config files
   sudo systemctl edit scanoss-hfh-api
   # Add these lines:
   # [Service]
   # Environment=HFH_CONFIG_METHOD=auto
   \`\`\`

5. Import your knowledge base data:
   \`\`\`bash
   ./scripts/import-collections.sh /path/to/collection-snapshots/
   \`\`\`

6. Start the service:
   \`\`\`bash
   sudo systemctl start scanoss-hfh-api
   \`\`\`

7. Verify installation:
   \`\`\`bash
   curl http://localhost:40061
   \`\`\`

## Configuration Methods

The API supports multiple configuration methods with the following priority order:

1. **Environment variables** (highest priority)
2. **JSON config file** via \`--json-config\` flag
3. **.env file** via \`--env-config\` flag
4. **Default values** (lowest priority)

### Configuration Templates

- **config.example.json**: Complete JSON configuration template
- **.env.example**: Environment variable configuration template

### Advanced Configuration Examples

**Direct Binary Usage:**
\`\`\`bash
# Using JSON config
/usr/local/bin/scanoss-hfh-api --json-config /path/to/config.json

# Using .env file
/usr/local/bin/scanoss-hfh-api --env-config /path/to/.env

# Using environment variables only
APP_PORT=50061 REST_PORT=40061 /usr/local/bin/scanoss-hfh-api
\`\`\`

**Startup Script Usage:**
\`\`\`bash
# Default (JSON config for prod environment)
./scripts/scanoss-hfh-api.sh

# Custom environment
./scripts/scanoss-hfh-api.sh staging

# Custom config method
./scripts/scanoss-hfh-api.sh prod env

# Custom config file
./scripts/scanoss-hfh-api.sh prod json /custom/path/config.json
./scripts/scanoss-hfh-api.sh prod env /custom/path/.env
\`\`\`

## Package Information

- **Platform**: $platform
- **Version**: $version
- **Package Date**: $(date '+%Y-%m-%d %H:%M:%S')
- **Type**: Binary + Scripts (snapshot not included)

## Support

For API documentation and troubleshooting, visit: https://docs.scanoss.com
EOF

# Create installation verification script
cat >"$package_dir/verify-installation.sh" <<'EOF'
#!/bin/bash
echo "🔍 SCANOSS Installation Verification"
echo "===================================="

# Check if services are running
echo "Checking Qdrant..."
if curl -f http://localhost:6333 >/dev/null 2>&1; then
    echo "✅ Qdrant is running"
else
    echo "❌ Qdrant is not responding"
    exit 1
fi

echo "Checking HFH API..."
if curl -f http://localhost:40061 >/dev/null 2>&1; then
    echo "✅ HFH API is running"
else
    echo "❌ HFH API is not responding"
    exit 1
fi

# Check collections
echo "Checking knowledge base..."
collections=$(curl -s http://localhost:6333/collections | grep -o '"name":"[^"]*"' | wc -l)
if [ "$collections" -gt 0 ]; then
    echo "✅ Knowledge base loaded ($collections collections)"
else
    echo "❌ No collections found in knowledge base"
    exit 1
fi

echo ""
echo "🎉 Installation verification successful!"
echo "🌐 API endpoint: http://localhost:40061"
echo "🌐 GRPC endpoint: http://localhost:50061"
echo "🔍 Qdrant dashboard: http://localhost:6333/dashboard"
EOF

chmod +x "$package_dir/verify-installation.sh"

# Create package metadata
cat >"$package_dir/package-info.json" <<EOF
{
  "name": "scanoss-hfh-api",
  "version": "$version",
  "platform": "$platform",
  "build": $build,
  "created": "$(date -u +%Y-%m-%dT%H:%M:%SZ)",
  "package_type": "binary_and_scripts",
  "components": {
    "api": "SCANOSS Folder Hashing API Binary",
    "scripts": "Deployment and Service Scripts",
    "config_template": "Configuration Example"
  },
  "requirements": {
    "ram": "32GB minimum",
    "disk": "90GB minimum", 
    "docker": "required",
    "docker_compose": "required",
    "snapshot_file": "Customer must provide SCANOSS knowledge base snapshot"
  },
  "customer_workflow": [
    "Extract package",
    "Run env_setup.sh for infrastructure setup",
    "Configure service using config.example.json",
    "Import data using import-collections.sh",
    "Start systemd service"
  ]
}
EOF

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
cd "$OLDPWD" # Change back to original directory
rm -rf "$temp_dir"

# Calculate package size (now in correct directory)
if [ -f "$tar_name" ]; then
  package_size=$(du -h "$tar_name" | cut -f1)
else
  echo "ERROR: Package file $tar_name not found after creation"
  exit 1
fi

echo ""
echo "✅ Package creation successful!"
echo ""
echo "📦 Package: $tar_name"
echo "📏 Size: $package_size"
echo "🏗️  Build: $build"
echo ""
echo "🚀 Ready for distribution to offline customers!"
echo ""
echo "Customer installation commands:"
echo "  tar -xzf $tar_name && cd $package_name"
echo "  sudo ./scripts/env_setup.sh prod"
echo "  ./scripts/import-collections.sh /path/to/collection-snapshots/"
echo ""

exit 0
