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

var SysMsgSummarizeChat = `You are an editor.  Rewrite the chat
history to make it about half as long, focusing on the following
topic.  Your answer must be in the same chat format.  Please include
the AI: and USER: headers in your response.\n\nTopic:\n%s\n`

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
func (g *GrokkerInternal) OpenChatHistory(sysmsg, relPath string) (history *ChatHistory, err error) {
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

// continueChat continues a chat history.  The debug map contains
// interesting statistics about the process, for testing and debugging
// purposes.
func (history *ChatHistory) continueChat(prompt string, limitContext, fast bool) (resp string, debug map[string]int, err error) {
	defer Return(&err)
	g := history.g

	Debug("continueChat: prompt len=%d", len(prompt))

	// create a temporary slice of messages to work with
	var msgs []ChatMsg

	if !fast {
		// get context
		maxTokens := int(float64(g.tokenLimit) * 0.5)
		var context string
		var files []string
		if limitContext {
			// get context only from the chat history file itself
			files = []string{history.relPath}
		} else {
			// get context from all files in the knowledge base -- this
			// includes the chat history file itself, because it is a
			// document in the knowledge base
			files = nil
		}
		context, err = g.getContext(prompt, maxTokens, false, false, files)
		Ck(err)
		if context != "" {
			// make context look like a message exchange
			msgs = []ChatMsg{
				ChatMsg{Role: "USER", Txt: context},
				ChatMsg{Role: "AI", Txt: "I understand the context."},
			}
		}
	}

	// append stored messages to the context
	msgs = append(msgs, history.msgs...)

	// append the prompt to the context
	msgs = append(msgs, ChatMsg{Role: "USER", Txt: prompt})

	peakCount, err := history.tokenCount(msgs)
	Ck(err)

	// summarize
	promptTc, err := g.TokenCount(prompt)
	Debug("continueChat: promptTc=%d", promptTc)
	Ck(err)
	maxTokens := g.tokenLimit / 2
	msgs, err = history.summarize(prompt, msgs, maxTokens, fast)
	Ck(err)

	finalCount, err := history.tokenCount(msgs)
	Ck(err)

	debug = map[string]int{
		"peakCount":  peakCount,
		"finalCount": finalCount,
	}

	// generate the response
	resp, err = g.completeChat(history.Sysmsg, msgs)

	// append the prompt and response to the stored messages
	history.msgs = append(history.msgs, ChatMsg{Role: "USER", Txt: prompt})
	history.msgs = append(history.msgs, ChatMsg{Role: "AI", Txt: resp})

	return
}

// summarize summarizes a chat history until it is within
// maxTokens.  It always leaves the last message intact.
func (history *ChatHistory) summarize(prompt string, msgs []ChatMsg, maxTokens int, fast bool) (summarized []ChatMsg, err error) {
	defer Return(&err)
	g := history.g

	// count tokens
	msgsCount, err := history.tokenCount(msgs)
	Ck(err)

	Debug("summarize: msgsCount=%d, maxTokens=%d", msgsCount, maxTokens)
	// Debug("summarize: msgsBefore=%v", Spprint(msgs))

	minTokens := g.tokenLimit / 10
	Assert(maxTokens > minTokens, "maxTokens must be greater than %d", minTokens)

	// if we are already within the limit, return
	if msgsCount <= maxTokens {
		summarized = msgs
		Debug("summarize: done")
		return
	}

	var sysmsg string
	if !fast {
		// generate a sysmsg that includes a short summary of the prompt
		topic, err := g.Msg("Summarize the topic in one sentence.", prompt)
		Ck(err)
		sysmsg = Spf(SysMsgSummarizeChat, topic)
	}
	sysmsgTc, err := g.TokenCount(sysmsg)
	Ck(err)
	Debug("summarize: sysmsgTc=%d", sysmsgTc)

	// find the middle message, where "middle" is defined as the point
	// either maxTokens from the start or endStop tokens from the end,
	// whichever comes first
	endStop := maxTokens - sysmsgTc*2
	Debug("endstop=%d", endStop)
	var middleI int
	// total token count of the first half of the messages
	var firstHalfCount int
	for i, msg := range msgs {
		// count tokens
		count, err := g.TokenCount(msg.Txt)
		Ck(err)
		if firstHalfCount+count > endStop {
			// we are nearing maxTokens from the end
			middleI = i
			break
		}
		if firstHalfCount+count > maxTokens {
			// we are nearing maxTokens from the start
			middleI = i
			break
		}
		firstHalfCount += count
	}
	// define a variable that shows where the second half of the messages
	// will begin
	secondHalfI := middleI + 1

	// Rewrite the middle message by splitting it into two messages
	// from the same role, such that the first message when added to
	// the first half of the messages will cause firstHalfCount to be
	// maxTokens.  We go to all this trouble because the middle
	// message might be longer than maxTokens.
	middleMsg := msgs[middleI]
	txt1, txt2 := g.splitAt(middleMsg.Txt, maxTokens-firstHalfCount)
	// create two new messages
	msg1 := ChatMsg{Role: middleMsg.Role, Txt: txt1}
	msg2 := ChatMsg{Role: middleMsg.Role, Txt: txt2}
	// replace the middle message with the first new message
	msgs[middleI] = msg1
	// append a new message to the end of the slice as a placeholder
	msgs = append(msgs, ChatMsg{})
	// shift the second half of the messages right by one
	// - this copies over the placeholder
	copy(msgs[secondHalfI+1:], msgs[secondHalfI:])
	// insert the second new message as the first message in the
	// second half of the messages
	msgs[secondHalfI] = msg2

	if !fast {
		for len(summarized) == 0 {
			// summarize the first half of the messages by converting them to
			// a text format that GPT-4 can understand, sending them to GPT-4,
			// and then converting the response back to a slice of messages.
			// The format is the same as the chat history file format.
			txt := history.chat2txt(msgs[:secondHalfI])
			// send to GPT-4
			resp, err := g.Msg(sysmsg, txt)
			Ck(err)
			// convert the response back to a slice of messages
			// Debug("summarize: resp=%s", resp)
			summarized = history.parseChat(resp)
		}
	}

	// append the second half of the messages to the summarized messages
	summarized = append(summarized, msgs[secondHalfI:]...)

	// recurse
	return history.summarize(prompt, summarized, maxTokens, fast)
}

// chat2txt returns the given history messages as a text string.
func (history *ChatHistory) chat2txt(msgs []ChatMsg) (txt string) {
	for _, msg := range msgs {
		if strings.TrimSpace(msg.Txt) == "" {
			continue
		}
		txt += Spf("%s:\n%s\n\n", msg.Role, msg.Txt)
	}
	return
}

// parseChat parses a chat history from a string.  See summarize for
// the format.
func (history *ChatHistory) parseChat(txt string) (msgs []ChatMsg) {
	// split into lines
	lines := strings.Split(txt, "\n")
	// the first line of each message is the role followed by a colon
	// and an optional line of text
	pattern := `^([A-Z]+):(.*$)`
	re := regexp.MustCompile(pattern)
	var msg *ChatMsg
	// parse each line
	for _, line := range lines {
		// look for a role
		m := re.FindStringSubmatch(line)
		if len(m) > 0 {
			if msg != nil {
				// add the previous message to the slice
				msgs = append(msgs, *msg)
			}
			role := history.fixRole(m[1])
			txt := strings.TrimSpace(m[2])
			// create a new message
			msg = &ChatMsg{Role: role, Txt: txt}
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

// tokenCount returns the number of tokens in the given chat history.
func (history *ChatHistory) tokenCount(msgs []ChatMsg) (count int, err error) {
	defer Return(&err)
	g := history.g
	// convert the chat history to a text string
	txt := history.chat2txt(msgs)
	// get the token count
	count, err = g.TokenCount(txt)
	Ck(err)
	return
}

// splitAt splits a string at the given token count.  It returns two
// strings, the first of which is the first part of the string, and
// the second of which is the second part of the string.
func (g *GrokkerInternal) splitAt(txt string, tokenCount int) (txt1, txt2 string) {
	middle := len(txt) / 2
	// use binary search to find the middle
	move := middle / 2
	for {
		// split the string in half
		txt1 = txt[:middle]
		txt2 = txt[middle:]
		// count the tokens in the first half
		count, err := g.TokenCount(txt1)
		Ck(err)
		// move a shorter distance
		move = int(float64(move) * 0.5)
		if move == 0 {
			// we are done without finding the exact token count
			break
		}
		// if the token count is too high, move the middle to the left
		if count > tokenCount {
			middle -= move
			continue
		}
		// if the token count is too low, move the middle to the right
		if count < tokenCount {
			middle += move
			continue
		}
		// if we get here, count == tokenCount
		break
	}
	return
}
