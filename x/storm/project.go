package main

import (
	"fmt"
	"log"
	"os"
)

// projectAdd creates a new project and initializes its chat history
// TODO should be a method of Projects map?
func projectAdd(projectID, baseDir, markdownFile string) (*Chat, error) {
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
		ID:         projectID,
		BaseDir:    baseDir,
		Chat:       chatInstance,
		ClientPool: clientPool,
	}

	// Register project in the projects registry
	projectsLock.Lock()
	projects[projectID] = project
	projectsLock.Unlock()

	log.Printf("Successfully registered project %s with %d chat rounds", projectID, len(chatInstance.history))

	// TODO: Store project metadata in registry (KV store)

	return chatInstance, nil
}
