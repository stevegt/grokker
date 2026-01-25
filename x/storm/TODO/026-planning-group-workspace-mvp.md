# TODO 026 - Planning group workspace MVP (research, call packets, notes, decisions)

Goal: ship a “shared workspace” mode suitable for a small planning group that reduces fragmentation across chat threads, docs, and call notes.

## MVP scope (what “done” means)

- A group can browse the workspace docs in the browser or CLI (see TODO 015).
- A group can safely accept LLM-proposed edits via a diff/approve/apply/commit gate (see TODO 014).
- A group has lightweight identity attribution (who reviewed/approved/committed) (see TODO 010).
- A repeatable “phone/video call packet” workflow exists: agenda/questions + research excerpts + call notes + outcomes/actions.
- The group has an explicit “publish/consensus” moment for important outcomes (diff-reviewed snapshot with attribution and a visible “synced” state).

## Leverage existing Storm work

- Diff/approve/apply/commit: build on `TODO/014-change-review-gate.md`.
    - For diff rendering, see:
        - github.com/computerscienceiscool/collab-editor
        - github.com/stevegt/collab-editor
    - For LLM-assisted change suggestions including local command execution, see:
        - github.com/computerscienceiscool/llm-runtime
        - github.com/stevegt/llm-runtime 
- Docs view: required by `TODO/014-change-review-gate.md` (Markdown rendering of arbitrary workspace files).  
    - For an online, shared-cursor editor, see:
        - github.com/computerscienceiscool/collab-editor
        - github.com/stevegt/collab-editor
- Identity: reuse/extend `TODO 010` (logins) but start with a simple cookie-based display name if needed.
    - For display name management, see:
        - github.com/computerscienceiscool/collab-editor
        - github.com/stevegt/collab-editor
- Auto-refresh: related to `TODO 012` (watch changes and reload/re-render).
- Desirable collaboration properties: common comparison set, explicit gated integration, `Co-authored-by:` attribution, and observable convergence (“synced”).

## Milestones

- [ ] 026.1 - Add “Docs” view: browse and render workspace Markdown files read-only (search + tree optional).
- [ ] 026.2 - Implement diff-first change review gate for any proposed file edits; per-file accept/reject; show a common comparison set of pending proposals.
- [ ] 026.3 - Apply + commit step with editable message and per-file summary; include `Co-authored-by:` attribution for contributors/reviewers.
- [ ] 026.4 - Add minimal identity (cookie name) and thread it through approvals/commits.
- [ ] 026.5 - Document the recommended “call packet” folder/template conventions.
- [ ] 026.6 - Add a visible “synced” signal per packet/notes/outcome (e.g., no pending proposals + all required reviewers approved current snapshot).
