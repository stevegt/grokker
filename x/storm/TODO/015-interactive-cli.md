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

- `storm sh` starts, connects to daemon, lets you pick a project.
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

## Codex vs Storm (fundamental differences)

This TODO is explicitly inspired by Codex/Claude Code’s *terminal UX*, but Storm’s architecture differs in ways that affect what we should copy vs avoid.

### 1) LLM calls: client-direct vs daemon-mediated

- **Codex**: the client calls the LLM provider directly (client owns API keys, rate limits, retries).
  - Pros: fewer moving parts; no daemon required; naturally single-user; simpler failure modes.
  - Cons: hard to share a session across clients; hard to centrally enforce policy/caching/logging.
- **Storm**: the client talks to the **daemon**, and the daemon calls the LLM.
  - Pros: multiple front-ends (web + terminal) share one implementation and one conversation state; centralized policy (models, token limits), logging, and future safety gates; easier to add server-side features (review gate, tool calls).
  - Cons: requires a running daemon; adds a hop; needs auth/permissions once multi-user is real.

Implication for `storm sh`: it must be great at connection UX (connect/reconnect/status), and it should avoid duplicating LLM/provider logic in the client.

### 2) File access: local direct vs server-managed

- **Codex**: operates on local files directly (edits working tree files via tools).
  - Pros: very direct; no server-side file model; easy to diff locally.
  - Cons: safety depends on sandbox + user review; collaboration/sharing is ad-hoc.
- **Storm**: the **server owns file policy**: project `BaseDir`, authorized file list, and explicit `inputFiles`/`outFiles`, plus extraction.
  - Pros: “only touch what’s selected” is enforceable; works with remote clients; aligns with unexpected-files gating.
  - Cons: daemon runs with host FS privileges; correctness and security depend on path validation + authorization rules; clients need good affordances to manage file selection.

Implication for `storm sh`: file operations should flow through server APIs/WS messages, not local filesystem reads/writes (except possibly for local rendering/diff viewing when safe).

### 3) Single-user vs multi-user projects

- **Codex**: effectively single-user (one operator, one terminal session).
  - Pros: simpler mental model; no auth; fewer concurrency issues.
  - Cons: doesn’t naturally support shared, long-running project “rooms”.
- **Storm**: multi-project and intended to become multi-user (multiple clients connected to a project).
  - Pros: shared project state, shared visibility, collaboration potential.
  - Cons: needs authentication/authorization and conflict handling (TODO 010).

Implication for `storm sh`: be explicit about project identity, current discussion file, and whether other clients are connected (future).

### 4) Unexpected files flow

- **Codex**: no dedicated “unexpected files” concept; edits are whatever the agent decides to change, and the user reviews diffs.
  - Pros: fewer workflow steps.
  - Cons: easy to miss that the agent proposed touching unrelated files.
- **Storm**: has an explicit unexpected-files workflow (`filesUpdated`, approvals, authorization).
  - Pros: strong guardrail; makes multi-user safer; aligns with “only write declared outputs”.
  - Cons: requires UX in every client (web + terminal) to handle approvals cleanly.

Implication for `storm sh`: unexpected-files handling is first-class (checklist approvals, guided “authorize then retry” flow).

### 5) Sandboxed function calls vs full FS + file filtering

- **Codex**: tool execution is sandboxed (filesystem/network/command restrictions + user approvals).
  - Pros: strong safety boundary by default.
  - Cons: can be frustrating when the sandbox blocks legitimate workflows.
- **Storm**: daemon typically has full host access; safety is currently via **file filtering** and explicit `inputFiles`/`outFiles`.
  - Pros: flexible; can work on any repo layout; easy to integrate with existing dev tools on the host.
  - Cons: for hosted/multi-user, this needs hardening (OS/container sandboxing, stricter allowlists).

Implication for `storm sh`: don’t assume sandboxed execution exists today; design the UI so future sandboxed operations slot in cleanly.

### 6) Tool/function calls: built-in vs “not yet”

- **Codex**: can run tool/function calls (shell, file edits, etc.) as part of agent execution.
  - Pros: higher autonomy (can run tests, inspect outputs, iterate automatically).
  - Cons: needs tight safety controls; failures can be harder to reason about.
- **Storm**: doesn’t have tool/function calls yet; it’s “prompt + files + extraction” today, but we want tools later.
  - Pros: simpler trust model; fewer moving parts.
  - Cons: more manual steps; harder to build agentic workflows (run tests, gather context) inside Storm.

Implication for `storm sh`: plan for a future “proposed tool call” UI (preview → approve → run → capture output), likely implemented server-side to keep policy centralized.

## Open questions

- Exact command name: `sh` vs `shell` vs `tui` vs `repl`.
- Where to store session state/history (global vs per-repo).
- How to render markdown responses in terminal (plain, or `glow`-style
  rendering).
- How much of TODO 014 belongs in server vs client.
- How to introduce sandboxed tool/function calls (commands + filesystem) without losing Storm’s file-filtering guarantees.

