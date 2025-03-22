package client

// ChatClient defines the interface for chat operations.
// Implementations of ChatClient (such as OpenAIChatClient and PerplexityChatClient)
// must implement this method to generate a complete chat response.
type ChatClient interface {
	CompleteChat(model, sysmsg string, messages []ChatMsg) (string, error)
}

// ChatMsg represents a single chat message.
type ChatMsg struct {
	Role    string
	Content string
}
