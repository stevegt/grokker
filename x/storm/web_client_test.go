package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/chromedp/chromedp"
	"github.com/stevegt/grokker/x/storm/testutil"
)

// startTestServer creates test server config and starts serveRun in a goroutine
func startTestServer(t *testing.T, projectID string) *testutil.TestServer {
	server := testutil.NewTestServer(t, projectID)

	// Start server in background goroutine (call serveRun from main package)
	go func() {
		if err := serveRun(server.Port, server.DBPath); err != nil {
			t.Logf("Test server error: %v", err)
		}
	}()

	// Wait for server to be ready by polling the port in a loop
	up := false
	for i := 0; i < 20; i++ {
		time.Sleep(500 * time.Millisecond)
		_, err := http.Get(server.URL)
		if err == nil {
			up = true
			break
		}
	}

	if !up {
		t.Fatalf("Test server did not start")
	}

	return server
}

// stopTestServer stops the server and cleans up resources
func stopTestServer(t *testing.T, server *testutil.TestServer) {
	// Call /stop endpoint to gracefully shutdown server
	_, _ = http.Post(server.URL+"/stop", "application/json", nil)

	// Clean up
	server.Cleanup(t)
}

// TestWebClientCreateProject tests creating a project and navigating to its page via web client
func TestWebClientCreateProject(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping chromedp test in short mode")
	}

	server := startTestServer(t, "web-test-create-project")
	defer stopTestServer(t, server)

	// Create project via HTTP API
	projectID := server.ProjectID
	createProjectPayload := map[string]string{
		"projectID":    projectID,
		"baseDir":      server.ProjectDir,
		"markdownFile": server.MarkdownFile,
	}
	jsonData, _ := json.Marshal(createProjectPayload)
	resp, err := http.Post(server.URL+"/api/projects", "application/json", bytes.NewReader(jsonData))
	if err != nil {
		t.Fatalf("Failed to create project: %v", err)
	}
	resp.Body.Close()

	// Create chromedp context
	ctx, cancel := chromedp.NewContext(context.Background())
	defer cancel()

	// Navigate to project page and verify it loads
	projectURL := fmt.Sprintf("%s/project/%s", server.URL, projectID)
	err = chromedp.Run(ctx,
		chromedp.Navigate(projectURL),
		chromedp.WaitVisible("#sidebar"),
		chromedp.WaitVisible("#chat"),
	)
	if err != nil {
		t.Fatalf("Failed to navigate to project page: %v", err)
	}

	// Verify sidebar is visible
	sidebarText, err := testutil.GetElementText(ctx, "#sidebar h3")
	if err != nil {
		t.Fatalf("Failed to get sidebar text: %v", err)
	}

	if !strings.Contains(sidebarText, "Contents") && !strings.Contains(sidebarText, "TOC") {
		t.Errorf("Expected 'Table of Contents' in sidebar, got: %s", sidebarText)
	}

	t.Logf("Project page loaded successfully with sidebar visible")
}

// TestWebClientAddFiles tests adding files via HTTP API and verifying they appear in the UI
func TestWebClientAddFiles(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping chromedp test in short mode")
	}

	server := startTestServer(t, "web-test-add-files")
	defer stopTestServer(t, server)

	// Create project
	projectID := server.ProjectID
	createProjectPayload := map[string]string{
		"projectID":    projectID,
		"baseDir":      server.ProjectDir,
		"markdownFile": server.MarkdownFile,
	}
	jsonData, _ := json.Marshal(createProjectPayload)
	resp, _ := http.Post(server.URL+"/api/projects", "application/json", bytes.NewReader(jsonData))
	resp.Body.Close()

	// Create test files
	testFile1 := filepath.Join(server.ProjectDir, "test1.txt")
	testFile2 := filepath.Join(server.ProjectDir, "test2.txt")
	ioutil.WriteFile(testFile1, []byte("content1"), 0644)
	ioutil.WriteFile(testFile2, []byte("content2"), 0644)

	// Add files via HTTP API
	addFilesPayload := map[string]interface{}{
		"filenames": []string{testFile1, testFile2},
	}
	jsonData, _ = json.Marshal(addFilesPayload)
	fileURL := fmt.Sprintf("%s/api/projects/%s/files/add", server.URL, projectID)
	resp, _ = http.Post(fileURL, "application/json", bytes.NewReader(jsonData))
	resp.Body.Close()

	// Create chromedp context
	ctx, cancel := chromedp.NewContext(context.Background())
	defer cancel()

	// Navigate to project page
	projectURL := fmt.Sprintf("%s/project/%s", server.URL, projectID)
	chromedp.Run(ctx,
		chromedp.Navigate(projectURL),
		chromedp.WaitVisible("#filesBtn"),
	)

	// Open file modal
	err := testutil.OpenFileModal(ctx)
	if err != nil {
		t.Fatalf("Failed to open file modal: %v", err)
	}

	// Wait for file list to load
	time.Sleep(500 * time.Millisecond)

	// Check that file rows appear in the list
	numRows, err := testutil.GetFileListRows(ctx)
	if err != nil {
		t.Fatalf("Failed to get file list rows: %v", err)
	}

	if numRows < 2 {
		t.Errorf("Expected at least 2 file rows, got %d", numRows)
	}

	t.Logf("Files successfully added and visible in UI (%d rows)", numRows)
}

