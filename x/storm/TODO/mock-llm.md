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
├── factory.go             # LLMFactory function: NewLLM(mode, config)
├── grokker/
│   └── adapter.go         # GrokkerAdapter wrapping grokker/v3/core
└── mock/
    ├── mock.go            # MockLLM implementation
    ├── templates.go       # Response templates for different scenarios
    └── builder.go         # Fluent builder for test setup
```

## Factory Pattern: Single Entry Point

Following the db.NewStore() pattern, use a single factory function with a mode/backend argument:

```go
// llm/factory.go

type LLMBackend string

const (
    LLMBackendGrokker LLMBackend = "grokker"
    LLMBackendMock    LLMBackend = "mock"
)

// NewLLM creates an LLM implementation based on the specified backend
func NewLLM(backend LLMBackend, config ...interface{}) (QueryLLM, error) {
    switch backend {
    case LLMBackendGrokker:
        // config[0] should be *core.Grokker instance
        if len(config) == 0 {
            return nil, fmt.Errorf("grokker backend requires *core.Grokker config")
        }
        grok, ok := config[0].(*core.Grokker)
        if !ok {
            return nil, fmt.Errorf("invalid grokker config type")
        }
        return grokker.NewGrokkerAdapter(grok), nil
    
    case LLMBackendMock:
        // config[0] is optional MockLLMBuilder or predefined template
        if len(config) == 0 {
            return mock.NewMockLLM(), nil
        }
        // Could accept a builder or template as config
        return config[0].(QueryLLM), nil
    
    default:
        return nil, fmt.Errorf("unknown LLM backend: %s", backend)
    }
}

// NewTokenCounter creates a TokenCounter implementation based on backend
func NewTokenCounter(backend LLMBackend, config ...interface{}) (TokenCounter, error) {
    switch backend {
    case LLMBackendGrokker:
        if len(config) == 0 {
            return nil, fmt.Errorf("grokker backend requires grokker config")
        }
        grok, ok := config[0].(*core.Grokker)
        if !ok {
            return nil, fmt.Errorf("invalid grokker config type")
        }
        return grokker.NewTokenCounterAdapter(grok), nil
    
    case LLMBackendMock:
        return mock.NewMockTokenCounter(), nil
    
    default:
        return nil, fmt.Errorf("unknown token counter backend: %s", backend)
    }
}
```

### Usage Examples

```go
// Production code
grok := core.New(...)
queryLLM, _ := llm.NewLLM(llm.LLMBackendGrokker, grok)

// Test code
mockLLM, _ := llm.NewLLM(llm.LLMBackendMock)

// Environment-based selection
backend := llm.LLMBackendGrokker
if os.Getenv("STORM_MOCK_LLM") == "true" {
    backend = llm.LLMBackendMock
}
queryLLM, _ := llm.NewLLM(backend, grok)
```

### Interface Definitions

```go
// llm/interface.go

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

## File-by-File Breakdown

**`llm/interface.go`** - Interface definitions for the abstraction contract

**`llm/factory.go`** - Single NewLLM() factory function with backend selection:
- LLMBackendGrokker - wraps actual grokker library
- LLMBackendMock - uses mock implementation

**`llm/grokker/adapter.go`** - Implements QueryLLM interface wrapping core.Grokker

**`llm/mock/mock.go`** - MockLLM implementing QueryLLM interface with:
- Response mapping (query → response)
- Call logging for test assertions
- Error simulation modes
- Configurable delays

**`llm/mock/templates.go`** - Predefined response templates:
- TemplateSimpleFile
- TemplateMultipleFiles
- TemplateModifyFile
- TemplateLongResponse
- etc.

**`llm/mock/builder.go`** - Fluent builder for composable test setup:
```go
llm := llm.NewMockLLMBuilder().
    WithResponse("edit main.go", template).
    WithUnexpectedFiles("helper.go").
    WithErrorOnQuery("invalid", ErrorModeNetworkTimeout).
    Build()
```

## Why This Pattern Works

| Aspect | Benefit |
|--------|---------|
| Single entry point | Easy to find where LLM is created |
| Mode/backend argument | Consistent with db.NewStore() pattern |
| No naming chaos | One NewLLM(), not NewMockLLM(), NewRealLLM(), etc. |
| Environment-based | Easy to switch production/mock via env var |
| Extensible | Adding new backends requires only NewLLM() changes |

## Suggested Improvements

### 1. **Deterministic Mock Responses**

