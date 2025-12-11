package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// TestAPIEndpoints tests the complete API workflow
func TestAPIEndpoints(t *testing.T) {
	// Create temporary project directory
	tmpDir, err := ioutil.TempDir("", "storm-test-")
	if err != nil {
		t.Fatalf("Failed to create temporary directory: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create database directory for this test
	dbDir := filepath.Join(tmpDir, "db")
	if err := os.MkdirAll(dbDir, 0755); err != nil {
		t.Fatalf("Failed to create database directory: %v", err)
	}
	dbPath := filepath.Join(dbDir, "test.db")

	// Create project subdirectory and markdown file
	projectID := "test-project-1"
	projectDir := filepath.Join(tmpDir, projectID)
	if err := os.MkdirAll(projectDir, 0755); err != nil {
		t.Fatalf("Failed to create project directory: %v", err)
	}

	markdownFile := filepath.Join(projectDir, "chat.md")
	if err := ioutil.WriteFile(markdownFile, []byte(""), 0644); err != nil {
		t.Fatalf("Failed to create markdown file: %v", err)
	}

	// Start daemon in background
	daemonPort := 59999
	daemonAddr := fmt.Sprintf("http://localhost:%d", daemonPort)

	// Run serveRun in a goroutine
	go func() {
		if err := serveRun(daemonPort, dbPath); err != nil {
			t.Logf("Daemon error: %v", err)
		}
	}()

	// Wait for daemon to start
	time.Sleep(2 * time.Second)

	// Test 1: Create a project
	createProjectPayload := map[string]string{
		"projectID":    projectID,
		"baseDir":      projectDir,
		"markdownFile": markdownFile,
	}
	jsonData, err := json.Marshal(createProjectPayload)
	if err != nil {
		t.Fatalf("Failed to marshal request: %v", err)
	}

	resp, err := http.Post(daemonAddr+"/api/projects", "application/json", bytes.NewReader(jsonData))
	if err != nil {
		t.Fatalf("Failed to create project: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		body, _ := ioutil.ReadAll(resp.Body)
		t.Fatalf("Create project failed with status %d: %s", resp.StatusCode, string(body))
	}

	var createResp map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&createResp); err != nil {
		t.Fatalf("Failed to decode create response: %v", err)
	}

	if id, ok := createResp["id"].(string); !ok || id != projectID {
		t.Errorf("Expected project ID %s, got %v", projectID, createResp["id"])
	}

	// Test 2: List projects
	resp, err = http.Get(daemonAddr + "/api/projects")
	if err != nil {
		t.Fatalf("Failed to list projects: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := ioutil.ReadAll(resp.Body)
		t.Fatalf("List projects failed with status %d: %s", resp.StatusCode, string(body))
	}

	var listResp map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&listResp); err != nil {
		t.Fatalf("Failed to decode list response: %v", err)
	}

	projects, ok := listResp["projects"].([]interface{})
	if !ok {
		t.Fatalf("Expected projects array in response")
	}

	if len(projects) == 0 {
		t.Errorf("Expected at least 1 project, got %d", len(projects))
	}

	// Test 3: Create input and output files in project directory
	inputFile := filepath.Join(projectDir, "input.csv")
	if err := ioutil.WriteFile(inputFile, []byte("col1,col2\nval1,val2\n"), 0644); err != nil {
		t.Fatalf("Failed to create input file: %v", err)
	}

	outputFile := filepath.Join(projectDir, "output.json")
	if err := ioutil.WriteFile(outputFile, []byte("{}"), 0644); err != nil {
		t.Fatalf("Failed to create output file: %v", err)
	}

	// Test 4: Add files to project
	addFilesPayload := map[string]interface{}{
		"filenames": []string{inputFile, outputFile},
	}
	jsonData, err = json.Marshal(addFilesPayload)
	if err != nil {
		t.Fatalf("Failed to marshal add files request: %v", err)
	}

	url := fmt.Sprintf("%s/api/projects/%s/files", daemonAddr, projectID)
	resp, err = http.Post(url, "application/json", bytes.NewReader(jsonData))
	if err != nil {
		t.Fatalf("Failed to add files: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		body, _ := ioutil.ReadAll(resp.Body)
		t.Fatalf("Add files failed with status %d: %s", resp.StatusCode, string(body))
	}

	var addFilesResp map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&addFilesResp); err != nil {
		t.Fatalf("Failed to decode add files response: %v", err)
	}

	added, ok := addFilesResp["added"].([]interface{})
	if !ok || len(added) == 0 {
		t.Errorf("Expected files to be added, got %v", addFilesResp)
	}

	// Test 5: List files in project
	resp, err = http.Get(fmt.Sprintf("%s/api/projects/%s/files", daemonAddr, projectID))
	if err != nil {
		t.Fatalf("Failed to list project files: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := ioutil.ReadAll(resp.Body)
		t.Fatalf("List files failed with status %d: %s", resp.StatusCode, string(body))
	}

	var listFilesResp map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&listFilesResp); err != nil {
		t.Fatalf("Failed to decode list files response: %v", err)
	}

	files, ok := listFilesResp["files"].([]interface{})
	if !ok {
		t.Fatalf("Expected files array in response")
	}

	if len(files) != 2 {
		t.Errorf("Expected 2 files, got %d", len(files))
	}

	// Test 6: Stop daemon
	resp, err = http.Post(daemonAddr+"/stop", "application/json", nil)
	if err != nil {
		t.Logf("Stop request completed (connection may have closed): %v", err)
	}
	if resp != nil {
		resp.Body.Close()
	}

	// Wait for daemon to stop
	time.Sleep(1 * time.Second)

	// ensure daemon has stopped by checking connection refusal
	_, err = http.Get(daemonAddr + "/api/projects")
	if err == nil {
		t.Errorf("Expected daemon to be stopped, but it is still running")
	}

}