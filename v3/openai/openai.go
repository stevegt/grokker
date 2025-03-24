package openai

import (
	"context"
	"os"
	"strings"

	gptLib "github.com/sashabaranov/go-openai"
	. "github.com/stevegt/goadapt"
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
func CompleteChat(upstreamName string, inmsgs []client.ChatMsg) (results client.Results, err error) {
	defer Return(&err)

	// convert the ChatMsg slice to an oai.ChatCompletionMessage slice
	omsgs := []gptLib.ChatCompletionMessage{}
	for _, msg := range inmsgs {
		// skip empty messages
		if len(strings.TrimSpace(msg.Content)) == 0 {
			continue
		}
		// convert msg.Role to uppercase
		msgRole := strings.ToUpper(msg.Role)
		// convert role to gptLib role
		var role string
		switch msgRole {
		case "SYSTEM":
			role = gptLib.ChatMessageRoleSystem
		case "USER":
			role = gptLib.ChatMessageRoleUser
		case "AI":
			role = gptLib.ChatMessageRoleAssistant
		case "ASSISTANT":
			role = gptLib.ChatMessageRoleAssistant
		default:
			Assert(false, "unknown role: %q", msg)
		}
		omsgs = append(omsgs, gptLib.ChatCompletionMessage{
			Role:    role,
			Content: msg.Content,
		})
	}

	authtoken := os.Getenv("OPENAI_API_KEY")
	client := gptLib.NewClient(authtoken)
	var res gptLib.ChatCompletionResponse
	res, err = client.CreateChatCompletion(
		context.Background(),
		gptLib.ChatCompletionRequest{
			Model:    upstreamName,
			Messages: omsgs,
		},
	)
	if err != nil {
		Pf("model: %s\n", upstreamName)
		Ck(err)
	}

	results.Body = res.Choices[0].Message.Content
	return
}
