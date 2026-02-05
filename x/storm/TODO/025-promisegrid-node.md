# 025 - PromiseGrid node mode (delegation + multi-node git)

TODO `015-interactive-cli.md` highlights a deeper architectural mismatch: Codex-like terminal UX assumes “the client can just do things locally”, while Storm’s model is “the client talks to a daemon which mediates LLM + file access”.

PromiseGrid’s model reframes this: “servers” are just other nodes you can delegate to, and a node can be local (same machine), a relay (NAT/firewall traversal), an HTTP gateway (browser clients), or a worker (executes delegated tasks). Capability tokens authorize what a node may do (files, tools/functions, LLM calls, git ops).

In grid terms, a node is a node: the same node can serve any combination of roles (hub, workspace/repo, worker, relay, shell), depending on what services it runs and what capabilities it holds.

Operational reality: today we run Storm on a workstation as a local user and (when needed) expose it via SSH tunneling; we would not run a privileged Storm daemon on a public server as a generic user. Grid-node mode should make this explicit by splitting *roles* across nodes: e.g. run hub/routing/UI roles on an unprivileged public node and run workspace (repo) + LLM + tool execution roles on a local node, connected via capability-gated tool calls.

Background:
- PromiseGrid overview: `~/lab/grid-poc/llms-full.txt` (pCID-first message envelope; capability-as-promise; kernel-mediated calls)
- PromiseGrid descriptors prototype: `~/lab/grid-poc/x/descriptors/` (CBOR message + executable descriptor + memfd exec; useful for capabilities/tool calls)
- Tool call protocol candidates: MCP/ACP (or PromiseGrid-native) for hub↔workspace tool calls (bridge to pCID-first over time)
- Git coordination inspiration: https://mob.sh/
- Local alternative script: `~/core/bin/mob-consensus`

## Goal

Evolve Storm so it can operate as one or more **grid nodes** (hub, workspace, worker), where:
- the **workspace** node (local) owns the repo and runs the LLM and tools
- the **hub** node multiplexes GUI/TUI clients and routes capability-gated tool calls to workspace/worker nodes
- nodes can optionally offer LLM/tool services to other nodes under capabilities
- results are auditable messages with clear provenance
- git operations can be coordinated across nodes when needed (mob-style or consensus-merge style)
- preserve current single-daemon UX (`storm serve`, web UI, existing CLI) as a default for local-only use

## Non-goals (initially)

- Replacing PromiseGrid with Storm-specific protocols long-term.
- Perfect multi-tenant security (we need staged capabilities + auditing first).
- Solving every NAT traversal/topology problem in v1; start with explicit peers.

## Current Storm shape (baseline)

- `storm serve` starts an HTTP+WS daemon that owns project state (baseDir, authorized files) and performs LLM calls + filesystem writes.
- In practice, the daemon is often reached from outside the workstation via an SSH tunnel to a public port; the daemon still runs locally as a user with repo access.
- Clients (web, CLI) are thin: they send queries, selections, file lists, and approvals; server enforces policy and writes files.

Grid-node mode should keep this working unchanged when “grid” is off.

## Preferred initial topology: hub + workspace (LLM + repo local)

This is a *deployment pattern*, not a different kind of node: two nodes run the same software but with different role+capability sets.

- **Node acting in hub role (public/unprivileged)**: runs the web UI and any shared collaboration state; multiplexes many GUI/TUI clients; routes messages/tool calls; does not need a repo checkout or LLM keys.
- **Node acting in workspace role (local/privileged)**: runs as the user who owns the repo; holds repo credentials and file/tool capabilities; runs the LLM locally; executes tool calls and file writes; enforces capabilities.
- Workspace nodes keep an outbound connection to a hub/relay to traverse NAT/firewalls and avoid exposing a privileged local daemon port directly.
- Any node may also serve additional roles (e.g. a workstation can run hub+workspace+shell in one process for local-only use).

## What “grid node” means for Storm

### Roles (a node can run any combination)

