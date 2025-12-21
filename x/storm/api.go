package main

import (
	"context"
	_ "embed"
	"log"

	"github.com/danielgtaylor/huma/v2"
	"github.com/stevegt/grokker/x/storm/version"
)

// ProjectAddInput uses explicit Body field for Huma request body control
type ProjectAddInput struct {
	Body struct {
		ProjectID    string `json:"projectID" doc:"Project identifier" required:"true"`
		BaseDir      string `json:"baseDir" doc:"Base directory for project files" required:"true"`
		MarkdownFile string `json:"markdownFile" doc:"Markdown file for chat history" required:"true"`
	} `doc:"Project details"`
}

type ProjectResponse struct {
	Body struct {
		ID        string `json:"id" doc:"Project identifier"`
		BaseDir   string `json:"baseDir" doc:"Base directory"`
		ChatRound int    `json:"chatRounds" doc:"Number of chat rounds"`
	} `doc:"Project details"`
}

type ProjectInfo struct {
	ID      string `json:"id" doc:"Project identifier"`
	BaseDir string `json:"baseDir" doc:"Base directory"`
}

type ProjectListResponse struct {
	Body struct {
		Projects []ProjectInfo `json:"projects" doc:"List of projects"`
	} `doc:"Projects list"`
}

type ProjectList struct {
	Projects []ProjectInfo `json:"projects" doc:"List of projects"`
}

// ProjectDeleteInput for deleting a project
type ProjectDeleteInput struct {
	ProjectID string `path:"projectID" doc:"Project identifier" required:"true"`
}

type ProjectDeleteResponse struct {
	Body struct {
		ProjectID string `json:"projectID" doc:"Project identifier"`
		Message   string `json:"message" doc:"Deletion status message"`
	} `doc:"Project deletion result"`
}

// FileAddInput for adding files to a project
type FileAddInput struct {
	ProjectID string `path:"projectID" doc:"Project identifier" required:"true"`
	Body      struct {
		Filenames []string `json:"filenames" doc:"List of absolute file paths" required:"true"`
	} `doc:"Files to add"`
}

type FileAddResponse struct {
	Body struct {
		ProjectID string   `json:"projectID" doc:"Project identifier"`
		Added     []string `json:"added" doc:"List of successfully added files"`
		Failed    []string `json:"failed" doc:"List of files that failed to add"`
	} `doc:"Result of file additions"`
}

// FileForgetInput for removing files from a project
type FileForgetInput struct {
	ProjectID string `path:"projectID" doc:"Project identifier" required:"true"`
	Body      struct {
		Filenames []string `json:"filenames" doc:"List of absolute file paths to remove" required:"true"`
	} `doc:"Files to remove"`
}

type FileForgetResponse struct {
	Body struct {
		ProjectID string   `json:"projectID" doc:"Project identifier"`
		Removed   []string `json:"removed" doc:"List of successfully removed files"`
		Failed    []string `json:"failed" doc:"List of files that failed to remove"`
	} `doc:"Result of file removals"`
}

// FileListInput for retrieving files from a project
type FileListInput struct {
	ProjectID string `path:"projectID" doc:"Project identifier" required:"true"`
}

type FileListResponse struct {
	Body struct {
		ProjectID string   `json:"projectID" doc:"Project identifier"`
		Files     []string `json:"files" doc:"List of authorized files (relative paths when inside base directory)"`
	} `doc:"Files list"`
}

// VersionResponse returns the server version
type VersionResponse struct {
	Body struct {
		Version string `json:"version" doc:"Server version"`
	} `doc:"Version information"`
}

// Empty input type for endpoints that don't require input
type EmptyInput struct{}

// postProjectsHandler handles POST /api/projects - add a new project
func postProjectsHandler(ctx context.Context, input *ProjectAddInput) (*ProjectResponse, error) {
	project, err := projects.Add(input.Body.ProjectID, input.Body.BaseDir, input.Body.MarkdownFile)
	if err != nil {
		return nil, huma.Error400BadRequest("Failed to add project", err)
	}

	res := &ProjectResponse{}
	res.Body.ID = project.ID
	res.Body.BaseDir = project.BaseDir
	res.Body.ChatRound = len(project.Chat.history)

	log.Printf("DEBUG: Returning response with ID=%s, BaseDir=%s, ChatRound=%d", res.Body.ID, res.Body.BaseDir, res.Body.ChatRound)
	return res, nil
}

