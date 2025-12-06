package main

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

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

// Helper function to start a test daemon and return its URL
func startTestDaemon(t *testing.T, port int) string {
	go func() {
		if err := serveRun(port); err != nil {
			t.Logf("Daemon error on port %d: %v", port, err)
		}
	}()

	// Wait for daemon to start
	time.Sleep(2 * time.Second)
	return fmt.Sprintf("http://localhost:%d", port)
}

// TestCLIProjectAdd tests the project add subcommand via shell execution
func TestCLIProjectAdd(t *testing.T) {
	daemonPort := 59998
	daemonURL := startTestDaemon(t, daemonPort)

	projectID := "cli-test-project"
	projectDir, markdownFile, cleanup := setupTestProject(t, projectID)
	defer cleanup()

	// Execute project add command via 'go run .'
	cmd := exec.Command("go", "run", ".", "project", "add", projectID, projectDir, markdownFile)
	cmd.Env = append(os.Environ(), fmt.Sprintf("STORM_DAEMON_URL=%s", daemonURL))

	var outBuf, errBuf bytes.Buffer
	cmd.Stdout = &outBuf
	cmd.Stderr = &errBuf

	if err := cmd.Run(); err != nil {
		t.Fatalf("project add command failed: %v, stderr: %s", err, errBuf.String())
	}

	output := outBuf.String()
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
	daemonURL := startTestDaemon(t, daemonPort)

	projectID := "cli-list-project"
	projectDir, markdownFile, cleanup := setupTestProject(t, projectID)
	defer cleanup()

	// Add project via CLI
	cmd := exec.Command("go", "run", ".", "project", "add", projectID, projectDir, markdownFile)
	cmd.Env = append(os.Environ(), fmt.Sprintf("STORM_DAEMON_URL=%s", daemonURL))
	if err := cmd.Run(); err != nil {
		t.Fatalf("Failed to add test project: %v", err)
	}

	// List projects via CLI
	cmd = exec.Command("go", "run", ".", "project", "list")
	cmd.Env = append(os.Environ(), fmt.Sprintf("STORM_DAEMON_URL=%s", daemonURL))

	var outBuf, errBuf bytes.Buffer
	cmd.Stdout = &outBuf
	cmd.Stderr = &errBuf

	if err := cmd.Run(); err != nil {
		t.Fatalf("project list command failed: %v, stderr: %s", err, errBuf.String())
	}

	output := outBuf.String()
	if !bytes.Contains([]byte(output), []byte(projectID)) {
		t.Errorf("Expected project ID in list output, got: %s", output)
	}
}

// TestCLIFileAdd tests the file add subcommand via shell execution
func TestCLIFileAdd(t *testing.T) {
	daemonPort := 59996
	daemonURL := startTestDaemon(t, daemonPort)

	projectID := "cli-file-test-project"
	projectDir, markdownFile, cleanup := setupTestProject(t, projectID)
	defer cleanup()

	// Create test input file
	inputFile := filepath.Join(projectDir, "input.csv")
	if err := ioutil.WriteFile(inputFile, []byte("col1,col2\nval1,val2\n"), 0644); err != nil {
		t.Fatalf("Failed to create input file: %v", err)
	}

	// Add project via CLI
	cmd := exec.Command("go", "run", ".", "project", "add", projectID, projectDir, markdownFile)
	cmd.Env = append(os.Environ(), fmt.Sprintf("STORM_DAEMON_URL=%s", daemonURL))
	if err := cmd.Run(); err != nil {
		t.Fatalf("Failed to add test project: %v", err)
	}

	// Add files via CLI
	cmd = exec.Command("go", "run", ".", "file", "add", "--project", projectID, inputFile)
	cmd.Env = append(os.Environ(), fmt.Sprintf("STORM_DAEMON_URL=%s", daemonURL))

	var outBuf, errBuf bytes.Buffer
	cmd.Stdout = &outBuf
	cmd.Stderr = &errBuf

	if err := cmd.Run(); err != nil {
		t.Fatalf("file add command failed: %v, stderr: %s", err, errBuf.String())
	}

	output := outBuf.String()
	if len(output) == 0 {
		t.Errorf("Expected file add output, got empty string")
	}
}

// TestCLIFileList tests the file list subcommand via shell execution
func TestCLIFileList(t *testing.T) {
	daemonPort := 59995
	daemonURL := startTestDaemon(t, daemonPort)

	projectID := "cli-filelist-test-project"
	projectDir, markdownFile, cleanup := setupTestProject(t, projectID)
	defer cleanup()

	// Create test input file
	inputFile := filepath.Join(projectDir, "input.csv")
	if err := ioutil.WriteFile(inputFile, []byte("col1,col2\n"), 0644); err != nil {
		t.Fatalf("Failed to create input file: %v", err)
	}

	// Add project via CLI
	cmd := exec.Command("go", "run", ".", "project", "add", projectID, projectDir, markdownFile)
	cmd.Env = append(os.Environ(), fmt.Sprintf("STORM_DAEMON_URL=%s", daemonURL))
	if err := cmd.Run(); err != nil {
		t.Fatalf("Failed to add test project: %v", err)
	}

	// Add files via CLI
	cmd = exec.Command("go", "run", ".", "file", "add", "--project", projectID, inputFile)
	cmd.Env = append(os.Environ(), fmt.Sprintf("STORM_DAEMON_URL=%s", daemonURL))
	if err := cmd.Run(); err != nil {
		t.Fatalf("Failed to add test files: %v", err)
	}

	// List files via CLI
	cmd = exec.Command("go", "run", ".", "file", "list", "--project", projectID)
	cmd.Env = append(os.Environ(), fmt.Sprintf("STORM_DAEMON_URL=%s", daemonURL))

	var outBuf, errBuf bytes.Buffer
	cmd.Stdout = &outBuf
	cmd.Stderr = &errBuf

	if err := cmd.Run(); err != nil {
		t.Fatalf("file list command failed: %v, stderr: %s", err, errBuf.String())
	}

	output := outBuf.String()
	if len(output) == 0 {
		t.Errorf("Expected file list output, got empty string")
	}
}

// TestCLIFileAddMissingProjectFlag tests that file add fails without --project flag
func TestCLIFileAddMissingProjectFlag(t *testing.T) {
	cmd := exec.Command("go", "run", ".", "file", "add", "somefile.txt")

	var errBuf bytes.Buffer
	cmd.Stderr = &errBuf

	err := cmd.Run()
	if err == nil {
		t.Errorf("Expected error for missing --project flag, but command succeeded")
	}

	errOutput := errBuf.String()
	if len(errOutput) == 0 {
		t.Logf("Command exited with error code but no stderr output")
	}
}

// TestCLIFileListMissingProjectFlag tests that file list fails without --project flag
func TestCLIFileListMissingProjectFlag(t *testing.T) {
	cmd := exec.Command("go", "run", ".", "file", "list")

	var errBuf bytes.Buffer
	cmd.Stderr = &errBuf

	err := cmd.Run()
	if err == nil {
		t.Errorf("Expected error for missing --project flag, but command succeeded")
	}

	errOutput := errBuf.String()
	if len(errOutput) == 0 {
		t.Logf("Command exited with error code but no stderr output")
	}
}
