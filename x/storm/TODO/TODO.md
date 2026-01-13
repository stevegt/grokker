# bugs

001. DONE 'storm project update --basedir' also needs to update file paths
002. resolve naming discrepancy between "discussion files" and "markdown files"
003. need spinner in status box visible to all users when processing
004. new unexpected files -- need to repopulate approval list so user can
     click 'out' box
005. cli_test.go needs random instead of hardcoded port numbers 
     - use getAvailablePort() from websocket_test.go (move it to testutils)
013. read any existing AGENTS.md file for instructions related to any prompt

# features

006. make 8k the default max token size, add larger presets
007. file list:
     - sort options (alphabetical, time created, folders first) 
008. wrap queries in code block in markdown file 
     - reformat on read, so result will be written to disk
     - add a version number at top of discussion file
009. status subcommand to show current status of daemon including queries
     in progress
     - websocket status endpoint to support this
010. logins so we can support co-authored-by headers in git commits
     - let's try github oauth
     - for now perhaps just a simple username/password system
     - or even manually issue JWT or CWT tokens to users -- see
       discussion.md
011. jump to end button:
     - make "jump to end" button auto-scroll to the left as well
     - make "jump to end" button be referenced to bottom of chat area
       instead of bottom of main window
012. monitor using inotify and auto reload and re-render markdown when markdown file changes?
014. change review gate (diff/approve/apply/commit) for file edits
     - side-by-side diffs in UI before writing
     - supports parallel scenario branches/worktrees
     - see TODO/014-change-review-gate.md
015. interactive CLI shell (Codex/Claude Code style)
     - include GitHub Copilot-like inline completion while typing input

# prompts

---

add a "queue" button below the "send" button.  when the user hits
"queue", the message is added to a local queue in IndexedDB instead of
being sent immediately.  

if the user hits "queue" with an empty message, show a popup dialog
listing the currently queued messages with options to delete individual
messages or move a message from the queue to the send box for editing and
sending.

---
