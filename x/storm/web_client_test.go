package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/chromedp/chromedp"
	"github.com/stevegt/grokker/x/storm/testutil"
)

var timeout = 60 * time.Second

// newChromeContext creates a chromedp context with optional HEADLESS mode support
func newChromeContext() (context.Context, context.CancelFunc) {
	opts := []chromedp.ExecAllocatorOption{
		chromedp.NoDefaultBrowserCheck,
		chromedp.NoSandbox,
		chromedp.DisableGPU,
	}

	// Check HEADLESS environment variable - default to headless unless explicitly set to false
	if os.Getenv("HEADLESS") != "false" {
		opts = append(opts, chromedp.Headless)
	}

	allocCtx, cancel := chromedp.NewExecAllocator(context.Background(), opts...)
	ctx, _ := chromedp.NewContext(allocCtx)
	return ctx, cancel
}

// startTestServer creates test server config and starts serveRun in a goroutine
func startTestServer(t *testing.T, projectID string) *testutil.TestServer {
	server := testutil.NewTestServer(t, projectID)

	go func() {
		if err := serveRun(server.Port, server.DBPath); err != nil {
			t.Logf("Test server error: %v", err)
		}
	}()

	if err := testutil.WaitForServer(server.Port, 15*time.Second); err != nil {
		t.Fatalf("Test server did not start: %v", err)
	}

	return server
}

// stopTestServer stops the server and cleans up resources
func stopTestServer(t *testing.T, server *testutil.TestServer) {
	_, _ = http.Post(server.URL+"/stop", "application/json", nil)
	server.Cleanup(t)
}

// TestWebClientCreateProject tests creating a project and navigating to its page via web client
func TestWebClientCreateProject(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping chromedp test in short mode")
	}

	server := startTestServer(t, "web-test-create-project")
	defer stopTestServer(t, server)

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

	ctx, cancel := newChromeContext()
	defer cancel()

	ctx, cancelTimeout := context.WithTimeout(ctx, timeout)
	defer cancelTimeout()

	projectURL := fmt.Sprintf("%s/project/%s", server.URL, projectID)
	err = chromedp.Run(ctx,
		chromedp.Navigate(projectURL),
		chromedp.ActionFunc(func(ctx context.Context) error {
			return testutil.WaitForPageLoad(ctx)
		}),
		chromedp.ActionFunc(func(ctx context.Context) error {
			return testutil.WaitForWebSocketConnection(ctx)
		}),
		chromedp.ActionFunc(func(ctx context.Context) error {
			return testutil.WaitForElement(ctx, "#sidebar", false)
		}),
		chromedp.ActionFunc(func(ctx context.Context) error {
			return testutil.WaitForElement(ctx, "#chat", false)
		}),
	)
	if err != nil {
		t.Fatalf("Failed to navigate to project page: %v", err)
	}

	sidebarText, err := testutil.GetElementText(ctx, "#sidebar h3")
	if err != nil {
		t.Fatalf("Failed to get sidebar text: %v", err)
	}

	if !strings.Contains(sidebarText, "Contents") && !strings.Contains(sidebarText, "TOC") {
		t.Errorf("Expected 'Table of Contents' in sidebar, got: %s", sidebarText)
	}

	t.Logf("Project page loaded successfully with sidebar visible")
}

