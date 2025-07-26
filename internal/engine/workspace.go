package engine

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

// WorkspaceManager provides copy-on-write workspace isolation for concurrent executions
type WorkspaceManager struct {
	baseDir    string
	workspaces map[string]*Workspace
	mu         sync.RWMutex
}

// Workspace represents an isolated execution environment
type Workspace struct {
	RunID      string
	Path       string
	BaseRepo   string
	IsIsolated bool
	mu         sync.RWMutex
}

// NewWorkspaceManager creates a new workspace manager
func NewWorkspaceManager(baseDir string) (*WorkspaceManager, error) {
	if err := os.MkdirAll(baseDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create workspace base directory: %v", err)
	}

	return &WorkspaceManager{
		baseDir:    baseDir,
		workspaces: make(map[string]*Workspace),
	}, nil
}

// CreateWorkspace creates a new isolated workspace for a run
func (wm *WorkspaceManager) CreateWorkspace(runID, baseRepoPath string) (*Workspace, error) {
	wm.mu.Lock()
	defer wm.mu.Unlock()

	// Check if workspace already exists
	if ws, exists := wm.workspaces[runID]; exists {
		return ws, nil
	}

	workspacePath := filepath.Join(wm.baseDir, runID)

	// Create workspace directory
	if err := os.MkdirAll(workspacePath, 0755); err != nil {
		return nil, fmt.Errorf("failed to create workspace directory: %v", err)
	}

	workspace := &Workspace{
		RunID:      runID,
		Path:       workspacePath,
		BaseRepo:   baseRepoPath,
		IsIsolated: true,
	}

	wm.workspaces[runID] = workspace

	return workspace, nil
}

// GetWorkspace returns an existing workspace or creates a new one
func (wm *WorkspaceManager) GetWorkspace(runID string) (*Workspace, error) {
	wm.mu.RLock()
	ws, exists := wm.workspaces[runID]
	wm.mu.RUnlock()

	if exists {
		return ws, nil
	}

	return nil, fmt.Errorf("workspace for run %s not found", runID)
}

// PrepareRepository prepares a repository in the workspace using copy-on-write semantics
func (ws *Workspace) PrepareRepository(repoPath string) (string, error) {
	ws.mu.Lock()
	defer ws.mu.Unlock()

	if !ws.IsIsolated {
		// If not isolated, return the original path
		return repoPath, nil
	}

	// Create repository-specific directory in workspace
	repoName := filepath.Base(repoPath)
	workspaceRepoPath := filepath.Join(ws.Path, "repos", repoName)

	// Check if already prepared
	if _, err := os.Stat(workspaceRepoPath); err == nil {
		return workspaceRepoPath, nil
	}

	// Create directory structure
	if err := os.MkdirAll(filepath.Dir(workspaceRepoPath), 0755); err != nil {
		return "", fmt.Errorf("failed to create workspace repo directory: %v", err)
	}

	// For now, create a symbolic link to implement copy-on-write
	// In a production implementation, this might use filesystem-level COW
	// or create actual copies when modifications are needed
	if err := os.Symlink(repoPath, workspaceRepoPath); err != nil {
		return "", fmt.Errorf("failed to create repository link: %v", err)
	}

	return workspaceRepoPath, nil
}

// GetExecutionDir returns the directory for execution-specific files
func (ws *Workspace) GetExecutionDir() string {
	return filepath.Join(ws.Path, "execution")
}

// GetStateDir returns the directory for state files
func (ws *Workspace) GetStateDir() string {
	return filepath.Join(ws.Path, "state")
}

// GetLogsDir returns the directory for log files
func (ws *Workspace) GetLogsDir() string {
	return filepath.Join(ws.Path, "logs")
}

// GetTempDir returns the directory for temporary files
func (ws *Workspace) GetTempDir() string {
	return filepath.Join(ws.Path, "tmp")
}

// EnsureDirectories creates all necessary workspace directories
func (ws *Workspace) EnsureDirectories() error {
	ws.mu.Lock()
	defer ws.mu.Unlock()

	dirs := []string{
		ws.GetExecutionDir(),
		ws.GetStateDir(),
		ws.GetLogsDir(),
		ws.GetTempDir(),
		filepath.Join(ws.Path, "repos"),
	}

	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("failed to create directory %s: %v", dir, err)
		}
	}

	return nil
}

// CreateCopyOnWrite creates a writable copy of a file when modification is needed
func (ws *Workspace) CreateCopyOnWrite(originalPath, workspacePath string) error {
	ws.mu.Lock()
	defer ws.mu.Unlock()

	// Check if it's already a copy (not a symlink)
	if info, err := os.Lstat(workspacePath); err == nil && info.Mode()&os.ModeSymlink == 0 {
		// Already a copy, nothing to do
		return nil
	}

	// Remove symlink if it exists
	if err := os.Remove(workspacePath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to remove symlink: %v", err)
	}

	// Create actual copy
	return copyFile(originalPath, workspacePath)
}

// Cleanup removes the workspace and all its contents
func (ws *Workspace) Cleanup() error {
	ws.mu.Lock()
	defer ws.mu.Unlock()

	if err := os.RemoveAll(ws.Path); err != nil {
		return fmt.Errorf("failed to cleanup workspace: %v", err)
	}

	return nil
}

// CleanupWorkspace removes a specific workspace
func (wm *WorkspaceManager) CleanupWorkspace(runID string) error {
	wm.mu.Lock()
	defer wm.mu.Unlock()

	ws, exists := wm.workspaces[runID]
	if !exists {
		return nil // Already cleaned up
	}

	if err := ws.Cleanup(); err != nil {
		return err
	}

	delete(wm.workspaces, runID)
	return nil
}

// CleanupAll removes all workspaces
func (wm *WorkspaceManager) CleanupAll() error {
	wm.mu.Lock()
	defer wm.mu.Unlock()

	var errs []error

	for runID, ws := range wm.workspaces {
		if err := ws.Cleanup(); err != nil {
			errs = append(errs, fmt.Errorf("failed to cleanup workspace %s: %v", runID, err))
		}
	}

	wm.workspaces = make(map[string]*Workspace)

	if len(errs) > 0 {
		return fmt.Errorf("errors during cleanup: %v", errs)
	}

	return nil
}

// ListWorkspaces returns a list of all active workspaces
func (wm *WorkspaceManager) ListWorkspaces() []string {
	wm.mu.RLock()
	defer wm.mu.RUnlock()

	var runIDs []string
	for runID := range wm.workspaces {
		runIDs = append(runIDs, runID)
	}

	return runIDs
}

// copyFile copies a file from src to dst
func copyFile(src, dst string) error {
	sourceFile, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("failed to open source file: %v", err)
	}
	defer sourceFile.Close()

	// Create destination directory if needed
	if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
		return fmt.Errorf("failed to create destination directory: %v", err)
	}

	destFile, err := os.Create(dst)
	if err != nil {
		return fmt.Errorf("failed to create destination file: %v", err)
	}
	defer destFile.Close()

	// Copy file contents
	if _, err := sourceFile.WriteTo(destFile); err != nil {
		return fmt.Errorf("failed to copy file contents: %v", err)
	}

	// Copy file permissions
	if info, err := sourceFile.Stat(); err == nil {
		if err := destFile.Chmod(info.Mode()); err != nil {
			return fmt.Errorf("failed to set file permissions: %v", err)
		}
	}

	return nil
}
