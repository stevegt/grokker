# 024 - Queue button for deferred sending

Add a **Queue** button below the **Send** button in the web client. Queueing lets users stage one or more messages locally (in the browser) before sending them to the server/LLM.

## Goals

- Allow batching prompts while a long query is running.
- Avoid losing drafted prompts on refresh/crash.
- Make queued prompts reviewable/editable before sending.

## UI behavior

### Queue button

- Add a **Queue** button next to/below **Send**.
- When the user clicks **Queue** and the input box is non-empty:
  - Save the current input text as a queued message in IndexedDB.
  - Clear the input box.
  - Show a small confirmation (toast/snackbar) with the queue length.

### Queue dialog (when Queue clicked with empty input)

If the input box is empty and the user clicks **Queue**, open a modal dialog listing queued messages:

- Each queued item shows:
  - Timestamp (queued-at)
  - First line / short preview
  - Buttons: **Edit**, **Delete**, **Send**
- Actions:
  - **Edit**: moves that queued message into the input box (removing it from the queue).
  - **Delete**: removes it from the queue.
  - **Send**: sends it immediately (same as typing it and pressing Send), then removes it from the queue.
- Optional bulk actions:
  - **Send all** (in order)
  - **Clear queue**

## Storage

- Store queued messages in IndexedDB per project:
  - Keyed by `projectID` (and optionally browser origin).
  - Schema:
    - `id` (uuid or auto-increment)
    - `queuedAt` (ms since epoch)
    - `text`
    - `selection` (optional; if applicable to Storm’s query payload)
    - `inputFiles` / `outFiles` (optional; if applicable)
    - `llm` and `tokenLimit` (optional; if the UI tracks defaults)

## Sending semantics

- When sending a queued message:
  - Reuse the existing query send path (WebSocket `query` message).
  - Respect current UI state for selected files, model, token limits unless the queued item stored overrides.
  - If a query is currently in progress:
    - Either allow sending anyway (multiple concurrent queryIDs), or disable **Send**/**Send queued** and keep queueing only.
    - If disabled, show why and keep items queued.

## Edge cases

- Duplicate queued messages: allow (no dedupe needed).
- Very large queued messages: cap size or show warning (TBD).
- Project deletion: clear associated queue entries.
- IndexedDB unavailable: fall back to localStorage (best-effort) or disable with error.

## Tests

- Unit-ish browser tests (where feasible) for queue persistence logic.
- chromedp flow:
  - Queue non-empty input → clears input, shows increased count.
  - Queue with empty input → opens dialog, shows queued items.
  - Edit from dialog → moves item into input box and removes from queue.
  - Delete from dialog → removes item.

