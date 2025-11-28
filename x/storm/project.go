package main

import (
	"fmt"
	"log"
)

// projectAdd creates a new project and initializes its chat history
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

	log.Printf("Adding project: projectID=%s, baseDir=%s, markdownFile=%s", projectID, baseDir, markdownFile)

	// Create the Chat instance for this project
	chatInstance := NewChat(markdownFile)
	if chatInstance == nil {
		return nil, fmt.Errorf("failed to create chat instance for project %s", projectID)
	}

	log.Printf("Successfully created chat instance for project %s with %d rounds", projectID, len(chatInstance.history))

	// TODO: Store project metadata in registry (KV store)
	// TODO: Store per-project Chat instances in a project registry map
	// TODO: Add routing to select which project's chat to use for HTTP handlers

	return chatInstance, nil
}
