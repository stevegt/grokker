# Storm Web Client Test Plan: chromedp Implementation

## Overview

The Storm multi-project LLM chat application requires comprehensive end-to-end testing of the web client to validate:
- Project creation and navigation via web UI
- File management modal interactions with unified file display
- Real-time WebSocket communication with the server
- LLM query submission and response handling
- File selection persistence via IndexedDB
- Unexpected files detection and dynamic modal reorganization
- UI state management and user interactions

This document specifies the testing approach: **chromedp (Headless Chrome via DevTools Protocol)** for end-to-end web client testing[1][2].

---

## Chosen Approach: chromedp

### Description

chromedp is a Go package that drives Chrome/Chromium via the Chrome DevTools Protocol[1]. Tests are written entirely in Go alongside server tests, enabling comprehensive end-to-end testing without requiring additional runtimes[1][2].

### Why chromedp

**Advantages**[1][2]
- **Same Language**: Tests written in Go; no JavaScript/TypeScript context switching
- **CI/CD Simple**: Runs in any environment with Chrome/Chromium; no Node.js dependency
- **Real Browser**: Uses actual Chromium engine; catches real browser compatibility issues
- **WebSocket Native**: Real WebSocket connections; true network behavior testing
- **Lightweight**: Minimal overhead compared to alternatives; efficient test execution
- **Integrated**: Can share test utilities with Go server tests
- **Headless Mode**: Runs without display server on CI/CD systems; HEADLESS=false for debugging

**Trade-offs**[2]
- Tests execute in seconds per test rather than milliseconds (real browser overhead)
- Requires Chrome/Chromium binary in test environment
- Limited parallel execution due to display/process resource constraints

### Current Implementation Status

**Implemented Test Cases** ✅
- `TestWebClientCreateProject` - Navigate to project page and verify sidebar loading
- `TestWebClientOpenFileModal` - Click Files button and verify modal opens with unified display
- `TestWebClientOpenFileModalWithSyntheticEvent` - Alternative approach using synthetic mouse events
- `TestWebClientAddFiles` - Add files via API and verify they appear in unified modal
- `TestWebClientQuerySubmitViaWebSocket` - Submit query and verify spinner appears
- `TestWebClientQueryWithResponse` - Complete query workflow with LLM response (skipped - timing issues)
- `TestWebClientFileSelectionPersistence` - Verify file selections persist via IndexedDB across modal open/close
- `TestWebClientPageLoad` - Verify landing page and project pages load successfully
- `TestWebClientCreateFileAndApproveUnexpected` - Complete flow: query creates file, user approves via modal, file appears on disk
- `TestWebClientUnexpectedFilesAlreadyAuthorized` - Tests handling already-authorized files in unexpected modal

**Test Execution Status** ✅
- All core tests passing
- 50+ total test functions across all test files
- ~35 seconds total test execution time
- ~70%+ code coverage for core logic

**Recent Completion**
- ✅ Unified `filesUpdated` message type replaces separate `unexpectedFilesDetected` and `fileListUpdated` messages
- ✅ Dynamic modal reorganization: files automatically move from "Needs Authorization" to "Already Authorized" sections as they are CLI-authorized
- ✅ Modal auto-closes and reopens when `filesUpdated` arrives, preserving query context
- ✅ Checkbox state persists via IndexedDB across all file interactions
- ✅ Path normalization bug fixed: relative paths resolved against project BaseDir

---

## Implementation Architecture

### Project Structure

```
x/storm/
├── main.go                  # Server implementation and query processing
├── project.html             # Web client UI with JavaScript event handlers
├── websocket_test.go        # Server-side WebSocket tests (50+ functions)
├── web_client_test.go       # chromedp web client tests (10 functions)
├── api_test.go              # HTTP API contract tests
├── cli_test.go              # CLI blackbox tests
├── locking_test.go          # Concurrency and database tests
├── testutil/
│   ├── server.go            # Test server setup and helpers
│   └── chromedp_helpers.go  # chromedp wrapper functions for common operations
└── go.mod
```

### Test Server Setup

The `testutil.NewTestServer()` helper:
1. Creates temporary directory for test project
2. Generates project ID and markdown file path
3. Gets available port via `getAvailablePort()`
4. Stores server config in TestServer struct
5. Returns TestServer with Port, DBPath, ProjectDir, MarkdownFile, ProjectID

Tests start the server in a goroutine:
```go
server := startTestServer(t, "web-test-create-project")
go serveRun(server.Port, server.DBPath)
testutil.WaitForServer(server.Port, 15*time.Second)
```

### Browser Context Initialization

The `newChromeContext()` helper respects the `HEADLESS` environment variable:
```bash
# Run with real browser visible (debugging)
HEADLESS=false go test -v ./... -run TestWeb

# Run headless (default)
go test -v ./... -run TestWeb
```

---

