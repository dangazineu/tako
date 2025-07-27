package engine

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNewWorkspaceManager(t *testing.T) {
	tempDir := t.TempDir()

	wm, err := NewWorkspaceManager(tempDir)
	if err != nil {
		t.Fatalf("Failed to create workspace manager: %v", err)
	}

	if wm.baseDir != tempDir {
		t.Errorf("Expected base dir %s, got %s", tempDir, wm.baseDir)
	}

	if wm.workspaces == nil {
		t.Error("Workspaces map should be initialized")
	}

	// Verify directory was created
	if _, err := os.Stat(tempDir); os.IsNotExist(err) {
		t.Error("Base directory should be created")
	}
}

func TestWorkspaceManager_CreateWorkspace(t *testing.T) {
	tempDir := t.TempDir()
	wm, err := NewWorkspaceManager(tempDir)
	if err != nil {
		t.Fatalf("Failed to create workspace manager: %v", err)
	}

	runID := "test-run-123"
	baseRepoPath := "/tmp/test-repo"

	// Create workspace
	ws, err := wm.CreateWorkspace(runID, baseRepoPath)
	if err != nil {
		t.Fatalf("Failed to create workspace: %v", err)
	}

	// Verify workspace properties
	if ws.RunID != runID {
		t.Errorf("Expected run ID %s, got %s", runID, ws.RunID)
	}
	if ws.BaseRepo != baseRepoPath {
		t.Errorf("Expected base repo %s, got %s", baseRepoPath, ws.BaseRepo)
	}
	if !ws.IsIsolated {
		t.Error("Workspace should be isolated")
	}

	expectedPath := filepath.Join(tempDir, runID)
	if ws.Path != expectedPath {
		t.Errorf("Expected path %s, got %s", expectedPath, ws.Path)
	}

	// Verify directory was created
	if _, err := os.Stat(ws.Path); os.IsNotExist(err) {
		t.Error("Workspace directory should be created")
	}

	// Creating same workspace again should return existing one
	ws2, err := wm.CreateWorkspace(runID, baseRepoPath)
	if err != nil {
		t.Fatalf("Failed to get existing workspace: %v", err)
	}
	if ws != ws2 {
		t.Error("Should return same workspace instance for same run ID")
	}
}

func TestWorkspaceManager_GetWorkspace(t *testing.T) {
	tempDir := t.TempDir()
	wm, err := NewWorkspaceManager(tempDir)
	if err != nil {
		t.Fatalf("Failed to create workspace manager: %v", err)
	}

	runID := "test-run-123"

	// Getting non-existent workspace should fail
	_, err = wm.GetWorkspace(runID)
	if err == nil {
		t.Error("Should fail to get non-existent workspace")
	}

	// Create workspace
	ws, err := wm.CreateWorkspace(runID, "/tmp/test-repo")
	if err != nil {
		t.Fatalf("Failed to create workspace: %v", err)
	}

	// Getting existing workspace should succeed
	ws2, err := wm.GetWorkspace(runID)
	if err != nil {
		t.Fatalf("Failed to get existing workspace: %v", err)
	}
	if ws != ws2 {
		t.Error("Should return same workspace instance")
	}
}

func TestWorkspace_PrepareRepository(t *testing.T) {
	tempDir := t.TempDir()
	wm, err := NewWorkspaceManager(tempDir)
	if err != nil {
		t.Fatalf("Failed to create workspace manager: %v", err)
	}

	// Create a test repository
	testRepo := filepath.Join(tempDir, "test-repo")
	err = os.MkdirAll(testRepo, 0755)
	if err != nil {
		t.Fatalf("Failed to create test repo: %v", err)
	}

	// Create a test file in the repo
	testFile := filepath.Join(testRepo, "test.txt")
	err = os.WriteFile(testFile, []byte("test content"), 0644)
	if err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	runID := "test-run-123"
	ws, err := wm.CreateWorkspace(runID, testRepo)
	if err != nil {
		t.Fatalf("Failed to create workspace: %v", err)
	}

	// Prepare repository
	workspaceRepoPath, err := ws.PrepareRepository(testRepo)
	if err != nil {
		t.Fatalf("Failed to prepare repository: %v", err)
	}

	// Verify workspace repo path
	expectedPath := filepath.Join(ws.Path, "repos", "test-repo")
	if workspaceRepoPath != expectedPath {
		t.Errorf("Expected workspace repo path %s, got %s", expectedPath, workspaceRepoPath)
	}

	// Verify symlink was created
	linkInfo, err := os.Lstat(workspaceRepoPath)
	if err != nil {
		t.Fatalf("Failed to stat workspace repo: %v", err)
	}
	if linkInfo.Mode()&os.ModeSymlink == 0 {
		t.Error("Expected symbolic link to be created")
	}

	// Preparing same repository again should return same path
	workspaceRepoPath2, err := ws.PrepareRepository(testRepo)
	if err != nil {
		t.Fatalf("Failed to prepare repository second time: %v", err)
	}
	if workspaceRepoPath != workspaceRepoPath2 {
		t.Error("Should return same path for same repository")
	}

	// Test non-isolated workspace
	ws.IsIsolated = false
	originalPath, err := ws.PrepareRepository(testRepo)
	if err != nil {
		t.Fatalf("Failed to prepare repository in non-isolated mode: %v", err)
	}
	if originalPath != testRepo {
		t.Errorf("Non-isolated workspace should return original path, got %s", originalPath)
	}
}

