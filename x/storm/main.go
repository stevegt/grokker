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
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humachi"
	"github.com/go-chi/chi/v5"
	"github.com/gofrs/flock"
	"github.com/gorilla/websocket"
	. "github.com/stevegt/goadapt"
	"github.com/stevegt/grokker/v3/client"
	"github.com/stevegt/grokker/v3/core"
	"github.com/stevegt/grokker/v3/util"
	"github.com/stevegt/grokker/x/storm/db"
	"github.com/stevegt/grokker/x/storm/split"
	"github.com/stevegt/grokker/x/storm/version"
	"github.com/yuin/goldmark"
)

//go:embed project.html
var projectHTML string

//go:embed index.html
var indexHTML string

var projectTemplate = template.Must(template.New("project").Parse(projectHTML))
var landingTemplate = template.Must(template.New("landing").Parse(indexHTML))

// PendingQuery tracks queries awaiting user approval for unexpected files
type PendingQuery struct {
	queryID                 string
	rawResponse             string
	outFiles                []core.FileLang
	approvalChannel         chan []string
	alreadyAuthorized       []string
	needsAuthorization      []string
	project                 *Project
	notificationTicker      *time.Ticker
	stopNotificationChannel chan bool
}

// Global variables for serve subcommand
var (
	grok     *core.Grokker
	srv      *http.Server
	projects *Projects
	dbMgr    *db.Manager

	upgrader = websocket.Upgrader{
		ReadBufferSize:  1024,
		WriteBufferSize: 1024,
		CheckOrigin: func(r *http.Request) bool {
			return true // Allow all origins for now
		},
	}

	// Track cancelled queries by queryID
	cancelledQueries = make(map[string]bool)
	cancelledMutex   sync.Mutex

	// Track pending queries by queryID
	pendingApprovals = make(map[string]*PendingQuery)
	pendingMutex     sync.Mutex
)

const (
	pingInterval                  = 20 * time.Second
	pongWait                      = 60 * time.Second
	unexpectedFilesNotifyInterval = 10 * time.Second
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
	ProjectID  string   `json:"projectID"`
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

// Project encapsulates project-specific data and state.
type Project struct {
	ID              string
	BaseDir         string
	MarkdownFile    string
	AuthorizedFiles []string
	Chat            *Chat
	ClientPool      *ClientPool
}

// WebSocket client connection.
type WSClient struct {
	conn      *websocket.Conn
	send      chan interface{}
	pool      *ClientPool
	id        string
	projectID string
}

// ClientPool manages all connected WebSocket clients for a project.
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
	return 8192 // default
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

// rootHandler serves the landing page listing all projects
func rootHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")

	projectIDs := projects.List()
	var projectInfos []struct {
		ID      string
		BaseDir string
	}

	for _, projectID := range projectIDs {
		project, err := projects.Get(projectID)
		if err != nil {
			log.Printf("Error loading project %s: %v", projectID, err)
			continue
		}
		projectInfos = append(projectInfos, struct {
			ID      string
			BaseDir string
		}{
			ID:      project.ID,
			BaseDir: project.BaseDir,
		})
	}

	if err := landingTemplate.Execute(w, projectInfos); err != nil {
		http.Error(w, "Template error", http.StatusInternalServerError)
	}
}

