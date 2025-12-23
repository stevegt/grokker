package main

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"github.com/stevegt/grokker/x/storm/db"
)

// TODO move this file to a ./projects package?

// Projects is a thread-safe registry for managing projects
// Projects are loaded from the database on-demand and kept in cache while active
type Projects struct {
	data  map[string]*Project // TODO can we get rid of this?
	mutex sync.RWMutex
	dbMgr *db.Manager
}

// NewProjectsWithDB creates a new Projects registry with database backend
func NewProjectsWithDB(dbMgr *db.Manager) *Projects {
	return &Projects{
		data:  make(map[string]*Project),
		dbMgr: dbMgr,
	}
}

// Get retrieves a project by ID, loading from database if not in cache
func (p *Projects) Get(projectID string) (*Project, error) {
	p.mutex.RLock()
	if project, exists := p.data[projectID]; exists {
		p.mutex.RUnlock()
		return project, nil
	}
	p.mutex.RUnlock()

	// Load from database
	meta, err := p.dbMgr.LoadProject(projectID)
	if err != nil {
		return nil, err
	}

	// Reconstruct runtime Project with fresh Chat and ClientPool
	project := &Project{
		ID:              meta.ID,
		BaseDir:         meta.BaseDir,
		MarkdownFile:    meta.CurrentDiscussionFile,
		AuthorizedFiles: meta.AuthorizedFiles,
		Chat:            NewChat(meta.CurrentDiscussionFile),
		ClientPool:      NewClientPool(),
	}

	// Store in cache
	p.mutex.Lock()
	p.data[projectID] = project
	p.mutex.Unlock()

	// Start the client pool's broadcast loop
	go project.ClientPool.Start()

	log.Printf("Loaded project %s from database", projectID)
	return project, nil
}

// Add adds a new project and persists to database
func (p *Projects) Add(projectID, baseDir, markdownFile string) (*Project, error) {
	if projectID == "" {
		return nil, fmt.Errorf("projectID cannot be empty")
	}
	if baseDir == "" {
		return nil, fmt.Errorf("baseDir cannot be empty")
	}
	if markdownFile == "" {
		return nil, fmt.Errorf("markdownFile cannot be empty")
	}

	// Verify base directory exists
	if _, err := os.Stat(baseDir); os.IsNotExist(err) {
		return nil, fmt.Errorf("base directory does not exist: %s", baseDir)
	}

	log.Printf("Adding project: projectID=%s, baseDir=%s, markdownFile=%s", projectID, baseDir, markdownFile)

	// Create the Chat instance
	chatInstance := NewChat(markdownFile)
	if chatInstance == nil {
		return nil, fmt.Errorf("failed to create chat instance for project %s", projectID)
	}

	// Create ClientPool
	clientPool := NewClientPool()

	// Create runtime Project
	project := &Project{
		ID:              projectID,
		BaseDir:         baseDir,
		MarkdownFile:    markdownFile,
		AuthorizedFiles: []string{},
		Chat:            chatInstance,
		ClientPool:      clientPool,
	}

	// Create persistent metadata
	persistedProj := &db.Project{
		ID:                    projectID,
		BaseDir:               baseDir,
		CurrentDiscussionFile: markdownFile,
		DiscussionFiles: []db.DiscussionFileRef{
			{
				Filepath:   markdownFile,
				CreatedAt:  time.Now(),
				RoundCount: len(chatInstance.history),
			},
		},
		AuthorizedFiles: []string{},
		CreatedAt:       time.Now(),
		EmbeddingCount:  0,
		RoundHistory:    []db.RoundEntry{},
	}

	// Persist to database
	if err := p.dbMgr.SaveProject(persistedProj); err != nil {
		return nil, fmt.Errorf("failed to save project to database: %w", err)
	}

	// Register in cache
	p.mutex.Lock()
	p.data[projectID] = project
	p.mutex.Unlock()

	// TODO why is this here?
	// Start the client pool's broadcast loop
	go project.ClientPool.Start()

	log.Printf("Successfully registered project %s", projectID)
	return project, nil
}

// List returns all project IDs from the database
func (p *Projects) List() []string {
	ids, err := p.dbMgr.ListProjectIDs()
	if err != nil {
		log.Printf("Error listing project IDs: %v", err)
		return []string{}
	}
	return ids
}

// Remove removes a project from database and cache
func (p *Projects) Remove(projectID string) error {
	if err := p.dbMgr.DeleteProject(projectID); err != nil {
		return fmt.Errorf("failed to delete project from database: %w", err)
	}

	p.mutex.Lock()
	defer p.mutex.Unlock()
	if _, exists := p.data[projectID]; !exists {
		return fmt.Errorf("project %s not found in cache", projectID)
	}
	delete(p.data, projectID)
	log.Printf("Removed project %s", projectID)
	return nil
}

