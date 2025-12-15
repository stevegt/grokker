
# Testing Unexpected Files Feature Without Playwright: Alternative Plans

Comprehensive automated testing for the unexpected files flow can be achieved primarily in Go without introducing Playwright and node_modules overhead. Here are several viable alternatives, each with specific strengths for different testing layers[1][2].

## Plan A: Go Integration Tests with Mock WebSocket and HTTP Server

**Approach**[1]
- Create a test HTTP server using Go's `net/http/httptest` package
- Mock the entire Huma API and WebSocket endpoints
- Use `github.com/gorilla/websocket` test utilities to simulate WebSocket connections
- Test the full query-to-file-extraction flow in Go

**Implementation Strategy**:
```go
// Start test server with mocked handlers
server := httptest.NewServer(chiRouter)
defer server.Close()

// Connect WebSocket client
wsURL := "ws" + server.URL[4:] + "/project/test/ws"
conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)

// Send query message, read response, verify behavior
```

**Pros**[1]:
- Entire test suite runs in Go; no external dependencies
- Tests run quickly (seconds, not minutes)
- Can easily mock database, file I/O, LLM responses
- Natural integration with existing `go test` framework
- Can test concurrency, race conditions, and timing-sensitive behavior
- Full control over WebSocket message ordering and timing

**Cons**[2]:
- WebSocket mocking can be complex; test code may become brittle if implementation details change
- No visual verification; can't see if UI actually renders correctly
- Requires detailed knowledge of WebSocket protocol and frame structure
- Browser-specific bugs won't be caught

**Best For**: Testing the backend query flow, file categorization logic, state management, and WebSocket message handling

---

## Plan B: Go CLI Blackbox Tests with Test Fixtures

**Approach**[1]
- Create integration tests that invoke the Storm CLI binaries directly
- Use temporary directories as test fixtures for projects and files
- Verify behavior through file system inspection and stdout/stderr analysis
- Test the complete end-to-end CLI flow without a running server

**Implementation Strategy**:
```go
// Create temporary project structure
tmpDir := t.TempDir()
projectDir := filepath.Join(tmpDir, "project")
os.MkdirAll(projectDir, 0755)

// Run CLI commands
cmd := exec.Command("./storm", "project", "add", "test-proj", projectDir, "chat.md")
err := cmd.Run()

// Verify results on disk
files, err := os.ReadDir(projectDir)
// Assert expected files exist or don't exist
```

**Pros**[1][2]:
- Tests the actual CLI behavior, not just library functions
- File system state is easily verifiable and inspectable
- Can test error conditions and edge cases (missing files, permission errors, etc.)
- Tests run independently; no server coordination needed
- Compatible with CI/CD without additional setup

**Cons**:
- Slower than unit tests (requires process spawning)
- Can't test WebSocket interactions directly
- File-based assertions can be fragile if system has unexpected state
- Harder to test race conditions or concurrent operations

**Best For**: Testing CLI commands (project add/forget, file add/forget), file system operations, and command-line error handling

---

## Plan C: Contract Tests on API Layer with JSON Verification

