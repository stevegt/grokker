# Storm TODO

## Bugs

- [x] 001 - 'storm project update --basedir' also needs to update file paths
- [ ] 002 - Resolve naming discrepancy between "discussion files" and "markdown files"
- [ ] 003 - Add spinner in status box visible to all users when processing
- [ ] 004 - New unexpected files: repopulate approval list so user can click "out" box
- [x] 005 - `cli_test.go` needs random instead of hardcoded port numbers
  - Uses `testutil.GetAvailablePort()` from `testutil/server.go`
- [x] 013 - Read any existing `AGENTS.md` file for instructions related to any prompt

## Features

- [ ] 026-planning-group-workspace-mvp.md Planning group workspace tool MVP (shared docs, decisions, collaboration)
- [ ] 006 - Make 8k the default max token size; add larger presets
- [ ] 007 - File list: add sort options (alphabetical, time created, folders first)
- [ ] 008 - Wrap queries in code block in markdown file
  - Reformat on read so result will be written to disk
  - Add a version number at top of discussion file
- [ ] 009 - Add `status` subcommand to show current status of daemon including queries in progress
  - Add websocket status endpoint to support this
- [ ] 010 - Add logins so we can support co-authored-by headers in git commits
  - Let's try GitHub OAuth
  - For now perhaps just a simple username/password system
  - Or manually issue JWT or CWT tokens to users (see `discussion.md`)
- [ ] 011 - Jump to end button improvements
  - Make "jump to end" button auto-scroll to the left as well
  - Reference the "jump to end" button to the bottom of chat area instead of bottom of main window
- [ ] 012 - Monitor using inotify and auto reload/re-render markdown when markdown file changes
- [ ] 014-change-review-gate.md Change review gate (diff/approve/apply/commit) for file edits
  - Side-by-side diffs in UI before writing
  - Supports parallel scenario branches/worktrees
- [ ] 015-interactive-cli.md Interactive CLI shell (Codex/Claude Code style)
  - Include GitHub Copilot-like inline completion while typing input

- [ ] 025-promisegrid-node.md PromiseGrid node mode (delegation + multi-node git)
- [ ] 027-descriptors-into-storm.md Derive tool descriptors / tool calls from grid-poc `x/descriptors`
- [ ] 016-merge-grok.md Merge Grok into Storm
- [ ] 017-mock-llm.md Mock LLM backend for tests
- [ ] 018-scenario-tree.md Scenario tree workflow (git + LLM)
- [ ] 019-scenario-tree-ga.md LLM-assisted GA over scenario branches
- [ ] 020-testing-plan.md Unexpected files testing plan (no Playwright)
- [ ] 021-unexpected-files-plan.md Unexpected files staged plan (finish remaining gaps)
- [ ] 022-vector-db.md Vector DB integration for semantic file selection
- [ ] 023-web-client-test-plan.md Web client chromedp E2E tests
- [ ] 024-queue-button.md Queue button for deferred sending
