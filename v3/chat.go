package grokker

import (
	"encoding/json"
	"io/ioutil"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	. "github.com/stevegt/goadapt"
)

type ChatHistory struct {
	Sysmsg  string
	Version string
	relPath string
	msgs    []ChatMsg
	g       *GrokkerInternal
}

type ChatMsg struct {
	Role string
	Txt  string
}

// OpenChatHistory opens a chat history file and returns a ChatHistory
// object.  The chat history file is a special format that is amenable
// to context chunking and summarization.  The first line of the file
// is a json string that contains a ChatHistory struct.  The rest of
// the file is a chat history in the following format:
//
// <role>:\n<message>\n\n
//
// ...where <role> is either "USER" or "AI", and <message> is the text
// of the message.  The last message in the file is the most recent
// message.
func (g *GrokkerInternal) OpenChatHistory(relPath, sysmsg string) (history *ChatHistory, err error) {
	defer Return(&err)
	Assert(relPath != "", "relPath is required")
	path := filepath.Join(g.Root, relPath)
	_, err = os.Stat(path)
	if err != nil {
		// file does not exist
		history = &ChatHistory{relPath: relPath,
			Sysmsg:  sysmsg,
			msgs:    make([]ChatMsg, 0),
			Version: version}
		err = nil
	} else {
		// file exists
		fh, err := os.Open(path)
		Ck(err)
		defer fh.Close()
		// the first line of the file is the ChatHistory struct; load
		// that into a ChatHistory object using json.Unmarshal
		history = &ChatHistory{}
		buf, err := ioutil.ReadAll(fh)
		Ck(err)
		// get first line
		lines := strings.Split(string(buf), "\n")
		// unmarshal first line
		err = json.Unmarshal([]byte(lines[0]), history)
		Ck(err)
		// the rest of the file is the chat history; load that into
		// the chat history object using parseChat
		history.msgs = history.parseChat(strings.Join(lines[1:], "\n"))
	}
	// if a sysmsg is provided, replace the one in the file
	if sysmsg != "" {
		history.Sysmsg = sysmsg
	}
	// if there is no sysmsg, use a default
	if history.Sysmsg == "" {
		history.Sysmsg = "You are a helpful assistant."
	}

	// set relPath
	history.relPath = relPath

	// set grokker
	history.g = g

	// add the history file to grokker's list of files
	g.AddDocument(relPath)

	return
}

// continueChat continues a chat history.
func (history *ChatHistory) continueChat(prompt string) (resp string, err error) {
	defer Return(&err)
	g := history.g

	Debug("continueChat: prompt=%s", prompt)

	// create a temporary slice of messages to work with
	var msgs []ChatMsg

	// get context from the knowledge base -- this includes the chat history
	// file itself, because it is a document in the knowledge base
	var context string
	maxTokens := int(float64(g.tokenLimit) * 0.5)
	if maxTokens > int(float64(g.tokenLimit)*0.1) {
		context, err = g.getContext(prompt, maxTokens, false, false, nil)
		Ck(err)
		// make context look like a message exchange
		msgs = []ChatMsg{
			ChatMsg{Role: "USER", Txt: context},
			ChatMsg{Role: "AI", Txt: "I understand the context."},
		}
	}

	// append stored messages to the context
	msgs = append(msgs, history.msgs...)

	// append the prompt to the context
	msgs = append(msgs, ChatMsg{Role: "USER", Txt: prompt})

	// summarize
	msgs, err = history.summarize(msgs)
	Ck(err)

	// generate the response
	resp, err = g.completeChat(history.Sysmsg, msgs)

	// append the prompt and response to the stored messages
	history.msgs = append(history.msgs, ChatMsg{Role: "USER", Txt: prompt})
	history.msgs = append(history.msgs, ChatMsg{Role: "AI", Txt: resp})

	return
}

