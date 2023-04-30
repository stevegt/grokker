- don't store text chunks -- just hash them instead, and store
  hash, filename, timestamp, position, length, and embedding.
- replace json storage with kv
  - write kv.go, use it as an adapter so we can use e.g. bbolt now and
    swap to puptree later
  - add 'migrate' subcommand to convert db from json to kv
- add cli test cases
- make workflow more like git:
  - add 'grok status'
  - add .grokignore
- consider implementing a 'chat' subcommand that would maintain 
  user/AI headings on each text block in an edit buffer and use those
  in chat completion message
- review/implement use of system message for gpt-4, since it's better
  supported in that model
- convert universe into a general purpose PUP processor 
  - define Go API for PUP calls
    - don't worry about wire format for now
  - universe implements PUP API 
  - bbolt is cache in PUP processor
  - cache and any external APIs are called via interfaces 
- add summary tree of all chunks
  - via PUP treeing
- write db.go, four buckets:
  - each chunk keyed by hash
  - each embedding keyed by hash
  - hash list keyed by filename
  - hash of chat message history as a merkle tree 
    - most recent key is "root"
- write context.go: 
  - takes a query, matches with embeddings, returns a synthesized
    slice of chat messages
  - provides increasingly summarized context in pseudo-chat messages
    going back in time
  - keeps track of user/assistant messages and summarizes them
    separately
- add 'sh' subcommand:
  - supports dot commands for directives:
    - .help
    - .add {fn}
    - .update updates all embeddings 
    - .review {fn} re-adds document chunks to recent context
  - otherwise acts as a chat client
    - adds each message to context
  - adds each tx/rx message to context tree
- add 'vet' subcommand
  - works like "go vet" -- look for inconsistencies, possibly by
    comparing summaries with individual chunks, or by negating and
    comparison; see demorgan etc.
- add 'downgrade' subcommand 
- think through what a scripting or compiled language would look like
  that would let us generate context from question+db, see and edit
  the query before sending, have results incorporated in same buffer
  as part of context for next query
