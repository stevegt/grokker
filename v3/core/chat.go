package core

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"regexp"
	"strings"
	"time"

	. "github.com/stevegt/goadapt"
	"github.com/stevegt/grokker/v3/client"
	"github.com/stevegt/grokker/v3/util"
)

type ChatHistory struct {
	Sysmsg  string
	Version string
	relPath string
	msgs    []client.ChatMsg
	g       *Grokker
}

type ChatMsg struct {
	Role    string
	Content string
}

const (
	RoleSystem = "SYSTEM"
	RoleUser   = "USER"
	RoleAI     = "ASSISTANT"
)

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
// ...where <role> is either "USER" or "ASSISTANT", and <message> is the text
// of the message.  The last message in the file is the most recent
// message.
func (g *Grokker) OpenChatHistory(sysmsg, relPath string) (history *ChatHistory, err error) {
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
			msgs:    make([]client.ChatMsg, 0),
			Version: Version}
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

// ContinueChat continues a chat history.  The debug map contains
// interesting statistics about the process, for testing and debugging
// purposes.
func (history *ChatHistory) ContinueChat(modelName, prompt string, contextLevel util.ContextLevel, infiles []string, outfiles []FileLang, promptTokenLimit int, edit bool) (resp string, debug map[string]int, err error) {
	defer Return(&err)
	g := history.g

	Debug("continueChat: context level=%s", contextLevel)

	// create a temporary slice of messages to work with
	var msgs []client.ChatMsg

	// add context
	getContext := false
	appendMsgs := false
	var files []string
	switch contextLevel {
	case util.ContextNone:
		// no context
	case util.ContextRecent:
		// get context only from the most recent messages
		appendMsgs = true
	case util.ContextChat:
		// get context from the entire chat history file
		files = []string{history.relPath}
		getContext = true
		appendMsgs = true
	case util.ContextAll:
		// get context from all files in the knowledge base -- this
		// includes the chat history file itself, because it is a
		// document in the knowledge base
		files = nil
		getContext = true
		appendMsgs = true
	default:
		Assert(false, "invalid context level: %s", contextLevel)
	}

	if getContext {
		maxTokens := int(float64(g.ModelObj.TokenLimit) * 0.5)
		if promptTokenLimit > 0 {
			maxTokens = promptTokenLimit
		}
		var context string
		context, err = g.getContext(prompt, maxTokens, false, false, files)
		Ck(err)
		if context != "" {
			// make context look like a message exchange
			msgs = []client.ChatMsg{
				client.ChatMsg{Role: "USER", Content: context},
				client.ChatMsg{Role: "ASSISTANT", Content: "I understand the context."},
			}
		}
		Debug("continueChat: context len=%d", len(context))
	}

	if appendMsgs {
		// append the most recent messages to the context
		msgs = append(msgs, history.msgs...)
	}

	if edit {
		// get the prompt from the most recent message
		Assert(len(prompt) == 0, "edit mode expects an empty prompt")
		prompt = history.msgs[len(history.msgs)-1].Content
	} else {
		// append the prompt to the context
		msgs = append(msgs, client.ChatMsg{Role: "USER", Content: prompt})
	}

	peakCount, err := history.tokenCount(msgs)
	Ck(err)

	// summarize
	promptTc, err := g.TokenCount(prompt)
	Debug("continueChat: promptTc=%d", promptTc)
	Ck(err)
	maxTokens := g.ModelObj.TokenLimit / 2
	if promptTokenLimit > 0 {
		maxTokens = promptTokenLimit
	}
	msgs, err = history.summarize(modelName, prompt, msgs, maxTokens, contextLevel)
	Ck(err)

	finalCount, err := history.tokenCount(msgs)
	Ck(err)

	debug = map[string]int{
		"peakCount":  peakCount,
		"finalCount": finalCount,
	}
	Fpf(os.Stderr, "Sending %d tokens to OpenAI...\n", finalCount)

	// generate the response
	resp, _, err = g.SendWithFiles(modelName, history.Sysmsg, msgs, infiles, outfiles)
	Ck(err)

	// append the prompt and response to the stored messages
	history.msgs = append(history.msgs, client.ChatMsg{Role: "USER", Content: prompt})
	history.msgs = append(history.msgs, client.ChatMsg{Role: "ASSISTANT", Content: resp})

	// save the output files
	_, err = ExtractFiles(outfiles, resp, ExtractOptions{
		DryRun:          false,
		ExtractToStdout: false,
	})
	Ck(err)

	return
}

