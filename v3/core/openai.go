package core

import (
	"context"
	"fmt"
	"os"
	"strings"

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

// CompleteChat uses the openai API to complete a chat.  It converts the
// role in the ChatMsg slice to the appropriate openai.ChatMessageRole
// value.
func (g *Grokker) CompleteChat(sysmsg string, msgs []ChatMsg) (response string, err error) {
	defer Return(&err)

	Debug("msgs: %s", Spprint(msgs))

	// initialize the messages slice with the system message as the
	// first message
	omsgs := initMessages(g, sysmsg)
	// add the rest of the messages
	for _, msg := range msgs {
		// skip empty messages
		if len(strings.TrimSpace(msg.Content)) == 0 {
			continue
		}
		omsgs = append(omsgs, ChatMsg{
			Role:    msg.Role,
			Content: msg.Content,
		})
	}

	Debug("sending to OpenAI: %s", Spprint(omsgs))

	response, err = g.complete(omsgs)
	Ck(err)

	Debug("response from OpenAI: %s", response)

	return
}

// AnswerWithRAG returns the answer to a question.
func (g *Grokker) AnswerWithRAG(sysmsg, question, ctxt string, global bool) (out string, err error) {
	defer Return(&err)

	// XXX don't exceed max tokens

	messages := initMessages(g, sysmsg)

	// first get global knowledge
	if global {
		messages = append(messages, ChatMsg{
			Role:    gptLib.ChatMessageRoleUser,
			Content: question,
		})
		var resp string
		resp, err = g.complete(messages)
		Ck(err)
		// add the response to the messages.
		messages = append(messages, ChatMsg{
			Role:    gptLib.ChatMessageRoleAssistant,
			Content: resp,
		})
	}

	// add context from local sources
	if len(ctxt) > 0 {
		messages = append(messages, []ChatMsg{
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
	messages = append(messages, ChatMsg{
		Role:    gptLib.ChatMessageRoleUser,
		Content: question,
	})

	// get the answer
	out, err = g.complete(messages)
	Ck(err, "context length: %d type: %T: %#v", len(ctxt), ctxt, ctxt)

	return
}

// msg uses the openai API to generate a response to a message.
func (g *Grokker) msg(sysmsg, input string) (output string, err error) {
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
	userMsg := ChatMsg{
		Role:    gptLib.ChatMessageRoleUser,
		Content: input,
	}
	messages = append(messages, userMsg)

	// get the answer
	output, err = g.complete(messages)
	Ck(err)

	return
}

// initMessages creates and returns the initial messages slice.  It includes
// the system message if the model supports it, otherwise it includes the
// system message in the first user message.
func initMessages(g *Grokker, sysmsg string) []ChatMsg {
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
	messages := []ChatMsg{
		{
			Role:    sysmsgRole,
			Content: sysmsg,
		},
	}
	if !sysmsgOk {
		sysmsgResponse := ChatMsg{
			Role:    gptLib.ChatMessageRoleAssistant,
			Content: "Got it!  I will use those instructions as my system message and will follow them faithfully in each of my responses.",
		}
		messages = append(messages, sysmsgResponse)
	}

	return messages
}

// complete uses the openai API to continue a conversation given a
// (possibly synthesized) message history.
//
// XXX turn this into a multi-model function by passing model in as an
// argument and having it create the appropriate client.  It may make
// sense to move it from the Grokker object to a Model or client.ChatClient
// object.  It may also make sense to move model.go into the client package.
func (g *Grokker) complete(inmsgs []ChatMsg) (out string, err error) {

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
			Model:    g.ModelObj.upstreamName,
			Messages: omsgs,
		},
	)
	out = res.Choices[0].Message.Content
	return
}

/*
// ClientFactory returns a new ChatClient object.
func ClientFactory() interface{} {
}
*/
