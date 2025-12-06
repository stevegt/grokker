# Vector Database Design for Storm

## Overview

Storm needs to support semantic search over markdown discussion files using embeddings. This document outlines the persistence architecture combining in-memory approximate nearest neighbor (ANN) search with embedded key-value storage.

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

### 3. Persistence Layer (BoltDB)
- **Role**: Embedded key-value store for metadata, embeddings, and chunks
- **Data Buckets**:
  - `projects/`: Project metadata (ID, baseDir, markdownFile, authorizedFiles)
  - `embeddings/{projectID}`: Raw embedding vectors with metadata
  - `chunks/{projectID}`: Markdown text chunks corresponding to embeddings
  - `hnsw_metadata/`: Versioning and index rebuild timestamps
  - `config/`: Storm daemon configuration settings
- **ACID Guarantees**: Transaction support prevents corruption
- **No C++ Dependencies**: Pure Go implementation

## Design Rationale

### Why HNSW + BoltDB Instead of Graph Database?

**Graph Database Considerations**:
- **Pros**: Efficient complex relationship queries, built-in graph traversal
- **Cons**: Overkill for Storm's simple data model, external service/process required, operational complexity

**Storm's Data Model**:
- Flat hierarchy: Projects → Chunks → Embeddings (no complex traversals)
- No multi-hop queries needed (e.g., no "find projects connected through users")
- Simple access patterns (project lookup, chunk retrieval by similarity)
- **Conclusion**: Graph database complexity unjustified

### Why Not Persist HNSW Graph?

- **Graph Serialization Overhead**: HNSW's hierarchical graph structure is expensive to serialize/deserialize
- **Rebuild Cost is Negligible**: Indexing 10k embeddings takes <1 second on modern hardware
- **Simplicity**: Avoids versioning and compatibility issues across daemon updates
- **Memory Efficiency**: In-memory index only loaded during daemon lifetime

## Implementation Strategy

### Phase 1: Basic Setup
```
On startup:
1. Open BoltDB instance at ~/.storm/data.db
2. Load project metadata from projects/ bucket
3. For each project, load embeddings from embeddings/{projectID}
4. Build HNSW index from loaded embeddings in memory
5. Ready for semantic search queries
```

### Phase 2: Embedding Storage
```
When processing chat round:
1. Extract markdown chunks from output files
2. Call Ollama to generate embeddings for new chunks
3. Store in BoltDB:
   - embeddings/{projectID}: { chunkID → vector_bytes }
   - chunks/{projectID}: { chunkID → { text, timestamp, queryID } }
4. Add embeddings to in-memory HNSW index
```

### Phase 3: Semantic Search
```
When querying with semantic context:
1. Generate embedding for query via Ollama
2. Search HNSW index for k nearest neighbors
3. Retrieve corresponding chunks from BoltDB
4. Return ranked results with content
```

## Bucket Schema (BoltDB)

### projects/ bucket
```json
Key: projectID
Value: {
  "id": "project-1",
  "baseDir": "/path/to/project",
  "markdownFile": "chat.md",
  "authorizedFiles": ["data.csv", "output.json"],
  "createdAt": "2025-12-06T03:00:00Z",
  "embeddingCount": 256
}
```

### embeddings/{projectID} bucket
```json
Key: chunkID (e.g., "chunk-0001")
Value: {
  "embedding": <binary float32 vector (768*4 bytes)>,
  "metadata": {
    "roundID": "round-5",
    "queryID": "query-abc123",
    "timestamp": "2025-12-06T03:15:00Z",
    "textLength": 512
  }
}
```

### chunks/{projectID} bucket
```json
Key: chunkID
Value: {
  "text": "This is the markdown chunk...",
  "startLine": 42,
  "endLine": 48,
  "queryID": "query-abc123",
  "roundID": "round-5"
}
```

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

### Alternative: Embedded Graph Databases

**Neo4j Embedded** / **DGraph** / **BadgerDB with graph layer**:
- **Pros**: Graph queries if relationships become complex, potential horizontal scaling
- **Cons**: Significantly larger Go binaries, more complex API, overkill for Storm's current model, operational overhead
- **Recommendation**: Revisit only if Storm needs true graph traversal patterns

## Project Registry Integration

### Store in Same BoltDB Instance

**Approach**: Use separate bucket `projects/` for project metadata
- **Key**: projectID
- **Value**: JSON-encoded project struct (name, baseDir, markdownFile, authorizedFiles, timestamps)

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
3. **Phase 2** (Week 2): Implement Ollama embedding generation and BoltDB storage
4. **Phase 3** (Week 3): Integrate fogfish/hnsw for semantic search over persisted embeddings
5. **Future** (if needed): Add graph database only if relationships become complex

## Performance Expectations

- **Startup**: Loading 1000 projects + 100k embeddings ≈ 2-5 seconds
- **Semantic Search**: HNSW query against 100k embeddings ≈ 5-20ms (ef parameter tuned)
- **Embedding Generation**: Ollama for 512-token chunk ≈ 50-100ms per chunk
- **Database Operations**: BoltDB get/put ≈ <1ms (in-memory mmap overhead minimal)

## Security Considerations

- **BoltDB Files**: Store at `~/.storm/data.db`, restrict file permissions (0600)
- **Embeddings**: Do not expose raw embeddings in API responses; only return semantic similarity scores
- **Project Isolation**: Embeddings scoped by projectID; cross-project queries not possible

## Recommended Configuration

```go
// hnsw parameters for Storm
M := 16                  // max connections per node
efConstruction := 200    // ef during insertion
efSearch := 100          // ef during search

// BoltDB tuning
FreelistType := "array" // faster than map for typical usage
PageSize := 4096        // default, suitable for embeddings
```
