# bugs

- need spinner in status box visible to all users when processing

# features

- move files list to dialog popup when clicking "files" button
- really need https://playwright.dev/ to test
- refactor to be able to use one daemon on one port number for multiple projects
  - see discussion.md
- logins so we can support co-authored-by headers in git commits
  - let's try github oauth
  - for now perhaps just a simple username/password system
  - or even manually issue JWT or CWT tokens to users -- see
    discussion.md
- jump to end button:
  - make "jump to end" button auto-scroll to the left as well
  - make "jump to end" button be referenced to bottom of chat area
    instead of bottom of main window
- monitor using inotify and auto relaod and re-render markdown when markdown file changes?
- wrap queries in code block in markdown file 
  - reformat on read, so result will be written to disk

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
