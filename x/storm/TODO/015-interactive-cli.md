# 015 - Interactive CLI shell (Codex/Claude Code style)

Build an interactive terminal client for Storm that feels like Codex/Claude Code: a persistent, multi-round session where you can manage projects/files and run LLM queries with real-time status, approvals, and review gates.

This is **not** a replacement for the existing `storm` subcommands; it’s an additional *interactive* mode that uses the same HTTP API and WebSocket protocol as the web UI.

## Goals

- **Fast multi-round loop**: send a query, see status, follow up, repeat.
- **Project-centric sessions**: pick a project once; keep state (model, token limit, file selections).
- **Terminal-first UX**: good line editing, history, copy/paste, predictable output.
- **Unexpected files workflow**: handle `filesUpdated` approvals from the terminal.
- **Review gate integration**: work toward TODO `014-change-review-gate.md` (diff/approve/apply/commit).
- **Inline completion (Copilot-like)**: provide useful input-time suggestions 
  - rule-based for / (slash) commands
  - LLM-backed, like copilot, for free text

## Non-goals (initially)

- Full-feature parity with the web UI.
- Multi-user auth/login (see TODO 010).
- Running LLM calls directly from the client (client should talk to daemon).
- Cross-platform “perfect” terminal UI from day one.

## Proposed entrypoint

Add a new CLI subcommand:

- `storm sh` 

Flags:

- `--daemon` (default: `STORM_DAEMON_URL` or `http://localhost:8080`)
- `--project <id>` (optional; if omitted, show picker)
- `--llm <name>` (default from server/web UI default)
- `--token-limit <n|1K|2K|4K|8K|...>`
- `--headless` / `--no-color` (for CI logs)
- `--debug` (log raw WS messages)

## Session model

State to keep in-memory (and optionally persist per project):

- active `projectID`
- current `llm`
- current `tokenLimit`
- working `inputFiles` and `outFiles` sets for the next query
- optional `selection` string
- last `queryID` (for cancel / follow-ups)
- a local queue of draft prompts (pairs well with TODO `024-queue-button.md`)

Suggested persistence:

XXX use session logs like codex, or do we keep one long session log
and use embeddings to compress context?

- `~/.storm/shell/state/<projectID>.json` (best-effort; tolerate missing/invalid)
- `~/.storm/shell/history` for input history

## Terminal UX

### Layout (TUI option)

If using a TUI (recommended), aim for:

XXX this is not codex-like, but do we like this better?  maybe left
bar could be collapsible?

- Left: project + file selection summary
- Main: scrollback chat log (queries and responses)
- Bottom: input box (multi-line)
- Status line: connected/connecting, active project, model, token limit, query in-flight, spinner

### Input editing

Minimum:

- line editing, history, Ctrl-R search
- multi-line input (either a TUI text area, or `$EDITOR` integration)
  - XXX editor integration via ^g like codex
  - open `$EDITOR` for long prompts, then send on save/exit

### In-session commands

Use slash-commands (or `:`) to avoid collisions with freeform queries:

XXX sync these with codex and claude code where possible, e.g. /model

- `/help`
- `/projects` (list) + `/use <projectID>`
- `/files` (list authorized) + `/in ...` + `/out ...`
- `/llm <name>` + `/token <n>`
- `/send` (send current draft) + `/cancel`
- `/approve` (handle current unexpected-files state)
- `/queue` (queue current draft) + `/queued` (manage queued drafts)
- `/open <path>` (optional: print file to terminal or open in `$PAGER`)
- `/exit`

## Protocol integration

### HTTP API (existing)

Reuse existing endpoints for:

- project list/info/add/forget/update
- file list/add/forget
- discussion list/add/forget/switch (if needed)

### WebSocket (existing)

Connect to the same WS endpoint as the browser (e.g. `/ws/{projectID}`) and speak the same message formats:

- Client → Server:
  - `type: "query"` with `query`, `llm`, `selection`, `inputFiles`, `outFiles`, `tokenLimit`, `queryID`, `projectID`
  - `type: "cancel"` with `queryID`
  - `type: "approveFiles"` with `queryID`, `approvedFiles`
- Server → Client:
  - `type: "query"` broadcast
  - `type: "response"` (or equivalent) with response text
  - `type: "filesUpdated"` with `alreadyAuthorized`, `needsAuthorization`, `files`, `isUnexpectedFilesContext`
  - `type: "error"`

