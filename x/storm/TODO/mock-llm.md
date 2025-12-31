# LLM Mocking Architecture Plan

## Overview
Implement a pluggable LLM abstraction layer to enable comprehensive testing without relying on actual LLM API calls. This allows deterministic, fast, and cost-free testing of the entire query processing pipeline.

## Proposed Architecture (User's Outline)

1. **Move LLM-related code to `llm.go`** - Consolidate grokker library interactions
2. **Generic LLM interface** - Define a contract any LLM implementation must follow
3. **Grokker adapter** - Wrap `grokker/v3/client` and `grokker/v3/core` calls
4. **Factory pattern** - Centralized creation of LLM instances
5. **Mock LLM implementation** - Predefined responses for testing
6. **Integration with tests** - Use mock LLM in websocket_test.go and web_client_test.go

## Suggested Improvements

### 1. **LLM Interface Design**

Instead of a single broad interface, consider **two separate interfaces**:

```go
// High-level interface for query processing
type QueryLLM interface {
    SendWithFiles(ctx context.Context, llm string, sysmsg string, msgs []client.ChatMsg, 
        inputFiles []string, outFilesConverted []core.FileLang) (response string, usage *TokenUsage, err error)
}

// Lower-level interface for token counting
type TokenCounter interface {
    TokenCount(text string) (int, error)
}
```

**Rationale**: Token counting is simpler and reusable; query processing is complex. Separating them allows testing token counting independently.

### 2. **Deterministic Mock Responses**

The mock LLM should support **query-to-response mapping**:

```go
type MockLLM struct {
    Responses map[string]string  // query substring → response
    CallLog   []MockLLMCall     // record of all calls for assertions
    Delay     time.Duration     // optional: simulate network latency
    Failures  []string          // queries that should fail
}

type MockLLMCall struct {
    Query string
    LLM string
    InputFiles []string
    OutFiles []string
    Timestamp time.Time
}
```

**Why this approach:**
- Predictable: same query always gets same response
- Inspectable: can assert what queries were sent
- Flexible: supports testing error scenarios
- Observable: call log helps debug test failures

### 3. **Response Templates for Different Scenarios**

Provide predefined response templates for common test cases:

```go
// MockLLMResponseTemplate defines patterns for different test scenarios
type MockLLMResponseTemplate string

const (
    // Simple file creation response
    TemplateSimpleFile = `# Created file

package main

func main() {
    println("Hello, world!")
}
`

    // Multiple files response
    TemplateMultipleFiles = `# Multiple files created

package main

package main
`

    // Unexpected files in response
    TemplateUnexpectedFiles = `# Response with unexpected files

package main
`

    // File extraction with errors (missing files, broken files)
    TemplateWithErrors = `# Response with issues

package main

invalid go code here
`

    // Long response that exceeds token limit
    TemplateLongResponse = `# Very long response

[repeated content to exceed token limit]
`

    // Response with references and reasoning sections
    TemplateWithMetadata = `# Solution

## References

