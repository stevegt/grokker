package grokker

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
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
	// path := filepath.Join(g.Root, relPath)
	path := relPath
	_, err = os.Stat(path)
	if err != nil {
		// file does not exist
		// XXX move sysmsg into SYSMSG headings in the file so we can
		// track changes to the sysmsg
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

	return
}

// continueChat continues a chat history.  The debug map contains
// interesting statistics about the process, for testing and debugging
// purposes.
func (history *ChatHistory) continueChat(prompt string, level ContextLevel, infiles, outfiles []string) (resp string, debug map[string]int, err error) {
	defer Return(&err)
	g := history.g

	// include the input files in the prompt
	prompt, err = history.includeFiles(prompt, infiles)
	if err != nil {
		return "", nil, err
	}

	Debug("continueChat: prompt len=%d", len(prompt))
	Debug("continueChat: context level=%s", level)

	// create a temporary slice of messages to work with
	var msgs []ChatMsg

	if level != ContextRecent {
		// get context
		maxTokens := int(float64(g.tokenLimit) * 0.5)
		var context string
		var files []string
		if level == ContextChat {
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
		Debug("continueChat: context len=%d", len(context))
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
	msgs, err = history.summarize(prompt, msgs, maxTokens, level)
	Ck(err)

	finalCount, err := history.tokenCount(msgs)
	Ck(err)

	debug = map[string]int{
		"peakCount":  peakCount,
		"finalCount": finalCount,
	}

	sysmsg := history.Sysmsg
	// add regex to sysmsg if outfiles are provided
	if len(outfiles) > 0 {
		sysmsg += Spf("\nYour response must include the following files: '%s'", strings.Join(outfiles, "', '"))
		sysmsg += Spf("\nYour response must match this regular expression: '%s'", OutfilesRegex(true))
		sysmsg += Spf("\n...where each file is in the format:\n\n<blank line>\nFile: <filename>\n```<language>\n<text>\n```")
		Fpf(os.Stderr, "System message:  %s\n", sysmsg)
	}
	Debug("continueChat: sysmsg %s", sysmsg)

	// generate the response
	Fpf(os.Stderr, "Sending %d tokens to OpenAI...\n", finalCount)
	resp, err = g.completeChat(sysmsg, msgs)

	// append the prompt and response to the stored messages
	history.msgs = append(history.msgs, ChatMsg{Role: "USER", Txt: prompt})
	history.msgs = append(history.msgs, ChatMsg{Role: "AI", Txt: resp})

	// save the output files
	err = history.extractFiles(outfiles, resp, false)

	return
}

// summarize summarizes a chat history until it is within
// maxTokens.  It always leaves the last message intact.
func (history *ChatHistory) summarize(prompt string, msgs []ChatMsg, maxTokens int, level ContextLevel) (summarized []ChatMsg, err error) {
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
	if level != ContextRecent {
		Fpf(os.Stderr, "Summarizing %d tokens...\n", msgsCount)
		// generate a sysmsg that includes a short summary of the prompt
		topic, err := g.Msg("Summarize the topic in one sentence.", prompt)
		Ck(err)
		sysmsg = Spf(SysMsgSummarizeChat, topic)
	}
	sysmsgTc, err := g.TokenCount(sysmsg)
	Ck(err)
	Debug("summarize: sysmsgTc=%d", sysmsgTc)

	/*
		// find the middle message, where "middle" is defined as
		// either maxTokens from the start or maxTokens from the end,
		// whichever comes first
		endStop := msgsCount - maxTokens - sysmsgTc*2
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
	*/

	// find the middle message, where "middle" is defined as
	// either maxTokens from the start or msgsCount/2, whichever
	// comes first
	var middleI int
	// total token count of the first half of the messages
	var firstHalfCount int
	for i, msg := range msgs {
		// count tokens
		count, err := g.TokenCount(msg.Txt)
		Ck(err)
		if firstHalfCount+count > msgsCount/2 {
			// we are nearing msgsCount/2
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

	if level != ContextRecent {
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
	return history.summarize(prompt, summarized, maxTokens, level)
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
	// path := filepath.Join(history.g.Root, history.relPath)
	path := history.relPath
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

	// call AddDocument to update the embeddings
	err = history.g.AddDocument(path)
	Ck(err)

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

// OutfilesRegex returns a regular expression that matches the format
// of output files embedded in chat responses.  The 'repeat' flag
// wraps the regex in a non-capturing group that allows the regex to
// match multiple files.
func OutfilesRegex(repeat bool) string {
	/*
		The regex matches the following:

		<ignored optional text>
		File: <filename>
		```foo
		<text>
		```
		<ignored optional text>
		File: <filename>
		```foo
		<text>
		```
		<ignored optional text>
		...

	*/

	// (?:...) is a non-capturing group
	// (?s:...) is a dot-all group
	// (?i:...) is a case-insensitive group

	openCodeBlock := "```" + `(\w*)\n`
	// we use extra characters in the close code block to ensure that
	// we don't match any close code block embedded in the text
	closeCodeBlock := "```" + `\nEOF\n`
	text := `(?s)(.*)`
	header := Spf(`(?:^|\n)(?i)File:\s*([\w\-\.]+)\s*\n`)
	body := Spf("%s%s%s", openCodeBlock, text, closeCodeBlock)
	zeroOrMoreBlankLines := `(?:\s*\n)*`

	// the regex for a single file
	singleFile := Spf(`%s%s%s`, header, zeroOrMoreBlankLines, body)

	// the regex for multiple files
	// - go doesn't support this, but GPT-4 seems to
	multipleFiles := Spf(`^(?:%s)+$`, singleFile)

	if repeat {
		return multipleFiles
	}
	return singleFile
}

// includeFiles includes the given files in the prompt.  It returns a
// new prompt string.
func (history *ChatHistory) includeFiles(prompt string, files []string) (newPrompt string, err error) {
	newPrompt = prompt
	for _, fn := range files {
		buf, err := ioutil.ReadFile(fn)
		if err != nil {
			return "", fmt.Errorf("could not read file '%s': %s", fn, err)
		}
		txt := string(buf)
		// ensure that the file ends with a newline
		if !strings.HasSuffix(txt, "\n") {
			txt += "\n"
		}
		newPrompt += Spf("\n\nFile: %s\n```%s\n```\n", fn, txt)
	}
	return
}

// extractFromChat extracts the Nth most recent version of the given
// files from the chat history and saves them to the given files,
// overwriting any existing files.  The most recent version is N=1.
func (history *ChatHistory) extractFromChat(outfiles []string, N int) (err error) {
	defer Return(&err)
	for _, fn := range outfiles {
		foundN := 0
		// iterate over responses starting with the most recent
		for i := len(history.msgs) - 1; i >= 0; i-- {
			// skip messages that are not from the AI
			if history.msgs[i].Role != "AI" {
				continue
			}
			// see if we have a match for this file
			dryrun := true
			err = history.extractFiles([]string{fn}, history.msgs[i].Txt, dryrun)
			if err != nil {
				// file not found in this response
				continue
			}
			foundN++
			if foundN == N {
				// we found the Nth most recent version of the file
				dryrun = false
				err = history.extractFiles([]string{fn}, history.msgs[i].Txt, dryrun)
				Ck(err)
				break
			}
		}
		if foundN < N {
			return fmt.Errorf("file '%s' was not found in the chat history", fn)
		}
	}
	return
}

// extractFiles extracts the output files from the given response and
// saves them to the given files, overwriting any existing files.
func (history *ChatHistory) extractFiles(outfiles []string, resp string, dryrun bool) (err error) {
	defer Return(&err)
	// compile the regex
	re := regexp.MustCompile(OutfilesRegex(false))
	// find all matches
	matches := re.FindAllStringSubmatch(resp, -1)
	// loop over the expected outfiles
	// - we ignore any files the AI provides that are not in the list
	for _, fn := range outfiles {
		// see if we have a match for this file
		found := false
		txt := ""
		// match[1] is file name
		// match[2] is language
		// match[3] is text
		for _, match := range matches {
			if match[1] == fn {
				found = true
				txt = match[3]
				break
			}
			// Pf("fn='%s', match[2]='%s'\n", fn, match[2])
		}
		if !found {
			if dryrun {
				return fmt.Errorf("file '%s' was not found in the response", fn)
			}
			// return fmt.Errorf("file '%s' was not found in the response", fn)
			Fpf(os.Stderr, "Warning: file not found in the response: '%s'\n", fn)
			continue
		}
		if !dryrun {
			// save the text to the file
			// path := filepath.Join(history.g.Root, fn)
			err = os.WriteFile(fn, []byte(txt), 0644)
			Ck(err)
		}
	}
	return
}
