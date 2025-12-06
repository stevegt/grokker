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
	daemonURL := startTestDaemon(t, daemonPort)

	projectID := "cli-list-project"
	projectDir, markdownFile, cleanup := setupTestProject(t, projectID)
	defer cleanup()

	_, _, err := runCLICommand(daemonURL, "project", "add", projectID, projectDir, markdownFile)
	if err != nil {
		t.Fatalf("Failed to add test project: %v", err)
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
	daemonURL := startTestDaemon(t, daemonPort)

	projectID := "cli-file-test-project"
	projectDir, markdownFile, cleanup := setupTestProject(t, projectID)
	defer cleanup()

	inputFile := filepath.Join(projectDir, "input.csv")
	if err := ioutil.WriteFile(inputFile, []byte("col1,col2\nval1,val2\n"), 0644); err != nil {
		t.Fatalf("Failed to create input file: %v", err)
	}

	_, _, err := runCLICommand(daemonURL, "project", "add", projectID, projectDir, markdownFile)
	if err != nil {
		t.Fatalf("Failed to add test project: %v", err)
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
	daemonURL := startTestDaemon(t, daemonPort)

	projectID := "cli-filelist-test-project"
	projectDir, markdownFile, cleanup := setupTestProject(t, projectID)
	defer cleanup()

	inputFile := filepath.Join(projectDir, "input.csv")
	if err := ioutil.WriteFile(inputFile, []byte("col1,col2\n"), 0644); err != nil {
		t.Fatalf("Failed to create input file: %v", err)
	}

	_, _, err := runCLICommand(daemonURL, "project", "add", projectID, projectDir, markdownFile)
	if err != nil {
		t.Fatalf("Failed to add test project: %v", err)
	}

	_, _, err = runCLICommand(daemonURL, "file", "add", "--project", projectID, inputFile)
	if err != nil {
		t.Fatalf("Failed to add test files: %v", err)
	}

	output, errOutput, err := runCLICommand(daemonURL, "file", "list", "--project", projectID)
	if err != nil {
		t.Fatalf("file list command failed: %v, stderr: %s", err, errOutput)
	}

	if len(output) == 0 {
		t.Errorf("Expected file list output, got empty string")
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
