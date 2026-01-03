# Vector Database Design for Storm

## Overview

Storm needs to support semantic search over markdown discussion files using embeddings. This document outlines the persistence architecture combining in-memory approximate nearest neighbor (ANN) search with embedded key-value storage using content-addressed chunk identifiers.

## How Storm Could Use a Vector Database

### Current Workflow vs. Vector-Enhanced Workflow

**Current Workflow (Manual File Selection)**:
1. User types query
2. User manually selects `inputFiles` (files relevant to the query)
3. User selects `outFiles` (files to include in LLM response)
4. Server sends query + selected files to LLM
5. LLM responds with code/text
6. Files are extracted and written to disk

**Problem**: Users must remember which files are relevant and manually specify them each time. In multi-round conversations, users re-specify files for follow-up questions even though context is already established.

**Vector-Enhanced Workflow (Semantic File Discovery)**:
1. User types query
2. Storm converts query to embeddings
3. Vector DB searches file embeddings for semantic similarity
4. Top-N relevant files automatically suggested as `inputFiles`
5. User optionally approves/modifies auto-selected files
6. Server sends query + files to LLM
7. LLM responds
8. Files are extracted and written to disk

**Benefit**: Automatic file discovery, especially powerful for multi-round queries where context is implicitly maintained.

### Enabling Multi-Round Queries with Vector DB

**Problem with Current Implementation**:
- Each query is independent
- Users must re-specify files for follow-up questions
- No automatic context linking between rounds
- Multi-round conversations become burdensome as projects grow

**Vector DB Solution - Implicit Context Management**:

When a user asks a follow-up question, the vector DB finds:
1. **Files mentioned in previous rounds** (high semantic similarity to previous queries)
2. **Files relevant to current query** (similarity to new query)
3. **Files bridging topics** (semantic connections between conversation threads)

**Example Flow**:
```
Round 1: User: "Add authentication to my API"
         Files automatically selected: auth.go, handlers.go
         LLM modifies these files
         Embeddings stored: query, response, modified files

Round 2: User: "Now add rate limiting"
         Vector search finds:
         - handlers.go (high relevance to "rate limiting")
         - middleware.go (semantic connection to handlers)
         - auth.go (high relevance to previous context - auth was just modified)
         User gets auto-selected files without manually re-specifying
         User can approve or adjust before sending

Round 3: User: "But don't block authenticated admin requests"
         Vector search finds:
         - handlers.go (rate limiting context from Round 2)
         - auth.go (authentication context from Rounds 1-2)
         - admin.go or config.go (if they exist - semantic match to "admin")
         Context is implicitly maintained across all three rounds
```

### Multi-Round Context Embedding Strategy

To support multi-round context, store embeddings for:

**Query Embeddings**:
- Each user query converted to embedding
- Stored with metadata: `{ query, queryID, roundID, timestamp }`
- Later queries compared for semantic similarity

**Response Embeddings**:
- Each LLM response converted to chunks and embedded
- Metadata links response to query and modified files
- Enables finding relevant prior responses for context

**File Modification Embeddings**:
- When files are modified by LLM, store embeddings of modified content
- Metadata links to which query/round caused the modification
- New query can find files that were recently modified in similar contexts

**Conversation Thread Embeddings**:
- Optional: Store aggregate embedding for entire conversation thread
- Enables finding files relevant to overall conversation topic
- Useful for suggesting files when starting a new related discussion

**Schema**:
```go
type RoundEmbedding struct {
    RoundID         string    // Links to specific round in discussion
    DiscussionFile  string    // Which discussion file (chat.md, chat-2.md, etc.)
    QueryEmbedding  []float32 // Embedding of user query
    FileEmbeddings  map[string][]float32 // CID → embedding of modified file content
    Timestamp       time.Time
}
```

### Impact on Manual File Selection

**Multi-Round Queries - Auto-Selection Becomes Default**:

| Query Type | Manual Selection Still Needed | Why |
|------------|-------------------------------|-----|
| Follow-up in same context | ❌ Rarely | Vector search maintains implicit context |
| Switching topics | ✓ Sometimes | New topic may need different files |
| Disambiguating similar files | ✓ Sometimes | User may want specific variant |
| Cross-cutting changes | ✓ Often | Requires precise file selection |
| Starting new discussion | ✓ Often | No context history yet |

**Suggested UX Flow**:

```javascript
// Round 1 of discussion
User types: "Add authentication"
→ Vector search finds: [] (first round, no history)
→ User manually selects: auth.go, handlers.go
→ LLM responds
→ Embeddings stored for Round 1

// Round 2 - Auto-selection enabled
User types: "Add rate limiting"
→ Vector search finds: handlers.go, middleware.go, auth.go
→ UI shows suggested files with confidence scores:
    [✓] handlers.go (95% relevance to "rate limiting")
    [✓] middleware.go (87% relevance - middleware handles rate limiting)
    [ ] auth.go (82% relevance - modified in previous round)
→ User can check/uncheck files
→ User clicks Send → LLM processes
→ Next embedding stored
```

