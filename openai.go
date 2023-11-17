package grokker

import (
	"context"
	"os"

	"github.com/fabiustech/openai"
	fabius_models "github.com/fabiustech/openai/models"
	oai "github.com/sashabaranov/go-openai"
	. "github.com/stevegt/goadapt"
)

// var promptTmpl = `You are a helpful assistant.  Answer the following question and summarize the context:
// var promptTmpl = `You are a helpful assistant.
var promptTmpl = `{{.Question}}

Context:
{{.Context}}`

var SysMsgChat = "You are an expert knowledgable in the provided context.  I will provide you with context, then you will respond with an acknowledgement, then I will ask you a question about the context, then you will provide me with an answer."

var SysMsgRevise = "You are an expert knowledgable in the provided context.  I will provide you with context, then you will respond with an acknowledgement, then I will provide you with a block of text.  You will revise the block of text based on the information in the context, maintaining the same style, vocabulary, and reading level."

var SysMsgContinue = "You are an expert knowledgable in the provided context.  I will provide you with context, then you will respond with an acknowledgement, then I will provide you with a block of text.  You will continue the block of text based on the information in the context, maintaining the same style, vocabulary, and reading level."

// createEmbeddings returns the embeddings for a slice of text chunks.
func (g *Grokker) createEmbeddings(texts []string) (embeddings [][]float64, err error) {
	defer Return(&err)
	// use github.com/fabiustech/openai library
	c := g.embeddingClient
	// simply call c.CreateEmbeddings() once for each text chunk.
	for i := 0; i < len(texts); i++ {
		text := texts[i]
		// set empty chunk embedding to nil
		if len(text) == 0 {
			embeddings = append(embeddings, nil)
			continue
		}
		inputs := []string{text}
		req := &openai.EmbeddingRequest{
			Input: inputs,
			Model: fabius_models.AdaEmbeddingV2,
		}
		Debug("creating embedding for chunk %d of %d ...", i+1, len(texts))
		Debug("text: %q", text)
		res, err := c.CreateEmbeddings(context.Background(), req)
		Ck(err)
		for _, em := range res.Data {
			embeddings = append(embeddings, em.Embedding)
		}
	}
	Debug("created %d embeddings", len(embeddings))
	Assert(len(embeddings) <= len(texts))
	return
}

// generate returns the answer to a question.
func (g *Grokker) generate(sysmsg, question, ctxt string, global bool) (resp oai.ChatCompletionResponse, err error) {
	defer Return(&err)

	// XXX don't exceed max tokens
	messages := []oai.ChatCompletionMessage{
		{
			Role:    oai.ChatMessageRoleSystem,
			Content: sysmsg,
		},
	}

	// first get global knowledge
	if global {
		messages = append(messages, oai.ChatCompletionMessage{
			Role:    oai.ChatMessageRoleUser,
			Content: question,
		})
		resp, err = g.chat(messages)
		Ck(err)
		// add the response to the messages.
		messages = append(messages, oai.ChatCompletionMessage{
			Role:    oai.ChatMessageRoleAssistant,
			Content: resp.Choices[0].Message.Content,
		})
	}

	// add context from local sources
	if len(ctxt) > 0 {
		messages = append(messages, []oai.ChatCompletionMessage{
			{
				Role:    oai.ChatMessageRoleUser,
				Content: Spf("Context:\n\n%s", ctxt),
			},
			{
				Role:    oai.ChatMessageRoleAssistant,
				Content: "Great! I've read the context.",
			},
		}...)
	}

	// now ask the question
	messages = append(messages, oai.ChatCompletionMessage{
		Role:    oai.ChatMessageRoleUser,
		Content: question,
	})

	// get the answer
	resp, err = g.chat(messages)
	Ck(err, "context length: %d", len(ctxt))

	// fmt.Println(resp.Choices[0].Message.Content)
	// Pprint(messages)
	// Pprint(resp)
	return
}

// chat uses the openai API to continue a conversation given a
// (possibly synthesized) message history.
func (g *Grokker) chat(messages []oai.ChatCompletionMessage) (resp oai.ChatCompletionResponse, err error) {
	defer Return(&err)

	model := g.oaiModel
	Debug("chat model: %s", model)
	Debug("chat: messages: %v", messages)

	// use 	"github.com/sashabaranov/go-openai"
	client := g.chatClient
	resp, err = client.CreateChatCompletion(
		context.Background(),
		oai.ChatCompletionRequest{
			Model:    model,
			Messages: messages,
		},
	)
	Ck(err, "%#v", messages)
	totalBytes := 0
	for _, msg := range messages {
		totalBytes += len(msg.Content)
	}
	totalBytes += len(resp.Choices[0].Message.Content)
	ratio := float64(totalBytes) / float64(resp.Usage.TotalTokens)
	Debug("chat response: %s", resp)
	Debug("total tokens: %d  char/token ratio: %.1f\n", resp.Usage.TotalTokens, ratio)
	return
}

// initClients initializes the OpenAI clients.
// This function needs to be idempotent because it might be called multiple
// times during the lifetime of a Grokker object.
func (g *Grokker) initClients() {
	authtoken := os.Getenv("OPENAI_API_KEY")
	g.embeddingClient = openai.NewClient(authtoken)
	g.chatClient = oai.NewClient(authtoken)
	return
}