- **Shell role**: interactive user entrypoint (TODO `015-interactive-cli.md`). Typically connects to a hub; does not require repo access.
- **Hub role**: HTTP gateway + router/relay for GUI/TUI clients; routes tool calls to workspace/worker roles; no repo/LLM by default.
- **Workspace role (repo agent)**: owns repo + LLM + tools; executes queries and applies edits under capability gating.
  - XXX pros and cons of workspace role code paths executing in-process in shell role node vs delegating to a local workspace role node?
- **Worker role** (optional): executes delegated tasks for other nodes; may offer LLM/tool services, and may or may not have a repo clone depending on capability.
- **Relay role**: forwards/store-and-forwards messages to bridge NAT/firewalls (no local execution required).
  - XXX what's the difference between Hub and Relay?

### Delegation primitives (what can be delegated)

- **LLM execution**: by default runs on a node acting in workspace role (typically local). Nodes may optionally offer “LLM as a service” to other nodes under capability tokens.
- **Repo/file operations**: run on the node in workspace role that owns the repo checkout. Remote nodes generally work via patch proposals or a declared snapshot/overlay unless explicitly authorized to clone/push.
- **Git operations**: performed by the workspace-role node owning the repo-of-record; other nodes can propose patches/branches for review+merge.
- **Tool/function calls** (future): executed on the workspace-role node by default; optionally on remote workers when capability-granted (see TODO `027-descriptors-into-storm.md`).
- **Routing/multiplex**: nodes in hub/relay roles route messages between clients and executor nodes.

### Capability tokens (authorization)

Storm already has “authorized files” as an allowlist. Grid mode generalizes this into capabilities:
- file read/write scope (repo/path globs, project baseDir, out-files only, etc.)
- git op scope (allowed remotes, allowed branch prefixes, whether merge is allowed, whether push is allowed)
- LLM scope (allowed models, max tokens, cost limits, whether network is allowed)
- tool/function scope (what tools, what args, what sandbox constraints)

Tokens should be **presented on each delegated request**, and enforced locally by the node that would execute the action.

In the hub+workspace deployment pattern, enforcement must happen at the executor node (workspace/worker role). Treat routing-only nodes as untrusted: do not rely on them for access control.

#### Capability encoding + signing (candidate direction)

`~/lab/grid-poc/x/descriptors/` is a useful prototype touchpoint here: it explores a CBOR array message plus a CWT-like payload and a COSE-style signature. For Storm grid-node mode, treat this as inspiration, not a final wire format:

- If we adopt CWT/COSE patterns for capabilities, use **integer-keyed** CWT claims (RFC 8392) and define exactly what bytes are signed (protocol-defined).
- When signatures/hashes matter (capability IDs, audit proofs), require **deterministic CBOR** encoding (dCBOR-like) so different implementations produce identical bytes.
- Keep the **pCID-first envelope** as the long-lived invariant; capability token structures should live inside protocol payloads, not as ad-hoc header fields.

## pCID-first message model (compatibility path)

PromiseGrid’s stable invariant is: messages are CBOR arrays beginning with `pCID` (protocol CID), allowing routers to forward/store messages even if they don’t understand the protocol.

Storm can evolve toward this without breaking existing clients by layering:

1. **Keep Storm’s existing HTTP+WS JSON protocol** between GUI/TUI clients and a node acting in hub role (web UI today; TODO `015-interactive-cli.md` next).
2. Add a **node↔node tool-call transport** (MCP/ACP-like, or PromiseGrid-native) where workspace/worker roles run LLM+repo+tools and return responses/patch proposals; hub role routes only.
  - XXX let's do this soonest
3. Quickly migrate **node-to-node** traffic (hub↔workspace, workspace↔workspace, relay) to a pCID-first CBOR envelope, optionally starting with a pCID that wraps existing JSON tool-call payloads.

This lets us ship “grid-like” behavior early while keeping the UI stable.

## Tool calls (descriptor-derived)

The `~/lab/grid-poc/x/descriptors/` prototype demonstrates a simple “executable descriptor” (CBOR-encoded struct containing binary bytes) and Linux `memfd_create` execution. The useful extraction for Storm is:

