package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type agentsDoc struct {
	AbsPath string
	RelPath string
	Depth   int
	Content string
}

func buildAgentsInstructions(project *Project, inputFiles, outFiles []string) (string, error) {
	if project == nil {
		return "", nil
	}

	projectRoot, err := findProjectRoot(project.BaseDir)
	if err != nil {
		return "", err
	}
	projectRoot = filepath.Clean(projectRoot)

	targets := make([]string, 0, len(outFiles)+len(inputFiles)+1)
	targets = append(targets, project.BaseDir)
	targets = append(targets, outFiles...)
	targets = append(targets, inputFiles...)

	agentsFiles := make(map[string]struct{})
	for _, target := range targets {
		absTarget := resolveFilePath(project, target)
		absTarget, err := filepath.Abs(absTarget)
		if err != nil {
			absTarget = filepath.Clean(absTarget)
		}

		startDir := absTarget
		if info, statErr := os.Stat(absTarget); statErr == nil && info.IsDir() {
			startDir = absTarget
		} else {
			startDir = filepath.Dir(absTarget)
		}

		if !isWithinDir(startDir, projectRoot) {
			continue
		}

		dir := startDir
		for {
			agentsPath := filepath.Join(dir, "AGENTS.md")
			if isRegularFile(agentsPath) {
				agentsFiles[agentsPath] = struct{}{}
			}

			if filepath.Clean(dir) == projectRoot {
				break
			}
			parent := filepath.Dir(dir)
			if parent == dir {
				break
			}
			dir = parent
		}
	}

	if len(agentsFiles) == 0 {
		return "", nil
	}

	docs := make([]agentsDoc, 0, len(agentsFiles))
	var errs error
	for agentsPath := range agentsFiles {
		content, err := os.ReadFile(agentsPath)
		if err != nil {
			errs = errors.Join(errs, fmt.Errorf("read %s: %w", agentsPath, err))
			continue
		}

		relPath, err := filepath.Rel(projectRoot, agentsPath)
		if err != nil {
			relPath = agentsPath
		}
		relPath = filepath.Clean(relPath)
		depth := strings.Count(relPath, string(os.PathSeparator))

		docs = append(docs, agentsDoc{
			AbsPath: agentsPath,
			RelPath: relPath,
			Depth:   depth,
			Content: string(content),
		})
	}

	sort.Slice(docs, func(i, j int) bool {
		if docs[i].Depth != docs[j].Depth {
			return docs[i].Depth < docs[j].Depth
		}
		return docs[i].RelPath < docs[j].RelPath
	})

	var b strings.Builder
	for i := 0; i < len(docs); i++ {
		if i > 0 {
			b.WriteString("\n\n---\n\n")
		}
		fmt.Fprintf(&b, "AGENTS.md (%s)\n\n", docs[i].RelPath)
		b.WriteString(strings.TrimSpace(docs[i].Content))
	}

	return b.String(), errs
}

func findProjectRoot(baseDir string) (string, error) {
	absBaseDir, err := filepath.Abs(baseDir)
	if err != nil {
		return "", err
	}

	dir := absBaseDir
	for {
		if _, err := os.Stat(filepath.Join(dir, ".git")); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return absBaseDir, nil
		}
		dir = parent
	}
}

func isWithinDir(path string, dir string) bool {
	rel, err := filepath.Rel(dir, path)
	if err != nil {
		return false
	}
	if rel == "." {
		return true
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
		return false
	}
	return true
}

func isRegularFile(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	return info.Mode().IsRegular()
}
