# Vector Database Design for Storm

## Overview

Storm needs to support semantic search over markdown discussion files using embeddings. This document outlines the persistence architecture combining in-memory approximate nearest neighbor (ANN) search with embedded key-value storage using content-addressed chunk identifiers.

## Architecture Components

### 1. Embedding Generation (Ollama)
- **Role**: Generates embedding vectors locally without external services
- **Integration**: HTTP API calls to `localhost:11434/api/embed`
- **Models**: `nomic-embed-text`, `mxbai-embed-large`
- **Output**: High-dimensional vectors (768-1024 dimensions)

### 2. Approximate Nearest Neighbor Search (HNSW)
- **Library**: `github.com/fogfish/hnsw` (pure Go)
- **Role**: In-memory index for fast similarity search
- **Data**: Stores embedding vectors and chunk references
- **Lifecycle**: Rebuilt on daemon startup from persisted embeddings
- **Scope**: Per-project indices (separate HNSW index for each project's embeddings)

### 3. Persistence Layer (BoltDB)
- **Role**: Embedded key-value store for metadata, embeddings, and file references
- **Data Buckets**:
  - `projects/`: Project metadata (ID, baseDir, markdownFile, authorizedFiles)
  - `embeddings/`: Raw embedding vectors keyed by chunkID (content CID)
  - `files/`: File inode-like structures mapping filepath → chunks with offsets
  - `hnsw_metadata/`: Versioning and index rebuild timestamps
  - `config/`: Storm daemon configuration settings
- **ACID Guarantees**: Transaction support prevents corruption
- **No C++ Dependencies**: Pure Go implementation

## Design Rationale

### Content-Addressed Chunks (ChunkID as CID)

**Why Hash-Based Keys**:
- **Deduplication**: Identical content blocks across multiple files share same CID and embedding
- **Integrity**: Chunk hash serves as tamper detection (CID mismatch = file modified)
- **Determinism**: Same chunk content always produces same CID regardless of source
- **Simplicity**: No need for sequential IDs or complex versioning

**Example**:
```
File A: "The capital of France is Paris"
File B: "The capital of France is Paris"

ChunkID = CID("The capital of France is Paris")

Both files reference the same CID; embedding computed once, reused for both.
```

### Unified Embeddings Bucket (No Separate FileReferences)

```
embeddings/{CID} → {
  "vector": <binary float32 embedding>,
  "metadata": { ... optional minimal metadata ... }
}

files/{filepath} → {
  "chunks": [
    { "CID": "offset": 0, "length": 512 },
    { "CID": "offset": 512, "length": 256 },
    ...
  ],
  "fileTimestamp": "2025-12-06T03:15:00Z"
}
```

**Advantages**:
- Single lookup per semantic search (embeddings bucket only)
- No queryID/roundID duplication per chunk
- File structure acts like filesystem inode (filepath → list of blocks)
- Supports file changes: update fileTimestamp, chunks remain with old CIDs until garbage collected

### Why Remove queryID and roundID from Storage?

- **Redundant**: Multiple chunks from same query/round share identical queryID/roundID
- **Space Waste**: Duplicated in every embedding entry
- **Versioning**: Query history belongs in projects metadata or separate audit log, not per-chunk
- **Alternative**: If ordering matters, store in projects bucket as `roundHistory`: `[{ roundID, queryID, timestamp, CIDs: [...] }]`

### Why Not Persist HNSW Graph?

- **Graph Serialization Overhead**: HNSW's hierarchical graph structure is expensive to serialize/deserialize
- **Rebuild Cost is Negligible**: Indexing 10k embeddings takes <1 second on modern hardware
- **Simplicity**: Avoids versioning and compatibility issues across daemon updates
- **Memory Efficiency**: In-memory index only loaded during daemon lifetime

### Why Store File References Instead of Text?

- **Storage Efficiency**: Eliminates text duplication; markdown file is authoritative source
- **Cache Validity**: File timestamp allows invalidating stale embeddings when files change
- **Content Verification**: CID hash enables integrity checking and deduplication
- **Precise Retrieval**: Offset and length allow exact chunk extraction from file

### Why Key by CID Instead of Filepath?

- **Reduces Duplication**: Identical chunks across projects/files stored once
- **Simplifies Garbage Collection**: Scan embeddings bucket, verify each CID exists in files bucket
- **Flexible Sharing**: Supports chunks shared across multiple files
- **Cleaner Queries**: Direct CID lookup is O(1), no filepath prefix scanning needed

## Implementation Strategy

### Phase 1: Basic Setup
```
On startup:
1. Open BoltDB instance at ~/.storm/data.db
2. Load project metadata from projects/ bucket
3. Rebuild HNSW indices per project:
   - For each project, iterate authorizedFiles
   - Scan files/ bucket for filepaths in authorizedFiles
   - Collect all CID referenced by those files
   - Load corresponding embeddings from embeddings/ bucket
   - Build HNSW index in memory
4. Ready for semantic search queries
```

### Phase 2: Embedding Storage
```
When processing chat round with output files:
1. Extract markdown chunks from output files
2. Call Ollama to generate embeddings for new chunks
3. Compute CID hash of chunk content
4. Store in BoltDB (single transaction):
   a. Check if CID exists in embeddings/
      - If yes: skip (embedding already computed)
      - If no: store in embeddings/{CID} → vector_bytes
   b. Update files/{filepath} to append new chunks:
      { CID, offset, length } to chunks[] array
5. Add embeddings to in-memory HNSW index for owning project
```

### Phase 3: Semantic Search
```
When querying with semantic context:
1. Generate embedding for query via Ollama
2. Search project-specific HNSW index for k nearest neighbors
   (returns list of CIDs)
3. For each CID:
   a. Find which files reference this CID by scanning files/ bucket
   b. Select authorized file from project's authorizedFiles
   c. Read chunk from file at specified offset/length
   d. Compute CID hash of read content, verify against expected CID
4. Return ranked results with content
```

### Phase 4: Garbage Collection
```
Background garbage collection (daily or on-demand):
1. Scan all embeddings/ bucket keys (all CIDs)
2. For each CID:
   a. Search files/ bucket for any reference to this CID
   b. Check if filepath containing chunk is in ANY project's authorizedFiles
   c. If not referenced by any authorized file:
      - Delete from embeddings/ bucket
      - Remove from files/ bucket entries
      - Log deletion
3. Optional: Rebuild files/ bucket entries with missing chunks removed
```

### Phase 5: Handling File Modifications

**When File Changes**:
1. Recompute chunks for modified file (may differ from before)
2. Generate new CIDs for new content blocks
3. Store new CIDs in embeddings/ bucket (if new content)
4. Update files/{filepath} with new chunk list
5. On next garbage collection: old CIDs no longer referenced by any file are deleted
6. Update projects/{projectID}/chunk_ordering if needed for audit trail

## Bucket Schema (BoltDB)

### projects/ bucket
```json
Key: projectID
Value: {
  "id": "project-1",
  "baseDir": "/path/to/project",
  "markdownFile": "chat.md",
  "authorizedFiles": ["data.csv", "output.json", "/absolute/path/to/file.md"],
  "createdAt": "2025-12-06T03:00:00Z",
  "embeddingCount": 256,
  "roundHistory": [
    {
      "roundID": "round-5",
      "queryID": "query-abc123",
      "timestamp": "2025-12-06T03:15:00Z",
      "CIDs": ["hash1", "hash2", "hash3"]
    }
  ]
}
```

### embeddings/ bucket
```json
Key: "{CID}" (CID hash of chunk content)
Value: {
  "vector": <binary float32 array (768*4 bytes for nomic-embed-text)>,
  "metadata": {
    "modelName": "nomic-embed-text",
    "generatedAt": "2025-12-06T03:15:00Z"
  }
}
```

**Key Characteristics**:
- CID is deterministic (same content → same CID everywhere)
- No queryID or roundID here (moved to projects.roundHistory)
- One embedding per unique content block (deduplication across files)

### files/ bucket
```json
Key: "{filepath}" (e.g., "/path/to/project/chat.md")
Value: {
  "filepath": "/path/to/project/chat.md",
  "fileTimestamp": "2025-12-06T03:15:00Z",
  "chunks": [
    {
      "CID": "QmXxxx...",
      "offset": 0,
      "length": 512
    },
    {
      "CID": "QmYyyy...",
      "offset": 512,
      "length": 256
    }
  ]
}
```

**Inode-Like Structure**:
- Filepath is the key (like inode number)
- Chunks array lists all content blocks with their location
- fileTimestamp detects external modifications
- Supports sparse files (gaps between chunks)

**On Retrieval**:
1. Look up files[filepath] to get chunks array
2. Find chunk with matching CID in array
3. Read file at offset for length bytes
4. Compute CID of read content, verify against stored CID
5. Return text if hash matches (file unchanged)
6. If hash mismatch: file was modified, embedding stale (skip or recompute)

**On File Deauthorization**:
1. Remove filepath from project's authorizedFiles
2. Do not immediately delete embeddings (may be referenced by other files/projects)
3. Next garbage collection pass will clean up orphaned CIDs

### hnsw_metadata/ bucket
```json
Key: "{projectID}:metadata"
Value: {
  "lastRebuild": "2025-12-06T03:00:00Z",
  "embeddingCount": 256,
  "hnswVersion": "v1",
  "dataFormatVersion": "1.0"
}
```

### config/ bucket
```json
Key: "config"
Value: {
  "port": 8080,
  "embeddingModel": "nomic-embed-text",
  "ollama": {
    "endpoint": "http://localhost:11434"
  },
  "hnsw": {
    "M": 16,
    "efConstruction": 200,
    "efSearch": 100
  }
}
```

## Merkle Tree Alternative (Not Recommended for Storm)

**Why NOT Use Merkle Trees for Chunk Ordering**:
- **Overhead**: Merkle trees require computing hashes for all nodes on insertion
- **Unnecessary Complexity**: Linear chunk ordering (offset/length) is sufficient
- **Scaling**: Round history stored in projects bucket is simpler than tree traversal
- **Use Case Mismatch**: Merkle trees excel for multi-writer consensus; Storm has single daemon
- **Recommendation**: Use simple array in `projects.roundHistory` tracking CIDs per round

If full audit trail needed:
```json
"roundHistory": [
  {
    "roundID": "round-5",
    "CIDs": ["hash1", "hash2"],
    "rootHash": "merkleRoot"  // Optional verification
  }
]
```

## Pros and Cons

### HNSW (fogfish/hnsw)

**Pros**:
- Pure Go implementation with no C++ dependencies
- Efficient approximate nearest neighbor search (sublinear complexity)
- Active maintenance and clear API
- Hierarchical structure enables logarithmic search traversal
- No external service required

**Cons**:
- Must rebuild index on daemon startup (acceptable cost)
- Memory usage proportional to dataset size (~1MB per 1000 embeddings)
- No persistence built-in (must manually serialize if needed)
- Requires careful parameter tuning (M, efConstruction, efSearch)

### BoltDB

**Pros**:
- Embedded, pure Go, no external process
- ACID transactions ensure consistency
- Fast key-value operations with range scanning
- Simple bucket hierarchy for organizational namespace
- Single file database (easy backup/distribution)
- Proven stability in production Go applications

**Cons**:
- Single writer limitation (acceptable for Storm's single daemon architecture)
- No built-in full-text search or vector indexing
- Entire dataset fits in mmap'd memory (scalability limited to RAM)
- No distributed transaction support (not needed for Storm)

## Project Registry Integration

### Store in Same BoltDB Instance

**Approach**: Use separate bucket `projects/` for project metadata
- **Key**: projectID
- **Value**: JSON-encoded project struct including roundHistory for query/chunk audit trail

**Advantages**:
- Single database file for all persistent state
- Atomic transactions across project and embedding updates
- Consistent backup strategy
- No coordination between multiple data stores

**Load on Startup**:
```go
func LoadProjectRegistry(db *bolt.DB) (map[string]*Project, error) {
    projects := make(map[string]*Project)
    err := db.View(func(tx *bolt.Tx) error {
        bucket := tx.Bucket([]byte("projects"))
        if bucket == nil {
            return nil // No projects yet
        }
        return bucket.ForEach(func(k, v []byte) error {
            var project Project
            json.Unmarshal(v, &project)
            projects[project.ID] = &project
            return nil
        })
    })
    return projects, err
}
```

## Migration Path

1. **Current State**: In-memory project registry, no embeddings
2. **Phase 1** (Week 1): Add BoltDB persistence for projects, load on startup
3. **Phase 2** (Week 2): Implement Ollama embedding generation and file storage with chunkID-based buckets
4. **Phase 3** (Week 3): Integrate fogfish/hnsw for semantic search over content-addressed embeddings
5. **Future** (if needed): Add graph database only if relationships become complex

## Performance Expectations

- **Startup**: Loading 1000 projects + 100k embeddings + HNSW rebuild ≈ 2-5 seconds
- **Semantic Search**: HNSW query against 100k embeddings ≈ 5-20ms (ef parameter tuned)
- **Embedding Generation**: Ollama for 512-token chunk ≈ 50-100ms per chunk
- **Chunk Retrieval**: Read from file at offset/length ≈ <1ms (OS page cache)
- **Database Operations**: BoltDB get/put ≈ <1ms (in-memory mmap overhead minimal)
- **Garbage Collection**: Scan 100k embeddings + verify files referencing them ≈ 500ms-2s

## Security Considerations

- **BoltDB Files**: Store at `~/.storm/data.db`, restrict file permissions (0600)
- **Markdown Files**: Ensure appropriate read permissions; validate paths against authorizedFiles
- **Embeddings**: Do not expose raw embeddings in API responses; only return semantic similarity scores
- **Project Isolation**: Embeddings filtered by project's authorizedFiles; cross-project queries not possible
- **Content Verification**: CID hashes detect tampering or accidental file modifications
- **Garbage Collection**: Verify filepath exists in authorizedFiles before deletion (prevent accidental cleanup)
- **Chunk Integrity**: Always compute CID on retrieved chunks; reject if hash mismatch indicates corruption

## Recommended Configuration

```go
// HNSW parameters for Storm
M := 16                  // max connections per node (trade-off: memory vs search quality)
efConstruction := 200    // ef during insertion (higher = better quality, slower insert)
efSearch := 100          // ef during search (higher = better recall, slower search)

// BoltDB tuning
FreelistType := "array"  // faster than map for typical usage
PageSize := 4096         // default, suitable for embeddings
BatchSize := 100         // embeddings to add per transaction
```

