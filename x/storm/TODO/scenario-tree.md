# Scenario Branches: Using Git + Codex as a “Scenario Tree” Tool

This repo can use git branches as “scenarios” (alternative designs/approaches) and use Codex (or another LLM) to compare them and evolve a scenario tree as conditions change. The goal is decision support (compare tradeoffs, preserve options, update assumptions), not necessarily merging everything into `main`.

## Branch Naming

- Reference branch: `main` (or `ref/<topic>`), used as a common baseline for diffs.
- Scenario branches: `scenario/<topic>/<variant>` (e.g., `scenario/kb-storage/bbolt`, `scenario/kb-storage/json`).
- Optional synthesis branch (still “just another scenario”): `scenario/<topic>/synthesis/<date>` (e.g., `scenario/kb-storage/synthesis/2026-01-12`).

## Human Workflow (Recommended)

1. Pick a reference point (often `main` at a specific commit SHA).
2. Create scenario branches from that point:
   - `git checkout <ref>`
   - `git checkout -b scenario/<topic>/<variant>`
3. Make changes normally; keep each scenario focused on one idea/assumption set.
4. As conditions change, update the tree:
   - For “shared early-stage” decisions, implement once on a shared branch and rebase/merge descendants onto it.
   - For hypothesis changes, update only the affected scenario branches.
   - Snapshot stable points with tags (or “snapshot/*” branches) before large rewrites if you want an audit trail.
5. Use Codex to compare/contrast scenarios and propose updates (see next section).

Tip: `git worktree` is useful for keeping multiple scenarios checked out at once:
- `git worktree add ../wt-a scenario/<topic>/a`
- `git worktree add ../wt-b scenario/<topic>/b`

## Instructions to Codex/LLMs (Agent Checklist)

When asked to reconcile scenarios, do this in order:

1. **Confirm inputs**
   - Identify: reference branch/commit, scenario branches to compare, and whether to (a) update existing scenarios, (b) create a new synthesis scenario, or (c) do both.
2. **Gather evidence (don’t guess)**
   - Inspect history: `git log --oneline --decorate --graph --all -n 50`
   - Find merge-base: `git merge-base <ref> <scenarioA> <scenarioB>`
   - Summarize changes per scenario:
     - `git diff --stat <ref>..<scenarioA>` and `git diff --stat <ref>..<scenarioB>`
     - `git diff --name-status <ref>..<scenarioA>` and `git diff --name-status <ref>..<scenarioB>`
   - Compare intent of commit series (best signal): `git range-diff <base>..<scenarioA> <base>..<scenarioB>`
3. **Compare/contrast (semantic, not just textual)**
   - Identify: API differences, data model changes, invariants, tests added/changed, and user-visible behavior.
   - Call out conflicts explicitly and propose decision points.
4. **Propose a consensus plan before editing**
   - List which parts to take from each scenario and what needs rework to unify them.
   - Highlight any “must decide” tradeoffs (performance vs safety, DX vs compatibility, etc.).
5. **Implement intentionally**
   - If you’re creating a synthesis scenario: `git checkout -b scenario/<topic>/synthesis/<date> <ref>`
   - Prefer *intentional assembly*:
     - Cherry-pick selected commits: `git cherry-pick <sha>...`
     - Or re-implement key changes directly when histories diverge.
   - For decisions that should apply across multiple scenarios (“shared early-stage”): implement once on a shared branch, then rebase/merge descendants.
   - Use `git merge` as scaffolding only when helpful, and resolve conflicts intentionally (don’t assume a mechanical merge is the desired outcome).
6. **Validate**
   - Run the closest test suite(s) for touched areas; expand to broader tests if needed.
7. **Document + commit**
   - Use `git diff` as the source of truth for the commit message summary.
   - Record which decisions are shared vs scenario-specific, and what new information triggered the change.

## Guardrails

- Treat LLM output as a drafting tool: always validate with tests and review diffs.
- Keep scenarios usable over time: snapshot before big rewrites if you need historical comparability.
- Avoid “branch churn” that destroys the tree: prefer additive branches/tags over deleting history.