func TestWorkspace_GetDirectories(t *testing.T) {
	tempDir := t.TempDir()
	wm, err := NewWorkspaceManager(tempDir)
	if err != nil {
		t.Fatalf("Failed to create workspace manager: %v", err)
	}

	runID := "test-run-123"
	ws, err := wm.CreateWorkspace(runID, "/tmp/test-repo")
	if err != nil {
		t.Fatalf("Failed to create workspace: %v", err)
	}

	// Test directory getters
	expectedBase := filepath.Join(tempDir, runID)

	executionDir := ws.GetExecutionDir()
	expectedExecDir := filepath.Join(expectedBase, "execution")
	if executionDir != expectedExecDir {
		t.Errorf("Expected execution dir %s, got %s", expectedExecDir, executionDir)
	}

	stateDir := ws.GetStateDir()
	expectedStateDir := filepath.Join(expectedBase, "state")
	if stateDir != expectedStateDir {
		t.Errorf("Expected state dir %s, got %s", expectedStateDir, stateDir)
	}

	logsDir := ws.GetLogsDir()
	expectedLogsDir := filepath.Join(expectedBase, "logs")
	if logsDir != expectedLogsDir {
		t.Errorf("Expected logs dir %s, got %s", expectedLogsDir, logsDir)
	}

	tempDirPath := ws.GetTempDir()
	expectedTempDir := filepath.Join(expectedBase, "tmp")
	if tempDirPath != expectedTempDir {
		t.Errorf("Expected temp dir %s, got %s", expectedTempDir, tempDirPath)
	}
}

func TestWorkspace_EnsureDirectories(t *testing.T) {
	tempDir := t.TempDir()
	wm, err := NewWorkspaceManager(tempDir)
	if err != nil {
		t.Fatalf("Failed to create workspace manager: %v", err)
	}

	runID := "test-run-123"
	ws, err := wm.CreateWorkspace(runID, "/tmp/test-repo")
	if err != nil {
		t.Fatalf("Failed to create workspace: %v", err)
	}

	// Ensure directories
	err = ws.EnsureDirectories()
	if err != nil {
		t.Fatalf("Failed to ensure directories: %v", err)
	}

	// Verify all directories were created
	dirs := []string{
		ws.GetExecutionDir(),
		ws.GetStateDir(),
		ws.GetLogsDir(),
		ws.GetTempDir(),
		filepath.Join(ws.Path, "repos"),
	}

	for _, dir := range dirs {
		if _, err := os.Stat(dir); os.IsNotExist(err) {
			t.Errorf("Directory %s should have been created", dir)
		}
	}
}

