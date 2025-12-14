# Staged Implementation Plan for Unexpected Files Flow

Implement unexpected file handling in Storm through a series of discrete stages, each maintaining full functionality and progressing toward real-time user approval workflows[1][2].

## Stage 1: Modify ExtractFiles to Return Result Struct (Grokker)

**Objective**: Establish new return type with comprehensive file metadata without changing behavior.

**Changes Required**[1]:
- Create `ExtractResult` struct in `grokker/v3/core/chat.go` with fields: `RawResponse`, `CookedResponse`, `ExtractedFiles`, `DetectedFiles`, `UnexpectedFiles`, `MissingFiles`, `BrokenFiles`
- Update `ExtractFiles()` signature to return `(result ExtractResult, err error)` instead of `(cookedResp string, err error)`
- Populate all struct fields during extraction; build complete list of all detected files (both expected and unexpected)
- Maintain identical extraction behavior; only the return type changes

**Testing**:
- Run existing Grokker tests; all should pass
- Verify `ExtractFiles()` correctly identifies unexpected files in test responses
- Test with responses containing zero, one, and multiple unexpected files

**Git Commit**: `grokker: Introduce ExtractResult struct for comprehensive file metadata`

---

## Stage 2: Update Grokker Callers to Use Result Struct

**Objective**: Adapt `ContinueChat()` and related functions to work with new return type.

**Changes Required**[1]:
- Update `ContinueChat()` in `grokker/v3/core/chat.go` to use `result.CookedResponse`
- Update `extractFromChat()` to handle new return type
- Update any other internal Grokker callers

**Testing**:
- Test `ContinueChat()` flow end-to-end with various LLM outputs
- Verify file extraction still works correctly
- Verify metadata about unexpected files is available (not yet used)

**Git Commit**: `grokker: Update internal callers to use ExtractResult struct`

---

## Stage 3: Update Storm's sendQueryToLLM to Handle Result

**Objective**: Integrate new return type into Storm's query processing pipeline.

**Changes Required**[1]:
- Modify `sendQueryToLLM()` in `main.go` to call `ExtractFiles()` and receive `ExtractResult`
- Use `result.CookedResponse` for the response that gets broadcast
- Detect unexpected files using `len(result.UnexpectedFiles) > 0` but don't act on them yet
- Log unexpected files for debugging

**Testing**:
- Run full Storm query workflow end-to-end
- Verify responses are processed and displayed correctly
- Check server logs to confirm unexpected files are detected and logged
- Test with queries that return unexpected files; verify they're detected but processing continues normally

**Git Commit**: `storm: Integrate ExtractResult into sendQueryToLLM query processing`

---

## Stage 4: Extend ReadPump to Handle "approveFiles" Messages

**Objective**: Add WebSocket message handler infrastructure for future user approval flow.

**Changes Required**[1]:
- Create pending query tracker: `map[string]PendingQuery` protected by mutex
- Define `PendingQuery` struct with fields: `queryID`, `rawResponse`, `outFiles`, `approvalChannel`
- Extend `readPump()` to handle `{type: "approveFiles", queryID, approvedFiles}` messages
- Send approval signal via channel to waiting `sendQueryToLLM()`

**Testing**:
- Send "approveFiles" message via WebSocket; verify it's received and logged
- Verify the message doesn't break existing query flow (no queries are pending approval yet)
- Test with multiple concurrent queries to verify channel synchronization works

**Git Commit**: `storm: Add pending query tracker and approveFiles message handler`

---

## Stage 5: Implement Dry-Run Detection and WebSocket Notification

**Objective**: Detect unexpected files after dry run and notify clients without requiring approval yet.

**Changes Required**[1][2]:
- Modify `sendQueryToLLM()` to call `ExtractFiles()` twice: first with `DryRun: true`, then real extraction
- After dry-run call, categorize `result.UnexpectedFiles`:
  - `alreadyAuthorized`: filenames in authorized files list
  - `needsAuthorization`: filenames NOT in authorized files list
- Send WebSocket notification: `{type: "unexpectedFilesDetected", queryID, alreadyAuthorized: [...], needsAuthorization: [...]}`
- Pause after notification, waiting for user approval via `approvalChannel`

**Testing**:
- Query that returns unexpected files should trigger WebSocket notification
- Verify notification contains correctly categorized files
- For now, approval doesn't arrive; query should timeout or need manual intervention (acceptable for this stage)
- Verify notifications don't appear when no unexpected files are detected

**Git Commit**: `storm: Implement dry-run detection and unexpected files WebSocket notification`

