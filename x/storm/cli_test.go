package main

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

// Helper function to execute CLI commands with environment setup
func runCLICommand(daemonURL string, args ...string) (string, string, error) {
	cmd := exec.Command("go", "run", ".")
	cmd.Args = append(cmd.Args, args...)
	cmd.Env = append(os.Environ(), fmt.Sprintf("STORM_DAEMON_URL=%s", daemonURL))

	var outBuf, errBuf bytes.Buffer
	cmd.Stdout = &outBuf
	cmd.Stderr = &errBuf

	err := cmd.Run()
	return outBuf.String(), errBuf.String(), err
}

// Helper function to set up a test project with temporary directory and markdown file
func setupTestProject(t *testing.T, projectID string) (string, string, func()) {
	// Create temporary project directory
	tmpDir, err := ioutil.TempDir("", fmt.Sprintf("storm-cli-test-%s-", projectID))
	if err != nil {
		t.Fatalf("Failed to create temporary directory: %v", err)
	}

	// Create project subdirectory
	projectDir := filepath.Join(tmpDir, projectID)
	if err := os.MkdirAll(projectDir, 0755); err != nil {
		t.Fatalf("Failed to create project directory: %v", err)
	}

	// Create markdown file
	markdownFile := filepath.Join(projectDir, "chat.md")
	if err := ioutil.WriteFile(markdownFile, []byte(""), 0644); err != nil {
		t.Fatalf("Failed to create markdown file: %v", err)
	}

	// Return project directory, markdown file, and cleanup function
	cleanup := func() {
		os.RemoveAll(tmpDir)
	}

	return projectDir, markdownFile, cleanup
}

// Helper function to check if daemon is ready
func waitForDaemon(daemonURL string, maxWait time.Duration) error {
	deadline := time.Now().Add(maxWait)
	for time.Now().Before(deadline) {
		resp, err := http.Get(daemonURL)
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return nil
			}
		}
		time.Sleep(100 * time.Millisecond)
	}
	return fmt.Errorf("daemon not ready after %v", maxWait)
}

// Helper function to start a test daemon and return its URL and cleanup function
func startTestDaemon(t *testing.T, port int) (string, func()) {
	// Create temporary database directory for this test
	tmpDir, err := ioutil.TempDir("", fmt.Sprintf("storm-db-test-%d-", port))
	if err != nil {
		t.Fatalf("Failed to create temporary database directory: %v", err)
	}
	dbPath := filepath.Join(tmpDir, "test.db")

	go func() {
		if err := serveRun(port, dbPath); err != nil {
			t.Logf("Daemon error on port %d: %v", port, err)
		}
	}()

	daemonURL := fmt.Sprintf("http://localhost:%d", port)

	// Wait for daemon to be ready with polling
	if err := waitForDaemon(daemonURL, 5*time.Second); err != nil {
		t.Logf("Warning: daemon may not be ready: %v", err)
	}

	// Return cleanup function that shuts down daemon and cleans up files
	cleanup := func() {
		// Call /stop endpoint to gracefully shut down daemon
		stopURL := fmt.Sprintf("%s/stop", daemonURL)
		resp, err := http.Post(stopURL, "application/json", nil)
		if err != nil {
			t.Logf("Error stopping daemon: %v", err)
		} else {
			resp.Body.Close()
		}

		// Wait a bit for daemon to shut down
		time.Sleep(500 * time.Millisecond)

		// Clean up temporary database directory
		os.RemoveAll(tmpDir)
	}

	return daemonURL, cleanup
}

// TestCLIProjectAdd tests the project add subcommand via shell execution
func TestCLIProjectAdd(t *testing.T) {
	daemonPort := 59998
	daemonURL, cleanup := startTestDaemon(t, daemonPort)
	defer cleanup()

	projectID := "cli-test-project"
	projectDir, markdownFile, cleanupProj := setupTestProject(t, projectID)
	defer cleanupProj()

	// Run project add command
	output, errOutput, err := runCLICommand(daemonURL, "project", "add", projectID, projectDir, markdownFile)
	if err != nil {
		t.Fatalf("project add command failed: %v, stderr: %s", err, errOutput)
	}

	if len(output) == 0 {
		t.Errorf("Expected project add output, got empty string")
	}
	if !bytes.Contains([]byte(output), []byte(projectID)) {
		t.Errorf("Expected project ID in output, got: %s", output)
	}
}

