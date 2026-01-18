# Storm TODO

## Bugs

- [x] 001 - 'storm project update --basedir' also needs to update file paths
- [ ] 002 - Resolve naming discrepancy between "discussion files" and "markdown files"
- [ ] 003 - Add spinner in status box visible to all users when processing
- [ ] 004 - New unexpected files: repopulate approval list so user can click "out" box
- [x] 005 - `cli_test.go` needs random instead of hardcoded port numbers
  - Uses `testutil.GetAvailablePort()` from `testutil/server.go`
- [ ] 013 - Read any existing `AGENTS.md` file for instructions related to any prompt

## Features

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
- [ ] 014 - Change review gate (diff/approve/apply/commit) for file edits
  - Side-by-side diffs in UI before writing
  - Supports parallel scenario branches/worktrees
  - See `014-change-review-gate.md`
- [ ] 015 - Interactive CLI shell (Codex/Claude Code style)
  - Include GitHub Copilot-like inline completion while typing input

## Plans / notes

- `014-change-review-gate.md`
- `merge-grok.md`
- `mock-llm.md`
- `scenario-tree.md`
- `scenario_tree_GA.md`
- `testing-plan.md`
- `unexpected-files-plan.md`
- `vector-db.md`
- `web-client-test-plan.md`

## Needs to be converted to a TODO file

The following needs to be converted into a proper TODO file with numbering and
details.

---

add a "queue" button below the "send" button. when the user hits
"queue", the message is added to a local queue in IndexedDB instead of
being sent immediately.

if the user hits "queue" with an empty message, show a popup dialog
listing the currently queued messages with options to delete individual
messages or move a message from the queue to the send box for editing and
sending.

---
