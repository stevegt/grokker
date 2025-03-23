package core

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	embedLib "github.com/fabiustech/openai"
	embedModelLib "github.com/fabiustech/openai/models"
	gptLib "github.com/sashabaranov/go-openai"
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
		req := &embedLib.EmbeddingRequest{
			Input: inputs,
			Model: embedModelLib.AdaEmbeddingV2,
		}
		Debug("creating embedding for chunk %d of %d ...", i+1, len(texts))
		// Debug("text: %q", text)
		// loop with backoff until we get a response
		var res *embedLib.EmbeddingResponse
		for backoff := 1; backoff < 10; backoff++ {
			res, err = c.CreateEmbeddings(context.Background(), req)
			if err == nil {
				break
			}
			Pf("openai API error, retrying: %#v", err)
			// wait and try again
			time.Sleep(time.Second * time.Duration(backoff))
		}
		Ck(err, "%T: %#v", err, err)
		for _, em := range res.Data {
			embeddings = append(embeddings, em.Embedding)
		}
	}
	Debug("created %d embeddings", len(embeddings))
	Assert(len(embeddings) <= len(texts))
	return
}

// CompleteChat uses the openai API to complete a chat.  It converts the
// role in the ChatMsg slice to the appropriate openai.ChatMessageRole
// value.
func (g *Grokker) CompleteChat(sysmsg string, msgs []ChatMsg) (response string, err error) {
	defer Return(&err)

	Debug("msgs: %s", Spprint(msgs))

	omsgs := initMessages(g, sysmsg)

	// convert the ChatMsg slice to an oai.ChatCompletionMessage slice
	for _, msg := range msgs {
		// skip empty messages
		if len(strings.TrimSpace(msg.Txt)) == 0 {
			continue
		}
		var role string
		switch msg.Role {
		case "USER":
			role = gptLib.ChatMessageRoleUser
		case "AI":
			role = gptLib.ChatMessageRoleAssistant
		default:
			Assert(false, "unknown role: %q", msg)
		}
		omsgs = append(omsgs, gptLib.ChatCompletionMessage{
			Role:    role,
			Content: msg.Txt,
		})
	}

	Debug("sending to OpenAI: %s", Spprint(omsgs))

	var resp gptLib.ChatCompletionResponse
	resp, err = g.complete(omsgs)
	Ck(err)
	response = resp.Choices[0].Message.Content

	Debug("response from OpenAI: %s", response)

	return
}

// generate returns the answer to a question.
func (g *Grokker) generate(sysmsg, question, ctxt string, global bool) (resp gptLib.ChatCompletionResponse, err error) {
	defer Return(&err)

	// XXX don't exceed max tokens

	messages := initMessages(g, sysmsg)

	// first get global knowledge
	if global {
		messages = append(messages, gptLib.ChatCompletionMessage{
			Role:    gptLib.ChatMessageRoleUser,
			Content: question,
		})
		resp, err = g.chat(messages)
		Ck(err)
		// add the response to the messages.
		messages = append(messages, gptLib.ChatCompletionMessage{
			Role:    gptLib.ChatMessageRoleAssistant,
			Content: resp.Choices[0].Message.Content,
		})
	}

	// add context from local sources
	if len(ctxt) > 0 {
		messages = append(messages, []gptLib.ChatCompletionMessage{
			{
				Role:    gptLib.ChatMessageRoleUser,
				Content: Spf("Context:\n\n%s", ctxt),
			},
			{
				Role:    gptLib.ChatMessageRoleAssistant,
				Content: "Great! I've read the context.",
			},
		}...)
	}

	// now ask the question
	messages = append(messages, gptLib.ChatCompletionMessage{
		Role:    gptLib.ChatMessageRoleUser,
		Content: question,
	})

	// get the answer
	resp, err = g.chat(messages)
	Ck(err, "context length: %d type: %T: %#v", len(ctxt), ctxt, ctxt)

	// fmt.Println(resp.Choices[0].Message.Content)
	// Pprint(messages)
	// Pprint(resp)
	return
}