// serveRun starts the HTTP server on the specified port with the given database path
func serveRun(port int, dbPath string) error {
	var err error
	var lock *flock.Flock
	grok, _, _, _, lock, err = core.Load("", true)
	if err != nil {
		return fmt.Errorf("failed to load LLM core: %w", err)
	}
	defer lock.Unlock()

	// Use provided dbPath or default
	if dbPath == "" {
		dbPath = filepath.Join(os.ExpandEnv("$HOME"), ".storm", "data.db")
	}
	dbDir := filepath.Dir(dbPath)
	if err := os.MkdirAll(dbDir, 0700); err != nil {
		return fmt.Errorf("failed to create database directory: %w", err)
	}

	dbMgr, err = db.NewManager(dbPath)
	if err != nil {
		return fmt.Errorf("failed to initialize database: %w", err)
	}
	defer dbMgr.Close()

	// Initialize projects registry with database backend (no eager loading)
	projects = NewProjectsWithDB(dbMgr)

	// Create chi router
	chiRouter := chi.NewRouter()

	// Create Huma API with version from build-time injection
	config := huma.DefaultConfig("Storm API", version.Version)
	config.DocsPath = "/docs"
	api := humachi.New(chiRouter, config)

	// Root handler for project list or landing page
	chiRouter.HandleFunc("/", rootHandler)

	// Huma API endpoints for project management
	huma.Post(api, "/api/projects", postProjectsHandler)
	huma.Get(api, "/api/projects", getProjectsHandler)
	huma.Delete(api, "/api/projects/{projectID}", deleteProjectHandler)
	huma.Post(api, "/api/projects/{projectID}/files/add", postProjectFilesAddHandler)
	huma.Post(api, "/api/projects/{projectID}/files/forget", postProjectFilesForgetHandler)
	huma.Get(api, "/api/projects/{projectID}/files", getProjectFilesHandler)

	// Project-specific routes (non-Huma for now, using chi directly)
	projectRouter := chiRouter.Route("/project/{projectID}", func(r chi.Router) {
		r.HandleFunc("/", projectHandlerFunc)
		r.HandleFunc("/ws", wsHandlerFunc)
		r.HandleFunc("/tokencount", tokenCountHandlerFunc)
		r.HandleFunc("/rounds", roundsHandlerFunc)
		r.HandleFunc("/open", openHandlerFunc)
	})

	_ = projectRouter

	// Global routes
	chiRouter.HandleFunc("/stop", stopHandler)

	addr := fmt.Sprintf(":%d", port)
	srv = &http.Server{Addr: addr, Handler: chiRouter}
	log.Printf("Starting server on %s\n", addr)
	log.Printf("API documentation available at http://localhost%s/docs\n", addr)
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		return err
	}
	return nil
}

// projectHandlerFunc is a wrapper to extract project and call handler
func projectHandlerFunc(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "projectID")

	project, err := projects.Get(projectID)
	if err != nil {
		http.Error(w, fmt.Sprintf("Project %s not found: %v", projectID, err), http.StatusNotFound)
		return
	}

	projectHandler(w, r, project)
}

// projectHandler handles the main chat page for a project
func projectHandler(w http.ResponseWriter, r *http.Request, project *Project) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	chatContent := project.Chat.getHistory(true)
	data := struct {
		ChatHTML template.HTML
	}{
		ChatHTML: template.HTML(markdownToHTML(chatContent)),
	}
	if err := projectTemplate.Execute(w, data); err != nil {
		http.Error(w, "Template error", http.StatusInternalServerError)
	}
}

// wsHandlerFunc is a wrapper to extract project and call handler
func wsHandlerFunc(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "projectID")

	project, err := projects.Get(projectID)
	if err != nil {
		http.Error(w, fmt.Sprintf("Project %s not found: %v", projectID, err), http.StatusNotFound)
		return
	}

	wsHandler(w, r, project)
}

// wsHandler handles WebSocket connections for a project
func wsHandler(w http.ResponseWriter, r *http.Request, project *Project) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("WebSocket upgrade error: %v", err)
		return
	}

	client := &WSClient{
		conn:      conn,
		send:      make(chan interface{}, 256),
		pool:      project.ClientPool,
		id:        fmt.Sprintf("client-%d", len(project.ClientPool.clients)),
		projectID: project.ID,
	}

	// Set up ping/pong handlers for keepalive
	conn.SetReadDeadline(time.Now().Add(pongWait))
	conn.SetPongHandler(func(string) error {
		conn.SetReadDeadline(time.Now().Add(pongWait))
		return nil
	})

	project.ClientPool.register <- client

	go client.writePump()
	go client.readPump(project)
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

// resolveFilePath converts a relative path to absolute using the project's BaseDir
func resolveFilePath(project *Project, filePath string) string {
	if filepath.IsAbs(filePath) {
		// Already absolute, return as-is
		return filePath
	}
	// Relative path: resolve against project BaseDir
	return filepath.Join(project.BaseDir, filePath)
}

