package main

import (
	"fmt"
	"time"
)

// LoadConfig retrieves the daemon configuration from the KV store
func LoadConfig(db KVStore) (*Config, error) {
	var config Config
	err := db.View(func(tx ReadTx) error {
		return LoadCBORIfExists(tx, "config", "config", &config)
	})
	if err != nil {
		return nil, fmt.Errorf("failed to load config: %w", err)
	}

	// Return defaults if config doesn't exist yet
	if config.Port == 0 {
		config.Port = 8080
		config.EmbeddingModel = "nomic-embed-text"
		config.Ollama.Endpoint = "http://localhost:11434"
		config.HNSW.M = 16
		config.HNSW.EfConstruction = 200
		config.HNSW.EfSearch = 100
	}

	return &config, nil
}

// SaveConfig persists the daemon configuration to the KV store
func SaveConfig(db KVStore, config *Config) error {
	err := db.Update(func(tx WriteTx) error {
		return StoreCBOR(tx, "config", "config", config)
	})
	if err != nil {
		return fmt.Errorf("failed to save config: %w", err)
	}
	return nil
}

// LoadProjectRegistry loads all projects from the KV store
func LoadProjectRegistry(db KVStore) (map[string]*Project, error) {
	projects := make(map[string]*Project)

	err := db.View(func(tx ReadTx) error {
		return tx.ForEach("projects", func(k, v []byte) error {
			var project Project
			err := UnmarshalCBOR(v, &project)
			if err != nil {
				return fmt.Errorf("failed to unmarshal project: %w", err)
			}
			projects[project.ID] = &project

			// Initialize Chat for current discussion file
			if project.CurrentDiscussionFile != "" {
				chatFilePath := project.BaseDir + "/" + project.CurrentDiscussionFile
				project.Chat = NewChat(chatFilePath)
			}

			return nil
		})
	})

	if err != nil {
		return nil, fmt.Errorf("failed to load project registry: %w", err)
	}

	return projects, nil
}

// SaveProject persists a single project to the KV store
func SaveProject(db KVStore, project *Project) error {
	if project.ID == "" {
		return fmt.Errorf("project ID cannot be empty")
	}

	err := db.Update(func(tx WriteTx) error {
		return StoreCBOR(tx, "projects", project.ID, project)
	})

	if err != nil {
		return fmt.Errorf("failed to save project %s: %w", project.ID, err)
	}

	return nil
}

// GetProject retrieves a single project by ID from the KV store
func GetProject(db KVStore, projectID string) (*Project, error) {
	var project Project

	err := db.View(func(tx ReadTx) error {
		return LoadCBOR(tx, "projects", projectID, &project)
	})

	if err != nil {
		return nil, fmt.Errorf("project %s not found: %w", projectID, err)
	}

	// Initialize Chat for current discussion file
	if project.CurrentDiscussionFile != "" {
		chatFilePath := project.BaseDir + "/" + project.CurrentDiscussionFile
		project.Chat = NewChat(chatFilePath)
	}

	return &project, nil
}

// RemoveProject deletes a project from the KV store
func RemoveProject(db KVStore, projectID string) error {
	err := db.Update(func(tx WriteTx) error {
		return tx.Delete("projects", projectID)
	})

	if err != nil {
		return fmt.Errorf("failed to remove project %s: %w", projectID, err)
	}

	return nil
}

// ListProjects returns all projects from the KV store as a slice
func ListProjects(db KVStore) ([]*Project, error) {
	var projects []*Project

	err := db.View(func(tx ReadTx) error {
		return tx.ForEach("projects", func(k, v []byte) error {
			var project Project
			err := UnmarshalCBOR(v, &project)
			if err != nil {
				return fmt.Errorf("failed to unmarshal project: %w", err)
			}

			// Initialize Chat for current discussion file
			if project.CurrentDiscussionFile != "" {
				chatFilePath := project.BaseDir + "/" + project.CurrentDiscussionFile
				project.Chat = NewChat(chatFilePath)
			}

			projects = append(projects, &project)
			return nil
		})
	})

	if err != nil {
		return nil, fmt.Errorf("failed to list projects: %w", err)
	}

	return projects, nil
}

// ProjectExists checks if a project with the given ID exists
func ProjectExists(db KVStore, projectID string) (bool, error) {
	var exists bool

	err := db.View(func(tx ReadTx) error {
		exists = tx.Get("projects", projectID) != nil
		return nil
	})

	return exists, err
}

