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
- **Probabilities**: stored in per-node metadata (don’t bake probabilities into branch names; they change).
- **Metrics**: stored in per-node metadata, ideally derived from reproducible commands (tests, benchmarks, model runs, etc.).

## What Goes on `main` vs Scenario Branches

Use `main` as the canonical place for shared facts and shared artifacts, and treat scenario branches as deltas that model *hypothetical* event sequences.

Put on `main`:
- Common files and templates (including evaluation scripts and metric definitions).
- History of actual events (what really happened and when).
- Background conditions, goals, requirements, constraints, and “current assumptions”.

Put on `scenario/...` branches:
- Only the code/config/doc changes that represent the *hypothetical* events in that branch’s event path.
- Per-node `SCENARIO.*` metadata (probabilities + metrics) for that scenario state.

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

## Per-Node Metadata (Probabilities + Metrics)

Add a small metadata file in each scenario branch to record the state, its parent, and its evaluation:

- File name: `SCENARIO.md` (simple) or `SCENARIO.yml` (structured)
- Location: repo root (so it’s easy to `git show branch:SCENARIO.md`)

Suggested fields:
- Parent: parent branch or parent commit SHA
- Event: the last event label (the final path segment)
- Probability: probability of the last event given the parent (chance node), or `1.0` for decision/forced transitions
- Paths: optional outgoing event probabilities, e.g. `paths: { <event>: <prob> }` (matches `godecide`), so each state node can declare its next-event distribution
- Metrics: anything you care about (cost, latency, risk score, time-to-ship, test results, etc.)
- Assumptions/notes: what changed, why, and what evidence supports it

Example (`SCENARIO.yml`):
```yml
parent: scenario/reg-change/vendor-delay
event: budget-cut
probability: 0.3
metrics:
  make_test: pass
  est_cost_usd: 120000
  risk: high
paths:
  hiring-freeze: 0.2
  no-hiring-freeze: 0.8
notes: >
  Budget cut reduces headcount; model assumes 25% slower delivery.
```

## Prior Art: `godecide` (YAML Decision Trees)

See `/home/stevegt/lab/godecide` for a prior (non-LLM) scenario tree modeler. It uses a compact YAML schema that maps closely to ops-research scenario trees:

- Each **node** is keyed by name and can include fields like `desc`, `cash`, `days`, `repeat`, `finrate`, `rerate`, `due`.
- Each node can have `paths: { childNodeName: probability }` to define **event transitions** and **probabilities**.
- `paths` keys can contain comma-separated child names (`a,b`) to represent a single transition that yields multiple concurrent children (a “hyperedge”).
- Nodes can also declare `prereqs` for PERT-style dependency/critical-path calculations.

Recommendation: reuse (or extend) that YAML schema for scenario metadata (probabilities + metrics), while using git branches named `scenario/<event1>/<event2>/...` to anchor each node to an exact code/config state.

## Human Workflow (Recommended)

1. Choose a root reference point (often a specific `main` commit):
   - `git rev-parse main` (record the SHA)
   - Optionally tag it: `git tag ref/scenario-root-2026-01-12 <sha>`
2. Create the first event node:
   - `git checkout -b scenario/<event1> <ref>`
   - Add/update `SCENARIO.md`/`SCENARIO.yml`, then commit.
3. Create child nodes as new events occur:
   - `git checkout -b scenario/<event1>/<event2> scenario/<event1>`
4. When assumptions change, update the impacted node(s) and propagate forward:
   - Prefer *additive* updates (new events / new child nodes).
   - If you must rewrite, tag snapshots first so you can compare old vs new.
5. Compare scenarios using git tools (diffs + history):
   - `git diff <scenarioA>..<scenarioB>`
   - `git range-diff <ref>..<scenarioA> <ref>..<scenarioB>`
   - `git show <scenarioX>:SCENARIO.yml` to compare metrics/probabilities.

Tip: `git worktree` is useful for evaluating multiple states side-by-side:
- `git worktree add ../wt-a scenario/<pathA>`
- `git worktree add ../wt-b scenario/<pathB>`

## Instructions to Codex/LLMs (Agent Checklist)

When asked to help with a scenario tree, do this in order:

1. **Confirm the modeling intent**
   - Identify: root reference commit, scenario nodes/branches in scope, and what counts as an event vs a decision.
2. **Read the node metadata**
   - For each node: read `SCENARIO.md`/`SCENARIO.yml` from that branch (and summarize probabilities + metrics).
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
   - Make changes on the requested scenario branch(es), update `SCENARIO.*`, then run the relevant evaluation commands.
7. **Document**
   - Summarize what changed, which event it represents, and how probabilities/metrics were updated.

## Guardrails

- Treat LLM output as a drafting tool: validate via diffs + reproducible metrics.
- Keep scenario history comparable: tag snapshots before big rewrites.
- Avoid merges in scenario branches unless you explicitly want a “multi-parent state” (which breaks the simple event-path naming invariant).
