package testutil

import (
	"fmt"
	"io/ioutil"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// TestServer encapsulates a temporary Storm test server instance
type TestServer struct {
	Port         int
	TmpDir       string
	DBPath       string
	URL          string
	ProjectID    string
	ProjectDir   string
	MarkdownFile string
}

// getAvailablePort returns an available TCP port by binding to port 0
func getAvailablePort() (int, error) {
	listener, err := net.Listen("tcp", ":0")
	if err != nil {
		return 0, err
	}
	defer listener.Close()
	return listener.Addr().(*net.TCPAddr).Port, nil
}

// WaitForServer polls the server until it responds or timeout occurs
func WaitForServer(port int, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		conn, err := net.DialTimeout("tcp", fmt.Sprintf("localhost:%d", port), 500*time.Millisecond)
		if err == nil {
			conn.Close()
			return nil
		}
		time.Sleep(100 * time.Millisecond)
	}
	return fmt.Errorf("server not responding after %v", timeout)
}

// NewTestServer creates test server configuration but does not start the server.
// Caller (from main package tests) is responsible for starting serveRun in a goroutine
func NewTestServer(t *testing.T, projectID string) *TestServer {
	// Get available port
	port, err := getAvailablePort()
	if err != nil {
		t.Fatalf("Failed to get available port: %v", err)
	}

	// Create temporary directory for this test
	tmpDir, err := ioutil.TempDir("", fmt.Sprintf("storm-test-%s-", projectID))
	if err != nil {
		t.Fatalf("Failed to create temporary directory: %v", err)
	}

	// Create project directory
	projectDir := filepath.Join(tmpDir, projectID)
	if err := os.MkdirAll(projectDir, 0755); err != nil {
		t.Fatalf("Failed to create project directory: %v", err)
	}

	// Create markdown file for chat history
	markdownFile := filepath.Join(projectDir, "chat.md")
	if err := ioutil.WriteFile(markdownFile, []byte(""), 0644); err != nil {
		t.Fatalf("Failed to create markdown file: %v", err)
	}

	// Database path
	dbPath := filepath.Join(tmpDir, "test.db")

	// Build server URL
	serverURL := fmt.Sprintf("http://localhost:%d", port)

	return &TestServer{
		Port:         port,
		TmpDir:       tmpDir,
		DBPath:       dbPath,
		URL:          serverURL,
		ProjectID:    projectID,
		ProjectDir:   projectDir,
		MarkdownFile: markdownFile,
	}
}

// Cleanup removes temporary files and resources
func (ts *TestServer) Cleanup(t *testing.T) {
	// Wait a bit for server to shut down
	time.Sleep(500 * time.Millisecond)

	// Clean up temporary directory
	if err := os.RemoveAll(ts.TmpDir); err != nil {
		t.Logf("Warning: failed to remove temporary directory: %v", err)
	}
}
