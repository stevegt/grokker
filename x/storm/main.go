package main

import (
	"bytes"
	"context"
	_ "embed"
	"encoding/json"
	"flag"
	"fmt"
	"html/template"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"strings"
	"sync"

	"github.com/stevegt/grokker/x/storm/split"

	"github.com/gofrs/flock"
	. "github.com/stevegt/goadapt"
	"github.com/stevegt/grokker/v3/client"
	"github.com/stevegt/grokker/v3/core"
	"github.com/stevegt/grokker/v3/util"
	"github.com/yuin/goldmark"
)

//go:embed index.html
var indexHTML string

var tmpl = template.Must(template.New("index").Parse(indexHTML))

// QueryRequest represents a user's query input.
type QueryRequest struct {
	Query      string   `json:"query"`
	LLM        string   `json:"llm"`
	Selection  string   `json:"selection"`
	InputFiles []string `json:"inputFiles"`
	OutFiles   []string `json:"outFiles"`
	WordCount  int      `json:"wordCount"`
}

// QueryResponse represents the LLM's response.
type QueryResponse struct {
	Response string `json:"response"`
}

// ChatRound contains a user query and its corresponding response.
type ChatRound struct {
	Query    string
	Response string
}

// Chat encapsulates chat history and synchronization.
type Chat struct {
	mutex    sync.RWMutex
	history  []*ChatRound
	filename string
}

// NewChat creates a new Chat instance using the given markdown filename.
// If the file exists, its content is loaded as the initial chat history.
func NewChat(filename string) *Chat {
	var history []*ChatRound
	if _, err := os.Stat(filename); err == nil {
		content, err := ioutil.ReadFile(filename)
		if err != nil {
			log.Printf("failed to read markdown file: %v", err)
		} else {

			// load the markdown file and parse it into chat rounds.
			roundTrips, err := split.Parse(bytes.NewReader(content))
			Ck(err)
			for _, rt := range roundTrips {
				response := Spf("%s\n\n## References\n\n%s\n\n## Reasoning\n\n%s\n\n", rt.Response, rt.References, rt.Reasoning)
				chatRound := &ChatRound{
					Query:    rt.Query,
					Response: response,
				}
				history = append(history, chatRound)
			}
		}
	}
	return &Chat{
		history:  history,
		filename: filename,
	}
}

// TotalRounds returns the total number of chat rounds.
func (c *Chat) TotalRounds() int {
	c.mutex.RLock()
	defer c.mutex.RUnlock()
	return len(c.history)
}

// _updateMarkdown writes the current chat history to the markdown file.
func (c *Chat) _updateMarkdown() error {

	// Convert the chat history slice into markdown content.
	// We don't need getHistory to lock, since we're already holding
	// the mutex, so 'false'.
	content := c.getHistory(false)

	// Write the old content to a backup file.
	if oldContent, err := ioutil.ReadFile(c.filename); err == nil {
		backupName := c.filename + ".bak.md"
		if err := ioutil.WriteFile(backupName, oldContent, 0644); err != nil {
			log.Printf("failed to create backup: %v", err)
		}
	}

	// Write the new content to a temporary file.
	tempFile, err := ioutil.TempFile("", "storm-chat-*.md")
	if err != nil {
		log.Printf("failed to create temporary file: %v", err)
		return fmt.Errorf("failed to create temporary file: %w", err)
	}
	log.Printf("created temporary file %s", tempFile.Name())
	defer os.Remove(tempFile.Name())
	if _, err := tempFile.WriteString(content); err != nil {
		log.Printf("failed to write to temporary file: %v", err)
		return fmt.Errorf("failed to write to temporary file: %w", err)
	}
	tempFile.Close()
	if err := os.Rename(tempFile.Name(), c.filename); err != nil {
		log.Printf("failed to rename temporary file to %s: %v", c.filename, err)
		return fmt.Errorf("failed to rename temporary file to %s: %w", c.filename, err)
	}
	log.Printf("updated markdown file %s", c.filename)
	return nil
}

// StartRound initializes a chat round.
func (c *Chat) StartRound(query, selection string) (r *ChatRound) {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	round := &ChatRound{}
	q := strings.TrimSpace(query)
	if selection != "" {
		q = fmt.Sprintf("%s: [%s]", q, selection)
	}
	round.Query = q
	c.history = append(c.history, round)
	log.Printf("started chat round: %s", query)
	return round
}

