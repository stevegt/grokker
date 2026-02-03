# Merge grok into storm: rename/reshape plan

## Current command surfaces

### v3 grok CLI (Kong)
- Entry point: `v3/cmd/grok/main.go` -> `cli.Cli(os.Args[1:], config)`
- CLI name/description set in `v3/cli/cli.go` (`Name: grokker`), tests call it as `grok`.
- Primary subcommands: `add`, `aidda`, `backup`, `chat`, `commit`, `ctx`, `embed`, `forget`, `init`, `ls`, `model`, `models`, `msg`, `q`, `qc`, `qi`, `qr`, `refresh`, `similarity`, `tc`, `version`.
- DB file: `.grok` in repo root (relative to project, found by walking up).

### x/storm CLI (Cobra)
- Root command: `storm` with `serve`, `project`, `discussion`, `file` subcommands and HTTP API.
- Storm runs a daemon + web UI, multi-project storage, and API endpoints under `/api/...`.

## Target shape (storm-first)

### Suggested command hierarchy
- `storm serve` (existing)
- `storm project ...` (existing)
- `storm discussion ...` (existing)
- `storm file ...` (existing)
- New group for grok commands under storm: `storm kb ...`

Example mapping (storm kb):
- `grok init` -> `storm kb init`
- `grok add` -> `storm kb add`
- `grok forget` -> `storm kb forget`
- `grok ls` -> `storm kb ls`
- `grok chat` -> `storm kb chat`
- `grok q|qc|qi|qr` -> `storm kb q|qc|qi|qr`
- `grok ctx|tc|embed|similarity|refresh` -> `storm kb ctx|tc|embed|similarity|refresh`
- `grok model|models|version|backup` -> `storm kb model|models|version|backup`
- `grok commit` -> `storm kb commit`

Rationale: avoids crowding the `storm` root, keeps KB behavior intact, and makes the CLI hierarchy explicit.

## Migration approach

### CLI integration
- Translate grok CLI from kong to cobra:
  - Recreate the grok command tree under `storm kb` directly in cobra.
  - Pros: single CLI framework, unified help. Cons: larger refactor, touch more tests.

### Versioning strategy (freeze `v3/`, work in `v4/` or `v5/`)

This repo’s `README.md` “Semantic Versioning” policy treats **odd-numbered elements** as unstable pre-releases collecting changes for the **next stable even** number (examples given: `3.X.X` is a pre-release of `4.0.0`). That matters because “merge grok into storm” is likely to cause API/CLI and storage changes that are hard to keep perfectly compatible.

There are two separate choices here:
- **Git branch strategy**: keep `main` moving vs long-lived maintenance branches.
- **Go module strategy**: keep one module vs introduce a new major-version module (`.../v4`, `.../v5`).

#### Option A: Keep doing the work in `v3/` (status quo)

Pros:
- Matches the current “odd major = pre-release of next stable major” story (`v3` → `v4`).
- No repo-wide duplication of code trees; simplest development workflow.
- Easier to keep `grok` CLI compatibility while iterating (wrapper/shims can stay in place).

Cons:
- High churn in the only active module path (`.../v3`) can destabilize downstream users.
- Harder to land big refactors safely without “shadowing” work in parallel.
- If the end state wants a daemon-first architecture, `v3` may accumulate half-integrated layers.

#### Option B: Freeze `v3/` and do the merge work in a new major module (`v4/` or `v5/`)

Interpret “freeze” as: only take small bugfixes/patches in `v3/`, avoid new features and avoid breaking CLI/API changes.

Pros:
- Isolates large refactors from current users of `.../v3`.
- Enables a clean-slate layout (move `x/storm` into the new module; redesign storage; new CLI wiring) without constantly threading backward-compat concerns.
- Lets `v3/` act as a stable reference implementation during migration/testing.

Cons:
- Requires a new Go module path (`github.com/stevegt/grokker/v4` or `/v5`) and associated duplication/maintenance cost.
- In-flight changes split across two trees; backports become overhead.
- Increased coordination burden for tests/docs/examples (which version does a command refer to?).

#### v4 vs v5 (under this repo’s SemVer scheme)

- **Start `v4/` (even major)**:
  - Pros: signals “stable release line” to users; aligns with the idea that `v3` was pre-release leading into `v4`.
  - Cons: you’re likely to want an extended unstable period while merging Storm; doing that under an even major may contradict expectations unless you rely heavily on odd minor/patch pre-release numbers (e.g., `4.1.x` as pre-release of `4.2.0`).