// SendWithFiles sends a chat to GPT-4 and returns the
// response.  It assumes the prompt is the last message in the
// history.  The sysmsg is a message that is sent to GPT-4 before the
// prompt.  The msgs are the chat history up to the prompt.  The
// infiles are the input files that are included in the prompt.  The
// outfiles are the output files that are required in the response.
func (g *Grokker) SendWithFiles(modelName, sysmsg string, msgs []client.ChatMsg, infiles []string, outfiles []FileLang) (resp string, ref []string, err error) {
	defer Return(&err)

	if len(infiles) > 0 {
		// include the input files in the prompt
		promptFrag, err := IncludeFiles(infiles)
		Ck(err)
		// append the prompt fragment to the last message
		msgs[len(msgs)-1].Content += promptFrag
	}

	if len(outfiles) > 0 {
		// require the output files in the response
		var fns []string
		for _, fn := range outfiles {
			fns = append(fns, fn.File)
		}
		sysmsg += Spf("\nYour response must include the following complete files: '%s'", strings.Join(fns, "', '"))
		sysmsg += Spf("\nReturn complete files only.  Do not return file fragments.")
		sysmsg += Spf("\nYour response must match this regular expression: '%s'", OutfilesRegex(outfiles))
		sysmsg += "\n...where each file is in the format:\n\n---FILE-START filename=\"<filename>\"---\n[file content]\n---FILE-END filename=\"<filename>\"---"
	}
	Debug("sysmsg %s", sysmsg)

	resp, ref, err = g.CompleteChat(modelName, sysmsg, msgs)
	Ck(err)
	return
}

// summarize summarizes a chat history until it is within
// maxTokens.  It always leaves the last message intact.
func (history *ChatHistory) summarize(modelName, prompt string, msgs []client.ChatMsg, maxTokens int, contextLevel util.ContextLevel) (summarized []client.ChatMsg, err error) {
	defer Return(&err)
	g := history.g

	// count tokens
	msgsCount, err := history.tokenCount(msgs)
	Ck(err)

	Debug("summarize: msgsCount=%d, maxTokens=%d", msgsCount, maxTokens)
	// Debug("summarize: msgsBefore=%v", Spprint(msgs))

	minTokens := g.ModelObj.TokenLimit / 10
	Assert(maxTokens > minTokens, "maxTokens must be greater than %d", minTokens)

	// if we are already within the limit, return
	if msgsCount <= maxTokens {
		summarized = msgs
		Debug("summarize: done")
		return
	}

	var sysmsg string
	if contextLevel > util.ContextRecent {
		Fpf(os.Stderr, "Summarizing %d tokens...\n", msgsCount)
		// generate a sysmsg that includes a short summary of the prompt
		topic, err := g.Msg(modelName, "Summarize the topic in one sentence.", prompt)
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
		count, err := g.TokenCount(msg.Content)
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
	txt1, txt2 := g.splitAt(middleMsg.Content, maxTokens-firstHalfCount)
	// create two new messages
	msg1 := client.ChatMsg{Role: middleMsg.Role, Content: txt1}
	msg2 := client.ChatMsg{Role: middleMsg.Role, Content: txt2}
	// replace the middle message with the first new message
	msgs[middleI] = msg1
	// append a new message to the end of the slice as a placeholder
	msgs = append(msgs, client.ChatMsg{})
	// shift the second half of the messages right by one
	// - this copies over the placeholder
	copy(msgs[secondHalfI+1:], msgs[secondHalfI:])
	// insert the second new message as the first message in the
	// second half of the messages
	msgs[secondHalfI] = msg2

	if contextLevel > util.ContextRecent {
		for len(summarized) == 0 {
			// summarize the first half of the messages by converting them to
			// a text format that GPT-4 can understand, sending them to GPT-4,
			// and then converting the response back to a slice of messages.
			// The format is the same as the chat history file format.
			txt := history.chat2txt(msgs[:secondHalfI])
			// send to GPT-4
			resp, err := g.Msg(modelName, sysmsg, txt)
			Ck(err)
			// convert the response back to a slice of messages
			// Debug("summarize: resp=%s", resp)
			summarized = history.parseChat(resp)
		}
	}

	// append the second half of the messages to the summarized messages
	summarized = append(summarized, msgs[secondHalfI:]...)

	// recurse
	return history.summarize(modelName, prompt, summarized, maxTokens, contextLevel)
}

// chat2txt returns the given history messages as a text string.
func (history *ChatHistory) chat2txt(msgs []client.ChatMsg) (txt string) {
	for _, msg := range msgs {
		if strings.TrimSpace(msg.Content) == "" {
			continue
		}
		txt += Spf("%s:\n%s\n\n", msg.Role, msg.Content)
	}
	return
}

// parseChat parses a chat history from a string.  See summarize for
// the format.
func (history *ChatHistory) parseChat(txt string) (msgs []client.ChatMsg) {
	// split into lines
	lines := strings.Split(txt, "\n")
	// the first line of each message is the role followed by a colon
	// and an optional line of text
	pattern := `^([A-Z]+):(.*$)`
	re := regexp.MustCompile(pattern)
	var msg *client.ChatMsg
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
			msg = &client.ChatMsg{Role: role, Content: txt}
			continue
		}
		if msg == nil {
			// we are in some sort of preamble -- credit it to the user
			msg = &client.ChatMsg{Role: "USER"}
		}
		// if we get here, we are in the middle of a message
		msg.Content += line + "\n"
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
	role = strings.TrimSpace(role)
	role = strings.ToUpper(role)
	switch role {
	case "USER":
		fallthrough
	case "ASSISTANT":
		fixed = role
	default:
		// default to AI for now, might need more sophisticated
		// logic later
		fixed = "ASSISTANT"
	}
	return
}