func TestWorkspace_CreateCopyOnWrite(t *testing.T) {
	tempDir := t.TempDir()
	wm, err := NewWorkspaceManager(tempDir)
	if err != nil {
		t.Fatalf("Failed to create workspace manager: %v", err)
	}

	// Create original file
	originalFile := filepath.Join(tempDir, "original.txt")
	originalContent := "original content"
	err = os.WriteFile(originalFile, []byte(originalContent), 0644)
	if err != nil {
		t.Fatalf("Failed to create original file: %v", err)
	}

	runID := "test-run-123"
	ws, err := wm.CreateWorkspace(runID, "/tmp/test-repo")
	if err != nil {
		t.Fatalf("Failed to create workspace: %v", err)
	}

	err = ws.EnsureDirectories()
	if err != nil {
		t.Fatalf("Failed to ensure directories: %v", err)
	}

	workspaceFile := filepath.Join(ws.Path, "copy.txt")

	// First create a symlink
	err = os.Symlink(originalFile, workspaceFile)
	if err != nil {
		t.Fatalf("Failed to create symlink: %v", err)
	}

	// Verify it's a symlink
	linkInfo, err := os.Lstat(workspaceFile)
	if err != nil {
		t.Fatalf("Failed to stat workspace file: %v", err)
	}
	if linkInfo.Mode()&os.ModeSymlink == 0 {
		t.Error("Expected symbolic link")
	}

	// Create copy-on-write
	err = ws.CreateCopyOnWrite(originalFile, workspaceFile)
	if err != nil {
		t.Fatalf("Failed to create copy-on-write: %v", err)
	}

	// Verify it's now a regular file
	fileInfo, err := os.Lstat(workspaceFile)
	if err != nil {
		t.Fatalf("Failed to stat workspace file after COW: %v", err)
	}
	if fileInfo.Mode()&os.ModeSymlink != 0 {
		t.Error("Should not be a symbolic link after COW")
	}

	// Verify content was copied
	content, err := os.ReadFile(workspaceFile)
	if err != nil {
		t.Fatalf("Failed to read workspace file: %v", err)
	}
	if string(content) != originalContent {
		t.Errorf("Expected content %s, got %s", originalContent, string(content))
	}

	// Calling COW again should be idempotent
	err = ws.CreateCopyOnWrite(originalFile, workspaceFile)
	if err != nil {
		t.Fatalf("Failed to call COW again: %v", err)
	}
}

func TestWorkspace_Cleanup(t *testing.T) {
	tempDir := t.TempDir()
	wm, err := NewWorkspaceManager(tempDir)
	if err != nil {
		t.Fatalf("Failed to create workspace manager: %v", err)
	}

	runID := "test-run-123"
	ws, err := wm.CreateWorkspace(runID, "/tmp/test-repo")
	if err != nil {
		t.Fatalf("Failed to create workspace: %v", err)
	}

	err = ws.EnsureDirectories()
	if err != nil {
		t.Fatalf("Failed to ensure directories: %v", err)
	}

	// Create test file
	testFile := filepath.Join(ws.Path, "test.txt")
	err = os.WriteFile(testFile, []byte("test"), 0644)
	if err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	// Verify workspace exists
	if _, err := os.Stat(ws.Path); os.IsNotExist(err) {
		t.Error("Workspace should exist before cleanup")
	}

	// Cleanup workspace
	err = ws.Cleanup()
	if err != nil {
		t.Fatalf("Failed to cleanup workspace: %v", err)
	}

	// Verify workspace is removed
	if _, err := os.Stat(ws.Path); !os.IsNotExist(err) {
		t.Error("Workspace should be removed after cleanup")
	}
}

func TestWorkspaceManager_CleanupWorkspace(t *testing.T) {
	tempDir := t.TempDir()
	wm, err := NewWorkspaceManager(tempDir)
	if err != nil {
		t.Fatalf("Failed to create workspace manager: %v", err)
	}

	runID := "test-run-123"
	ws, err := wm.CreateWorkspace(runID, "/tmp/test-repo")
	if err != nil {
		t.Fatalf("Failed to create workspace: %v", err)
	}

	// Verify workspace is tracked
	if len(wm.workspaces) != 1 {
		t.Error("Workspace should be tracked in manager")
	}

	// Cleanup workspace via manager
	err = wm.CleanupWorkspace(runID)
	if err != nil {
		t.Fatalf("Failed to cleanup workspace: %v", err)
	}

	// Verify workspace is no longer tracked
	if len(wm.workspaces) != 0 {
		t.Error("Workspace should no longer be tracked after cleanup")
	}

	// Verify workspace directory is removed
	if _, err := os.Stat(ws.Path); !os.IsNotExist(err) {
		t.Error("Workspace directory should be removed")
	}

	// Cleaning up non-existent workspace should be safe
	err = wm.CleanupWorkspace("non-existent")
	if err != nil {
		t.Fatalf("Cleanup of non-existent workspace should not fail: %v", err)
	}
}

func TestWorkspaceManager_CleanupAll(t *testing.T) {
	tempDir := t.TempDir()
	wm, err := NewWorkspaceManager(tempDir)
	if err != nil {
		t.Fatalf("Failed to create workspace manager: %v", err)
	}

	// Create multiple workspaces
	runIDs := []string{"run-1", "run-2", "run-3"}
	workspaces := make([]*Workspace, len(runIDs))

	for i, runID := range runIDs {
		ws, err := wm.CreateWorkspace(runID, "/tmp/test-repo")
		if err != nil {
			t.Fatalf("Failed to create workspace %s: %v", runID, err)
		}
		workspaces[i] = ws
	}

	// Verify all workspaces are tracked
	if len(wm.workspaces) != len(runIDs) {
		t.Errorf("Expected %d workspaces, got %d", len(runIDs), len(wm.workspaces))
	}

	// Cleanup all workspaces
	err = wm.CleanupAll()
	if err != nil {
		t.Fatalf("Failed to cleanup all workspaces: %v", err)
	}

	// Verify no workspaces are tracked
	if len(wm.workspaces) != 0 {
		t.Error("No workspaces should be tracked after cleanup all")
	}

	// Verify all workspace directories are removed
	for _, ws := range workspaces {
		if _, err := os.Stat(ws.Path); !os.IsNotExist(err) {
			t.Errorf("Workspace %s should be removed", ws.Path)
		}
	}
}