// getProjectsHandler handles GET /api/projects - list all projects
func getProjectsHandler(ctx context.Context, input *EmptyInput) (*ProjectListResponse, error) {
	projectIDs := projects.List()
	var projectInfos []ProjectInfo
	for i := 0; i < len(projectIDs); i++ {
		id := projectIDs[i]
		project, err := projects.Get(id)
		if err != nil {
			log.Printf("Error loading project %s: %v", id, err)
			continue
		}
		projectInfos = append(projectInfos, ProjectInfo{
			ID:      project.ID,
			BaseDir: project.BaseDir,
		})
	}
	res := &ProjectListResponse{}
	res.Body.Projects = projectInfos
	return res, nil
}

// deleteProjectHandler handles DELETE /api/projects/{projectID} - delete a project
func deleteProjectHandler(ctx context.Context, input *ProjectDeleteInput) (*ProjectDeleteResponse, error) {
	projectID := input.ProjectID

	if err := projects.Remove(projectID); err != nil {
		return nil, huma.Error404NotFound("Failed to delete project")
	}

	res := &ProjectDeleteResponse{}
	res.Body.ProjectID = projectID
	res.Body.Message = "Project deleted successfully"

	log.Printf("DEBUG: Project %s deleted", projectID)
	return res, nil
}

// postProjectFilesAddHandler handles POST /api/projects/{projectID}/files/add - add files to project
func postProjectFilesAddHandler(ctx context.Context, input *FileAddInput) (*FileAddResponse, error) {
	projectID := input.ProjectID

	res := &FileAddResponse{}
	res.Body.ProjectID = projectID
	res.Body.Added = []string{}
	res.Body.Failed = []string{}

	for i := 0; i < len(input.Body.Filenames); i++ {
		filename := input.Body.Filenames[i]
		if err := projects.AddFile(projectID, filename); err != nil {
			res.Body.Failed = append(res.Body.Failed, filename)
		} else {
			res.Body.Added = append(res.Body.Added, filename)
		}
	}

	// Broadcast file list update to all connected WebSocket clients
	project, err := projects.Get(projectID)
	if err == nil {
		updatedFiles := project.GetFilesAsRelative()
		broadcast := map[string]interface{}{
			"type":      "fileListUpdated",
			"projectID": projectID,
			"files":     updatedFiles,
		}
		project.ClientPool.Broadcast(broadcast)
		log.Printf("Broadcasted file list update for project %s", projectID)
	}

	return res, nil
}

// postProjectFilesForgetHandler handles POST /api/projects/{projectID}/files/forget - remove files from project
func postProjectFilesForgetHandler(ctx context.Context, input *FileForgetInput) (*FileForgetResponse, error) {
	projectID := input.ProjectID

	res := &FileForgetResponse{}
	res.Body.ProjectID = projectID
	res.Body.Removed = []string{}
	res.Body.Failed = []string{}

	for i := 0; i < len(input.Body.Filenames); i++ {
		filename := input.Body.Filenames[i]
		if err := projects.RemoveFile(projectID, filename); err != nil {
			log.Printf("Error removing file %s from project %s: %v", filename, projectID, err)
			res.Body.Failed = append(res.Body.Failed, filename)
		} else {
			res.Body.Removed = append(res.Body.Removed, filename)
		}
	}

	// Broadcast file list update to all connected WebSocket clients
	project, err := projects.Get(projectID)
	if err == nil {
		updatedFiles := project.GetFilesAsRelative()
		broadcast := map[string]interface{}{
			"type":      "fileListUpdated",
			"projectID": projectID,
			"files":     updatedFiles,
		}
		project.ClientPool.Broadcast(broadcast)
		log.Printf("Broadcasted file list update for project %s", projectID)
	}

	return res, nil
}

// getProjectFilesHandler handles GET /api/projects/{projectID}/files - list files for project
func getProjectFilesHandler(ctx context.Context, input *FileListInput) (*FileListResponse, error) {
	projectID := input.ProjectID

	project, err := projects.Get(projectID)
	if err != nil {
		return nil, huma.Error404NotFound("Project not found")
	}

	res := &FileListResponse{}
	res.Body.ProjectID = projectID
	res.Body.Files = project.GetFilesAsRelative()

	return res, nil
}

// getVersionHandler handles GET /api/version - return server version
func getVersionHandler(ctx context.Context, input *EmptyInput) (*VersionResponse, error) {
	res := &VersionResponse{}
	res.Body.Version = version.Version
	return res, nil
}
