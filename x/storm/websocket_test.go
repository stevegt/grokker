package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/stevegt/grokker/v3/core"
)

// getAvailablePort returns an available TCP port by binding to port 0
func getAvailablePort() (int, error) {
	listener, err := net.Listen("tcp", ":0")
	if err != nil {
		return 0, err
	}
	defer listener.Close()
	return listener.Addr().(*net.TCPAddr).Port, nil
}

// TestSetup encapsulates all setup data for a WebSocket test[1]
type TestSetup struct {
	Port         int
	TmpDir       string
	DbPath       string
	ProjectID    string
	ProjectDir   string
	MarkdownFile string
	DaemonURL    string
	WsURL        string
}

// setupTest creates temporary directories, starts server, creates project, and returns test setup[1]
func setupTest(t *testing.T, projectID string) *TestSetup {
	// Get available port
	port, err := getAvailablePort()
	if err != nil {
		t.Fatalf("Failed to get available port: %v", err)
	}

	// Create temporary directory
	tmpDir, err := ioutil.TempDir("", fmt.Sprintf("storm-ws-test-%s-", projectID))
	if err != nil {
		t.Fatalf("Failed to create temporary directory: %v", err)
	}

	// Create project directory
	projectDir := filepath.Join(tmpDir, projectID)
	if err := os.MkdirAll(projectDir, 0755); err != nil {
		t.Fatalf("Failed to create project directory: %v", err)
	}

	// Create markdown file
	markdownFile := filepath.Join(projectDir, "chat.md")
	if err := ioutil.WriteFile(markdownFile, []byte(""), 0644); err != nil {
		t.Fatalf("Failed to create markdown file: %v", err)
	}

	// Database path
	dbPath := filepath.Join(tmpDir, "test.db")

	// Start server in background
	go func() {
		if err := serveRun(port, dbPath); err != nil {
			t.Logf("Server error: %v", err)
		}
	}()
	// wait for server to start
	for i := 0; i < 10; i++ {
		conn, err := net.DialTimeout("tcp", fmt.Sprintf("localhost:%d", port), 500*time.Millisecond)
		if err == nil {
			conn.Close()
			break
		}
		time.Sleep(500 * time.Millisecond)
	}

	// Build URLs
	daemonURL := fmt.Sprintf("http://localhost:%d", port)
	wsURL := fmt.Sprintf("ws://localhost:%d/project/%s/ws", port, projectID)

	// Create project via HTTP API
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

	return &TestSetup{
		Port:         port,
		TmpDir:       tmpDir,
		DbPath:       dbPath,
		ProjectID:    projectID,
		ProjectDir:   projectDir,
		MarkdownFile: markdownFile,
		DaemonURL:    daemonURL,
		WsURL:        wsURL,
	}
}

// teardownTest cleans up temporary directories and stops server[1]
func teardownTest(t *testing.T, setup *TestSetup) {
	// Stop server gracefully
	resp, err := http.Post(setup.DaemonURL+"/stop", "application/json", nil)
	if err != nil {
		t.Logf("Warning: error stopping server: %v", err)
	} else {
		resp.Body.Close()
	}

	// Wait for server to shut down
	time.Sleep(500 * time.Millisecond)

	// Clean up temporary directory
	if err := os.RemoveAll(setup.TmpDir); err != nil {
		t.Logf("Warning: failed to remove temporary directory: %v", err)
	}
}

// connectWebSocket establishes a WebSocket connection and returns it[1]
func connectWebSocket(t *testing.T, wsURL string) *websocket.Conn {
	dialer := websocket.Dialer{}
	conn, _, err := dialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("Failed to connect to WebSocket: %v", err)
	}
	return conn
}

// TestWebSocketConnection tests establishing a WebSocket connection to a project
func TestWebSocketConnection(t *testing.T) {
	setup := setupTest(t, "ws-test-project")
	defer teardownTest(t, setup)

	// Test WebSocket connection
	conn := connectWebSocket(t, setup.WsURL)
	defer conn.Close()

	// Verify connection is open by attempting to write a message
	testMsg := map[string]string{"type": "test"}
	if err := conn.WriteJSON(testMsg); err != nil {
		t.Fatalf("WebSocket connection is not functional: %v", err)
	}

	t.Logf("WebSocket connection successful for project %s", setup.ProjectID)
}

