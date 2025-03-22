package openai

import (
	"context"

	gptLib "github.com/sashabaranov/go-openai"
	"github.com/stevegt/grokker/v3/client"
)

// OpenAIChatClient implements the ChatClient interface for OpenAI.
type OpenAIChatClient struct {
	client *gptLib.Client
	model  string
}

// NewOpenAIChatClient creates a new OpenAIChatClient instance.
func NewOpenAIChatClient(apiKey, model string) *OpenAIChatClient {
	c := gptLib.NewClient(apiKey)
	return &OpenAIChatClient{client: c, model: model}
}

// CompleteChat sends a chat request to the OpenAI API and returns the response.
// It converts core.ChatMsg messages into OpenAI's ChatCompletionMessage format.
func (oc *OpenAIChatClient) CompleteChat(model, sysmsg string, messages []client.ChatMsg) (string, error) {
	omsgs := []gptLib.ChatCompletionMessage{
		{
			Role:    gptLib.ChatMessageRoleSystem,
			Content: sysmsg,
		},
	}
	for _, msg := range messages {
		var role string
		switch msg.Role {
		case "USER":
			role = gptLib.ChatMessageRoleUser
		case "AI":
			role = gptLib.ChatMessageRoleAssistant
		default:
			role = gptLib.ChatMessageRoleUser
		}
		omsgs = append(omsgs, gptLib.ChatCompletionMessage{
			Role:    role,
			Content: msg.Content,
		})
	}
	req := gptLib.ChatCompletionRequest{
		Model:    model,
		Messages: omsgs,
	}
	resp, err := oc.client.CreateChatCompletion(context.Background(), req)
	if err != nil {
		return "", err
	}
	return resp.Choices[0].Message.Content, nil
}

// Assert that OpenAIChatClient implements client.ChatClient.
var _ client.ChatClient = (*OpenAIChatClient)(nil)