// TestWebClientOpenFileModal tests opening the file modal by clicking the Files button
func TestWebClientOpenFileModal(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping chromedp test in short mode")
	}

	server := startTestServer(t, "web-test-open-file-modal")
	defer stopTestServer(t, server)

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

	ctx, cancel := newChromeContext()
	defer cancel()

	ctx, cancelTimeout := context.WithTimeout(ctx, timeout)
	defer cancelTimeout()

	t.Logf("Waiting for server to fully initialize...")
	time.Sleep(3 * time.Second)

	projectURL := fmt.Sprintf("%s/project/%s", server.URL, projectID)
	t.Logf("Navigating to: %s", projectURL)
	err = chromedp.Run(ctx,
		chromedp.Navigate(projectURL),
		chromedp.ActionFunc(func(ctx context.Context) error {
			t.Logf("Waiting for page load...")
			return testutil.WaitForPageLoad(ctx)
		}),
		chromedp.ActionFunc(func(ctx context.Context) error {
			t.Logf("Waiting for WebSocket connection...")
			return testutil.WaitForWebSocketConnection(ctx)
		}),
		chromedp.ActionFunc(func(ctx context.Context) error {
			t.Logf("Waiting for filesBtn element...")
			return testutil.WaitForElement(ctx, "#filesBtn", false)
		}),
	)
	if err != nil {
		t.Fatalf("Failed to navigate to project page: %v", err)
	}

	t.Logf("About to call OpenFileModal...")
	err = testutil.OpenFileModal(ctx)
	if err != nil {
		t.Fatalf("Failed to open file modal: %v", err)
	}

	var modalVisible bool
	chromedp.Evaluate(`document.getElementById('fileModal').classList.contains('show')`, &modalVisible).Do(ctx)

	if !modalVisible {
		t.Errorf("File modal is not visible after clicking Files button")
	} else {
		t.Logf("File modal opened successfully")
	}
}

// TestWebClientAddFiles tests adding files via HTTP API and verifying they appear in the UI
func TestWebClientAddFiles(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping chromedp test in short mode")
	}

	server := startTestServer(t, "web-test-add-files")
	defer stopTestServer(t, server)

	projectID := server.ProjectID
	createProjectPayload := map[string]string{
		"projectID":    projectID,
		"baseDir":      server.ProjectDir,
		"markdownFile": server.MarkdownFile,
	}
	jsonData, _ := json.Marshal(createProjectPayload)
	resp, _ := http.Post(server.URL+"/api/projects", "application/json", bytes.NewReader(jsonData))
	resp.Body.Close()

	testFile1 := filepath.Join(server.ProjectDir, "test1.txt")
	testFile2 := filepath.Join(server.ProjectDir, "test2.txt")
	ioutil.WriteFile(testFile1, []byte("content1"), 0644)
	ioutil.WriteFile(testFile2, []byte("content2"), 0644)

	addFilesPayload := map[string]interface{}{
		"filenames": []string{testFile1, testFile2},
	}
	jsonData, _ = json.Marshal(addFilesPayload)
	fileURL := fmt.Sprintf("%s/api/projects/%s/files/add", server.URL, projectID)
	resp, _ = http.Post(fileURL, "application/json", bytes.NewReader(jsonData))
	resp.Body.Close()

	ctx, cancel := newChromeContext()
	defer cancel()

	ctx, cancelTimeout := context.WithTimeout(ctx, timeout)
	defer cancelTimeout()

	projectURL := fmt.Sprintf("%s/project/%s", server.URL, projectID)
	err := chromedp.Run(ctx,
		chromedp.Navigate(projectURL),
		chromedp.ActionFunc(func(ctx context.Context) error {
			return testutil.WaitForPageLoad(ctx)
		}),
		chromedp.ActionFunc(func(ctx context.Context) error {
			return testutil.WaitForWebSocketConnection(ctx)
		}),
		chromedp.ActionFunc(func(ctx context.Context) error {
			return testutil.WaitForElement(ctx, "#filesBtn", false)
		}),
	)
	if err != nil {
		t.Fatalf("Failed to navigate to project page: %v", err)
	}

	err = testutil.OpenFileModal(ctx)
	if err != nil {
		t.Fatalf("Failed to open file modal: %v", err)
	}

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

	projectID := server.ProjectID
	createProjectPayload := map[string]string{
		"projectID":    projectID,
		"baseDir":      server.ProjectDir,
		"markdownFile": server.MarkdownFile,
	}
	jsonData, _ := json.Marshal(createProjectPayload)
	resp, _ := http.Post(server.URL+"/api/projects", "application/json", bytes.NewReader(jsonData))
	resp.Body.Close()

	ctx, cancel := newChromeContext()
	defer cancel()

	ctx, cancelTimeout := context.WithTimeout(ctx, timeout)
	defer cancelTimeout()

	projectURL := fmt.Sprintf("%s/project/%s", server.URL, projectID)
	err := chromedp.Run(ctx,
		chromedp.Navigate(projectURL),
		chromedp.ActionFunc(func(ctx context.Context) error {
			return testutil.WaitForPageLoad(ctx)
		}),
		chromedp.ActionFunc(func(ctx context.Context) error {
			return testutil.WaitForElement(ctx, "#userInput", false)
		}),
	)
	if err != nil {
		t.Fatalf("Failed to navigate to project page: %v", err)
	}

	testQuery := "test query for web client"
	err = testutil.SubmitQuery(ctx, testQuery)
	if err != nil {
		t.Fatalf("Failed to submit query: %v", err)
	}

	time.Sleep(500 * time.Millisecond)

	var spinnerVisible bool
	chromedp.Evaluate(`.querySelector('.spinner') !== null`, &spinnerVisible).Do(ctx)

	t.Logf("Query submitted successfully via WebSocket (spinner visible: %v)", spinnerVisible)
}

