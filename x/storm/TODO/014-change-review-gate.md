# Change Review Gate (Diff/Approve/Apply/Commit)

Goal: make Storm safe and pleasant for multiuser workflows where LLM queries may propose edits across multiple files.

## Requirements (ordered)

1. **Diff-first, no writes by default**
   - Treat the LLM response as a *proposal*.
   - Produce a per-file diff against the current workspace state without writing to disk.

2. **Per-file approval in the browser**
   - Show a side-by-side diff per file (with Accept/Reject).
   - Support “accept all”, “reject all”, and “download patch”.

3. **Apply + commit as an explicit step**
   - Only approved files are written.
   - Offer an editable commit message and show a summary of changed files.

4. **Parallel exploration (branches/worktrees)**
   - Allow choosing a target branch/worktree per query so multiple users can explore alternatives without collisions.
   - Keep branch naming flexible; support structured paths (e.g., `scenario/...`) as a convention, not a hard rule.

5. **Docs view (workspace Markdown rendering)**
   - Render arbitrary project Markdown files as read-only HTML (not only the discussion file).
   - Optionally auto-reload on file change (related: TODO 012).

6. **Lightweight consensus signals (optional)**
   - Attach votes/scores/notes to a branch/worktree and keep them versioned (append-only records).

## Implementation sketch

- Reuse the existing “unexpected files approval” plumbing:
  - generalize from “approve new out files” to “approve any proposed file change”.
- Compute diffs server-side:
  - proposed content in-memory / temp dir, then diff vs current file bytes.
- On approval:
  - write files, then `git status` + `git diff --stat` summary, then commit.

