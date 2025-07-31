# Issue #144 - Local Go CI Pipeline E2E Test - Background Research

## Issue Summary
Issue #144 requires creating a real-world, end-to-end testing scenario for a Local Go CI Pipeline that demonstrates the features implemented in Milestone 3. This is the final step in completing issue #106 (subscription-based workflow triggering).

## Parent Issue Context
- **Issue #106**: "feat(engine): Implement subscription-based workflow triggering" (CLOSED)
- **Milestone**: Milestone 3: MVP - Local, Synchronous Execution
- **Goal**: Demonstrate a common use case for `tako`: a local CI pipeline for a Go application

## Target Scenario Requirements
The scenario should implement a multi-step CI pipeline with:
1. **Linting**: `go vet ./...` (native shell execution)
2. **Testing**: `go test -v ./...` (native shell execution)  
3. **Building**: Go binary compilation (containerized with `golang:1.22-alpine`)
4. **Packaging**: Docker image build (native shell execution with docker)

## Existing E2E Test Infrastructure
The project has a comprehensive E2E testing framework in `/test/e2e/`:

### Current Test Cases (19 total):
- **Graph operations**: `graph-simple`
- **Run operations**: `run-touch-command`, `run-dry-run-prevents-execution`
- **Java dependency management**: `java-binary-incompatibility`
- **Exec workflows**: 
  - `exec-basic-workflow`
  - `exec-advanced-input-validation`
  - `exec-step-output-passing`
  - `exec-template-variable-resolution`
  - `exec-error-handling`
  - `exec-long-running-workflow`
  - `exec-dry-run-mode`
  - `exec-debug-mode`
  - `exec-workflow-not-found`
  - `exec-security-functions`
  - `exec-malformed-config`

### Test Environment Structure:
- Tests run in both `local` and `remote` modes
- Tests support both `entrypoint-path` and `entrypoint-repo` scenarios
- Test templates are in `/test/e2e/templates/`
- Examples include `fan-out-test`, `java-binary-incompatibility`, `malformed-config`, `single-repo-workflow`

## Container Support Analysis
Tako has robust container support (`internal/engine/container.go`):
- **Runtime detection**: Docker/Podman auto-detection
- **Security hardening**: User isolation, read-only filesystem, capability dropping
- **Resource limits**: CPU, memory, disk constraints
- **Volume mounting**: Workspace and artifact sharing
- **Image management**: Pull, cache, and registry authentication

## Fan-Out and Orchestrator Features
Recent implementations include:
- **Fan-out executor** (`internal/engine/fanout.go`): Event-driven workflow triggering
- **Orchestrator** (`internal/engine/orchestrator.go`): Subscription discovery and coordination
- **Idempotency support**: Duplicate event detection with SHA256 fingerprinting
- **State management**: Persistent state across process restarts

## Implementation Requirements
Based on the issue description, I need to:

1. **Create test files**: `main.go`, `Dockerfile`, `tako.yml` matching exact specifications
2. **Add to E2E framework**: New test case in `get_test_cases.go`
3. **Create test environment**: Template directory structure
4. **Test scenarios**: Execute full pipeline with input validation
5. **Verify results**: Docker image creation, artifact production

## Files to Modify/Create
- `test/e2e/get_test_cases.go` - Add new test case
- `test/e2e/environments.go` - Add new environment (if needed)
- `test/e2e/templates/local-go-ci-pipeline/` - New template directory
  - `main.go` - Simple web server
  - `Dockerfile` - Alpine-based container
  - `tako.yml` - CI workflow definition

## Key Technical Considerations
1. **Container steps**: The `build` step uses `golang:1.22-alpine` image
2. **Artifact production**: Binary must be produced and available for packaging
3. **Template variables**: Support for `{{ .Inputs.image_tag }}` 
4. **Input validation**: String inputs with defaults
5. **Mixed execution**: Shell commands + containerized steps

## Questions and Resolutions (via Gemini consultation)

### Q1: Test Environment Structure
**Question**: Should this be a new environment or use existing `single-repo-workflow`?
**Resolution**: Create new environment `local-go-ci-pipeline` following best practices of existing specific scenarios.

### Q2: Docker-in-Docker Considerations  
**Question**: How to handle Docker-in-Docker requirements?
**Resolution**: No complex Docker-in-Docker needed. Host Docker daemon is accessible. Containerized build step mounts working directory, native packaging step uses host Docker.

### Q3: Artifact Validation
**Question**: What level of verification is appropriate?
**Resolution**: Full artifact validation required:
- Check Docker image exists with `docker image inspect`  
- Verify image runs correctly with `docker run`
- Include cleanup step with `docker rmi`

### Q4: Test Coverage
**Question**: Should negative test cases be included?
**Resolution**: Yes, create separate test cases for:
- `local-go-ci-pipeline-lint-failure`
- `local-go-ci-pipeline-build-failure` 
- `local-go-ci-pipeline-package-failure`
EOF < /dev/null