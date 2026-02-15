# 029 - Grid developer guide (0.1 → 1.0)

`~/lab/cswg/coordination/timeline.md` calls for a “grid dev guide” deliverable:
- **Mar**: grid dev guide **0.1** (alongside Storm multi-node)
- **Apr**: grid dev guide **0.9** + SDK/libraries
- **May**: refactor Storm into grid/app layers + grid dev guide **1.0**

This TODO defines the scope, structure, and staged milestones for that developer guide, and how it stays aligned with Storm’s grid evolution work (TODO `025-promisegrid-node.md`, TODO `027-descriptors-into-storm.md`).

## Purpose

Produce a developer-facing guide that:
- defines terms and invariants (node/role/capability/pCID envelope)
- documents the “compatibility path” from today’s Storm to a grid model
- specifies message envelopes/transports and capability enforcement points
- helps contributors build against the emerging grid layer without guessing

## Key scope

- Node roles are composable: a node may run hub/workspace/worker/relay/shell roles (see TODO `025-promisegrid-node.md`).
- Capability tokens gate all privileged actions (files, tools/functions, LLM calls, git ops).
- Message evolution targets a **pCID-first envelope** (PromiseGrid) while keeping UI protocols stable during transition.
- Tool calls are expected to become first-class and transmissible (see TODO `027-descriptors-into-storm.md`).

## Deliverables

### v0.1 (Mar)

“Enough to build the first multi-node prototype safely.”
- Terminology + roles + trust boundaries.
- Minimal architecture diagrams: hub+workspace, relay traversal, worker delegation.
- Protocol sketch: existing UI JSON + a node↔node tool-call transport + the planned pCID envelope.
- Capability token model: what is enforced where, what is logged/audited.
- One worked example flow: “hub routes query → workspace executes → returns response + patch proposal”.

### v0.9 (Apr)

“Almost complete; ready for SDK/libraries and broader contributors.”
- Concrete message envelope definition (or the JSON-bridged pCID wrapper) with examples.
- Capability token format guidance (claims, signing, rotation/revocation, audit ids).
- SDK/libraries overview: minimal client, node router, capability verification, provenance log writer.
- Testing guidance: fixtures for node-to-node messages and capability enforcement.

### v1.0 (May)

“Stable doc aligned with a grid/app split in Storm.”
- Clear layering: grid kernel/transport/capabilities/audit vs app surfaces (Storm web UI, CLI, workspace tools).
- Deployment patterns: local-only, hub+workspace split, multi-workspace, multi-worker, relays.
- Git coordination patterns and recommended baseline workflow.
- Compatibility commitments: what is stable, what may change, versioning expectations.

## Subtasks

- [ ] 029.1 Decide canonical home for the guide (likely `ciwg/storm` repo under `v5/` docs)
- [ ] 029.2 Draft table-of-contents and cross-link to TODOs (`025`, `027`, `015`, `014`)
- [ ] 029.3 Write v0.1: roles/trust boundaries + one end-to-end delegated flow
- [ ] 029.4 Add minimal diagrams (topologies + message paths)
- [ ] 029.5 Write v0.9: message envelope + capability token spec + SDK boundaries
- [ ] 029.6 Write v1.0: grid/app layering + deployments + compatibility notes
- [ ] 029.7 Add a “keep in sync” checklist tied to TODO `025` phases

## References

- Timeline: `~/lab/cswg/coordination/timeline.md`
- PromiseGrid overview: `~/lab/grid-poc/llms-full.txt`
- Storm node mode plan: TODO `025-promisegrid-node.md`
- Tool descriptors/tool calls: TODO `027-descriptors-into-storm.md`

