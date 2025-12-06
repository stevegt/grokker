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

### 3. Multi-Discussion File Support

Each project maintains multiple markdown discussion files, each with its own query-response history. Users switch between discussions via CLI or UI without data loss. Each discussion file lives in the `files/` bucket with identical structure as other project files, distinguished only by project metadata pointers.

**Key Design Points**:
- Discussion files use identical chunking and embedding as input/output files
- `currentDiscussionFile` pointer in projects metadata indicates active discussion
- `discussionFiles` array tracks all available discussions with metadata
- Rounds are filtered by `discussionFile` context in roundHistory
- Switching discussions requires only pointer update; content persists

### 4. Persistence Layer (BoltDB)
- **Role**: Embedded key-value store for metadata, embeddings, and file references
- **ACID Guarantees**: Transaction support prevents corruption
- **No C++ Dependencies**: Pure Go implementation

## Bucket Schema (BoltDB)

All persistent data in Storm is organized into BoltDB buckets. This section documents the schema for each bucket including key structure, value format, and lifecycle management.

### projects/ bucket

**Purpose**: Store project metadata including configuration, discussion files, and round history.

**Key**: `{projectID}` (string identifier)

**Value**:
```json
{
  "id": "project-1",
  "baseDir": "/path/to/project",
  "currentDiscussionFile": "chat.md",
  "discussionFiles": [
    {
      "filepath": "chat.md",
      "createdAt": "2025-12-05T10:00:00Z",
      "roundCount": 12
    },
    {
      "filepath": "chat-2.md",
      "createdAt": "2025-12-06T14:30:00Z",
      "roundCount": 3
    }
  ],
  "authorizedFiles": ["data.csv", "output.json", "/absolute/path/to/file.md"],
  "createdAt": "2025-12-06T03:00:00Z",
  "embeddingCount": 256,
  "roundHistory": [
    {
      "roundID": "round-5",
      "discussionFile": "chat.md",
      "queryID": "query-abc123",
      "timestamp": "2025-12-06T03:15:00Z",
      "CIDs": ["hash1", "hash2", "hash3"]
    },
    {
      "roundID": "round-6",
      "discussionFile": "chat-2.md",
      "queryID": "query-xyz789",
      "timestamp": "2025-12-06T04:00:00Z",
      "CIDs": ["hash4", "hash5"]
    }
  ]
}
```

**Key Changes from Original**:
- `markdownFile` → `currentDiscussionFile` (pointer to active discussion)
- New `discussionFiles` array tracking all available discussions
- `roundHistory` entries include `discussionFile` field for filtering

### files/ bucket

**Purpose**: Store file inode-like structures mapping filepath to its constituent chunks.

**Key**: `{filepath}` (full path to file, e.g., "/projects/project-1/chat.md")

