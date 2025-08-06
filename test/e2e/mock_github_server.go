package e2e

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/mux"
)

// MockGitHubServer provides a mock implementation of GitHub API for E2E testing.
type MockGitHubServer struct {
	server    *http.Server
	prs       map[string]*PullRequest
	prCounter int
	mu        sync.RWMutex
}

// PullRequest represents a mock GitHub Pull Request.
type PullRequest struct {
	ID          int    `json:"id"`
	Number      int    `json:"number"`
	Title       string `json:"title"`
	Body        string `json:"body"`
	State       string `json:"state"` // "open", "closed", "merged"
	Head        Branch `json:"head"`
	Base        Branch `json:"base"`
	Owner       string `json:"-"`
	Repo        string `json:"-"`
	CheckStatus string `json:"-"` // "pending", "success", "failure"
	CreatedAt   string `json:"created_at"`
	UpdatedAt   string `json:"updated_at"`
}

// Branch represents a git branch.
type Branch struct {
	Ref string `json:"ref"`
	SHA string `json:"sha"`
}

// CheckRun represents a GitHub check run.
type CheckRun struct {
	ID         int    `json:"id"`
	Status     string `json:"status"`     // "queued", "in_progress", "completed"
	Conclusion string `json:"conclusion"` // "success", "failure", "neutral", "cancelled", "timed_out", "action_required"
	Name       string `json:"name"`
}

// CheckSuite represents a collection of check runs.
type CheckSuite struct {
	CheckRuns []CheckRun `json:"check_runs"`
}

// NewMockGitHubServer creates a new mock GitHub API server.
func NewMockGitHubServer() *MockGitHubServer {
	return &MockGitHubServer{
		prs:       make(map[string]*PullRequest),
		prCounter: 1000, // Start with a high number to avoid conflicts
	}
}

// Start starts the mock server on the specified port.
func (m *MockGitHubServer) Start(port int) error {
	router := mux.NewRouter()

	// GitHub API endpoints
	router.HandleFunc("/repos/{owner}/{repo}/pulls", m.handleCreatePR).Methods("POST")
	router.HandleFunc("/repos/{owner}/{repo}/pulls/{pr}", m.handleGetPR).Methods("GET")
	router.HandleFunc("/repos/{owner}/{repo}/pulls/{pr}/merge", m.handleMergePR).Methods("PUT")
	router.HandleFunc("/repos/{owner}/{repo}/pulls/{pr}/checks", m.handleGetChecks).Methods("GET")

	// Test orchestration endpoints
	router.HandleFunc("/test/ci/{owner}/{repo}/{pr}/complete", m.handleCompleteCI).Methods("POST")
	router.HandleFunc("/test/ci/{owner}/{repo}/{pr}/fail", m.handleFailCI).Methods("POST")
	router.HandleFunc("/test/reset", m.handleReset).Methods("POST")

	// Health check
	router.HandleFunc("/health", m.handleHealth).Methods("GET")

	addr := fmt.Sprintf(":%d", port)
	m.server = &http.Server{
		Addr:    addr,
		Handler: router,
	}

	log.Printf("Mock GitHub API server starting on %s", addr)
	return m.server.ListenAndServe()
}

// Stop stops the mock server.
func (m *MockGitHubServer) Stop() error {
	if m.server != nil {
		return m.server.Close()
	}
	return nil
}

// handleCreatePR handles PR creation requests.
func (m *MockGitHubServer) handleCreatePR(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	owner := vars["owner"]
	repo := vars["repo"]

	var createReq struct {
		Title string `json:"title"`
		Body  string `json:"body"`
		Head  string `json:"head"`
		Base  string `json:"base"`
	}

	if err := json.NewDecoder(r.Body).Decode(&createReq); err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	// Create new PR
	m.prCounter++
	prNumber := m.prCounter
	prKey := fmt.Sprintf("%s/%s/%d", owner, repo, prNumber)

	pr := &PullRequest{
		ID:          prNumber,
		Number:      prNumber,
		Title:       createReq.Title,
		Body:        createReq.Body,
		State:       "open",
		Head:        Branch{Ref: createReq.Head, SHA: "abc123"},
		Base:        Branch{Ref: createReq.Base, SHA: "def456"},
		Owner:       owner,
		Repo:        repo,
		CheckStatus: "pending", // Start with pending CI checks
		CreatedAt:   time.Now().Format(time.RFC3339),
		UpdatedAt:   time.Now().Format(time.RFC3339),
	}

	m.prs[prKey] = pr

	log.Printf("Created PR #%d for %s/%s: %s", prNumber, owner, repo, createReq.Title)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(pr)
}