```go
type MockLLM struct {
    Responses map[string]string  // query substring → response
    CallLog   []MockLLMCall      // track all calls for assertions
    Delay     time.Duration      // simulate network latency
    Failures  []string           // queries that should fail
}

type MockLLMCall struct {
    Query     string
    LLM       string
    InputFiles []string
    OutFiles  []string
    Timestamp time.Time
}
```

### 2. **Response Templates for Different Scenarios**

Predefined responses for:
- Simple file creation
- Multiple files
- File modifications
- Long responses exceeding token limit
- Responses with references and reasoning sections

### 3. **Enhanced Mock Capabilities**

```go
type MockLLM struct {
    ErrorMode             ErrorMode          // simulate various failures
    ResponseDelay         time.Duration      // simulate network latency
    TokenUsageMultiplier  float64            // vary token usage
    ConditionalResponses  map[string]func... // dynamic response selection
}

type ErrorMode int
const (
    ErrorModeNone ErrorMode = iota
    ErrorModeNetworkTimeout
    ErrorModeRateLimit
    ErrorModeInvalidResponse
    ErrorModePartialFailure
)
```

### 4. **Test Scenario Builders**

Fluent API for complex test setups:

```go
llm := llm.NewMockLLMBuilder().
    WithResponse("edit main.go", llm.TemplateSimpleFile).
    WithUnexpectedFiles("primes.go", "utils.go").
    WithTokenUsageMultiplier(1.5).
    Build()
```

### 5. **Token Counting Isolation**

Implement MockTokenCounter independently for speed:

```go
type MockTokenCounter struct {
    CountPerWord int              // default: 1 word = 0.75 tokens
    Cache        map[string]int
}

func (m *MockTokenCounter) TokenCount(text string) (int, error)
```

### 6. **Configuration via Environment Variables**

```go
const (
    EnvLLMBackend      = "STORM_LLM_BACKEND"    // "grokker" or "mock"
    EnvMockTemplate    = "STORM_MOCK_TEMPLATE"  // which template to use
    EnvMockDelay       = "STORM_MOCK_DELAY"     // simulated delay
    EnvMockFailRate    = "STORM_MOCK_FAIL_RATE" // failure percentage
)
```

### 7. **Test Utilities**

Provide helpers in testutil package:

```go
func NewTestLLM() QueryLLM { /* default mock */ }
func NewTestLLMWithError(err error) QueryLLM
func AssertLLMWasCalled(t *testing.T, llm *mock.MockLLM, expectedQuery string)
func AssertLLMCallCount(t *testing.T, llm *mock.MockLLM, expected int)
func DumpLLMCallLog(t *testing.T, llm *mock.MockLLM) string
```

### 8. **Integration with Existing Code**

Modify `main.go` to accept LLM parameter:

```go
// Before
response, _, err := grok.SendWithFiles(llm, sysmsg, msgs, inputFiles, outFilesConverted)

// After
response, usage, err := currentLLM.SendWithFiles(ctx, llm, sysmsg, msgs, inputFiles, outFilesConverted)
```

## Implementation Order

1. **Phase 1** - Create llm/interface.go with interface definitions
2. **Phase 2** - Create llm/factory.go with NewLLM() factory
3. **Phase 3** - Create llm/grokker/adapter.go wrapping grokker calls
4. **Phase 4** - Create llm/mock/ package with MockLLM implementation
5. **Phase 5** - Add test utilities to testutil package
6. **Phase 6** - Modify main.go to use llm.QueryLLM instead of direct grokker
7. **Phase 7** - Refactor existing tests to use NewLLM(llm.LLMBackendMock)
8. **Phase 8** - Add comprehensive mock-based tests for edge cases

## Testing Scenarios Enabled by Mock LLM

✅ Token limit exceeded (retry logic)  
✅ Unexpected files detection and user approval  
✅ File extraction with mixed success/failure  
✅ Query cancellation mid-processing  
✅ Concurrent queries with different responses  
✅ Error recovery and retry logic  
✅ WebSocket message ordering and delivery  
✅ Timeout simulation without actual waiting  
✅ Various file encoding scenarios  
✅ Large response handling  

## Benefits

| Aspect | Current | With Mock LLM |
|--------|---------|---------------|
| Test Speed | 30+ sec | <1 sec |
| Cost | Multiple API calls | $0 |
| Reliability | Rate limits, timeouts | Deterministic |
| Debugging | Black box | Full visibility |
| Edge Cases | Hard to trigger | Trivial to create |
| CI/CD | Flaky | Stable |
