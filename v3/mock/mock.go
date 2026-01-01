package mock

import (
	"github.com/stevegt/grokker/v3/client"
)

// Client is a mock LLM provider for testing.
// It implements the ChatClient interface and returns pre-configured responses
// based on the model name. Tests can configure responses using SetResponse.
type Client struct {
	Responses map[string]string // model name -> response
}

// NewClient creates a new mock client.
func NewClient() *Client {
	return &Client{
		Responses: make(map[string]string),
	}
}

// SetResponse sets the response for a given model name.
// This allows tests to configure the mock provider with specific responses.
func (c *Client) SetResponse(model, response string) {
	c.Responses[model] = response
}

// CompleteChat returns a pre-configured response based on the model name.
// If no response has been configured for the given model, it returns a default response.
// This method implements the ChatClient interface.
func (c *Client) CompleteChat(model string, msgs []client.ChatMsg) (client.Results, error) {
	response, ok := c.Responses[model]
	if !ok {
		response = "default mock response"
	}
	return client.Results{
		Body:      response,
		Citations: []string{},
	}, nil
}