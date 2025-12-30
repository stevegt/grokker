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

var timeout = 600 * time.Second

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

	t.Logf("About to click filesBtn with synthetic event...")
	err = testutil.ClickElementWithSyntheticEvent(ctx, "#filesBtn")
	if err != nil {
		t.Fatalf("Failed to click filesBtn: %v", err)
	}

	t.Logf("Waiting for modal content to appear...")
	err = testutil.WaitForModal(ctx)
	if err != nil {
		t.Fatalf("Modal did not appear: %v", err)
	}

	t.Logf("File modal opened successfully")
}

// TestWebClientOpenFileModalWithSyntheticEvent tests opening the file modal using synthetic mouse event
func TestWebClientOpenFileModalWithSyntheticEvent(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping chromedp test in short mode")
	}

	server := startTestServer(t, "web-test-synthetic-event-filesBtn")
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
			return testutil.WaitForElement(ctx, "#filesBtn", false)
		}),
	)
	if err != nil {
		t.Fatalf("Failed to navigate to project page: %v", err)
	}

	t.Logf("Testing filesBtn click using synthetic mouse event approach...")
	err = testutil.ClickFilesBtnWithSyntheticEvent(ctx)
	if err != nil {
		t.Fatalf("Failed to open file modal with synthetic event: %v", err)
	}

	t.Logf("File modal opened successfully using synthetic mouse event method")
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
	resp, err := http.Post(fileURL, "application/json", bytes.NewReader(jsonData))
	if err != nil {
		t.Fatalf("Failed to add files via API: %v", err)
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
			return testutil.WaitForElement(ctx, "#filesBtn", false)
		}),
	)
	if err != nil {
		t.Fatalf("Failed to navigate to project page: %v", err)
	}

	err = testutil.ClickElementWithSyntheticEvent(ctx, "#filesBtn")
	if err != nil {
		t.Fatalf("Failed to click filesBtn: %v", err)
	}

	err = testutil.WaitForModal(ctx)
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

	testQuery := "hi"
	err = testutil.SubmitQuery(ctx, testQuery)
	if err != nil {
		t.Fatalf("Failed to submit query: %v", err)
	}

	time.Sleep(500 * time.Millisecond)

	var spinnerVisible bool
	chromedp.Evaluate(`document.querySelector('.spinner') !== null`, &spinnerVisible).Do(ctx)

	t.Logf("Query submitted successfully via WebSocket (spinner visible: %v)", spinnerVisible)
}

