# Scenario Trees: Using Git + Codex for Decision Support

Scenario trees in ops research are decision tools: an acyclic state machine where **events** trigger transitions, each event has an associated **probability**, and each resulting **state node** has **metrics**. Git branches can model this well if we treat each branch as a *state node* and each branch-creation as an *event transition*.

This document describes a lightweight convention for using git + Codex (or other LLMs) to:
- keep multiple futures/options alive,
- update scenarios as conditions change,
- compare/contrast scenarios and their metrics,
- create new branches as *new states* (not “merge to main”).

## Concept Mapping (Scenario Tree ↔ Git)

- **`main`**: shared “real world” record (common files, actual event history, background conditions, goals, requirements, and evaluation tooling).
- **State node**: a branch tip (a commit) representing the world after a sequence of events.
- **Event transition**: creating a child branch from a parent node and applying changes that represent the event’s consequences.
- **Acyclic**: keep a single-parent lineage for scenario branches (avoid merges) so the branch name remains a valid “event path”.
- **Probabilities + node metadata**: canonical, shared, evolving records live in an append-only ledger on `main` (don’t bake probabilities into branch names; they change).
- **Metrics**: stored in that same `main` ledger; ideally derived from reproducible commands (tests, benchmarks, model runs, etc.).

## What Goes on `main` vs Scenario Branches

Use `main` as the canonical place for shared facts and shared artifacts, and treat scenario branches as deltas that model *hypothetical* event sequences.

Put on `main`:
- Common files and templates (including evaluation scripts and metric definitions).
- History of actual events (what really happened and when).
- Background conditions, goals, requirements, constraints, and “current assumptions”.
- Canonical scenario ledger: append-only probabilities + metadata + metrics (forecasts and observations in the same structure; latest record for a given branch/event is the current value).

Put on `scenario/...` branches:
- Only the code/config/doc changes that represent the *hypothetical* events in that branch’s event path.
- Avoid canonical probability/metric metadata; keep scenarios focused on “what changes under this event path”.

If you learn new background conditions or requirements, update `main` first, then rebase/branch new scenario nodes from the updated `main` (instead of forking “facts” across scenario branches).

## Branch Naming (Event Paths)

Use the branch name itself as the event path from the root reference point:

- Reference commit/branch: `main` (or a tag like `ref/scenario-root-<date>`)
- Scenario nodes: `scenario/<event1>/<event2>/...`

Example:
- `scenario/reg-change` (state after event “reg-change”)
- `scenario/reg-change/vendor-delay` (state after “reg-change”, then “vendor-delay”)
- `scenario/reg-change/vendor-delay/budget-cut` (further event)

Rule of thumb: each path segment is the *event label* that produced the transition from parent → child.

## Main Ledger (Probabilities + Metrics)

Keep canonical, shared, evolving scenario metadata in a ledger on `main`. The ledger is append-only: each new record updates the “current” probability/metrics for a specific branch/event, but old records remain for history/audit.

Practical format options:
- `scenario/ledger.jsonl` (append-friendly; one JSON object per line)
- `scenario/ledger.yml` as a YAML stream (`---`-separated documents)

Suggested record fields:
- `ts`: timestamp
- `node`: scenario branch name (state node), e.g. `scenario/reg-change/vendor-delay/budget-cut`
- `parent`: optional (can be derived from `node`), but useful for validation and renames
- `event`: last path segment (transition label)
- `p`: probability for the event transition at time `ts` (forecast or observed)
- `kind`: `forecast` or `observed` (keep both in the same ledger)
- `metrics`: anything you care about (cost, latency, risk score, time-to-ship, test/bench results, etc.)
- `notes`: what changed, why, and what evidence supports it

“Last record wins” rule:
- The current probability/metrics for a given `node` (or `parent`+`event`) is taken from the most recent matching record in the ledger.
- When an event actually occurs, append an `observed` record (typically `p: 1.0`) on `main` rather than editing history.

We do not need an explicit `paths:` adjacency structure: the scenario *paths* are the git branch paths (`scenario/<event1>/<event2>/...`). Outgoing transitions are the set of existing child branches under a prefix.

Example (YAML stream, `scenario/ledger.yml`):
```yml
---
ts: 2026-01-12T19:00:00Z
node: scenario/reg-change/vendor-delay/budget-cut
parent: scenario/reg-change/vendor-delay
event: budget-cut
p: 0.3
metrics:
  make_test: pass
  est_cost_usd: 120000
notes: >
  Forecast: budget cut reduces headcount; assume 25% slower delivery.
---
ts: 2026-02-01T02:00:00Z
node: scenario/reg-change/vendor-delay/budget-cut
p: 1.0
notes: "Observed: budget-cut occurred."
```

