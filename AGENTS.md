# Repository Guidelines

## Project Structure & Module Organization
- `v3/` is the primary Go module; core logic lives under `v3/core/`, CLI under `v3/cli/`, and the `grok` binary entry point under `v3/cmd/grok/`.
- `vscode-plugin/grokker/` contains the VS Code extension (Node/JS) and its tests.
- `x/` holds experimental prototypes.
- Root docs (`README.md`, `TODO.md`, `STORIES.md`) describe usage, plans, and examples.
- Local Grokker state files like `.grok` are ignored; do not commit generated state or binaries.

## Build, Test, and Development Commands
- `go build -o grok ./v3/cmd/grok` (from repo root) builds the CLI binary.
- Prefer `make test` for testing storm; use a longer timeout (about 4 minutes) when running storm's full test suite.
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

## TODO Tracking 

- Maintain a `./TODO/` directory for tracking tasks and plans.
- Maintain a `./TODO/TODO.md` file that lists small tasks and the other TODO files.
- Number each TODO using 3 digits, zero-padded (e.g., 001, 002).
- Do not renumber TODOs when adding new ones; assign the next available number.
- Sort `TODO.md` by priority, not number.
- When discussing a TODO, use its number (e.g., "fix TODO 005").
- When completing a TODO, mark it as done by checking it off (e.g., `- [x] 005 - ...`).

## Commit & Pull Request Guidelines
- Commit messages are short, imperative, and capitalized (e.g., "Refactor chat client", "Fix WS path").
- In commit message bodies, include a section per changed file with bullet points summarizing the edits.
- When adding commit bodies, use multiple `-m` flags or a here-doc (`git commit -F -`) to avoid literal `\n` escapes.
- PRs should include a concise summary, relevant test commands run, and linked issues when applicable.
- If a change affects behavior or output, include before/after notes or example output.

## Security & Configuration Tips
- Set `OPENAI_API_KEY` before running `grok` (see `README.md`).
- Local Grokker state files like `.grok` are ignored; do not commit generated state or binaries.

## Agent-Specific Notes
- Prefer small, focused edits and avoid rearranging files without a clear need.
- Do not remove comments or documentation; update them if outdated or incorrect.
- If you add new files or directories, document them here and explain the rationale in the PR.
- Before and after major changes, after tests pass, and after significant milestones, prompt the user to commit their work.
- Always summarize `git diff` output to generate commit messages rather than relying only on chat context.
- Treat a line containing only `commit` as: "add and commit all changes with an AGENTS.md-compliant message".
- When running 'git add', list the individual files to be added, not 'git add .' or 'git add -A'.
- Frequently check `~/.codex/AGENTS.md` for updates to these guidelines, as they may evolve over time.