func TestWorkspaceManager_ListWorkspaces(t *testing.T) {
	tempDir := t.TempDir()
	wm, err := NewWorkspaceManager(tempDir)
	if err != nil {
		t.Fatalf("Failed to create workspace manager: %v", err)
	}

	// Initially no workspaces
	runIDs := wm.ListWorkspaces()
	if len(runIDs) != 0 {
		t.Error("Should have no workspaces initially")
	}

	// Create workspaces
	expectedRunIDs := []string{"run-1", "run-2", "run-3"}
	for _, runID := range expectedRunIDs {
		_, err := wm.CreateWorkspace(runID, "/tmp/test-repo")
		if err != nil {
			t.Fatalf("Failed to create workspace %s: %v", runID, err)
		}
	}

	// List workspaces
	runIDs = wm.ListWorkspaces()
	if len(runIDs) != len(expectedRunIDs) {
		t.Errorf("Expected %d workspaces, got %d", len(expectedRunIDs), len(runIDs))
	}

	// Verify all expected run IDs are present
	runIDMap := make(map[string]bool)
	for _, runID := range runIDs {
		runIDMap[runID] = true
	}

	for _, expectedRunID := range expectedRunIDs {
		if !runIDMap[expectedRunID] {
			t.Errorf("Expected run ID %s not found in list", expectedRunID)
		}
	}
}

func TestCopyFile(t *testing.T) {
	tempDir := t.TempDir()

	// Create source file
	srcFile := filepath.Join(tempDir, "source.txt")
	srcContent := "test content for copy"
	err := os.WriteFile(srcFile, []byte(srcContent), 0644)
	if err != nil {
		t.Fatalf("Failed to create source file: %v", err)
	}

	// Test copying to new file
	dstFile := filepath.Join(tempDir, "subdir", "dest.txt")
	err = copyFile(srcFile, dstFile)
	if err != nil {
		t.Fatalf("Failed to copy file: %v", err)
	}

	// Verify destination file exists and has correct content
	dstContent, err := os.ReadFile(dstFile)
	if err != nil {
		t.Fatalf("Failed to read destination file: %v", err)
	}
	if string(dstContent) != srcContent {
		t.Errorf("Expected content %s, got %s", srcContent, string(dstContent))
	}

	// Verify permissions are copied
	srcInfo, err := os.Stat(srcFile)
	if err != nil {
		t.Fatalf("Failed to stat source file: %v", err)
	}
	dstInfo, err := os.Stat(dstFile)
	if err != nil {
		t.Fatalf("Failed to stat destination file: %v", err)
	}
	if srcInfo.Mode() != dstInfo.Mode() {
		t.Errorf("Expected mode %v, got %v", srcInfo.Mode(), dstInfo.Mode())
	}

	// Test copying non-existent file
	err = copyFile(filepath.Join(tempDir, "nonexistent.txt"), filepath.Join(tempDir, "dest2.txt"))
	if err == nil {
		t.Error("Should fail to copy non-existent file")
	}
}

func TestWorkspace_InvalidDirectoryCreation(t *testing.T) {
	tempDir := t.TempDir()
	wm, err := NewWorkspaceManager(tempDir)
	if err != nil {
		t.Fatalf("Failed to create workspace manager: %v", err)
	}

	runID := "test-run-123"
	ws, err := wm.CreateWorkspace(runID, "/tmp/test-repo")
	if err != nil {
		t.Fatalf("Failed to create workspace: %v", err)
	}

	// Create a file where we expect a directory (to cause conflict)
	conflictPath := filepath.Join(ws.Path, "execution")
	err = os.WriteFile(conflictPath, []byte("conflict"), 0644)
	if err != nil {
		t.Fatalf("Failed to create conflict file: %v", err)
	}

	// EnsureDirectories should fail due to conflict
	err = ws.EnsureDirectories()
	if err == nil {
		t.Error("EnsureDirectories should fail when file conflicts with directory")
	}
	if !strings.Contains(err.Error(), "failed to create directory") {
		t.Errorf("Expected directory creation error, got: %v", err)
	}
}