**Approach**[2]
- Test HTTP API endpoints directly using `net/http` client
- Send JSON payloads and verify response JSON structure and values
- Ensure API contracts are maintained (request/response formats don't break)
- Test error responses and edge cases

**Implementation Strategy**:
```go
// Test file add endpoint
payload := map[string]interface{}{
    "filenames": []string{"/absolute/path/file.go"},
}
jsonBody, _ := json.Marshal(payload)

req, _ := http.NewRequest("POST",
    fmt.Sprintf("http://localhost:8080/api/projects/test/files/add"),
    bytes.NewReader(jsonBody))
client := &http.Client{}
resp, _ := client.Do(req)

// Verify response
var result map[string]interface{}
json.NewDecoder(resp.Body).Decode(&result)
assert.Equal(t, http.StatusOK, resp.StatusCode)
assert.NotNil(t, result["added"])
```

**Pros**[1]:
- Tests actual HTTP endpoints; very close to real usage
- Verifies API contract changes don't break compatibility
- Can test with real database and file I/O (not mocked)
- Catches serialization/deserialization bugs

**Cons**:
- Requires running server; more setup per test
- Slower test execution
- Hard to mock or control internal behavior
- Can't easily test network failures or timing issues

**Best For**: Testing API stability, request/response format verification, and end-to-end API workflows

---

## Plan D: Go Unit Tests for Business Logic with Focused Mocks

**Approach**[1]
- Create unit tests for core logic (file categorization, path resolution, etc.)
- Mock only the external dependencies (database, file system, LLM)
- Keep tests fast by avoiding HTTP/WebSocket layers
- Focus on correctness of algorithms and state management

**Implementation Strategy**:
```go
// Test file categorization logic
type MockProject struct {
    AuthorizedFiles []string
}

func TestCategorizeUnexpectedFiles(t *testing.T) {
    project := &MockProject{
        AuthorizedFiles: []string{"/path/file1.go", "/path/file2.md"},
    }
    unexpected := []string{"/path/file1.go", "/path/file3.py"}

    alreadyAuthorized, needsAuth := categorizeFiles(project, unexpected)
    assert.Equal(t, []string{"/path/file1.go"}, alreadyAuthorized)
    assert.Equal(t, []string{"/path/file3.py"}, needsAuth)
}
```

**Pros**[1][2]:
- Tests run extremely fast (milliseconds)
- Easy to test edge cases and error conditions
- Highly focused; failures clearly indicate which component broke
- No external dependencies; reliable on any machine

**Cons**:
- Only tests individual components in isolation
- Doesn't catch integration issues
- Requires maintaining mock implementations as code evolves
- May miss real-world bugs that only appear when components interact

**Best For**: Testing categorization logic, path resolution, file matching algorithms, and state transitions

---

## Plan E: Hybrid Approach (Recommended)

**Combine all four strategies** for comprehensive coverage[1][2]:

1. **Unit Tests (Plan D)**: Core business logic - run on every commit
   - Fast feedback for developers
   - Focus: categorization, path resolution, data transformation

2. **Contract Tests (Plan C)**: API contracts - run on every commit
   - Verify request/response formats
   - Catch API breaking changes early
   - Database integration tests

3. **Integration Tests (Plan A)**: Query flow with mocked WebSocket - run on every commit
   - Full query-to-completion flow
   - File extraction and re-extraction scenarios
   - WebSocket message handling

4. **CLI Blackbox Tests (Plan B)**: End-to-end CLI - run before release
   - Verify actual binary behavior
   - File system integration
   - User-facing error messages

**Test Execution Strategy**:
```bash
# Fast suite (runs in CI on every commit - ~30 seconds)
make test-fast  # Plans D + C + A

# Full suite (runs before release - ~2 minutes)
make test       # Plans D + C + A + B

# Development (run locally while coding)
go test -v ./...  # Plan D only (fastest)
```

**Coverage Distribution**:
- Unit tests (Plan D): ~40% of test time, 60% of code coverage
- Contract tests (Plan C): ~30% of test time, 30% of API coverage
- Integration tests (Plan A): ~25% of test time, WebSocket/async coverage
- CLI tests (Plan B): ~5% of test time, end-to-end validation

## Why This Avoids Playwright

**All testing happens in Go** without requiring Node.js, JavaScript, or browser automation:
- No `npm install` overhead
- No browser binaries to download
- Faster CI/CD pipelines
- Simpler local development environment
- Single language across codebase

The WebSocket layer is the most critical to test properly. **Plan A handles this thoroughly using Go's native WebSocket testing capabilities**. You don't need browser automation to verify WebSocket behavior—Go's `net` package and test utilities are sufficient[1][2].

**When You Might Add Playwright Later**:
Only if you need to test:
- Visual rendering and CSS
- Browser-specific JavaScript bugs
- Accessibility with screen readers
- Mobile viewport interactions

For testing the unexpected files feature itself, Plans A-E are sufficient.

## References

[1] [https://golang.org/pkg/net/http/httptest/](https://golang.org/pkg/net/http/httptest/)
[2] [https://golang.org/pkg/testing/](https://golang.org/pkg/testing/)
