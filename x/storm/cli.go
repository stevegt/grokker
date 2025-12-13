package main

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
	// . "github.com/stevegt/goadapt"
)

// CLI Helper Functions

// getDaemonURL retrieves the daemon URL from environment or returns default
// TODO use a config file, PID file, or flag -- maybe use viper
// TODO allow for multiple storm daemons on different ports, add an 'ls' command to show running daemons and their pids/ports.  registry should support multiple daemons.
func getDaemonURL() string {
	daemonURL := os.Getenv("STORM_DAEMON_URL")
	if daemonURL == "" {
		daemonURL = "http://localhost:8080"
	}
	return daemonURL
}

// makeRequest makes an HTTP request with consistent error handling
func makeRequest(method, endpoint string, payload interface{}) (*http.Response, error) {
	daemonURL := getDaemonURL()
	url := daemonURL + endpoint

	var req *http.Request
	var err error

	if payload != nil {
		jsonData, err := json.Marshal(payload)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal request: %w", err)
		}
		req, err = http.NewRequest(method, url, bytes.NewReader(jsonData))
	} else {
		req, err = http.NewRequest(method, url, nil)
	}

	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to daemon at %s: %w", daemonURL, err)
	}

	return resp, nil
}

// decodeJSON decodes a JSON response with error handling
func decodeJSON(resp *http.Response, v interface{}) error {
	return json.NewDecoder(resp.Body).Decode(v)
}

// validateRequiredFlag validates that a required flag has been set
func validateRequiredFlag(flagValue, flagName string) error {
	if flagValue == "" {
		return fmt.Errorf("--%s flag is required", flagName)
	}
	return nil
}

// checkStatusCode validates HTTP response status code
func checkStatusCode(resp *http.Response, acceptedCodes ...int) error {
	for _, code := range acceptedCodes {
		if resp.StatusCode == code {
			return nil
		}
	}
	body, _ := ioutil.ReadAll(resp.Body)
	return fmt.Errorf("daemon returned status %d: %s", resp.StatusCode, string(body))
}

// resolvePath converts relative paths to absolute paths; returns absolute paths unchanged
func resolvePath(path string) (string, error) {
	if filepath.IsAbs(path) {
		// Already absolute, return as-is
		return path, nil
	}
	// Relative path: resolve against current working directory
	cwd, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("failed to get current working directory: %w", err)
	}
	return filepath.Join(cwd, path), nil
}

// CLI Command Handlers

// runServe implements the serve command
func runServe(cmd *cobra.Command, args []string) error {
	port, err := cmd.Flags().GetInt("port")
	if err != nil {
		return err
	}

	dbPath, err := cmd.Flags().GetString("db-path")
	if err != nil {
		return err
	}

	return serveRun(port, dbPath)
}

