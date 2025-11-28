package main

import (
	"bytes"
	"context"
	_ "embed"
	"encoding/json"
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
	"time"

	"github.com/stevegt/grokker/x/storm/split"

	"github.com/gofrs/flock"
	"github.com/gorilla/websocket"
	"github.com/spf13/cobra"
	. "github.com/stevegt/goadapt"
	"github.com/stevegt/grokker/v3/client"
	"github.com/stevegt/grokker/v3/core"
	"github.com/stevegt/grokker/v3/util"
	"github.com/yuin/goldmark"
)

//go:embed index.html
var indexHTML string

var tmpl = template.Must(template.New("index").Parse(indexHTML))

// Global variables for serve subcommand
var (
	chat       *Chat
	grok       *core.Grokker
	srv        *http.Server
	clientPool *ClientPool
	upgrader   = websocket.Upgrader{
		ReadBufferSize:  1024,
		WriteBufferSize: 1024,
		CheckOrigin: func(r *http.Request) bool {
			return true // Allow all origins for now
		},
	}
)

const (
	pingInterval = 20 * time.Second
	pongWait     = 60 * time.Second
)

// QueryRequest represents a user's query input.
type QueryRequest struct {
	Query      string   `json:"query"`
	LLM        string   `json:"llm"`
	Selection  string   `json:"selection"`
	InputFiles []string `json:"inputFiles"`
	OutFiles   []string `json:"outFiles"`
	TokenLimit int      `json:"tokenLimit"`
	QueryID    string   `json:"queryID"`
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

// WebSocket client connection.
type WSClient struct {
	conn *websocket.Conn
	send chan interface{}
	pool *ClientPool
	id   string
}

// ClientPool manages all connected WebSocket clients.
type ClientPool struct {
	clients    map[*WSClient]bool
	broadcast  chan interface{}
	register   chan *WSClient
	unregister chan *WSClient
	mutex      sync.RWMutex
}

// NewClientPool creates a new client pool.
func NewClientPool() *ClientPool {
	return &ClientPool{
		clients:    make(map[*WSClient]bool),
		broadcast:  make(chan interface{}, 256),
		register:   make(chan *WSClient),
		unregister: make(chan *WSClient),
	}
}

// Start begins the client pool's broadcast loop.
func (cp *ClientPool) Start() {
	for {
		select {
		case client := <-cp.register:
			cp.mutex.Lock()
			cp.clients[client] = true
			cp.mutex.Unlock()
			log.Printf("Client %s registered, total clients: %d", client.id, len(cp.clients))

		case client := <-cp.unregister:
			cp.mutex.Lock()
			if _, ok := cp.clients[client]; ok {
				delete(cp.clients, client)
				close(client.send)
			}
			cp.mutex.Unlock()
			log.Printf("Client %s unregistered, total clients: %d", client.id, len(cp.clients))

		case message := <-cp.broadcast:
			cp.mutex.RLock()
			for client := range cp.clients {
				select {
				case client.send <- message:
				default:
					// Client's send channel is full, skip
				}
			}
			cp.mutex.RUnlock()
		}
	}
}

// Broadcast sends a message to all connected clients.
func (cp *ClientPool) Broadcast(message interface{}) {
	cp.broadcast <- message
}

// parseTokenLimit converts shorthand notation (1K, 2M, etc.) to integer
func parseTokenLimit(val interface{}) int {
	switch v := val.(type) {
	case float64:
		return int(v)
	case string:
		v = strings.TrimSpace(strings.ToUpper(v))
		// Check for K, M, B suffixes
		if strings.HasSuffix(v, "K") {
			numStr := strings.TrimSuffix(v, "K")
			if num, err := strconv.ParseFloat(numStr, 64); err == nil {
				return int(num * 1000)
			}
		} else if strings.HasSuffix(v, "M") {
			numStr := strings.TrimSuffix(v, "M")
			if num, err := strconv.ParseFloat(numStr, 64); err == nil {
				return int(num * 1000000)
			}
		} else if strings.HasSuffix(v, "B") {
			numStr := strings.TrimSuffix(v, "B")
			if num, err := strconv.ParseFloat(numStr, 64); err == nil {
				return int(num * 1000000000)
			}
		} else if num, err := strconv.Atoi(v); err == nil {
			return num
		}
	}
	return 500 // default
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

func main() {
	fmt.Println("storm v0.0.76")

	rootCmd := &cobra.Command{
		Use:   "storm",
		Short: "Storm - Multi-project LLM chat application",
		Long:  `Storm is a single-daemon, single-port multi-project chat application for interacting with LLMs and local files.`,
	}

	// Serve command
	var port int
	var file string
	serveCmd := &cobra.Command{
		Use:   "serve",
		Short: "Start the Storm server",
		Long:  `Start the Storm server on the specified port with the given markdown file.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if file == "" {
				// XXX must not require or accept a file on startup -- files are per-project now.
				return fmt.Errorf("must provide a markdown filename with --file")
			}
			return serveRun(port, file)
		},
	}

	serveCmd.Flags().IntVar(&port, "port", 8080, "port to listen on")
	// XXX no file on startup
	serveCmd.Flags().StringVar(&file, "file", "", "markdown file to store chat history")
	serveCmd.MarkFlagRequired("file")

	rootCmd.AddCommand(serveCmd)

	// Project command (placeholder for future implementation)
	projectCmd := &cobra.Command{
		Use:   "project",
		Short: "Manage projects",
		Long:  `Manage Storm projects.`,
	}

	projectAddCmd := &cobra.Command{
		Use:   "add [projectID] [baseDir] [markdownFile]",
		Short: "Add a new project",
		Long:  `Add a new project to the registry.`,
		Args:  cobra.ExactArgs(3),
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Printf("Would add project: %s with baseDir: %s and markdown file: %s\n", args[0], args[1], args[2])
			return nil
		},
	}

	projectListCmd := &cobra.Command{
		Use:   "list",
		Short: "List all projects",
		Long:  `List all registered projects.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Println("Would list all projects")
			return nil
		},
	}

	projectCmd.AddCommand(projectAddCmd, projectListCmd)
	rootCmd.AddCommand(projectCmd)

	// File command (placeholder)
	fileCmd := &cobra.Command{
		Use:   "file",
		Short: "Manage project files",
		Long:  `Manage files associated with projects.`,
	}

	fileAddCmd := &cobra.Command{
		Use:   "add",
		Short: "Add a file to a project",
		Long:  `Add an authorized file to a project.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Println("Would add file to project")
			return nil
		},
	}

	fileCmd.AddCommand(fileAddCmd)
	rootCmd.AddCommand(fileCmd)

	// Token command (placeholder)
	tokenCmd := &cobra.Command{
		Use:   "issue-token",
		Short: "Issue a CWT token",
		Long:  `Issue a CBOR Web Token for project access.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Println("Would issue CWT token")
			return nil
		},
	}

	rootCmd.AddCommand(tokenCmd)

	if err := rootCmd.Execute(); err != nil {
		log.Fatal(err)
	}
}

