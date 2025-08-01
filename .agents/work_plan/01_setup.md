# Setup Phase

This phase prepares the development environment and establishes a clean baseline for feature implementation.

## 1. Create Feature Branch

- Create branch using format: `feature/<issue-number>-<short-description>`
- Switch to the new branch

## 2. Establish Baseline

- Run full test suite to ensure clean starting state (add `-v` flag to understand failures, if any):
  - Run linters: `go test .`
  - Run unit tests:
    - `go test -race ./internal/...`
    - `go test -race ./cmd/tako/...`
  - Run e2e tests: `go test -tags=e2e --local ./...`
- Generate and record test coverage baseline:
  - Generate coverage profile: `go test -coverprofile=coverage.out ./...`
  - Record coverage numbers in `issue_coverage.md` (read from the generated `coverage.out` file). This will be the baseline we will compare to as we continue changing the project.
- Commit baseline coverage

## Key Requirements

- Ensure all tests pass before proceeding to the next phase
- The baseline commit serves as a reference point for the entire feature development