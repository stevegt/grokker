package core

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"strings"
)

// PerplexityClient encapsulates the API client for Perplexity.ai.
type PerplexityClient struct {
	APIKey   string
	Endpoint string
}

// NewPerplexityClient creates a new PerplexityClient instance.
// It reads the PERPLEXITY_API_KEY from the environment and sets the API endpoint.
func NewPerplexityClient() *PerplexityClient {
	key := os.Getenv("PERPLEXITY_API_KEY")
	if key == "" {
		fmt.Fprintln(os.Stderr, "Warning: PERPLEXITY_API_KEY environment variable not set")
	}
	return &PerplexityClient{
		APIKey:   key,
		Endpoint: "https://api.perplexity.ai/chat/completions",
	}
}

// PerplexityRequest defines the request payload sent to Perplexity.ai.
type PerplexityRequest struct {
	Model    string    `json:"model"`
	Messages []Message `json:"messages"`
}

// Message represents a single message in the chat conversation.
type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// PerplexityResponse defines the structure of the response from Perplexity.ai.
type PerplexityResponse struct {
	Citations []string `json:"citations"`
	Choices   []Choice `json:"choices"`
}

// Choice holds a single generated chat choice.
type Choice struct {
	FinishReason string  `json:"finish_reason"`
	Role         string  `json:"role"`
	Message      Message `json:"message"`
}

// CompleteChat implements the ChatProvider interface for PerplexityClient.
// It sends the system message along with the chat history to the Perplexity.ai API
// and returns the content of the first choice in the response.
func (pc *PerplexityClient) CompleteChat(sysmsg string, messages []ChatMsg) (string, error) {
	// Prepare the request payload.
	reqPayload := PerplexityRequest{
		Model: "sonar-deep-research", // This can be adjusted or made configurable.
		Messages: []Message{
			{
				Role:    "system",
				Content: sysmsg,
			},
		},
	}

	// Convert ChatMsg (from Grokker) to Message for Perplexity.ai.
	for _, m := range messages {
		// Perplexity.ai prefers lowercase role names.
		reqPayload.Messages = append(reqPayload.Messages, Message{
			Role:    strings.ToLower(m.Role),
			Content: m.Txt,
		})
	}

	// Marshal the request payload.
	payloadBytes, err := json.Marshal(reqPayload)
	if err != nil {
		return "", err
	}

	// Create the HTTP request.
	req, err := http.NewRequest("POST", pc.Endpoint, strings.NewReader(string(payloadBytes)))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", pc.APIKey))

	// Execute the HTTP request.
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	// Check for non-200 status codes.
	if resp.StatusCode != http.StatusOK {
		body, _ := ioutil.ReadAll(resp.Body)
		return "", fmt.Errorf("Perplexity API returned status %d: %s", resp.StatusCode, string(body))
	}

	// Read and unmarshal the response.
	respBytes, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	var perplexityResp PerplexityResponse
	if err := json.Unmarshal(respBytes, &perplexityResp); err != nil {
		return "", err
	}

	if len(perplexityResp.Choices) == 0 {
		return "", fmt.Errorf("no choices in Perplexity response")
	}

	// Return the content of the first choice.
	return perplexityResp.Choices[0].Message.Content, nil
}
