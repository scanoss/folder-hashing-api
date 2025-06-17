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
  echo "$0 [-help] <platform> <version> [options]"
  echo "   Create a complete SCANOSS Folder Hashing API offline distribution package"
  echo ""
  echo "Arguments:"
  echo "   <platform>          platform the package is destined for (linux_amd64, linux_arm64)"
  echo "   <version>           version number of the package"
  echo ""
  echo "Options (environment variables):"
  echo "   BINARY_PATH=path    path to scanoss-hfh-api binary (default: ./dist/scanoss-hfh-api)"
  echo "   SNAPSHOT_PATH=path  path to .snapshot file (default: auto-detect in ./snapshots/)"
  echo "   CONFIG_PATH=path    path to config file (default: ./config-templates/app-config-prod.json)"
  echo ""
  echo "Required:"
  echo "   scripts/            deployment and service scripts (always required)"
  echo ""
  echo "Examples:"
  echo "   $0 linux_amd64 1.0.0"
  echo "   BINARY_PATH=./dist/scanoss-hfh-api SNAPSHOT_PATH=./kb-2025-01-15.snapshot $0 linux_amd64 1.0.0"
  echo "   SNAPSHOT_PATH=/path/to/snapshots/latest.snapshot $0 linux_amd64 1.0.0"
  exit 1
fi

if [ -z "$1" ]; then
  echo "ERROR: Please provide a package platform: linux_amd64 or linux_arm64"
  exit 1
fi

if [ -z "$2" ]; then
  echo "ERROR: Please provide a package version"
  exit 1
fi

# Check scripts directory exists (always required)
if [ ! -d "scripts" ]; then
  echo "ERROR: Required directory 'scripts' does not exist."
  echo "This directory contains deployment scripts and must be present."
  exit 1
fi

export COPYFILE_DISABLE=true # Required if packaging on OSX
platform=$1
version=$2

# Set default paths (can be overridden by environment variables)
BINARY_PATH="${BINARY_PATH:-./dist/scanoss-hfh-api}"
CONFIG_PATH="${CONFIG_PATH:-./config.example.json}"
SNAPSHOT_PATH="${SNAPSHOT_PATH:-}"

# Auto-detect snapshot if not provided
if [ -z "$SNAPSHOT_PATH" ]; then
  if [ -d "snapshots" ]; then
    SNAPSHOT_PATH=$(find snapshots/ -name "*.snapshot" | head -1)
  fi
  if [ -z "$SNAPSHOT_PATH" ]; then
    echo "ERROR: No snapshot file specified and none found in snapshots/ directory"
    echo "Please set SNAPSHOT_PATH environment variable or place a .snapshot file in snapshots/"
    echo "Example: SNAPSHOT_PATH=/path/to/scanoss-kb-2025-01-15.snapshot $0 $platform $version"
    exit 1
  fi
  echo "Auto-detected snapshot: $SNAPSHOT_PATH"
fi

# Verify all required files exist
if [ ! -f "$BINARY_PATH" ]; then
  echo "ERROR: Binary file not found: $BINARY_PATH"
  echo "Please build the binary and set BINARY_PATH, or place it at the default location"
  echo "Example: BINARY_PATH=./dist/scanoss-hfh-api $0 $platform $version"
  exit 1
fi

if [ ! -f "$CONFIG_PATH" ]; then
  echo "ERROR: Configuration file not found: $CONFIG_PATH"
  echo "Please set CONFIG_PATH to point to a valid configuration file"
  echo "Example: CONFIG_PATH=./my-config.json $0 $platform $version"
  exit 1
fi

if [ ! -f "$SNAPSHOT_PATH" ]; then
  echo "ERROR: Snapshot file not found: $SNAPSHOT_PATH"
  echo "Please ensure the snapshot file exists or update SNAPSHOT_PATH"
  exit 1
fi

build=1
prefix_name="scanoss-hfh-offline"
package_name="${prefix_name}-${platform}-${version}"
tar_name="${package_name}-${build}.tar.gz"

# Get a unique archive name
while [ -f "$tar_name" ]; do
  ((build++))
  tar_name="${package_name}-${build}.tar.gz"
done

