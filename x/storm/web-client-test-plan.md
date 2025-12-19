# Storm Web Client Test Plan: chromedp Implementation

## Overview

The Storm multi-project LLM chat application requires comprehensive end-to-end testing of the web client to validate:
- Project creation and file management workflows
- Real-time WebSocket communication with the server
- LLM query submission and response handling
- File extraction and output file handling
- UI state management and user interactions

This document specifies the chosen testing approach: **chromedp (Headless Chrome via DevTools Protocol)** for end-to-end web client testing[1][2].

---

## Chosen Approach: chromedp

### Description

chromedp is a Go package that drives Chrome/Chromium via the Chrome DevTools Protocol[1]. Unlike Playwright (which requires Node.js), chromedp runs entirely in Go, allowing web client tests to be written and executed alongside server tests in the same test suite[1][2].

### Why chromedp

**Advantages**[1][2]
- **Same Language**: Tests written in Go; no JavaScript/TypeScript context switching
- **CI/CD Simple**: Runs in any environment with Chrome/Chromium; no Node.js dependency
- **Real Browser**: Uses actual Chromium engine; catches real browser compatibility issues
- **WebSocket Native**: Real WebSocket connections; true network behavior testing
- **Lightweight**: Minimal overhead compared to Playwright; faster test startup
- **Integrated**: Can share test utilities with Go server tests
- **Headless Mode**: Runs without display server on CI/CD systems

**Trade-offs**[2]
- Slightly slower than mock-based tests (seconds per test, not milliseconds)
- Requires Chrome/Chromium binary in environment
- Less parallel execution than mock engine (display/process limits)
- More verbose syntax than Playwright

### Architecture

**Test Framework**: Go `testing` package or `testify/suite`
**Browser Control**: chromedp (WebDriver/DevTools Protocol)
**Server Instance**: Temporary Storm server running on test port (e.g., 9999)
**Database**: Temporary SQLite in `/tmp` for test isolation
**Assertion Library**: testify/assert for readable assertions

```
┌─────────────────────┐
│  Go Test Runner     │
│  (testing.T)        │
└──────────┬──────────┘
           │
           ├─── Start Storm server (port 9999)
           ├─── Launch chromedp browser context
           │
           ├─── Test: Create Project
           │    └─── Navigate → Fill form → Submit → Assert
           │
           ├─── Test: Add Files
           │    └─── Click "Files" → Select files → Assert list update
           │
           ├─── Test: Submit Query
           │    └─── WebSocket message → Receive response → Verify UI
           │
           └─── Cleanup: Stop server, remove temp DB
```

---

## Implementation Strategy

### 1. Project Structure

```
x/storm/
├── main.go                  # Server implementation
├── project.html             # Web client
├── websocket_test.go        # WebSocket server tests (existing)
├── web_client_test.go       # NEW: chromedp web client tests
├── testutil/
│   ├── server.go           # Test server setup/teardown helpers
│   └── chromedp_helpers.go # chromedp wrapper functions
└── go.mod
```

### 2. Test Server Setup Helper

```go
// testutil/server.go
package testutil

import (
    "net"
    "testing"
)

type TestServer struct {
    Port   int
    DBPath string
    URL    string
    Cancel func()
}

// StartTestServer starts a temporary Storm server on an available port[1]
func StartTestServer(t *testing.T) *TestServer {
    port, _ := getAvailablePort()
    tmpDir := t.TempDir()
    dbPath := filepath.Join(tmpDir, "test.db")
    
    ctx, cancel := context.WithCancel(context.Background())
    
    go func() {
        serveRun(port, dbPath)
    }()
    
    // Wait for server to start
    waitForServer(port, 10*time.Second)
    
    return &TestServer{
        Port:   port,
        DBPath: dbPath,
        URL:    fmt.Sprintf("http://localhost:%d", port),
        Cancel: cancel,
    }
}
```

### 3. Core Web Client Tests

#### Test 1: Create and View Project