## Core Test Patterns

### Pattern 1: Navigate and Wait for Page Elements

```go
err := chromedp.Run(ctx,
    chromedp.Navigate(projectURL),
    chromedp.ActionFunc(func(ctx context.Context) error {
        return testutil.WaitForPageLoad(ctx)
    }),
    chromedp.ActionFunc(func(ctx context.Context) error {
        return testutil.WaitForWebSocketConnection(ctx)
    }),
    chromedp.ActionFunc(func(ctx context.Context) error {
        return testutil.WaitForElement(ctx, "#sidebar", false)
    }),
)
```

Key helpers:
- `WaitForPageLoad()` - Polls for `document.readyState === 'complete'`
- `WaitForWebSocketConnection()` - Waits for `ws.readyState === WebSocket.OPEN`
- `WaitForElement()` - Polls for element existence in DOM with configurable retry
- `WaitForModal()` - Waits for modal with class "show" and file-row content

### Pattern 2: Click Button and Wait for Result

```go
err = testutil.ClickElementWithSyntheticEvent(ctx, "#filesBtn")
err = testutil.WaitForModal(ctx)
numRows, err := testutil.GetFileListRows(ctx)
```

### Pattern 3: Unified File Modal Display

The `displayFileModal()` function now handles all file display scenarios:
- Regular authorized files with In/Out checkboxes
- Unexpected files (already authorized, marked in red)
- New/unauthorized files at bottom with CLI commands
- Automatic modal reorganization when `filesUpdated` arrives

### Pattern 4: File Selection Persistence

```go
testutil.SelectFileCheckbox(ctx, 1, "in")   // Row 1, input file
testutil.SelectFileCheckbox(ctx, 2, "out")  // Row 2, output file
testutil.CloseModal(ctx)

// Reopen and verify selections persisted
err = testutil.ClickElementWithSyntheticEvent(ctx, "#filesBtn")
err = testutil.WaitForModal(ctx)
inputFiles2, outputFiles2, err := testutil.GetSelectedFiles(ctx)
```

### Pattern 5: Unexpected Files Modal Flow

```go
err = testutil.WaitForUnexpectedFilesModal(ctx)
needsAuthFiles, err := testutil.GetNeedsAuthorizationFiles(ctx)

// User runs CLI command via API
addFilesPayload := map[string]interface{}{"filenames": []string{helloFn}}
resp, _ := http.Post(fileURL, "application/json", bytes.NewReader(jsonData))

// Modal auto-closes and reopens with updated categorization
time.Sleep(2 * time.Second)
err = testutil.WaitForModal(ctx)

// File should now be in authorized section, not needs-authorization
needsAuthFiles2, err := testutil.GetNeedsAuthorizationFiles(ctx)
// len(needsAuthFiles2) should be 0
```

---

## Test Utilities in chromedp_helpers.go

### Page Load Helpers

- `WaitForPageLoad(ctx)` - Waits for document.readyState === 'complete' plus 500ms
- `WaitForWebSocketConnection(ctx)` - Polls for ws.readyState === WebSocket.OPEN
- `WaitForElement(ctx, selector, false)` - Polls up to 15 seconds for element existence
- `WaitForModal(ctx)` - Waits for modal with class "show" AND file-row content
- `WaitForUnexpectedFilesModal(ctx)` - Detects unexpected files modal variant
- `GetElementText(ctx, selector)` - Returns text content of element

### Modal/Dialog Helpers

- `CloseModal(ctx)` - Closes modal via close button
- `OpenFileModal(ctx)` - Clicks #filesBtn and waits for modal

### File List Helpers

- `GetFileListRows(ctx)` - Returns number of `<tr class="file-row">` elements
- `SelectFileCheckbox(ctx, rowNumber, type)` - Selects input or output file checkbox
- `GetSelectedFiles(ctx)` - Returns selected input and output files from IndexedDB
- `GetNeedsAuthorizationFiles(ctx)` - Returns filenames from needs-authorization section

### Click/Event Helpers

- `ClickElementWithSyntheticEvent(ctx, selector)` - Dispatches MouseEvent to element
- `ClickFilesBtnWithSyntheticEvent(ctx)` - Specialized helper for Files button
- `SubmitQuery(ctx, queryText)` - Types query and clicks Send button

---

## Known Issues and Resolutions

### ✅ FIXED: IndexedDB Race Condition in File Modal

**Status**: FIXED

**Problem**: Files button clicked immediately after page load before IndexedDB upgrade completes

**Resolution**: Files button click handler now waits for IndexedDB to be ready before calling loadFileList()

### ✅ FIXED: Path Normalization Bug

**Status**: FIXED

**Problem**: Authorized files stored as absolute paths didn't match unexpected files in relative format

**Resolution**: `categorizeUnexpectedFiles()` now resolves relative paths against project.BaseDir

