package main

import (
	"context"
	_ "embed"
	"log"

	"github.com/danielgtaylor/huma/v2"
)

// FileDeleteInput for deleting a file from a project
type FileDeleteInput struct {
	ProjectID string `path:"projectID" doc:"Project identifier" required:"true"`
	Filename  string `path:"filename" doc:"Filename to delete" required:"true"`
}

type FileDeleteResponse struct {
	Body struct {
		ProjectID string `json:"projectID" doc:"Project identifier"`
		Filename  string `json:"filename" doc:"Deleted filename"`
		Message   string `json:"message" doc:"Deletion status"`
	} `doc:"File deletion result"`
}

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

// FileAddInput for adding files to a project - extract projectID from path parameter
type FileAddInput struct {
	ProjectID string `path:"projectID" doc:"Project identifier" required:"true"`
	Body      struct {
		Filenames []string `json:"filenames" doc:"List of files to add" required:"true"`
	} `doc:"Files to add"`
}

type FileAddResponse struct {
	Body struct {
		ProjectID string   `json:"projectID" doc:"Project identifier"`
		Added     []string `json:"added" doc:"List of successfully added files"`
		Failed    []string `json:"failed" doc:"List of files that failed to add"`
	} `doc:"Result of file additions"`
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
	for _, id := range projectIDs {
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

// postProjectFilesHandler handles POST /api/projects/{projectID}/files - add files to project
func postProjectFilesHandler(ctx context.Context, input *FileAddInput) (*FileAddResponse, error) {
	projectID := input.ProjectID

	res := &FileAddResponse{}
	res.Body.ProjectID = projectID
	res.Body.Added = []string{}
	res.Body.Failed = []string{}

	for _, filename := range input.Body.Filenames {
		if err := projects.AddFile(projectID, filename); err != nil {
			res.Body.Failed = append(res.Body.Failed, filename)
		} else {
			res.Body.Added = append(res.Body.Added, filename)
		}
	}

	return res, nil
}

func deleteProjectFilesHandler(ctx context.Context, input *FileDeleteInput) (*FileDeleteResponse, error) {
	projectID := input.ProjectID
	filename := input.Filename

	if err := projects.RemoveFile(projectID, filename); err != nil {
		return nil, huma.Error404NotFound("Failed to delete file")
	}

	res := &FileDeleteResponse{}
	res.Body.ProjectID = projectID
	res.Body.Filename = filename
	res.Body.Message = "File removed successfully"

	log.Printf("DEBUG: File %s removed from project %s", filename, projectID)
	return res, nil
}

// getProjectFilesHandler handles GET /api/projects/{projectID}/files - list files for project
// Returns relative paths when files are inside the project's base directory[1]
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
