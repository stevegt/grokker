# Storm Web Client Test Plan

## Overview

The Storm multi-project LLM chat application requires comprehensive end-to-end testing of the web client to validate:
- Project creation and file management workflows
- Real-time WebSocket communication with the server
- LLM query submission and response handling
- File extraction and output file handling
- UI state management and user interactions

This document compares two alternative testing approaches: Playwright-based browser testing and Go-based JavaScript engine mocking.

---

## Alternative 1: Playwright-Based Testing

### Description

Playwright is a modern browser automation framework that controls real browser instances (Chromium, Firefox, WebKit) to test web applications in a near-production environment[1]. Tests write user interactions in a familiar programming model while Playwright handles browser communication via the Chrome DevTools Protocol.

### Architecture

**Test Runner**: Playwright with TypeScript or JavaScript
**Browser Engine**: Real Chromium, Firefox, or WebKit instances
**Communication**: WebSocket connections to test server instance
**Assertions**: Playwright's built-in locators and matchers

### Implementation Approach

1. **Setup Phase**
   - Start Storm server (`storm serve`) on test port (e.g., 9999)
   - Initialize temporary database for test isolation
   - Launch Playwright browser instance
   - Navigate to `http://localhost:9999`

2. **Test Scenarios**[1]
   - **Project Management**: Create project → verify in sidebar → delete project
   - **File Operations**: Add files → list files → forget files → verify removal
   - **Query Execution**: Submit query → verify UI shows pending state → receive response → verify markdown rendering
   - **File Selection**: Select input/output files from UI → submit query → verify files are passed to LLM
   - **Real-Time Updates**: Open project in multiple browser tabs → verify file list syncs across tabs
   - **WebSocket Resilience**: Simulate network interruption → verify automatic reconnection

3. **Example Test Structure**
```typescript
import { test, expect } from '@playwright/test';

test('Create project and add files', async ({ page }) => {
  await page.goto('http://localhost:9999');

  // Create project
  await page.click('button:has-text("Add Project")');
  await page.fill('input[name="projectID"]', 'test-project');
  // ... fill form fields
  await page.click('button:has-text("Submit")');

  // Verify project appears in sidebar
  await expect(page.locator('text=test-project')).toBeVisible();

  // Add files
  await page.click('text=test-project');
  await page.click('button:has-text("Add Files")');
  // ... select files

  // Verify file list updated
  await expect(page.locator('text=filename.csv')).toBeVisible();
});
```

### Advantages

- **Real Browser Environment**[1]: Tests run in actual browsers, catching real browser compatibility issues
- **High Fidelity**: WebSocket communication is genuine; real network effects tested
- **Visual Regression**: Can capture screenshots for UI regression testing
- **Familiar Testing Model**: Looks like traditional browser automation (Selenium, Cypress)
- **Well-Supported**: Mature framework with extensive documentation
- **Parallel Execution**: Tests run in parallel across multiple browser instances
- **Debugging**: Full browser devtools integration; pause and inspect in real browser

### Disadvantages

- **Slow Execution**[2]: Real browser startup and interaction is slower; typical test run 30-60 seconds
- **Resource Heavy**: Each test instance requires full browser process (100-200MB per instance)
- **CI/CD Complexity**: Requires display server (X11) or container headless mode configuration
- **Flakiness**: Network timing, rendering delays can cause flaky tests ("sleep and retry" antipattern)
- **Setup Overhead**: Browser downloads, caching, cleanup between tests
- **Server Dependency**: Full Storm server must be running; harder to test server behavior changes

### Test Maintenance Effort

- Moderate - Playwright selectors are stable and well-documented
- Regular updates needed when UI structure changes
- Screenshot-based assertions require manual review of changes

### Cost and Resources

- **Time**: ~40-60 seconds per test suite run
- **CI/CD Minutes**: Significant (useful for final integration testing, not frequent commits)
- **Disk Space**: ~500MB+ for browsers and cache
- **Network**: WebSocket testing requires real network stack

---

## Alternative 2: Go-Based JavaScript Engine Mocking

### Description

Instead of running real browsers, use a Go HTTP client library combined with a client-side JavaScript engine to simulate browser behavior. The JavaScript engine (WASM or Go-based VM) executes the Storm web client's JavaScript, while a Go HTTP client mocks network requests and WebSocket connections[2].

### Architecture

**Test Runner**: Go testing framework (`testing` or `testify`)
**JavaScript Engine**: chromedp (headless Chrome via Go), or goja (pure Go JavaScript VM)
**HTTP/WebSocket Mocking**: Go net/http with custom WebSocket server, or httptest
**Browser State**: In-memory DOM simulation via JavaScript engine

### Implementation Approach

Option A: **chromedp (Headless Chrome via DevTools Protocol)**
```go
// chromedp still requires Chrome binary, but runs without display
ctx, cancel := chromedp.NewContext(context.Background())
defer cancel()

chromedp.Run(ctx,
  chromedp.Navigate(`http://localhost:9999`),
  chromedp.Click(`button[name="submit"]`),
  chromedp.WaitVisible(`.project-list`),
)
```

Option B: **goja (Pure Go JavaScript VM)**
```go
// Execute JavaScript directly in process, mock DOM and network APIs
vm := goja.New()
vm.SetFieldNameMapper(goja.TagFieldNameMapper("json", true))

// Mock fetch/WebSocket APIs
vm.Set("fetch", mockFetch)
vm.Set("WebSocket", mockWebSocket)

// Load and execute client-side JavaScript
clientJS, _ := ioutil.ReadFile("project.html")
vm.RunString(string(clientJS))