// Save saves the chat history file.
func (history *ChatHistory) Save(addToDb bool) (err error) {
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
	Assert(history.relPath != "", "relPath is required")
	path := history.relPath
	// move the existing chat history file to a backup file
	var backup string
	_, err = os.Stat(path)
	if err == nil {
		// file exists
		// get a timestamp string
		ts := time.Now().Format("20060102-150405")
		// split on dots
		parts := strings.Split(path, ".")
		// insert the timestamp before the file extension
		parts = append(parts[:len(parts)-1], ts, parts[len(parts)-1])
		// join the parts
		backup = strings.Join(parts, ".")
		err = os.Rename(path, backup)
		Ck(err)
	}
	// move the temp file to the chat history file
	err = os.Rename(fh.Name(), path)
	Ck(err)

	// XXX this should be a flag
	if false {
		// remove the backup file
		_, err = os.Stat(backup)
		if err == nil {
			// file exists
			err = os.Remove(backup)
			Ck(err)
		}
	}

	if addToDb {
		// call AddDocument to update the embeddings
		err = history.g.AddDocument(path)
		Ck(err)
	}

	Debug("chat history saved to %s", path)
	return nil
}

// tokenCount returns the number of tokens in the given chat history.
func (history *ChatHistory) tokenCount(msgs []client.ChatMsg) (count int, err error) {
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
func (g *Grokker) splitAt(txt string, tokenCount int) (txt1, txt2 string) {
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

type FileLang struct {
	File     string
	Language string
}

type FileEntry struct {
	Filename string
	Content  []string
}

type ExtractResult struct {
	RawResponse     string      // Original response unchanged
	CookedResponse  string      // Response with all files removed
	ExtractedFiles  []string    // Files that matched outfiles list
	MissingFiles    []string    // Files expected but not found in response
	BrokenFiles     []string    // Files found but missing end marker
	UnexpectedFiles []FileEntry // Files found but NOT in outfiles list
}

var fileStartTmpl = `(?:^|\n)---FILE-START filename="%s"---\n`
var fileEndTmpl = `(?:^|\n)---FILE-END filename="%s"---(?:$|\n)`
var fileStartPat = fmt.Sprintf(fileStartTmpl, "(.*?)")
var fileEndPat = fmt.Sprintf(fileEndTmpl, "(.*?)")

// OutfilesRegex returns a regular expression that matches the format
// of output files embedded in chat responses.  The Language field of
// each FileLang struct is used to generate a repeating regex that
// matches multiple files.  If the files argument is nil, the regex
// matches a single file.
func OutfilesRegex(files []FileLang) string {
	/*
		The regex matches the following:

		<ignored optional text>
		---FILE-START filename="filename"---
		[file content]
		---FILE-END filename="filename"---
		[ignored optional text]
		---FILE-START filename="filename"---
		[file content]
		---FILE-END filename="filename"---
		[ignored optional text]
		...

	*/

	// (?:...) is a non-capturing group
	// (?s:...) is a dot-all group
	// (?i:...) is a case-insensitive group

	testPat := `(?s)(.*)`

	var header, closeBlock, out string
	if len(files) == 0 {
		// single file, unknown name
		header = Spf(fileStartTmpl, `[\w\-\.]+`)
		closeBlock = Spf(fileEndTmpl, `[\w\-\.]+`)
		out = Spf(`%s%s%s`, header, testPat, closeBlock)
	} else {
		// multiple files, known names
		for _, fileLang := range files {
			header = Spf(fileStartTmpl, fileLang.File)
			closeBlock = Spf(fileEndTmpl, fileLang.File)
			out += Spf(`%s%s%s`, header, testPat, closeBlock)
		}
	}
	return out
}

// IncludeFiles returns the contents of the given files as a prompt
// fragment.
func IncludeFiles(files []string) (prompt string, err error) {
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
		prompt += Spf("\n\n---FILE-START filename=\"%s\"---\n%s\n---FILE-END filename=\"%s\"---\n", fn, txt, fn)
	}
	return
}

// extractFromChat extracts the Nth most recent version of the given
// files from the chat history and saves them to the given files,
// overwriting any existing files unless extractToStdout is true, in
// which case the extracted files are written to stdout.  The most
// recent version of a file is N=1.
func (history *ChatHistory) extractFromChat(outfiles []FileLang, N int, extractToStdout bool) (err error) {
	defer Return(&err)
	for _, fileLang := range outfiles {
		fl := []FileLang{fileLang}
		foundN := 0
		// iterate over responses starting with the most recent
		for i := len(history.msgs) - 1; i >= 0; i-- {
			// skip messages that are not from the AI
			if history.msgs[i].Role != "ASSISTANT" && history.msgs[i].Role != "AI" {
				continue
			}
			// see if we have a match for this file
			_, err := ExtractFiles(fl, history.msgs[i].Content, ExtractOptions{
				DryRun:          true,
				ExtractToStdout: false,
			})
			if err != nil {
				// file not found in this response
				continue
			}
			foundN++
			if foundN == N {
				// we found the Nth most recent version of the file
				_, err = ExtractFiles(fl, history.msgs[i].Content, ExtractOptions{
					DryRun:          false,
					ExtractToStdout: extractToStdout,
				})
				Ck(err)
				break
			}
		}
		if foundN < N {
			return fmt.Errorf("file '%s' was not found in the chat history", fl[0].File)
		}
	}
	return
}

// ExtractOptions contains options for the ExtractFiles function.
type ExtractOptions struct {
	DryRun          bool // if true, do not write files
	ExtractToStdout bool // if true, write files to stdout instead of disk
}

// ExtractFiles extracts the output files from the given response using line-by-line
// scanning to identify file blocks. It returns an ExtractResult containing metadata
// about the extraction including detected files, extracted files, unexpected files,
// missing files, and broken files (missing end markers).
func ExtractFiles(outfiles []FileLang, rawResp string, opts ExtractOptions) (result ExtractResult, err error) {
	defer Return(&err)

	result.RawResponse = rawResp
	dryrun := opts.DryRun
	extractToStdout := opts.ExtractToStdout

	// remove the first <think>.*</think> section found
	thinkStartPat := `(?i)^<think>$`
	thinkEndPat := `^</think>$`
	thinkStartRe := regexp.MustCompile(thinkStartPat)
	thinkEndRe := regexp.MustCompile(thinkEndPat)
	thinkStartPair := thinkStartRe.FindStringIndex(rawResp)
	thinkEndPair := thinkEndRe.FindStringIndex(rawResp)
	var resp string
	if thinkStartPair != nil && thinkEndPair != nil {
		// we have a think section, remove it
		thinkStartIdx := thinkStartPair[0]
		thinkEndIdx := thinkEndPair[1]
		if thinkStartIdx < thinkEndIdx {
			resp = rawResp[:thinkStartIdx] + rawResp[thinkEndIdx:]
		}
	} else {
		// no think section, use the raw response
		resp = rawResp
	}

	// Build a map of expected outfiles for quick lookup
	expectedFiles := make(map[string]bool)
	for _, fileLang := range outfiles {
		expectedFiles[fileLang.File] = true
	}

	// Compile regex patterns for file detection
	fileStartRe := regexp.MustCompile(fileStartPat)
	fileEndRe := regexp.MustCompile(fileEndPat)

	// Line-by-line scanning to detect file blocks
	lines := strings.Split(resp, "\n")
	var cookedLines []string

	// active files is a stack so files can nest
	activeFiles := []FileEntry{}

	// detectedFilesMap keeps track of files that were detected with both
	// start and end markers
	detectedFilesMap := make(map[string]bool)

	for _, line := range lines {
		// Check if this line starts a file block
		startMatches := fileStartRe.FindStringSubmatch(line)
		if len(startMatches) > 1 {
			fn := startMatches[1]
			newActiveFile := FileEntry{Filename: fn}
			// push onto stack
			activeFiles = append(activeFiles, newActiveFile)
			continue
		}

		// Check if this line ends a file block
		endMatches := fileEndRe.FindStringSubmatch(line)
		if len(endMatches) > 1 {
			fn := endMatches[1]
			// ensure stack is not empty
			if len(activeFiles) == 0 {
				// Found an end marker without a matching start marker; add to broken files
				result.BrokenFiles = append(result.BrokenFiles, fn)
			}
			// ensure fn matches the top of the stack
			top := activeFiles[len(activeFiles)-1]
			if top.Filename != fn {
				// Mismatched end marker; mark both files as broken
				result.BrokenFiles = append(result.BrokenFiles, top.Filename)
				result.BrokenFiles = append(result.BrokenFiles, fn)
			} else {
				// fn == top.Filename; finalize this file
				detectedFilesMap[fn] = true
				// pop from stack
				activeFiles = activeFiles[:len(activeFiles)-1]
				// Process the file if it's expected
				if expectedFiles[fn] {
					result.ExtractedFiles = append(result.ExtractedFiles, fn)
					fileContent := strings.Join(top.Content, "\n")
					if !dryrun {
						if extractToStdout {
							_, err = Pf("%s", fileContent)
							Ck(err)
						} else {
							err = os.WriteFile(fn, []byte(fileContent), 0644)
							Ck(err)
						}
					}
				} else {
					// File was not expected
					result.UnexpectedFiles = append(result.UnexpectedFiles, top)
				}
			}
		}

		// If we're in a file block, accumulate content in each file
		// in the stack
		if len(activeFiles) > 0 {
			for i := range activeFiles {
				activeFiles[i].Content = append(activeFiles[i].Content, line)
			}
		} else {
			// If we're not in a file block, add the line to cookedLines
			cookedLines = append(cookedLines, line)
		}
	}

	// If we ended while still in a file block, mark each active file as broken
	if len(activeFiles) > 0 {
		for _, activeFile := range activeFiles {
			result.BrokenFiles = append(result.BrokenFiles, activeFile.Filename)
		}
	}

	// Identify missing files: expected files that were not detected
	for _, fileLang := range outfiles {
		found := false
		for detectedFile := range detectedFilesMap {
			if detectedFile == fileLang.File {
				found = true
				break
			}
		}
		if !found {
			result.MissingFiles = append(result.MissingFiles, fileLang.File)
		}
	}

	// Generate cookedResponse by joining non-file lines
	result.CookedResponse = strings.Join(cookedLines, "\n")

	return
}
