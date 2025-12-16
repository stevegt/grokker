# Staged Implementation Plan for Unexpected Files Flow

Implement unexpected file handling in Storm through a series of discrete stages, each maintaining full functionality and progressing toward real-time user approval workflows[1][2].

## Stage 1: Modify ExtractFiles to Return Result Struct (Grokker)

**Status**: ✅ DONE

**Objective**: Establish new return type with comprehensive file metadata without changing behavior.

**Changes Completed**[1]:
- ✅ Created `ExtractResult` struct in `grokker/v3/core/chat.go` with fields: `RawResponse`, `CookedResponse`, `ExtractedFiles`, `DetectedFiles`, `UnexpectedFiles`, `MissingFiles`, `BrokenFiles`
- ✅ Updated `ExtractFiles()` signature to return `(result ExtractResult, err error)` instead of `(cookedResp string, err error)`
- ✅ Populated all struct fields during extraction; builds complete list of all detected files (both expected and unexpected)
- ✅ Maintained identical extraction behavior; only the return type changed

**Testing** ✅:
- ✅ Existing Grokker tests pass
- ✅ `ExtractFiles()` correctly identifies unexpected files in test responses
- ✅ Tested with responses containing zero, one, and multiple unexpected files

**Git Commit**: ✅ `grokker: Introduce ExtractResult struct for comprehensive file metadata`

---

## Stage 2: Update Grokker Callers to Use Result Struct

**Status**: ✅ DONE

**Objective**: Adapt `ContinueChat()` and related functions to work with new return type.

**Changes Completed**[1]:
- ✅ Updated `ContinueChat()` in `grokker/v3/core/chat.go` to use `result.CookedResponse`
- ✅ Updated `extractFromChat()` to handle new return type
- ✅ Updated internal Grokker callers

**Testing** ✅:
- ✅ `ContinueChat()` flow end-to-end with various LLM outputs
- ✅ File extraction still works correctly
- ✅ Metadata about unexpected files is available

**Git Commit**: ✅ `grokker: Update internal callers to use ExtractResult struct`

---

## Stage 3: Update Storm's sendQueryToLLM to Handle Result

**Status**: ✅ DONE

**Objective**: Integrate new return type into Storm's query processing pipeline.

**Changes Completed**[1]:
- ✅ Modified `sendQueryToLLM()` in `main.go` to call `ExtractFiles()` and receive `ExtractResult`
- ✅ Using `result.CookedResponse` for the response that gets broadcast
- ✅ Code detects unexpected files using `len(result.UnexpectedFiles) > 0`
- ✅ Unexpected files are logged for debugging

**Testing** ✅:
- ✅ Full Storm query workflow end-to-end
- ✅ Responses are processed and displayed correctly
- ✅ Server logs confirm unexpected files are detected and logged
- ✅ Tested with queries that return unexpected files; processing continues normally

**Git Commit**: ✅ `storm: Integrate ExtractResult into sendQueryToLLM query processing`

---

## Stage 4: Extend ReadPump to Handle "approveFiles" Messages

**Status**: ✅ DONE

**Objective**: Add WebSocket message handler infrastructure for user approval flow.

**Changes Completed**[1]:
- ✅ Created `PendingQuery` struct with fields: `queryID`, `rawResponse`, `outFiles`, `approvalChannel`
- ✅ Added pending query tracker: `map[string]PendingQuery` protected by `pendingMutex`
- ✅ Extended `readPump()` to handle `{type: "approveFiles", queryID, approvedFiles}` messages
- ✅ Implemented `addPendingQuery()` to register queries awaiting approval
- ✅ Implemented `removePendingQuery()` to clean up completed queries
- ✅ Implemented `waitForApproval()` to block indefinitely until user approves files

**Implementation Details**[1]:
- `PendingQuery` struct (main.go, lines 13-18): Encapsulates query state with approval channel for signaling user decisions
- `pendingApprovals` map (main.go, line 67): Thread-safe registry of queries awaiting user approval via mutex protection
- `readPump()` "approveFiles" handler (main.go, lines 416-433): Extended WebSocket message handler to recognize and process `approveFiles` messages
- Helper functions: `addPendingQuery()` (line 595), `removePendingQuery()` (line 610), `waitForApproval()` (line 626)