// readPump reads messages from the WebSocket client and processes queries.
func (c *WSClient) readPump(project *Project) {
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

		// Handle incoming messages from clients
		if msgType, ok := msg["type"].(string); ok {
			if msgType == "query" {
				log.Printf("Received query from %s in project %s: %v", c.id, c.projectID, msg)

				// Extract query parameters
				query, _ := msg["query"].(string)
				llm, _ := msg["llm"].(string)
				selection, _ := msg["selection"].(string)
				queryID, _ := msg["queryID"].(string)

				// Extract arrays and resolve relative paths to absolute
				var inputFiles, outFiles []string
				if inputFilesRaw, ok := msg["inputFiles"].([]interface{}); ok {
					for i := 0; i < len(inputFilesRaw); i++ {
						if s, ok := inputFilesRaw[i].(string); ok {
							absPath := resolveFilePath(project, s)
							inputFiles = append(inputFiles, absPath)
						}
					}
				}
				if outFilesRaw, ok := msg["outFiles"].([]interface{}); ok {
					for i := 0; i < len(outFilesRaw); i++ {
						if s, ok := outFilesRaw[i].(string); ok {
							absPath := resolveFilePath(project, s)
							outFiles = append(outFiles, absPath)
						}
					}
				}

				// Extract and parse tokenLimit with shorthand support (1K, 2M, etc.)
				tokenLimit := parseTokenLimit(msg["tokenLimit"])

				// Process the query
				go processQuery(project, queryID, query, llm, selection, inputFiles, outFiles, tokenLimit)
			} else if msgType == "cancel" {
				// Handle query cancellation
				queryID, _ := msg["queryID"].(string)
				cancelledMutex.Lock()
				cancelledQueries[queryID] = true
				cancelledMutex.Unlock()
				log.Printf("Query %s marked for cancellation", queryID)

				// Stop notification ticker for this query if it exists
				pendingMutex.Lock()
				if pending, exists := pendingApprovals[queryID]; exists {
					if pending.notificationTicker != nil {
						pending.notificationTicker.Stop()
						pending.notificationTicker = nil
					}
					select {
					case pending.stopNotificationChannel <- true:
					default:
					}
				}
				pendingMutex.Unlock()
			} else if msgType == "approveFiles" {
				// Handle file approval for unexpected files
				queryID, _ := msg["queryID"].(string)
				approvedFilesRaw, _ := msg["approvedFiles"].([]interface{})

				// Convert approved files to string slice
				var approvedFiles []string
				for i := 0; i < len(approvedFilesRaw); i++ {
					if f, ok := approvedFilesRaw[i].(string); ok {
						approvedFiles = append(approvedFiles, f)
					}
				}

				// Send approval to pending query over channel to unblock processing
				pendingMutex.Lock()
				pending, exists := pendingApprovals[queryID]
				pendingMutex.Unlock()

				if exists && pending != nil {
					log.Printf("Sending approval for query %s with %d approved files", queryID, len(approvedFiles))

					// Stop the notification ticker
					if pending.notificationTicker != nil {
						pending.notificationTicker.Stop()
						pending.notificationTicker = nil
					}

					select {
					case pending.approvalChannel <- approvedFiles:
						log.Printf("Approval sent to query %s", queryID)
					default:
						log.Printf("WARNING: approval channel full for query %s", queryID)
					}
				} else {
					log.Printf("WARNING: received approval for unknown query %s", queryID)
				}
			}
		}
	}
}

// addPendingQuery registers a query waiting for user approval
func addPendingQuery(queryID string, rawResponse string, outFiles []core.FileLang, alreadyAuthorized, needsAuthorization []string, project *Project) *PendingQuery {
	pending := &PendingQuery{
		queryID:                 queryID,
		rawResponse:             rawResponse,
		outFiles:                outFiles,
		approvalChannel:         make(chan []string, 1),
		alreadyAuthorized:       alreadyAuthorized,
		needsAuthorization:      needsAuthorization,
		project:                 project,
		stopNotificationChannel: make(chan bool, 1),
	}

	pendingMutex.Lock()
	pendingApprovals[queryID] = pending
	pendingMutex.Unlock()

	log.Printf("Added pending query %s, waiting for approval", queryID)
	return pending
}

// removePendingQuery cleans up a query that no longer needs approval
func removePendingQuery(queryID string) {
	pendingMutex.Lock()
	pending, exists := pendingApprovals[queryID]
	if exists {
		delete(pendingApprovals, queryID)
		// Stop the notification ticker if it exists
		if pending.notificationTicker != nil {
			pending.notificationTicker.Stop()
			pending.notificationTicker = nil
		}
		// Close the channel to signal completion
		close(pending.approvalChannel)
	}
	pendingMutex.Unlock()

	if exists {
		log.Printf("Removed pending query %s", queryID)
	}
}