// FinishRound finalizes a chat round.
func (c *Chat) FinishRound(r *ChatRound, response string) error {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	if r == nil {
		return fmt.Errorf("cannot finish a nil chat round")
	}
	r.Response = response
	err := c._updateMarkdown()
	if err != nil {
		log.Printf("error updating markdown: %v", err)
		return fmt.Errorf("error updating markdown: %w", err)
	}
	log.Printf("finished chat round: %s", r.Query)
	return nil
}

// getHistory returns the chat history as markdown.
func (c *Chat) getHistory(lock bool) string {
	if lock {
		c.mutex.RLock()
		defer c.mutex.RUnlock()
	}
	var result string
	for _, msg := range c.history {
		// skip rounds with empty responses -- they're still pending.
		if msg.Response == "" {
			continue
		}
		if msg.Query != "" {
			result += fmt.Sprintf("\n\n**%s**\n", msg.Query)
		}
		result += fmt.Sprintf("\n\n%s\n\n---\n\n", msg.Response)
	}
	return result
}

var chat *Chat
var grok *core.Grokker
var srv *http.Server

func main() {

	fmt.Println("index.html length:", len(indexHTML))

	fmt.Println("storm v0.0.75")
	port := flag.Int("port", 8080, "port to listen on")
	filePtr := flag.String("file", "", "markdown file to store chat history")
	flag.Parse()
	if *filePtr == "" {
		log.Fatal("must provide a markdown filename with -file")
	}

	var err error
	var lock *flock.Flock
	grok, _, _, _, lock, err = core.Load("", true)
	if err != nil {
		log.Fatalf("failed to load Grokker: %v", err)
	}
	defer lock.Unlock()

	chat = NewChat(*filePtr)

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		log.Printf("Received request for %s", r.URL.Path)
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		chatContent := chat.getHistory(true)
		data := struct {
			ChatHTML template.HTML
		}{
			ChatHTML: template.HTML(markdownToHTML(chatContent)),
		}
		if err := tmpl.Execute(w, data); err != nil {
			http.Error(w, "Template error", http.StatusInternalServerError)
		}
	})

	http.HandleFunc("/query", queryHandler)
	http.HandleFunc("/tokencount", tokenCountHandler)
	http.HandleFunc("/rounds", roundsHandler)
	http.HandleFunc("/stop", stopHandler)
	http.HandleFunc("/open", openHandler)

	addr := fmt.Sprintf(":%d", *port)
	srv = &http.Server{Addr: addr}
	log.Printf("Starting server on %s\n", addr)
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatal(err)
	}
}

// openHandler serves a file based on the filename query parameter.
func openHandler(w http.ResponseWriter, r *http.Request) {
	filename := r.URL.Query().Get("filename")
	if filename == "" {
		http.Error(w, "Missing filename parameter", http.StatusBadRequest)
		return
	}
	if _, err := os.Stat(filename); os.IsNotExist(err) {
		http.Error(w, "File not found", http.StatusNotFound)
		return
	}
	http.ServeFile(w, r, filename)
}

// stopHandler gracefully shuts down the server.
func stopHandler(w http.ResponseWriter, r *http.Request) {
	log.Printf("Received stop server request: %s", r.URL.Path)
	if r.Method != "POST" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("Server stopping"))
	// Shutdown the server gracefully in a separate goroutine.
	go func() {
		if err := srv.Shutdown(context.Background()); err != nil {
			log.Printf("Error shutting down server: %v", err)
		}
	}()
}

// roundsHandler returns the total number of chat rounds as JSON.
func roundsHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	rounds := chat.TotalRounds()
	json.NewEncoder(w).Encode(map[string]int{"rounds": rounds})
}

var TailLength = 300000

