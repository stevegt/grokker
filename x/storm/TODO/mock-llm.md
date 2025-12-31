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

Use a separate `llm/common` package for shared definitions (interfaces and config) to avoid circular imports. This mirrors the pattern in db/kv where interfaces live in their own subpackage:

```
llm/
├── common/
│   ├── interface.go        # Interface definitions (QueryLLM, TokenCounter)
│   └── config.go           # Configuration structs (LLMConfig, LLMBackend)
├── factory.go              # NewLLM() factory function
├── grokker/
│   └── adapter.go          # Implements QueryLLM, wraps grokker/v3/core
└── mock/
    ├── mock.go             # MockLLM implementing QueryLLM
    ├── templates.go        # Response templates for different scenarios
    └── builder.go          # Fluent builder for test setup
```

### Dependency Graph (No Circular Imports)

```
llm/common/
├── interface.go       ← QueryLLM, TokenCounter (NO imports from other llm packages)
└── config.go          ← LLMConfig, LLMBackend (NO imports from other llm packages)
    ↑
    ├─ llm/grokker/adapter.go  (imports llm/common only)
    ├─ llm/mock/mock.go        (imports llm/common only)
    └─ llm/factory.go          (imports llm/common, llm/grokker, llm/mock)
```

**Key principle:** Interfaces and config go in a separate package with NO imports from any other llm subpackages:
- `llm/common` - Pure definitions, zero dependencies on subpackages
- `llm/grokker/` - Only imports `llm/common`
- `llm/mock/` - Only imports `llm/common`
- `llm/factory.go` - Imports `llm/common` and all subpackages (safe, subpackages don't import each other)

## Configuration: Typed Struct (Not interface{})

Following Go best practices and the db.NewStore() pattern, use a typed configuration struct:

```go
// llm/common/config.go

type LLMBackend string

const (
    LLMBackendGrokker LLMBackend = "grokker"
    LLMBackendMock    LLMBackend = "mock"
)

// LLMConfig holds configuration for LLM creation
type LLMConfig struct {
    Backend        LLMBackend         // Which backend to use
    Grokker        *core.Grokker      // Required if Backend == LLMBackendGrokker
    MockTemplate   string             // Optional: which template to use for mock
    MockDelay      time.Duration      // Optional: simulated delay for mock
    MockErrorMode  string             // Optional: error simulation mode
}

// TokenCounterConfig holds configuration for TokenCounter creation
type TokenCounterConfig struct {
    Backend        LLMBackend
    Grokker        *core.Grokker      // Required if Backend == LLMBackendGrokker
    CountPerWord   float64            // Optional: for mock, tokens per word (default 0.75)
}
```

## Interface Definitions

```go
// llm/common/interface.go

type QueryLLM interface {
    SendWithFiles(ctx context.Context, llm string, sysmsg string, msgs []client.ChatMsg, 
        inputFiles []string, outFilesConverted []core.FileLang) (response string, usage *TokenUsage, err error)
}

type TokenCounter interface {
    TokenCount(text string) (int, error)
}

type TokenUsage struct {
    Tokens        int
    EstimatedUSD  float64
}
```

## Factory Function (Single Entry Point)

Single factory function taking typed config, like `db.NewStore()`:

```go
// llm/factory.go

import (
    "github.com/stevegt/grokker/x/storm/llm/common"
    "github.com/stevegt/grokker/x/storm/llm/grokker"
    "github.com/stevegt/grokker/x/storm/llm/mock"
)

// NewLLM creates an LLM implementation based on the specified backend
func NewLLM(config common.LLMConfig) (common.QueryLLM, error) {
    switch config.Backend {
    case common.LLMBackendGrokker:
        if config.Grokker == nil {
            return nil, fmt.Errorf("grokker backend requires Grokker config")
        }
        return grokker.NewGrokkerAdapter(config), nil
    
    case common.LLMBackendMock:
        return mock.NewMockLLM(config), nil
    
    default:
        return nil, fmt.Errorf("unknown LLM backend: %s", config.Backend)
    }
}

// NewTokenCounter creates a TokenCounter implementation based on backend
func NewTokenCounter(config common.TokenCounterConfig) (common.TokenCounter, error) {
    switch config.Backend {
    case common.LLMBackendGrokker:
        if config.Grokker == nil {
            return nil, fmt.Errorf("grokker backend requires Grokker config")
        }
        return grokker.NewTokenCounterAdapter(config), nil
    
    case common.LLMBackendMock:
        return mock.NewMockTokenCounter(config), nil
    
    default:
        return nil, fmt.Errorf("unknown token counter backend: %s", config.Backend)
    }
}
```

### Usage Examples

```go
// Production code
grok := core.New(...)
queryLLM, err := llm.NewLLM(common.LLMConfig{
    Backend: common.LLMBackendGrokker,
    Grokker: grok,
})

// Test code with default mock
mockLLM, err := llm.NewLLM(common.LLMConfig{
    Backend: common.LLMBackendMock,
})

// Test code with configured mock
mockLLM, err := llm.NewLLM(common.LLMConfig{
    Backend:       common.LLMBackendMock,
    MockTemplate:  "simple-file",
    MockDelay:     100 * time.Millisecond,
    MockErrorMode: "rate-limit",
})

// Environment-based selection
backend := common.LLMBackendGrokker
if os.Getenv("STORM_MOCK_LLM") == "true" {
    backend = common.LLMBackendMock
}
queryLLM, err := llm.NewLLM(common.LLMConfig{
    Backend: backend,
    Grokker: grok,
})
```

## File-by-File Breakdown

**`llm/common/interface.go`** - Interface definitions for the abstraction contract:
- `QueryLLM` interface for LLM query execution
- `TokenCounter` interface for token counting
- `TokenUsage` struct for response metadata
- **Critical:** No imports from other llm packages

**`llm/common/config.go`** - Configuration structs with typed fields:
- `LLMConfig` for QueryLLM factory
- `TokenCounterConfig` for TokenCounter factory
- `LLMBackend` enum constants
- Validation of required fields
- **Critical:** No imports from other llm packages (only stdlib)

**`llm/factory.go`** - NewLLM() and NewTokenCounter() factory functions:
- Backend selection via switch statement
- Configuration validation
- Returns implementations that satisfy interfaces from llm/common
- Imports: llm/common, llm/grokker, llm/mock (safe - subpackages don't import each other)

**`llm/grokker/adapter.go`** - Implements QueryLLM and TokenCounter interfaces:
- Wraps core.Grokker calls
- `NewGrokkerAdapter(config common.LLMConfig) common.QueryLLM`
- `NewTokenCounterAdapter(config common.LLMConfig) common.TokenCounter`
- Imports: only `llm/common` for interfaces and config
- Never imports from llm/mock or llm/factory

**`llm/mock/mock.go`** - MockLLM implementing QueryLLM interface with:
- Response mapping (query → response)
- Call logging for test assertions
- Error simulation modes
- Configurable delays
- `NewMockLLM(config common.LLMConfig) common.QueryLLM`
- `NewMockTokenCounter(config common.LLMConfig) common.TokenCounter`
- Imports: only `llm/common` for interfaces and config
- Never imports from llm/grokker or llm/factory

**`llm/mock/templates.go`** - Predefined response templates:
- TemplateSimpleFile
- TemplateMultipleFiles
- TemplateModifyFile
- TemplateLongResponse
- TemplateWithErrors
- Etc.

**`llm/mock/builder.go`** - Fluent builder for composable test setup:
```go
llm := llm.NewMockLLMBuilder().
    WithResponse("edit main.go", template).
    WithUnexpectedFiles("helper.go").
    WithErrorMode(mock.ErrorModeNetworkTimeout).
    Build()
```

## Why This Pattern Works

| Aspect | Benefit |
|--------|---------|
| Mirrors db/kv pattern | Interfaces in separate subpackage, just like db/kv |
| Typed config struct | Compiler catches missing fields at compile time |
| Explicit requirements | Factory signature shows all needed config |
| No interface{} magic | No type assertions, no surprises |
| Optional fields | Zero-values for optional parameters |
| Single entry point | Easy to find where LLM is created |
| Environment-based | Easy to switch production/mock via env var |
| Extensible | Adding new backends requires minimal changes |
| **NO circular imports** | llm/common has zero dependencies, safe for all to import |

## Suggested Enhancements

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
    ErrorMode             string             // simulate various failures
    ResponseDelay         time.Duration      // simulate network latency
    TokenUsageMultiplier  float64            // vary token usage
    ConditionalResponses  map[string]func... // dynamic response selection
}

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
queryLLM := mock.NewMockLLMBuilder().
    WithResponse("edit main.go", llm.TemplateSimpleFile).
    WithUnexpectedFiles("primes.go", "utils.go").
    WithTokenUsageMultiplier(1.5).
    Build()
```

### 5. **Token Counting Isolation**

Implement MockTokenCounter independently for speed:

```go
type MockTokenCounter struct {
    CountPerWord float64            // default: 1 word = 0.75 tokens
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
func NewTestLLMConfig() common.LLMConfig { /* returns mock config */ }
func AssertLLMWasCalled(t *testing.T, m *mock.MockLLM, expectedQuery string)
func AssertLLMCallCount(t *testing.T, m *mock.MockLLM, expected int)
func DumpLLMCallLog(t *testing.T, m *mock.MockLLM) string
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

1. **Phase 1** - Create llm/common/interface.go with interface definitions
2. **Phase 2** - Create llm/common/config.go with typed configuration structs
3. **Phase 3** - Create llm/factory.go with NewLLM() factory
4. **Phase 4** - Create llm/grokker/adapter.go wrapping grokker calls
5. **Phase 5** - Create llm/mock/ package with MockLLM implementation
6. **Phase 6** - Add test utilities to testutil package
7. **Phase 7** - Modify main.go to use llm.QueryLLM instead of direct grokker
8. **Phase 8** - Refactor existing tests to use llm.NewLLM(common.LLMConfig{Backend: common.LLMBackendMock})
9. **Phase 9** - Add comprehensive mock-based tests for edge cases

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