---

## Stage 6: Update project.html to Display Unexpected Files Modal

**Objective**: Show user-facing UI for file approval without yet implementing approval logic.

**Changes Required**[2]:
- Add WebSocket handler for `message.type === "unexpectedFilesDetected"`
- Open file modal automatically when notification arrives
- Create two sections in modal:
  - "Already Authorized" section with table of files (same format as file list)
  - "Needs Authorization" section with simple list showing CLI command to add each file
- Add "Confirm" button to close modal (approval not yet functional)

**Testing**:
- Trigger unexpected files notification; verify modal opens automatically
- Verify file categorization is displayed correctly
- Verify modal can be closed; verify query completes without re-extraction (using first extraction results)
- Test with mix of already-authorized and needs-authorization files

**Git Commit**: `storm: Add unexpected files modal UI to project.html`

---

## Stage 7: Implement Approval Flow and Re-extraction

**Objective**: Complete the two-phase extraction workflow with user approval.

**Changes Required**[1][2]:
- In `project.html`, add change listeners to "Already Authorized" file checkboxes
- When user checks "Out" column, capture the selection and send `approveFiles` message
- In `readPump()`, when `approveFiles` message arrives, extract selected files from `PendingQuery`
- In `sendQueryToLLM()`, upon receiving approval via channel:
  - Expand `outfiles` list to include approved files
  - Call `ExtractFiles()` again with `DryRun: false` and expanded list
  - Continue normal completion of response

**Testing**:
- Query returns unexpected files
- Modal opens showing already-authorized files
- User checks "Out" checkbox for some files
- Approval is sent via WebSocket
- New files are extracted via second `ExtractFiles()` call
- Verify all files (original + newly-approved) are written to disk
- Test edge cases: user approves nothing, user approves all, user approves subset

**Git Commit**: `storm: Implement user approval flow with re-extraction of approved files`

---

## Stage 8: Handle Files Needing Authorization

**Objective**: Support adding unauthorized files via CLI during modal approval window.

**Changes Required**[1][2]:
- In `project.html`, when user clicks "Add File" button for needs-authorization file, copy CLI command to clipboard
- User manually runs: `storm file add --project storm <filename>`
- Existing `fileListUpdated` WebSocket broadcast handler already updates file list
- Update modal to reflect newly-authorized files moving from "Needs Authorization" to "Already Authorized" section
- User can then approve the newly-authorized file

**Testing**:
- Query returns needs-authorization files
- Modal shows them in "Needs Authorization" section
- User copies CLI command and runs it manually
- File list updates via WebSocket broadcast
- Modal refreshes to show file moved to "Already Authorized"
- User enables the file and sends approval
- File is extracted via re-extraction call

**Git Commit**: `storm: Support adding unauthorized files during approval window`

---

## Stage 9: Handle Declined Files and Modal Closure

**Objective**: Support users declining to approve files and gracefully complete queries without re-extraction.

**Changes Required**[1]:
- If user closes modal without sending approval message, query should complete with first extraction results (status quo)
- Files that were declined are simply not re-extracted
- No permanent tracking needed; ephemeral choice

**Testing**:
- Query returns unexpected files
- Modal opens
- User closes modal without approving anything
- Query completes with original extraction (no re-extraction)
- Verify declined file content is NOT in output files

**Git Commit**: `storm: Support modal closure without approval; complete query with original extraction`

---

## Stage 10: End-to-End Testing and Documentation

**Objective**: Comprehensive testing across all scenarios and document the feature.

**Testing Scenarios**[1][2]:
1. No unexpected files returned - query completes normally (no modal)
2. Only already-authorized files returned - user enables them, re-extraction occurs
3. Only needs-authorization files returned - user adds via CLI, then enables
4. Mixed unexpected files - user approves subset, adds some via CLI
5. User closes modal without approval - query completes with original extraction
6. Multiple concurrent queries with unexpected files
7. Cancel button still works during approval flow
8. File permissions and error handling for failed re-extractions

**Documentation**:
- Add comments to `sendQueryToLLM()` explaining dry-run and two-phase extraction
- Document WebSocket message format for `unexpectedFilesDetected`
- Update README with unexpected files feature description

**Git Commit**: `storm: Complete end-to-end testing and document unexpected files feature`

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



## References

