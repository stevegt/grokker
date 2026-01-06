package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stevegt/grokker/x/storm/db"
)

func TestGetSeedsDiscussionFiles(t *testing.T) {
	tmpDir := t.TempDir()
	projectID := "seed-discussions-project"
	projectDir := filepath.Join(tmpDir, projectID)
	if err := os.MkdirAll(projectDir, 0755); err != nil {
		t.Fatalf("Failed to create project directory: %v", err)
	}

	markdownFile := filepath.Join(projectDir, "chat.md")
	if err := os.WriteFile(markdownFile, []byte(""), 0644); err != nil {
		t.Fatalf("Failed to create markdown file: %v", err)
	}

	dbPath := filepath.Join(tmpDir, "storm.db")
	dbMgr, err := db.NewManager(dbPath)
	if err != nil {
		t.Fatalf("Failed to create database manager: %v", err)
	}
	t.Cleanup(func() {
		dbMgr.Close()
	})

	err = dbMgr.SaveProject(&db.Project{
		ID:                    projectID,
		BaseDir:               projectDir,
		CurrentDiscussionFile: markdownFile,
		DiscussionFiles:       []db.DiscussionFileRef{},
		AuthorizedFiles:       []string{},
		CreatedAt:             time.Now(),
	})
	if err != nil {
		t.Fatalf("Failed to save project: %v", err)
	}

	projects := NewProjectsWithDB(dbMgr)
	project, err := projects.Get(projectID)
	if err != nil {
		t.Fatalf("Failed to load project: %v", err)
	}

	if len(project.DiscussionFiles) != 1 {
		t.Fatalf("Expected 1 discussion file, got %d", len(project.DiscussionFiles))
	}
	if project.DiscussionFiles[0].Filepath != markdownFile {
		t.Fatalf("Expected discussion file %s, got %s", markdownFile, project.DiscussionFiles[0].Filepath)
	}

	meta, err := dbMgr.LoadProject(projectID)
	if err != nil {
		t.Fatalf("Failed to reload project metadata: %v", err)
	}
	if len(meta.DiscussionFiles) != 1 {
		t.Fatalf("Expected 1 discussion file in metadata, got %d", len(meta.DiscussionFiles))
	}
}
