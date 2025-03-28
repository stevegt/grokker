# Migrating to a Multi-Provider Chat Client via a Factory Pattern

This document outlines a proposed strategy for transitioning our chat client implementation from a legacy OpenAI-specific design to a more flexible, multi-provider design that can support additional chat providers such as Perplexity. In this document, we discuss the overall migration approach, list some design alternatives with their pros and cons, and provide example code for a factory pattern implementation that demonstrates the desired migration behavior.

---

## 1. Background

Our current implementation is built around legacy code that directly interacts with OpenAI’s API. While a partial implementation exists for new providers (see `v3/client/chatclient.go`, `v3/openai/openai.go`, and `v3/perplexity/perplexity.go`), these are not fully integrated and the package structure may evolve. Our goal is to allow the system to support multiple providers while reusing the legacy code for OpenAI and incorporating the new design for Perplexity.

---

## 2. Design Alternatives

Several design patterns can be considered for migrating our chat client architecture:

### 2.1. Original Approach: Adapters/Structs

**Pros:**
- Strong type safety and compile-time checks.
- Clear, explicit contracts defined via interfaces.
- Straightforward testing by mocking the provider implementations.

**Cons:**
- Requires boilerplate conversion code between shared and provider-specific formats.
- Can become rigid if provider requirements diverge over time.

### 2.2. Message Interface

Passing objects that implement a `Message` interface can decouple the provider’s internal representations from our shared domain model.

**Pros:**
- Increased flexibility for handling diverse provider formats.
- Providers may work with their natively optimized message types.

**Cons:**
- Reduced compile-time safety; runtime type assertions may be necessary.
- May introduce performance overhead from dynamic dispatch.

### 2.3. Factory Pattern (Recommended)

Using a factory to instantiate chat clients based on a configuration or runtime choice centralizes initialization logic and cleanly separates provider concerns.

**Pros:**
- **Centralized Initialization:** All providers are constructed via a single creation method.
- **Encapsulation:** Provider-specific configurations are hidden within their respective factory branches.
- **Runtime Flexibility:** Easily switch between providers by selecting a different factory branch.

**Cons:**
- Introduces an extra layer of abstraction that might obscure misconfigurations if not well-logged.
- Requires maintaining a registry of supported providers, but the benefits in modularity outweigh the overhead.

### 2.4. Other Patterns

**Strategy Pattern:** Allows dynamic switching between algorithms (provider behaviors) at runtime; best when providers need truly interchangeable behavior.

**Decorator Pattern:** Enables composability, such as adding retry or logging features without altering core client logic.

**Builder Pattern:** Facilitates complex client configuration in a fluent manner, ensuring all parameters are validated before creation.

---

## 3. Factory Pattern Example Code

Below is an example implementation of a factory pattern designed for our chat client migration. In this example, the factory creates a chat client based on the requested provider name. For the "openai" provider, the factory calls legacy code (which we assume is encapsulated within an existing function such as `NewLegacyOpenAIClient()`), while for the "perplexity" provider the factory returns a new client as defined in our updated design.

```go
package chatclientfactory

import (
	"fmt"

	"github.com/stevegt/grokker/v3/client/chatclient"
	"github.com/stevegt/grokker/v3/openai"
	"github.com/stevegt/grokker/v3/perplexity"
)

// Provider constants
const (
	ProviderOpenAI     = "openai"
	ProviderPerplexity = "perplexity"
)

// ChatClientFactory centralizes the instantiation of provider clients.
type ChatClientFactory struct{}

// NewChatClientFactory returns a new instance of ChatClientFactory.
func NewChatClientFactory() *ChatClientFactory {
	return &ChatClientFactory{}
}

// Create instantiates a ChatClient based on the provider argument.
// For "openai", it calls the legacy code; for "perplexity", it calls the new provider implementation.
func (f *ChatClientFactory) Create(provider string, apiKey string, model string) (chatclient.ChatClient, error) {
	switch provider {
	case ProviderOpenAI:
		// Call the legacy code for OpenAI.
		// Assume NewLegacyOpenAIClient() wraps our legacy client functionality.
		client := openai.NewLegacyOpenAIClient(apiKey, model)
		return client, nil
	case ProviderPerplexity:
		// Call new code for Perplexity.
		client := perplexity.NewClient()
		return client, nil
	default:
		return nil, fmt.Errorf("unknown provider: %s", provider)
	}
}
```

**Usage Example:**

```go
// Example usage inside some higher level component:
func initializeChatClient() error {
	factory := NewChatClientFactory()

	// Suppose the configuration indicates which provider to use.
	provider := "openai" // or "perplexity"
	apiKey := "YOUR_API_KEY"
	model := "gpt-4"

	client, err := factory.Create(provider, apiKey, model)
	if err != nil {
		return err
	}

	// client now implements chatclient.ChatClient interface.
	response, err := client.CompleteChat(model, "You are a coding assistant. Please help.", []chatclient.ChatMsg{
		{Role: "USER", Content: "How do I implement the factory pattern in Go?"},
	})
	if err != nil {
		return err
	}
	fmt.Println("Chat response:", response)
	return nil
}
```

---

## 4. Next Steps & Conclusion

1. **Integrate the Factory:** Replace existing direct client creation code with the factory call so that the migration from legacy OpenAI to a provider-agnostic design is seamless.
2. **Testing:** Thoroughly test both the legacy path (OpenAI) and the new path (Perplexity) to ensure that the factory returns the correct implementation.
3. **Extendability:** In the future, additional providers can be added by augmenting the factory without changing high-level business logic.
4. **Documentation:** Update our overall architecture documentation to mention the adoption of the factory pattern and the rationale behind it.

By incorporating the factory pattern, we can gradually migrate our codebase, maintain backward compatibility with legacy systems, and pave the way for easily integrating future chat service providers.

---
  
This plan should serve as a roadmap for our chat client migration strategy, providing both the necessary design insights and a practical example to move forward with multi-provider support.