// serveRun implements the serve command
func serveRun(port int, markdownFile string) error {
	var err error
	var lock *flock.Flock
	grok, _, _, _, lock, err = core.Load("", true)
	if err != nil {
		return fmt.Errorf("failed to load LLM core: %w", err)
	}
	defer lock.Unlock()

	chat = NewChat(markdownFile)
	clientPool = NewClientPool()
	go clientPool.Start()

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

	http.HandleFunc("/ws", wsHandler)
	http.HandleFunc("/tokencount", tokenCountHandler)
	http.HandleFunc("/rounds", roundsHandler)
	http.HandleFunc("/stop", stopHandler)
	http.HandleFunc("/open", openHandler)

	addr := fmt.Sprintf(":%d", port)
	srv = &http.Server{Addr: addr}
	log.Printf("Starting server on %s\n", addr)
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		return err
	}
	return nil
}

// wsHandler handles WebSocket connections.
func wsHandler(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("WebSocket upgrade error: %v", err)
		return
	}

	client := &WSClient{
		conn: conn,
		send: make(chan interface{}, 256),
		pool: clientPool,
		id:   fmt.Sprintf("client-%d", len(clientPool.clients)),
	}

	// Set up ping/pong handlers for keepalive
	conn.SetReadDeadline(time.Now().Add(pongWait))
	conn.SetPongHandler(func(string) error {
		conn.SetReadDeadline(time.Now().Add(pongWait))
		return nil
	})

	clientPool.register <- client

	go client.writePump()
	go client.readPump()
}

// writePump writes messages to the WebSocket client and sends periodic pings.
func (c *WSClient) writePump() {
	ticker := time.NewTicker(pingInterval)
	defer func() {
		ticker.Stop()
		c.conn.Close()
	}()

	for {
		select {
		case message, ok := <-c.send:
			c.conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if !ok {
				// Client pool closed the send channel
				c.conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}
			if err := c.conn.WriteJSON(message); err != nil {
				log.Printf("WebSocket write error: %v", err)
				return
			}
		case <-ticker.C:
			c.conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if err := c.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				log.Printf("WebSocket ping error: %v", err)
				return
			}
		}
	}
}