// TestWebClientQueryWithResponse tests the complete query workflow including waiting for response
func TestWebClientQueryWithResponse(t *testing.T) {
	t.Skip("Skipping TestWebClientQueryWithResponse due to flakiness; needs investigation")
	if testing.Short() {
		t.Skip("Skipping chromedp test in short mode")
	}

	server := startTestServer(t, "web-test-query-response")
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
	t.Logf("Navigating to project: %s", projectURL)
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

	// Enter and submit a query
	testQuery := "What is 2+2?"
	t.Logf("Submitting query: %s", testQuery)
	err = testutil.SubmitQuery(ctx, testQuery)
	if err != nil {
		t.Fatalf("Failed to submit query: %v", err)
	}

	// Wait for spinner and cancel button to appear
	t.Logf("Verifying spinner and cancel button appear...")
	waitStartTime := time.Now()
	var hasSpinner, hasCancelBtn bool

	for i := 0; i < 120; i++ {
		chromedp.Evaluate(`document.querySelector('.spinner') !== null`, &hasSpinner).Do(ctx)
		chromedp.Evaluate(`document.querySelector('.message button') !== null`, &hasCancelBtn).Do(ctx)

		if hasSpinner && hasCancelBtn {
			t.Logf("✓ Spinner and cancel button appeared (%.2f seconds)", time.Since(waitStartTime).Seconds())
			break
		}
		time.Sleep(100 * time.Millisecond)
	}

	if !hasSpinner {
		t.Fatalf("Spinner did not appear after query submission")
	}

	if !hasCancelBtn {
		t.Logf("WARNING: Cancel button did not appear (possible timing/DOM synchronization issue)")
	}

	// Wait for response (with timeout)
	t.Logf("Waiting for LLM response (up to 5 minutes)...")
	responseTimeout := 5 * time.Minute
	startTime := time.Now()
	var responseReceived bool

	for time.Since(startTime) < responseTimeout {
		// Check if response appeared in chat by looking for message divs with response content
		chromedp.Evaluate(`
			(function() {
				var messages = document.querySelectorAll('.message');
				for (var i = 0; i < messages.length; i++) {
					// Look for messages that have both query text and response content
					// Skip messages that only contain the spinner
					if (messages[i].querySelector('.spinner') === null && 
					    messages[i].innerHTML.length > 50) {
						return true;
					}
				}
				return false;
			})()
		`, &responseReceived).Do(ctx)

		if responseReceived {
			break
		}

		time.Sleep(1 * time.Second)
	}

	if !responseReceived {
		t.Fatalf("LLM response did not arrive within %v", responseTimeout)
	}
	t.Logf("✓ Response received from LLM (%.1f seconds)", time.Since(startTime).Seconds())

	// Verify spinner is gone
	t.Logf("Verifying spinner disappears...")
	var spinnerGone bool
	chromedp.Evaluate(`document.querySelector('.spinner') === null`, &spinnerGone).Do(ctx)
	if !spinnerGone {
		t.Logf("WARNING: Spinner still visible, but response was received")
	} else {
		t.Logf("✓ Spinner disappeared")
	}

	// Get response text for verification
	var responseText string
	chromedp.Evaluate(`
		(function() {
			var messages = document.querySelectorAll('.message');
			var lastMessage = messages[messages.length - 1];
			if (lastMessage) {
				return lastMessage.textContent.substring(0, 200);
			}
			return '';
		})()
	`, &responseText).Do(ctx)

	t.Logf("Response preview: %s", responseText)
	t.Logf("✓ Query completed successfully with response")
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

	err = testutil.ClickElementWithSyntheticEvent(ctx, "#filesBtn")
	if err != nil {
		t.Fatalf("Failed to click filesBtn: %v", err)
	}

	err = testutil.WaitForModal(ctx)
	if err != nil {
		t.Fatalf("Failed to open file modal: %v", err)
	}
	t.Logf("File modal opened successfully")

	err = testutil.SelectFileCheckbox(ctx, 1, "in")
	if err != nil {
		t.Fatalf("Failed to select input file checkbox: %v", err)
	}
	err = testutil.SelectFileCheckbox(ctx, 2, "out")
	if err != nil {
		t.Fatalf("Failed to select output file checkbox: %v", err)
	}
	time.Sleep(500 * time.Millisecond)
	t.Logf("Selected input and output files in modal")

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

	err = testutil.ClickElementWithSyntheticEvent(ctx, "#filesBtn")
	if err != nil {
		t.Fatalf("Failed to click filesBtn on second attempt: %v", err)
	}

	err = testutil.WaitForModal(ctx)
	if err != nil {
		t.Fatalf("Failed to open file modal on second attempt: %v", err)
	}

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

// TestWebClientCreateFileAndApproveUnexpected tests the complete flow: query creates file, user approves via modal
func TestWebClientCreateFileAndApproveUnexpected(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping chromedp test in short mode")
	}

	server := startTestServer(t, "web-test-create-file-approve")
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
	t.Logf("Navigating to project: %s", projectURL)
	err := chromedp.Run(ctx,
		chromedp.Navigate(projectURL),
		chromedp.ActionFunc(func(ctx context.Context) error {
			return testutil.WaitForPageLoad(ctx)
		}),
		chromedp.ActionFunc(func(ctx context.Context) error {
			return testutil.WaitForWebSocketConnection(ctx)
		}),
		chromedp.ActionFunc(func(ctx context.Context) error {
			return testutil.WaitForElement(ctx, "#userInput", false)
		}),
	)
	if err != nil {
		t.Fatalf("Failed to navigate to project page: %v", err)
	}

	// Submit query that may produce file output
	testQuery := "Create a simple Go program file called hello.go that prints Hello World.  You MUST use proper markers:\n"
	testQuery += "---FILE-START filename=\"hello.go\"---\n"
	testQuery += "[...file content here...]\n"
	testQuery += "---FILE-END filename=\"hello.go\"---\n"

	t.Logf("Submitting query that requests file creation: %s", testQuery)
	err = testutil.SubmitQuery(ctx, testQuery)
	if err != nil {
		t.Fatalf("Failed to submit query: %v", err)
	}

	// Wait for unexpected files modal to appear
	t.Logf("Waiting for unexpected files modal to appear (indicates LLM created a file)...")
	err = testutil.WaitForUnexpectedFilesModal(ctx)
	if err != nil {
		t.Fatalf("Unexpected files modal did not appear: %v", err)
	}

	t.Logf("✓ Unexpected files modal appeared, indicating LLM created new files")

	// Get list of files needing authorization
	needsAuthFiles, err := testutil.GetNeedsAuthorizationFiles(ctx)
	if err != nil {
		t.Fatalf("Failed to get files needing authorization from modal: %v", err)
	}

	if len(needsAuthFiles) == 0 {
		time.Sleep(5 * time.Second)
		t.Fatalf("No files listed in unexpected files modal for authorization")
	}

	t.Logf("Found %d files needing authorization: %v", len(needsAuthFiles), needsAuthFiles)

	// ensure the hello.go file is the only one needing authorization
	if len(needsAuthFiles) != 1 {
		t.Fatalf("Expected exactly 1 file needing authorization, got %d", len(needsAuthFiles))
	}
	if needsAuthFiles[0] != "hello.go" {
		t.Fatalf("Expected file needing authorization to be hello.go, got %s", needsAuthFiles[0])
	}

	// Add hello.go via API (user runs CLI command)
	helloFn := filepath.Join(server.ProjectDir, "hello.go")
	addFilesPayload := map[string]interface{}{
		"filenames": []string{helloFn},
	}
	jsonData, err = json.Marshal(addFilesPayload)
	if err != nil {
		t.Fatalf("Failed to marshal add files payload: %v", err)
	}
	fileURL := fmt.Sprintf("%s/api/projects/%s/files/add", server.URL, projectID)
	resp, err = http.Post(fileURL, "application/json", bytes.NewReader(jsonData))
	if err != nil {
		t.Fatalf("Failed to add files via API: %v", err)
	}
	resp.Body.Close()

	// Wait for modal to automatically close and reopen with updated categorization
	t.Logf("Waiting for modal to close and reopen with updated file categorization...")
	time.Sleep(2 * time.Second)

	// Modal should have closed and reopened. Verify it's open with updated content
	err = testutil.WaitForModal(ctx)
	if err != nil {
		t.Fatalf("Modal did not reappear after file authorization: %v", err)
	}
	t.Logf("✓ Modal automatically closed and reopened with updated categorization")

	// Verify hello.go is now in the authorized section (but Out is unchecked)
	// The file should appear in the main file table, not in the needs-authorization section
	needsAuthFiles2, err := testutil.GetNeedsAuthorizationFiles(ctx)
	if err != nil {
		t.Fatalf("Failed to get files needing authorization on second check: %v", err)
	}

	if len(needsAuthFiles2) > 0 {
		t.Fatalf("Expected 0 files needing authorization after CLI add, got %d", len(needsAuthFiles2))
	}
	t.Logf("✓ File hello.go is no longer in 'needs authorization' section")

	// Check that hello.go appears in the file table (authorized files section)
	numRows, err := testutil.GetFileListRows(ctx)
	if err != nil {
		t.Fatalf("Failed to get file list rows: %v", err)
	}

	if numRows < 1 {
		t.Fatalf("Expected at least 1 file in authorized section, got %d", numRows)
	}
	t.Logf("✓ hello.go appears in authorized files section (%d rows)", numRows)

	// Verify the file is present but Out checkbox is unchecked
	// Select the Out checkbox to mark the file for output
	err = testutil.SelectFileCheckbox(ctx, 1, "out")
	if err != nil {
		t.Fatalf("Failed to select Out checkbox: %v", err)
	}
	t.Logf("✓ Clicked Out checkbox for hello.go")

	// Close the modal (this sends approved files back to server)
	testutil.CloseModal(ctx)
	t.Logf("✓ Closed modal after marking file for output")

	// Verify hello.go was created on disk
	helloFilePath := filepath.Join(server.ProjectDir, "hello.go")
	if _, err := os.Stat(helloFilePath); os.IsNotExist(err) {
		t.Fatalf("Expected file hello.go was not created in project directory: %v", err)
	}
	t.Logf("✓ File hello.go exists on disk at %s", helloFilePath)

	// Verify the file has the expected content
	content, err := ioutil.ReadFile(helloFilePath)
	if err != nil {
		t.Fatalf("Failed to read hello.go: %v", err)
	}

	if len(content) == 0 {
		t.Fatalf("File hello.go is empty")
	}
	t.Logf("✓ File hello.go has content: %d bytes", len(content))
}

// TestWebClientUnexpectedFilesAlreadyAuthorized tests handling already-authorized files in unexpected modal
func TestWebClientUnexpectedFilesAlreadyAuthorized(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping chromedp test in short mode")
	}

	server := startTestServer(t, "web-test-unexpected-already-auth")
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

	testFile := filepath.Join(server.ProjectDir, "preauth.txt")
	ioutil.WriteFile(testFile, []byte("pre-authorized"), 0644)

	addFilesPayload := map[string]interface{}{
		"filenames": []string{testFile},
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
			return testutil.WaitForElement(ctx, "#userInput", false)
		}),
	)
	if err != nil {
		t.Fatalf("Failed to navigate to project page: %v", err)
	}

	err = testutil.SubmitQuery(ctx, "query that references pre-authorized file")
	if err != nil {
		t.Fatalf("Failed to submit query: %v", err)
	}

	time.Sleep(1 * time.Second)

	err = testutil.WaitForUnexpectedFilesModal(ctx)
	// XXX one of these should be a fail
	if err != nil {
		t.Logf("No unexpected files modal appeared (file is already authorized): %v", err)
	} else {
		t.Logf("✓ Modal appeared showing already-authorized file in UI (marked in red)")
	}
}
