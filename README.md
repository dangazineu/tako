# Tako: Specification

[![codecov](https://codecov.io/gh/dangazineu/tako/graph/badge.svg?token=7847S2ZSQ3)](https://codecov.io/gh/dangazineu/tako)

This document outlines the specification for Tako, a command-line interface for multi-repository operations.

## 1. Vision

### 1.1. Problem Statement
To execute systemic operations (e.g., refactors, releases, testing) across multiple GitHub repositories **in the order of their dependencies**. For instance, changing a server's API in one repository, and then updating the client repositories that depend on it, or releasing a cascade of dependent artifacts.

### 1.2. Target Audience
Individual developers and small teams who manage multiple related repositories.

### 1.3. Elevator Pitch
Tako is a command-line tool that simplifies multi-repository workflows by understanding the dependencies between your projects. It allows you to run commands across your repositories in the correct order, ensuring that changes are built, tested, and released reliably.

### 1.4. Competitive Landscape
Tako differentiates itself from existing tools by focusing on **dependency-aware execution for local development workflows**.
*   **vs. Lerna/Nx:** These are excellent monorepo managers but are primarily focused on the JavaScript ecosystem and assume a single, unified repository.
*   **vs. Bazel:** A powerful build system for large-scale monorepos, but requires the code to be built and tested with Bazel. Tako borrows many concepts from Bazel, but meets developers where they are, by integrating with their language-specific tools.
*   **vs. Git Submodules/Scripts:** This is the manual, error-prone approach that Tako aims to replace with a structured, repeatable process.
*   **vs. GitHub Actions:** A CI/CD platform for automation *after* code is pushed. Tako is a developer tool for the local machine, designed to ensure code is correct *before* it's pushed. Additionally, Tako can be used for automation like GHA, allowing its configuration to be reusable for multiple purposes.

## 2. Getting Started

### Developer Setup

To ensure consistent code formatting, this project uses a `gofmt` pre-commit hook. To install the hooks, run the following command from the root of the repository:

```bash
./scripts/install-hooks.sh
```

## 3. Core Concepts

### 2.1. Workspace & Repository Management
*   **Workspace Root:** A "workspace" is not a formal concept with a global configuration file. For any given `tako` command, the **workspace root is the repository from which the command is executed**.
*   **Repository Sourcing & Caching:**
    *   The workspace root repository is the local version, which can have uncommitted changes.
    *   All downstream dependent repositories will be cloned from GitHub. To mitigate performance issues, Tako will cache these repositories locally in a well-known directory (`~/.tako/cache/repos`). On subsequent runs, it will fetch updates instead of performing a full clone.
    *   This caching mechanism will be responsible for cleaning up old repositories.
*   **Authentication:** Tako will rely on the user's local Git and SSH configuration for authentication with Git hosts. The initial version will prioritize SSH key authentication. Future versions will explicitly support credential helpers and integration with tools like the `gh` CLI.
*   **Platform Support:** The primary development target is a Unix-like environment (Linux, macOS). Windows support, particularly around container volume mounting and path handling, will be considered a future enhancement and is not a goal for the initial versions.

### 2.2. Dependency Graph
*   **Definition:** The dependency graph is defined manually via `tako.yml` files within each repository.
*   **Dependency Declaration:** The `tako.yml` file lists the repositories that *depend on* the current repository. This inverse declaration is crucial for propagating operations outwards. Each dependent is an object with a `repo` key in the format `owner/repo:branch`.
*   **Multiple Dependencies (Fan-in):** The graph model fully supports scenarios where a single repository is a dependent of multiple upstream repositories (e.g., a web client depending on both an API service and a shared library). The topological sort execution model ensures that any given repository is processed only once per `tako` run, after all of its direct dependencies have completed their execution.
*   **Circular Dependencies:** If a circular dependency is detected, Tako will refuse to operate and will output a clear error message identifying the cycle.

### 2.3. Execution Model
*   **Order & Parallelism:** Operations are executed based on a topological sort of the dependency graph. Independent branches are processed in parallel by default (`--serial` flag available).
*   **Error Handling & Recovery:**
    *   Execution halts on the first error by default. `--continue-on-error` and `--summarize-errors` flags provide more flexible control.
    *   For path-based overrides, file restoration is guaranteed. Tako modifies the dependent's configuration file in place and uses a mechanism similar to Go's `defer` to ensure the file is restored to its original state, even if the command fails.
    *   For transient network errors (e.g., cloning a repo, pulling a container image), Tako will implement a configurable retry mechanism.
    *   Errors will be structured with unique codes (e.g., `TAKO_E001`) to aid in debugging and programmatic handling.
*   **Observability:** Tako will use OpenTelemetry for logging and metrics. This will provide insights into command duration, successes, and failures, which can be exported to a variety of backends.

### 2.4. Inter-Repository Artifacts & Local Testing
*   **Mechanism:** Tako uses a **Path-Based Override** strategy, managed through an `artifacts` block in the `tako.yml` of the source repository. When a dependent repository needs an artifact, Tako will:
    1.  Build the artifact in the source repository.
    2.  Execute the `install_command` in the dependent repository's directory. The `install_command` is a shell command that can use the `${TAKO_ARTIFACT_PATH}` environment variable to access the built artifact.
*   **Artifact Caching:**
    *   To avoid redundant builds, Tako will cache generated artifacts. The cache key should be a hash of the artifact's build command, its source files, and the git commit of the repository. This ensures that artifacts are only rebuilt when their inputs change.
*   **Version Conflicts:**
    *   The initial version of Tako will not support workflows where a single dependent needs to test against multiple, different versions of the same artifact simultaneously. This is a highly complex edge case that can be addressed in the future if a strong use case emerges.
*   **Cleanup:** All generated artifacts and temporary directories will be cleaned up by Tako after execution, unless a debug flag (`--preserve-tmp`) is passed.

### 2.5. Containerized Execution Environments
*   **Mechanism:** A workflow or an artifact definition can optionally specify a Docker `image`. If specified, Tako will execute commands inside a container.
*   **Network Access:**
    *   By default, containers will have network access. A `network: none` option should be available in the `tako.yml` for workflows that need to run in a hermetic environment.
*   **Resource Constraints:**
    *   The `tako.yml` should support optional `memory` and `cpu` limits for containers to prevent resource exhaustion.
*   **Artifact Path Handling:** When an artifact is built in a container, Tako will manage copying it out of the build container and mounting it into any subsequent dependent containers, ensuring seamless handoff.
*   **Docker Unavailability:** If a workflow requires an `image` but Docker is not running, the command will fail with a clear error message. A fallback to local execution is not planned, as it would violate the principle of a consistent environment.

## 3. Command-Line Interface (CLI)

*   **Syntax:** `tako <command> [options] [args]`
*   **Core Commands:** 
    *   **Implemented:** `version`, `graph`, `cache`, `completion`, `validate`
    *   **Planned:** `run`, `exec`, `init`, `artifacts`, `deps`
*   **`tako graph`:** Displays the dependency graph.
    *   `--root`: The root directory of the project. Defaults to the current directory.
    *   `--repo`: The remote repository to use as the entrypoint (e.g. `owner/repo:ref`). This flag takes precedence over `--root`.
    *   `--local`: Only use local repositories, do not clone or update remote repositories.
*   **`tako completion`:** A command to generate shell completion scripts for different shells.
*   **`tako cache`:** A command to manage Tako's cache.
    *   `tako cache clean`: Removes all cached repositories and artifacts from Tako's cache directory.
*   **`tako validate`:** A command to validate the workspace health, checking `tako.yml` syntax, dependency availability, and Docker connectivity.
*   **Flags:** `--dry-run`, `--verbose`, `--debug`, `--only`, `--ignore`, `--serial`, `--continue-on-error`, `--summarize-errors`, `--preserve-tmp`.

## 4. Configuration (`tako.yml`)

*   **Schema Versioning:** A `version` field will be included. Tako will be backward compatible with older schema versions to a documented extent. A `tako migrate` command is a potential future feature to help users upgrade their configuration files.

    ```yaml
    # Version of the tako.yml schema
    version: "1.2"

    # Metadata about the repository
    metadata:
      name: "my-service"

    # Defines the artifacts this repository can produce for local testing.
    artifacts:
      api-client:
        description: "The generated Go API client"
        image: "golang:1.21-alpine" # Optional: build in a container
        command: "make generate-go-client"
        path: "./sdk/go/client.zip"
        install_command: "unzip -o ${TAKO_ARTIFACT_PATH} -d ./vendor/api-client"
        # Optional: command to verify the artifact was installed correctly
        verify_command: "go mod verify"
      docs:
        description: "The generated API documentation"
        command: "make generate-docs"
        path: "./dist/docs.tar.gz"
        install_command: "tar -xzf ${TAKO_ARTIFACT_PATH} -C ./public/docs"    

    # Repositories that depend on this one.
    dependents:
      - repo: "my-org/client-a:main"
        # A list of artifact names defined in the `artifacts` block.
        artifacts: ["api-client"]
        # Optionally, limit the workflows that are propagated to this dependent repo
        workflows: ["test-ci"]
      - repo: "my-org/docs-website:main"
        # This dependent needs the 'docs' artifact.
        artifacts: ["docs"]
    
    # Pre-defined command sequences.
    workflows:
      test-ci:
        image: "golang:1.21-alpine"
        # Optional: environment variables for the container
        env:
          CGO_ENABLED: "0"
        # Optional: resource limits
        resources:
          cpu: "2"
          memory: "4Gi"
        steps:
          - go test -v ./...
    ```

## 5. Security
*   **Command Execution:**  Tako executes shell commands defined in `tako.yml` files. This implies a level of trust in the repositories being used. A flag (e.g., `--allow-unsafe-workflows`) may be required to run potentially destructive workflows (TBD).
*   **Path Validation:** All file paths will be validated to prevent directory traversal attacks.



## 8. Testing

This project includes a comprehensive suite of tests to ensure the quality and correctness of the code.

### Running Unit Tests

To run the unit tests, use the following command:

```bash
go test -v ./...
```

### Running Integration Tests

The integration tests include a series of checks for code formatting, linting, and other quality gates. To run the integration tests, use the following command:

```bash
go test -v -tags=integration ./...
```

### Running End-to-End (E2E) Tests

The E2E tests create and interact with real GitHub repositories. They are designed to be run in two modes: `local` and `remote`.

**Prerequisites for Remote Tests:**

*   A GitHub Personal Access Token with `repo` and `delete_repo` scopes.
*   The `GITHUB_PERSONAL_ACCESS_TOKEN` environment variable must be set to your token.

**Running E2E Tests:**

To run the E2E tests, you must specify either the `--local` or `--remote` flag. You can also specify the `--entrypoint` flag to run tests in a specific entrypoint mode (`path` or `repo`).

To run the local E2E tests (which do not require a GitHub token), use the `--local` flag:

```bash
go test -v -tags=e2e --local ./...
```

To run the remote E2E tests, use the `--remote` flag:

```bash
go test -v -tags=e2e --remote ./...
```

### Manual Verification

To manually verify the application's behavior, you can use the `takotest` CLI tool to set up and tear down the test infrastructure.

**1. Install the tools:**

```bash
go install ./cmd/tako
go install ./cmd/takotest
```

**2. Set up the test environment:**

To set up the test environment on your local filesystem, use the `--local` flag. The `takotest setup` command will output a JSON object with the `workDir` and `cacheDir` paths.

```bash
takotest setup <testcase-name> --local --owner <owner>
```

To set up the test environment on GitHub, make sure your `GITHUB_PERSONAL_ACCESS_TOKEN` is set and run:

```bash
takotest setup <testcase-name> --owner <owner>
```

**3. Run the `tako graph` command:**

You can then use the `workDir` and `cacheDir` paths from the `takotest setup` output to run `tako`:

```bash
tako graph --root <workDir>/<repo-name> --cache-dir <cacheDir>
```

**4. Clean up the test environment:**

To clean up the remote test environment, run:

```bash
takotest cleanup <testcase-name> --owner <owner> --force
```

## 9. AI-Assisted Development Workflow

This project includes a phase-based development workflow system designed to provide structured guidance for AI assistants working on feature development.

### Custom Slash Command

The `/work_on <issue_number>` command provides a complete development workflow for any issue. This command uses a phase-based approach to ensure consistent, high-quality development practices.

### Workflow Phases

The workflow is divided into five modular phases, each with its own detailed instructions:

1. **Setup Phase** (`.agents/work_plan/01_setup.md`): Create feature branch and establish baseline
2. **Analysis & Planning Phase** (`.agents/work_plan/02_analysis.md`): Research, question formulation, and implementation planning
3. **Implementation Phase** (`.agents/work_plan/03_implementation.md`): Phase-by-phase development with continuous testing
4. **Finalization Phase** (`.agents/work_plan/04_finalization.md`): Cleanup and manual verification
5. **Pull Request Phase** (`.agents/work_plan/05_pull_request.md`): PR creation and CI monitoring

### Usage

To use the AI-assisted workflow:

```
/work_on 123
```

This will automatically generate a comprehensive TODO list and begin executing the structured development process for issue #123.

### Key Benefits

- **Consistent Process**: Every feature follows the same proven development workflow
- **Quality Assurance**: Built-in testing and verification at each phase
- **Maintainability**: Modular approach allows easy updates to specific workflow steps
- **Traceability**: Clear documentation and planning artifacts throughout development

## 7. Future Features
*   Watch mode for automatic rebuilds on file changes.
*   A plugin system for custom command types and integrations.
*   Support for using local copies of dependent repositories.
*   **Asynchronous, Observable Workflows:** A "remote mode" where Tako can execute long-running, asynchronous workflows in a cloud environment. This would transform Tako into a powerful automation platform with features like human-in-the-loop approvals and a centralized web UI for observing progress. See issue #47 for the detailed vision.
*   **Agentic, Zero-Configuration Workflows:** An "agentic mode" where Tako can infer workflows and dependency graphs directly from the source code and its environment, reducing the need for manual configuration. See issue #50 for the detailed vision.
*   A more advanced filtering syntax might be needed in the future to support ignoring a single repo without ignoring its downstream dependencies. For example, a flag like `--ignore-single <repo>` could be introduced.