// TestCLIProjectList tests the project list subcommand via shell execution
func TestCLIProjectList(t *testing.T) {
	daemonPort := 59997
	daemonURL, cleanup := startTestDaemon(t, daemonPort)
	defer cleanup()

	projectID := "cli-list-project"
	projectDir, markdownFile, cleanupProj := setupTestProject(t, projectID)
	defer cleanupProj()

	_, errOutput, err := runCLICommand(daemonURL, "project", "add", projectID, projectDir, markdownFile)
	if err != nil {
		t.Fatalf("Failed to add test project: %v, stderr: %s", err, errOutput)
	}

	output, errOutput, err := runCLICommand(daemonURL, "project", "list")
	if err != nil {
		t.Fatalf("project list command failed: %v, stderr: %s", err, errOutput)
	}

	if !bytes.Contains([]byte(output), []byte(projectID)) {
		t.Errorf("Expected project ID in list output, got: %s", output)
	}
}

// TestCLIFileAdd tests the file add subcommand via shell execution
func TestCLIFileAdd(t *testing.T) {
	daemonPort := 59996
	daemonURL, cleanup := startTestDaemon(t, daemonPort)
	defer cleanup()

	projectID := "cli-file-test-project"
	projectDir, markdownFile, cleanupProj := setupTestProject(t, projectID)
	defer cleanupProj()

	inputFile := filepath.Join(projectDir, "input.csv")
	if err := ioutil.WriteFile(inputFile, []byte("col1,col2\nval1,val2\n"), 0644); err != nil {
		t.Fatalf("Failed to create input file: %v", err)
	}

	_, errOutput, err := runCLICommand(daemonURL, "project", "add", projectID, projectDir, markdownFile)
	if err != nil {
		t.Fatalf("Failed to add test project: %v, stderr: %s", err, errOutput)
	}

	output, errOutput, err := runCLICommand(daemonURL, "file", "add", "--project", projectID, inputFile)
	if err != nil {
		t.Fatalf("file add command failed: %v, stderr: %s", err, errOutput)
	}

	if len(output) == 0 {
		t.Errorf("Expected file add output, got empty string")
	}
}

// TestCLIFileList tests the file list subcommand via shell execution
func TestCLIFileList(t *testing.T) {
	daemonPort := 59995
	daemonURL, cleanup := startTestDaemon(t, daemonPort)
	defer cleanup()

	projectID := "cli-filelist-test-project"
	projectDir, markdownFile, cleanupProj := setupTestProject(t, projectID)
	defer cleanupProj()

	inputFile := filepath.Join(projectDir, "input.csv")
	if err := ioutil.WriteFile(inputFile, []byte("col1,col2\n"), 0644); err != nil {
		t.Fatalf("Failed to create input file: %v", err)
	}

	_, errOutput, err := runCLICommand(daemonURL, "project", "add", projectID, projectDir, markdownFile)
	if err != nil {
		t.Fatalf("Failed to add test project: %v, stderr: %s", err, errOutput)
	}

	_, errOutput, err = runCLICommand(daemonURL, "file", "add", "--project", projectID, inputFile)
	if err != nil {
		t.Fatalf("Failed to add test files: %v, stderr: %s", err, errOutput)
	}

	output, errOutput, err := runCLICommand(daemonURL, "file", "list", "--project", projectID)
	if err != nil {
		t.Fatalf("file list command failed: %v, stderr: %s", err, errOutput)
	}

	if len(output) == 0 {
		t.Errorf("Expected file list output, got empty string")
	}
}