// AddProjectMetadata creates or updates a project with proper initialization
func AddProjectMetadata(db KVStore, projectID, baseDir, markdownFile string) (*Project, error) {
	// Check if project already exists
	existingProject, err := GetProject(db, projectID)
	if err == nil && existingProject != nil {
		return nil, fmt.Errorf("project %s already exists", projectID)
	}

	project := &Project{
		ID:                    projectID,
		BaseDir:               baseDir,
		CurrentDiscussionFile: markdownFile,
		DiscussionFiles: []DiscussionFileRef{
			{
				Filepath:  markdownFile,
				CreatedAt: time.Now(),
				RoundCount: 0,
			},
		},
		AuthorizedFiles: []string{},
		CreatedAt:       time.Now(),
		EmbeddingCount:  0,
		RoundHistory:    []RoundEntry{},
	}

	// Initialize Chat
	chatFilePath := baseDir + "/" + markdownFile
	project.Chat = NewChat(chatFilePath)

	// Persist to KV store
	if err := SaveProject(db, project); err != nil {
		return nil, fmt.Errorf("failed to create project: %w", err)
	}

	return project, nil
}

// UpdateProjectAuthorizedFiles updates the authorized files list for a project
func UpdateProjectAuthorizedFiles(db KVStore, projectID string, files []string) error {
	err := db.Update(func(tx WriteTx) error {
		var project Project
		err := LoadCBOR(tx, "projects", projectID, &project)
		if err != nil {
			return fmt.Errorf("project not found: %w", err)
		}

		project.AuthorizedFiles = files
		return StoreCBOR(tx, "projects", projectID, &project)
	})

	return err
}

// SwitchDiscussionFile changes the active discussion file for a project
func SwitchDiscussionFile(db KVStore, projectID, newDiscussionFile string) error {
	err := db.Update(func(tx WriteTx) error {
		var project Project
		err := LoadCBOR(tx, "projects", projectID, &project)
		if err != nil {
			return fmt.Errorf("project not found: %w", err)
		}

		// Verify the discussion file exists in the project's discussion files
		found := false
		for _, df := range project.DiscussionFiles {
			if df.Filepath == newDiscussionFile {
				found = true
				break
			}
		}

		if !found {
			return fmt.Errorf("discussion file %s not found in project", newDiscussionFile)
		}

		project.CurrentDiscussionFile = newDiscussionFile

		// Reinitialize Chat for the new discussion file
		chatFilePath := project.BaseDir + "/" + newDiscussionFile
		project.Chat = NewChat(chatFilePath)

		return StoreCBOR(tx, "projects", projectID, &project)
	})

	return err
}

// CreateDiscussionFile adds a new discussion file to a project
func CreateDiscussionFile(db KVStore, projectID, filename string) error {
	err := db.Update(func(tx WriteTx) error {
		var project Project
		err := LoadCBOR(tx, "projects", projectID, &project)
		if err != nil {
			return fmt.Errorf("project not found: %w", err)
		}

		// Check if discussion file already exists
		for _, df := range project.DiscussionFiles {
			if df.Filepath == filename {
				return fmt.Errorf("discussion file %s already exists", filename)
			}
		}

		// Add new discussion file
		newDiscussionFile := DiscussionFileRef{
			Filepath:  filename,
			CreatedAt: time.Now(),
			RoundCount: 0,
		}
		project.DiscussionFiles = append(project.DiscussionFiles, newDiscussionFile)

		return StoreCBOR(tx, "projects", projectID, &project)
	})

	return err
}

// GetDiscussionFiles returns all discussion files for a project
func GetDiscussionFiles(db KVStore, projectID string) ([]DiscussionFileRef, error) {
	var project Project

	err := db.View(func(tx ReadTx) error {
		return LoadCBOR(tx, "projects", projectID, &project)
	})

	if err != nil {
		return nil, fmt.Errorf("project %s not found: %w", projectID, err)
	}

	return project.DiscussionFiles, nil
}

// IncrementRoundCount increments the round count for a discussion file
func IncrementRoundCount(db KVStore, projectID, discussionFile string) error {
	err := db.Update(func(tx WriteTx) error {
		var project Project
		err := LoadCBOR(tx, "projects", projectID, &project)
		if err != nil {
			return fmt.Errorf("project not found: %w", err)
		}

		// Find and increment the discussion file's round count
		for i := 0; i < len(project.DiscussionFiles); i++ {
			if project.DiscussionFiles[i].Filepath == discussionFile {
				project.DiscussionFiles[i].RoundCount++
				return StoreCBOR(tx, "projects", projectID, &project)
			}
		}

		return fmt.Errorf("discussion file %s not found", discussionFile)
	})

	return err
}

// SaveRound appends a new round to a project's round history
func SaveRound(db KVStore, projectID string, round RoundEntry) error {
	err := db.Update(func(tx WriteTx) error {
		var project Project
		err := LoadCBOR(tx, "projects", projectID, &project)
		if err != nil {
			return fmt.Errorf("project not found: %w", err)
		}

		// Set timestamp if not already set
		if round.Timestamp.IsZero() {
			round.Timestamp = time.Now()
		}

		project.RoundHistory = append(project.RoundHistory, round)
		project.EmbeddingCount += len(round.CIDs)

		return StoreCBOR(tx, "projects", projectID, &project)
	})

	return err
}

