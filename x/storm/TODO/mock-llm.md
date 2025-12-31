# LLM Mocking Architecture Plan

## Overview
Implement a pluggable LLM abstraction layer to enable comprehensive testing without relying on actual LLM API calls. This allows deterministic, fast, and cost-free testing of the entire query processing pipeline.

## Proposed Architecture (User's Outline)

1. **Move LLM-related code to `llm/` package** - Consolidate grokker library interactions
2. **Generic LLM interface** - Define a contract any LLM implementation must follow
3. **Grokker adapter** - Wrap `grokker/v3/client` and `grokker/v3/core` calls
4. **Factory pattern** - Centralized creation of LLM instances
5. **Mock LLM implementation** - Predefined responses for testing
6. **Integration with tests** - Use mock LLM in websocket_test.go and web_client_test.go

## Recommended Directory Structure

Mirror the db/* package structure for consistency:

```
llm/
├── interface.go           # Interface definitions (QueryLLM, TokenCounter)
├── factory.go             # LLMFactory and NewLLM() function
├── grokker/
│   └── adapter.go         # GrokkerAdapter wrapping grokker/v3/core
└── mock/
    ├── mock.go            # MockLLM implementation
    ├── templates.go       # Response templates for different scenarios
    └── builder.go         # Fluent builder for test setup
```

### File-by-File Breakdown

**`llm/interface.go`** - Define the contract:
```go
type QueryLLM interface {
    SendWithFiles(ctx context.Context, llm string, sysmsg string, msgs []client.ChatMsg, 
        inputFiles []string, outFilesConverted []core.FileLang) (response string, usage *TokenUsage, err error)
}

type TokenCounter interface {
    TokenCount(text string) (int, error)
}

type TokenUsage struct {
    Tokens     int
    EstimatedUSD float64
}
```

**`llm/factory.go`** - Factory pattern with dependency injection:
```go
func NewLLM(useMock bool, config ...interface{}) QueryLLM
func NewTokenCounter(useMock bool, config ...interface{}) TokenCounter
```

**`llm/grokker/adapter.go`** - Wraps actual grokker library:
```go
type GrokkerAdapter struct {
    grok *core.Grokker
}

func (g *GrokkerAdapter) SendWithFiles(...) (string, *TokenUsage, error)
```

**`llm/mock/mock.go`** - Mock implementation:
```go
type MockLLM struct {
    Responses map[string]string
    CallLog   []MockLLMCall
    ErrorMode ErrorMode
    // ... other fields
}

func (m *MockLLM) SendWithFiles(...) (string, *TokenUsage, error)
```

**`llm/mock/templates.go`** - Predefined responses:
```go
const (
    TemplateSimpleFile MockLLMResponseTemplate = "..."
    TemplateMultipleFiles = "..."
    TemplateUnexpectedFiles = "..."
    // ... other templates
)
```

**`llm/mock/builder.go`** - Fluent test setup:
```go
type MockLLMBuilder struct { ... }

func (b *MockLLMBuilder) WithResponse(query, response string) *MockLLMBuilder
func (b *MockLLMBuilder) WithUnexpectedFiles(files ...string) *MockLLMBuilder
func (b *MockLLMBuilder) Build() *MockLLM
```

## Why Mirror db/* Structure

✅ **Consistency** - Same pattern used in the codebase  
✅ **Familiarity** - Developers understand the architecture immediately  
✅ **Maintainability** - Clear separation of concerns (interface, adapter, factory, mock)  
✅ **Extensibility** - Adding new LLM providers later is straightforward  
✅ **Testability** - Clean abstraction boundaries make testing easy

## Suggested Improvements

### 1. **LLM Interface Design**

Separate concerns into **two interfaces**:

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

    // Response with file modifications
    TemplateModifyFile = `# Edit main.go to call helper function

package main

func main() {
    helper()
}

func helper() {
    println("Helper function")
}
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

code here
`
)
```

### 4. **Factory Pattern with Dependency Injection**

```go
type LLMFactory interface {
    CreateQueryLLM() QueryLLM
    CreateTokenCounter() TokenCounter
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
llm := llm.NewMockLLMBuilder().
    WithResponse("edit main.go", llm.TemplateSimpleFile).
    WithResponse("add file", llm.TemplateMultipleFiles).
    WithUnexpectedFiles("primes.go", "utils.go").
    WithErrorOnQuery("invalid query", llm.ErrorModeNetworkTimeout).
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

1. **Phase 1**: Create llm/interface.go with interface definitions
2. **Phase 2**: Create llm/grokker/adapter.go wrapping existing grokker calls
3. **Phase 3**: Create llm/mock/ package with MockLLM implementation
4. **Phase 4**: Create llm/factory.go with factory pattern
5. **Phase 5**: Add test utilities to testutil package
6. **Phase 6**: Modify main.go to use llm.QueryLLM interface instead of direct grokker calls
7. **Phase 7**: Refactor existing tests to use mock LLM
8. **Phase 8**: Add comprehensive mock-based tests for edge cases

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
