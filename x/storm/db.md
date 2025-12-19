# Vector Database Design for Storm

## Overview

Storm needs to support semantic search over markdown discussion files using embeddings. This document outlines the persistence architecture combining in-memory approximate nearest neighbor (ANN) search with embedded key-value storage using content-addressed chunk identifiers.

## Architecture Components

### 1. KV Store Abstraction Layer

All persistent operations go through a `KVStore` interface, allowing runtime selection of the underlying key-value store implementation. Currently implemented with BoltDB; future implementations can use BadgerDB, RocksDB, or SQL-backed stores without application logic changes[1][2].

**Interface Methods**:
```go
type KVStore interface {
    // Transactions
    View(func(ReadTx) error) error
    Update(func(WriteTx) error) error
    
    // Bucket operations (within transactions)
    BucketNames() []string
    
    // Connection management
    Close() error
}

type ReadTx interface {
    Get(bucket, key string) []byte
    ForEach(bucket string, fn func(k, v []byte) error) error
}

type WriteTx interface {
    ReadTx
    Put(bucket, key string, value []byte) error
    Delete(bucket, key string) error
    CreateBucketIfNotExists(bucket string) error
}
```

**Advantages**[1][2]:
- **No Coupling**: Business logic never imports BoltDB directly
- **Testability**: Mock KVStore for unit tests
- **Flexibility**: Swap implementations at runtime or compile-time
- **Gradual Migration**: Run multiple backends during transition periods

### 2. Embedding Generation (Ollama)
- **Role**: Generates embedding vectors locally without external services
- **Integration**: HTTP API calls to `localhost:11434/api/embed`
- **Models**: `nomic-embed-text`, `mxbai-embed-large`
- **Output**: High-dimensional vectors (768-1024 dimensions)

