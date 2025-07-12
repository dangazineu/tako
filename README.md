# Tako: Specification

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

## 2. Core Concepts

### 2.1. Workspace & Repository Management
*   **Workspace Root:** A "workspace" is not a formal concept with a global configuration file. For any given `tako` command, the **workspace root is the repository from which the command is executed**.
*   **Repository Sourcing:** The initial implementation of Tako will operate with a hybrid model. The workspace root repository is the local version, which can have uncommitted changes. All downstream dependent repositories will be cloned fresh from GitHub into a temporary directory for the duration of the command execution.
*   **Future Support:** Future versions will support using existing local copies of dependent repositories to allow for more complex, multi-repository changes.

### 2.2. Dependency Graph
*   **Definition:** The dependency graph is defined manually via `tako.yml` files within each repository.
*   **Dependency Declaration:** The `tako.yml` file lists the repositories that *depend on* the current repository, specified in the format `owner/repo:branch`. This inverse declaration is crucial for propagating operations outwards.
*   **Multiple Dependencies (Fan-in):** The graph model fully supports scenarios where a single repository is a dependent of multiple upstream repositories (e.g., a web client depending on both an API service and a shared library). The topological sort execution model ensures that any given repository is processed only once per `tako` run, after all of its direct dependencies have completed their execution.
*   **Circular Dependencies:** If a circular dependency is detected, Tako will refuse to operate and will output a clear error message identifying the cycle.

### 2.3. Execution Model
*   **Order & Parallelism:** Operations are executed based on a topological sort of the dependency graph. Independent branches are processed in parallel by default (`--serial` flag available).
*   **Error Handling:** Execution halts on the first error by default. `--continue-on-error` and `--summarize-errors` flags provide more flexible control.
*   **Rollback:** For path-based overrides, file restoration is guaranteed. For other multi-step workflow failures, the initial version will offer "best-effort" cleanup. True transactional rollback is a future goal.

### 2.4. Inter-Repository Artifacts & Local Testing
For testing local changes against dependent repositories, Tako will use a **Path-Based Override** strategy. This is managed through an `artifacts` block in the `tako.yml` of the source repository.

*   **Artifact Definition:** A source repository can define multiple, named artifacts. Each artifact has its own build command, output path, and a corresponding `install_command` that instructs dependents on how to consume it.
*   **Granular Dependency:** A dependent repository declares which specific named artifact(s) it requires from its dependencies.
*   **Execution Flow:**
    1.  When a `tako` workflow is run, it identifies which artifacts are required by downstream repositories.
    2.  It runs the `command` for **only the required artifacts** in the source repository.
    3.  For each dependent, it executes the corresponding `install_command`, passing the path to the freshly built artifact.
*   **Guarantee:** Tako **must** guarantee that any files modified by an `install_command` are reverted to their original state after the run, regardless of success or failure.

### 2.5. Containerized Execution Environments
To solve the problem of managing complex, language-specific toolchains, Tako supports running workflows and **artifact builds** in a containerized environment.
*   **Mechanism:** A workflow or an artifact definition can optionally specify a Docker `image`. If specified, Tako will not execute commands on the host machine. Instead, it will:
    1.  Start a Docker container using the specified image.
    2.  Mount the repository's directory into the container's working directory.
    3.  Execute all commands inside the container.
*   **Workspace Consistency:** This ensures that every team member, and the CI/CD system, runs commands in the exact same environment, eliminating "it works on my machine" issues.
*   **Prerequisites:** This feature requires the user to have Docker installed and running on the host machine.

## 3. Command-Line Interface (CLI)

*   **Syntax:** `tako <command> [options] [args]`
*   **Core Commands:** `version`, `graph`, `run <command>`, `exec <workflow>`
*   **Flags:**
    *   `--dry-run`: Preview the commands that would be executed without running them.
    *   `--verbose`/`--debug`: Provide detailed output for troubleshooting.
    *   `--only <repo>` / `--ignore <repo>`: Allow filtering to target specific parts of the dependency graph.
*   **UX:**
    *   **Interrupts:** `Ctrl+C` should gracefully stop all running tasks and perform necessary cleanup.
    *   **Timeouts:** Commands will have a configurable global timeout.
    *   **Progress:** A progress indicator will be displayed for long-running operations.

## 4. Configuration (`tako.yml`)

