# Repository Guidelines

## Project Structure & Module Organization
- `v3/` is the primary Go module; core logic lives under `v3/core/`, CLI under `v3/cli/`, and the `grok` binary entry point under `v3/cmd/grok/`.
- `vscode-plugin/grokker/` contains the VS Code extension (Node/JS) and its tests.
- `x/` holds experimental or auxiliary modules and prototypes.
- Root docs (`README.md`, `TODO.md`, `STORIES.md`) describe usage, plans, and examples.

## Build, Test, and Development Commands
- `go build -o grok ./v3/cmd/grok` (from repo root) builds the CLI binary.
- `go test ./...` (from `v3/`) runs Go unit tests for the main module.
- `npm test` (from `vscode-plugin/grokker/`) runs VS Code extension tests.
- `npm run lint` (from `vscode-plugin/grokker/`) runs ESLint for the extension.

## Coding Style & Naming Conventions
- Go code follows standard `gofmt` formatting; keep package names short and lower-case.
- Tests use `*_test.go` filenames and table-driven patterns where helpful.
- JS files for the VS Code extension are plain Node style; lint with ESLint before PRs.

## Testing Guidelines
- Go tests rely on the standard `testing` package; add coverage alongside new features.
- VS Code extension tests run via the `vscode-test` harness in `vscode-plugin/grokker/test/`.
- Prefer deterministic tests; avoid network calls unless explicitly required.

## Commit & Pull Request Guidelines
- Commit messages are short, imperative, and capitalized (e.g., "Refactor chat client", "Fix WS path").
- In commit message bodies, include a section per changed file with bullet points summarizing the edits.
- PRs should include a concise summary, relevant test commands run, and linked issues when applicable.
- If a change affects behavior or output, include before/after notes or example output.

## Security & Configuration Tips
- Set `OPENAI_API_KEY` before running `grok` (see `README.md`).
- Local Grokker state files like `.grok` are ignored; do not commit generated state or binaries.

## Agent-Specific Notes
- Prefer small, focused edits and avoid rearranging files without a clear need.
- Do not remove comments or documentation; update them if outdated or incorrect.
- If you add new files or directories, document them here and explain the rationale in the PR.
- Before major changes, and after significant milestones, remind the user to commit their work.
- Treat a line containing only `commit` as: add and commit all changes with an AGENTS.md-compliant message.
