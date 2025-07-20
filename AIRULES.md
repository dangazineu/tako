# AI Rules for `tako`

This document provides a set of rules and guidelines for AI-assisted development on the `tako` project. The goal is to ensure that all contributions are consistent, high-quality, and align with the project's standards.

## 1. Introduction

These rules are designed to be used by an AI assistant to help with development tasks. They are based on the project's existing documentation, configuration, and conventions. All AI-generated code and contributions should adhere to these rules.

## 2. Getting Started

Before starting, ensure your development environment is set up correctly:

 **Install `tako` and `takotest`:**
    ```bash
    go install ./cmd/tako
    go install ./cmd/takotest
    ```

## 3. Development Workflow

### 3.1. Branching

All new development should be done on a feature branch. Branches should be named using the format `feature/<issue-number>-<short-description>`. For example: `feature/52-add-new-command`.

### 3.2. Commits

This project follows the [Conventional Commits](https://www.conventionalcommits.org/en/v1.0.0/) specification. Each commit message should have a clear and descriptive message, prefixed with a type (e.g., `feat`, `fix`, `docs`, `test`).
Commit messages should not reference the issue number, instead they should describe the change itself. 

### 3.3. Pull Requests

-   PR titles should follow the same standard established for commit messages.
-   The PR description should refer to itself as a "change" not a "pull request" (i.e.: "This change introduces an integration testing framework"). 
-   The PR description should clearly explain the changes and how to test them.
-   All CI checks must pass before a PR can be merged.
-   The last line on the Pull Request should reference the issue that it addresses (i.e.: "Fixes #5" or "Related to #7").

## 4. Code Style and Quality

-   All code must be formatted with `gofmt`.
-   All linter tests are implemented in `linter_test.go`, which is invoked when you run `go test -v ./...`.

## 5. Testing

-   All new features must include unit tests.
-   Run all tests with `go test -v ./...`.
-   Run E2E tests with `go test -v -tags=e2e --local ./...` or `go test -v -tags=e2e --remote ./...`.
-   All new testing tags and flags must be documented on the `README.md` and on `AIRULES.md`.

### Pre-Commit Workflow
Before committing any changes, the following sequence of tests **must** be executed and pass to ensure the stability and quality of the codebase:

1. **Short Unit Tests:** Use this for quick feedback on functionality. These tests don't include linters, only functional tests. 
    ```bash
    go test -v -test.short ./...
    ```
2. **Unit Tests:** All the above, plus linters
    ```bash
    go test -v ./...
    ```
3. **Local End-to-End Tests:** Safe for CI, does not require external resources.
    ```bash
    go test -v -tags=e2e --local ./...
    ```
3.  **Remote End-to-End Tests:** Requires access to a shared GitHub organization (cannot be run in parallel).
    ```bash
    go test -v -tags=e2e --remote ./...
    ```
4.  **CI Simulation:**
    ```bash
    act --container-architecture linux/amd64 -P ubuntu-latest=catthehacker/ubuntu:act-latest
    ```
Only when all of these steps complete successfully should the changes be committed.

## 6. Configuration (`tako.yml`)

-   Any changes to the `tako.yml` schema must be backward compatible.
-   The `version` field in `tako.yml` should be updated according to the changes.
-   The documentation for the `tako.yml` schema must be updated to reflect any changes.

## 7. Command-Line Interface (CLI)

-   New commands should be added to the `cmd/tako` directory.
-   New commands should follow the existing Cobra conventions.
-   All new commands and flags must be documented in the `README.md`.

## 8. Documentation

-   The `README.md` file is the single source of truth for the project's documentation.
-   All new features, commands, and configuration options must be documented in the `README.md`.
-   The implementation plan in the `README.md` should be kept up-to-date.

## 9. CI/CD

To run the GitHub Actions locally, use `act`:
```bash
act --container-architecture linux/amd64 -P ubuntu-latest=catthehacker/ubuntu:act-latest
```

- `--container-architecture linux/amd64`: This flag is necessary when running on Apple M-series chips to ensure compatibility.
- `-P ubuntu-latest=catthehacker/ubuntu:act-latest`: This specifies a modern, well-maintained Docker image for the `ubuntu-latest` runner.

## 10. Repository Cache and Lookup

-   **Consistent Cache Structure:** The repository cache path must be consistent for both local and remote operations. The structure should always be `~/.tako/cache/repos/<owner>/<repo>/<branch>`.
-   **The `--local` Flag:** The `--local` flag's only purpose is to prevent network access (e.g., `git fetch` or `git clone`). It should not change the directory path where `tako` looks for a cached repository. When running with `--local`, if a repository is not found in the cache at the expected path, the operation should fail.