- **Start `v5/` (odd major)**:
  - Pros: matches “this is a long-running, breaking, development line” semantics; keeps `v4` available as the “next stable” target for today’s `v3` line.
  - Cons: implies the next stable major after `v5` is `v6` (not `v4`), which is a bigger jump for users and may complicate messaging unless we’re explicitly maintaining a `v4` stable branch in parallel.

Decision framing:
- If the merge can preserve compatibility well enough to land in `v3` and then release `v4`, stay on `v3`.
- If the merge is fundamentally a new architecture (daemon-first + capabilities + tool calls), freeze `v3` and do it in a new major; choose `v5` if you expect a long unstable runway.

- [ ] 016.1 Decide: keep work in `v3/` vs new `v4/` vs new `v5/`
- [ ] 016.2 If new major: define module layout + install paths + compatibility shims (`grok` wrapper)

### Sequencing with TODO `015-interactive-cli.md` (interactive shell)

There’s a real ordering tradeoff between doing TODO 016 (“merge grok into storm”) and TODO 015 (“ship `storm sh`”).

#### Do TODO 016 before TODO 015

Pros:
- Decide the eventual command tree (`storm kb`, `storm sh`, etc.) and module home (`v3` vs `v4`/`v5`) before writing a lot of CLI code.
- Avoid building `storm sh` against APIs/layout that are about to move (especially if we choose a new major module).
- Unify on cobra/config conventions first, then add the interactive shell as “just another command”.

Cons:
- Higher risk of a long refactor runway before any user-facing UX improvement.
- Less early pressure-testing of Storm’s WS/query/approval workflows (which the merge work will still need to interact with).

#### Do TODO 015 before TODO 016

Pros:
- Faster productivity win for day-to-day Storm development and dogfooding.
- Exercises the daemon protocol, approvals, and review gate UX that the merge work will depend on.
- Lets the interactive shell become a useful front-end for driving and observing the migration.

Cons:
- Some rework risk if TODO 016 changes the module location or the CLI command tree.
- If TODO 016 changes the daemon protocol significantly, the shell may need updates.

Pragmatic approach:
- Do **016.1** (where the work lives: `v3` vs new major) early.
- Implement the TODO 015 MVP in that chosen home, with the protocol/client logic isolated so it can be moved.
- Then proceed with TODO 016 incrementally behind `storm kb ...` while using `storm sh` to dogfood.

- [ ] 016.3 Decide sequencing: 015-first vs 016-first (or hybrid)

### Code layout (move storm into v3)

- XXX no, do all of this in v5

- Create `v3/storm/` (or `v3/server/`) for daemon, API, web assets.
- Move `x/storm` packages into `v3/storm` and update module import paths.
- Add `v3/cmd/storm/main.go` as the new entry point; keep `v3/cmd/grok/main.go` as a shim.

### Vector DB and storage recommendations
- Storm already has the beginning of a stronger storage design in the bbolt KV store; grok still uses a monolithic JSON file that grows quickly.
- Recommendation: converge on the bbolt-backed design and phase out the JSON DB for embeddings and metadata.
- Suggested steps:
  - Define a KV schema that matches grok’s needs (documents, chunks, embeddings, chat history) and map `.grok` fields to buckets.
  - Add a one-time migration command (`storm kb migrate` or similar) with progress output and validation.
  - Implement dual-read during transition (read JSON if KV missing; write KV only) to reduce risk.
  - Keep a backup/export path for JSON during early migrations.
- Outcome: smaller on-disk footprint, better incremental updates, and a cleaner path to multi-project or shared DB layouts.

### Backward compatibility
- Keep `grok` binary as thin wrapper:
  - `grok <args>` -> `storm kb <args>`
- Keep `.grok` database format initially; consider optional `.storm` rename later with explicit migration.
- Keep existing `grok` tests as compatibility tests, or alias them to `storm kb`.

### Documentation and tests
- Update README(s) to show storm-first usage; keep a deprecation note for direct `grok` usage.
- Update `v3/cli` tests or add parallel `storm kb` tests.
- Add CLI help examples showing new hierarchy.

## Risks and decisions to make
- Decide if `.grok` stays or migrates to `.storm`.
  - XXX .grok moves to .storm/.db?
- Plan for API/daemon integration points between storm server and grok KB (if any).
- DONE choose the subcommand namespace: `storm kb` vs `storm grok`.
  - XXX storm kb
- DONE decide whether to unify on cobra or keep kong inside a wrapper.
  - XXX unify on cobra