// summarize summarizes a chat history until it is within the
// tokenLimit/2.  It always leaves the last message intact.
func (history *ChatHistory) summarize(msgs []ChatMsg) (summarized []ChatMsg, err error) {
	defer Return(&err)
	g := history.g

	// count tokens
	msgsCount, err := history.TokenCount(msgs)
	Ck(err)

	Debug("summarize: msgsCount=%d, tokenLimit=%d", msgsCount, g.tokenLimit)
	Debug("summarize: msgs=%v", Spprint(msgs))

	// if we are already within the limit, return
	if msgsCount <= g.tokenLimit/2 {
		summarized = msgs
		return
	}

	for len(summarized) == 0 {
		// summarize the first half of the messages by converting them to
		// a text format that GPT-4 can understand, sending them to GPT-4,
		// and then converting the response back to a slice of messages.
		// The format is the same as the chat history file format.
		txt := history.chat2txt(msgs[:len(msgs)/2])
		// send to GPT-4
		resp, err := g.Msg("You are an editor.  Rewrite the chat history to make it about half as long.  Your answer must be in the same chat format.  Please include the AI: and USER: headers in your response.", txt)
		Ck(err)
		// convert the response back to a slice of messages
		Debug("summarize: resp=%s", resp)
		summarized = history.parseChat(resp)
	}

	// append the second half of the messages to the summarized messages
	summarized = append(summarized, msgs[len(msgs)/2:]...)

	// recurse
	summarized, err = history.summarize(summarized)
	Ck(err)

	return
}

// chat2txt returns the given history messages as a text string.
func (history *ChatHistory) chat2txt(msgs []ChatMsg) (txt string) {
	for _, msg := range msgs {
		txt += Spf("%s:\n%s\n\n", msg.Role, msg.Txt)
	}
	return
}

// parseChat parses a chat history from a string.  See summarize for
// the format.
func (history *ChatHistory) parseChat(txt string) (msgs []ChatMsg) {
	// split into lines
	lines := strings.Split(txt, "\n")
	// parse each line
	pattern := `^([A-Z]+):`
	re := regexp.MustCompile(pattern)
	var msg *ChatMsg
	for _, line := range lines {
		// look for a role
		m := re.FindStringSubmatch(line)
		if len(m) > 0 {
			if msg != nil {
				// add the previous message to the slice
				msgs = append(msgs, *msg)
			}
			role := history.fixRole(m[1])
			// create a new message
			msg = &ChatMsg{Role: role}
			continue
		}
		if msg == nil {
			// we are in some sort of preamble -- credit it to the AI
			msg = &ChatMsg{Role: "AI"}
		}
		// if we get here, we are in the middle of a message
		msg.Txt += line + "\n"
	}
	// add the last message to the slice
	if msg != nil {
		msgs = append(msgs, *msg)
	}
	return
}

// fixRole fixes the role in a chat history file.  This might be
// necessary if GPT-4 changes the role names.
func (history *ChatHistory) fixRole(role string) (fixed string) {
	role = strings.ToUpper(role)
	switch role {
	case "USER":
		fallthrough
	case "AI":
		fixed = role
	default:
		// default to AI for now, might need more sophisticated
		// logic later
		fixed = "AI"
	}
	return
}

// Save saves the chat history file.
func (history *ChatHistory) Save() (err error) {
	defer Return(&err)
	// marshal the struct into a json string
	buf, err := json.Marshal(history)
	Ck(err)
	// open a temp file
	fh, err := ioutil.TempFile("", "chat")
	Ck(err)
	// write the json string to the temp file
	_, err = fh.Write(buf)
	Ck(err)
	// write a newline to the temp file
	_, err = fh.Write([]byte("\n"))
	Ck(err)
	// write the chat history to the temp file
	txt := history.chat2txt(history.msgs)
	_, err = fh.Write([]byte(txt))
	Ck(err)
	// close the temp file
	err = fh.Close()
	Ck(err)
	// replace the chat history file with the temp file
	Assert(history.relPath != "", "relPath is required")
	path := filepath.Join(history.g.Root, history.relPath)
	// move the chat history file to a backup file
	var backup string
	_, err = os.Stat(path)
	if err == nil {
		// file exists
		backup = path + ".bak"
		err = os.Rename(path, backup)
		Ck(err)
	}
	// move the temp file to the chat history file
	err = os.Rename(fh.Name(), path)
	Ck(err)
	// remove the backup file
	_, err = os.Stat(backup)
	if err == nil {
		// file exists
		err = os.Remove(backup)
		Ck(err)
	}
	Debug("chat history saved to %s", path)
	return nil
}

// TokenCount returns the number of tokens in the given chat history.
func (history *ChatHistory) TokenCount(msgs []ChatMsg) (count int, err error) {
	defer Return(&err)
	g := history.g
	// convert the chat history to a text string
	txt := history.chat2txt(msgs)
	// get the token count
	count, err = g.TokenCount(txt)
	Ck(err)
	return
}
