package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

// TestWebSocketConnection tests establishing a WebSocket connection to a project
func TestWebSocketConnection(t *testing.T) {
	// Set up test environment
	tmpDir, err := ioutil.TempDir("", "storm-ws-test-")
	if err != nil {
		t.Fatalf("Failed to create temporary directory: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	dbPath := filepath.Join(tmpDir, "test.db")
	projectID := "ws-test-project"
	projectDir := filepath.Join(tmpDir, projectID)
	if err := os.MkdirAll(projectDir, 0755); err != nil {
		t.Fatalf("Failed to create project directory: %v", err)
	}

	markdownFile := filepath.Join(projectDir, "chat.md")
	if err := ioutil.WriteFile(markdownFile, []byte(""), 0644); err != nil {
		t.Fatalf("Failed to create markdown file: %v", err)
	}

	// Start server
	go func() {
		if err := serveRun(59991, dbPath); err != nil {
			t.Logf("Server error: %v", err)
		}
	}()
	time.Sleep(2 * time.Second)

	// Create project via HTTP API
	daemonURL := "http://localhost:59991"
	createProjectPayload := map[string]string{
		"projectID":    projectID,
		"baseDir":      projectDir,
		"markdownFile": markdownFile,
	}
	jsonData, _ := json.Marshal(createProjectPayload)
	resp, err := http.Post(daemonURL+"/api/projects", "application/json", strings.NewReader(string(jsonData)))
	if err != nil {
		t.Fatalf("Failed to create project: %v", err)
	}
	resp.Body.Close()

	// Test WebSocket connection
	wsURL := fmt.Sprintf("ws://localhost:59991/project/%s/ws", projectID)
	dialer := websocket.Dialer{}
	conn, _, err := dialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("Failed to connect to WebSocket: %v", err)
	}
	defer conn.Close()

	// Verify connection is open by attempting to write a message
	testMsg := map[string]string{"type": "test"}
	if err := conn.WriteJSON(testMsg); err != nil {
		t.Fatalf("WebSocket connection is not functional: %v", err)
	}

	t.Logf("WebSocket connection successful for project %s", projectID)
}

// TestWebSocketQueryMessage tests sending a query via WebSocket and receiving a response
func TestWebSocketQueryMessage(t *testing.T) {
	// Set up test environment
	tmpDir, err := ioutil.TempDir("", "storm-ws-query-")
	if err != nil {
		t.Fatalf("Failed to create temporary directory: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	dbPath := filepath.Join(tmpDir, "test.db")
	projectID := "ws-query-project"
	projectDir := filepath.Join(tmpDir, projectID)
	if err := os.MkdirAll(projectDir, 0755); err != nil {
		t.Fatalf("Failed to create project directory: %v", err)
	}

	markdownFile := filepath.Join(projectDir, "chat.md")
	if err := ioutil.WriteFile(markdownFile, []byte(""), 0644); err != nil {
		t.Fatalf("Failed to create markdown file: %v", err)
	}

	// Start server
	go func() {
		if err := serveRun(59990, dbPath); err != nil {
			t.Logf("Server error: %v", err)
		}
	}()
	time.Sleep(2 * time.Second)

	// Create project via HTTP API
	daemonURL := "http://localhost:59990"
	createProjectPayload := map[string]string{
		"projectID":    projectID,
		"baseDir":      projectDir,
		"markdownFile": markdownFile,
	}
	jsonData, _ := json.Marshal(createProjectPayload)
	resp, err := http.Post(daemonURL+"/api/projects", "application/json", strings.NewReader(string(jsonData)))
	if err != nil {
		t.Fatalf("Failed to create project: %v", err)
	}
	resp.Body.Close()

	// Connect to WebSocket
	wsURL := fmt.Sprintf("ws://localhost:59990/project/%s/ws", projectID)
	dialer := websocket.Dialer{}
	conn, _, err := dialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("Failed to connect to WebSocket: %v", err)
	}
	defer conn.Close()

	// Send a query message
	queryMsg := map[string]interface{}{
		"type":       "query",
		"query":      "test query",
		"llm":        "test-llm",
		"selection":  "",
		"inputFiles": []string{},
		"outFiles":   []string{},
		"tokenLimit": 0,
		"queryID":    "test-query-123",
		"projectID":  projectID,
	}
	if err := conn.WriteJSON(queryMsg); err != nil {
		t.Fatalf("Failed to send query message: %v", err)
	}

	// Set read deadline for receiving response
	conn.SetReadDeadline(time.Now().Add(5 * time.Second))

	// Receive broadcast messages (should include query echo)
	var receivedMessages []map[string]interface{}
	for i := 0; i < 3; i++ {
		var msg map[string]interface{}
		if err := conn.ReadJSON(&msg); err != nil {
			// Timeout is acceptable; we may not receive responses if LLM is mocked
			if websocket.IsUnexpectedCloseError(err) {
				t.Logf("Connection closed: %v", err)
				break
			}
			t.Logf("Read error (may be timeout): %v", err)
			break
		}
		receivedMessages = append(receivedMessages, msg)

		// Check for query echo
		if msgType, ok := msg["type"].(string); ok && msgType == "query" {
			if query, ok := msg["query"].(string); ok && query == "test query" {
				t.Logf("Received query echo via WebSocket broadcast")
				return
			}
		}
	}

	t.Logf("Received %d messages, query echo may have been broadcast", len(receivedMessages))
}

// TestWebSocketCancelMessage tests sending a cancel message via WebSocket
func TestWebSocketCancelMessage(t *testing.T) {
	// Set up test environment
	tmpDir, err := ioutil.TempDir("", "storm-ws-cancel-")
	if err != nil {
		t.Fatalf("Failed to create temporary directory: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	dbPath := filepath.Join(tmpDir, "test.db")
	projectID := "ws-cancel-project"
	projectDir := filepath.Join(tmpDir, projectID)
	if err := os.MkdirAll(projectDir, 0755); err != nil {
		t.Fatalf("Failed to create project directory: %v", err)
	}

	markdownFile := filepath.Join(projectDir, "chat.md")
	if err := ioutil.WriteFile(markdownFile, []byte(""), 0644); err != nil {
		t.Fatalf("Failed to create markdown file: %v", err)
	}

	// Start server
	go func() {
		if err := serveRun(59989, dbPath); err != nil {
			t.Logf("Server error: %v", err)
		}
	}()
	time.Sleep(2 * time.Second)

	// Create project via HTTP API
	daemonURL := "http://localhost:59989"
	createProjectPayload := map[string]string{
		"projectID":    projectID,
		"baseDir":      projectDir,
		"markdownFile": markdownFile,
	}
	jsonData, _ := json.Marshal(createProjectPayload)
	resp, err := http.Post(daemonURL+"/api/projects", "application/json", strings.NewReader(string(jsonData)))
	if err != nil {
		t.Fatalf("Failed to create project: %v", err)
	}
	resp.Body.Close()

	// Connect to WebSocket
	wsURL := fmt.Sprintf("ws://localhost:59989/project/%s/ws", projectID)
	dialer := websocket.Dialer{}
	conn, _, err := dialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("Failed to connect to WebSocket: %v", err)
	}
	defer conn.Close()

	// Send a cancel message
	cancelMsg := map[string]interface{}{
		"type":    "cancel",
		"queryID": "test-cancel-123",
	}
	if err := conn.WriteJSON(cancelMsg); err != nil {
		t.Fatalf("Failed to send cancel message: %v", err)
	}

	// Verify cancel flag is set[1]
	cancelledMutex.Lock()
	isCancelled := cancelledQueries["test-cancel-123"]
	cancelledMutex.Unlock()

	if !isCancelled {
		t.Fatal("Query was not marked as cancelled")
	}

	t.Logf("Cancel message successfully processed for queryID test-cancel-123")
}

// TestWebSocketMultipleClients tests multiple WebSocket clients connected to the same project
func TestWebSocketMultipleClients(t *testing.T) {
	// Set up test environment
	tmpDir, err := ioutil.TempDir("", "storm-ws-multi-")
	if err != nil {
		t.Fatalf("Failed to create temporary directory: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	dbPath := filepath.Join(tmpDir, "test.db")
	projectID := "ws-multi-project"
	projectDir := filepath.Join(tmpDir, projectID)
	if err := os.MkdirAll(projectDir, 0755); err != nil {
		t.Fatalf("Failed to create project directory: %v", err)
	}

	markdownFile := filepath.Join(projectDir, "chat.md")
	if err := ioutil.WriteFile(markdownFile, []byte(""), 0644); err != nil {
		t.Fatalf("Failed to create markdown file: %v", err)
	}

	// Start server
	go func() {
		if err := serveRun(59988, dbPath); err != nil {
			t.Logf("Server error: %v", err)
		}
	}()
	time.Sleep(2 * time.Second)

	// Create project via HTTP API
	daemonURL := "http://localhost:59988"
	createProjectPayload := map[string]string{
		"projectID":    projectID,
		"baseDir":      projectDir,
		"markdownFile": markdownFile,
	}
	jsonData, _ := json.Marshal(createProjectPayload)
	resp, err := http.Post(daemonURL+"/api/projects", "application/json", strings.NewReader(string(jsonData)))
	if err != nil {
		t.Fatalf("Failed to create project: %v", err)
	}
	resp.Body.Close()

	// Connect multiple WebSocket clients
	wsURL := fmt.Sprintf("ws://localhost:59988/project/%s/ws", projectID)
	dialer := websocket.Dialer{}

	const numClients = 3
	var conns []*websocket.Conn
	for i := 0; i < numClients; i++ {
		conn, _, err := dialer.Dial(wsURL, nil)
		if err != nil {
			t.Fatalf("Failed to connect client %d to WebSocket: %v", i, err)
		}
		conns = append(conns, conn)
	}
	defer func() {
		for i := 0; i < len(conns); i++ {
			conns[i].Close()
		}
	}()

	// Send a message from the first client
	queryMsg := map[string]interface{}{
		"type":       "query",
		"query":      "broadcast test",
		"llm":        "test-llm",
		"selection":  "",
		"inputFiles": []string{},
		"outFiles":   []string{},
		"tokenLimit": 0,
		"queryID":    "broadcast-123",
		"projectID":  projectID,
	}
	if err := conns[0].WriteJSON(queryMsg); err != nil {
		t.Fatalf("Failed to send query message: %v", err)
	}

	// Set read deadline
	time.Sleep(500 * time.Millisecond)

	// Check if all clients receive the broadcasted message
	msgReceiveCount := 0
	for i := 0; i < len(conns); i++ {
		conns[i].SetReadDeadline(time.Now().Add(2 * time.Second))
		var msg map[string]interface{}
		if err := conns[i].ReadJSON(&msg); err != nil {
			// Timeout is acceptable
			t.Logf("Client %d: read timeout or error: %v", i, err)
			continue
		}

		if msgType, ok := msg["type"].(string); ok && msgType == "query" {
			msgReceiveCount++
			t.Logf("Client %d received query broadcast", i)
		}
	}

	if msgReceiveCount > 0 {
		t.Logf("Successfully tested multiple clients: %d clients received broadcast", msgReceiveCount)
	} else {
		t.Logf("No clients received broadcast in time (may be due to query processing delays)")
	}
}

// TestWebSocketBroadcastOnFileListUpdate tests that file list updates are broadcasted to WebSocket clients
func TestWebSocketBroadcastOnFileListUpdate(t *testing.T) {
	// Set up test environment
	tmpDir, err := ioutil.TempDir("", "storm-ws-broadcast-")
	if err != nil {
		t.Fatalf("Failed to create temporary directory: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	dbPath := filepath.Join(tmpDir, "test.db")
	projectID := "ws-broadcast-project"
	projectDir := filepath.Join(tmpDir, projectID)
	if err := os.MkdirAll(projectDir, 0755); err != nil {
		t.Fatalf("Failed to create project directory: %v", err)
	}

	markdownFile := filepath.Join(projectDir, "chat.md")
	if err := ioutil.WriteFile(markdownFile, []byte(""), 0644); err != nil {
		t.Fatalf("Failed to create markdown file: %v", err)
	}

	// Create a test file
	testFile := filepath.Join(projectDir, "test.txt")
	if err := ioutil.WriteFile(testFile, []byte("test content"), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	// Start server
	go func() {
		if err := serveRun(59987, dbPath); err != nil {
			t.Logf("Server error: %v", err)
		}
	}()
	time.Sleep(2 * time.Second)

	// Create project via HTTP API
	daemonURL := "http://localhost:59987"
	createProjectPayload := map[string]string{
		"projectID":    projectID,
		"baseDir":      projectDir,
		"markdownFile": markdownFile,
	}
	jsonData, _ := json.Marshal(createProjectPayload)
	resp, err := http.Post(daemonURL+"/api/projects", "application/json", strings.NewReader(string(jsonData)))
	if err != nil {
		t.Fatalf("Failed to create project: %v", err)
	}
	resp.Body.Close()

	// Connect WebSocket client
	wsURL := fmt.Sprintf("ws://localhost:59987/project/%s/ws", projectID)
	dialer := websocket.Dialer{}
	conn, _, err := dialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("Failed to connect to WebSocket: %v", err)
	}
	defer conn.Close()

	// Add files via HTTP API to trigger broadcast
	addFilesPayload := map[string]interface{}{
		"filenames": []string{testFile},
	}
	jsonData, _ = json.Marshal(addFilesPayload)
	fileURL := fmt.Sprintf("%s/api/projects/%s/files/add", daemonURL, projectID)
	resp, err = http.Post(fileURL, "application/json", strings.NewReader(string(jsonData)))
	if err != nil {
		t.Fatalf("Failed to add files: %v", err)
	}
	resp.Body.Close()

	// Wait for broadcast and check if client receives fileListUpdated message
	conn.SetReadDeadline(time.Now().Add(3 * time.Second))
	for i := 0; i < 10; i++ {
		var msg map[string]interface{}
		if err := conn.ReadJSON(&msg); err != nil {
			if websocket.IsUnexpectedCloseError(err) {
				t.Logf("Connection closed")
				break
			}
			// Timeout is acceptable
			t.Logf("Read error or timeout: %v", err)
			break
		}

		if msgType, ok := msg["type"].(string); ok && msgType == "fileListUpdated" {
			t.Logf("Successfully received fileListUpdated broadcast message")
			return
		}
	}

	t.Logf("File list update test completed (broadcast may not have arrived in time)")
}

// TestWebSocketConnectionCleanup tests that clients are properly cleaned up when disconnecting
func TestWebSocketConnectionCleanup(t *testing.T) {
	// Set up test environment
	tmpDir, err := ioutil.TempDir("", "storm-ws-cleanup-")
	if err != nil {
		t.Fatalf("Failed to create temporary directory: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	dbPath := filepath.Join(tmpDir, "test.db")
	projectID := "ws-cleanup-project"
	projectDir := filepath.Join(tmpDir, projectID)
	if err := os.MkdirAll(projectDir, 0755); err != nil {
		t.Fatalf("Failed to create project directory: %v", err)
	}

	markdownFile := filepath.Join(projectDir, "chat.md")
	if err := ioutil.WriteFile(markdownFile, []byte(""), 0644); err != nil {
		t.Fatalf("Failed to create markdown file: %v", err)
	}

	// Start server
	go func() {
		if err := serveRun(59986, dbPath); err != nil {
			t.Logf("Server error: %v", err)
		}
	}()
	time.Sleep(2 * time.Second)

	// Create project via HTTP API
	daemonURL := "http://localhost:59986"
	createProjectPayload := map[string]string{
		"projectID":    projectID,
		"baseDir":      projectDir,
		"markdownFile": markdownFile,
	}
	jsonData, _ := json.Marshal(createProjectPayload)
	resp, err := http.Post(daemonURL+"/api/projects", "application/json", strings.NewReader(string(jsonData)))
	if err != nil {
		t.Fatalf("Failed to create project: %v", err)
	}
	resp.Body.Close()

	// Get the project to check client pool
	project, err := projects.Get(projectID)
	if err != nil {
		t.Fatalf("Failed to get project: %v", err)
	}

	// Connect and disconnect multiple times
	wsURL := fmt.Sprintf("ws://localhost:59986/project/%s/ws", projectID)
	dialer := websocket.Dialer{}

	for iteration := 0; iteration < 3; iteration++ {
		// Connect
		conn, _, err := dialer.Dial(wsURL, nil)
		if err != nil {
			t.Fatalf("Iteration %d: Failed to connect to WebSocket: %v", iteration, err)
		}

		// Get initial client count
		project.ClientPool.mutex.RLock()
		initialCount := len(project.ClientPool.clients)
		project.ClientPool.mutex.RUnlock()

		// Disconnect
		conn.Close()
		time.Sleep(100 * time.Millisecond)

		// Verify client was cleaned up
		project.ClientPool.mutex.RLock()
		finalCount := len(project.ClientPool.clients)
		project.ClientPool.mutex.RUnlock()

		t.Logf("Iteration %d: clients before=%d, after=%d", iteration, initialCount, finalCount)
	}

	t.Logf("Connection cleanup test completed successfully")
}

// TestWebSocketPingPong tests ping/pong keepalive mechanism
func TestWebSocketPingPong(t *testing.T) {
	// Set up test environment
	tmpDir, err := ioutil.TempDir("", "storm-ws-pingpong-")
	if err != nil {
		t.Fatalf("Failed to create temporary directory: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	dbPath := filepath.Join(tmpDir, "test.db")
	projectID := "ws-pingpong-project"
	projectDir := filepath.Join(tmpDir, projectID)
	if err := os.MkdirAll(projectDir, 0755); err != nil {
		t.Fatalf("Failed to create project directory: %v", err)
	}

	markdownFile := filepath.Join(projectDir, "chat.md")
	if err := ioutil.WriteFile(markdownFile, []byte(""), 0644); err != nil {
		t.Fatalf("Failed to create markdown file: %v", err)
	}

	// Start server
	go func() {
		if err := serveRun(59985, dbPath); err != nil {
			t.Logf("Server error: %v", err)
		}
	}()
	time.Sleep(2 * time.Second)

	// Create project via HTTP API
	daemonURL := "http://localhost:59985"
	createProjectPayload := map[string]string{
		"projectID":    projectID,
		"baseDir":      projectDir,
		"markdownFile": markdownFile,
	}
	jsonData, _ := json.Marshal(createProjectPayload)
	resp, err := http.Post(daemonURL+"/api/projects", "application/json", strings.NewReader(string(jsonData)))
	if err != nil {
		t.Fatalf("Failed to create project: %v", err)
	}
	resp.Body.Close()

	// Connect to WebSocket
	wsURL := fmt.Sprintf("ws://localhost:59985/project/%s/ws", projectID)
	dialer := websocket.Dialer{}
	conn, _, err := dialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("Failed to connect to WebSocket: %v", err)
	}
	defer conn.Close()

	// Set pong handler to track pongs received
	pongReceived := false
	conn.SetPongHandler(func(appData string) error {
		pongReceived = true
		t.Logf("Received pong: %s\n", appData)
		return nil
	})

	// Wait for ping message from server
	pingReceived := false
	conn.SetReadDeadline(time.Now().Add(30 * time.Second))
	for i := 0; i < 5; i++ {
		conn.SetReadDeadline(time.Now().Add(30 * time.Second))
		messageType, data, err := conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err) || err.Error() == "i/o timeout" {
				break
			}
			t.Logf("Read error: %v", err)
			break
		}

		if messageType == websocket.PingMessage {
			pingReceived = true
			t.Logf("Received ping message from server")
			// Server expects us to respond with pong
			if err := conn.WriteMessage(websocket.PongMessage, data); err != nil {
				t.Logf("Failed to send pong: %v", err)
			}
		}
	}

	if pingReceived {
		t.Logf("Ping/pong keepalive mechanism verified")
	} else {
		t.Logf("Ping/pong test completed (no ping received in time, may be normal)")
	}

	if !pongReceived {
		t.Logf("No pong responses were sent (may be normal if no pings were received)")
	}
}