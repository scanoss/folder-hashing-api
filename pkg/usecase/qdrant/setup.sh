#!/bin/bash

# Setup script for Qdrant import tool

echo "Setting up Qdrant import tool..."

# Add Qdrant Go client dependency
echo "Adding Qdrant Go client dependency..."
go get github.com/qdrant/go-client

# Start Qdrant server using docker-compose
echo "Starting Qdrant server..."
docker-compose -f pkg/usecase/qdrant/docker-compose.yml up -d

# Wait for Qdrant to be ready
echo "Waiting for Qdrant to be ready..."
sleep 10

# Check if Qdrant is running
echo "Checking Qdrant health..."
curl -f http://localhost:6333 || {
    echo "Warning: Qdrant may not be ready yet. Please check docker logs."
    echo "Run: docker-compose -f pkg/usecase/qdrant/docker-compose.yml logs"
}

echo "Setup complete!"
echo ""
echo "To build the import tool:"
echo "  go build -o qdrant-import pkg/usecase/qdrant/cmd/import_main/import_main_collection.go"
echo ""
echo "To run the import:"
echo "  ./qdrant-import -dir /path/to/csv/files"
echo ""
echo "To stop Qdrant:"
echo "  docker-compose -f pkg/usecase/qdrant/docker-compose.yml down"
