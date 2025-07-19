# E2E Test Framework Design Discussion (TODO)

This document captures the design and implementation plan for the E2E test framework.

## 1. High-Level Scenarios

The E2E framework must be capable of validating the following real-world scenarios:

*   **Scenario 1: Status Check:** A developer runs `tako run "git status"` to get a quick overview of all repositories.
*   **Scenario 2: Dependency Update:** A developer runs `tako run "npm update <package>"` to patch a vulnerability across all relevant repositories in the correct order.
*   **Scenario 3: Cache Cleaning:** A developer runs `tako run "rm -rf dist"` to clean build artifacts from all projects.
*   **Scenario 4: Java Binary Incompatibility:** A complex, multi-step scenario to ensure `tako` correctly handles and rebuilds the full dependency chain after a binary-incompatible change is introduced in a core Java library.

## 2. Decided Architecture: "Developer Workflow" Model

The `takotest` setup tool will follow a unified **"Developer Workflow" Model**.

1.  **Generate Locally:** `takotest setup` will always begin by creating Git repositories in a temporary local directory.
2.  **Handle Remote/Local:**
    *   **Remote Mode:** It will create corresponding empty repos on GitHub and push the local contents to them.
    *   **Local Mode:** It will move the local Git repos into the appropriate cache structure.
3.  **Prepare Workspace:** It will then prepare the final `workDir` and `cacheDir` for the test run, deleting the initial generation directory.

This ensures a consistent, Git-based foundation for all tests.

## 3. Plan of Action

This plan tracks the implementation of the new E2E framework.

### Phase 1: Core Framework Refactoring

-   [x] **Update Cache Path Logic & Documentation:** The codebase and documentation have been updated to use the correct cache structure: `<cache-dir>/repos/<owner>/<repo>/<branch>`. A branch sanitization function was implemented.
-   [x] **Refactor `cmd/takotest/internal/setup.go`:** The `setup` command has been refactored into a single, unified workflow. It now orchestrates `git` commands and generates mode-aware `tako.yml` files.
-   [x] **Update `test/e2e/environments.go`:** The `TestEnvironmentDef` and `RepositoryDef` structs have been updated to support file templates and branch names.
-   [x] **Refactor `e2e_test.go`:** The test runner has been refactored into a generic engine that executes declarative, multi-step `TestCase` definitions.

### Phase 2: Scenario Implementation (Next Steps)

-   [ ] **Implement the Java Binary Incompatibility Scenario:**
    *   [ ] Create a new `TestEnvironmentDef` for the Java scenario in `test/e2e/environments.go`.
    *   [ ] Add the required Java and Maven template files (e.g., `pom.xml`, `Main.java`) to the `test/e2e/templates/` directory.
    *   [ ] Create a new multi-step `TestCase` in a `test/e2e/modifying_test.go` file that orchestrates the full 5-step test flow described in "Scenario 4". This will involve `tako` and `mvn` commands, as well as steps to programmatically modify the Java source code.
-   [ ] **Implement Other Scenarios:**
    *   [ ] Add test cases for the "Status Check", "Dependency Update", and "Cache Cleaning" scenarios to provide broader coverage of `tako run`.