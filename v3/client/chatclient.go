package client

// ChatClient defines the interface for chat operations.
// Implementations of ChatClient (such as OpenAIChatClient and PerplexityChatClient)
// must implement this method to generate a complete chat response.
type ChatClient interface {
	CompleteChat(model string, messages []ChatMsg) (Results, error)
}

// ChatMsg represents a single chat message.
type ChatMsg struct {
	Role    string
	Content string
}

// Results represents the results of a chat operation.
type Results struct {
	Body      string
	Citations []string
}