echo "🚀 Creating SCANOSS Folder Hashing API offline distribution package..."
echo "Platform: $platform"
echo "Version: $version"
echo "Binary: $BINARY_PATH"
echo "Config: $CONFIG_PATH"
echo "Snapshot: $(basename $SNAPSHOT_PATH)"
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

echo "  - Configuration file..."
mkdir -p "$package_dir/config"
cp "$CONFIG_PATH" "$package_dir/config/app-config-prod.json"

echo "  - Binary file..."
mkdir -p "$package_dir/dist"
cp "$BINARY_PATH" "$package_dir/dist/scanoss-hfh-api"

echo "  - Knowledge base snapshot..."
mkdir -p "$package_dir/snapshots"
cp "$SNAPSHOT_PATH" "$package_dir/snapshots/$(basename $SNAPSHOT_PATH)"

# Create package documentation
echo "  - Creating package documentation..."
cat >"$package_dir/README.md" <<EOF
# SCANOSS Folder Hashing API - Offline Distribution

This package contains the complete SCANOSS Folder Hashing API for offline deployment with pre-loaded knowledge base.

## Package Contents

- **scripts/**: Installation and management scripts
- **config/**: API configuration files  
- **dist/**: Compiled API binary
- **snapshots/**: Qdrant knowledge base snapshot

## System Requirements

- Linux x86_64 or ARM64
- Docker and Docker Compose
- Minimum 32GB RAM
- 100GB+ available disk space
- Root access for installation

## Quick Installation

1. Extract this package:
   \`\`\`bash
   tar -xzf $tar_name
   cd $package_name
   \`\`\`

2. Create scanoss user:
   \`\`\`bash
   useradd --system scanoss
   \`\`\`

3. Run installation:
   \`\`\`bash
   sudo ./scripts/env_setup.sh
   \`\`\`

4. Verify installation:
   \`\`\`bash
   curl http://localhost:40061/health
   \`\`\`

## Package Information

- **Platform**: $platform
- **Version**: $version
- **Knowledge Base**: $(basename $SNAPSHOT_PATH)
- **Package Date**: $(date '+%Y-%m-%d %H:%M:%S')

## Support

For detailed installation instructions, see scripts/readme.md

For API documentation and troubleshooting, visit: https://docs.scanoss.com
EOF

# Create installation verification script
cat >"$package_dir/verify-installation.sh" <<'EOF'
#!/bin/bash
echo "🔍 SCANOSS Installation Verification"
echo "===================================="

# Check if services are running
echo "Checking Qdrant..."
if curl -f http://localhost:6333/health >/dev/null 2>&1; then
    echo "✅ Qdrant is running"
else
    echo "❌ Qdrant is not responding"
    exit 1
fi

echo "Checking HFH API..."
if curl -f http://localhost:40061/health >/dev/null 2>&1; then
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
echo "🔍 Qdrant dashboard: http://localhost:6333/dashboard"
EOF

chmod +x "$package_dir/verify-installation.sh"

# Create package metadata
cat >"$package_dir/package-info.json" <<EOF
{
  "name": "scanoss-hfh-offline",
  "version": "$version",
  "platform": "$platform",
  "snapshot": "$(basename $SNAPSHOT_PATH)",
  "build": $build,
  "created": "$(date -u +%Y-%m-%dT%H:%M:%SZ)",
  "components": {
    "api": "SCANOSS Folder Hashing API",
    "database": "Qdrant Vector Database",
    "knowledge_base": "SCANOSS Component Fingerprints"
  },
  "requirements": {
    "ram": "32GB minimum",
    "disk": "100GB minimum", 
    "docker": "required",
    "docker_compose": "required"
  }
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
rm -rf "$temp_dir"

# Calculate package size
package_size=$(du -h "$tar_name" | cut -f1)

echo ""
echo "✅ Package creation successful!"
echo ""
echo "📦 Package: $tar_name"
echo "📏 Size: $package_size"
echo "🏗️  Build: $build"
echo ""
echo "🚀 Ready for distribution to offline customers!"
echo ""
echo "Customer installation command:"
echo "  tar -xzf $tar_name && cd $package_name && sudo ./scripts/env_setup.sh"
echo ""

exit 0
