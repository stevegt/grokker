# LLM-Assisted GA Over Scenario Branches

This document proposes a Genetic Algorithm (GA) layer on top of the branch-based scenario tree approach described in `x/storm/TODO/scenario-tree.md`.

Instead of manually authoring each scenario branch, we maintain a **population** of scenario branches and use LLMs to perform **mutation**, **crossover**, and **repair** operations. We then compute **fitness scores** by running repo tests and/or judging against requirements documents, and we incorporate a **human audit loop** as a second fitness signal.

The goal is to continuously evolve a set of viable, inspectable “candidate futures” while preserving provenance and enabling deliberate decision-making.

## Core Idea

- Each **individual** = a git branch representing a scenario state (a full working tree).
- The **genome** = the branch’s diff relative to a declared base node (typically a scenario node branch).
- Reproduction uses git’s diff/merge machinery plus LLM assistance:
  - use `git diff`/`range-diff` for genome extraction and comparison,
  - use `git apply`/`cherry-pick`/`merge` as crossover primitives,
  - use an LLM to synthesize patches or resolve conflicts when mechanical merges fail.
- Fitness is multi-dimensional:
  - automated tests / builds,
  - requirement satisfaction (LLM-as-judge, grounded in evidence),
  - optional performance and static-analysis signals,
  - periodic human review scores.

## How This Fits the Scenario Tree Model

In ops-research terms, a scenario tree has:
- **chance events** (probabilistic),
- **decisions** (chosen actions),
- node metrics and rolling forecasts.

This GA layer is best viewed as an **idea generator / policy search** tool attached to one or more *decision points* in the scenario tree:
- A scenario node (branch path `scenario/<event1>/<event2>/...`) defines the “world state”.
- The GA produces multiple “decision variants” (individual branches) under that node.
- Canonical facts, requirements, and probability/metric ledgers remain on `main` (per `scenario-tree.md`).

## Naming and Branch Topology

### Base scenario nodes

Scenario-tree branches remain event-path named:
- `scenario/<event1>/<event2>/...`

### GA individuals (recommended convention)

Keep GA-generated individuals as *children* of a base node so they’re easy to list and prune:

- `scenario/<event-path>/ga/<runID>/<gen>/<indID>`

Example:
- Base: `scenario/reg-change/vendor-delay`
- Individual: `scenario/reg-change/vendor-delay/ga/r2026-01-13/g003/i0017`

Notes:
- Branch names encode “where” (base node prefix) and “when” (run/generation), not semantics.
- Lineage (parents, operator, prompts) is recorded in a ledger on `main`, not in the branch name.

### Avoiding multi-parent commits (recommended)

Pure git merges create multi-parent commits, which complicates “event-path = single-parent lineage” assumptions.

Recommendation for GA crossover:
- Materialize each child as a **single-parent** commit sequence based on a chosen primary parent.
- Record the other parent(s) in the `main` ledger as conceptual parents.

This preserves linear branch history while still doing crossover logically.

## Main Ledger for GA Runs (Canonical Metadata)

Per `scenario-tree.md`, canonical, shared, evolving metadata should live on `main` in an append-only ledger, with “last record wins” for current values.

For GA, add an append-only GA ledger file (format examples):
- `scenario/ga/ledger.jsonl` (recommended for append-only updates)
- or `scenario/ga/ledger.yml` (YAML stream)

Suggested record types:

1. **Run config** (one per GA run):
   - `run_id`, `base_node`, `created_at`, `budget`, `population_size`, `mutation_rate`, `crossover_rate`, `models`, `eval_cmds`, `requirements_refs`, etc.
2. **Individual creation**:
   - `ind_id`, `branch`, `gen`, `parents[]`, `operator` (`mutate|crossover|repair|seed`), `seed`, `prompt_ref`, `patch_hash`.
3. **Evaluation result** (append-many per individual):
   - `ind_id`, `branch`, `ts`, `kind` (`test|req_judge|lint|perf|human_audit`), `metrics`, `artifacts`, `status`.
4. **Selection/survivor decision**:
   - `gen`, `survivors[]`, `reasons`, `weights`, `notes`.

The ledger is the canonical “memory” of the GA, while branches are the canonical artifact states.

## Genome Representation Using Git

For an individual branch `B` with base node branch `S`:

- Genome as patch: `git diff S..B`
- Genome as commit series: `git log --oneline S..B`
- Genome comparison between two individuals: `git range-diff S..B1 S..B2`

This allows:
- deduplication (tree hash or diff hash),
- diversity scoring (diff similarity),
- targeted crossover (hunk-level selection),
- constrained mutation (patch edits only).

## Operators

All operators should be **budgeted** and **bounded** (max files changed, max diff size) and should produce **auditable patches**.

### 1) Mutation

Input:
- base node `S`, parent branch `P`, requirements references, current failing tests (if any), and optional mutation “theme”.

Process:
1. Collect context:
   - `git diff S..P` (what makes this individual unique)
   - `git status` (clean working tree)
   - optionally, small file excerpts (not entire repo)
2. Ask LLM to produce a patch:
   - “make small improvement toward requirement X”
   - “reduce complexity / improve testability”
   - “fix failing test output”
3. Apply patch onto a worktree for the child branch.
4. Optionally run a short “repair” pass if patch doesn’t apply or doesn’t compile.

Hard limits (example):
- max 5 files changed
- max 200 diff lines
- forbid dependency additions unless explicitly allowed

### 2) Crossover (breeding)

Parents: `P1`, `P2` (both descendants of the same base node `S`).