// startNotificationTicker begins periodically re-sending the unexpected files notification every 10 seconds[1]
func startNotificationTicker(pending *PendingQuery) {
	pending.notificationTicker = time.NewTicker(unexpectedFilesNotifyInterval)

	go func() {
		for {
			select {
			case <-pending.notificationTicker.C:
				// Re-send the unexpected files notification
				unexpectedFilesMsg := map[string]interface{}{
					"type":               "unexpectedFilesDetected",
					"queryID":            pending.queryID,
					"alreadyAuthorized":  pending.alreadyAuthorized,
					"needsAuthorization": pending.needsAuthorization,
					"projectID":          pending.project.ID,
				}
				pending.project.ClientPool.Broadcast(unexpectedFilesMsg)
				log.Printf("Re-broadcasted unexpected files notification for query %s", pending.queryID)

			case <-pending.stopNotificationChannel:
				// Stop sending notifications when approval is received or query is cancelled
				if pending.notificationTicker != nil {
					pending.notificationTicker.Stop()
				}
				log.Printf("Stopped notification ticker for query %s", pending.queryID)
				return
			}
		}
	}()
}

// waitForApproval blocks indefinitely until user approves files
func waitForApproval(pending *PendingQuery) ([]string, error) {
	approvedFiles := <-pending.approvalChannel
	log.Printf("Received approval for query %s with %d files", pending.queryID, len(approvedFiles))
	return approvedFiles, nil
}

// categorizeUnexpectedFiles separates unexpected files into authorized and needs-authorization categories[1]
func categorizeUnexpectedFiles(project *Project, unexpectedFileNames []string) ([]string, []string) {
	var alreadyAuthorized []string
	var needsAuthorization []string

	// Create a set of authorized files for efficient lookup
	authorizedSet := make(map[string]bool)
	for i := 0; i < len(project.AuthorizedFiles); i++ {
		authorizedSet[project.AuthorizedFiles[i]] = true
	}

	// Categorize each unexpected file
	for i := 0; i < len(unexpectedFileNames); i++ {
		file := unexpectedFileNames[i]
		if authorizedSet[file] {
			alreadyAuthorized = append(alreadyAuthorized, file)
		} else {
			needsAuthorization = append(needsAuthorization, file)
		}
	}

	return alreadyAuthorized, needsAuthorization
}

// processQuery processes a query and broadcasts results to all clients in the project.
func processQuery(project *Project, queryID, query, llm, selection string, inputFiles, outFiles []string, tokenLimit int) {
	// Clean up cancellation flag when done
	defer func() {
		cancelledMutex.Lock()
		delete(cancelledQueries, queryID)
		cancelledMutex.Unlock()
	}()

	// Broadcast the query to all clients in this project
	queryBroadcast := map[string]interface{}{
		"type":      "query",
		"query":     query,
		"queryID":   queryID,
		"projectID": project.ID,
	}
	project.ClientPool.Broadcast(queryBroadcast)

	round := project.Chat.StartRound(query, selection)

	history := project.Chat.getHistory(true)
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
	responseText, err := sendQueryToLLM(project, queryID, query, llm, selection, lastN, inputFiles, outFiles, tokenLimit)
	if err != nil {
		log.Printf("Error processing query: %v", err)
		// Broadcast error to all connected clients
		errorBroadcast := map[string]interface{}{
			"type":      "error",
			"queryID":   queryID,
			"message":   fmt.Sprintf("Error processing query: %v", err),
			"projectID": project.ID,
		}
		project.ClientPool.Broadcast(errorBroadcast)
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

	err = project.Chat.FinishRound(round, responseText)
	if err != nil {
		log.Printf("Error finishing round: %v", err)
		errorBroadcast := map[string]interface{}{
			"type":      "error",
			"queryID":   queryID,
			"message":   fmt.Sprintf("Error finishing round: %v", err),
			"projectID": project.ID,
		}
		project.ClientPool.Broadcast(errorBroadcast)
		return
	}

	// Broadcast the response to all connected clients in this project
	responseBroadcast := map[string]interface{}{
		"type":      "response",
		"queryID":   queryID,
		"response":  markdownToHTML(responseText) + "\n\n<hr>\n\n",
		"projectID": project.ID,
	}
	project.ClientPool.Broadcast(responseBroadcast)
}

// openHandlerFunc is a wrapper to extract project and call handler
func openHandlerFunc(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "projectID")

	project, err := projects.Get(projectID)
	if err != nil {
		http.Error(w, fmt.Sprintf("Project %s not found: %v", projectID, err), http.StatusNotFound)
		return
	}

	openHandler(w, r, project)
}