// msg uses the openai API to generate a response to a message.
func (g *Grokker) msg(sysmsg, input string) (resp gptLib.ChatCompletionResponse, err error) {
	defer Return(&err)

	// don't exceed max tokens
	sysmsgTc, err := g.TokenCount(sysmsg)
	Ck(err)
	inputTc, err := g.TokenCount(input)
	Ck(err)
	if sysmsgTc+inputTc > g.ModelObj.TokenLimit {
		err = fmt.Errorf("token count %d exceeds token limit %d", sysmsgTc+inputTc, g.ModelObj.TokenLimit)
		return
	}

	messages := initMessages(g, sysmsg)

	// add the user message
	userMsg := gptLib.ChatCompletionMessage{
		Role:    gptLib.ChatMessageRoleUser,
		Content: input,
	}
	messages = append(messages, userMsg)

	// get the answer
	resp, err = g.chat(messages)
	Ck(err)

	return
}

// initMessages creates and returns the initial messages slice.  It includes
// the system message if the model supports it, otherwise it includes the
// system message in the first user message.
func initMessages(g *Grokker, sysmsg string) []gptLib.ChatCompletionMessage {
	// noSysMsg that do not support system messages
	noSysMsg := []string{
		"o1-preview",
		"o1-mini",
		"o3-mini",
	}
	sysmsgOk := true
	for _, model := range noSysMsg {
		if g.Model == model {
			sysmsgOk = false
			break
		}
	}
	sysmsgRole := gptLib.ChatMessageRoleSystem
	if !sysmsgOk {
		sysmsgRole = gptLib.ChatMessageRoleUser
	}
	messages := []gptLib.ChatCompletionMessage{
		{
			Role:    sysmsgRole,
			Content: sysmsg,
		},
	}
	if !sysmsgOk {
		sysmsgResponse := gptLib.ChatCompletionMessage{
			Role:    gptLib.ChatMessageRoleAssistant,
			Content: "Got it!  I will use those instructions as my system message and will follow them faithfully in each of my responses.",
		}
		messages = append(messages, sysmsgResponse)
	}

	return messages
}

// chat uses the openai API to continue a conversation given a
// (possibly synthesized) message history.
func (g *Grokker) chat(messages []gptLib.ChatCompletionMessage) (resp gptLib.ChatCompletionResponse, err error) {
	defer Return(&err)

	resp, err = g.complete(messages)
	Ck(err, "%#v", messages)
	totalBytes := 0
	for _, msg := range messages {
		totalBytes += len(msg.Content)
	}
	totalBytes += len(resp.Choices[0].Message.Content)
	ratio := float64(totalBytes) / float64(resp.Usage.TotalTokens)
	// Debug("chat response: %s", resp)
	Debug("total tokens: %d  char/token ratio: %.1f\n", resp.Usage.TotalTokens, ratio)
	return
}

func (g *Grokker) complete(messages []gptLib.ChatCompletionMessage) (res gptLib.ChatCompletionResponse, err error) {
	client := g.chatClient
	res, err = client.CreateChatCompletion(
		context.Background(),
		gptLib.ChatCompletionRequest{
			Model:    g.ModelObj.upstreamName,
			Messages: messages,
		},
	)
	return res, err
}

// initClients initializes the OpenAI clients.
// This function needs to be idempotent because it might be called multiple
// times during the lifetime of a Grokker object.
func (g *Grokker) initClients() {
	// XXX using only OpenAI for embedding -- need to support more providers
	authtoken := os.Getenv("OPENAI_API_KEY")
	g.embeddingClient = embedLib.NewClient(authtoken)
	g.chatClient = gptLib.NewClient(authtoken)
	return
}

/*
// ClientFactory returns a new ChatClient object.
func ClientFactory() interface{} {
}
*/