### When Manual Selection Still Provides Value

Even with vector DB, users benefit from manual control when:

**1. Intentionally Excluding Files**:
```
User: "Add this feature but don't touch database code"
→ Vector search might suggest db.go as relevant
→ User unchecks db.go before sending
→ LLM avoids file that's semantically related but explicitly excluded
```

**2. Disambiguation**:
```
Project has: handlers.go, api_handlers.go, request_handlers.go
Vector search returns all three
User wants only: api_handlers.go
→ User unchecks the others
```

**3. Cross-Cutting Concerns**:
```
User: "Refactor these three specific files to use new interface"
→ Vector search might not correlate the three files together
→ User manually selects all three
→ LLM sees coherent set of related files
```

**4. Starting New Context**:
```
User: "Start a new module for payment processing"
→ No history exists yet
→ Vector search returns [] or generic files
→ User manually selects: payments.go, stripe_client.go, etc.
→ Establishes new context thread
```

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

### 4. Persistence Layer (KV Store)
- **Role**: Embedded key-value store for metadata, embeddings, and file references
- **ACID Guarantees**: Transaction support prevents corruption
- **No C++ Dependencies**: Pure Go implementations available

## Bucket Schema (KV Store)

All persistent data in Storm is organized into KV store buckets using CBOR-encoded values.

### projects/ bucket

**Purpose**: Store project metadata including configuration, discussion files, and round history.

**Key**: `{projectID}` (string identifier)

**Value** (CBOR-encoded project struct with discussion file context)

### files/ bucket

**Purpose**: Store file inode-like structures mapping filepath to its constituent chunks.

**Key**: `{filepath}` (full path to file)

**Value** (CBOR-encoded file structure with chunk references)

### embeddings/ bucket

**Purpose**: Store raw embedding vectors keyed by content hash.

**Key**: `{CID}` (content-addressed identifier)

**Value** (CBOR-encoded embedding with float32 vector)

### hnsw_metadata/ bucket

**Purpose**: Store versioning and rebuild metadata for HNSW indices.

**Key**: `{projectID}:metadata`

**Value** (CBOR-encoded HNSW metadata)

### config/ bucket

**Purpose**: Store Storm daemon configuration settings.

**Key**: `config`

**Value** (CBOR-encoded configuration)

## Design Rationale

### Content-Addressed Chunks (ChunkID as CID)

**Why Hash-Based Keys**:
- **Deduplication**: Identical content blocks across multiple files share same CID and embedding
- **Integrity**: Chunk hash serves as tamper detection
- **Determinism**: Same chunk content always produces same CID
- **Simplicity**: No need for sequential IDs or complex versioning

### Why Discussion Files Are Not Special in Storage

- **Chunks**: Use identical CID-based mechanism as other files
- **Embeddings**: Shared with any file containing matching content
- **Structure**: Stored in `files/` bucket with identical schema
- **Distinction**: Semantic (contains query-response history) rather than structural

### Why Store File References Instead of Text?

- **Storage Efficiency**: Eliminates text duplication; markdown file is authoritative source
- **Cache Validity**: File timestamp allows invalidating stale embeddings when files change
- **Content Verification**: CID hash enables integrity checking and deduplication
- **Precise Retrieval**: Offset and length allow exact chunk extraction from file

### Why Key by CID Instead of Filepath?

- **Reduces Duplication**: Identical chunks across projects/files stored once
- **Simplifies Garbage Collection**: Scan embeddings bucket, verify each CID exists in files bucket
- **Flexible Sharing**: Supports chunks shared across multiple files and discussion contexts
- **Cleaner Queries**: Direct CID lookup is O(1)

## Storage Operations

### Embedding Storage
```
When processing chat round with output files:
1. Extract markdown chunks from output files
2. Call Ollama to generate embeddings for new chunks
3. Compute CID hash of chunk content
4. Execute Update transaction to store embeddings and file references
5. Add embeddings to in-memory HNSW index
```

### Semantic Search
```
When querying with semantic context:
1. Generate embedding for query via Ollama
2. Search project-specific HNSW index for k nearest neighbors
3. For each CID: verify it exists and is in authorized files
4. Return ranked results with content and source discussion file
```

### Garbage Collection
```
Background garbage collection (daily or on-demand):
1. Scan all embeddings/ bucket keys (all CIDs)
2. For each CID: check if referenced by any authorized file
3. If not referenced: delete from embeddings/ bucket
```

## Migration Path

1. **Phase 1**: Add KV persistence with CBOR encoding for projects
2. **Phase 2**: Implement Ollama embedding generation and file storage
3. **Phase 3**: Integrate fogfish/hnsw for semantic search
4. **Phase 4**: Add CLI commands for discussion file management
5. **Future**: Add graph database only if relationships become complex