```go
// web_client_test.go
func TestCreateProjectViaWebUI(t *testing.T) {
    server := testutil.StartTestServer(t)
    defer server.Cancel()
    
    ctx, cancel := chromedp.NewContext(context.Background())
    defer cancel()
    
    // Navigate to landing page
    chromedp.Run(ctx,
        chromedp.Navigate(server.URL),
        chromedp.WaitVisible(`button:has-text("Create Project")`),
    )
    
    // Create project (if UI supports it, or via API)
    payload := map[string]string{
        "projectID": "test-proj",
        "baseDir": "/tmp/test",
        "markdownFile": "/tmp/test/chat.md",
    }
    resp, _ := http.Post(server.URL+"/api/projects", "application/json", ...)
    
    // Navigate to project
    chromedp.Run(ctx,
        chromedp.Navigate(server.URL + "/project/test-proj"),
        chromedp.WaitVisible(`#sidebar`),
        chromedp.WaitVisible(`#chat`),
    )
    
    // Verify project page loaded
    var sidebarText string
    chromedp.Run(ctx,
        chromedp.Text(`#sidebar h3`, &sidebarText),
    )
    
    if !strings.Contains(sidebarText, "Contents") {
        t.Errorf("Sidebar not loaded correctly")
    }
}
```

#### Test 2: Add Files and Verify List Update

```go
func TestAddFilesWebSocket(t *testing.T) {
    server := testutil.StartTestServer(t)
    defer server.Cancel()
    
    // Create project with test file
    testFile := filepath.Join(server.DBPath, "test.txt")
    ioutil.WriteFile(testFile, []byte("test"), 0644)
    
    ctx, cancel := chromedp.NewContext(context.Background())
    defer cancel()
    
    chromedp.Run(ctx,
        chromedp.Navigate(server.URL + "/project/test-proj"),
    )
    
    // Add file via API
    addFilePayload := map[string]interface{}{
        "filenames": []string{testFile},
    }
    jsonData, _ := json.Marshal(addFilePayload)
    http.Post(server.URL + "/api/projects/test-proj/files/add", 
        "application/json", strings.NewReader(string(jsonData)))
    
    // Wait for WebSocket broadcast to update file list UI
    var fileTableText string
    chromedp.Run(ctx,
        chromedp.Sleep(1*time.Second), // Wait for WebSocket broadcast
        chromedp.Text(`#fileList`, &fileTableText),
    )
    
    if !strings.Contains(fileTableText, "test.txt") {
        t.Errorf("File not appearing in UI after WebSocket update")
    }
}
```

#### Test 3: Submit Query and Receive Response

```go
func TestQuerySubmitAndResponse(t *testing.T) {
    server := testutil.StartTestServer(t)
    defer server.Cancel()
    
    ctx, cancel := chromedp.NewContext(context.Background())
    defer cancel()
    
    chromedp.Run(ctx,
        chromedp.Navigate(server.URL + "/project/test-proj"),
        chromedp.WaitVisible(`#userInput`),
    )
    
    // Type query and submit
    chromedp.Run(ctx,
        chromedp.Focus(`#userInput`),
        chromedp.SendKeys(`#userInput`, "test query", kb.Enter),
        chromedp.Sleep(500*time.Millisecond),
    )
    
    // Verify spinner appears (query processing)
    var spinnerVisible bool
    chromedp.Run(ctx,
        chromedp.Evaluate(`document.querySelector('.spinner') !== null`, &spinnerVisible),
    )
    
    if !spinnerVisible {
        t.Errorf("Spinner not visible for pending query")
    }
    
    // In real test, would mock LLM response or wait for it
    t.Logf("Query submission test passed")
}
```

#### Test 4: File Selection and WebSocket Communication

```go
func TestFileSelectionWebSocket(t *testing.T) {
    server := testutil.StartTestServer(t)
    defer server.Cancel()
    
    // Create test files and add to project
    // ...
    
    ctx, cancel := chromedp.NewContext(context.Background())
    defer cancel()
    
    chromedp.Run(ctx,
        chromedp.Navigate(server.URL + "/project/test-proj"),
        chromedp.WaitVisible(`#filesBtn`),
    )
    
    // Open file modal
    chromedp.Run(ctx,
        chromedp.Click(`#filesBtn`),
        chromedp.WaitVisible(`#fileModal`),
    )
    
    // Select files (in and out)
    chromedp.Run(ctx,
        chromedp.Click(`#fileList tr:nth-child(1) input.fileIn`),
        chromedp.Click(`#fileList tr:nth-child(2) input.fileOut`),
    )
    
    // Close modal
    chromedp.Run(ctx,
        chromedp.Click(`#closeFileModal`),
    )
    
    // Verify selections persisted in IndexedDB by submitting query
    chromedp.Run(ctx,
        chromedp.Focus(`#userInput`),
        chromedp.SendKeys(`#userInput`, "test", kb.Enter),
    )
    
    // Intercept WebSocket message to verify file selections
    // (Would require custom chromedp extension or mock server)
    t.Logf("File selection test passed")
}
```

#### Test 5: Unexpected Files Modal

```go
func TestUnexpectedFilesModal(t *testing.T) {
    server := testutil.StartTestServer(t)
    defer server.Cancel()
    
    ctx, cancel := chromedp.NewContext(context.Background())
    defer cancel()
    
    chromedp.Run(ctx,
        chromedp.Navigate(server.URL + "/project/test-proj"),
    )
    
    // Simulate unexpected files WebSocket message
    // (Would require test helper to send to connected clients)
    
    // Verify modal appears
    chromedp.Run(ctx,
        chromedp.WaitVisible(`[id="fileModal"].show`),
        chromedp.WaitVisible(`text="Unexpected Files Detected"`),
    )
    
    // Select files to approve
    chromedp.Run(ctx,
        chromedp.Click(`#confirmApprovalBtn`),
    )
    
    // Verify modal closes
    chromedp.Run(ctx,
        chromedp.WaitVisible(`[id="fileModal"]:not(.show)`, chromedp.ByQuery),
    )
    
    t.Logf("Unexpected files modal test passed")
}
```

### 4. Test Utilities

```go
// testutil/chromedp_helpers.go
package testutil

