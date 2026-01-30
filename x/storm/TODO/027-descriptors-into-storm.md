# 027 - Derive tool descriptors / tool calls from `grid-poc/x/descriptors/`

Storm grid-node mode (TODO `025-promisegrid-node.md`) needs **tool/function calls** with strong capability gating and good provenance. The `~/lab/grid-poc/x/descriptors/` prototype explores two relevant building blocks:

- **CBOR envelope sketch**: a fixed-size CBOR array message struct (prototype is “5-element” with a `"grid"` string, CWT-like payload, and COSE-like signature).
- **Executable descriptors**: a CBOR-encoded struct that embeds an executable (or script), and a Linux-only execution path via `memfd_create` (`/proc/self/fd/<n>`), i.e. “run bytes without writing a temp file”.

This TODO tracks how Storm should integrate or *derive from* those ideas while staying aligned with PromiseGrid’s current pCID-first envelope and Storm’s safety goals.

## Key takeaways from `x/descriptors/`

- Useful:
  - “Descriptor” as a first-class object: name + metadata + bytes/reference → cacheable artifact.
  - “Execute-from-memory” is a convenient delivery mechanism for worker nodes (no temp files), and suggests an implementation path for tool calls.
- Mismatches / cautions:
  - PromiseGrid’s current invariant is **pCID-first** `grid([pCID,payload,signature])`; the descriptors prototype uses a different “5-element” layout with a `"grid"` string field.
  - CWT standard claims are **integer keyed** (RFC 8392); the prototype uses string keys in the payload map.
  - `memfd_create` is **not** a sandbox. It must be wrapped in explicit capability checks and (eventually) a real isolation boundary (WASM/container/VM), with auditing.

## Goal

Define and prototype a Storm “tool call” mechanism that:

- can run locally in today’s daemon (to ship value early)
- can later be delegated to worker nodes (PromiseGrid grid-node mode)
- is capability-token authorized (what tool, what args, what files, what network)
- is auditable (who/where/when/under what capability; deterministic IDs for replay/caching)
- integrates with the review gate (TODO `014-change-review-gate.md`) for any file outputs

## Design options

### Option A: Named tools only (fastest)

Tool call references an allowed tool name (`git`, `go`, etc.) and args; no shipped binaries.
- Pros: simplest; avoids “shipping executables” security questions.
- Cons: depends on worker node images being consistent; harder to reproduce exactly.

### Option B: Descriptor ships bytes (closest to `x/descriptors/`)

Tool call payload includes (or references by CID) an executable/script blob; worker runs it via `memfd_create` (Linux) or temp file fallback.
- Pros: reproducible; cacheable by hash; aligns with content-addressed artifacts.
- Cons: high risk without strong sandboxing; cross-platform issues; needs a blob transport (CAR/IPLD or similar).

### Option C: WASM/WASI descriptors (preferred long-term sandbox story)

Descriptor references a WASM module (bytes by CID); worker runs under WASI with an explicit filesystem and network capability model.
- Pros: portable; clearer sandbox boundary; fits “capability syscall” thinking.
- Cons: requires a WASM runtime and toolchain strategy.

### Option D: Container image descriptors (heavyweight but familiar)

Descriptor references an OCI image digest; worker runs in a container with explicit mounts and limits.
- Pros: integrates with existing ops practices.
- Cons: heavy for small tool calls; more infrastructure.

## Protocol sketch (Storm × PromiseGrid alignment)

- Use PromiseGrid’s **pCID-first envelope** for node-to-node messages.
- Define a Storm tool-call protocol (example name only): `storm.toolcall.v1` with payload fields like:
  - `capabilityToken` (presented-by caller; verified by executor)
  - `toolRef` (name or CID)
  - `args`, `env`, `cwd`
  - declared `reads`/`writes` scopes (for enforcement + audit)
  - result: `exitCode`, `stdoutCID`, `stderrCID`, `artifacts` (CIDs), plus structured metadata
- Signing/canonicalization: require deterministic encoding of the signed portion (protocol-defined).

## Milestones

- [ ] 027.1 Inventory Storm tool-call needs (git ops, diffs, formatters, tests, builds) and rank by value/risk.
- [ ] 027.2 Pick an initial implementation option (A/B/C/D) and document why.
- [ ] 027.3 Define a minimal capability model for tool calls (allowlist tool+args; FS scope; net scope; resource limits).
- [ ] 027.4 Define the message/protocol shape (pCID, payload, signature rules, deterministic encoding expectations).
- [ ] 027.5 Prototype local-only tool calls inside Storm (no delegation) behind a feature flag, with audit logging.
- [ ] 027.6 Integrate tool outputs with Storm review gate (`diff/approve/apply/commit`) for any file modifications.
- [ ] 027.7 Add a worker-node executor path for delegated tool calls (start with relay-free explicit peers).