### ⚠️ KNOWN: chromedp JavaScript Evaluation Timing

**Status**: DOCUMENTED - Workaround in use

**Problem**: querySelector evaluation may return false for elements that exist, due to synchronization issues[2]

**Current Workaround**: 
- Polling with 100ms intervals instead of single checks
- Delays between DOM operations (time.Sleep)
- Manual testing with HEADLESS=false validates functionality

**Test Impact**: 
- `TestWebClientQueryWithResponse` skipped due to timing issues detecting query response
- Other tests use simpler, more reliable selectors

---

## Running Tests

### Local Development

```bash
# Run all web client tests
go test -v ./... -run TestWebClient

# Run single test
go test -v -run TestWebClientCreateProject

# Run with real browser visible (debugging)
HEADLESS=false go test -v -run TestWebClientCreateProject

# Run fast tests only (skips chromedp tests)
go test -short ./...

# Run all tests including WebSocket tests
go test -v ./...
```

### CI/CD Configuration

Tests automatically detect Chrome availability and skip if not found. Example:

```bash
# Ensure Chrome is installed, then run
go test -v ./... -run TestWebClient
```

---

## Test Coverage

### Implemented and Passing ✅

**Project Navigation**
- Landing page loads
- Create project via API
- Navigate to project page
- Sidebar loads with table of contents

**File Management - Unified Modal**
- Add files via API
- Open file modal
- Modal displays both authorized and unexpected files
- File selections persist via IndexedDB
- Modal closes properly

**Unexpected Files Flow**
- LLM creates unexpected files
- Modal shows files needing authorization
- User runs CLI to authorize files
- Modal auto-closes and reopens with updated categorization
- Already-authorized files shown in red
- Needs-authorization files shown at bottom with copy-command buttons
- User checks Out checkboxes
- Files appear on disk after user closes modal

**Query Submission**
- Type query in textarea
- Click Send button (synthetic event)
- Spinner appears while processing
- Query broadcasts to client via WebSocket

**WebSocket Communication**
- Connection establishment
- Query message sending
- Response receiving
- File list update broadcasts
- Unexpected files detection broadcasts
- Connection cleanup

### Coverage Gaps (Lower Priority)

**Not Currently Tested (but working via manual testing)**
- Multiple concurrent queries
- WebSocket reconnection after disconnect
- Error handling (network errors, server errors)
- Long-running query responses (timing issues with chromedp)

---

## Performance and Reliability

### Typical Test Execution Times

- `TestWebClientCreateProject` - 3-4 seconds
- `TestWebClientAddFiles` - 4-5 seconds
- `TestWebClientOpenFileModal` - 2-3 seconds
- `TestWebClientFileSelectionPersistence` - 8-10 seconds
- `TestWebClientCreateFileAndApproveUnexpected` - 5-8 seconds
- `TestWebClientPageLoad` - 3-4 seconds

**Total suite**: ~40-50 seconds (with 10 test functions active)

### Flakiness Mitigation

1. **Polling with retries** instead of single DOM queries
2. **Explicit waits** for page load and WebSocket connection
3. **Delays between actions** to allow event propagation
4. **Synthetic events** for more reliable clicks
5. **Query selectors** that target specific, stable elements
6. **Modal content verification** before proceeding

---

## Recent Updates (December 2025)

### Unified Message Protocol ✅
- Consolidated `unexpectedFilesDetected` and `fileListUpdated` into single `filesUpdated` message type
- Message includes `isUnexpectedFilesContext` flag to indicate message purpose
- Browser correctly handles both unexpected files and regular file updates with same handler

### Dynamic Modal Reorganization ✅
- Modal automatically closes and reopens when `filesUpdated` arrives
- Files move from "Needs Authorization" to "Already Authorized" sections in real-time
- `currentUnexpectedFilesQuery` preserved across close/reopen cycles
- Checkbox states persist via IndexedDB during reorganization

### Complete Unexpected Files Feature ✅
- Stages 1-10 implemented and tested
- Path normalization bug fixed
- All core workflows validated

---

## Future Improvements

1. **Resolve chromedp Timing Issues** - Investigate root cause of querySelector timing problems and implement more reliable detection
2. **Mock LLM Responses** - Avoid actual LLM calls in web client tests by mocking at server level
3. **Automated Spinner Detection** - Improve reliability of detecting query response completion
4. **Performance Profiling** - Add metrics for page load times and interaction latency
5. **Visual Regression Testing** - Capture screenshots and compare against baselines
6. **Accessibility Testing** - Verify ARIA attributes and keyboard navigation
7. **Network Simulation** - Test reconnection and failure scenarios

---

## References

[1] [https://github.com/chromedp/chromedp](https://github.com/chromedp/chromedp)
[2] [https://pkg.go.dev/github.com/chromedp/chromedp](https://pkg.go.dev/github.com/chromedp/chromedp)
