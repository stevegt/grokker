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

	// Test 3: Add files to project using /files/add endpoint[1]
	addFilesPayload := map[string]interface{}{
		"filenames": []string{inputFile, outputFile},
	}
	jsonData, err = json.Marshal(addFilesPayload)
	if err != nil {
		t.Fatalf("Failed to marshal add files request: %v", err)
	}

	url := fmt.Sprintf("%s/api/projects/%s/files/add", daemonAddr, projectID)
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

	// Test 5: Forget files using POST with list[1]
	forgetPayload := map[string]interface{}{
		"filenames": []string{inputFile},
	}
	jsonData, err = json.Marshal(forgetPayload)
	if err != nil {
		t.Fatalf("Failed to marshal forget files request: %v", err)
	}

	forgetURL := fmt.Sprintf("%s/api/projects/%s/files/forget", daemonAddr, projectID)
	req, err := http.NewRequest("POST", forgetURL, bytes.NewReader(jsonData))
	if err != nil {
		t.Fatalf("Failed to create POST request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{}
	resp, err = client.Do(req)
	if err != nil {
		t.Fatalf("Failed to forget files: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := ioutil.ReadAll(resp.Body)
		t.Fatalf("Forget files failed with status %d: %s", resp.StatusCode, string(body))
	}

	// Test 6: Verify file was forgotten
	resp, err = http.Get(fmt.Sprintf("%s/api/projects/%s/files", daemonAddr, projectID))
	if err != nil {
		t.Fatalf("Failed to list project files after deletion: %v", err)
	}
	defer resp.Body.Close()

	var listFilesResp2 map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&listFilesResp2); err != nil {
		t.Fatalf("Failed to decode list files response: %v", err)
	}

	files2, ok := listFilesResp2["files"].([]interface{})
	if !ok {
		t.Fatalf("Expected files array in response")
	}

	if len(files2) != 1 {
		t.Errorf("Expected 1 file after forget, got %d", len(files2))
	}

	// Test 7: Delete project
	deleteProjectURL := fmt.Sprintf("%s/api/projects/%s", daemonAddr, projectID)
	req, err = http.NewRequest("DELETE", deleteProjectURL, nil)
	if err != nil {
		t.Fatalf("Failed to create DELETE project request: %v", err)
	}

	resp, err = client.Do(req)
	if err != nil {
		t.Fatalf("Failed to delete project: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := ioutil.ReadAll(resp.Body)
		t.Fatalf("Delete project failed with status %d: %s", resp.StatusCode, string(body))
	}

	// Test 9: Verify project was deleted
	resp, err = http.Get(daemonAddr + "/api/projects")
	if err != nil {
		t.Fatalf("Failed to list projects after deletion: %v", err)
	}
	defer resp.Body.Close()

	var listResp2 map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&listResp2); err != nil {
		t.Fatalf("Failed to decode list response: %v", err)
	}

	projects2, ok := listResp2["projects"].([]interface{})
	if !ok {
		projects2 = []interface{}{}
	}

	if len(projects2) != 0 {
		t.Errorf("Expected 0 projects after deletion, got %d", len(projects2))
	}

	// Test 10: Stop daemon
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