// openHandler serves a file based on the filename query parameter.
func openHandler(w http.ResponseWriter, r *http.Request, project *Project) {
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

// roundsHandlerFunc is a wrapper to extract project and call handler
func roundsHandlerFunc(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "projectID")

	project, err := projects.Get(projectID)
	if err != nil {
		http.Error(w, fmt.Sprintf("Project %s not found: %v", projectID, err), http.StatusNotFound)
		return
	}

	roundsHandler(w, r, project)
}

// roundsHandler returns the total number of chat rounds as JSON.
func roundsHandler(w http.ResponseWriter, r *http.Request, project *Project) {
	w.Header().Set("Content-Type", "application/json")
	rounds := 0
	if project.Chat != nil {
		rounds = project.Chat.TotalRounds()
	}
	json.NewEncoder(w).Encode(map[string]int{"rounds": rounds})
}

// tokenCountHandlerFunc is a wrapper to extract project and call handler
func tokenCountHandlerFunc(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "projectID")

	project, err := projects.Get(projectID)
	if err != nil {
		http.Error(w, fmt.Sprintf("Project %s not found: %v", projectID, err), http.StatusNotFound)
		return
	}

	tokenCountHandler(w, r, project)
}

// tokenCountHandler calculates the token count for the current conversation.
func tokenCountHandler(w http.ResponseWriter, r *http.Request, project *Project) {
	w.Header().Set("Content-Type", "application/json")
	if project.Chat == nil {
		json.NewEncoder(w).Encode(map[string]int{"tokens": 0})
		return
	}
	chatText := project.Chat.getHistory(true)
	count, err := grok.TokenCount(chatText)
	if err != nil {
		log.Printf("Token count error: %v", err)
		count = 0
	}
	json.NewEncoder(w).Encode(map[string]int{"tokens": count})
}

// isQueryCancelled checks if a query has been marked for cancellation
func isQueryCancelled(queryID string) bool {
	cancelledMutex.Lock()
	defer cancelledMutex.Unlock()
	return cancelledQueries[queryID]
}

