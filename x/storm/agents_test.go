package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBuildAgentsInstructions_UsesGitRootWhenPresent(t *testing.T) {
	t.Parallel()

	rootDir := t.TempDir()
	if err := os.Mkdir(filepath.Join(rootDir, ".git"), 0o755); err != nil {
		t.Fatalf("create .git dir: %v", err)
	}

	rootAgents := "root agents instructions"
	if err := os.WriteFile(filepath.Join(rootDir, "AGENTS.md"), []byte(rootAgents), 0o644); err != nil {
		t.Fatalf("write root AGENTS.md: %v", err)
	}

	subDir := filepath.Join(rootDir, "sub")
	if err := os.MkdirAll(subDir, 0o755); err != nil {
		t.Fatalf("create subdir: %v", err)
	}

	subAgents := "sub agents instructions"
	if err := os.WriteFile(filepath.Join(subDir, "AGENTS.md"), []byte(subAgents), 0o644); err != nil {
		t.Fatalf("write sub AGENTS.md: %v", err)
	}

	if err := os.WriteFile(filepath.Join(subDir, "file.go"), []byte("package p\n"), 0o644); err != nil {
		t.Fatalf("write file.go: %v", err)
	}

	project := &Project{BaseDir: subDir}
	instructions, err := buildAgentsInstructions(project, nil, []string{"file.go"})
	if err != nil {
		t.Fatalf("buildAgentsInstructions: %v", err)
	}
	if !strings.Contains(instructions, rootAgents) {
		t.Fatalf("expected root agents instructions to be included, got:\n%s", instructions)
	}
	if !strings.Contains(instructions, subAgents) {
		t.Fatalf("expected sub agents instructions to be included, got:\n%s", instructions)
	}
	if strings.Index(instructions, rootAgents) > strings.Index(instructions, subAgents) {
		t.Fatalf("expected root agents instructions to appear before sub agents instructions, got:\n%s", instructions)
	}
}

func TestBuildAgentsInstructions_DoesNotScanAboveBaseDirWithoutGit(t *testing.T) {
	t.Parallel()

	rootDir := t.TempDir()

	rootAgents := "root agents instructions"
	if err := os.WriteFile(filepath.Join(rootDir, "AGENTS.md"), []byte(rootAgents), 0o644); err != nil {
		t.Fatalf("write root AGENTS.md: %v", err)
	}

	subDir := filepath.Join(rootDir, "sub")
	if err := os.MkdirAll(subDir, 0o755); err != nil {
		t.Fatalf("create subdir: %v", err)
	}

	subAgents := "sub agents instructions"
	if err := os.WriteFile(filepath.Join(subDir, "AGENTS.md"), []byte(subAgents), 0o644); err != nil {
		t.Fatalf("write sub AGENTS.md: %v", err)
	}

	if err := os.WriteFile(filepath.Join(subDir, "file.go"), []byte("package p\n"), 0o644); err != nil {
		t.Fatalf("write file.go: %v", err)
	}

	project := &Project{BaseDir: subDir}
	instructions, err := buildAgentsInstructions(project, nil, []string{"file.go"})
	if err != nil {
		t.Fatalf("buildAgentsInstructions: %v", err)
	}
	if strings.Contains(instructions, rootAgents) {
		t.Fatalf("did not expect root agents instructions to be included, got:\n%s", instructions)
	}
	if !strings.Contains(instructions, subAgents) {
		t.Fatalf("expected sub agents instructions to be included, got:\n%s", instructions)
	}
}

func TestBuildAgentsInstructions_IncludesProjectAGENTSWhenNoFiles(t *testing.T) {
	t.Parallel()

	rootDir := t.TempDir()

	agents := "root agents instructions"
	if err := os.WriteFile(filepath.Join(rootDir, "AGENTS.md"), []byte(agents), 0o644); err != nil {
		t.Fatalf("write AGENTS.md: %v", err)
	}

	project := &Project{BaseDir: rootDir}
	instructions, err := buildAgentsInstructions(project, nil, nil)
	if err != nil {
		t.Fatalf("buildAgentsInstructions: %v", err)
	}
	if !strings.Contains(instructions, agents) {
		t.Fatalf("expected agents instructions to be included, got:\n%s", instructions)
	}
}

func TestBuildAgentsInstructions_IncludesProjectAGENTSWhenOnlyOutsideFiles(t *testing.T) {
	t.Parallel()

	rootDir := t.TempDir()
	if err := os.Mkdir(filepath.Join(rootDir, ".git"), 0o755); err != nil {
		t.Fatalf("create .git dir: %v", err)
	}

	agents := "root agents instructions"
	if err := os.WriteFile(filepath.Join(rootDir, "AGENTS.md"), []byte(agents), 0o644); err != nil {
		t.Fatalf("write AGENTS.md: %v", err)
	}

	outsideDir := t.TempDir()
	outFile := filepath.Join(outsideDir, "out.go")

	project := &Project{BaseDir: rootDir}
	instructions, err := buildAgentsInstructions(project, nil, []string{outFile})
	if err != nil {
		t.Fatalf("buildAgentsInstructions: %v", err)
	}
	if !strings.Contains(instructions, agents) {
		t.Fatalf("expected project agents instructions to be included, got:\n%s", instructions)
	}
}
