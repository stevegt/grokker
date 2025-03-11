package core

// ChatProvider defines an interface for LLM chat providers. It
// abstracts the functionality required for creating embeddings from
// text and for handling chat completions. This interface allows
// multiple providers, such as OpenAI or Perplexity.ai, to be used
// interchangeably within Grokker.
type ChatProvider interface {
	// CompleteChat takes a system message and a slice of chat
	// messages (ChatMsg) and returns the resulting chat response as a
	// string. The chat messages represent the conversation history.
	// The roles in each message should be set to the appropriate
	// value per the Provider's API.
	CompleteChat(sysmsg string, messages []ChatMsg) (string, error)
}
