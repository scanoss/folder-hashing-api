#!/bin/bash

# Script to execute sequential steps with error checking
# If any step fails, the script will stop immediately

set -e  # This causes the script to terminate if any command fails

echo "=== STARTING IMPORT PROCESS ==="

# Step 1: Download and start Milvus standalone
echo "Step 1: Downloading and starting Milvus standalone..."
curl -sfL https://raw.githubusercontent.com/milvus-io/milvus/master/scripts/standalone_embed.sh -o standalone_embed.sh
bash standalone_embed.sh start

# Check if step 1 was successful
if [ $? -ne 0 ]; then
    echo "Error in Step 1: Could not start Milvus standalone"
    exit 1
fi
echo "Step 1 completed successfully"

# Step 2: Execute the main import
echo "Step 2: Executing main test collection import..."
go run ./cmd/import_main/import_main_collection.go -database test -dir ./test/source/main

# Check if step 2 was successful
if [ $? -ne 0 ]; then
    echo "Error in Step 2: Could not import the main collection"
    exit 1
fi
echo "Step 2 completed successfully"

# Step 3: Execute the secondary import
echo "Step 3: Executing secondary test collection import..."
go run ./cmd/import_secondary/import_secondary_collection.go -database test -overwrite -dir ./test/source/secondary/

# Check if step 3 was successful
if [ $? -ne 0 ]; then
    echo "Error in Step 3: Could not import the secondary collection"
    exit 1
fi
echo "Step 3 completed successfully"

echo "=== IMPORT PROCESS COMPLETED SUCCESSFULLY ==="