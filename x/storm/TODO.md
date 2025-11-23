# bugs

- DONE inconsistent display of prompts and responses (not all users see both)
  - was due to read timeout setting -- replaced with ping/pong keepalive
- need spinner in status box
- DONE need to stop removing '[\d+]' indexes from array variables

# features

- really need https://playwright.dev/ to test
- refactor to be able to use one daemon on one port number for multiple projects
- logins so we can support co-authored-by headers in git commits
  - let's try github oauth
- jump to end button:
  - DONE add "jump to end" button
  - make "jump to end" button auto-scroll to the left as well
  - make "jump to end" button be referenced to bottom of chat area
    instead of bottom of main window