**Testing** ✅:
- ✅ `TestWebSocketApproveFilesMessage` - Send approval message via WebSocket; verify no errors
- ✅ `TestWebSocketPendingQueryTracking` - Create pending query, verify it's tracked correctly, verify cleanup
- ✅ `TestWebSocketApprovalChannelReceival` - Verify approval signals flow through channel to receiver
- ✅ `TestWebSocketMultipleConcurrentApprovals` - Test 5 concurrent pending queries with different approvals simultaneously, verify no interference between queries
- ✅ `TestWebSocketApprovalIndefiniteWait` - Verify server waits indefinitely for user approval without timeout

**Git Commit**: ✅ `storm: Implement Stage 4 - pending query tracker and approveFiles message handler`

---

## Stage 5: Implement Dry-Run Detection and WebSocket Notification

**Status**: ✅ DONE

**Objective**: Detect unexpected files after dry run and notify clients, then wait for user approval before re-extraction.

**Changes Completed**[1][2]:
- ✅ Modified `sendQueryToLLM()` to call `ExtractFiles()` with `DryRun: true` to detect all files
- ✅ After dry-run call, categorize `result.UnexpectedFiles` into:
  - `alreadyAuthorized`: filenames in project's `AuthorizedFiles` list
  - `needsAuthorization`: filenames NOT in project's `AuthorizedFiles` list
- ✅ Created `categorizeUnexpectedFiles()` helper function to separate files by authorization status
- ✅ Send WebSocket notification: `{type: "unexpectedFilesDetected", queryID, alreadyAuthorized: [...], needsAuthorization: [...]}`
- ✅ Create pending query and wait for user approval via `approvalChannel`
- ✅ Upon receiving approval, expand `outfiles` list with approved files
- ✅ Re-run `ExtractFiles()` with `DryRun: false` and expanded list to extract both original and newly-approved files

**Implementation Details**[1][2]:
- `sendQueryToLLM()` function signature updated to accept `project *Project` parameter (line 695)
- Dry-run extraction with detection of unexpected files (lines 750-800)
- File categorization by calling `categorizeUnexpectedFiles()` (line 805)
- WebSocket notification broadcast to all clients (lines 808-815)
- Pending query creation and approval waiting (line 819)
- Re-extraction with expanded outfiles after approval (lines 823-828)
- `categorizeUnexpectedFiles()` helper function (line 647)

**Testing** ✅:
- ✅ `TestWebSocketUnexpectedFilesDetection` - Categorize files into authorized and needs-authorization
- ✅ `TestWebSocketUnexpectedFilesNotification` - Verify WebSocket broadcasts unexpected files notification
- ✅ Query with unexpected files triggers detection and categorization
- ✅ WebSocket clients receive the notification with correct categories
- ✅ Queries with no unexpected files don't trigger notifications

**Git Commit**: ✅ `storm: Implement Stage 5 - dry-run detection and unexpected files WebSocket notification`

---

## Stage 6: Update project.html to Display Unexpected Files Modal

**Status**: ⏳ TODO

**Objective**: Show user-facing UI for file approval.

**Changes Required**[2]:
- ⏳ TODO: Add WebSocket handler for `message.type === "unexpectedFilesDetected"`
- ⏳ TODO: Open file modal automatically when notification arrives
- ⏳ TODO: Create two sections in modal:
  - "Already Authorized" section with table of files (same format as file list)
  - "Needs Authorization" section with simple list showing CLI command to add each file
- ⏳ TODO: Add "Confirm" button to close modal and send approval

**Testing** ⏳:
- ⏳ TODO: Trigger unexpected files notification; verify modal opens automatically
- ⏳ TODO: Verify file categorization is displayed correctly
- ⏳ TODO: Verify modal can be closed; verify query completes
- ⏳ TODO: Test with mix of already-authorized and needs-authorization files

**Git Commit**: ⏳ TODO `storm: Add unexpected files modal UI to project.html`

---

## Stage 7: Implement Approval Flow and Re-extraction

**Status**: ⏳ TODO

**Objective**: Complete the two-phase extraction workflow with user approval.