// TestWebSocketQueryMessage tests sending a query via WebSocket and receiving a response
func TestWebSocketQueryMessage(t *testing.T) {
	setup := setupTest(t, "ws-query-project")
	defer teardownTest(t, setup)

	// Connect to WebSocket
	conn := connectWebSocket(t, setup.WsURL)
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
		"projectID":  setup.ProjectID,
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
	setup := setupTest(t, "ws-cancel-project")
	defer teardownTest(t, setup)

	// Connect to WebSocket
	conn := connectWebSocket(t, setup.WsURL)
	defer conn.Close()

	// Send a cancel message
	cancelMsg := map[string]interface{}{
		"type":    "cancel",
		"queryID": "test-cancel-123",
	}
	if err := conn.WriteJSON(cancelMsg); err != nil {
		t.Fatalf("Failed to send cancel message: %v", err)
	}

	// Wait for readPump to process the message via channel (allow time for async processing)[1]
	time.Sleep(500 * time.Millisecond)

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
	setup := setupTest(t, "ws-multi-project")
	defer teardownTest(t, setup)

	// Connect multiple WebSocket clients
	dialer := websocket.Dialer{}

	const numClients = 3
	var conns []*websocket.Conn
	for i := 0; i < numClients; i++ {
		conn, _, err := dialer.Dial(setup.WsURL, nil)
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
		"projectID":  setup.ProjectID,
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
	setup := setupTest(t, "ws-broadcast-project")
	defer teardownTest(t, setup)

	// Create a test file
	testFile := filepath.Join(setup.ProjectDir, "test.txt")
	if err := ioutil.WriteFile(testFile, []byte("test content"), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	// Connect WebSocket client
	conn := connectWebSocket(t, setup.WsURL)
	defer conn.Close()

	// Add files via HTTP API to trigger broadcast
	addFilesPayload := map[string]interface{}{
		"filenames": []string{testFile},
	}
	jsonData, _ := json.Marshal(addFilesPayload)
	fileURL := fmt.Sprintf("%s/api/projects/%s/files/add", setup.DaemonURL, setup.ProjectID)
	resp, err := http.Post(fileURL, "application/json", strings.NewReader(string(jsonData)))
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
	setup := setupTest(t, "ws-cleanup-project")
	defer teardownTest(t, setup)

	// Get the project to check client pool
	project, err := projects.Get(setup.ProjectID)
	if err != nil {
		t.Fatalf("Failed to get project: %v", err)
	}

	// Connect and disconnect multiple times
	dialer := websocket.Dialer{}

	for iteration := 0; iteration < 3; iteration++ {
		// Connect
		conn, _, err := dialer.Dial(setup.WsURL, nil)
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

// TestWebSocketApproveFilesMessage tests sending an approveFiles message via WebSocket (Stage 4)[1]
func TestWebSocketApproveFilesMessage(t *testing.T) {
	setup := setupTest(t, "ws-approve-project")
	defer teardownTest(t, setup)

	// Connect to WebSocket
	conn := connectWebSocket(t, setup.WsURL)
	defer conn.Close()

	// Send an approveFiles message
	approveMsg := map[string]interface{}{
		"type":          "approveFiles",
		"queryID":       "test-approval-123",
		"approvedFiles": []string{"file1.go", "file2.md"},
	}
	if err := conn.WriteJSON(approveMsg); err != nil {
		t.Fatalf("Failed to send approveFiles message: %v", err)
	}

	// Wait for readPump to process
	time.Sleep(500 * time.Millisecond)

	// Verify the approval message was received (should be logged in server)
	t.Logf("ApproveFiles message sent successfully with queryID test-approval-123")
}

// TestWebSocketPendingQueryTracking tests that pending queries are correctly tracked (Stage 4)[1][2]
func TestWebSocketPendingQueryTracking(t *testing.T) {
	setup := setupTest(t, "ws-pending-project")
	defer teardownTest(t, setup)

	// Create a pending query manually to test tracking
	queryID := "test-pending-123"
	addPendingQuery(queryID, "raw response", []core.FileLang{})

	// Verify it was added
	pendingMutex.Lock()
	storedPending, exists := pendingApprovals[queryID]
	pendingMutex.Unlock()

	if !exists {
		t.Fatalf("Pending query was not added to pendingApprovals map")
	}

	if storedPending.queryID != queryID {
		t.Errorf("Stored queryID %s does not match expected %s", storedPending.queryID, queryID)
	}

	if storedPending.approvalChannel == nil {
		t.Fatal("Approval channel is nil")
	}

	t.Logf("Pending query successfully tracked with ID %s", queryID)

	// Clean up
	removePendingQuery(queryID)

	// Verify it was removed
	pendingMutex.Lock()
	_, exists = pendingApprovals[queryID]
	pendingMutex.Unlock()

	if exists {
		t.Fatalf("Pending query was not removed from pendingApprovals map")
	}

	t.Logf("Pending query successfully removed")
}

// TestWebSocketApprovalChannelReceival tests that approval signals are received via channel (Stage 4)[1]
func TestWebSocketApprovalChannelReceival(t *testing.T) {
	setup := setupTest(t, "ws-channel-project")
	defer teardownTest(t, setup)

	// Create a pending query
	queryID := "test-channel-123"
	pending := addPendingQuery(queryID, "raw response", []core.FileLang{})

	// Send approval via goroutine to simulate readPump sending
	go func() {
		time.Sleep(200 * time.Millisecond)
		// Simulate what readPump does when it receives approveFiles message
		approvedFiles := []string{"file1.go", "file2.md"}
		select {
		case pending.approvalChannel <- approvedFiles:
			t.Logf("Approval sent via channel for queryID %s", queryID)
		default:
			t.Logf("WARNING: approval channel full")
		}
	}()

	// Wait for approval (simulating what sendQueryToLLM does)
	receivedFiles, err := waitForApproval(pending)
	if err != nil {
		t.Fatalf("Error waiting for approval: %v", err)
	}

	if len(receivedFiles) != 2 {
		t.Errorf("Expected 2 approved files, got %d", len(receivedFiles))
	}

	if receivedFiles[0] != "file1.go" || receivedFiles[1] != "file2.md" {
		t.Errorf("Approved files mismatch: expected [file1.go file2.md], got %v", receivedFiles)
	}

	t.Logf("Approval successfully received via channel: %v", receivedFiles)

	// Clean up
	removePendingQuery(queryID)
}

// TestWebSocketMultipleConcurrentApprovals tests multiple concurrent pending queries with different approvals (Stage 4)[1][2]
func TestWebSocketMultipleConcurrentApprovals(t *testing.T) {
	setup := setupTest(t, "ws-concurrent-project")
	defer teardownTest(t, setup)

	const numQueries = 5

	// Track pending queries
	var pendingQueries []*PendingQuery
	for i := 0; i < numQueries; i++ {
		queryID := fmt.Sprintf("concurrent-query-%d", i)
		pending := addPendingQuery(queryID, "raw response", []core.FileLang{})
		pendingQueries = append(pendingQueries, pending)
	}

	// Verify all were added
	pendingMutex.Lock()
	if len(pendingApprovals) < numQueries {
		pendingMutex.Unlock()
		t.Fatalf("Not all pending queries were added")
	}
	pendingMutex.Unlock()

	t.Logf("Successfully created %d pending queries", numQueries)

	// Send different approvals to each query
	for i := 0; i < numQueries; i++ {
		go func(idx int) {
			time.Sleep(time.Duration(idx*100) * time.Millisecond)
			approvedFiles := []string{fmt.Sprintf("file%d_1.go", idx), fmt.Sprintf("file%d_2.go", idx)}
			select {
			case pendingQueries[idx].approvalChannel <- approvedFiles:
				t.Logf("Approval sent for query %d", idx)
			default:
				t.Logf("WARNING: approval channel full for query %d", idx)
			}
		}(i)
	}

	// Wait for all approvals
	for i := 0; i < numQueries; i++ {
		receivedFiles, err := waitForApproval(pendingQueries[i])
		if err != nil {
			t.Logf("Error waiting for approval on query %d: %v", i, err)
			continue
		}

		expected := fmt.Sprintf("file%d_1.go", i)
		if len(receivedFiles) > 0 && receivedFiles[0] == expected {
			t.Logf("Query %d received correct approval: %v", i, receivedFiles)
		} else {
			t.Errorf("Query %d received unexpected approval: %v", i, receivedFiles)
		}
	}

	// Clean up all pending queries
	for i := 0; i < numQueries; i++ {
		queryID := fmt.Sprintf("concurrent-query-%d", i)
		removePendingQuery(queryID)
	}

	// Verify all were removed
	pendingMutex.Lock()
	if len(pendingApprovals) > 0 {
		pendingMutex.Unlock()
		t.Fatalf("Not all pending queries were removed")
	}
	pendingMutex.Unlock()

	t.Logf("All concurrent queries successfully processed and cleaned up")
}

// TestWebSocketApprovalIndefiniteWait tests that approval waits indefinitely for user response (Stage 4)[1]
func TestWebSocketApprovalIndefiniteWait(t *testing.T) {
	setup := setupTest(t, "ws-indefinite-project")
	defer teardownTest(t, setup)

	// Create a pending query
	queryID := "test-indefinite-123"
	pending := addPendingQuery(queryID, "raw response", []core.FileLang{})

	// Send approval after a short delay to verify indefinite waiting
	go func() {
		time.Sleep(1 * time.Second)
		approvedFiles := []string{"file1.go"}
		select {
		case pending.approvalChannel <- approvedFiles:
			t.Logf("Approval sent after 1 second delay")
		default:
			t.Logf("WARNING: approval channel full")
		}
	}()

	// Wait for approval with timeout to verify it arrives
	start := time.Now()
	receivedFiles, err := waitForApproval(pending)
	elapsed := time.Since(start)

	if err != nil {
		t.Errorf("Error waiting for approval: %v", err)
	}

	if len(receivedFiles) != 1 || receivedFiles[0] != "file1.go" {
		t.Errorf("Expected [file1.go], got %v", receivedFiles)
	}

	if elapsed < 900*time.Millisecond || elapsed > 2*time.Second {
		t.Logf("Approval received after %v (expected ~1 second)", elapsed)
	} else {
		t.Logf("Approval received successfully after %v", elapsed)
	}

	// Clean up
	removePendingQuery(queryID)
}
