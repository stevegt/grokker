- complete usage splash
- replace json storage with kv
  - write kv.go, use it as an adapter so we can use e.g. bbolt now and
    swap to puptree later
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

  
