# Qdrant Import Tool

This directory contains tools for importing data into Qdrant vector database, replacing the previous Milvus implementation.

## Key Changes from Milvus

1. **Single Intelligent Combined Vector**: Instead of storing three separate binary vectors (`hfhDirs`, `hfhNames`, `hfhContents`), we now create a single intelligent 64-bit hash that combines information from all three sources
2. **Snake Case Fields**: All field names are now in snake_case format
3. **Metadata Storage**: The original hash values and the combined hash are stored as metadata/payload for filtering and querying
4. **Simplified Architecture**: No partitions needed - category information is stored as metadata
5. **Optimized Performance**: 64-bit vector instead of 192-bit for better performance and storage efficiency

## Vector Combination Strategy

The three 64-bit hashes are intelligently combined into a single 64-bit hash using:

1. **Weighted Approach**: 
   - Content hash gets highest priority (most important for similarity)
   - File names get medium priority
   - Directory structure gets lower priority

2. **Bit Rotation and XOR**:
   - Each hash is rotated by different amounts (13, 23, 37 bits respectively)
   - All three rotated hashes are XORed together
   - Additional mixing ensures good bit distribution

3. **Final Vector**: The combined 64-bit hash is converted to a 64-dimensional binary vector where each bit becomes a float32 value (0.0 or 1.0)

## Prerequisites

1. **Add Qdrant Go Client Dependency**:
   ```bash
   go get github.com/qdrant/go-client
   ```

2. **Run Qdrant Server**:
   Use the provided docker-compose file:
   ```bash
   docker-compose -f pkg/usecase/qdrant/docker-compose.yml up -d
   ```

   Or run manually:
   ```bash
   docker run -p 6333:6333 -p 6334:6334 qdrant/qdrant
   ```

## Usage

```bash
# Build the import tool
go build -o qdrant-import pkg/usecase/qdrant/cmd/import_main/import_main_collection.go

# Run the import
./qdrant-import -dir /path/to/csv/files

# With custom collection name
./qdrant-import -dir /path/to/csv/files -collectionName my_collection

# Overwrite existing collection
./qdrant-import -dir /path/to/csv/files -overwrite
```

## Configuration

- **Qdrant Host**: `localhost` (default)
- **Qdrant Port**: `6334` (gRPC port, default)
- **Batch Size**: `1000` (optimized for Qdrant)
- **Vector Dimension**: `64` (single intelligent combined hash)
- **Distance Metric**: Cosine (suitable for simhash)

## Data Structure

### Vector
- Single 64-bit vector intelligently combining all three hash types
- Uses cosine distance for similarity search
- Optimized for performance and storage efficiency

### Payload (Metadata)
All fields are stored as payload with snake_case naming:

```json
{
  "hfh_dirs_hash": "string (hex)",
  "hfh_names_hash": "string (hex)", 
  "hfh_contents_hash": "string (hex)",
  "combined_hash": "string (hex)",
  "url_hash": "integer",
  "vendor": "string",
  "component": "string",
  "version": "string",
  "release_date": "string",
  "license": "string",
  "purl": "string",
  "url": "string",
  "total_files": "integer",
  "indexed_files": "integer",
  "source_files": "integer",
  "ignored_files": "integer",
  "size": "integer",
  "category": "string",
  "category_id": "integer"
}
```

## Performance Notes

- Batch size reduced to 1000 for optimal Qdrant performance
- Uses upsert operations for data consistency
- Parallel processing with configurable worker count
- Optimized collection settings for better search performance

## Querying Examples

After import, you can query the collection using vector similarity:

```python
# Example Python query (using qdrant-client)
from qdrant_client import QdrantClient

client = QdrantClient("localhost", port=6333)

# Create combined hash from your three hashes (same logic as Go code)
def create_combined_hash(dir_hash, name_hash, content_hash):
    # Rotate each hash by different amounts
    rotated_content = ((content_hash << 13) | (content_hash >> (64 - 13))) & 0xFFFFFFFFFFFFFFFF
    rotated_names = ((name_hash << 23) | (name_hash >> (64 - 23))) & 0xFFFFFFFFFFFFFFFF
    rotated_dirs = ((dir_hash << 37) | (dir_hash >> (64 - 37))) & 0xFFFFFFFFFFFFFFFF
    
    # XOR all three rotated hashes
    combined = rotated_content ^ rotated_names ^ rotated_dirs
    
    # Apply additional mixing
    combined ^= combined >> 16
    combined ^= combined << 13
    combined ^= combined >> 7
    combined &= 0xFFFFFFFFFFFFFFFF
    
    return combined

# Convert hash to 64-dimensional binary vector
def hash_to_vector(hash_value):
    return [1.0 if (hash_value >> i) & 1 else 0.0 for i in range(64)]

# Search for similar vectors
your_combined_hash = create_combined_hash(dir_hash, name_hash, content_hash)
query_vector = hash_to_vector(your_combined_hash)

results = client.search(
    collection_name="url_collection",
    query_vector=query_vector,  # Your 64-dimensional vector
    limit=10,
    with_payload=True
)
```

## Migration from Milvus

When migrating from the Milvus implementation:

1. Export data from Milvus if needed
2. Update your client code to use the new snake_case field names
3. Modify vector search logic to use the single combined vector
4. Update distance calculations to use cosine similarity
5. Replace partition-based filtering with payload filtering
