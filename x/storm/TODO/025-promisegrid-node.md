# 025 - PromiseGrid node mode (delegation + multi-node git)

TODO `015-interactive-cli.md` highlights a deeper architectural mismatch: Codex-like terminal UX assumes “the client can just do things locally”, while Storm’s model is “the client talks to a daemon which mediates LLM + file access”.

PromiseGrid’s model reframes this: “servers” are just other nodes you can delegate to, and a node can be local (same machine), a relay (NAT/firewall traversal), an HTTP gateway (browser clients), or a worker (executes delegated tasks). Capability tokens authorize what a node may do (files, tools/functions, LLM calls, git ops).

Background:
- PromiseGrid overview: `~/lab/grid-poc/llms-full.txt` (pCID-first message envelope; capability-as-promise; kernel-mediated calls)
- PromiseGrid descriptors prototype: `~/lab/grid-poc/x/descriptors/` (CBOR message + executable descriptor + memfd exec; useful for capabilities/tool calls)
- Git coordination inspiration: https://mob.sh/
- Local alternative script: `~/core/bin/mob-consensus`

## Goal

Evolve Storm so a local Storm process (shell and/or daemon) can operate as a **grid node**:
- delegate work to other nodes (LLM calls, repo/file operations, tool calls when added)
- receive results as auditable messages
- coordinate git operations across nodes (mob-style or consensus-merge style)
- preserve current single-daemon UX (`storm serve`, web UI, existing CLI) as the default path

## Non-goals (initially)

- Replacing PromiseGrid with Storm-specific protocols long-term.
- Perfect multi-tenant security (we need staged capabilities + auditing first).
- Solving every NAT traversal/topology problem in v1; start with explicit peers.

## Current Storm shape (baseline)

- `storm serve` starts an HTTP+WS daemon that owns project state (baseDir, authorized files) and performs LLM calls + filesystem writes.
- Clients (web, CLI) are thin: they send queries, selections, file lists, and approvals; server enforces policy and writes files.

Grid-node mode should keep this working unchanged when “grid” is off.

## What “grid node” means for Storm

### Roles (a node can run multiple roles)

- **Shell node**: interactive user entrypoint (TODO `015-interactive-cli.md`). Chooses where to run work (local vs remote).
- **HTTP gateway node**: runs Storm’s current HTTP+WS UI/API, but can delegate actual work to worker nodes.
- **Worker node**: executes delegated tasks (LLM calls, repo operations, tool/function calls) under explicit capabilities.
- **Relay node**: forwards/store-and-forwards messages to bridge NAT/firewalls (no local execution required).

### Delegation primitives (what can be delegated)

- **LLM execution**: run a query against an allowed provider/model and return a response + proposed edits.
- **Repo/file operations**: read/scan/patch within a repo clone on that node, subject to capabilities.
- **Git operations**: branch creation, commit, push/pull, merge, worktree creation, etc.
- **Tool/function calls** (future): run a bounded tool call in a sandbox and return artifacts (see TODO `027-descriptors-into-storm.md`).

### Capability tokens (authorization)

Storm already has “authorized files” as an allowlist. Grid mode generalizes this into capabilities:
- file read/write scope (repo/path globs, project baseDir, out-files only, etc.)
- git op scope (allowed remotes, allowed branch prefixes, whether merge is allowed, whether push is allowed)
- LLM scope (allowed models, max tokens, cost limits, whether network is allowed)
- tool/function scope (what tools, what args, what sandbox constraints)

Tokens should be **presented on each delegated request**, and enforced locally by the node that would execute the action.

#### Capability encoding + signing (candidate direction)

`~/lab/grid-poc/x/descriptors/` is a useful prototype touchpoint here: it explores a CBOR array message plus a CWT-like payload and a COSE-style signature. For Storm grid-node mode, treat this as inspiration, not a final wire format:

- If we adopt CWT/COSE patterns for capabilities, use **integer-keyed** CWT claims (RFC 8392) and define exactly what bytes are signed (protocol-defined).
- When signatures/hashes matter (capability IDs, audit proofs), require **deterministic CBOR** encoding (dCBOR-like) so different implementations produce identical bytes.
- Keep the **pCID-first envelope** as the long-lived invariant; capability token structures should live inside protocol payloads, not as ad-hoc header fields.

## pCID-first message model (compatibility path)

PromiseGrid’s stable invariant is: messages are CBOR arrays beginning with `pCID` (protocol CID), allowing routers to forward/store messages even if they don’t understand the protocol.

Storm can evolve toward this without breaking existing clients by layering:

1. **Keep existing Storm HTTP+WS JSON protocol** for browser + CLI clients.
2. Add **node-to-node transport** that carries pCID-first messages (grid envelope).
3. Define a minimal Storm grid protocol (or a “JSON wrapper” pCID) so nodes can forward Storm requests/responses and later refine to richer protocols without changing transports.

This lets us ship “grid-like” behavior early while keeping the UI stable.

## Tool calls (descriptor-derived)