## Git Notes (Optional; Non-Canonical)

Use git notes for local/ephemeral annotations or intermediate evaluation results you might later promote into the `main` ledger:

- Attach: `git notes add -m "…" <commit>`
- View: `git notes show <commit>` or `git log --show-notes`

Notes are easy to miss in normal review flows and require explicit sharing (`git push origin refs/notes/*`), so treat them as a scratchpad rather than the canonical source of truth.

## Prior Art: `godecide` (YAML Decision Trees)

See `/home/stevegt/lab/godecide` for a prior (non-LLM) scenario tree modeler. It uses a compact YAML schema that maps closely to ops-research scenario trees:

- Each **node** is keyed by name and can include fields like `desc`, `cash`, `days`, `repeat`, `finrate`, `rerate`, `due`.
- Each node can have `paths: { childNodeName: probability }` to define **event transitions** and **probabilities** (that adjacency isn’t needed when the git branch path is the path).
- `paths` keys can contain comma-separated child names (`a,b`) to represent a single transition that yields multiple concurrent children (a “hyperedge”).
- Nodes can also declare `prereqs` for PERT-style dependency/critical-path calculations.

Recommendation: reuse (or extend) the `godecide` *node fields* (`cash`, `days`, `due`, etc.) as metrics in the `main` ledger, while using git branches named `scenario/<event1>/<event2>/...` to define the tree structure.

## Human Workflow (Recommended)

1. Choose a root reference point (often a specific `main` commit):
   - `git rev-parse main` (record the SHA)
   - Optionally tag it: `git tag ref/scenario-root-2026-01-12 <sha>`
2. Create the first event node:
   - `git checkout -b scenario/<event1> <ref>`
   - Make the code/config/doc changes for the hypothetical event; commit on the scenario branch.
   - On `main`, append a ledger record for the new node/event probability + metrics; commit the ledger update.
3. Create child nodes as new events occur:
   - `git checkout -b scenario/<event1>/<event2> scenario/<event1>`
4. When assumptions change, update the impacted node(s) and propagate forward:
   - Prefer *additive* updates (new events / new child nodes).
   - If you must rewrite, tag snapshots first so you can compare old vs new.
   - Update probabilities/metadata by appending new records to the `main` ledger (don’t edit old records).
5. Compare scenarios using git tools (diffs + history):
   - `git diff <scenarioA>..<scenarioB>`
   - `git range-diff <ref>..<scenarioA> <ref>..<scenarioB>`
   - `git show main:scenario/ledger.yml` (or `scenario/ledger.jsonl`) to compare probabilities/metrics for the nodes.

Tip: `git worktree` is useful for evaluating multiple states side-by-side:
- `git worktree add ../wt-a scenario/<pathA>`
- `git worktree add ../wt-b scenario/<pathB>`

## Instructions to Codex/LLMs (Agent Checklist)

When asked to help with a scenario tree, do this in order:

1. **Confirm the modeling intent**
   - Identify: root reference commit, scenario nodes/branches in scope, and what counts as an event vs a decision.
2. **Read the `main` ledger**
   - Summarize the latest (last-record) probability + metrics for each node/event in scope, and call out recent changes over time.
   - If present, treat git notes as local scratch annotations that may be promoted into the ledger.
3. **Gather git evidence**
   - `git log --oneline --decorate --graph --all -n 50`
   - `git diff --name-status <ref>..<scenarioX>`
   - `git range-diff <ref>..<scenarioA> <ref>..<scenarioB>`
4. **Compare/contrast as a decision tool**
   - Highlight: differences in assumptions, probability assignments, and metrics, plus the code deltas that cause them.
   - Call out missing metrics or non-reproducible evaluations.
5. **Propose updates as new events/nodes**
   - Prefer creating a new child node to represent a new event (“conditions changed”), rather than forcing scenarios to converge.
6. **Implement carefully**
   - Make changes on the requested scenario branch(es), then run the relevant evaluation commands.
   - Update probabilities/metadata by appending records to the `main` ledger (and optionally promote useful git notes into the ledger).
7. **Document**
   - Summarize what changed, which event it represents, and how probabilities/metrics were updated.

## Guardrails

- Treat LLM output as a drafting tool: validate via diffs + reproducible metrics.
- Keep scenario history comparable: tag snapshots before big rewrites.
- Avoid merges in scenario branches unless you explicitly want a “multi-parent state” (which breaks the simple event-path naming invariant).
