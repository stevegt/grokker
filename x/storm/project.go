package main

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/stevegt/grokker/x/storm/db"
)

// TODO move this file to a ./projects package?

// Project encapsulates project-specific data and state.
type Project struct {
	ID              string
	BaseDir         string
	MarkdownFile    string
	AuthorizedFiles []string
	DiscussionFiles []db.DiscussionFileRef
	Chat            *Chat
	ClientPool      *ClientPool
}

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
		DiscussionFiles: meta.DiscussionFiles,
		Chat:            NewChat(meta.CurrentDiscussionFile),
		ClientPool:      NewClientPool(),
	}

	if len(meta.DiscussionFiles) == 0 && meta.CurrentDiscussionFile != "" {
		meta.DiscussionFiles = []db.DiscussionFileRef{
			{
				Filepath:   meta.CurrentDiscussionFile,
				CreatedAt:  time.Now(),
				RoundCount: len(project.Chat.history),
			},
		}
		project.DiscussionFiles = meta.DiscussionFiles
		if err := p.dbMgr.SaveProject(meta); err != nil {
			return nil, fmt.Errorf("failed to backfill discussion files: %w", err)
		}
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
		DiscussionFiles: []db.DiscussionFileRef{
			{
				Filepath:   markdownFile,
				CreatedAt:  time.Now(),
				RoundCount: len(chatInstance.history),
			},
		},
		Chat:       chatInstance,
		ClientPool: clientPool,
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

// UpdateBaseDir updates a project's base directory in cache and persistent storage.
func (p *Projects) UpdateBaseDir(projectID, baseDir string) (*Project, error) {
	if baseDir == "" {
		return nil, fmt.Errorf("baseDir cannot be empty")
	}
	if _, err := os.Stat(baseDir); os.IsNotExist(err) {
		return nil, fmt.Errorf("base directory does not exist: %s", baseDir)
	}

	// Load project for runtime state (cache-backed) so we can update live fields.
	project, err := p.Get(projectID)
	if err != nil {
		return nil, fmt.Errorf("project not found: %w", err)
	}

	oldBaseDir := project.BaseDir
	// Rewrite only paths that live under the old baseDir; leave absolute paths outside untouched.
	project.BaseDir = baseDir

	// Load persisted metadata separately to avoid clobbering fields we don't manage in memory.
	persistedProj, err := p.dbMgr.LoadProject(projectID)
	if err != nil {
		return nil, fmt.Errorf("failed to load project metadata: %w", err)
	}
	persistedProj.BaseDir = baseDir

	rewritePath := func(path string) string {
		if path == "" {
			return path
		}
		cleanPath := filepath.Clean(path)
		cleanOld := filepath.Clean(oldBaseDir)
		if cleanPath == cleanOld {
			// Preserve the baseDir root when the old path equals the baseDir itself.
			return baseDir
		}
		withSep := cleanOld + string(os.PathSeparator)
		if strings.HasPrefix(cleanPath, withSep) {
			// Rewrite only paths under the old baseDir to the new baseDir.
			return filepath.Join(baseDir, cleanPath[len(withSep):])
		}
		return path
	}

	// Update runtime paths and reload chat if the active discussion file moved.
	project.MarkdownFile = rewritePath(project.MarkdownFile)
	if project.MarkdownFile != persistedProj.CurrentDiscussionFile {
		project.Chat = NewChat(project.MarkdownFile)
	}

	// Rewrite tracked files in both runtime state and persisted metadata.
	project.AuthorizedFiles = rewritePathSlice(project.AuthorizedFiles, rewritePath)
	persistedProj.AuthorizedFiles = rewritePathSlice(persistedProj.AuthorizedFiles, rewritePath)

	for i := 0; i < len(project.DiscussionFiles); i++ {
		project.DiscussionFiles[i].Filepath = rewritePath(project.DiscussionFiles[i].Filepath)
	}
	for i := 0; i < len(persistedProj.DiscussionFiles); i++ {
		persistedProj.DiscussionFiles[i].Filepath = rewritePath(persistedProj.DiscussionFiles[i].Filepath)
	}
	persistedProj.CurrentDiscussionFile = rewritePath(persistedProj.CurrentDiscussionFile)

	if err := p.dbMgr.SaveProject(persistedProj); err != nil {
		return nil, fmt.Errorf("failed to save project metadata: %w", err)
	}

	return project, nil
}

func rewritePathSlice(paths []string, rewrite func(string) string) []string {
	rewritten := make([]string, 0, len(paths))
	for i := 0; i < len(paths); i++ {
		rewritten = append(rewritten, rewrite(paths[i]))
	}
	return rewritten
}

// AddDiscussionFile adds a discussion markdown file to a project.
func (p *Projects) AddDiscussionFile(projectID, filename string) (*Project, error) {
	if filename == "" {
		return nil, fmt.Errorf("filename cannot be empty")
	}

	project, err := p.Get(projectID)
	if err != nil {
		return nil, fmt.Errorf("project not found: %w", err)
	}

	absFilename := filename
	if !filepath.IsAbs(filename) {
		absFilename = filepath.Join(project.BaseDir, filename)
	}

	if _, err := os.Stat(absFilename); err != nil {
		if os.IsNotExist(err) {
			if err := os.WriteFile(absFilename, []byte(""), 0644); err != nil {
				return nil, fmt.Errorf("failed to create discussion file: %w", err)
			}
		} else {
			return nil, fmt.Errorf("failed to stat discussion file: %w", err)
		}
	}

	for _, ref := range project.DiscussionFiles {
		if ref.Filepath == absFilename {
			return nil, fmt.Errorf("discussion file already exists: %s", filename)
		}
	}

	chat := NewChat(absFilename)
	project.DiscussionFiles = append(project.DiscussionFiles, db.DiscussionFileRef{
		Filepath:   absFilename,
		CreatedAt:  time.Now(),
		RoundCount: chat.TotalRounds(),
	})

	persistedProj, err := p.dbMgr.LoadProject(projectID)
	if err != nil {
		return nil, fmt.Errorf("failed to load project metadata: %w", err)
	}
	persistedProj.DiscussionFiles = project.DiscussionFiles

	if err := p.dbMgr.SaveProject(persistedProj); err != nil {
		return nil, fmt.Errorf("failed to save project metadata: %w", err)
	}

	return project, nil
}

// ForgetDiscussionFile removes a discussion markdown file from a project.
func (p *Projects) ForgetDiscussionFile(projectID, filename string) (*Project, error) {
	if filename == "" {
		return nil, fmt.Errorf("filename cannot be empty")
	}

	project, err := p.Get(projectID)
	if err != nil {
		return nil, fmt.Errorf("project not found: %w", err)
	}

	absFilename := filename
	if !filepath.IsAbs(filename) {
		absFilename = filepath.Join(project.BaseDir, filename)
	}

	if absFilename == project.MarkdownFile {
		return nil, fmt.Errorf("cannot forget current discussion file")
	}

	if len(project.DiscussionFiles) <= 1 {
		return nil, fmt.Errorf("cannot forget the only discussion file")
	}

	idx := -1
	for i, ref := range project.DiscussionFiles {
		if ref.Filepath == absFilename {
			idx = i
			break
		}
	}
	if idx == -1 {
		return nil, fmt.Errorf("discussion file not found: %s", filename)
	}

	project.DiscussionFiles = append(project.DiscussionFiles[:idx], project.DiscussionFiles[idx+1:]...)

	persistedProj, err := p.dbMgr.LoadProject(projectID)
	if err != nil {
		return nil, fmt.Errorf("failed to load project metadata: %w", err)
	}
	persistedProj.DiscussionFiles = project.DiscussionFiles

	if err := p.dbMgr.SaveProject(persistedProj); err != nil {
		return nil, fmt.Errorf("failed to save project metadata: %w", err)
	}

	return project, nil
}

// SwitchDiscussionFile switches the active discussion markdown file.
func (p *Projects) SwitchDiscussionFile(projectID, filename string) (*Project, error) {
	if filename == "" {
		return nil, fmt.Errorf("filename cannot be empty")
	}

	project, err := p.Get(projectID)
	if err != nil {
		return nil, fmt.Errorf("project not found: %w", err)
	}

	absFilename := filename
	if !filepath.IsAbs(filename) {
		absFilename = filepath.Join(project.BaseDir, filename)
	}

	found := false
	for _, ref := range project.DiscussionFiles {
		if ref.Filepath == absFilename {
			found = true
			break
		}
	}
	if !found {
		return nil, fmt.Errorf("discussion file not found: %s", filename)
	}

	project.MarkdownFile = absFilename
	project.Chat = NewChat(absFilename)

	persistedProj, err := p.dbMgr.LoadProject(projectID)
	if err != nil {
		return nil, fmt.Errorf("failed to load project metadata: %w", err)
	}
	persistedProj.CurrentDiscussionFile = absFilename

	if err := p.dbMgr.SaveProject(persistedProj); err != nil {
		return nil, fmt.Errorf("failed to save project metadata: %w", err)
	}

	return project, nil
}

// AddFile adds a file to a project's authorized files
func (p *Projects) AddFile(projectID, filename string) error {
	if filename == "" {
		return fmt.Errorf("filename cannot be empty")
	}

	// Load project for runtime state (cache-backed) so we can update live fields.
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

	// Load persisted metadata separately to avoid clobbering fields we don't manage in memory.
	persistedProj, err := p.dbMgr.LoadProject(projectID)
	if err != nil {
		return fmt.Errorf("failed to load project metadata: %w", err)
	}
	persistedProj.AuthorizedFiles = project.AuthorizedFiles
	persistedProj.CurrentDiscussionFile = project.MarkdownFile

	// Persist to database
	return p.dbMgr.SaveProject(persistedProj)
}

// Add RemoveFile method to Projects struct:
func (p *Projects) RemoveFile(projectID, filename string) error {
	// Load project for runtime state (cache-backed) so we can update live fields.
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

	// Load persisted metadata separately to avoid clobbering fields we don't manage in memory.
	persistedProj, err := p.dbMgr.LoadProject(projectID)
	if err != nil {
		return fmt.Errorf("failed to load project metadata: %w", err)
	}
	persistedProj.AuthorizedFiles = project.AuthorizedFiles
	persistedProj.CurrentDiscussionFile = project.MarkdownFile

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

// GetDiscussionFilesAsRelative returns discussion file paths relative to BaseDir when possible.
func (p *Project) GetDiscussionFilesAsRelative() []string {
	var relativeFiles []string
	for i := 0; i < len(p.DiscussionFiles); i++ {
		relativeFiles = append(relativeFiles, p.toRelativePath(p.DiscussionFiles[i].Filepath))
	}
	return relativeFiles
}
