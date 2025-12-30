package main

import (
	_ "embed"
	"fmt"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/gorilla/websocket"

	// . "github.com/stevegt/goadapt"
	"github.com/stevegt/grokker/v3/core"
)

var (
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

// WebSocket client connection.
type WSClient struct {
	conn      *websocket.Conn
	send      chan interface{}
	pool      *ClientPool
	id        string
	projectID string
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

				// Convert approved files to string slice and sanitize to absolute paths
				var approvedFiles []string
				for i := 0; i < len(approvedFilesRaw); i++ {
					if f, ok := approvedFilesRaw[i].(string); ok {
						// Sanitize: convert relative paths to absolute at entry point
						// TODO: further sanitize to ensure files are
						// within project directory, avoiding relative
						// path traversal attacks
						absPath := resolveFilePath(project, f)
						approvedFiles = append(approvedFiles, absPath)
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
			} else if msgType == "debug" {
				// Handle debug message from browser client
				debugMessage, _ := msg["message"].(string)
				clientID, _ := msg["clientID"].(string)
				log.Printf("[DEBUG %s] %s", clientID, debugMessage)
			}
		}
	}
}

// startNotificationTicker begins periodically re-sending the unexpected files notification every 10 seconds
func startNotificationTicker(pending *PendingQuery) {
	pending.notificationTicker = time.NewTicker(unexpectedFilesNotifyInterval)

	go func() {
		for {
			select {
			case <-pending.notificationTicker.C:
				// Re-send the unexpected files notification using unified filesUpdated message
				filesUpdatedMsg := map[string]interface{}{
					"type":                     "filesUpdated",
					"projectID":                pending.project.ID,
					"isUnexpectedFilesContext": true,
					"queryID":                  pending.queryID,
					"alreadyAuthorized":        pending.alreadyAuthorized,
					"needsAuthorization":       pending.needsAuthorization,
					"files":                    pending.project.GetFilesAsRelative(),
				}
				pending.project.ClientPool.Broadcast(filesUpdatedMsg)
				log.Printf("Re-broadcasted filesUpdated notification for query %s", pending.queryID)

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