// runStop implements the stop command
func runStop(cmd *cobra.Command, args []string) error {
	resp, err := makeRequest("POST", "/stop", nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if err := checkStatusCode(resp, http.StatusOK); err != nil {
		return err
	}

	fmt.Println("Daemon stopped successfully")
	return nil
}

// runProjectAdd implements the project add command
func runProjectAdd(cmd *cobra.Command, args []string) error {
	projectID := args[0] // note to LLM: do NOT replace args[0] with args
	baseDir := args[1]
	markdownFile := args[2]

	// Resolve paths to absolute (relative paths resolved against cwd at add-time)
	resolvedBaseDir, err := resolvePath(baseDir)
	if err != nil {
		return err
	}
	resolvedMarkdownFile, err := resolvePath(markdownFile)
	if err != nil {
		return err
	}

	// TODO default projectID to current repo -- would need .storm file or directory in repo top
	// TODO ensure baseDir exists (could be checked here or let server validate)
	// TODO add a `mv` subcommand to relocate projects

	payload := map[string]string{
		"projectID":    projectID,
		"baseDir":      resolvedBaseDir,
		"markdownFile": resolvedMarkdownFile,
	}

	resp, err := makeRequest("POST", "/api/projects", payload)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if err := checkStatusCode(resp, http.StatusOK, http.StatusCreated); err != nil {
		return err
	}

	var result map[string]interface{}
	if err := decodeJSON(resp, &result); err != nil {
		return fmt.Errorf("failed to decode response: %w", err)
	}

	fmt.Printf("Project %s added successfully\n", projectID)
	fmt.Printf("  BaseDir: %s\n", resolvedBaseDir)
	fmt.Printf("  MarkdownFile: %s\n", resolvedMarkdownFile)
	if rounds, ok := result["chatRounds"].(float64); ok {
		fmt.Printf("  Chat rounds loaded: %d\n", int(rounds))
	}
	return nil
}

// runProjectList implements the project list command
func runProjectList(cmd *cobra.Command, args []string) error {
	resp, err := makeRequest("GET", "/api/projects", nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if err := checkStatusCode(resp, http.StatusOK, http.StatusNoContent); err != nil {
		return err
	}

	if resp.StatusCode == http.StatusNoContent {
		fmt.Println("No projects registered")
		return nil
	}

	var projectList ProjectList
	if err := decodeJSON(resp, &projectList); err != nil {
		return fmt.Errorf("failed to decode response: %w", err)
	}

	if len(projectList.Projects) == 0 {
		fmt.Println("No projects registered")
		return nil
	}

	fmt.Println("Registered projects:")
	for _, proj := range projectList.Projects {
		fmt.Printf("  - %s (baseDir: %s)\n", proj.ID, proj.BaseDir)
	}
	return nil
}

// runProjectForget implements the project forget command
func runProjectForget(cmd *cobra.Command, args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("project ID is required")
	}
	projectID := args[0]

	endpoint := fmt.Sprintf("/api/projects/%s", projectID)
	resp, err := makeRequest("DELETE", endpoint, nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if err := checkStatusCode(resp, http.StatusOK); err != nil {
		return err
	}

	var result map[string]interface{}
	if err := decodeJSON(resp, &result); err != nil {
		return fmt.Errorf("failed to decode response: %w", err)
	}

	fmt.Printf("Project %s forgotten and removed from database\n", projectID)
	return nil
}

// runFileAdd implements the file add command
func runFileAdd(cmd *cobra.Command, args []string) error {
	projectID, err := cmd.Flags().GetString("project")
	if err != nil {
		return err
	}
	if err := validateRequiredFlag(projectID, "project"); err != nil {
		return err
	}

	payload := map[string]interface{}{
		"filenames": args,
	}

	endpoint := fmt.Sprintf("/api/projects/%s/files", projectID)
	resp, err := makeRequest("POST", endpoint, payload)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if err := checkStatusCode(resp, http.StatusOK, http.StatusCreated); err != nil {
		return err
	}

	var result map[string]interface{}
	if err := decodeJSON(resp, &result); err != nil {
		return fmt.Errorf("failed to decode response: %w", err)
	}

	fmt.Printf("Files added to project %s:\n", projectID)
	if added, ok := result["added"].([]interface{}); ok {
		for _, f := range added {
			fmt.Printf("  + %s\n", f)
		}
	}
	if failed, ok := result["failed"].([]interface{}); ok {
		if len(failed) > 0 {
			fmt.Printf("Failed to add:\n")
			for _, f := range failed {
				fmt.Printf("  - %s\n", f)
			}
		}
	}
	return nil
}

// runFileList implements the file list command
func runFileList(cmd *cobra.Command, args []string) error {
	projectID, err := cmd.Flags().GetString("project")
	if err != nil {
		return err
	}
	if err := validateRequiredFlag(projectID, "project"); err != nil {
		return err
	}

	endpoint := fmt.Sprintf("/api/projects/%s/files", projectID)
	resp, err := makeRequest("GET", endpoint, nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if err := checkStatusCode(resp, http.StatusOK, http.StatusNoContent); err != nil {
		return err
	}

	if resp.StatusCode == http.StatusNoContent {
		fmt.Printf("No files authorized for project %s\n", projectID)
		return nil
	}

	var result map[string]interface{}
	if err := decodeJSON(resp, &result); err != nil {
		return fmt.Errorf("failed to decode response: %w", err)
	}

	fmt.Printf("Authorized files for project %s:\n", projectID)
	if files, ok := result["files"].([]interface{}); ok {
		if len(files) == 0 {
			fmt.Println("  (no files)")
		} else {
			for _, f := range files {
				fmt.Printf("  - %s\n", f)
			}
		}
	}
	return nil
}

// runFileForget implements the file forget command[1]
func runFileForget(cmd *cobra.Command, args []string) error {
	projectID, err := cmd.Flags().GetString("project")
	if err != nil {
		return err
	}
	if err := validateRequiredFlag(projectID, "project"); err != nil {
		return err
	}

	if len(args) < 1 {
		return fmt.Errorf("filename is required")
	}
	var filename string
	filename = args[0]

	endpoint := fmt.Sprintf("/api/projects/%s/files/%s", projectID, filename)
	resp, err := makeRequest("DELETE", endpoint, nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if err := checkStatusCode(resp, http.StatusOK); err != nil {
		return err
	}

	fmt.Printf("File %s removed from project %s\n", filename, projectID)
	return nil
}

// runIssueToken implements the issue-token command
func runIssueToken(cmd *cobra.Command, args []string) error {
	fmt.Println("Would issue CWT token via HTTP API")
	return nil
}

func main() {
	fmt.Println("storm v0.0.76")

	rootCmd := &cobra.Command{
		Use:   "storm",
		Short: "Storm - Multi-project LLM chat application",
		Long:  `Storm is a single-daemon, single-port multi-project chat application for interacting with LLMs and local files.`,
	}

	// Serve command
	serveCmd := &cobra.Command{
		Use:   "serve",
		Short: "Start the Storm server",
		Long:  `Start the Storm server on the specified port.`,
		RunE:  runServe,
	}
	serveCmd.Flags().IntP("port", "p", 8080, "port to listen on")
	serveCmd.Flags().StringP("db-path", "d", "", "path to database file (default: ~/.storm/data.db)")
	rootCmd.AddCommand(serveCmd)

	// Stop command
	stopCmd := &cobra.Command{
		Use:   "stop",
		Short: "Stop the Storm daemon",
		Long:  `Stop the running Storm daemon gracefully.`,
		RunE:  runStop,
	}
	rootCmd.AddCommand(stopCmd)

	// Project command
	projectCmd := &cobra.Command{
		Use:   "project",
		Short: "Manage projects",
		Long:  `Manage Storm projects.`,
	}

	projectAddCmd := &cobra.Command{
		Use:   "add [projectID] [baseDir] [markdownFile]",
		Short: "Add a new project",
		Long:  `Add a new project to the registry. Paths can be absolute or relative (resolved against current working directory).`,
		Args:  cobra.ExactArgs(3),
		RunE:  runProjectAdd,
	}

	projectListCmd := &cobra.Command{
		Use:   "list",
		Short: "List all projects",
		Long:  `List all registered projects via HTTP API.`,
		RunE:  runProjectList,
	}

	projectForgetCmd := &cobra.Command{
		Use:   "forget [projectID]",
		Short: "Delete a project",
		Long:  `Delete a project and all its data from the database via HTTP API.`,
		Args:  cobra.ExactArgs(1),
		RunE:  runProjectForget,
	}

	projectCmd.AddCommand(projectAddCmd, projectListCmd, projectForgetCmd)
	rootCmd.AddCommand(projectCmd)

	// File command
	fileCmd := &cobra.Command{
		Use:   "file",
		Short: "Manage project files",
		Long:  `Manage files associated with projects.`,
	}

	fileAddCmd := &cobra.Command{
		Use:   "add [files...]",
		Short: "Add files to a project",
		Long:  `Add one or more authorized files to a project.`,
		Args:  cobra.MinimumNArgs(1),
		RunE:  runFileAdd,
	}
	fileAddCmd.Flags().StringP("project", "p", "", "Project ID (required)")

	fileListCmd := &cobra.Command{
		Use:   "list",
		Short: "List files in a project",
		Long:  `List all authorized files for a project.`,
		RunE:  runFileList,
	}
	fileListCmd.Flags().StringP("project", "p", "", "Project ID (required)")

	fileForgetCmd := &cobra.Command{
		Use:   "forget [filename]",
		Short: "Remove a file from a project",
		Long:  `Remove an authorized file from a project.`,
		Args:  cobra.ExactArgs(1),
		RunE:  runFileForget,
	}
	fileForgetCmd.Flags().StringP("project", "p", "", "Project ID (required)")

	fileCmd.AddCommand(fileAddCmd, fileListCmd, fileForgetCmd)
	rootCmd.AddCommand(fileCmd)

	// Token command
	tokenCmd := &cobra.Command{
		Use:   "issue-token",
		Short: "Issue a CWT token",
		Long:  `Issue a CBOR Web Token for project access.`,
		RunE:  runIssueToken,
	}
	rootCmd.AddCommand(tokenCmd)

	if err := rootCmd.Execute(); err != nil {
		log.Fatal(err)
	}
}