### 3. Approximate Nearest Neighbor Search (HNSW)
- **Library**: `github.com/fogfish/hnsw` (pure Go)
- **Role**: In-memory index for fast similarity search
- **Data**: Stores embedding vectors and chunk references
- **Lifecycle**: Rebuilt on daemon startup from persisted embeddings
- **Scope**: Per-project indices (separate HNSW index for each project's embeddings)

### 4. Multi-Discussion File Support

Each project maintains multiple markdown discussion files, each with its own query-response history. Users switch between discussions via CLI or UI without data loss. Each discussion file lives in the `files/` bucket with identical structure as other project files, distinguished only by project metadata pointers.

**Key Design Points**:
- Discussion files use identical chunking and embedding as input/output files
- `currentDiscussionFile` pointer in projects metadata indicates active discussion
- `discussionFiles` array tracks all available discussions with metadata
- Rounds are filtered by `discussionFile` context in roundHistory
- Switching discussions requires only pointer update; content persists

### 5. Persistence Layer (KV Store)
- **Role**: Embedded key-value store for metadata, embeddings, and file references
- **ACID Guarantees**: Transaction support prevents corruption (via KVStore interface)
- **No C++ Dependencies**: Pure Go implementations available
- **Serialization**: CBOR (Concise Binary Object Representation) for compact binary storage

## Serialization Format: CBOR

**Why CBOR over JSON**[1]:
- **Compact**: Binary encoding reduces storage by 30-50% compared to JSON
- **Fast**: Encoding/decoding faster than JSON text parsing
- **Type-Safe**: Preserves data types (integers, floats, byte strings) without ambiguity
- **Standard**: RFC 8949 standard with excellent Go library support
- **Suitable for Embedded**: Designed for constrained environments and efficient storage

**Go Implementation**: Use `github.com/fxamacker/cbor/v2` for RFC 8949-compliant CBOR encoding/decoding.

## Bucket Schema (KV Store)

All persistent data in Storm is organized into KV store buckets using CBOR-encoded values. This section documents the schema for each bucket including key structure and value structure.

### projects/ bucket

**Purpose**: Store project metadata including configuration, discussion files, and round history.

**Key**: `{projectID}` (string identifier)

**Value** (CBOR-encoded):
```go
type Project struct {
  ID                    string                 // "project-1"
  BaseDir               string                 // "/path/to/project"
  CurrentDiscussionFile string                 // "chat.md"
  DiscussionFiles       []DiscussionFileRef    // Available discussion files
  AuthorizedFiles       []string               // ["data.csv", "output.json"]
  CreatedAt             time.Time              // Timestamp
  EmbeddingCount        int                    // 256
  RoundHistory          []RoundEntry           // Query history with discussion context
}

type DiscussionFileRef struct {
  Filepath  string    // "chat.md"
  CreatedAt time.Time // "2025-12-05T10:00:00Z"
  RoundCount int      // 12
}

type RoundEntry struct {
  RoundID        string        // "round-5"
  DiscussionFile string        // "chat.md" - which discussion this belongs to
  QueryID        string        // "query-abc123"
  Timestamp      time.Time     // Round execution time
  CIDs           []string      // ["hash1", "hash2", "hash3"] - chunk identifiers
}
```

### files/ bucket

**Purpose**: Store file inode-like structures mapping filepath to its constituent chunks.

**Key**: `{filepath}` (full path to file, e.g., "/projects/project-1/chat.md")

**Value** (CBOR-encoded):
```go
type FileInode struct {
  Filepath      string          // "/path/to/project/chat.md"
  FileTimestamp time.Time       // Last modification time
  Chunks        []ChunkRef      // Array of chunk references
}

type ChunkRef struct {
  CID    string // "QmXxxx..." - content identifier hash
  Offset int    // 0 - byte offset in file
  Length int    // 512 - number of bytes
}
```

**Structure**:
- `Filepath`: Full path for consistent lookups
- `FileTimestamp`: Modification time for cache invalidation
- `Chunks`: Array of chunk references (order matters for file reconstruction)
  - `CID`: Content identifier hash of chunk
  - `Offset`: Byte offset in original file
  - `Length`: Number of bytes in chunk

**Important**: Discussion files and other files coexist with identical structure. No distinction in storage layer.

### embeddings/ bucket

**Purpose**: Store raw embedding vectors keyed by content hash.

**Key**: `{CID}` (content-addressed identifier, typically SHA256 hash)

**Value** (CBOR-encoded):
```go
type EmbeddingEntry struct {
  Vector   []float32          // Binary embedding vector
  Metadata EmbeddingMetadata
}

type EmbeddingMetadata struct {
  ModelName   string    // "nomic-embed-text"
  GeneratedAt time.Time // Timestamp
}
```

**Structure**:
- `Vector`: Float32 array (768 or 1024 elements typically)
- `Metadata.ModelName`: Which Ollama model generated this embedding
- `Metadata.GeneratedAt`: Timestamp for cache invalidation

**Key Property**: CID-based keys enable deduplication—identical chunks across multiple files share the same embedding vector.

### hnsw_metadata/ bucket

**Purpose**: Store versioning and rebuild metadata for HNSW indices.

**Key**: `{projectID}:metadata` (project-specific metadata)

**Value** (CBOR-encoded):
```go
type HNSWMetadata struct {
  LastRebuild       time.Time // Timestamp of last rebuild
  EmbeddingCount    int       // 256
  HNSWVersion       string    // "v1"
  DataFormatVersion string    // "1.0"
}
```

**Structure**:
- `LastRebuild`: Timestamp of last HNSW index rebuild
- `EmbeddingCount`: Number of embeddings in this project's index
- `HNSWVersion`: Version of HNSW implementation for compatibility tracking
- `DataFormatVersion`: Schema version for migrations

**Note**: The HNSW graph structure itself is not persisted; only metadata about rebuilds is stored.

### config/ bucket

**Purpose**: Store Storm daemon configuration settings.

**Key**: `config` (single configuration document)

**Value** (CBOR-encoded):
```go
type Config struct {
  Port           int          // 8080
  EmbeddingModel string       // "nomic-embed-text"
  Ollama         OllamaConfig
  HNSW           HNSWConfig
}

type OllamaConfig struct {
  Endpoint string // "http://localhost:11434"
}

type HNSWConfig struct {
  M              int // 16 - max connections per node
  EfConstruction int // 200 - ef during construction
  EfSearch       int // 100 - ef during search
}
```

**Structure**:
- `Port`: HTTP server port
- `EmbeddingModel`: Which Ollama model to use
- `Ollama.Endpoint`: Ollama API endpoint
- `HNSW`: HNSW algorithm parameters (M=connections per node, ef parameters for search width)

## Design Rationale

### Content-Addressed Chunks (ChunkID as CID)

**Why Hash-Based Keys**:
- **Deduplication**: Identical content blocks across multiple files share same CID and embedding
- **Integrity**: Chunk hash serves as tamper detection (CID mismatch = file modified)
- **Determinism**: Same chunk content always produces same CID regardless of source
- **Simplicity**: No need for sequential IDs or complex versioning

### Why Discussion Files Are Not Special in Storage

- **Chunks**: Use identical CID-based mechanism as other files
- **Embeddings**: Shared with any file containing matching content
- **Structure**: Stored in `files/` bucket with identical schema as input/output files
- **Distinction**: Semantic (contains query-response history) rather than structural

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

### CBOR Format Benefits for Embedded Storage

- **Compact Serialization**: Binary format reduces storage by 30-50% vs JSON text
- **Fast Encoding/Decoding**: No string parsing overhead; direct binary operations
- **Type Safety**: Numbers stored as binary integers/floats, not string representations
- **Bandwidth Efficient**: If ever replicating data, smaller serialization size
- **Standard Format**: RFC 8949 compliance ensures long-term compatibility

## Storage Operations

### Embedding Storage
```
When processing chat round with output files:
1. Extract markdown chunks from output files
2. Call Ollama to generate embeddings for new chunks
3. Compute CID hash of chunk content
4. Execute Update transaction:
   a. Check if CID exists in embeddings/
      - If yes: skip (embedding already computed)
      - If no: CBOR-encode EmbeddingEntry, Put in embeddings/{CID}
   b. Update files/{currentDiscussionFile}:
      Append ChunkRef{ CID, offset, length } to chunks array
      Encode FileInode to CBOR, Put in files/{filepath}
   c. Update projects/{projectID}:
      Append RoundEntry, CBOR-encode entire Project
5. Add embeddings to in-memory HNSW index for owning project
```

### Semantic Search
```
When querying with semantic context:
1. Generate embedding for query via Ollama
2. Search project-specific HNSW index for k nearest neighbors
   (returns list of CIDs)
3. Execute View transaction:
   For each CID:
   a. Get from embeddings/{CID} to verify it exists
   b. Scan files/ bucket for references to this CID
   c. Filter by authorizedFiles for this project
   d. Read chunk from file at specified offset/length
   e. Verify CID hash of read content
4. Return ranked results with content and source discussion file
```

### Garbage Collection
```
Background garbage collection (daily or on-demand):
1. Execute View transaction to scan embeddings/ bucket
2. For each CID:
   a. Scan files/ bucket for any reference to this CID
   b. Check if filepath is in ANY project's authorizedFiles
   c. If not referenced:
      - Schedule for deletion
3. Execute Update transaction to delete orphaned CIDs
```

### Handling File Modifications

**When File Changes**:
1. Recompute chunks for modified file (may differ from before)
2. Generate new CIDs for new content blocks
3. Store new CIDs in embeddings/ bucket (if new content), CBOR-encoded
4. Update files/{filepath} with new chunk list and fileTimestamp
5. Update projects/{projectID}/discussionFiles[].roundCount if modified file is discussion file
6. On next garbage collection: old CIDs no longer referenced by any file are deleted

### Managing Discussion Files

**Creating new discussion file**:
```
storm-cli discussion create --project X --name "chat-2.md"

Operations (Update transaction):
1. Create new Chat instance at project baseDir/chat-2.md
2. Add to discussionFiles array in projects/{projectID}
3. Optionally set as currentDiscussionFile
4. CBOR-encode and Put updated Project struct
```

**Switching discussion files**:
```
storm-cli discussion switch --project X chat-2.md
OR UI: Dropdown selector → switch

Operations (Update transaction):
1. Update projects/{projectID}/currentDiscussionFile pointer
2. CBOR-encode and Put updated Project struct
3. UI/Browser reloads chat history from new file
4. Subsequent rounds append to active discussion file only
5. Update projects/{projectID} in BoltDB, CBOR-encoded
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
1. Initialize KVStore (currently BoltDB)
2. Execute View transaction to load project metadata:
   - For each key in projects/ bucket:
     - Get and CBOR-decode value to Project struct
3. For each project:
   a. Identify currentDiscussionFile
   b. Load Chat instance for that discussion file
4. Rebuild HNSW indices per project (View transaction):
   - For each project, iterate authorizedFiles
   - Scan files/ bucket for filepaths in authorizedFiles
   - For each file: Get and CBOR-decode FileInode
   - Collect all CIDs referenced
   - Load corresponding embeddings from embeddings/ bucket
   - Populate float32 vectors from EmbeddingEntry entries
   - Build HNSW index in memory
5. Ready for semantic search queries
```

## Discussion Files vs Input/Output Files

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

## KV Store Abstraction Benefits

### Testability

Mock KVStore for unit tests without touching real database:
```go
type MockKVStore struct {
    data map[string]map[string][]byte
}

func (m *MockKVStore) View(fn func(ReadTx) error) error {
    return fn(&MockReadTx{data: m.data})
}

func (m *MockKVStore) Update(fn func(WriteTx) error) error {
    return fn(&MockWriteTx{data: m.data})
}
```

### Future Migration Path

If BoltDB's single-writer limitation becomes a bottleneck:

1. **BadgerDB**: Implements same KVStore interface, supports concurrent writes
2. **RocksDB**: C++ library with Go bindings; optimized for write-heavy workloads
3. **SQLite**: WAL mode supports concurrent readers + single writer
4. **DuckDB**: Columnar format suitable for analytical queries over embeddings

Simply implement `KVStore` interface for new backend, swap instantiation, no application changes needed.

## Project Registry Integration

### Store in Same KV Instance

**Approach**: Use separate bucket `projects/` for project metadata
- **Key**: projectID (string)
- **Value**: CBOR-encoded Project struct

**Advantages**:
- Single database file for all persistent state
- Atomic transactions across project and embedding updates
- Consistent backup strategy
- No coordination between multiple data stores

**Load on Startup** (Go pseudocode):
```go
import "github.com/fxamacker/cbor/v2"

func LoadProjectRegistry(db *KV.DB) (map[string]*Project, error) {
    projects := make(map[string]*Project)
    err := db.View(func(tx *KV.Tx) error {
        bucket := tx.Bucket([]byte("projects"))
        if bucket == nil {
            return nil // No projects yet
        }
        return bucket.ForEach(func(k, v []byte) error {
            var project Project
            err := cbor.Unmarshal(v, &project)
            if err != nil {
                return err
            }
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
2. **Phase 1** (Week 1): Add KV persistence with CBOR encoding for projects, load on startup, support multiple discussionFiles
3. **Phase 2** (Week 2): Implement Ollama embedding generation and file storage with chunkID-based buckets, CBOR-encode entries
4. **Phase 3** (Week 3): Integrate fogfish/hnsw for semantic search over content-addressed embeddings
5. **Phase 4** (Week 4): Add CLI commands for discussion file management (create, switch, list)
6. **Future** (if needed): Add graph database only if relationships become complex

## Performance Expectations

- **Storage Efficiency**: CBOR encoding reduces KV file size 30-50% vs JSON
- **Startup**: Loading 1000 projects + 100k embeddings + HNSW rebuild ≈ 2-5 seconds (CBOR decoding ~0.5s)
- **Discussion File Switch**: Update pointer + reload Chat ≈ <100ms
- **Semantic Search**: HNSW query against 100k embeddings ≈ 5-20ms (ef parameter tuned)
- **Embedding Generation**: Ollama for 512-token chunk ≈ 50-100ms per chunk
- **Chunk Retrieval**: Read from file at offset/length ≈ <1ms (OS page cache)
- **Database Operations**: KV get/put with CBOR ≈ <1ms (CBOR codec overhead negligible)
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
- **CBOR Parsing**: Validate CBOR-encoded data on deserialization; reject malformed entries

## Recommended Configuration

```go
// CBOR encoding parameters
import "github.com/fxamacker/cbor/v2"
var cborEncOptions = cbor.EncOptions{
    Sort: cbor.SortCanonical, // Canonical CBOR for deterministic encoding
}

// HNSW parameters for Storm
M := 16                  // max connections per node (trade-off: memory vs search quality)
efConstruction := 200    // ef during insertion (higher = better quality, slower insert)
efSearch := 100          // ef during search (higher = better recall, slower search)

// BoltDB tuning
FreelistType := "array"  // faster than map for typical usage
PageSize := 4096         // default, suitable for embeddings
BatchSize := 100         // entries to write per transaction
```

