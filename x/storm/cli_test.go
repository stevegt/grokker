package main

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// TestCLIProjectAdd tests the project add subcommand
func TestCLIProjectAdd(t *testing.T) {
	// Create temporary project directory
	tmpDir, err := ioutil.TempDir("", "storm-cli-test-")
	if err != nil {
		t.Fatalf("Failed to create temporary directory: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create project subdirectory and markdown file
	projectID := "cli-test-project"
	projectDir := filepath.Join(tmpDir, projectID)
	if err := os.MkdirAll(projectDir, 0755); err != nil {
		t.Fatalf("Failed to create project directory: %v", err)
	}

	markdownFile := filepath.Join(projectDir, "chat.md")
	if err := ioutil.WriteFile(markdownFile, []byte(""), 0644); err != nil {
		t.Fatalf("Failed to create markdown file: %v", err)
	}

	// Start daemon in background
	daemonPort := 59998
	go func() {
		if err := serveRun(daemonPort); err != nil {
			t.Logf("Daemon error: %v", err)
		}
	}()

	// Wait for daemon to start
	time.Sleep(2 * time.Second)

	// Set daemon URL for CLI
	os.Setenv("STORM_DAEMON_URL", fmt.Sprintf("http://localhost:%d", daemonPort))

	// Create and execute project add command
	rootCmd := createTestRootCmd()
	projectCmd := rootCmd.Commands() // serve
	for _, cmd := range rootCmd.Commands() {
		if cmd.Use == "project" {
			projectCmd = cmd
			break
		}
	}

	var outBuf bytes.Buffer
	var errBuf bytes.Buffer
	rootCmd.SetOut(&outBuf)
	rootCmd.SetErr(&errBuf)
	rootCmd.SetArgs([]string{"project", "add", projectID, projectDir, markdownFile})

	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("project add command failed: %v", err)
	}

	output := outBuf.String()
	if len(output) == 0 {
		t.Errorf("Expected project add output, got empty string")
	}
	if !bytes.Contains(outBuf.Bytes(), []byte(projectID)) {
		t.Errorf("Expected project ID in output, got: %s", output)
	}
}

// TestCLIProjectList tests the project list subcommand
func TestCLIProjectList(t *testing.T) {
	// Create temporary project directory
	tmpDir, err := ioutil.TempDir("", "storm-cli-test-list-")
	if err != nil {
		t.Fatalf("Failed to create temporary directory: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create two projects
	for i := 1; i <= 2; i++ {
		projectID := fmt.Sprintf("cli-list-project-%d", i)
		projectDir := filepath.Join(tmpDir, projectID)
		if err := os.MkdirAll(projectDir, 0755); err != nil {
			t.Fatalf("Failed to create project directory: %v", err)
		}

		markdownFile := filepath.Join(projectDir, "chat.md")
		if err := ioutil.WriteFile(markdownFile, []byte(""), 0644); err != nil {
			t.Fatalf("Failed to create markdown file: %v", err)
		}
	}

	// Start daemon in background
	daemonPort := 59997
	go func() {
		if err := serveRun(daemonPort); err != nil {
			t.Logf("Daemon error: %v", err)
		}
	}()

	// Wait for daemon to start
	time.Sleep(2 * time.Second)

	// Set daemon URL for CLI
	os.Setenv("STORM_DAEMON_URL", fmt.Sprintf("http://localhost:%d", daemonPort))

	// Add projects via API first
	projectID1 := "cli-list-project-1"
	projectDir1 := filepath.Join(tmpDir, projectID1)
	markdownFile1 := filepath.Join(projectDir1, "chat.md")

	rootCmd := createTestRootCmd()
	var outBuf bytes.Buffer
	rootCmd.SetOut(&outBuf)
	rootCmd.SetArgs([]string{"project", "add", projectID1, projectDir1, markdownFile1})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("Failed to add test project: %v", err)
	}

	// Now test list command
	outBuf.Reset()
	rootCmd = createTestRootCmd()
	rootCmd.SetOut(&outBuf)
	rootCmd.SetArgs([]string{"project", "list"})

	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("project list command failed: %v", err)
	}

	output := outBuf.String()
	if !bytes.Contains(outBuf.Bytes(), []byte(projectID1)) {
		t.Errorf("Expected project ID in list output, got: %s", output)
	}
}

// TestCLIFileAdd tests the file add subcommand
func TestCLIFileAdd(t *testing.T) {
	// Create temporary project directory
	tmpDir, err := ioutil.TempDir("", "storm-cli-test-files-")
	if err != nil {
		t.Fatalf("Failed to create temporary directory: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	projectID := "cli-file-test-project"
	projectDir := filepath.Join(tmpDir, projectID)
	if err := os.MkdirAll(projectDir, 0755); err != nil {
		t.Fatalf("Failed to create project directory: %v", err)
	}

	markdownFile := filepath.Join(projectDir, "chat.md")
	if err := ioutil.WriteFile(markdownFile, []byte(""), 0644); err != nil {
		t.Fatalf("Failed to create markdown file: %v", err)
	}

	// Create test files
	inputFile := filepath.Join(projectDir, "input.csv")
	if err := ioutil.WriteFile(inputFile, []byte("col1,col2\nval1,val2\n"), 0644); err != nil {
		t.Fatalf("Failed to create input file: %v", err)
	}

	// Start daemon in background
	daemonPort := 59996
	go func() {
		if err := serveRun(daemonPort); err != nil {
			t.Logf("Daemon error: %v", err)
		}
	}()

	// Wait for daemon to start
	time.Sleep(2 * time.Second)

	// Set daemon URL for CLI
	os.Setenv("STORM_DAEMON_URL", fmt.Sprintf("http://localhost:%d", daemonPort))

	// Add project via API first
	rootCmd := createTestRootCmd()
	var outBuf bytes.Buffer
	rootCmd.SetOut(&outBuf)
	rootCmd.SetArgs([]string{"project", "add", projectID, projectDir, markdownFile})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("Failed to add test project: %v", err)
	}

	// Now test file add command
	outBuf.Reset()
	rootCmd = createTestRootCmd()
	rootCmd.SetOut(&outBuf)
	rootCmd.SetArgs([]string{"file", "add", "--project", projectID, inputFile})

	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("file add command failed: %v", err)
	}

	output := outBuf.String()
	if len(output) == 0 {
		t.Errorf("Expected file add output, got empty string")
	}
}

// TestCLIFileList tests the file list subcommand
func TestCLIFileList(t *testing.T) {
	// Create temporary project directory
	tmpDir, err := ioutil.TempDir("", "storm-cli-test-filelist-")
	if err != nil {
		t.Fatalf("Failed to create temporary directory: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	projectID := "cli-filelist-test-project"
	projectDir := filepath.Join(tmpDir, projectID)
	if err := os.MkdirAll(projectDir, 0755); err != nil {
		t.Fatalf("Failed to create project directory: %v", err)
	}

	markdownFile := filepath.Join(projectDir, "chat.md")
	if err := ioutil.WriteFile(markdownFile, []byte(""), 0644); err != nil {
		t.Fatalf("Failed to create markdown file: %v", err)
	}

	// Create test files
	inputFile := filepath.Join(projectDir, "input.csv")
	if err := ioutil.WriteFile(inputFile, []byte("col1,col2\n"), 0644); err != nil {
		t.Fatalf("Failed to create input file: %v", err)
	}

	// Start daemon in background
	daemonPort := 59995
	go func() {
		if err := serveRun(daemonPort); err != nil {
			t.Logf("Daemon error: %v", err)
		}
	}()

	// Wait for daemon to start
	time.Sleep(2 * time.Second)

	// Set daemon URL for CLI
	os.Setenv("STORM_DAEMON_URL", fmt.Sprintf("http://localhost:%d", daemonPort))

	// Add project via API
	rootCmd := createTestRootCmd()
	var outBuf bytes.Buffer
	rootCmd.SetOut(&outBuf)
	rootCmd.SetArgs([]string{"project", "add", projectID, projectDir, markdownFile})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("Failed to add test project: %v", err)
	}

	// Add files via API
	outBuf.Reset()
	rootCmd = createTestRootCmd()
	rootCmd.SetOut(&outBuf)
	rootCmd.SetArgs([]string{"file", "add", "--project", projectID, inputFile})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("Failed to add test files: %v", err)
	}

	// Test file list command
	outBuf.Reset()
	rootCmd = createTestRootCmd()
	rootCmd.SetOut(&outBuf)
	rootCmd.SetArgs([]string{"file", "list", "--project", projectID})

	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("file list command failed: %v", err)
	}

	output := outBuf.String()
	if len(output) == 0 {
		t.Errorf("Expected file list output, got empty string")
	}
}

