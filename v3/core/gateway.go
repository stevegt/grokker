package core

import (
	"fmt"
	"strings"

	. "github.com/stevegt/goadapt"
	"github.com/stevegt/grokker/v3/client"
	"github.com/stevegt/grokker/v3/openai"
	"github.com/stevegt/grokker/v3/perplexity"
)

var promptTmpl = `{{.Question}}

Context:
{{.Context}}`

var SysMsgChat = "You are an expert knowledgable in the provided context.  I will provide you with context, then you will respond with an acknowledgement, then I will ask you a question about the context, then you will provide me with an answer."

var SysMsgRevise = "You are an expert knowledgable in the provided context.  I will provide you with context, then you will respond with an acknowledgement, then I will provide you with a block of text.  You will revise the block of text based on the information in the context, maintaining the same style, vocabulary, and reading level."

var SysMsgContinue = "You are an expert knowledgable in the provided context.  I will provide you with context, then you will respond with an acknowledgement, then I will provide you with a block of text.  You will continue the block of text based on the information in the context, maintaining the same style, vocabulary, and reading level."

// CompleteChat uses the openai API to complete a chat.  It converts the
// role in the ChatMsg slice to the appropriate openai.ChatMessageRole
// value.
func (g *Grokker) CompleteChat(modelName, sysmsg string, msgs []client.ChatMsg) (response string, err error) {
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
		omsgs = append(omsgs, client.ChatMsg{
			Role:    msg.Role,
			Content: msg.Content,
		})
	}

	Debug("sending to OpenAI: %s", Spprint(omsgs))

	response, err = g.complete(modelName, omsgs)
	Ck(err)

	Debug("response from OpenAI: %s", response)

	return
}

// AnswerWithRAG returns the answer to a question.
func (g *Grokker) AnswerWithRAG(modelName, sysmsg, question, ctxt string, global bool) (out string, err error) {
	defer Return(&err)

	messages := initMessages(g, sysmsg)

	// first get global knowledge
	if global {
		messages = append(messages, client.ChatMsg{
			Role:    RoleUser,
			Content: question,
		})
		var resp string
		resp, err = g.complete(modelName, messages)
		Ck(err)
		// add the response to the messages.
		messages = append(messages, client.ChatMsg{
			Role:    RoleAI,
			Content: resp,
		})
	}

	// add context from local sources
	if len(ctxt) > 0 {
		messages = append(messages, []client.ChatMsg{
			{
				Role:    RoleUser,
				Content: Spf("Context:\n\n%s", ctxt),
			},
			{
				Role:    RoleAI,
				Content: "Great! I've read the context.",
			},
		}...)
	}

	// now ask the question
	messages = append(messages, client.ChatMsg{
		Role:    RoleUser,
		Content: question,
	})

	// don't exceed max tokens
	// XXX might want to summarize the context
	totalTc := 0
	for _, msg := range messages {
		tc, err := g.TokenCount(msg.Content)
		Ck(err)
		totalTc += tc
	}
	if totalTc > g.ModelObj.TokenLimit {
		err = fmt.Errorf("token count %d exceeds token limit %d -- try reducing context", totalTc, g.ModelObj.TokenLimit)
		return
	}

	// get the answer
	out, err = g.complete(modelName, messages)
	Ck(err, "context length: %d type: %T: %#v", len(ctxt), ctxt, ctxt)

	return
}

// initMessages creates and returns the initial messages slice.  It includes
// the system message if the model supports it, otherwise it includes the
// system message in the first user message.
func initMessages(g *Grokker, sysmsg string) []client.ChatMsg {
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
	sysmsgRole := RoleSystem
	if !sysmsgOk {
		sysmsgRole = RoleUser
	}
	messages := []client.ChatMsg{
		{
			Role:    sysmsgRole,
			Content: sysmsg,
		},
	}
	if !sysmsgOk {
		sysmsgResponse := client.ChatMsg{
			Role:    RoleAI,
			Content: "Got it!  I will use those instructions as my system message and will follow them faithfully in each of my responses.",
		}
		messages = append(messages, sysmsgResponse)
	}

	return messages
}

// complete acts as a router to the appropriate completion function based on
// provider.
func (g *Grokker) complete(modelName string, inmsgs []client.ChatMsg) (out string, err error) {
	defer Return(&err)
	_, modelObj, err := g.models.FindModel(modelName)
	Ck(err)

	upstreamName := modelObj.upstreamName

	switch modelObj.providerName {
	case "openai":
		return openai.CompleteChat(upstreamName, inmsgs)
	case "perplexity":
		pp := perplexity.NewClient()
		return pp.CompleteChat(upstreamName, inmsgs)
	default:
		Assert(false, "unknown provider: %s", modelObj.providerName)
	}
	return
}