// sendQueryToLLM calls the Grokker API to obtain a markdown-formatted text.
// Checks if the query was cancelled after the LLM call completes and discards the result if so.
// Implements Stage 5: Dry-run detection and WebSocket notification of unexpected files
func sendQueryToLLM(project *Project, queryID, query string, llm string, selection, backgroundContext string, inputFiles []string, outFiles []string, tokenLimit int) (string, error) {
	if tokenLimit == 0 {
		tokenLimit = 8192
	}

	wordLimit := int(float64(tokenLimit) / 3.5)

	sysmsg := "You are a researcher.  I will start my prompt with some context, followed by a query.  Answer the query -- don't answer other questions you might see elsewhere in the context.  Always enclose reference numbers in square brackets; ignore empty brackets in the prompt or context, and DO NOT INCLUDE EMPTY SQUARE BRACKETS in your response, regardless of what you see in the context.  Always start your response with a markdown heading.  Try as much as possible to not rearrange any file you are making changes to -- I need to be able to easily diff your changes.  If writing Go code, you MUST ensure you are not skipping the index on slices or arrays, e.g. if you mean `foo[0]` then say `foo[0]`, not `foo`."

	sysmsg = fmt.Sprintf("%s\n\nYou MUST limit the discussion portion of your response to no more than %d tokens (about %d words).  Output files (marked with ---FILE-START and ---FILE-END blocks) are not counted against this limit and can be unlimited size. You MUST ignore any previous instruction regarding a 10,000 word goal.", sysmsg, tokenLimit, wordLimit)

	prompt := fmt.Sprintf("---CONTEXT START---\n%s\n---CONTEXT END---\n\nNew Query: %s", backgroundContext, query)
	if selection != "" {
		prompt += fmt.Sprintf(" {%s}", selection)
	}

	// repeat until we get a valid response that fits within tokenLimit
	// but increase tokenLimit each time as well, up to 5 tries
	var cookedResponse string
	var msgs []client.ChatMsg
	for i := 0; i < 5; i++ {

		msgs = []client.ChatMsg{
			{Role: "USER", Content: prompt},
		}

		var outFilesConverted []core.FileLang
		for j := 0; j < len(outFiles); j++ {
			f := outFiles[j]
			lang, known, err := util.Ext2Lang(f)
			if err != nil {
				log.Printf("Ext2Lang error for file %s: %v", f, err)
				lang = "text"
			}
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

		// Check if query was cancelled after LLM call completes
		if isQueryCancelled(queryID) {
			log.Printf("Query %s was cancelled, discarding LLM result", queryID)
			return "", fmt.Errorf("query cancelled")
		}

		fmt.Printf("Received response from LLM '%s'\n", llm)
		fmt.Printf("Response: %s\n", response)

		// run ExtractFiles first as a dry run to see if we fit in token limit
		result, err := core.ExtractFiles(outFilesConverted, response, core.ExtractOptions{
			DryRun:          true,
			ExtractToStdout: false,
		})

		if err != nil {
			log.Printf("ExtractFiles error: %v", err)
			return "", fmt.Errorf("failed to extract files from response: %w", err)
		}

		cookedResponse = result.CookedResponse

		// Stage 5: Detect unexpected files and notify user via WebSocket
		if len(result.UnexpectedFiles) > 0 {
			// Extract filenames from UnexpectedFiles (which are FileEntry structs with Name and Content)
			var unexpectedFileNames []string
			for k := 0; k < len(result.UnexpectedFiles); k++ {
				// Extract the filename from the FileEntry struct
				unexpectedFileNames = append(unexpectedFileNames, result.UnexpectedFiles[k].Filename)
			}

			// Categorize unexpected files
			alreadyAuthorized, needsAuthorization := categorizeUnexpectedFiles(project, unexpectedFileNames)

			log.Printf("Found %d unexpected files: %d authorized, %d need authorization", len(unexpectedFileNames), len(alreadyAuthorized), len(needsAuthorization))

			// Send WebSocket notification only if there are actually unexpected files
			if len(alreadyAuthorized) > 0 || len(needsAuthorization) > 0 {
				unexpectedFilesMsg := map[string]interface{}{
					"type":               "unexpectedFilesDetected",
					"queryID":            queryID,
					"alreadyAuthorized":  alreadyAuthorized,
					"needsAuthorization": needsAuthorization,
					"projectID":          project.ID,
				}
				project.ClientPool.Broadcast(unexpectedFilesMsg)
				log.Printf("Broadcasted unexpected files notification for query %s", queryID)

				// Create pending query and wait for user approval[2]
				pending := addPendingQuery(queryID, response, outFilesConverted, alreadyAuthorized, needsAuthorization, project)

				// Start periodic re-sending of unexpected files notification[1]
				startNotificationTicker(pending)

				// Wait indefinitely for user to approve or decline files
				approvedFiles, err := waitForApproval(pending)
				if err != nil {
					log.Printf("Error waiting for approval: %v", err)
					// Continue with original extraction if approval fails
					approvedFiles = []string{}
				}

				// If user approved files, expand outFiles list and re-run extraction
				if len(approvedFiles) > 0 {
					log.Printf("User approved %d files, re-running extraction with expanded list", len(approvedFiles))

					// Convert approved filenames to FileLang and add to outFilesConverted
					for k := 0; k < len(approvedFiles); k++ {
						approvedFile := approvedFiles[k]
						lang, known, err := util.Ext2Lang(approvedFile)
						if err != nil {
							log.Printf("Ext2Lang error for approved file %s: %v", approvedFile, err)
							lang = "text"
						}
						if !known {
							log.Printf("Unknown file extension for approved file %s; assuming language is %s", approvedFile, lang)
						}
						outFilesConverted = append(outFilesConverted, core.FileLang{File: approvedFile, Language: lang})
					}
				}

				// Clean up pending query
				removePendingQuery(queryID)
			}
		}

		// check token count of cookedResponse -- but first, remove
		// any ## References, <references>, and <think> sections
		discussionOnly := cookedResponse
		refHeadRe := regexp.MustCompile(`(?s)## References.*?`)
		discussionOnly = refHeadRe.ReplaceAllString(discussionOnly, "")
		reftagsRe := regexp.MustCompile(`(?s)<references>.*?</references>`)
		discussionOnly = reftagsRe.ReplaceAllString(discussionOnly, "")
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
			tokenLimit *= 2
			continue
		}

		// successful response within token limit, so now run ExtractFiles for real
		result, err = core.ExtractFiles(outFilesConverted, response, core.ExtractOptions{
			DryRun:          false,
			ExtractToStdout: false,
		})

		cookedResponse = result.CookedResponse

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
	return input
	/*
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
	*/
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
