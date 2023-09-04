- add a 'qr' subcommand 
  - optimized for iteratively evolving a document
  - uses stdio 
  - uses gpt-4-32k 
  - uses first paragraph as system message
  - uses remainder of document as query
  - provides context based on query 
  - emits response on stdout
- support 'dot' subcommand:
  - uses stdio 
  - .model {model name}
  - .sys 
    - system message follows in next paragraph
  - .itr
    - find context that closely matches remainder of document, add it
      as a user message, then add remainder of the document as a user
      message.
- write a neovim plugin
  - https://github.com/neovim/go-client
- add a ~/.grokconfig file
  - default model for new repos
- don't store text chunks -- just hash them instead, and store
  hash, filename, timestamp, position, length, and embedding.
- consider using a single .grok database in the user's home directory
  - hash all chunks
  - one embedding db
  - store hash, path, timestamp, pos, length instead of chunk content
  - maintain context by dir/subdir nesting
  - to include files from other projects as context, let 'grok add'
    maintain references in the db
- add a subcommand that automates the "grok init; touch context.md; grok add context.md" workflow
- replace json storage with kv
  - write kv.go, use it as an adapter so we can use e.g. bbolt now and
    swap to puptree later
  - add 'migrate' subcommand to convert db from json to kv
- add cli test cases
- make workflow more like git:
  - add 'grok status'
  - add .grokignore
  - one of:
    - when first detecting a grok subrepo, show subdir/context.md as untracked
    - include subdir/.grok db embeddings in parent context
- some sort of 'XXX' workflow, two steps:
  - find chunks with 'XXX' in them, via either regex of cos diff
  - send each chunk as a question
    - include file path and pos in response
- consider implementing a 'chat' subcommand that would maintain 
  user/AI headings on each text block in an edit buffer and use those
  in chat completion message
- consider adding a subcommand like 'chat' or 'context' that would let
  the user asynchronously enter queries while the previous query was
  still running; the code would automatically append queries and
  responses to context.md as responses arrive.
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
- add 'vet' subcommand
  - works like "go vet" -- look for inconsistencies, possibly by
    comparing summaries with individual chunks, or by negating and
    comparison; see demorgan etc.
- add 'downgrade' subcommand 
- think through what a scripting or compiled language would look like
  that would let us generate context from question+db, see and edit
  the query before sending, have results incorporated in same buffer
  as part of context for next query