// TestCLIFileAddMissingProjectFlag tests that file add fails without --project flag
func TestCLIFileAddMissingProjectFlag(t *testing.T) {
	rootCmd := createTestRootCmd()
	var outBuf bytes.Buffer
	var errBuf bytes.Buffer
	rootCmd.SetOut(&outBuf)
	rootCmd.SetErr(&errBuf)
	rootCmd.SetArgs([]string{"file", "add", "somefile.txt"})

	err := rootCmd.Execute()
	if err == nil {
		t.Errorf("Expected error for missing --project flag, but command succeeded")
	}
}

// TestCLIFileListMissingProjectFlag tests that file list fails without --project flag
func TestCLIFileListMissingProjectFlag(t *testing.T) {
	rootCmd := createTestRootCmd()
	var outBuf bytes.Buffer
	var errBuf bytes.Buffer
	rootCmd.SetOut(&outBuf)
	rootCmd.SetErr(&errBuf)
	rootCmd.SetArgs([]string{"file", "list"})

	err := rootCmd.Execute()
	if err == nil {
		t.Errorf("Expected error for missing --project flag, but command succeeded")
	}
}

// createTestRootCmd creates a fresh root command for testing
// This duplicates the command creation from main() to allow clean test isolation
func createTestRootCmd() *cobra.Command {
	// This would need to be extracted as a function in main.go
	// For now, we'll need to refactor main() to support this
	// Placeholder - in real implementation, call the function that creates rootCmd
	rootCmd := &cobra.Command{
		Use:   "storm",
		Short: "Storm - Multi-project LLM chat application",
	}
	return rootCmd
}