The `tako.yml` file is the heart of Tako.
*   **Schema Versioning:** A `version` field will be included for future compatibility.
*   **Artifact-centric Schema:**

    ```yaml
    # Version of the tako.yml schema
    version: "1.1"

    # Metadata about the repository
    metadata:
      name: "my-service"
      description: "Core API service"

    # Defines the artifacts this repository can produce for local testing.
    artifacts:
      api-client:
        description: "The generated Go API client"
        # This build runs in a container to ensure the right tools are present.
        image: "golang:1.21-alpine"
        command: "make generate-go-client"
        path: "./sdk/go/client.zip"
        # How a dependent should consume THIS artifact.
        # ${TAKO_ARTIFACT_PATH} is replaced by the absolute path to the artifact.
        install_command: "unzip -o ${TAKO_ARTIFACT_PATH} -d ./vendor/api-client"
      docs:
        description: "The generated API documentation"
        command: "make generate-docs"
        path: "./dist/docs.tar.gz"
        install_command: "tar -xzf ${TAKO_ARTIFACT_PATH} -C ./public/docs"

    # Repositories that depend on this one.
    dependents:
      - repo: "my-org/client-a:main"
        # This dependent needs the 'api-client' artifact.
        artifacts: ["api-client"]
        # Optionally, limit the workflows that are propagated to this dependent repo
        workflows: ["test-ci"]
      - repo: "my-org/docs-website:main"
        # This dependent needs the 'docs' artifact.
        artifacts: ["docs"]

    # Pre-defined command sequences.
    workflows:
      test-local:
        # This workflow runs on the host machine.
        steps:
          - go clean -testcache
          - go test ./...
      test-ci:
        # This workflow runs inside a container for consistency.
        image: "golang:1.21-alpine"
        steps:
          - go test -v ./...
    ```

## 5. Security
*   **Command Execution:** Tako executes shell commands defined in `tako.yml` files. This implies a level of trust in the repositories being used. A flag (e.g., `--allow-unsafe-workflows`) may be required to run potentially destructive workflows.
*   **Path Validation:** All file paths read from configuration will be validated to prevent directory traversal attacks.

---

## 6. Implementation Plan

### Milestone 0: Research & Validation
*Goal: Prove the core concepts with a minimal prototype.*
1.  **Prototype:** Build a proof-of-concept script with 2-3 real repositories to validate the inverse dependency model and the path-based override mechanism.
2.  **Refine Spec:** Adjust the specification based on prototype findings.

### Milestone 1: Project Foundation & Graphing
*Goal: Establish the project and visualize the dependency graph.*
1.  **Project Setup:** Initialize Go module, Cobra CLI, and directory structure.
2.  **Configuration & Validation:** Implement `config` loading for `tako.yml`. Add validation for schema, paths, and cycles.
3.  **Graph Construction:** Implement the `graph` package to build the dependency graph from a starting repository.
4.  **`tako graph` Command:** Implement the `tako graph` command with text and DOT output.

### Milestone 2: Basic Command Execution
*Goal: Run a single command across all repositories.*
1.  **Command Runner:** Implement the `runner` package with support for timeouts and interrupt handling.
2.  **`tako run` Command:** Implement `tako run` with support for `--dry-run`, `--only`, and `--ignore`.
3.  **Execution & Output:** Implement parallel execution with topological sorting and grouped output.

### Milestone 3: Workflows & Local Testing
*Goal: Enable multi-step workflows and downstream testing.*
1.  **Workflow Engine:** Extend the runner to execute multi-step workflows defined in `tako.yml`.
2.  **Path-Override Logic:** Implement the file modification and guaranteed restoration logic for local testing.
3.  **`tako exec` Command:** Implement the `tako exec` command to run workflows.
4.  **Context Passing:** Implement environment variable context passing (`TAKO_...`).

### Milestone 4: Containerized Workflows & Builds
*Goal: Enable reproducible builds by running workflows and artifact builds in Docker containers.*
1.  **Docker Integration:** Implement the logic to detect an `image` property on workflows and artifacts, and execute the corresponding steps/commands using `docker run`.
2.  **Volume Mounting:** Ensure the repository directory is correctly mounted into the container.
3.  **Workspace & Artifacts:** Ensure that `TAKO_` environment variables and paths to dependent artifacts are correctly passed into the container.

### Milestone 5: Polish & Developer Experience
*Goal: Make Tako robust and user-friendly.*
1.  **`tako init`:** Create a command to bootstrap a new `tako.yml` file.
2.  **`tako doctor`:** Create a command to validate the workspace and configuration, including checking for Docker availability.
3.  **Shell Completion:** Add shell completion support (bash, zsh, fish).
4.  **Logging:** Implement a robust logging strategy with multiple levels (`info`, `debug`).

### Milestone 6: Distribution & Documentation
*Goal: Prepare Tako for its first release.*
1.  **Git Commands:** Implement the built-in convenience commands (`tako status`, etc.).
2.  **CI/CD:** Set up GitHub Actions to build, run unit tests, and run integration tests (including containerized workflows).
3.  **Release Automation:** Automate cross-platform binary builds and releases.
4.  **Documentation:** Write a comprehensive user guide and create examples for both local and containerized workflows.
5.  **Homebrew:** Create and document the Homebrew formula.

## 7. Future Features
*   Watch mode for automatic rebuilds on file changes.
*   A plugin system for custom command types and integrations.
*   Caching of dependency resolution and build artifacts.
*   Support for using local copies of dependent repositories.