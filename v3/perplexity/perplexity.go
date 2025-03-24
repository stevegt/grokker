package perplexity

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"strings"

	"github.com/stevegt/grokker/v3/client"
)

// Client encapsulates the API client for Perplexity.ai.
// This client implements the ChatClient interface (as defined in the client package)
// for generating chat completions.
type Client struct {
	APIKey   string
	Endpoint string
}

// NewClient creates a new instance of the Perplexity chat client.
// It loads the PERPLEXITY_API_KEY from the environment.
func NewClient() *Client {
	key := os.Getenv("PERPLEXITY_API_KEY")
	if key == "" {
		fmt.Fprintln(os.Stderr, "Warning: PERPLEXITY_API_KEY environment variable not set")
	}
	return &Client{
		APIKey:   key,
		Endpoint: "https://api.perplexity.ai/chat/completions",
	}
}

// Request defines the payload sent to Perplexity.ai.
type Request struct {
	Model    string    `json:"model"`
	Messages []ChatMsg `json:"messages"`
}

// ChatMsg represents a single chat message.
type ChatMsg struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// Response defines Perplexity.ai's response structure.
type Response struct {
	Citations []string `json:"citations"`
	Choices   []Choice `json:"choices"`
	Error     *struct {
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

// Choice holds a generated chat choice.
type Choice struct {
	FinishReason string  `json:"finish_reason"`
	Role         string  `json:"role"`
	Message      ChatMsg `json:"message"`
}

// CompleteChat sends a chat completion request to Perplexity.ai and returns the generated text.
// This method conforms to the ChatClient interface.
func (c *Client) CompleteChat(model string, messagesIn []client.ChatMsg) (results client.Results, err error) {

	// Prepare the request payload.
	reqPayload := Request{
		Model:    model,
		Messages: []ChatMsg{},
	}

	// Convert ChatMsg (from client interface) to Message for Perplexity.ai.
	for _, m := range messagesIn {
		// Perplexity.ai prefers lowercase role names.
		reqPayload.Messages = append(reqPayload.Messages, ChatMsg{
			Role:    strings.ToLower(m.Role),
			Content: m.Content,
		})
	}

	// Marshal the request payload.
	payloadBytes, err := json.Marshal(reqPayload)
	if err != nil {
		return
	}

	// Create the HTTP request.
	req, err := http.NewRequest("POST", c.Endpoint, strings.NewReader(string(payloadBytes)))
	if err != nil {
		return
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", c.APIKey))

	// Execute the HTTP request.
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return
	}
	defer resp.Body.Close()

	// Check for non-200 status codes.
	if resp.StatusCode != http.StatusOK {
		body, _ := ioutil.ReadAll(resp.Body)
		err = fmt.Errorf("Perplexity API returned status %d: %s", resp.StatusCode, string(body))
		return
	}

	// Read and unmarshal the response.
	respBytes, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return
	}

	var response Response
	if err = json.Unmarshal(respBytes, &response); err != nil {
		return
	}

	if len(response.Choices) == 0 {
		err = fmt.Errorf("no choices in Perplexity response")
		return
	}

	// Return the content of the first choice.
	results.Body = response.Choices[0].Message.Content
	results.Citations = response.Citations

	return
}