**Changes Required**[1][2]:
- ⏳ TODO: In `project.html`, add change listeners to "Already Authorized" file checkboxes
- ⏳ TODO: When user checks "Out" column, capture the selection and send `approveFiles` message
- ⏳ TODO: In `readPump()`, when `approveFiles` message arrives, extract selected files
- ⏳ TODO: Verify re-extraction completes with both original and newly-approved files

**Testing** ⏳:
- ⏳ TODO: Query returns unexpected files, modal opens
- ⏳ TODO: User checks "Out" checkbox for some files
- ⏳ TODO: Approval is sent via WebSocket
- ⏳ TODO: Files are re-extracted via second `ExtractFiles()` call
- ⏳ TODO: Verify all files are written to disk
- ⏳ TODO: Test edge cases: user approves nothing, all, or subset

**Git Commit**: ⏳ TODO `storm: Implement user approval flow with re-extraction`

---

## Stage 8: Handle Files Needing Authorization

**Status**: ⏳ TODO

**Objective**: Support adding unauthorized files via CLI during modal approval window.

**Changes Required**[1][2]:
- ⏳ TODO: In `project.html`, show CLI command for needs-authorization files
- ⏳ TODO: User manually runs: `storm file add --project storm <filename>`
- ⏳ TODO: Update modal to reflect newly-authorized files moving from "Needs Authorization" to "Already Authorized"

**Testing** ⏳:
- ⏳ TODO: Query returns needs-authorization files
- ⏳ TODO: Modal shows them in "Needs Authorization" section
- ⏳ TODO: User adds file via CLI
- ⏳ TODO: File list updates via WebSocket broadcast
- ⏳ TODO: Modal refreshes, file now in "Already Authorized"

**Git Commit**: ⏳ TODO `storm: Support adding unauthorized files during approval window`

---

## Stage 9: Handle Declined Files and Modal Closure

**Status**: ⏳ TODO

**Objective**: Support users declining to approve files gracefully.

**Changes Required**[1]:
- ⏳ TODO: If user closes modal without approval, query completes with original extraction
- ⏳ TODO: Files that were declined are simply not re-extracted

**Testing** ⏳:
- ⏳ TODO: Query returns unexpected files
- ⏳ TODO: Modal opens, user closes without approving
- ⏳ TODO: Query completes with original extraction

**Git Commit**: ⏳ TODO `storm: Support modal closure without approval`

---

## Stage 10: End-to-End Testing and Documentation

**Status**: ⏳ TODO

**Objective**: Comprehensive testing across all scenarios and document the feature.

**Testing Scenarios** ⏳:
1. ⏳ TODO: No unexpected files - query completes normally
2. ⏳ TODO: Only already-authorized files - user enables them
3. ⏳ TODO: Only needs-authorization - user adds via CLI
4. ⏳ TODO: Mixed files - user approves subset
5. ⏳ TODO: User closes modal without approval
6. ⏳ TODO: Multiple concurrent queries with unexpected files
7. ⏳ TODO: Cancel button works during approval flow
8. ⏳ TODO: Error handling for failed re-extractions

**Documentation** ⏳:
- ⏳ TODO: Add comments explaining dry-run and two-phase extraction
- ⏳ TODO: Document WebSocket message format for `unexpectedFilesDetected`
- ⏳ TODO: Update README with unexpected files feature

**Git Commit**: ⏳ TODO `storm: Complete end-to-end testing and document unexpected files feature`

---

## Testing After Each Stage

**General Approach for All Stages**[1]:

After each stage:
1. **Functional Test**: Verify core Storm query flow still works
2. **Unit Tests**: Test new functions in isolation
3. **Integration Test**: Test with unexpected files present
4. **Regression Test**: Run full test suite
5. **Manual Test**: Interact with web UI

**Tool Commands**:
```bash
go test -v -p 1 ./...
make build
./storm serve --port 8080
```

## References

[1] [https://golang.org/doc/effective_go#concurrency](https://golang.org/doc/effective_go#concurrency)
[2] [https://developer.mozilla.org/en-US/docs/Web/API/WebSocket](https://developer.mozilla.org/en-US/docs/Web/API/WebSocket)
