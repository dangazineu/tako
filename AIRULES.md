# AI Rules for `tako`

This document provides a set of rules and guidelines for AI-assisted development on the `tako` project. The goal is to ensure that all contributions are consistent, high-quality, and align with the project's standards.

## 1. Introduction

These rules are designed to be used by an AI assistant to help with development tasks. They are based on the project's existing documentation, configuration, and conventions. All AI-generated code and contributions should adhere to these rules.

## 2. Getting Started

Before starting, ensure your development environment is set up correctly:

1.  **Install Go:** Version 1.24.4 or later.
2.  **Install `golangci-lint`:** The project uses `golangci-lint` for linting.
3.  **Install `tako` and `takotest`:**
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
-   All code must pass the linting checks defined in `.golangci.yaml`. Run `golangci-lint run` to check your code.
-   The following linters are enabled: `containedctx`, `contextcheck`, `fatcontext`, `godot`, `govet`, `ineffassign`, `misspell`, `staticcheck`, `unparam`, `unused`, `usetesting`.

## 5. Testing

-   All new features must include unit tests.
-   Run all tests with `go test -v ./...`.
-   Run integration tests with `go test -v -tags=integration ./...`.
-   Run E2E tests with `go test -v -tags=e2e --local ./...` or `go test -v -tags=e2e --remote ./...`.
-   All new testing tags and flags must be documented on the `README.md` and on `AIRULES.md`.

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
act
```