// queryHandler processes each query, sends it to the Grokker API,
// updates the markdown file with the current chat state, and returns the LLM response as HTML.
func queryHandler(w http.ResponseWriter, r *http.Request) {
	log.Printf("Received query request: %s", r.URL.Path)
	if r.Method != "POST" {
		log.Printf("Method not allowed: %s", r.Method)
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req QueryRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		log.Printf("error decoding request body: %v", err)
		http.Error(w, "Bad request: "+err.Error(), http.StatusBadRequest)
		return
	}

	// Removed word count manipulation from here.
	round := chat.StartRound(req.Query, req.Selection)
	history := chat.getHistory(true)
	// add the last TailLength characters of the chat history as context.
	// XXX should really use embeddings and a vector db to find relevant context.
	startIndex := len(history) - TailLength
	if startIndex < 0 {
		startIndex = 0
	}
	lastN := history[startIndex:]
	lastNTokenCount, err := grok.TokenCount(lastN)
	if err != nil {
		log.Printf("Token count error: %v", err)
		lastNTokenCount = 0
	}
	log.Printf("Added %d tokens of context to query: %s", lastNTokenCount, req.Query)

	// Pass the word count along to sendQueryToLLM.
	responseText := sendQueryToLLM(req.Query, req.LLM, req.Selection, lastN, req.InputFiles, req.OutFiles, req.WordCount)

	// convert references to a bulleted list
	refIndex := strings.Index(responseText, "<references>")
	if refIndex != -1 {
		refEndIndex := strings.Index(responseText, "</references>") + len("</references>")
		// every non-blank line after <references> is a reference --
		// insert a '- ' before each line until we hit the closing tag.
		firstRefIndex := refIndex + len("<references>")
		references := strings.Split(responseText[firstRefIndex:], "\n")
		var refLines []string
		for _, line := range references {
			line = strings.TrimSpace(line)
			if line == "</references>" {
				break // stop at the closing tag
			}
			if line == "" {
				continue // skip empty lines
			}

			// if the line looks like [N] followed by a URL, convert
			// the URL to a markdown link.
			regex := `^\s*\[(\d+)\]\s*(http[s]?://\S+)\s*$`
			re := regexp.MustCompile(regex)
			m := re.FindStringSubmatch(line)
			if len(m) > 0 {
				// m[1] is the reference number, m[2] is the URL
				line = fmt.Sprintf("- [%s] [%s](%s)", m[1], m[2], m[2])
			}

			refLines = append(refLines, line)
		}
		// replace the original <references> section with the new ## References section.
		beforeRefs := responseText[:refIndex]
		refHead := "\n\n## References\n\n"
		afterRefs := responseText[refEndIndex:]
		responseText = beforeRefs + refHead + strings.Join(refLines, "\n") + "\n" + afterRefs
	}

	// move the <think> section to the end of the response
	thinkIndex := strings.Index(responseText, "<think>")
	if thinkIndex != -1 {
		thinkEndIndex := strings.Index(responseText, "</think>") + len("</think>")
		if thinkEndIndex > thinkIndex {
			thinkSection := responseText[thinkIndex:thinkEndIndex]
			// remove the think section from the response
			responseText = responseText[:thinkIndex] + responseText[thinkEndIndex:]
			// append the think section to the end of the response
			responseText += "\n\n" + thinkSection
		} else {
			log.Printf("Malformed <think> section in response: %s", responseText)
		}
	}
	// convert <think> tags to a markdown heading
	replacer := strings.NewReplacer("<think>", "## Reasoning\n", "</think>", "")
	responseText = replacer.Replace(responseText)

	err = chat.FinishRound(round, responseText)
	if err != nil {
		http.Error(w, "Internal server error: "+err.Error(), http.StatusInternalServerError)
		return
	}

	resp := QueryResponse{
		Response: markdownToHTML(responseText) + "\n\n<hr>\n\n",
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

// tokenCountHandler calculates the token count for the current conversation
// using Grokker's TokenCount function and returns it as JSON.
func tokenCountHandler(w http.ResponseWriter, r *http.Request) {
	chatText := chat.getHistory(true)
	count, err := grok.TokenCount(chatText)
	if err != nil {
		log.Printf("Token count error: %v", err)
		count = 0
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]int{"tokens": count})
}

// sendQueryToLLM calls the Grokker API to obtain a markdown-formatted text.
func sendQueryToLLM(query string, llm string, selection, backgroundContext string, inputFiles []string, outFiles []string, wordCount int) string {
	// Move word count handling into LLM prompt construction.
	if wordCount == 0 {
		// limit to 100 words by default if wordCount not specified
		wordCount = 100
	}
	query = query + "\n\nPlease limit your response to " + strconv.Itoa(wordCount) + " words."

	sysmsg := "You are a researcher.  I will start my prompt with some context, followed by a query.  Answer the query -- don't answer other questions you might see elsewhere in the context.  Always enclose reference numbers in square brackets; ignore empty brackets in the prompt or context, and DO NOT INCLUDE EMPTY SQUARE BRACKETS in your response, regardless of what you see in the context.  Always start your response with a markdown heading."

	prompt := fmt.Sprintf("---CONTEXT START---\n%s\n---CONTEXT END---\n\nNew Query: %s", backgroundContext, query)
	if selection != "" {
		prompt += fmt.Sprintf(" {%s}", selection)
	}

	msgs := []client.ChatMsg{
		{Role: "USER", Content: prompt},
	}
	var outFilesConverted []core.FileLang
	for _, f := range outFiles {
		lang, known, err := util.Ext2Lang(f)
		Ck(err)
		if !known {
			log.Printf("Unknown file extension for output file %s; assuming language is %s", f, lang)
		}
		outFilesConverted = append(outFilesConverted, core.FileLang{File: f, Language: lang})
	}
	fmt.Printf("Sending query to LLM '%s'\n", llm)
	fmt.Printf("Query: %s\n", query)
	response, _, err := grok.SendWithFiles(llm, sysmsg, msgs, inputFiles, outFilesConverted)
	if err != nil {
		log.Printf("SendWithFiles error: %v", err)
		return fmt.Sprintf("Error sending query: %v", err)
	}
	fmt.Printf("Received response from LLM '%s'\n", llm)
	fmt.Printf("Response: %s\n", response)

	cookedResponse, err := core.ExtractFiles(outFilesConverted, response, core.ExtractOptions{
		DryRun:             false,
		ExtractToStdout:    false,
		RemoveFromResponse: true,
	})

	return cookedResponse
}

// splitMarkdown splits the markdown input into sections separated by a horizontal rule (^---$).
func splitMarkdown(input string) []string {
	re := regexp.MustCompile("(?m)^---$")
	sections := re.Split(input, -1)
	return sections
}

// collectReferences scans the markdown input for reference lines of the form "- [N] URL"
// and returns a map of URLs keyed by the reference number.
func collectReferences(input string) map[string]string {
	re := regexp.MustCompile(`(?m)^-\s+\[(\d+)\]\s+\[(http[s]?://\S+)\]`)
	matches := re.FindAllStringSubmatch(input, -1)
	refs := make(map[string]string)
	for _, m := range matches {
		if len(m) == 3 {
			refs[m[1]] = m[2]
		}
	}
	return refs
}

// linkifyReferences replaces occurrences of references like "[N]" with markdown links to the corresponding URL.
func linkifyReferences(input string, refs map[string]string) string {
	re := regexp.MustCompile(`\[(\d+)\]`)
	result := re.ReplaceAllStringFunc(input, func(match string) string {
		m := re.FindStringSubmatch(match)
		if len(m) == 2 {
			url, ok := refs[m[1]]
			if ok && m[1] != "" && url != "" {
				return fmt.Sprintf("[[%s](%s)]", m[1], url)
			}
		}
		return match
	})
	return result
}

// markdownToHTML converts markdown text to HTML using goldmark.
// It first splits the markdown into sections, collects any reference URLs, and replaces each "[N]"
// with a markdown link to the corresponding URL before rendering.
func markdownToHTML(markdown string) string {

	// linkify references in the markdown
	sections := splitMarkdown(markdown)
	for i, sec := range sections {
		refs := collectReferences(sec)
		// log.Printf("Found %d references in section %d", len(refs), i)
		sections[i] = linkifyReferences(sec, refs)
	}
	processed := strings.Join(sections, "\n\n---\n\n")

	/*
		// replace '^---$' with an HTML horizontal rule
		pattern := regexp.MustCompile("(?m)^---$")
		processed = pattern.ReplaceAllString(processed, "<hr>")
	*/

	var buf bytes.Buffer
	if err := goldmark.Convert([]byte(processed), &buf); err != nil {
		log.Printf("Markdown conversion error: %v", err)
		return "<p>Error rendering markdown</p>"
	}

	return buf.String()
}
