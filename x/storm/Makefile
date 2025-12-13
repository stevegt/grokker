```

So I need to provide:
1. cli.go - with version imported from a package
2. main.go - with version imported from a package
3. Makefile - with build-time version injection

I need to create a version package and update the files to use build-time injection.

The approach:
1. Create version/version.go with a Version variable
2. Update cli.go to use the version constant
3. Update main.go to use the version constant
4. Create a Makefile that extracts git info and builds with -ldflags

Let me implement this.
</think>

# Build-Time Version Extraction from Git Commit Hash and Tags

Implements build-time version extraction using git commit hash and tags, injecting the version via linker flags during the build process[1][2]. The version is extracted at build time and embedded into the binary without runtime dependencies.

---FILE-START filename="/home/stevegt/lab/grokker/x/storm/version/version.go"---
package version

// Version is the version string for Storm, injected at build time via -ldflags[1]
// Format: "tag-hash" if tag exists, otherwise "dev-hash"
// Example: "v0.1.5-abc1234" or "dev-abc1234"
var Version = "dev-unknown"
---FILE-END filename="/home/stevegt/lab/grokker/x/storm/version/version.go"---

---FILE-START filename="/home/stevegt/lab/grokker/x/storm/cli.go"---
package main

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
	"github.com/stevegt/grokker/x/storm/version"
)

// CLI Helper Functions

// getDaemonURL retrieves the daemon URL from environment or returns default
func getDaemonURL() string {
	daemonURL := os.Getenv("STORM_DAEMON_URL")
	if daemonURL == "" {
		daemonURL = "http://localhost:8080"
	}
	return daemonURL
}

// makeRequest makes an HTTP request with consistent error handling
func makeRequest(method, endpoint string, payload interface{}) (*http.Response, error) {
	daemonURL := getDaemonURL()
	url := daemonURL + endpoint

	var req *http.Request
	var err error

	if payload != nil {
		jsonData, err := json.Marshal(payload)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal request: %w", err)
		}
		req, err = http.NewRequest(method, url, bytes.NewReader(jsonData))
	} else {
		req, err = http.NewRequest(method, url, nil)
	}

	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to daemon at %s: %w", daemonURL, err)
	}

	return resp, nil
}

// decodeJSON decodes a JSON response with error handling
func decodeJSON(resp *http.Response, v interface{}) error {
	return json.NewDecoder(resp.Body).Decode(v)
}

// validateRequiredFlag validates that a required flag has been set
func validateRequiredFlag(flagValue, flagName string) error {
	if flagValue == "" {
		return fmt.Errorf("--%s flag is required", flagName)
	}
	return nil
}

// checkStatusCode validates HTTP response status code
func checkStatusCode(resp *http.Response, acceptedCodes ...int) error {
	for i := 0; i < len(acceptedCodes); i++ {
		code := acceptedCodes[i]
		if resp.StatusCode == code {
			return nil
		}
	}
	body, _ := ioutil.ReadAll(resp.Body)
	return fmt.Errorf("daemon returned status %d: %s", resp.StatusCode, string(body))
}

// resolvePath converts relative paths to absolute paths; returns absolute paths unchanged[1]
func resolvePath(path string) (string, error) {
	if filepath.IsAbs(path) {
		// Already absolute, return as-is
		return path, nil
	}
	// Relative path: resolve against current working directory
	cwd, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("failed to get current working directory: %w", err)
	}
	return filepath.Join(cwd, path), nil
}

// CLI Command Handlers

// runServe implements the serve command
func runServe(cmd *cobra.Command, args []string) error {
	port, err := cmd.Flags().GetInt("port")
	if err != nil {
		return err
	}
	dbPath, err := cmd.Flags().GetString("db-path")
	if err != nil {
		return err
	}
	return serveRun(port, dbPath)
}

// runStop implements the stop command
func runStop(cmd *cobra.Command, args []string) error {
	resp, err := makeRequest("POST", "/stop", nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if err := checkStatusCode(resp, http.StatusOK); err != nil {
		return err
	}

	fmt.Println("Daemon stopped successfully")
	return nil
}

// runProjectAdd implements the project add command
func runProjectAdd(cmd *cobra.Command, args []string) error {
	projectID := args
	baseDir := args[1]
	markdownFile := args[2]

	// Resolve paths to absolute (relative paths resolved against cwd at add-time)
	resolvedBaseDir, err := resolvePath(baseDir)
	if err != nil {
		return err
	}
	resolvedMarkdownFile, err := resolvePath(markdownFile)
	if err != nil {
		return err
	}

	payload := map[string]string{
		"projectID":    projectID,
		"baseDir":      resolvedBaseDir,
		"markdownFile": resolvedMarkdownFile,
	}

	resp, err := makeRequest("POST", "/api/projects", payload)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if err := checkStatusCode(resp, http.StatusOK, http.StatusCreated); err != nil {
		return err
	}

	var result map[string]interface{}
	if err := decodeJSON(resp, &result); err != nil {
		return fmt.Errorf("failed to decode response: %w", err)
	}

	fmt.Printf("Project %s added successfully\n", projectID)
	fmt.Printf("  BaseDir: %s\n", resolvedBaseDir)
	fmt.Printf("  MarkdownFile: %s\n", resolvedMarkdownFile)
	if rounds, ok := result["chatRounds"].(float64); ok {
		fmt.Printf("  Chat rounds loaded: %d\n", int(rounds))
	}
	return nil
}

// runProjectList implements the project list command
func runProjectList(cmd *cobra.Command, args []string) error {
	resp, err := makeRequest("GET", "/api/projects", nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if err := checkStatusCode(resp, http.StatusOK, http.StatusNoContent); err != nil {
		return err
	}

	if resp.StatusCode == http.StatusNoContent {
		fmt.Println("No projects registered")
		return nil
	}

	var projectList ProjectList
	if err := decodeJSON(resp, &projectList); err != nil {
		return fmt.Errorf("failed to decode response: %w", err)
	}

	if len(projectList.Projects) == 0 {
		fmt.Println("No projects registered")
		return nil
	}

	fmt.Println("Registered projects:")
	for _, proj := range projectList.Projects {
		fmt.Printf("  - %s (baseDir: %s)\n", proj.ID, proj.BaseDir)
	}
	return nil
}

// runProjectForget implements the project forget command
func runProjectForget(cmd *cobra.Command, args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("project ID is required")
	}
	projectID := args

	endpoint := fmt.Sprintf("/api/projects/%s", projectID)
	resp, err := makeRequest("DELETE", endpoint, nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if err := checkStatusCode(resp, http.StatusOK); err != nil {
		return err
	}

	var result map[string]interface{}
	if err := decodeJSON(resp, &result); err != nil {
		return fmt.Errorf("failed to decode response: %w", err)
	}

	fmt.Printf("Project %s forgotten and removed from database\n", projectID)
	return nil
}

// runFileAdd implements the file add command
func runFileAdd(cmd *cobra.Command, args []string) error {
	projectID, err := cmd.Flags().GetString("project")
	if err != nil {
		return err
	}
	if err := validateRequiredFlag(projectID, "project"); err != nil {
		return err
	}

	// Resolve relative paths to absolute
	var resolvedFilenames []string
	for i := 0; i < len(args); i++ {
		resolved, err := resolvePath(args[i])
		if err != nil {
			return err
		}
		resolvedFilenames = append(resolvedFilenames, resolved)
	}

	payload := map[string]interface{}{
		"filenames": resolvedFilenames,
	}

	endpoint := fmt.Sprintf("/api/projects/%s/files/add", projectID)
	resp, err := makeRequest("POST", endpoint, payload)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if err := checkStatusCode(resp, http.StatusOK, http.StatusCreated); err != nil {
		return err
	}

	var result map[string]interface{}
	if err := decodeJSON(resp, &result); err != nil {
		return fmt.Errorf("failed to decode response: %w", err)
	}

	fmt.Printf("Files added to project %s:\n", projectID)
	if added, ok := result["added"].([]interface{}); ok {
		for i := 0; i < len(added); i++ {
			f := added[i]
			fmt.Printf("  + %s\n", f)
		}
	}
	if failed, ok := result["failed"].([]interface{}); ok {
		if len(failed) > 0 {
			fmt.Printf("Failed to add:\n")
			for i := 0; i < len(failed); i++ {
				f := failed[i]
				fmt.Printf("  - %s\n", f)
			}
		}
	}
	return nil
}

// runFileList implements the file list command
func runFileList(cmd *cobra.Command, args []string) error {
	projectID, err := cmd.Flags().GetString("project")
	if err != nil {
		return err
	}
	if err := validateRequiredFlag(projectID, "project"); err != nil {
		return err
	}

	endpoint := fmt.Sprintf("/api/projects/%s/files", projectID)
	resp, err := makeRequest("GET", endpoint, nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if err := checkStatusCode(resp, http.StatusOK, http.StatusNoContent); err != nil {
		return err
	}

	if resp.StatusCode == http.StatusNoContent {
		fmt.Printf("No files authorized for project %s\n", projectID)
		return nil
	}

	var result map[string]interface{}
	if err := decodeJSON(resp, &result); err != nil {
		return fmt.Errorf("failed to decode response: %w", err)
	}

	fmt.Printf("Authorized files for project %s:\n", projectID)
	if files, ok := result["files"].([]interface{}); ok {
		if len(files) == 0 {
			fmt.Println("  (no files)")
		} else {
			for i := 0; i < len(files); i++ {
				f := files[i]
				fmt.Printf("  - %s\n", f)
			}
		}
	}
	return nil
}

// runFileForget implements the file forget command - accepts multiple files
func runFileForget(cmd *cobra.Command, args []string) error {
	projectID, err := cmd.Flags().GetString("project")
	if err != nil {
		return err
	}
	if err := validateRequiredFlag(projectID, "project"); err != nil {
		return err
	}

	if len(args) < 1 {
		return fmt.Errorf("filename(s) required")
	}

	// Resolve relative paths to absolute
	var resolvedFilenames []string
	for i := 0; i < len(args); i++ {
		resolved, err := resolvePath(args[i])
		if err != nil {
			return err
		}
		resolvedFilenames = append(resolvedFilenames, resolved)
	}

	payload := map[string]interface{}{
		"filenames": resolvedFilenames,
	}

	endpoint := fmt.Sprintf("/api/projects/%s/files/forget", projectID)
	resp, err := makeRequest("POST", endpoint, payload)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if err := checkStatusCode(resp, http.StatusOK); err != nil {
		return err
	}

	var result map[string]interface{}
	if err := decodeJSON(resp, &result); err != nil {
		return fmt.Errorf("failed to decode response: %w", err)
	}

	fmt.Printf("Files forgotten from project %s:\n", projectID)
	if removed, ok := result["removed"].([]interface{}); ok {
		for i := 0; i < len(removed); i++ {
			f := removed[i]
			fmt.Printf("  - %s\n", f)
		}
	}
	if failed, ok := result["failed"].([]interface{}); ok {
		if len(failed) > 0 {
			fmt.Printf("Failed to remove:\n")
			for i := 0; i < len(failed); i++ {
				f := failed[i]
				fmt.Printf("  - %s\n", f)
			}
		}
	}
	return nil
}

// runIssueToken implements the issue-token command
func runIssueToken(cmd *cobra.Command, args []string) error {
	fmt.Println("Would issue CWT token via HTTP API")
	return nil
}

func main() {
	fmt.Printf("storm v%s\n", version.Version)

	rootCmd := &cobra.Command{
		Use:   "storm",
		Short: "Storm - Multi-project LLM chat application",
		Long:  `Storm is a single-daemon, single-port multi-project chat application for interacting with LLMs and local files.`,
	}

	// Serve command
	serveCmd := &cobra.Command{
		Use:   "serve",
		Short: "Start the Storm server",
		Long:  `Start the Storm server on the specified port.`,
		RunE:  runServe,
	}
	serveCmd.Flags().IntP("port", "p", 8080, "port to listen on")
	serveCmd.Flags().StringP("db-path", "d", "", "path to database file (default: ~/.storm/data.db)")
	rootCmd.AddCommand(serveCmd)

	// Stop command
	stopCmd := &cobra.Command{
		Use:   "stop",
		Short: "Stop the Storm daemon",
		Long:  `Stop the running Storm daemon gracefully.`,
		RunE:  runStop,
	}
	rootCmd.AddCommand(stopCmd)

	// Project command
	projectCmd := &cobra.Command{
		Use:   "project",
		Short: "Manage projects",
		Long:  `Manage Storm projects.`,
	}

	projectAddCmd := &cobra.Command{
		Use:   "add [projectID] [baseDir] [markdownFile]",
		Short: "Add a new project",
		Long:  `Add a new project to the registry. Paths can be absolute or relative (resolved to absolute against current working directory).`,
		Args:  cobra.ExactArgs(3),
		RunE:  runProjectAdd,
	}

	projectListCmd := &cobra.Command{
		Use:   "list",
		Short: "List all projects",
		Long:  `List all registered projects via HTTP API.`,
		RunE:  runProjectList,
	}

	projectForgetCmd := &cobra.Command{
		Use:   "forget [projectID]",
		Short: "Delete a project",
		Long:  `Delete a project and all its data from the database via HTTP API.`,
		Args:  cobra.ExactArgs(1),
		RunE:  runProjectForget,
	}

	projectCmd.AddCommand(projectAddCmd, projectListCmd, projectForgetCmd)
	rootCmd.AddCommand(projectCmd)

	// File command
	fileCmd := &cobra.Command{
		Use:   "file",
		Short: "Manage project files",
		Long:  `Manage files associated with projects.`,
	}

	fileAddCmd := &cobra.Command{
		Use:   "add [files...]",
		Short: "Add files to a project",
		Long:  `Add one or more authorized files to a project.`,
		Args:  cobra.MinimumNArgs(1),
		RunE:  runFileAdd,
	}
	fileAddCmd.Flags().StringP("project", "p", "", "Project ID (required)")

	fileListCmd := &cobra.Command{
		Use:   "list",
		Short: "List files in a project",
		Long:  `List all authorized files for a project.`,
		RunE:  runFileList,
	}
	fileListCmd.Flags().StringP("project", "p", "", "Project ID (required)")

	fileForgetCmd := &cobra.Command{
		Use:   "forget [files...]",
		Short: "Remove files from a project",
		Long:  `Remove one or more authorized files from a project.`,
		Args:  cobra.MinimumNArgs(1),
		RunE:  runFileForget,
	}
	fileForgetCmd.Flags().StringP("project", "p", "", "Project ID (required)")

	fileCmd.AddCommand(fileAddCmd, fileListCmd, fileForgetCmd)
	rootCmd.AddCommand(fileCmd)

	// Token command
	tokenCmd := &cobra.Command{
		Use:   "issue-token",
		Short: "Issue a CWT token",
		Long:  `Issue a CBOR Web Token for project access.`,
		RunE:  runIssueToken,
	}
	rootCmd.AddCommand(tokenCmd)

	if err := rootCmd.Execute(); err != nil {
		log.Fatal(err)
	}
}

---FILE-END filename="/home/stevegt/lab/grokker/x/storm/cli.go"---

---FILE-START filename="/home/stevegt/lab/grokker/x/storm/main.go"---
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

	// Track cancelled queries by queryID[1]
	cancelledQueries = make(map[string]bool)
	cancelledMutex   sync.Mutex
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

	// Create Huma API with dynamic version from build-time injection[1]
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

// resolveFilePath converts a relative path to absolute using the project's BaseDir[1]
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

		// Handle incoming query messages from clients
		if msgType, ok := msg["type"].(string); ok {
			if msgType == "query" {
				log.Printf("Received query from %s in project %s: %v", c.id, c.projectID, msg)

				// Extract query parameters
				query, _ := msg["query"].(string)
				llm, _ := msg["llm"].(string)
				selection, _ := msg["selection"].(string)
				queryID, _ := msg["queryID"].(string)

				// Extract arrays and resolve relative paths to absolute[1]
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
				// Handle query cancellation[1]
				queryID, _ := msg["queryID"].(string)
				cancelledMutex.Lock()
				cancelledQueries[queryID] = true
				cancelledMutex.Unlock()
				log.Printf("Query %s marked for cancellation", queryID)
			}
		}
	}
}

// processQuery processes a query and broadcasts results to all clients in the project.
func processQuery(project *Project, queryID, query, llm, selection string, inputFiles, outFiles []string, tokenLimit int) {
	// Clean up cancellation flag when done[1]
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
	responseText, err := sendQueryToLLM(queryID, query, llm, selection, lastN, inputFiles, outFiles, tokenLimit)
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

// isQueryCancelled checks if a query has been marked for cancellation[1]
func isQueryCancelled(queryID string) bool {
	cancelledMutex.Lock()
	defer cancelledMutex.Unlock()
	return cancelledQueries[queryID]
}

// sendQueryToLLM calls the Grokker API to obtain a markdown-formatted text.
// Checks if the query was cancelled after the LLM call completes and discards the result if so.
func sendQueryToLLM(queryID, query string, llm string, selection, backgroundContext string, inputFiles []string, outFiles []string, tokenLimit int) (string, error) {
	if tokenLimit == 0 {
		tokenLimit = 8192
	}

	wordLimit := int(float64(tokenLimit) / 3.5)

	sysmsg := "You are a researcher.  I will start my prompt with some context, followed by a query.  Answer the query -- don't answer other questions you might see elsewhere in the context.  Always enclose reference numbers in square brackets; ignore empty brackets in the prompt or context, and DO NOT INCLUDE EMPTY SQUARE BRACKETS in your response, regardless of what you see in the context.  Always start your response with a markdown heading.  Try as much as possible to not rearrange any file you are making changes to -- I need to be able to easily diff your changes.  If writing Go code, you MUST ensure you are not skipping the index on slices or arrays, e.g. if you mean `foo` then say `foo`, not `foo`."

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
		for _, f := range outFiles {
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
		cookedResponse, err = core.ExtractFiles(outFilesConverted, response, core.ExtractOptions{
			DryRun:             true,
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
			tokenLimit *= 2
			continue
		}

		// successful response within token limit, so now run ExtractFiles for real
		cookedResponse, err = core.ExtractFiles(outFilesConverted, response, core.ExtractOptions{
			DryRun:             false,
			ExtractToStdout:    false,
			RemoveFromResponse: true,
		})

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

---FILE-END filename="/home/stevegt/lab/grokker/x/storm/main.go"---

---FILE-START filename="/home/stevegt/lab/grokker/x/storm/Makefile"---
.PHONY: build clean test run serve

# Build Storm CLI binary with build-time version injection from git[1]
build:
	@echo "Building Storm with build-time version injection..."
	@VERSION=$$(git describe --tags --exact-match 2>/dev/null || echo "dev-$$(git rev-parse --short HEAD)") && \
	go build -ldflags="-X github.com/stevegt/grokker/x/storm/version.Version=$$VERSION" -o storm ./cmd/cli.go

# Alternative: Include branch name in development builds
build-dev:
	@echo "Building Storm (dev) with git info..."
	@BRANCH=$$(git rev-parse --abbrev-ref HEAD) && \
	HASH=$$(git rev-parse --short HEAD) && \
	VERSION="dev-$$BRANCH-$$HASH" && \
	go build -ldflags="-X github.com/stevegt/grokker/x/storm/version.Version=$$VERSION" -o storm ./cmd/cli.go

# Run tests
test:
	@go test -v -p 1 ./...

# Run the serve command (daemon mode)
serve: build
	@./storm serve

# Clean build artifacts
clean:
	@rm -f storm

# Display current version
version:
	@go run -ldflags="-X github.com/stevegt/grokker/x/storm/version.Version=$$(git describe --tags --exact-match 2>/dev/null || echo 'dev-$$(git rev-parse --short HEAD)')" ./cli.go

