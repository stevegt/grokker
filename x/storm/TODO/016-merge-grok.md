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