- [1] [https://www.honeybadger.io/blog/a-definitive-guide-to-regular-expressions-in-go/](https://www.honeybadger.io/blog/a-definitive-guide-to-regular-expressions-in-go/)
- [2] [https://zetcode.com/golang/regexp-findallsubmatch/](https://zetcode.com/golang/regexp-findallsubmatch/)
- [3] [https://tutorialedge.net/golang/parsing-json-with-golang/](https://tutorialedge.net/golang/parsing-json-with-golang/)
- [4] [https://gobyexample.com/regular-expressions](https://gobyexample.com/regular-expressions)
- [5] [https://forums.ni.com/t5/LabVIEW/Regular-expression-help-please-extracting-a-string-subset/td-p/2117616](https://forums.ni.com/t5/LabVIEW/Regular-expression-help-please-extracting-a-string-subset/td-p/2117616)
- [6] [https://forum.golangbridge.org/t/how-to-use-go-to-parse-text-files/31074](https://forum.golangbridge.org/t/how-to-use-go-to-parse-text-files/31074)
- [7] [https://yalantis.com/blog/how-to-build-websockets-in-go/](https://yalantis.com/blog/how-to-build-websockets-in-go/)
- [8] [https://aws.amazon.com/blogs/compute/serverless-strategies-for-streaming-llm-responses/](https://aws.amazon.com/blogs/compute/serverless-strategies-for-streaming-llm-responses/)
- [9] [https://ably.com/topic/websocket-architecture-best-practices](https://ably.com/topic/websocket-architecture-best-practices)
- [10] [https://developer.mozilla.org/en-US/docs/Web/API/WebSockets_API](https://developer.mozilla.org/en-US/docs/Web/API/WebSockets_API)
- [11] [https://upstash.com/blog/resumable-llm-streams](https://upstash.com/blog/resumable-llm-streams)
- [12] [https://learn.microsoft.com/en-us/azure/architecture/patterns/async-request-reply](https://learn.microsoft.com/en-us/azure/architecture/patterns/async-request-reply)
- [13] [https://golangbot.com/context-timeout-cancellation/](https://golangbot.com/context-timeout-cancellation/)
- [14] [https://www.micahwalter.com/2023/11/working-with-amazon-bedrocks-streaming-response-api-and-go/](https://www.micahwalter.com/2023/11/working-with-amazon-bedrocks-streaming-response-api-and-go/)
- [15] [https://elixirforum.com/t/any-tips-on-streaming-llm-responses-from-phoenix-sockets/65574](https://elixirforum.com/t/any-tips-on-streaming-llm-responses-from-phoenix-sockets/65574)
- [16] [https://konradreiche.com/blog/context-is-for-goroutine-cancellation/](https://konradreiche.com/blog/context-is-for-goroutine-cancellation/)
- [17] [https://betterprogramming.pub/writing-a-stream-api-in-go-afbc3c4350e2](https://betterprogramming.pub/writing-a-stream-api-in-go-afbc3c4350e2)
- [18] [https://github.com/stanford-oval/storm](https://github.com/stanford-oval/storm)
- [19] [https://www.datastudios.org/post/grok-ai-context-window-token-limits-and-memory-architecture-performance-and-retention-behavior](https://www.datastudios.org/post/grok-ai-context-window-token-limits-and-memory-architecture-performance-and-retention-behavior)



## Reasoning

The user is asking me to "restate a detailed plan for the unexpected files flow. describe it as several stages, with a git commit and testing after each stage. keep storm functional after each stage."

This is asking me to provide a staged implementation plan for the unexpected files feature, with clear milestones, commits, and testing between each stage, ensuring the application remains functional throughout.

Looking at the system prompt:
1. I'm a researcher
2. Answer ONLY the query at the end
3. The query is asking for a detailed plan in stages with commits and testing
4. Start with markdown heading
5. Limit discussion to 8192 tokens (about 2340 words)
6. Don't include empty square brackets
7. Enclose reference numbers in square brackets
8. Don't skip array indices in Go code
9. Output files don't count against limit
10. Return complete files only

The user wants a staged plan for implementing the unexpected files feature. Given the code they've provided, I should outline stages that:
- Keep Storm functional after each stage
- Include git commits after each stage
- Include testing after each stage
- Build toward the final unexpected files feature

Looking at the current architecture:
- Stage 1: Modify ExtractFiles to return ExtractResult struct with metadata
- Stage 2: Update callers to use new ExtractResult struct
- Stage 3: Add WebSocket cancellation support (already partially done)
- Stage 4: Add dry-run detection for unexpected files
- Stage 5: Add WebSocket notification for unexpected files
- Stage 6: Add modal UI for file approval
- Stage 7: Implement re-extraction with user-approved files
- Stage 8: Test end-to-end flow

Let me outline this as a detailed plan.