Goal: construct child `C` that combines desirable traits.

Crossover strategies (progressive complexity):

**A. Patch union with conflict detection**
- Compute `d1 = git diff S..P1`, `d2 = git diff S..P2`
- Try to apply both patches onto `S` in a deterministic order.
- If conflicts:
  - feed conflict hunks + both diffs to LLM and request a resolved patch.

**B. File-level crossover**
- For each touched file, choose source from P1 or P2 using:
  - fitness attribution (“this file contributes most to passing tests”),
  - novelty/diversity considerations,
  - LLM recommendation (“prefer simpler implementation”).
- Then run repair.

**C. Semantic crossover**
- Provide both parents’ diffs + summaries + fitness evidence.
- Ask LLM to synthesize a coherent child, explicitly resolving design mismatches.

Use git as a primitive:
- `git merge --no-commit --no-ff P2` in a worktree based on `P1` (or vice-versa)
- If conflicts appear, capture conflict markers and ask LLM to resolve.

Recommendation: even if you use `git merge` as scaffolding, **finish with a single-parent history** by squashing the result into a normal commit on top of `P1` (record `P2` as conceptual parent in the ledger).

### 3) Repair (test-guided)

Triggered when:
- build fails
- tests fail
- format/lint fails

Process:
1. Run evaluation; collect failing output.
2. Provide the failing output + minimal relevant code context to LLM.
3. Ask for minimal patch to fix failures.
4. Repeat up to N iterations, then discard or mark as low-fitness.

## Evaluation and Fitness

Fitness should be a vector, not a scalar, so you can evolve tradeoffs and avoid over-optimizing a single metric.

### Automated evaluation stages (tiered)

Cheap → expensive:

1. **Structural checks**
   - patch applies cleanly
   - `go test` compile-only or `go test ./...` for touched packages
2. **Repo test suites**
   - for storm: `make test` in `x/storm` (per repo guidance)
3. **Requirements judge**
   - evidence-first: feed actual outputs, not guesses
4. **Perf/benchmark** (optional)
5. **Static analysis** (optional)

### Requirements judging (LLM-as-judge, evidence-first)

To avoid hallucinated scoring:
- The judge should see:
  - requirement text (versioned on `main`)
  - commands run + outputs
  - key diffs (or summary)
- The judge output should be structured (JSON) with:
  - score per requirement
  - “evidence pointers” (file paths, test names, output snippets)
  - uncertainty flags

### Human audit loop

Periodic sampling to prevent “reward hacking” and capture qualitative constraints:
- Always review:
  - top-k by fitness
  - high-novelty outliers
  - random sample
- Human inputs:
  - accept/reject
  - scalar rating
  - tags (unsafe, unmaintainable, violates intent, etc.)
- These become additional ledger records and influence selection weights.

## Selection and Diversity

Selection should balance:
- exploitation (keep good solutions)
- exploration (maintain diversity)

Common approach:
- elitism: keep top N
- tournament selection for parents
- diversity penalty:
  - diff similarity (e.g., Jaccard over changed file sets + hunk hashes)
  - tree-hash uniqueness (skip exact duplicates)
- optional clustering (embeddings over diffs/commit messages) to preserve multiple “species”

## Orchestration: Making It Run

### Worktrees for parallelism

Use `git worktree` to avoid constant checkout churn and to run tests in parallel:
- maintain a pool of worktrees under `./.worktrees/ga/<id>`
- each worker:
  - checks out an individual branch
  - applies patches / merges
  - runs evaluation commands
  - writes artifacts to an artifact store

### Budgeting and caching

- Cache evaluation results keyed by commit SHA:
  - if branch tip unchanged, don’t re-run full tests
- Hard budgets:
  - max LLM tokens/calls per generation
  - max CPU minutes per generation
  - max concurrent test runs
- Tiered evaluation prevents wasting time on obviously broken individuals.

## Artifacts, Provenance, and Auditing

For each operator/evaluation, store:
- patch(s) applied
- command logs, exit codes, durations
- model name, prompt template version, temperature, seed
- parent references (conceptual parents, even if git history is linear)

Artifact storage options:
- filesystem under `scenario/ga/artifacts/<run>/<ind>/<ts>/...` (simple)
- bbolt KV store with content-addressed blobs (more scalable)

## Safety and Guardrails

- Run evaluations in a sandbox:
  - no network by default
  - no secrets in environment
  - resource limits
- Constrain LLM edits:
  - patch-only output
  - path allowlist
  - size limits
- Make “promotion” explicit:
  - GA branches are candidates; canonical changes to `main` require a deliberate action and review.

## Suggested MVP Plan

1. **Define interfaces and config**
   - base node branch, population size, generation count
   - mutation-only to start
   - evaluation command: `make test` in `x/storm`
2. **Implement branch + worktree management**
   - create `scenario/<base>/ga/<run>/g000/i0000...` from base
3. **Implement mutation operator**
   - LLM patch generator with tight limits
4. **Implement evaluation harness**
   - run tests, capture logs, write ledger records on `main`
5. **Add crossover**
   - start with patch union; add LLM conflict resolution
6. **Add human review queue**
   - minimal UI or CLI that prints top-k and opens diffs for review

## Open Questions

- What is the authoritative requirements representation (docs, tests, both)?
- Where should GA orchestration live (storm daemon, separate CLI, CI job)?
- How do we prevent “branch explosion” over long runs (retention policy, archiving, pruning)?
- Do we want multi-parent commits for traceability, or linear commits + ledger lineage for simplicity?
- How should fitness weighting be tuned, and how do humans override it?