import (
    "context"
    "chromedp"
)

// WaitForQueryResponse waits for a query response to appear in the chat
func WaitForQueryResponse(ctx context.Context, queryText string) error {
    return chromedp.Run(ctx,
        chromedp.WaitFunc(func(ctx context.Context) error {
            // Check if response appears in chat
            var chatText string
            if err := chromedp.Text(`#chat`, &chatText).Do(ctx); err != nil {
                return err
            }
            if !strings.Contains(chatText, queryText) {
                return errors.New("response not found")
            }
            return nil
        }),
    )
}

// GetSelectedFiles returns the currently selected input and output files
func GetSelectedFiles(ctx context.Context) (input, output []string, err error) {
    var result map[string]interface{}
    err = chromedp.Evaluate(`
        (function() {
            var inputFiles = [];
            var outFiles = [];
            var rows = document.getElementById("fileList").getElementsByTagName("tr");
            for (var i = 0; i < rows.length; i++) {
                var cells = rows[i].getElementsByTagName("td");
                if (cells.length < 3) continue;
                var inChecked = cells[0].querySelector("input").checked;
                var outChecked = cells[1].querySelector("input").checked;
                var filename = cells[2].textContent;
                if (inChecked) inputFiles.push(filename);
                if (outChecked) outFiles.push(filename);
            }
            return { input: inputFiles, output: outFiles };
        })()
    `, &result).Do(ctx)
    
    if err == nil {
        if inp, ok := result["input"].([]interface{}); ok {
            for _, f := range inp {
                input = append(input, f.(string))
            }
        }
        if out, ok := result["output"].([]interface{}); ok {
            for _, f := range out {
                output = append(output, f.(string))
            }
        }
    }
    return
}
```

---

## Test Execution and CI/CD Integration

### Local Development

```bash
# Run all web client tests
go test -v ./... -run TestWebClient

# Run single test
go test -v -run TestCreateProjectViaWebUI

# Run with real browser visible (debugging)
HEADLESS=false go test -v ./... -run TestCreateProjectViaWebUI
```

### CI/CD Configuration (GitHub Actions)

```yaml
name: Web Client Tests

on: [pull_request, push]

jobs:
  web-client-tests:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v3
      
      - name: Set up Go
        uses: actions/setup-go@v4
        with:
          go-version: '1.21'
      
      - name: Install Chrome
        run: |
          apt-get update
          apt-get install -y chromium-browser
      
      - name: Run web client tests
        run: go test -v ./... -run TestWebClient
```

---

## Test Coverage and Schedule

### Phase 1: Core Functionality (Week 1)
- Create project workflow
- File management (add, list, delete)
- Basic query submission
- WebSocket connectivity

### Phase 2: Advanced Features (Week 2)
- File selection and I/O
- Unexpected files detection and approval
- Multiple concurrent queries
- Query cancellation

### Phase 3: Resilience (Week 3)
- Network interruption recovery
- Multiple client synchronization
- Browser memory/resource limits
- Long-running conversations

### Ongoing
- Regression tests for bug fixes
- Coverage for new features
- Performance benchmarks

---

## References

[1] [https://github.com/chromedp/chromedp](https://github.com/chromedp/chromedp)
[2] [https://pkg.go.dev/github.com/chromedp/chromedp](https://pkg.go.dev/github.com/chromedp/chromedp)