// TestCLIFileForget tests the file forget subcommand
func TestCLIFileForget(t *testing.T) {
	daemonPort := 59994
	daemonURL, cleanup := startTestDaemon(t, daemonPort)
	defer cleanup()

	projectID := "cli-forget-test-project"
	projectDir, markdownFile, cleanupProj := setupTestProject(t, projectID)
	defer cleanupProj()

	inputFile := filepath.Join(projectDir, "input.csv")
	if err := ioutil.WriteFile(inputFile, []byte("col1,col2\n"), 0644); err != nil {
		t.Fatalf("Failed to create input file: %v", err)
	}

	_, errOutput, err := runCLICommand(daemonURL, "project", "add", projectID, projectDir, markdownFile)
	if err != nil {
		t.Fatalf("Failed to add test project: %v, stderr: %s", err, errOutput)
	}

	_, errOutput, err = runCLICommand(daemonURL, "file", "add", "--project", projectID, inputFile)
	if err != nil {
		t.Fatalf("Failed to add test file: %v, stderr: %s", err, errOutput)
	}

	// Forget using absolute path (inputFile is absolute)
	output, errOutput, err := runCLICommand(daemonURL, "file", "forget", "--project", projectID, inputFile)
	if err != nil {
		t.Fatalf("file forget command failed: %v, stderr: %s", err, errOutput)
	}

	if !bytes.Contains([]byte(output), []byte("forgotten")) {
		t.Errorf("Expected 'forgotten' in output, got: %s", output)
	}

	// Verify file is no longer listed
	listOutput, errOutput, err := runCLICommand(daemonURL, "file", "list", "--project", projectID)
	if err != nil {
		t.Fatalf("file list command failed: %v, stderr: %s", err, errOutput)
	}
	if bytes.Contains([]byte(listOutput), []byte("input.csv")) {
		t.Errorf("Expected file to be absent from list output, got: %s", listOutput)
	}
}

// TestCLIProjectForget tests the project forget subcommand
func TestCLIProjectForget(t *testing.T) {
	daemonPort := 59993
	daemonURL, cleanup := startTestDaemon(t, daemonPort)
	defer cleanup()

	projectID := "cli-forget-project-test"
	projectDir, markdownFile, cleanupProj := setupTestProject(t, projectID)
	defer cleanupProj()

	_, errOutput, err := runCLICommand(daemonURL, "project", "add", projectID, projectDir, markdownFile)
	if err != nil {
		t.Fatalf("Failed to add test project: %v, stderr: %s", err, errOutput)
	}

	output, errOutput, err := runCLICommand(daemonURL, "project", "forget", projectID)
	if err != nil {
		t.Fatalf("project forget command failed: %v, stderr: %s", err, errOutput)
	}

	if !bytes.Contains([]byte(output), []byte("forgotten")) {
		t.Errorf("Expected 'forgotten' in output, got: %s", output)
	}

	// Verify project is no longer listed
	listOutput, errOutput, err := runCLICommand(daemonURL, "project", "list")
	if err != nil {
		t.Fatalf("project list command failed: %v, stderr: %s", err, errOutput)
	}

	if bytes.Contains([]byte(listOutput), []byte(projectID)) {
		t.Errorf("Expected project ID to be absent from list output, got: %s", listOutput)
	}

}

// TestCLIStop tests the stop subcommand[1]
func TestCLIStop(t *testing.T) {
	daemonPort := 59992
	daemonURL, cleanup := startTestDaemon(t, daemonPort)
	defer cleanup()

	output, errOutput, err := runCLICommand(daemonURL, "stop")
	if err != nil {
		t.Fatalf("stop command failed: %v, stderr: %s", err, errOutput)
	}

	if !bytes.Contains([]byte(output), []byte("stopped")) {
		t.Errorf("Expected 'stopped' in output, got: %s", output)
	}

	// Verify daemon is actually stopped by attempting to connect
	time.Sleep(500 * time.Millisecond)
	resp, err := http.Get(daemonURL)
	if err == nil {
		resp.Body.Close()
		t.Errorf("Expected daemon to be stopped, but it's still running")
	}
}

// TestCLIFileAddMissingProjectFlag tests that file add fails without --project flag
func TestCLIFileAddMissingProjectFlag(t *testing.T) {
	_, errOutput, err := runCLICommand("http://localhost:8080", "file", "add", "somefile.txt")
	if err == nil {
		t.Errorf("Expected error for missing --project flag, but command succeeded")
	}

	if len(errOutput) == 0 {
		t.Logf("Command exited with error code but no stderr output")
	}
}

// TestCLIFileListMissingProjectFlag tests that file list fails without --project flag
func TestCLIFileListMissingProjectFlag(t *testing.T) {
	_, errOutput, err := runCLICommand("http://localhost:8080", "file", "list")
	if err == nil {
		t.Errorf("Expected error for missing --project flag, but command succeeded")
	}

	if len(errOutput) == 0 {
		t.Logf("Command exited with error code but no stderr output")
	}
}