// readPump reads messages from the WebSocket client and processes queries.
func (c *WSClient) readPump() {
	defer func() {
		c.pool.unregister <- c
		c.conn.Close()
	}()

	for {
		var msg map[string]interface{}
		if err := c.conn.ReadJSON(&msg); err != nil {
			log.Printf("WebSocket read error: %v", err)
			break
		}

		// Handle incoming query messages from clients
		if msgType, ok := msg["type"].(string); ok && msgType == "query" {
			log.Printf("Received query from %s: %v", c.id, msg)

			// Extract query parameters
			query, _ := msg["query"].(string)
			llm, _ := msg["llm"].(string)
			selection, _ := msg["selection"].(string)
			queryID, _ := msg["queryID"].(string)

			// Extract arrays
			var inputFiles, outFiles []string
			if inputFilesRaw, ok := msg["inputFiles"].([]interface{}); ok {
				for _, f := range inputFilesRaw {
					if s, ok := f.(string); ok {
						inputFiles = append(inputFiles, s)
					}
				}
			}
			if outFilesRaw, ok := msg["outFiles"].([]interface{}); ok {
				for _, f := range outFilesRaw {
					if s, ok := f.(string); ok {
						outFiles = append(outFiles, s)
					}
				}
			}

			// Extract and parse tokenLimit with shorthand support (1K, 2M, etc.)
			tokenLimit := parseTokenLimit(msg["tokenLimit"])

			// Process the query
			go processQuery(queryID, query, llm, selection, inputFiles, outFiles, tokenLimit)
		}
	}
}

// processQuery processes a query and broadcasts results to all clients.
func processQuery(queryID, query, llm, selection string, inputFiles, outFiles []string, tokenLimit int) {
	// Broadcast the query to all clients
	queryBroadcast := map[string]interface{}{
		"type":    "query",
		"query":   query,
		"queryID": queryID,
	}
	clientPool.Broadcast(queryBroadcast)

	round := chat.StartRound(query, selection)

	history := chat.getHistory(true)
	// add the last TailLength characters of the chat history as context.
	const TailLength = 300000
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
	log.Printf("Added %d tokens of context to query: %s", lastNTokenCount, query)

	// Pass the token limit along to sendQueryToLLM.
	responseText, err := sendQueryToLLM(query, llm, selection, lastN, inputFiles, outFiles, tokenLimit)
	if err != nil {
		log.Printf("Error processing query: %v", err)
		// Broadcast error to all connected clients
		errorBroadcast := map[string]interface{}{
			"type":    "error",
			"queryID": queryID,
			"message": fmt.Sprintf("Error processing query: %v", err),
		}
		clientPool.Broadcast(errorBroadcast)
		return
	}

	// convert references to a bulleted list
	refIndex := strings.Index(responseText, "<references>")
	if refIndex != -1 {
		refEndIndex := strings.Index(responseText, "</references>") + len("</references>")
		firstRefIndex := refIndex + len("<references>")
		references := strings.Split(responseText[firstRefIndex:], "\n")
		var refLines []string
		for _, line := range references {
			line = strings.TrimSpace(line)
			if line == "</references>" {
				break
			}
			if line == "" {
				continue
			}

			regex := `^\s*\[(\d+)\]\s*(http[s]?://\S+)\s*$`
			re := regexp.MustCompile(regex)
			m := re.FindStringSubmatch(line)
			if len(m) > 0 {
				line = fmt.Sprintf("- [%s] [%s](%s)", m[1], m[2], m[2])
			}

			refLines = append(refLines, line)
		}
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
			responseText = responseText[:thinkIndex] + responseText[thinkEndIndex:]
			responseText += "\n\n" + thinkSection
		} else {
			log.Printf("Malformed <think> section in response: %s", responseText)
		}
	}
	replacer := strings.NewReplacer("<think>", "## Reasoning\n", "</think>", "")
	responseText = replacer.Replace(responseText)

	err = chat.FinishRound(round, responseText)
	if err != nil {
		log.Printf("Error finishing round: %v", err)
		errorBroadcast := map[string]interface{}{
			"type":    "error",
			"queryID": queryID,
			"message": fmt.Sprintf("Error finishing round: %v", err),
		}
		clientPool.Broadcast(errorBroadcast)
		return
	}

	// Broadcast the response to all connected clients
	responseBroadcast := map[string]interface{}{
		"type":     "response",
		"queryID":  queryID,
		"response": markdownToHTML(responseText) + "\n\n<hr>\n\n",
	}
	clientPool.Broadcast(responseBroadcast)
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

