# Issue #144 Implementation Plan - Local Go CI Pipeline E2E Test

## Overview
Implement a comprehensive E2E test scenario demonstrating a real-world Local Go CI Pipeline. This involves creating test templates, adding test cases to the E2E framework, and ensuring proper validation of containerized and native execution steps.

## Phase 1: Create Test Template Infrastructure
**Goal**: Set up the basic file structure and content for the test scenario
**Deliverables**: Template files that match the exact issue specification

### Tasks:
1. Create template directory structure:
   - `test/e2e/templates/local-go-ci-pipeline/`
   
2. Create `main.go` (exact match to issue spec):
   ```go
   package main
   
   import (
       "fmt"
       "net/http"
   )
   
   func handler(w http.ResponseWriter, r *http.Request) {
       fmt.Fprintf(w, "Hello, Tako\!")
   }
   
   func main() {
       http.HandleFunc("/", handler)
       http.ListenAndServe(":8080", nil)
   }
   ```

3. Create `Dockerfile` (exact match to issue spec):
   ```dockerfile
   FROM alpine:latest
   WORKDIR /app
   COPY my-app .
   CMD ["./my-app"]
   ```

4. Create `tako.yml` (exact match to issue spec with template support):
   ```yaml
   version: v1
   workflows:
     ci-pipeline:
       inputs:
         image_tag:
           type: string
           description: "The tag for the final Docker image"
           default: "latest"
       steps:
         - id: lint
           name: "Run Go Linter"
           run: "go vet ./..."
         - id: test
           name: "Run Unit Tests"
           run: "go test -v ./..."
         - id: build
           name: "Build for Linux"
           image: "golang:1.22-alpine"
           script: |
             echo "Building Go binary for linux/amd64..."
             GOOS=linux GOARCH=amd64 go build -o my-app main.go
             echo "Build complete."
           produces:
             artifacts:
               - path: ./my-app
         - id: package
           name: "Package Docker Image"
           run: |
             echo "Building Docker image my-app:{{ .Inputs.image_tag }}..."
             docker build . -t my-app:{{ .Inputs.image_tag }}
             echo "Image built successfully."
   ```

**Test Requirements**: Files created, basic structure validated

## Phase 2: Add Primary Success Test Case
**Goal**: Implement the main positive test case in the E2E framework
**Deliverables**: Working test case that validates full pipeline execution

### Tasks:
1. Add new test case to `test/e2e/get_test_cases.go`:
   - Name: `local-go-ci-pipeline-success`
   - Environment: `local-go-ci-pipeline`
   - Full pipeline execution with verification

2. Test steps implementation:
   - Execute `tako exec ci-pipeline --inputs.image_tag=v1.0.0`
   - Verify output contains expected workflow steps
   - Verify Docker image creation with `docker image inspect`
   - Verify image functionality with `docker run`
   - Cleanup with `docker rmi`

3. Environment configuration (if needed):
   - Add to `test/e2e/environments.go` if new environment type required

**Test Requirements**: Test passes locally, validates all pipeline steps

## Phase 3: Add Negative Test Cases  
**Goal**: Ensure error handling works correctly across different failure scenarios
**Deliverables**: Three additional test cases covering different failure modes

### Tasks:
1. Create separate template directories for test isolation:
   - `test/e2e/templates/local-go-ci-pipeline/` (main success case)
   - `test/e2e/templates/local-go-ci-pipeline-lint-failure/`
   - `test/e2e/templates/local-go-ci-pipeline-build-failure/`  
   - `test/e2e/templates/local-go-ci-pipeline-package-failure/`

2. Create `local-go-ci-pipeline-lint-failure`:
   - Separate template with lint errors in Go code
   - Assert pipeline fails at lint step
   - Verify appropriate error messages

3. Create `local-go-ci-pipeline-build-failure`:
   - Separate template with syntax errors  
   - Assert pipeline fails at build step
   - Verify containerized build failure handling

4. Create `local-go-ci-pipeline-package-failure`:
   - Separate template with invalid Dockerfile
   - Assert pipeline fails at package step
   - Verify Docker build failure handling

**Test Requirements**: All negative cases fail appropriately with expected error messages

## Phase 4: Integration Testing
**Goal**: Ensure all test cases work properly in both local and remote modes
**Deliverables**: Fully tested implementation ready for production

### Tasks:
1. Run local E2E tests:
   - `go test -v -tags=e2e --local ./...`
   - Fix any issues found

2. Run remote E2E tests (if applicable):
   - `go test -v -tags=e2e --remote ./...`
   - Address remote-specific issues

3. CI simulation validation:
   - Run tests using `act --container-architecture linux/amd64 -P ubuntu-latest=catthehacker/ubuntu:act-latest`
   - Address any CI-specific issues

4. Validate test coverage and assertions:
   - Ensure all specified acceptance criteria are met
   - Verify test output matches expected patterns
   - Confirm artifact handling between containerized build and native package steps

**Test Requirements**: All tests pass in local, remote, and CI simulation modes

## Phase 5: Documentation and Cleanup
**Goal**: Finalize implementation with proper documentation
**Deliverables**: Clean, documented, production-ready code

### Tasks:
1. Update coverage tracking:
   - Run coverage tests
   - Update `issue_coverage.md`
   - Ensure no significant coverage drops

2. Code quality validation:
   - Run `go fmt ./...`
   - Run linters: `go test -v .`  
   - Fix any issues

3. Documentation updates:
   - Update README.md if needed (likely not required for test-only changes)
   - Ensure code comments are appropriate

**Test Requirements**: All quality checks pass, coverage maintained

## Success Criteria
- [ ] Primary test case executes full CI pipeline successfully
- [ ] Docker image `my-app:v1.0.0` is created and functional
- [ ] Negative test cases fail appropriately with clear error messages
- [ ] Tests work in both local and remote E2E modes
- [ ] No test coverage regression
- [ ] All linter and formatter checks pass
- [ ] Code follows existing project patterns and conventions

## Risk Mitigation
- **Docker availability**: Tests will skip gracefully if Docker is not available
- **Container pull failures**: Use common, stable base images (golang:1.22-alpine, alpine:latest)
- **Test isolation**: Each test case cleans up its Docker images
- **Environment consistency**: Template-based approach ensures reproducible test environments
EOF < /dev/null