- [1] [https://example.com](https://example.com)

## Reasoning

<think>This is reasoning text</think>
`
)
```

### 4. **Factory Pattern with Dependency Injection**

```go
type LLMFactory interface {
    CreateLLM() QueryLLM
}

type ProductionLLMFactory struct {
    grokker *core.Grokker
}

type MockLLMFactory struct {
    template MockLLMResponseTemplate
    responses map[string]string
}

func NewLLMFactory(useMock bool, config ...interface{}) LLMFactory {
    if useMock {
        return &MockLLMFactory{}
    }
    return &ProductionLLMFactory{grokker: config[0].(*core.Grokker)}
}
```

### 5. **Enhanced Mock Capabilities**

Add features for realistic testing:

```go
type MockLLM struct {
    // ... existing fields ...
    
    // Simulate various error conditions
    ErrorMode ErrorMode // NONE, NETWORK_TIMEOUT, RATE_LIMIT, INVALID_RESPONSE
    
    // Simulate partial responses or streaming delays
    ResponseDelay time.Duration
    
    // Simulate varying token usage based on input
    TokenUsageMultiplier float64
    
    // Conditional response selection based on input
    ConditionalResponses map[string]func(query string) string
}

type ErrorMode int
const (
    ErrorModeNone ErrorMode = iota
    ErrorModeNetworkTimeout
    ErrorModeRateLimit
    ErrorModeInvalidResponse
    ErrorModePartialFailure  // some files succeed, some fail
)
```

### 6. **Test Scenario Builders**

Create fluent builders for complex test setups:

```go
// Example usage in tests:
llm := NewMockLLMBuilder().
    WithResponse("edit main.go", TemplateSimpleFile).
    WithResponse("add file", TemplateMultipleFiles).
    WithUnexpectedFiles("primes.go", "utils.go").
    WithErrorOnQuery("invalid query", ErrorModeNetworkTimeout).
    WithTokenUsageMultiplier(1.5).
    Build()
```

### 7. **Integration Points in Existing Code**

Modify `main.go` `sendQueryToLLM()` to accept an LLM parameter:

```go
// Before:
response, _, err := grok.SendWithFiles(llm, sysmsg, msgs, inputFiles, outFilesConverted)

// After:
response, usage, err := currentLLM.SendWithFiles(ctx, llm, sysmsg, msgs, inputFiles, outFilesConverted)
```

### 8. **Test Utilities**

Provide helper functions in testutil package:

```go
// testutil/mock_llm.go

func NewTestLLM() *MockLLM { /* ... */ }

func NewTestLLMWithFiles(files ...string) *MockLLM { /* ... */ }

func NewTestLLMWithError(err error) *MockLLM { /* ... */ }

func AssertLLMWasCalled(t *testing.T, llm *MockLLM, expectedQuery string)

func AssertLLMCallCount(t *testing.T, llm *MockLLM, expected int)

func DumpLLMCallLog(t *testing.T, llm *MockLLM) string // for debugging
```

### 9. **Token Counting Isolation**

Implement `TokenCounter` independently for speed:

```go
type MockTokenCounter struct {
    CountPerWord int  // default: 1 word = 0.75 tokens
    Cache map[string]int
}

func (m *MockTokenCounter) TokenCount(text string) (int, error) {
    if cached, ok := m.Cache[text]; ok {
        return cached, nil
    }
    
    words := len(strings.Fields(text))
    tokens := int(float64(words) * float64(m.CountPerWord) / 100)
    return tokens, nil
}
```

This allows testing token limit enforcement without calling the actual tokenizer.

### 10. **Configuration via Environment Variables**

```go
const (
    EnvUseMockLLM       = "STORM_MOCK_LLM"      // set to "true" to use mock
    EnvMockLLMTemplate  = "STORM_MOCK_TEMPLATE" // which template to use
    EnvMockLLMDelay     = "STORM_MOCK_DELAY"    // simulated network delay
    EnvMockLLMFailRate  = "STORM_MOCK_FAIL_RATE" // percentage of queries to fail
)
```

This allows:
- Running tests with real LLM for integration testing
- Quick tests with mock for CI/CD
- Stress testing by simulating failures

## Implementation Order

1. **Phase 1**: Create `llm.go` with interface definitions and Grokker adapter
2. **Phase 2**: Implement `MockLLM` with response templates
3. **Phase 3**: Add factory pattern to `main.go` and existing code paths
4. **Phase 4**: Add test utilities to testutil package
5. **Phase 5**: Refactor existing tests to use mock LLM
6. **Phase 6**: Add comprehensive mock-based tests for edge cases

## Testing Scenarios Enabled by Mock LLM

- ✅ Token limit exceeded (retry logic)
- ✅ Unexpected files detection and user approval flow
- ✅ File extraction with mixed success/failure
- ✅ Query cancellation mid-processing
- ✅ Concurrent queries with different responses
- ✅ Error recovery and retry logic
- ✅ WebSocket message ordering and delivery
- ✅ Timeout simulation without actual waiting
- ✅ Various file encoding scenarios
- ✅ Large response handling

## Benefits

| Aspect | Current | With Mock LLM |
|--------|---------|---------------|
| Test Speed | 30+ sec | <1 sec |
| Cost | Multiple API calls | $0 |
| Reliability | Rate limits, timeouts | Deterministic |
| Debugging | Black box | Full visibility |
| Edge Cases | Hard to trigger | Trivial to create |
| CI/CD | Flaky | Stable |

## Open Questions

1. Should mock LLM support streaming responses, or just complete responses?
2. Should we persist mock LLM call logs to disk for debugging?
3. Should production code have conditional logic based on LLM type, or should the abstraction be completely transparent?
4. How should we handle the `grok.TokenCount()` calls in `tokenCountHandler`? Mock those too?