**Value**:
```json
{
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

**Structure**:
- `filepath`: Full path for consistent lookups
- `fileTimestamp`: Modification time for cache invalidation
- `chunks`: Array of chunk references (order matters for file reconstruction)
  - `CID`: Content identifier hash of chunk
  - `offset`: Byte offset in original file
  - `length`: Number of bytes in chunk

**Important**: Discussion files and other files coexist with identical structure. No distinction in storage layer.

### embeddings/ bucket

**Purpose**: Store raw embedding vectors keyed by content hash.

**Key**: `{CID}` (content-addressed identifier, typically SHA256 hash)

**Value**:
```json
{
  "vector": <binary float32 array>,
  "metadata": {
    "modelName": "nomic-embed-text",
    "generatedAt": "2025-12-06T03:15:00Z"
  }
}
```

**Structure**:
- `vector`: Binary-encoded embedding (typically 768 or 1024 float32 values)
- `metadata.modelName`: Which Ollama model generated this embedding
- `metadata.generatedAt`: Timestamp for cache invalidation

**Key Property**: CID-based keys enable deduplication—identical chunks across multiple files share the same embedding vector.

### hnsw_metadata/ bucket

**Purpose**: Store versioning and rebuild metadata for HNSW indices.

**Key**: `{projectID}:metadata` (project-specific metadata)

**Value**:
```json
{
  "lastRebuild": "2025-12-06T03:00:00Z",
  "embeddingCount": 256,
  "hnswVersion": "v1",
  "dataFormatVersion": "1.0"
}
```

**Structure**:
- `lastRebuild`: Timestamp of last HNSW index rebuild
- `embeddingCount`: Number of embeddings in this project's index
- `hnswVersion`: Version of HNSW implementation for compatibility tracking
- `dataFormatVersion`: Schema version for migrations

**Note**: The HNSW graph structure itself is not persisted; only metadata about rebuilds is stored.

### config/ bucket

**Purpose**: Store Storm daemon configuration settings.

**Key**: `config` (single configuration document)

**Value**:
```json
{
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

**Structure**:
- `port`: HTTP server port
- `embeddingModel`: Which Ollama model to use
- `ollama.endpoint`: Ollama API endpoint
- `hnsw`: HNSW algorithm parameters (M=connections per node, ef parameters for search width)

## Design Rationale

### Content-Addressed Chunks (ChunkID as CID)

**Why Hash-Based Keys**:
- **Deduplication**: Identical content blocks across multiple files share same CID and embedding
- **Integrity**: Chunk hash serves as tamper detection (CID mismatch = file modified)
- **Determinism**: Same chunk content always produces same CID regardless of source
- **Simplicity**: No need for sequential IDs or complex versioning

**Example**:
```
File A (chat.md): "The capital of France is Paris"
File B (data.csv): "The capital of France is Paris"

ChunkID = CID("The capital of France is Paris")

Both files reference the same CID; embedding computed once, reused for both.
```

### Why Discussion Files Are Not Special in Storage

- **Chunks**: Use identical CID-based mechanism as other files
- **Embeddings**: Shared with any file containing matching content
- **Structure**: Stored in `files/` bucket with identical schema as input/output files
- **Distinction**: Semantic (contains query-response history) rather than structural

**Storage Implication**: No separate handling needed; `currentDiscussionFile` pointer and `discussionFile` context in roundHistory provide all necessary semantics.

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
- No queryID/roundID duplication per chunk (moved to roundHistory in projects bucket)
- File structure acts like filesystem inode (filepath → list of blocks)
- Supports file changes: update fileTimestamp, chunks remain with old CIDs until garbage collected

### Why Remove queryID and roundID from Storage?

- **Redundant**: Multiple chunks from same query/round share identical queryID/roundID
- **Space Waste**: Duplicated in every embedding entry
- **Versioning**: Query history belongs in projects metadata roundHistory or separate audit log, not per-chunk
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
- **Flexible Sharing**: Supports chunks shared across multiple files and discussion contexts
- **Cleaner Queries**: Direct CID lookup is O(1), no filepath prefix scanning needed

## Storage Operations

### Embedding Storage
```
When processing chat round with output files:
1. Extract markdown chunks from output files
2. Call Ollama to generate embeddings for new chunks
3. Compute CID hash of chunk content
4. Store in BoltDB (single transaction):
   a. Check if CID exists in embeddings/
      - If yes: skip (embedding already computed)
      - If no: store in embeddings/{CID} → vector_bytes
   b. Update files/{currentDiscussionFile} to append new chunks:
      { CID, offset, length } to chunks[] array
   c. Update projects/{projectID}/roundHistory with discussionFile context
5. Add embeddings to in-memory HNSW index for owning project
```

### Semantic Search
```
When querying with semantic context:
1. Generate embedding for query via Ollama
2. Search project-specific HNSW index for k nearest neighbors
   (returns list of CIDs)
3. For each CID:
   a. Find which files reference this CID by scanning files/ bucket
   b. Filter by authorizedFiles for this project
   c. Optionally filter by discussion file if searching within specific discussion
   d. Read chunk from file at specified offset/length
   e. Compute CID hash of read content, verify against expected CID
4. Return ranked results with content and source discussion file
```

### Garbage Collection
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

### Handling File Modifications

**When File Changes**:
1. Recompute chunks for modified file (may differ from before)
2. Generate new CIDs for new content blocks
3. Store new CIDs in embeddings/ bucket (if new content)
4. Update files/{filepath} with new chunk list and fileTimestamp
5. Update projects/{projectID}/discussionFiles[].roundCount if modified file is discussion file
6. On next garbage collection: old CIDs no longer referenced by any file are deleted

### Managing Discussion Files

**Creating new discussion file**:
```
storm-cli discussion create --project X --name "chat-2.md"

Operations:
1. Create new Chat instance at project baseDir/chat-2.md
2. Add to discussionFiles array in projects/{projectID}
3. Optionally set as currentDiscussionFile
4. Store changes via BoltDB transaction
```

**Switching discussion files**:
```
storm-cli discussion switch --project X chat-2.md
OR UI: Dropdown selector → switch

Operations:
1. Update projects/{projectID}/currentDiscussionFile pointer
2. Load Chat instance for new markdown file
3. UI/Browser reloads chat history from new file
4. Subsequent rounds append to active discussion file only
```

**Listing discussion files**:
```
storm-cli discussion list --project X

Returns discussionFiles array with metadata:
- filepath
- createdAt
- roundCount
```

## Startup Procedure

```
On startup:
1. Open BoltDB instance at ~/.storm/data.db
2. Load project metadata from projects/ bucket
3. For each project:
   a. Identify currentDiscussionFile
   b. Load Chat instance for that discussion file
4. Rebuild HNSW indices per project:
   - For each project, iterate authorizedFiles
   - Scan files/ bucket for filepaths in authorizedFiles
   - Collect all CIDs referenced by those files
   - Load corresponding embeddings from embeddings/ bucket
   - Build HNSW index in memory
5. Ready for semantic search queries
```

## Discussion Files vs Input/Output Files

### Key Differences

| Aspect | Discussion File | Input/Output Files |
|--------|-----------------|-------------------|
| **Purpose** | Stores query-response history | Static input data or LLM output |
| **Mutability** | Grows via appended rounds | Immutable from DB perspective |
| **Creation** | Created with project or on-demand | Authorized via CLI/API |
| **Tracking** | currentDiscussionFile pointer + discussionFiles list | Stored in authorizedFiles array |
| **Chunks** | Same CID-based chunking as other files | Identical chunking mechanism |
| **Embeddings** | Shared embeddings with other files (if content matches) | Shared embeddings with discussion files |
| **Semantic Search** | Searchable within current discussion context | Searchable; results include source file |
| **Garbage Collection** | Preserved across rounds; old chunks cleaned only if file deauthorized | Same cleanup process |

### Storage Implications

**No Structural Difference**:
- Both stored in `files/` bucket with identical schema
- Both use CID-based chunks
- Both share embeddings in `embeddings/` bucket
- Both tracked in `projects/{projectID}/roundHistory` (discussion files have `discussionFile` context)

**Operational Difference**:
- Discussion files linked via `currentDiscussionFile` (active pointer)
- Input/output files linked via `authorizedFiles` array (permissions)
- When switching discussion files, only the pointer changes; content remains persistent

## Project Registry Integration

### Store in Same BoltDB Instance

**Approach**: Use separate bucket `projects/` for project metadata
- **Key**: projectID
- **Value**: JSON-encoded project struct including discussionFiles list and roundHistory

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
            // Load Chat for currentDiscussionFile
            project.Chat = NewChat(project.CurrentDiscussionFile)
            return nil
        })
    })
    return projects, err
}
```

## Migration Path

1. **Current State**: In-memory project registry, no embeddings, single markdownFile per project
2. **Phase 1** (Week 1): Add BoltDB persistence for projects, load on startup, support multiple discussionFiles
3. **Phase 2** (Week 2): Implement Ollama embedding generation and file storage with chunkID-based buckets
4. **Phase 3** (Week 3): Integrate fogfish/hnsw for semantic search over content-addressed embeddings
5. **Phase 4** (Week 4): Add CLI commands for discussion file management (create, switch, list)
6. **Future** (if needed): Add graph database only if relationships become complex

## Performance Expectations

- **Startup**: Loading 1000 projects + 100k embeddings + HNSW rebuild ≈ 2-5 seconds
- **Discussion File Switch**: Update pointer + reload Chat ≈ <100ms
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
- **Discussion File Access**: Only currentDiscussionFile is writable; switching requires explicit user action

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

