package main

import (
	"fmt"
	"log"
	"os"
	"sync"
)

// Projects is a thread-safe wrapper around project registry
type Projects struct {
	data  map[string]*Project
	mutex sync.RWMutex
}

// NewProjects creates a new Projects registry
func NewProjects() *Projects {
	return &Projects{
		data: make(map[string]*Project),
	}
}

// Add adds a new project to the registry
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

	// Register project in the registry
	p.mutex.Lock()
	p.data[projectID] = project
	p.mutex.Unlock()

	log.Printf("Successfully registered project %s with %d chat rounds", projectID, len(chatInstance.history))

	// TODO: Store project metadata in KV store for persistence

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

// Remove removes a project by ID
func (p *Projects) Remove(projectID string) error {
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

// AddFile adds a file to the authorized files list
func (p *Project) AddFile(filename string) error {
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
	return nil
}


