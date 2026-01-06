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
	"github.com/stevegt/grokker/x/storm/version"
)

// CLI Helper Functions

// getDaemonURL retrieves the daemon URL from environment or returns default
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
	for i := 0; i < len(acceptedCodes); i++ {
		code := acceptedCodes[i]
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

// runVersion implements the version command
func runVersion(cmd *cobra.Command, args []string) error {
	// Show CLI version
	fmt.Printf("storm %s (CLI)\n", version.Version)

	// Try to contact the server and get its version
	resp, err := makeRequest("GET", "/api/version", nil)
	if err != nil {
		// Server not reachable - this is not an error, just informational
		fmt.Printf("(server not reachable at %s)\n", getDaemonURL())
		return nil
	}
	defer resp.Body.Close()

	if err := checkStatusCode(resp, http.StatusOK); err != nil {
		// Server returned an error - also not critical
		fmt.Printf("(error querying server version)\n")
		return nil
	}

	var result map[string]interface{}
	if err := decodeJSON(resp, &result); err != nil {
		fmt.Printf("(error decoding server version)\n")
		return nil
	}

	// Extract version from map
	if serverVersion, ok := result["version"].(string); ok {
		fmt.Printf("storm %s (server)\n", serverVersion)
	} else {
		fmt.Printf("(unexpected server version format %#v)\n", result)
	}

	return nil
}

// runServe implements the serve command
func runServe(cmd *cobra.Command, args []string) error {
	fmt.Printf("storm %s\n", version.Version)
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
	projectID := args[0]
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
	for i := 0; i < len(projectList.Projects); i++ {
		proj := projectList.Projects[i]
		fmt.Printf("  - %s (baseDir: %s)\n", proj.ID, proj.BaseDir)
	}
	return nil
}

// runProjectInfo implements the project info command
func runProjectInfo(cmd *cobra.Command, args []string) error {
	projectID := args[0]

	endpoint := fmt.Sprintf("/api/projects/%s", projectID)
	resp, err := makeRequest("GET", endpoint, nil)
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

	fmt.Printf("Project %s\n", projectID)
	if baseDir, ok := result["baseDir"].(string); ok {
		fmt.Printf("  BaseDir: %s\n", baseDir)
	}
	if current, ok := result["current"].(string); ok {
		fmt.Printf("  Current discussion: %s\n", current)
	}
	if files, ok := result["discussionFiles"].([]interface{}); ok {
		fmt.Printf("  Discussions:\n")
		for i := 0; i < len(files); i++ {
			if f, ok := files[i].(string); ok {
				fmt.Printf("    - %s\n", f)
			}
		}
	}
	if files, ok := result["authorizedFiles"].([]interface{}); ok {
		fmt.Printf("  Authorized files:\n")
		for i := 0; i < len(files); i++ {
			if f, ok := files[i].(string); ok {
				fmt.Printf("    - %s\n", f)
			}
		}
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

// runProjectUpdate implements the project update command
func runProjectUpdate(cmd *cobra.Command, args []string) error {
	projectID := args[0]
	baseDir, err := cmd.Flags().GetString("basedir")
	if err != nil {
		return err
	}
	if err := validateRequiredFlag(baseDir, "basedir"); err != nil {
		return err
	}

	resolvedBaseDir, err := resolvePath(baseDir)
	if err != nil {
		return err
	}

	payload := map[string]string{
		"basedir": resolvedBaseDir,
	}

	endpoint := fmt.Sprintf("/api/projects/%s/update", projectID)
	resp, err := makeRequest("POST", endpoint, payload)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if err := checkStatusCode(resp, http.StatusOK); err != nil {
		return err
	}

	fmt.Printf("Project %s baseDir updated to %s\n", projectID, resolvedBaseDir)
	return nil
}

// runDiscussionList implements the discussion list command
func runDiscussionList(cmd *cobra.Command, args []string) error {
	projectID, err := cmd.Flags().GetString("project")
	if err != nil {
		return err
	}
	if err := validateRequiredFlag(projectID, "project"); err != nil {
		return err
	}

	endpoint := fmt.Sprintf("/api/projects/%s/discussions", projectID)
	resp, err := makeRequest("GET", endpoint, nil)
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

	current, _ := result["current"].(string)
	files, _ := result["files"].([]interface{})

	fmt.Printf("Discussions for project %s:\n", projectID)
	for i := 0; i < len(files); i++ {
		f, _ := files[i].(string)
		marker := " "
		if f == current {
			marker = "*"
		}
		fmt.Printf("  %s %s\n", marker, f)
	}
	return nil
}

// runDiscussionAdd implements the discussion add command
func runDiscussionAdd(cmd *cobra.Command, args []string) error {
	projectID, err := cmd.Flags().GetString("project")
	if err != nil {
		return err
	}
	if err := validateRequiredFlag(projectID, "project"); err != nil {
		return err
	}

	filename := args[0]
	resolvedFilename, err := resolvePath(filename)
	if err != nil {
		return err
	}

	payload := map[string]string{
		"filename": resolvedFilename,
	}

	endpoint := fmt.Sprintf("/api/projects/%s/discussions/add", projectID)
	resp, err := makeRequest("POST", endpoint, payload)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if err := checkStatusCode(resp, http.StatusOK); err != nil {
		return err
	}

	fmt.Printf("Discussion file added to project %s: %s\n", projectID, resolvedFilename)
	return nil
}

// runDiscussionForget implements the discussion forget command
func runDiscussionForget(cmd *cobra.Command, args []string) error {
	projectID, err := cmd.Flags().GetString("project")
	if err != nil {
		return err
	}
	if err := validateRequiredFlag(projectID, "project"); err != nil {
		return err
	}

	filename := args[0]
	resolvedFilename, err := resolvePath(filename)
	if err != nil {
		return err
	}

	payload := map[string]string{
		"filename": resolvedFilename,
	}

	endpoint := fmt.Sprintf("/api/projects/%s/discussions/forget", projectID)
	resp, err := makeRequest("POST", endpoint, payload)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if err := checkStatusCode(resp, http.StatusOK); err != nil {
		return err
	}

	fmt.Printf("Discussion file forgotten from project %s: %s\n", projectID, resolvedFilename)
	return nil
}

// runDiscussionSwitch implements the discussion switch command
func runDiscussionSwitch(cmd *cobra.Command, args []string) error {
	projectID, err := cmd.Flags().GetString("project")
	if err != nil {
		return err
	}
	if err := validateRequiredFlag(projectID, "project"); err != nil {
		return err
	}

	filename := args[0]
	resolvedFilename, err := resolvePath(filename)
	if err != nil {
		return err
	}
	payload := map[string]string{
		"filename": resolvedFilename,
	}

	endpoint := fmt.Sprintf("/api/projects/%s/discussions/switch", projectID)
	resp, err := makeRequest("POST", endpoint, payload)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if err := checkStatusCode(resp, http.StatusOK); err != nil {
		return err
	}

	fmt.Printf("Discussion switched for project %s: %s\n", projectID, resolvedFilename)
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

	// Resolve relative paths to absolute
	var resolvedFilenames []string
	for i := 0; i < len(args); i++ {
		resolved, err := resolvePath(args[i])
		if err != nil {
			return err
		}
		resolvedFilenames = append(resolvedFilenames, resolved)
	}

	payload := map[string]interface{}{
		"filenames": resolvedFilenames,
	}

	endpoint := fmt.Sprintf("/api/projects/%s/files/add", projectID)
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
		for i := 0; i < len(added); i++ {
			f := added[i]
			fmt.Printf("  + %s\n", f)
		}
	}
	if failed, ok := result["failed"].([]interface{}); ok {
		if len(failed) > 0 {
			fmt.Printf("Failed to add:\n")
			for i := 0; i < len(failed); i++ {
				f := failed[i]
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
			for i := 0; i < len(files); i++ {
				f := files[i]
				fmt.Printf("  - %s\n", f)
			}
		}
	}
	return nil
}

// runFileForget implements the file forget command - accepts multiple files
func runFileForget(cmd *cobra.Command, args []string) error {
	projectID, err := cmd.Flags().GetString("project")
	if err != nil {
		return err
	}
	if err := validateRequiredFlag(projectID, "project"); err != nil {
		return err
	}

	if len(args) < 1 {
		return fmt.Errorf("filename(s) required")
	}

	// Resolve relative paths to absolute
	var resolvedFilenames []string
	for i := 0; i < len(args); i++ {
		resolved, err := resolvePath(args[i])
		if err != nil {
			return err
		}
		resolvedFilenames = append(resolvedFilenames, resolved)
	}

	payload := map[string]interface{}{
		"filenames": resolvedFilenames,
	}

	endpoint := fmt.Sprintf("/api/projects/%s/files/forget", projectID)
	resp, err := makeRequest("POST", endpoint, payload)
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

	fmt.Printf("Files forgotten from project %s:\n", projectID)
	if removed, ok := result["removed"].([]interface{}); ok {
		for i := 0; i < len(removed); i++ {
			f := removed[i]
			fmt.Printf("  - %s\n", f)
		}
	}
	if failed, ok := result["failed"].([]interface{}); ok {
		if len(failed) > 0 {
			fmt.Printf("Failed to remove:\n")
			for i := 0; i < len(failed); i++ {
				f := failed[i]
				fmt.Printf("  - %s\n", f)
			}
		}
	}
	return nil
}

// runIssueToken implements the issue-token command
func runIssueToken(cmd *cobra.Command, args []string) error {
	fmt.Println("Would issue CWT token via HTTP API")
	return nil
}

func main() {
	// fmt.Printf("storm %s\n", version.Version)

	rootCmd := &cobra.Command{
		Use:   "storm",
		Short: "Storm - Multi-project LLM chat application",
		Long:  `Storm is a single-daemon, single-port multi-project chat application for interacting with LLMs and local files.`,
	}

	// Version command
	versionCmd := &cobra.Command{
		Use:   "version",
		Short: "Display version information",
		Long:  `Display the current version of Storm CLI, and server if available.`,
		RunE:  runVersion,
	}
	rootCmd.AddCommand(versionCmd)

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
		Long:  `Add a new project to the registry. Paths can be absolute or relative (resolved to absolute against current working directory).`,
		Args:  cobra.ExactArgs(3),
		RunE:  runProjectAdd,
	}

	projectListCmd := &cobra.Command{
		Use:   "list",
		Short: "List all projects",
		Long:  `List all registered projects via HTTP API.`,
		RunE:  runProjectList,
	}

	projectInfoCmd := &cobra.Command{
		Use:   "info [projectID]",
		Short: "Show project details",
		Long:  `Show project details via HTTP API.`,
		Args:  cobra.ExactArgs(1),
		RunE:  runProjectInfo,
	}

	projectForgetCmd := &cobra.Command{
		Use:   "forget [projectID]",
		Short: "Delete a project",
		Long:  `Delete a project and all its data from the database via HTTP API.`,
		Args:  cobra.ExactArgs(1),
		RunE:  runProjectForget,
	}

	projectUpdateCmd := &cobra.Command{
		Use:   "update [projectID]",
		Short: "Update a project's settings",
		Long:  `Update project settings via HTTP API.`,
		Args:  cobra.ExactArgs(1),
		RunE:  runProjectUpdate,
	}
	projectUpdateCmd.Flags().String("basedir", "", "New base directory for the project (required)")

	projectCmd.AddCommand(projectAddCmd, projectListCmd, projectInfoCmd, projectForgetCmd, projectUpdateCmd)
	rootCmd.AddCommand(projectCmd)

	// Discussion command
	discussionCmd := &cobra.Command{
		Use:   "discussion",
		Short: "Manage discussion files",
		Long:  `Manage discussion files associated with projects.`,
	}

	discussionListCmd := &cobra.Command{
		Use:   "list",
		Short: "List discussion files for a project",
		Long:  `List discussion files and the current selection.`,
		RunE:  runDiscussionList,
	}
	discussionListCmd.Flags().StringP("project", "p", "", "Project ID (required)")

	discussionAddCmd := &cobra.Command{
		Use:   "add [filename]",
		Short: "Add a discussion file to a project",
		Long:  `Add a discussion markdown file to the project.`,
		Args:  cobra.ExactArgs(1),
		RunE:  runDiscussionAdd,
	}
	discussionAddCmd.Flags().StringP("project", "p", "", "Project ID (required)")

	discussionForgetCmd := &cobra.Command{
		Use:   "forget [filename]",
		Short: "Forget a discussion file from a project",
		Long:  `Remove a discussion markdown file from the project list.`,
		Args:  cobra.ExactArgs(1),
		RunE:  runDiscussionForget,
	}
	discussionForgetCmd.Flags().StringP("project", "p", "", "Project ID (required)")

	discussionSwitchCmd := &cobra.Command{
		Use:   "switch [filename]",
		Short: "Switch to a discussion file",
		Long:  `Switch the active discussion markdown file.`,
		Args:  cobra.ExactArgs(1),
		RunE:  runDiscussionSwitch,
	}
	discussionSwitchCmd.Flags().StringP("project", "p", "", "Project ID (required)")

	discussionCmd.AddCommand(discussionListCmd, discussionAddCmd, discussionForgetCmd, discussionSwitchCmd)
	rootCmd.AddCommand(discussionCmd)

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
		Use:   "forget [files...]",
		Short: "Remove files from a project",
		Long:  `Remove one or more authorized files from a project.`,
		Args:  cobra.MinimumNArgs(1),
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
