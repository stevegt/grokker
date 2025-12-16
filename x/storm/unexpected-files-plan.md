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

**Status**: ⏳ TODO

**Objective**: Detect unexpected files after dry run and notify clients without requiring approval yet.

**Changes Required**[1][2]:
- ⏳ TODO: Modify `sendQueryToLLM()` to call `ExtractFiles()` twice: first with `DryRun: true`, then real extraction
- ⏳ TODO: After dry-run call, categorize `result.UnexpectedFiles`:
  - `alreadyAuthorized`: filenames in authorized files list
  - `needsAuthorization`: filenames NOT in authorized files list
- ⏳ TODO: Send WebSocket notification: `{type: "unexpectedFilesDetected", queryID, alreadyAuthorized: [...], needsAuthorization: [...]}`
- ⏳ TODO: Pause after notification, waiting for user approval via `approvalChannel`

**Testing** ⏳:
- ⏳ TODO: Query that returns unexpected files should trigger WebSocket notification
- ⏳ TODO: Verify notification contains correctly categorized files
- ⏳ TODO: Verify notifications don't appear when no unexpected files are detected

**Git Commit**: ⏳ TODO `storm: Implement dry-run detection and unexpected files WebSocket notification`

---

## Stage 6: Update project.html to Display Unexpected Files Modal

**Status**: ⏳ TODO

**Objective**: Show user-facing UI for file approval without yet implementing approval logic.

**Changes Required**[2]:
- ⏳ TODO: Add WebSocket handler for `message.type === "unexpectedFilesDetected"`
- ⏳ TODO: Open file modal automatically when notification arrives
- ⏳ TODO: Create two sections in modal:
  - "Already Authorized" section with table of files (same format as file list)
  - "Needs Authorization" section with simple list showing CLI command to add each file
- ⏳ TODO: Add "Confirm" button to close modal (approval not yet functional)

**Testing** ⏳:
- ⏳ TODO: Trigger unexpected files notification; verify modal opens automatically
- ⏳ TODO: Verify file categorization is displayed correctly
- ⏳ TODO: Verify modal can be closed; verify query completes without re-extraction
- ⏳ TODO: Test with mix of already-authorized and needs-authorization files

**Git Commit**: ⏳ TODO `storm: Add unexpected files modal UI to project.html`

---

## Stage 7: Implement Approval Flow and Re-extraction

**Status**: ⏳ TODO

**Objective**: Complete the two-phase extraction workflow with user approval.

**Changes Required**[1][2]:
- ⏳ TODO: In `project.html`, add change listeners to "Already Authorized" file checkboxes
- ⏳ TODO: When user checks "Out" column, capture the selection and send `approveFiles` message
- ⏳ TODO: In `readPump()`, when `approveFiles` message arrives, extract selected files from `PendingQuery`
- ⏳ TODO: In `sendQueryToLLM()`, upon receiving approval via channel:
  - Expand `outfiles` list to include approved files
  - Call `ExtractFiles()` again with `DryRun: false` and expanded list
  - Continue normal completion of response

**Testing** ⏳:
- ⏳ TODO: Query returns unexpected files
- ⏳ TODO: Modal opens showing already-authorized files
- ⏳ TODO: User checks "Out" checkbox for some files
- ⏳ TODO: Approval is sent via WebSocket
- ⏳ TODO: New files are extracted via second `ExtractFiles()` call
- ⏳ TODO: Verify all files (original + newly-approved) are written to disk
- ⏳ TODO: Test edge cases: user approves nothing, user approves all, user approves subset

**Git Commit**: ⏳ TODO `storm: Implement user approval flow with re-extraction of approved files`

---

## Stage 8: Handle Files Needing Authorization

**Status**: ⏳ TODO

**Objective**: Support adding unauthorized files via CLI during modal approval window.

