package main

import (
	"fmt"
	"log"
	"os"
	"sync"
	"time"

	"github.com/stevegt/grokker/x/storm/db"
)

// Projects is a thread-safe wrapper around project registry backed by database
type Projects struct {
	data    map[string]*Project
	mutex   sync.RWMutex
	dbMgr   *db.Manager
}

// NewProjectsWithDB creates a new Projects registry with database backend
func NewProjectsWithDB(dbMgr *db.Manager) *Projects {
	return &Projects{
		data:  make(map[string]*Project),
		dbMgr: dbMgr,
	}
}

// LoadFromDB loads all projects from the database into memory cache
func (p *Projects) LoadFromDB() error {
	persistedProjects, err := p.dbMgr.LoadAllProjects()
	if err != nil {
		return fmt.Errorf("failed to load projects from database: %w", err)
	}

	p.mutex.Lock()
	defer p.mutex.Unlock()

	for _, persistedProj := range persistedProjects {
		// Reconstruct runtime-only fields
		project := &Project{
			ID:              persistedProj.ID,
			BaseDir:         persistedProj.BaseDir,
			MarkdownFile:    persistedProj.CurrentDiscussionFile,
			AuthorizedFiles: persistedProj.AuthorizedFiles,
		}

		// Create Chat instance from current discussion file
		if persistedProj.CurrentDiscussionFile != "" {
			project.Chat = NewChat(persistedProj.CurrentDiscussionFile)
		}

		// Create ClientPool for this project
		project.ClientPool = NewClientPool()
		go project.ClientPool.Start()

		p.data[project.ID] = project
		log.Printf("Loaded project from database: %s with %d authorized files", project.ID, len(project.AuthorizedFiles))
	}

	return nil
}

// Add adds a new project to the registry and persists to database
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

	// Create the Chat instance for this project
	chatInstance := NewChat(markdownFile)
	if chatInstance == nil {
		return nil, fmt.Errorf("failed to create chat instance for project %s", projectID)
	}

	// Create ClientPool for this project
	clientPool := NewClientPool()
	go clientPool.Start()

	// Create Project struct
	project := &Project{
		ID:              projectID,
		BaseDir:         baseDir,
		MarkdownFile:    markdownFile,
		AuthorizedFiles: []string{},
		Chat:            chatInstance,
		ClientPool:      clientPool,
	}

	// Create persistent metadata for database storage
	persistedProj := &db.Project{
		ID:                    projectID,
		BaseDir:               baseDir,
		CurrentDiscussionFile: markdownFile,
		DiscussionFiles: []db.DiscussionFileRef{
			{
				Filepath:  markdownFile,
				CreatedAt: time.Now(),
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

	// Register project in memory cache
	p.mutex.Lock()
	p.data[projectID] = project
	p.mutex.Unlock()

	log.Printf("Successfully registered project %s with %d chat rounds", projectID, len(chatInstance.history))

	return project, nil
}

// Get retrieves a project by ID
func (p *Projects) Get(projectID string) (*Project, bool) {
	p.mutex.RLock()
	defer p.mutex.RUnlock()
	project, exists := p.data[projectID]
	return project, exists
}

// List returns all project IDs
func (p *Projects) List() []string {
	p.mutex.RLock()
	defer p.mutex.RUnlock()
	var ids []string
	for id := range p.data {
		ids = append(ids, id)
	}
	return ids
}

// Remove removes a project by ID and deletes from database
func (p *Projects) Remove(projectID string) error {
	// Delete from database
	if err := p.dbMgr.DeleteProject(projectID); err != nil {
		return fmt.Errorf("failed to delete project from database: %w", err)
	}

	// Remove from memory cache
	p.mutex.Lock()
	defer p.mutex.Unlock()
	if _, exists := p.data[projectID]; !exists {
		return fmt.Errorf("project %s not found", projectID)
	}
	delete(p.data, projectID)
	log.Printf("Removed project %s", projectID)
	return nil
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

// AddFile adds a file to the authorized files list and persists
func (p *Project) AddFile(filename string, dbMgr *db.Manager) error {
	if filename == "" {
		return fmt.Errorf("filename cannot be empty")
	}
	for _, f := range p.AuthorizedFiles {
		if f == filename {
			return fmt.Errorf("file %s already in authorized list", filename)
		}
	}
	p.AuthorizedFiles = append(p.AuthorizedFiles, filename)
	log.Printf("Added file %s to project %s", filename, p.ID)

	// Persist to database
	persistedProj := &db.Project{
		ID:                    p.ID,
		BaseDir:               p.BaseDir,
		CurrentDiscussionFile: p.MarkdownFile,
		DiscussionFiles: []db.DiscussionFileRef{
			{
				Filepath:  p.MarkdownFile,
				CreatedAt: time.Now(),
				RoundCount: p.Chat.TotalRounds(),
			},
		},
		AuthorizedFiles: p.AuthorizedFiles,
		CreatedAt:       time.Now(),
		EmbeddingCount:  0,
		RoundHistory:    []db.RoundEntry{},
	}
	return dbMgr.SaveProject(persistedProj)
}