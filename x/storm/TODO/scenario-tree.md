# Scenario Branches: Using Git + Codex as a “Scenario Tree” Tool

This repo can use git branches as “scenarios” (alternative designs/approaches) and use Codex (or another LLM) to compare them and build a new “consensus” branch intentionally (instead of doing an automatic `git merge` and hoping conflicts resolve well).

## Branch Naming

- Scenario branches: `scenario/<topic>/<variant>` (e.g., `scenario/kb-storage/bbolt`, `scenario/kb-storage/json`).
- Consensus branch: `consensus/<topic>` (e.g., `consensus/kb-storage`).
- Pick a base branch (often `main`) and treat it as the stable trunk.

## Human Workflow (Recommended)

1. Create scenarios from a known base:
   - `git checkout main`
   - `git pull --ff-only`
   - `git checkout -b scenario/<topic>/<variant>`
2. Make changes normally; keep each scenario focused on one idea.
3. When you want convergence, create a new consensus branch from the chosen base:
   - `git checkout -b consensus/<topic> <base>`
4. Use Codex to compare scenarios and implement the consensus branch (see next section).
5. Run tests, then commit with a clear message explaining what was selected from each scenario.

Tip: `git worktree` is useful for keeping multiple scenarios checked out at once:
- `git worktree add ../wt-a scenario/<topic>/a`
- `git worktree add ../wt-b scenario/<topic>/b`

## Instructions to Codex/LLMs (Agent Checklist)

When asked to reconcile scenarios, do this in order:

1. **Confirm inputs**
   - Identify: base branch, scenario branches, and the target consensus branch name.
2. **Gather evidence (don’t guess)**
   - Inspect history: `git log --oneline --decorate --graph --all -n 50`
   - Find merge-base: `git merge-base <base> <scenarioA> <scenarioB>`
   - Summarize changes per scenario:
     - `git diff --stat <base>..<scenarioA>` and `git diff --stat <base>..<scenarioB>`
     - `git diff --name-status <base>..<scenarioA>` and `git diff --name-status <base>..<scenarioB>`
   - Compare intent of commit series (best signal): `git range-diff <base>..<scenarioA> <base>..<scenarioB>`
3. **Compare/contrast (semantic, not just textual)**
   - Identify: API differences, data model changes, invariants, tests added/changed, and user-visible behavior.
   - Call out conflicts explicitly and propose decision points.
4. **Propose a consensus plan before editing**
   - List which parts to take from each scenario and what needs rework to unify them.
   - Highlight any “must decide” tradeoffs (performance vs safety, DX vs compatibility, etc.).
5. **Implement on a new consensus branch**
   - Create: `git checkout -b consensus/<topic> <base>`
   - Prefer *intentional assembly*:
     - Cherry-pick selected commits: `git cherry-pick <sha>...`
     - Or re-implement key changes directly when histories diverge.
   - If using merge as scaffolding, use it in a controlled way:
     - `git merge --no-commit --no-ff <scenarioA>` then resolve intentionally, commit, repeat for `<scenarioB>`.
6. **Validate**
   - Run the closest test suite(s) for touched areas; expand to broader tests if needed.
7. **Document + commit**
   - Use `git diff` as the source of truth for the commit message summary.
   - Record which scenario decisions were adopted and why.

## Guardrails

- Treat LLM output as a drafting tool: always validate with tests and review diffs.
- Keep scenario branches intact for auditability; don’t rewrite them after review starts.
- Prefer creating a new consensus branch over force-updating an existing shared branch.