// handleGetPR handles PR retrieval requests.
func (m *MockGitHubServer) handleGetPR(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	owner := vars["owner"]
	repo := vars["repo"]
	prStr := vars["pr"]

	prNumber, err := strconv.Atoi(prStr)
	if err != nil {
		http.Error(w, "Invalid PR number", http.StatusBadRequest)
		return
	}

	prKey := fmt.Sprintf("%s/%s/%d", owner, repo, prNumber)

	m.mu.RLock()
	pr, exists := m.prs[prKey]
	m.mu.RUnlock()

	if !exists {
		http.Error(w, "PR not found", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(pr)
}

// handleMergePR handles PR merge requests.
func (m *MockGitHubServer) handleMergePR(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	owner := vars["owner"]
	repo := vars["repo"]
	prStr := vars["pr"]

	prNumber, err := strconv.Atoi(prStr)
	if err != nil {
		http.Error(w, "Invalid PR number", http.StatusBadRequest)
		return
	}

	prKey := fmt.Sprintf("%s/%s/%d", owner, repo, prNumber)

	m.mu.Lock()
	defer m.mu.Unlock()

	pr, exists := m.prs[prKey]
	if !exists {
		http.Error(w, "PR not found", http.StatusNotFound)
		return
	}

	if pr.State != "open" {
		http.Error(w, "PR is not open", http.StatusConflict)
		return
	}

	if pr.CheckStatus != "success" {
		http.Error(w, "CI checks have not passed", http.StatusConflict)
		return
	}

	// Merge the PR
	pr.State = "merged"
	pr.UpdatedAt = time.Now().Format(time.RFC3339)

	log.Printf("Merged PR #%d for %s/%s", prNumber, owner, repo)

	mergeResult := map[string]interface{}{
		"merged":  true,
		"sha":     "merged-commit-sha",
		"message": "Pull request successfully merged",
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(mergeResult)
}

// handleGetChecks handles CI check status requests.
func (m *MockGitHubServer) handleGetChecks(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	owner := vars["owner"]
	repo := vars["repo"]
	prStr := vars["pr"]

	prNumber, err := strconv.Atoi(prStr)
	if err != nil {
		http.Error(w, "Invalid PR number", http.StatusBadRequest)
		return
	}

	prKey := fmt.Sprintf("%s/%s/%d", owner, repo, prNumber)

	m.mu.RLock()
	pr, exists := m.prs[prKey]
	m.mu.RUnlock()

	if !exists {
		http.Error(w, "PR not found", http.StatusNotFound)
		return
	}

	// Create check run based on PR status
	var status, conclusion string
	switch pr.CheckStatus {
	case "pending":
		status = "in_progress"
		conclusion = ""
	case "success":
		status = "completed"
		conclusion = "success"
	case "failure":
		status = "completed"
		conclusion = "failure"
	}

	checkSuite := CheckSuite{
		CheckRuns: []CheckRun{
			{
				ID:         1,
				Status:     status,
				Conclusion: conclusion,
				Name:       "ci/tests",
			},
		},
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(checkSuite)
}

// handleCompleteCI marks CI as completed for a PR (test orchestration).
func (m *MockGitHubServer) handleCompleteCI(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	owner := vars["owner"]
	repo := vars["repo"]
	prStr := vars["pr"]

	prNumber, err := strconv.Atoi(prStr)
	if err != nil {
		http.Error(w, "Invalid PR number", http.StatusBadRequest)
		return
	}

	prKey := fmt.Sprintf("%s/%s/%d", owner, repo, prNumber)

	m.mu.Lock()
	defer m.mu.Unlock()

	pr, exists := m.prs[prKey]
	if !exists {
		http.Error(w, "PR not found", http.StatusNotFound)
		return
	}

	pr.CheckStatus = "success"
	pr.UpdatedAt = time.Now().Format(time.RFC3339)

	log.Printf("Marked CI as complete for PR #%d in %s/%s", prNumber, owner, repo)

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"status": "success"})
}

// handleFailCI marks CI as failed for a PR (test orchestration).
func (m *MockGitHubServer) handleFailCI(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	owner := vars["owner"]
	repo := vars["repo"]
	prStr := vars["pr"]

	prNumber, err := strconv.Atoi(prStr)
	if err != nil {
		http.Error(w, "Invalid PR number", http.StatusBadRequest)
		return
	}

	prKey := fmt.Sprintf("%s/%s/%d", owner, repo, prNumber)

	m.mu.Lock()
	defer m.mu.Unlock()

	pr, exists := m.prs[prKey]
	if !exists {
		http.Error(w, "PR not found", http.StatusNotFound)
		return
	}

	pr.CheckStatus = "failure"
	pr.UpdatedAt = time.Now().Format(time.RFC3339)

	log.Printf("Marked CI as failed for PR #%d in %s/%s", prNumber, owner, repo)

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"status": "failure"})
}

// handleReset resets the mock server state (test orchestration).
func (m *MockGitHubServer) handleReset(w http.ResponseWriter, r *http.Request) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.prs = make(map[string]*PullRequest)
	m.prCounter = 1000

	log.Println("Reset mock GitHub server state")

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"status": "reset"})
}

// handleHealth provides health check endpoint.
func (m *MockGitHubServer) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "healthy"})
}

// GetPR returns a PR by key (for testing purposes).
func (m *MockGitHubServer) GetPR(owner, repo string, prNumber int) *PullRequest {
	prKey := fmt.Sprintf("%s/%s/%d", owner, repo, prNumber)
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.prs[prKey]
}

// ListPRs returns all PRs for a repository (for testing purposes).
func (m *MockGitHubServer) ListPRs(owner, repo string) []*PullRequest {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var prs []*PullRequest
	prefix := fmt.Sprintf("%s/%s/", owner, repo)

	for key, pr := range m.prs {
		if strings.HasPrefix(key, prefix) {
			prs = append(prs, pr)
		}
	}

	return prs
}