// Trigger user interactions programmatically
vm.RunString("getSelectedFiles()")
```

Option C: **httptest + Custom WebSocket Mock**
```go
// Use Go's httptest for HTTP responses and mock WebSocket
mux := http.NewServeMux()
mux.HandleFunc("/", serveHTML)
mux.HandleFunc("/api/projects", handleProjectsAPI)
mux.HandleFunc("/ws", mockWebSocket)

server := httptest.NewServer(mux)
defer server.Close()

// Use Go HTTP client to make requests, parse responses
client := &http.Client{}
resp, _ := client.Get(server.URL + "/api/projects")
// ... parse JSON response
```

### Test Scenarios

Tests would focus on validating client-side logic:
1. **State Management**: Verify JavaScript state changes correctly after API responses
2. **Request/Response Handling**: Validate WebSocket messages are formatted correctly
3. **Event Handling**: Verify button clicks trigger correct actions
4. **Error Handling**: Test client behavior when API returns errors

### Example Test Structure (Option B: goja)
```go
func TestFileSelection(t *testing.T) {
  vm := goja.New()

  // Mock DOM elements
  vm.Set("document", map[string]interface{}{
    "getElementById": mockGetElementById,
  })

  // Mock network APIs
  vm.Set("fetch", mockFetch)

  // Load client code
  clientCode, _ := ioutil.ReadFile("project.html")
  vm.RunString(string(clientCode))

  // Trigger user action
  result, err := vm.RunString("getSelectedFiles()")
  require.NoError(t, err)

  // Verify result
  files := result.Export().(map[string]interface{})
  assert.Contains(t, files["inputFiles"], "test.csv")
}
```

### Advantages

- **Fast Execution**[2]: No browser startup; tests complete in milliseconds to seconds
- **Lightweight**: Process-per-test uses minimal memory (< 50MB)
- **Deterministic**: No timing issues; execution is synchronous
- **CI/CD Friendly**: Runs anywhere Go runs; no display server needed
- **Parallel Testing**: Hundreds of tests can run simultaneously
- **Debugging**: Standard Go debugging tools; easy to set breakpoints
- **Mocking Flexibility**: Complete control over server responses and timing
- **Cost Efficient**: Low resource usage; suitable for frequent test runs

### Disadvantages

- **Limited Real Browser Testing**[2]: Can't catch real browser bugs (HTML5 API differences, CSS rendering)
- **JavaScript Engine Limitations**: goja (pure Go VM) doesn't support all JavaScript features; may miss runtime errors
- **WebSocket Simulation**: Mock WebSocket doesn't fully replicate network behavior (TCP retransmit, backpressure)
- **DOM Simulation**: Limited HTML/CSS rendering; can't verify visual layout or responsive design
- **Learning Curve**: Developers must understand JavaScript engine internals or chromedp protocol
- **Maintenance**: Mock APIs must be kept in sync with real server API changes
- **False Confidence**: Tests might pass but fail in real browser

### Test Maintenance Effort

- Low maintenance for unit-like tests (minimal API changes)
- Moderate effort for mock server updates when API contracts change
- No screenshot maintenance or visual regression handling

### Cost and Resources

- **Time**: ~100-500ms per test (very fast)
- **CI/CD Minutes**: Minimal (suitable for every commit)
- **Disk Space**: ~50MB (just Go binaries)
- **Network**: Mock network; fully controllable timing

---

## Comparison Summary

| Criterion | Playwright | Go Mock Engine |
|-----------|-----------|----------------|
| **Execution Speed** | 40-60 sec/suite | 100-500ms/suite |
| **Real Browser Coverage** | ✅ Yes | ❌ No |
| **WebSocket Fidelity** | ✅ Real | ⚠️ Mocked |
| **Resource Intensity** | High (100-200MB) | Low (< 50MB) |
| **CI/CD Friendliness** | ⚠️ Complex setup | ✅ Simple |
| **Parallel Execution** | Limited | ✅ High |
| **Learning Curve** | Low | Medium-High |
| **Maintenance Burden** | Moderate | Low-Moderate |
| **Visual Regression** | ✅ Yes | ❌ No |
| **Network Edge Cases** | ✅ Real | ⚠️ Mocked |

---

## Recommendation: Hybrid Approach

**Implement both strategies** for maximum coverage:

1. **Go Mock Engine Tests (Primary)**
   - Unit and integration tests for client-side logic
   - Run on every commit (fast feedback loop)
   - Target 80%+ code coverage
   - Test error paths, edge cases, state management

2. **Playwright Tests (Integration/Regression)**
   - End-to-end workflow validation
   - Run on pull request or before release
   - Focus on critical user paths
   - Include visual regression checks
   - Test real browser compatibility

This hybrid approach balances **development velocity** (fast Go tests) with **confidence** (real browser tests before release)[1][2].

---

## Implementation Timeline

### Phase 1: Go Mock Engine Setup (Week 1)
- Choose JavaScript engine (chromedp for real browser fidelity, goja for speed)
- Implement mock HTTP/WebSocket server
- Create 10-15 basic client-side logic tests
- Integrate with CI/CD

### Phase 2: Playwright Setup (Week 2)
- Set up Playwright test infrastructure
- Write 5-7 critical user flow tests
- Configure CI/CD for headless mode
- Add screenshot capture for regression detection

### Phase 3: Expansion (Ongoing)
- Add more tests based on bug reports
- Increase coverage targets
- Refine mock APIs based on production issues
- Document test patterns for team

## References

[1] [https://playwright.dev/docs/intro](https://playwright.dev/docs/intro)
[2] [https://github.com/chromedp/chromedp](https://github.com/chromedp/chromedp)