The interactive client should treat WS as authoritative for query lifecycle events and use HTTP for CRUD operations.

## Unexpected files UX (terminal)

When receiving `filesUpdated` with `isUnexpectedFilesContext=true`:

- Display:
  - `alreadyAuthorized`: can be approved immediately for extraction
  - `needsAuthorization`: needs `storm file add --project <id> <path>` (or an in-shell equivalent)
- Provide guided actions:
  - `/approve` shows a checklist UI (or numbered list) for `alreadyAuthorized`
  - `/file add <path>` (optional alias) calls the HTTP endpoint and then waits for the next `filesUpdated` tick

## Review gate (tie-in to TODO 014)

This shell becomes much more valuable once the server supports a diff/approve/apply flow. Target behavior:

1. LLM response arrives (includes proposed file edits).
2. Server produces per-file diffs (or the client computes diffs locally from extracted temp files).
3. Client displays a diff UI and asks for explicit approval:
   - approve individual files or all
   - optionally request regenerate/repair
4. Apply patches to working tree.
5. Optionally run tests.
6. Optionally commit (with a structured message).

This likely requires new server endpoints and/or changes to how extraction is performed (write to temp + diff first, then apply).

## Inline completion (Copilot-like)

Phase this:

### Phase A (no LLM)

- Tab completion for:
  - slash commands
  - project IDs
  - known file paths (from authorized files + repo walk under baseDir)
  - known models (from UI list or server-provided list)
- Simple suggestion bar:
  - last N queries
  - templates/snippets (“Explain”, “Fix”, “Refactor”, “Add tests”, etc.)

### Phase B (LLM-backed, optional)

- Behind a flag: `--suggest` or env `STORM_SHELL_SUGGEST=1`.
- Debounced calls (e.g., 300–500ms) that propose a *single-line* completion only.
- Hard limits: max tokens, no file writes, no network beyond daemon.
- Must be cancellable and must never block sending a real query.

## Implementation approach

Two viable approaches:

### Option 1: Readline + plain terminal output (fastest to ship)

- Pros: minimal dependencies, easy to debug, simpler tests
- Cons: harder to do rich UI (panes, scrollback, checklists), weaker multi-line editing

### Option 2: Bubble Tea TUI (recommended)

- Pros: good multi-pane layout, better UX for approvals/diffs, easier to integrate queues and status
- Cons: new dependency + more state management

Regardless of UI, structure code so protocol/client logic is independent from the UI layer.

Suggested package split:

- `shell/` (or `tui/`): UI layer (Bubble Tea model, rendering, key handling)
- `client/`: daemon HTTP client + WS client, message structs, reconnection
- `session/`: state machine (project selection, in-flight query, approvals)

## Acceptance criteria (MVP)

- `storm shell` starts, connects to daemon, lets you pick a project.
- Can send a query and display the response text in the terminal.
- Can cancel a query.
- Can handle `filesUpdated` notifications and approve `alreadyAuthorized` files.
- Remembers selected project + model + token limit between runs (best-effort).

## Tests

- Unit tests for:
  - command parsing (`/llm`, `/token`, `/use`, `/in`, `/out`, `/approve`)
  - WS message decoding/dispatch
  - session state transitions
- Integration tests:
  - start a test daemon, connect via WS, send a query (using mock LLM once TODO 017 lands)
  - exercise unexpected-files approval path

## Open questions

- Exact command name: `shell` vs `tui` vs `repl`.
- Where to store session state/history (global vs per-repo).
- How to render markdown responses in terminal (plain, or `glow`-style
  rendering).
- How much of TODO 014 belongs in server vs client.
- XXX we need a later, sandbox phase, where we support function and
  tool calls, and filter on commands and/or files instead of just
  files.
- XXX address the fundamental differences and pros/cons between
  codex vs storm:
  - codex (LLM calls directly from client) vs storm (client talks to daemon)
  - codex (acts on local files directly) vs storm (server manages files)
  - codex (single-user) vs storm (multi-user projects)
  - codex (no unexpected files flow) vs storm (has unexpected files flow)
  - codex (sandboxed function calls) vs storm (full FS access with
    files filtering)
  - codex (can run tool and function calls) vs storm (no tool/function
    calls yet but we want them)