**Changes Required**[1][2]:
- ⏳ TODO: In `project.html`, when user clicks "Add File" button for needs-authorization file, copy CLI command to clipboard
- ⏳ TODO: User manually runs: `storm file add --project storm <filename>`
- ⏳ TODO: Ensure `fileListUpdated` WebSocket broadcast handler updates file list
- ⏳ TODO: Update modal to reflect newly-authorized files moving from "Needs Authorization" to "Already Authorized" section
- ⏳ TODO: User can then approve the newly-authorized file

**Testing** ⏳:
- ⏳ TODO: Query returns needs-authorization files
- ⏳ TODO: Modal shows them in "Needs Authorization" section
- ⏳ TODO: User copies CLI command and runs it manually
- ⏳ TODO: File list updates via WebSocket broadcast
- ⏳ TODO: Modal refreshes to show file moved to "Already Authorized"
- ⏳ TODO: User enables the file and sends approval
- ⏳ TODO: File is extracted via re-extraction call

**Git Commit**: ⏳ TODO `storm: Support adding unauthorized files during approval window`

---

## Stage 9: Handle Declined Files and Modal Closure

**Status**: ⏳ TODO

**Objective**: Support users declining to approve files and gracefully complete queries without re-extraction.

**Changes Required**[1]:
- ⏳ TODO: If user closes modal without sending approval message, query should complete with first extraction results (status quo)
- ⏳ TODO: Files that were declined are simply not re-extracted
- ⏳ TODO: No permanent tracking needed; ephemeral choice

**Testing** ⏳:
- ⏳ TODO: Query returns unexpected files
- ⏳ TODO: Modal opens
- ⏳ TODO: User closes modal without approving anything
- ⏳ TODO: Query completes with original extraction (no re-extraction)
- ⏳ TODO: Verify declined file content is NOT in output files

**Git Commit**: ⏳ TODO `storm: Support modal closure without approval; complete query with original extraction`

---

## Stage 10: End-to-End Testing and Documentation

**Status**: ⏳ TODO

**Objective**: Comprehensive testing across all scenarios and document the feature.

**Testing Scenarios** ⏳:
1. ⏳ TODO: No unexpected files returned - query completes normally (no modal)
2. ⏳ TODO: Only already-authorized files returned - user enables them, re-extraction occurs
3. ⏳ TODO: Only needs-authorization files returned - user adds via CLI, then enables
4. ⏳ TODO: Mixed unexpected files - user approves subset, adds some via CLI
5. ⏳ TODO: User closes modal without approval - query completes with original extraction
6. ⏳ TODO: Multiple concurrent queries with unexpected files
7. ⏳ TODO: Cancel button still works during approval flow
8. ⏳ TODO: File permissions and error handling for failed re-extractions

**Documentation** ⏳:
- ⏳ TODO: Add comments to `sendQueryToLLM()` explaining dry-run and two-phase extraction
- ⏳ TODO: Document WebSocket message format for `unexpectedFilesDetected`
- ⏳ TODO: Update README with unexpected files feature description

**Git Commit**: ⏳ TODO `storm: Complete end-to-end testing and document unexpected files feature`

---

## Testing After Each Stage

**General Approach for All Stages**[1]:

After each stage:
1. **Functional Test**: Verify core Storm query flow still works (no unexpected files)
2. **Unit Tests**: Test new functions in isolation
3. **Integration Test**: Test with unexpected files present in LLM responses
4. **Regression Test**: Run full Storm test suite; all existing tests must pass
5. **Manual Test**: Interact with web UI; verify no UI regressions

**Tool Commands**:
```bash
# Run tests after each commit
go test -v -p 1 ./...

# Build and test locally
make build
./storm serve --port 8080
# Open browser and test queries
```

## References

[1] [https://golang.org/doc/effective_go#concurrency](https://golang.org/doc/effective_go#concurrency)
[2] [https://developer.mozilla.org/en-US/docs/Web/API/WebSocket](https://developer.mozilla.org/en-US/docs/Web/API/WebSocket)