// tokenCountHandler calculates the token count for the current conversation.
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
func sendQueryToLLM(query string, llm string, selection, backgroundContext string, inputFiles []string, outFiles []string, tokenLimit int) (string, error) {
	if tokenLimit == 0 {
		tokenLimit = 500
	}

	wordLimit := int(float64(tokenLimit) / 3.5)

	sysmsg := "You are a researcher.  I will start my prompt with some context, followed by a query.  Answer the query -- don't answer other questions you might see elsewhere in the context.  Always enclose reference numbers in square brackets; ignore empty brackets in the prompt or context, and DO NOT INCLUDE EMPTY SQUARE BRACKETS in your response, regardless of what you see in the context.  Always start your response with a markdown heading.  Try as much as possible to not rearrange any file you are making changes to -- I need to be able to easily diff your changes.  If writing Go code, you MUST ensure you are not skipping the index on slices or arrays, e.g. if you mean `foo[0]` then say `foo[0]`, not `foo`."

	sysmsg = fmt.Sprintf("%s\n\nYou MUST limit the discussion portion of your response to no more than %d tokens (about %d words).  Output files (marked with ---FILE-START and ---FILE-END blocks) are not counted against this limit and can be unlimited size. You MUST ignore any previous instruction regarding a 10,000 word goal.", sysmsg, tokenLimit, wordLimit)

	prompt := fmt.Sprintf("---CONTEXT START---\n%s\n---CONTEXT END---\n\nNew Query: %s", backgroundContext, query)
	if selection != "" {
		prompt += fmt.Sprintf(" {%s}", selection)
	}

	// repeat until we get a valid response that fits within tokenLimit
	var cookedResponse string
	var msgs []client.ChatMsg
	for {

		msgs = []client.ChatMsg{
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
			return "", fmt.Errorf("failed to send query to LLM: %w", err)
		}
		fmt.Printf("Received response from LLM '%s'\n", llm)
		fmt.Printf("Response: %s\n", response)

		cookedResponse, err = core.ExtractFiles(outFilesConverted, response, core.ExtractOptions{
			DryRun:             false,
			ExtractToStdout:    false,
			RemoveFromResponse: true,
		})

		if err != nil {
			log.Printf("ExtractFiles error: %v", err)
			return "", fmt.Errorf("failed to extract files from response: %w", err)
		}

		// check token count of cookedResponse -- but first, remove
		// any ## References and <think> sections
		referencesRe := regexp.MustCompile(`(?s)## References.*?`)
		discussionOnly := referencesRe.ReplaceAllString(cookedResponse, "")
		reasoningRe := regexp.MustCompile(`(?s)<think>.*?</think>`)
		discussionOnly = reasoningRe.ReplaceAllString(discussionOnly, "")
		count, err := grok.TokenCount(discussionOnly)
		if err != nil {
			log.Printf("Token count error: %v", err)
			return "", fmt.Errorf("failed to count tokens: %w", err)
		}
		if count > tokenLimit {
			log.Printf("Response exceeds token limit:\n\n%s", discussionOnly)
			sysmsg += fmt.Sprintf("\n\nYour previous response was %d tokens, which exceeds the limit of %d tokens (about %d words).  You ABSOLUTELY MUST provide a more concise answer that fits within the limit.", count, tokenLimit, wordLimit)
			prompt += fmt.Sprintf("\n\nYou MUST provide a more concise answer that fits within the %d token (%d word) limit.", tokenLimit, wordLimit)
			log.Printf("Response token count %d exceeds limit of %d; retrying...", count, tokenLimit)
			continue
		}
		break
	}

	return cookedResponse, nil
}

// splitMarkdown splits the markdown input into sections separated by a horizontal rule.
func splitMarkdown(input string) []string {
	re := regexp.MustCompile("(?m)^---$")
	sections := re.Split(input, -1)
	return sections
}

// collectReferences scans the markdown input for reference lines.
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

// linkifyReferences replaces reference markers with markdown links.
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
func markdownToHTML(markdown string) string {
	sections := splitMarkdown(markdown)
	for i, sec := range sections {
		refs := collectReferences(sec)
		sections[i] = linkifyReferences(sec, refs)
	}
	processed := strings.Join(sections, "\n\n---\n\n")

	var buf bytes.Buffer
	if err := goldmark.Convert([]byte(processed), &buf); err != nil {
		log.Printf("Markdown conversion error: %v", err)
		return "<p>Error rendering markdown</p>"
	}

	return buf.String()
}