- A **descriptor** can name a tool, declare metadata, and reference/contain its bytes; this is one way to make “tool calls” transmissible and cacheable across nodes.
- Execution-from-memory is **not** isolation; it must sit behind capability checks and (eventually) a real sandbox boundary (WASM/container/VM), with full provenance/audit recording.

Track the concrete “derive a tool-call protocol from descriptors” work in TODO `027-descriptors-into-storm.md`.

## Multi-node git coordination (options)

We need a way to integrate work performed on other nodes back into the repo-of-record on the user’s workspace-role node, with review gates (TODO `014-change-review-gate.md`) and predictable provenance.

### Option A: Patch-first (lowest coupling)

- Worker node returns a patch/diff + metadata (files touched, base commit hash, co-authors).
- Workspace node (repo-of-record) applies patch locally (review gate), then commits/pushes.
- Pros: minimal remote git complexity; keeps write authority local.
- Cons: remote node still needs repo context to compute diffs; conflicts surface at apply time.

### Option B: Branch-first (mob / “remote branch”)

- Worker node operates on its own clone/worktree, creates a branch, commits, pushes.
- Workspace node (repo-of-record) fetches, diffs, merges (or uses scenario-tree style branches).
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
- `LocalExecutor`: current behavior (runs on a node with workspace capabilities: do LLM call, extract files, write outputs)
- `RemoteExecutor`: delegates to another node and returns a normalized result (used by a hub routing to a workspace node, or by nodes delegating to other nodes)

The rest of Storm (HTTP handlers, WS broadcasts, approval flows) should depend on the executor, not on direct filesystem/LLM implementations.

### 2) Add a node identity + peer configuration

Add configuration for:
- node ID / name
- peer list (addresses + transport)
- role flags (hub/workspace/worker/relay)
- connection mode (listen vs connect; workspace outbound to hub/relay)
- capabilities issued/accepted (or verification keys)

### 3) Add a new subcommand (or extend `serve`)

Two viable approaches:

- **Add `storm node ...`** where a single process can enable one or more roles:
  - Example flags: `--hub`, `--workspace`, `--worker`, `--relay` (or `--role hub --role workspace ...`)
  - Optional convenience aliases for common single-role defaults (but the underlying model is composable roles).
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

- [ ] 025.1 Define “grid node mode” UX (subcommand + flags) and role behaviors (hub vs workspace).
- [ ] 025.2 Add executor abstraction and keep `storm serve` behavior identical by default.
- [ ] 025.3 Implement hub↔workspace delegation for LLM calls (LLM runs on workspace; hub routes only), return response + patch proposal.
- [ ] 025.4 Add capability token checks for delegated requests (start with file scopes + model scopes).
- [ ] 025.5 Implement one git integration path (pick A/B/C above) and document the workflow.
- [ ] 025.6 Add relay-only forwarding (store-and-forward) for NAT/firewall traversal.
- [ ] 025.7 Convert node-to-node messages to pCID-first CBOR envelope (or a pCID-wrapped JSON bridge), keeping UI protocol stable.
- [ ] 025.8 Integrate review gate (TODO `014-change-review-gate.md`) so remote changes are always diff/approve/apply/commit.
- [ ] 025.9 Add tool/function call delegation (capabilities + sandbox contract; see TODO `027-descriptors-into-storm.md`).

## Open questions

- Where should “project state” live when hub and workspace roles are split: what minimal state is safe to persist on hub-role nodes, vs only on workspace-role nodes?
- How do we scope and minimize file flow (context, patches, artifacts) so the hub doesn’t become a de facto data lake?
- Who mints capability tokens (user, org, node operator), how are they revoked, and how are they presented end-to-end through the hub?
  - XXX follow promise theory -- capabilities are promises made by an
    agent about its own behavior.  nodes cannot make promises about
    other nodes, so can't issue capabilities for other nodes. the node
    executing an action must verify the capability itself.
- Should “git ops” be treated as just another tool/function call under capabilities, or modeled explicitly?
- How do we reconcile Storm’s unexpected-files workflow with patch proposals coming from other nodes (who approves what, and where)?