// AddFile adds a file to a project's authorized files
func (p *Projects) AddFile(projectID, filename string) error {
	if filename == "" {
		return fmt.Errorf("filename cannot be empty")
	}

	// Load project (from cache or database)
	project, err := p.Get(projectID)
	if err != nil {
		return fmt.Errorf("project not found: %w", err)
	}

	// Check if file already exists
	for _, f := range project.AuthorizedFiles {
		if f == filename {
			return fmt.Errorf("file %s already in authorized list", filename)
		}
	}

	// Add file to project
	project.AuthorizedFiles = append(project.AuthorizedFiles, filename)
	log.Printf("Added file %s to project %s", filename, projectID)

	// Create updated persistent metadata
	persistedProj := &db.Project{
		ID:                    project.ID,
		BaseDir:               project.BaseDir,
		CurrentDiscussionFile: project.MarkdownFile,
		DiscussionFiles: []db.DiscussionFileRef{
			{
				Filepath:   project.MarkdownFile,
				CreatedAt:  time.Now(),
				RoundCount: project.Chat.TotalRounds(),
			},
		},
		AuthorizedFiles: project.AuthorizedFiles,
		CreatedAt:       time.Now(),
		EmbeddingCount:  0,
		RoundHistory:    []db.RoundEntry{},
	}

	// Persist to database
	return p.dbMgr.SaveProject(persistedProj)
}

// Add RemoveFile method to Projects struct:
func (p *Projects) RemoveFile(projectID, filename string) error {
	project, err := p.Get(projectID)
	if err != nil {
		return err
	}

	idx := -1
	for i, f := range project.AuthorizedFiles {
		// translate both to absolute paths for comparison
		absF, err1 := filepath.Abs(f)
		if err1 != nil {
			log.Printf("Error getting absolute path for %s: %v", f, err1)
			continue
		}
		absFilename, err2 := filepath.Abs(filename)
		if err2 != nil {
			log.Printf("Error getting absolute path for %s: %v", filename, err2)
			continue
		}

		if absF == absFilename {
			idx = i
			break
		}
	}
	if idx == -1 {
		log.Printf("Project files: %+v", project.AuthorizedFiles)
		return fmt.Errorf("file %s not found in project %s", filename, projectID)
	}

	project.AuthorizedFiles = append(project.AuthorizedFiles[:idx], project.AuthorizedFiles[idx+1:]...)

	persistedProj := &db.Project{
		ID:                    project.ID,
		BaseDir:               project.BaseDir,
		CurrentDiscussionFile: project.MarkdownFile,
		DiscussionFiles: []db.DiscussionFileRef{
			{
				Filepath:   project.MarkdownFile,
				CreatedAt:  time.Now(),
				RoundCount: project.Chat.TotalRounds(),
			},
		},
		AuthorizedFiles: project.AuthorizedFiles,
		CreatedAt:       time.Now(),
		EmbeddingCount:  0,
		RoundHistory:    []db.RoundEntry{},
	}

	return p.dbMgr.SaveProject(persistedProj)
}

// toRelativePath converts an absolute path to relative if it's within BaseDir,
// otherwise returns the original path unchanged
func (p *Project) toRelativePath(absPath string) string {
	if !filepath.IsAbs(absPath) {
		// Already relative, return as-is
		return absPath
	}

	// Try to make it relative to BaseDir
	relPath, err := filepath.Rel(p.BaseDir, absPath)
	if err != nil {
		// Failed to compute relative path, return original
		log.Printf("Failed to compute relative path for %s relative to %s: %v", absPath, p.BaseDir, err)
		return absPath
	}

	// Check if the result tries to go outside BaseDir (contains ..)
	// If so, return the original absolute path
	if filepath.IsAbs(relPath) || filepath.HasPrefix(relPath, "..") {
		return absPath
	}

	return relPath
}

// GetChat returns the Chat instance for a project
func (p *Project) GetChat() *Chat {
	return p.Chat
}

// GetClientPool returns the ClientPool for a project
func (p *Project) GetClientPool() *ClientPool {
	return p.ClientPool
}

// GetFiles returns the authorized files list for a project
func (p *Project) GetFiles() []string {
	return p.AuthorizedFiles
}

// GetFilesAsRelative returns the authorized files list with paths converted to relative
// when they are inside the project's BaseDir, absolute paths otherwise, sorted alphabetically
func (p *Project) GetFilesAsRelative() []string {
	var relativeFiles []string
	for i := 0; i < len(p.AuthorizedFiles); i++ {
		relativeFiles = append(relativeFiles, p.toRelativePath(p.AuthorizedFiles[i]))
	}
	// Sort files alphabetically
	sort.Strings(relativeFiles)
	return relativeFiles
}