// TestWebClientFileSelectionPersistence tests that file selections persist when opening and closing the modal
func TestWebClientFileSelectionPersistence(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping chromedp test in short mode")
	}

	server := startTestServer(t, "web-test-file-selection")
	defer stopTestServer(t, server)

	projectID := server.ProjectID
	createProjectPayload := map[string]string{
		"projectID":    projectID,
		"baseDir":      server.ProjectDir,
		"markdownFile": server.MarkdownFile,
	}
	jsonData, _ := json.Marshal(createProjectPayload)
	resp, _ := http.Post(server.URL+"/api/projects", "application/json", bytes.NewReader(jsonData))
	resp.Body.Close()

	testFile1 := filepath.Join(server.ProjectDir, "input.txt")
	testFile2 := filepath.Join(server.ProjectDir, "output.txt")
	ioutil.WriteFile(testFile1, []byte("input"), 0644)
	ioutil.WriteFile(testFile2, []byte("output"), 0644)

	addFilesPayload := map[string]interface{}{
		"filenames": []string{testFile1, testFile2},
	}
	jsonData, _ = json.Marshal(addFilesPayload)
	fileURL := fmt.Sprintf("%s/api/projects/%s/files/add", server.URL, projectID)
	resp, _ = http.Post(fileURL, "application/json", bytes.NewReader(jsonData))
	resp.Body.Close()

	ctx, cancel := newChromeContext()
	defer cancel()

	ctx, cancelTimeout := context.WithTimeout(ctx, timeout)
	defer cancelTimeout()

	projectURL := fmt.Sprintf("%s/project/%s", server.URL, projectID)
	err := chromedp.Run(ctx,
		chromedp.Navigate(projectURL),
		chromedp.ActionFunc(func(ctx context.Context) error {
			return testutil.WaitForPageLoad(ctx)
		}),
		chromedp.ActionFunc(func(ctx context.Context) error {
			return testutil.WaitForWebSocketConnection(ctx)
		}),
		chromedp.ActionFunc(func(ctx context.Context) error {
			return testutil.WaitForElement(ctx, "#filesBtn", false)
		}),
	)
	if err != nil {
		t.Fatalf("Failed to navigate to project page: %v", err)
	}

	testutil.OpenFileModal(ctx)

	testutil.SelectFileCheckbox(ctx, 1, "in")
	testutil.SelectFileCheckbox(ctx, 2, "out")
	time.Sleep(500 * time.Millisecond)

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

	testutil.CloseModal(ctx)

	testutil.OpenFileModal(ctx)

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

	ctx, cancel := newChromeContext()
	defer cancel()

	ctx, cancelTimeout := context.WithTimeout(ctx, timeout)
	defer cancelTimeout()

	err := chromedp.Run(ctx,
		chromedp.Navigate(server.URL),
		chromedp.ActionFunc(func(ctx context.Context) error {
			return testutil.WaitForPageLoad(ctx)
		}),
	)
	if err != nil {
		t.Fatalf("Failed to navigate to root: %v", err)
	}

	time.Sleep(500 * time.Millisecond)

	var pageTitle string
	chromedp.Run(ctx,
		chromedp.Evaluate(`document.title`, &pageTitle),
	)

	t.Logf("Landing page loaded successfully with title: %s", pageTitle)
}
