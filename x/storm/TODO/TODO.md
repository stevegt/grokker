# bugs

- need spinner in status box visible to all users when processing
- new unexpected files -- need to repopulate approval list so user can
  click 'out' box
- cli_test.go needs random instead of hardcoded port numbers 
  - use getAvailablePort() from websocket_test.go (move it to testutils)

# features

- make 8k the default max token size, add larger presets
- file list:
  - sort options (alphabetical, time created, folders first) 
- wrap queries in code block in markdown file 
  - reformat on read, so result will be written to disk
  - add a version number at top of discussion file
- status subcommand to show current status of daemon including queries
  in progress
  - websocket status endpoint to support this
- logins so we can support co-authored-by headers in git commits
  - let's try github oauth
  - for now perhaps just a simple username/password system
  - or even manually issue JWT or CWT tokens to users -- see
    discussion.md
- jump to end button:
  - make "jump to end" button auto-scroll to the left as well
  - make "jump to end" button be referenced to bottom of chat area
    instead of bottom of main window
- monitor using inotify and auto reload and re-render markdown when markdown file changes?

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
