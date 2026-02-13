# 028 - Migrate repo-root TODO.md into Storm TODO index

The repo-root `TODO.md` is a legacy scratchpad list. It mixes ideas, partial designs, and actionable items with no stable identifiers.

Storm’s backlog uses `x/storm/TODO/TODO.md` as an index with stable, numbered TODO files for larger topics. This TODO outlines how to migrate the useful parts of repo-root `TODO.md` into the Storm TODO system without losing information or creating duplicated tasks.

This also intersects with the planned repo split (`ciwg/storm` starting at `v5/`): decide what should move into the Storm-first repo vs stay as Grokker v3 backlog.

## Goals

- Preserve useful ideas from repo-root `TODO.md` without keeping it as the active backlog.
- Produce stable references (numbered TODOs) that can be linked from commits/docs.
- Reduce duplication by merging overlapping bullets into single canonical TODOs.
- Clarify scope boundaries: what belongs in Storm v5 vs Grokker v3.

## Key decisions

1. **Scope**
   - Which items from repo-root `TODO.md` are relevant to Storm (daemon/web/CLI/workspace/workflow)?
   - Which are Grokker-specific (embeddings DB, `grok` CLI details) and should stay with v3?

2. **Where does the canonical backlog live?**
   - Short-term: `x/storm/TODO/TODO.md` in this repo.
   - Medium-term: migrate the Storm backlog into `ciwg/storm` and treat this repo’s Storm TODOs as archived pointers.

3. **How to handle “research ideas” vs “actionable tasks”**
   - Keep research topics as TODO files with explicit “non-goals / unknowns”.
   - Convert actionable items into checkboxes with clear completion criteria.

## Proposed migration approach

1. Inventory each bullet from repo-root `TODO.md` into a working copy, preserving the original wording and sub-bullets.
2. Classify each item into buckets:
   - Storm v5 (new repo)
   - Grokker v3 maintenance (this repo’s `v3/`)
   - General notes (keep as archived context, not an active TODO)
3. Normalize each actionable item into a single-sentence checkbox (what done looks like).
4. De-duplicate: merge near-duplicates into one canonical TODO and add cross-references.
5. Create new `x/storm/TODO/NNN-*.md` files for the larger clusters (multi-phase work).
6. Update `x/storm/TODO/TODO.md` to include:
   - small items as checkbox lines
   - links to new TODO files for larger items
7. When the backlog is migrated, convert repo-root `TODO.md` into:
   - either an archived “legacy notes” file with a prominent “do not add new items here” note
   - or a short pointer to the canonical TODO index (preferred)

## Subtasks

- [ ] 028.1 Decide scope split: Storm v5 vs Grokker v3 vs archive-only
- [ ] 028.2 Create an inventory copy of repo-root `TODO.md` with classification tags
- [ ] 028.3 Draft new Storm TODO entries (small items) in `x/storm/TODO/TODO.md`
- [ ] 028.4 Create new Storm TODO files for the biggest clusters and link them from the index
- [ ] 028.5 De-duplicate and add cross-references between related TODOs
- [ ] 028.6 Replace repo-root `TODO.md` with an archive/pointer once migration is complete
- [ ] 028.7 When `ciwg/storm` exists, migrate the canonical Storm backlog there and leave pointers here