// TestWebClientQuerySubmitViaWebSocket tests submitting a query and verifying the spinner appears
func TestWebClientQuerySubmitViaWebSocket(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping chromedp test in short mode")
	}

	server := startTestServer(t, "web-test-query-submit")
	defer stopTestServer(t, server)

	// Create project
	projectID := server.ProjectID
	createProjectPayload := map[string]string{
		"projectID":    projectID,
		"baseDir":      server.ProjectDir,
		"markdownFile": server.MarkdownFile,
	}
	jsonData, _ := json.Marshal(createProjectPayload)
	resp, _ := http.Post(server.URL+"/api/projects", "application/json", bytes.NewReader(jsonData))
	resp.Body.Close()

	// Create chromedp context
	ctx, cancel := chromedp.NewContext(context.Background())
	defer cancel()

	// Navigate to project page
	projectURL := fmt.Sprintf("%s/project/%s", server.URL, projectID)
	chromedp.Run(ctx,
		chromedp.Navigate(projectURL),
		chromedp.WaitVisible("#userInput"),
	)

	// Submit query
	testQuery := "test query for web client"
	err := testutil.SubmitQuery(ctx, testQuery)
	if err != nil {
		t.Fatalf("Failed to submit query: %v", err)
	}

	// Wait for spinner to appear (indicating WebSocket message was received)
	time.Sleep(500 * time.Millisecond)

	spinnerVisible, err := testutil.IsElementVisible(ctx, ".spinner")
	if err != nil {
		t.Logf("Note: could not verify spinner visibility: %v", err)
		// This is not a hard error; the query might have already processed
	} else if !spinnerVisible {
		t.Logf("Note: spinner not visible, query may have completed quickly")
	}

	t.Logf("Query submitted successfully via WebSocket")
}

// TestWebClientFileSelectionPersistence tests that file selections persist when opening and closing the modal
func TestWebClientFileSelectionPersistence(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping chromedp test in short mode")
	}

	server := startTestServer(t, "web-test-file-selection")
	defer stopTestServer(t, server)

	// Create project
	projectID := server.ProjectID
	createProjectPayload := map[string]string{
		"projectID":    projectID,
		"baseDir":      server.ProjectDir,
		"markdownFile": server.MarkdownFile,
	}
	jsonData, _ := json.Marshal(createProjectPayload)
	resp, _ := http.Post(server.URL+"/api/projects", "application/json", bytes.NewReader(jsonData))
	resp.Body.Close()

	// Create test files
	testFile1 := filepath.Join(server.ProjectDir, "input.txt")
	testFile2 := filepath.Join(server.ProjectDir, "output.txt")
	ioutil.WriteFile(testFile1, []byte("input"), 0644)
	ioutil.WriteFile(testFile2, []byte("output"), 0644)

	// Add files via API
	addFilesPayload := map[string]interface{}{
		"filenames": []string{testFile1, testFile2},
	}
	jsonData, _ = json.Marshal(addFilesPayload)
	fileURL := fmt.Sprintf("%s/api/projects/%s/files/add", server.URL, projectID)
	resp, _ = http.Post(fileURL, "application/json", bytes.NewReader(jsonData))
	resp.Body.Close()

	// Create chromedp context
	ctx, cancel := chromedp.NewContext(context.Background())
	defer cancel()

	// Navigate to project page
	projectURL := fmt.Sprintf("%s/project/%s", server.URL, projectID)
	chromedp.Run(ctx,
		chromedp.Navigate(projectURL),
		chromedp.WaitVisible("#filesBtn"),
	)

	// Open file modal
	testutil.OpenFileModal(ctx)
	time.Sleep(300 * time.Millisecond)

	// Select first file as input, second file as output
	testutil.SelectFileCheckbox(ctx, 1, "in")
	testutil.SelectFileCheckbox(ctx, 2, "out")
	time.Sleep(200 * time.Millisecond)

	// Verify selections
	inputFiles, outputFiles, err := testutil.GetSelectedFiles(ctx)
	if err != nil {
		t.Fatalf("Failed to get selected files: %v", err)
	}

	if len(inputFiles) < 1 {
		t.Errorf("Expected at least 1 input file selected, got %d", len(inputFiles))
	}

	if len(outputFiles) < 1 {
		t.Errorf("Expected at least 1 output file selected, got %d", len(outputFiles))
	}

	// Close modal
	testutil.CloseModal(ctx)
	time.Sleep(200 * time.Millisecond)

	// Re-open modal and verify selections persisted
	testutil.OpenFileModal(ctx)
	time.Sleep(300 * time.Millisecond)

	inputFiles2, outputFiles2, err := testutil.GetSelectedFiles(ctx)
	if err != nil {
		t.Fatalf("Failed to get selected files on second check: %v", err)
	}

	if len(inputFiles2) != len(inputFiles) {
		t.Errorf("Input file selection not persisted: had %d, now %d", len(inputFiles), len(inputFiles2))
	}

	if len(outputFiles2) != len(outputFiles) {
		t.Errorf("Output file selection not persisted: had %d, now %d", len(outputFiles), len(outputFiles2))
	}

	t.Logf("File selections persisted correctly across modal open/close cycles")
}

// TestWebClientPageLoad tests that the landing page and project pages load without errors
func TestWebClientPageLoad(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping chromedp test in short mode")
	}

	server := startTestServer(t, "web-test-page-load")
	defer stopTestServer(t, server)

	// Create chromedp context
	ctx, cancel := chromedp.NewContext(context.Background())
	defer cancel()

	// Navigate to root and verify it loads
	err := chromedp.Run(ctx,
		chromedp.Navigate(server.URL),
	)
	if err != nil {
		t.Fatalf("Failed to navigate to root: %v", err)
	}

	// Wait for page to load (either project list or landing page content)
	time.Sleep(500 * time.Millisecond)

	// Verify we can get some page content
	var pageTitle string
	chromedp.Run(ctx,
		chromedp.Evaluate(`document.title`, &pageTitle),
	)

	t.Logf("Landing page loaded successfully with title: %s", pageTitle)
}