The `x/descriptors/` prototype demonstrates a simple “executable descriptor” (CBOR-encoded struct containing binary bytes) and Linux `memfd_create` execution. The useful extraction for Storm is:

- A **descriptor** can name a tool, declare metadata, and reference/contain its bytes; this is one way to make “tool calls” transmissible and cacheable across nodes.
- Execution-from-memory is **not** isolation; it must sit behind capability checks and (eventually) a real sandbox boundary (WASM/container/VM), with full provenance/audit recording.

Track the concrete “derive a tool-call protocol from descriptors” work in TODO `027-descriptors-into-storm.md`.

## Multi-node git coordination (options)

We need a way to integrate work performed on remote nodes back into the user’s repo, with review gates (TODO `014-change-review-gate.md`) and predictable provenance.

### Option A: Patch-first (lowest coupling)

- Worker node returns a patch/diff + metadata (files touched, base commit hash, co-authors).
- Shell/gateway node applies patch locally (review gate), then commits/pushes.
- Pros: minimal remote git complexity; keeps write authority local.
- Cons: remote node still needs repo context to compute diffs; conflicts surface at apply time.

### Option B: Branch-first (mob / “remote branch”)

- Worker node operates on its own clone/worktree, creates a branch, commits, pushes.
- Shell/gateway node fetches, diffs, merges (or uses scenario-tree style branches).
- Pros: uses git as the transport for file changes; easy to inspect/rewind.
- Cons: requires remotes/credentials on worker; needs explicit branch naming + push policy.

### Option C: Consensus-merge (mob-consensus style)

Use a scripted/structured merge workflow where integrating changes includes:
- generating `Co-authored-by:` lines from commit history
- manual merge (or merge-with-tool) + explicit diff review prior to commit

This resembles `~/core/bin/mob-consensus`: merge `OTHER_BRANCH` onto current branch, review with difftool, commit with co-author trailers, and push.

## Architectural changes (minimal-refactor path)

### 1) Introduce an internal “executor” interface

Refactor the daemon’s “do the work” code to target an interface:
- `LocalExecutor`: current behavior (do LLM call, extract files, write outputs)
- `RemoteExecutor`: delegates to another node and returns a normalized result

The rest of Storm (HTTP handlers, WS broadcasts, approval flows) should depend on the executor, not on direct filesystem/LLM implementations.

### 2) Add a node identity + peer configuration

Add configuration for:
- node ID / name
- peer list (addresses + transport)
- role flags (gateway/worker/relay)
- capabilities issued/accepted (or verification keys)

### 3) Add a new subcommand (or extend `serve`)

Two viable approaches:

- **Add `storm node ...`**:
  - `storm node serve` (HTTP gateway + node transport)
  - `storm node worker` (no HTTP, accepts delegated tasks)
  - `storm node relay` (forward/store-and-forward only)
- **Extend `storm serve`** with `--grid` flags:
  - keeps today’s UX, turns on node transport + delegation

Prefer the approach that keeps `storm serve` unchanged by default, and makes grid mode opt-in.

### 4) Map Storm operations to delegated “tasks”

Define task types (even if initially only in-process structs):
- `QueryTask` (LLM call)
- `ExtractTask` (parse/propose file changes)
- `ApplyTask` (write approved changes)
- `GitTask` (branch/merge/commit/push)

Remote execution returns:
- response text
- proposed file edits (patches or file blobs)
- provenance (node ID, capabilities presented, timestamps, pCID envelope ids)

### 5) Add auditing/provenance plumbing early

Even before full PromiseGrid, store enough to debug:
- “who ran what where” (node id)
- “under what capability” (token id / claims)
- inputs (query id, base commit)
- outputs (patch hash / commit hash)

This will make later PromiseGrid integration less painful.

## Phases (staged delivery)

- [ ] 025.1 Define “grid node mode” UX (subcommand + flags) and role behaviors.
- [ ] 025.2 Add executor abstraction and keep `storm serve` behavior identical by default.
- [ ] 025.3 Implement node-to-node delegation for LLM calls only (no remote FS writes), return response + patch proposal.
- [ ] 025.4 Add capability token checks for delegated requests (start with file scopes + model scopes).
- [ ] 025.5 Implement one git integration path (pick A/B/C above) and document the workflow.
- [ ] 025.6 Add relay-only forwarding (store-and-forward) for NAT/firewall traversal.
- [ ] 025.7 Convert node-to-node messages to pCID-first CBOR envelope (or a pCID-wrapped JSON bridge), keeping UI protocol stable.
- [ ] 025.8 Integrate review gate (TODO `014-change-review-gate.md`) so remote changes are always diff/approve/apply/commit.
- [ ] 025.9 Add tool/function call delegation (capabilities + sandbox contract; see TODO `027-descriptors-into-storm.md`).

## Open questions

- Where should project state live in grid mode: only on the gateway node, or replicated per worker?
- How do we express “this node may touch this repo” without leaking unexpected files across trust boundaries?
- Should “git ops” be treated as just another tool/function call under capabilities, or modeled explicitly?
- How do we reconcile Storm’s unexpected-files workflow with remote patch proposals (who approves what, and where)?
