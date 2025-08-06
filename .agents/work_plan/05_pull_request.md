# Pull Request and Integration Phase

This phase handles the creation and monitoring of the pull request until successful integration.

## 12. Create Pull Request

- Use conventional commit format for PR title
- Include comprehensive testing instructions
- Reference the issue on the last line (e.g., "Fixes #123")

## 13. Monitor and Fix CI

- Wait 2 minutes, then check PR status
- If automated checks fail:
  - Gather failure logs
  - Reproduce issues locally
  - Fix problems
  - Run full test suite locally:
    - Format the code: `go fmt ./...`
    - Run linters: `go test -v .`
    - Run unit tests:
      1. `go test -v -race ./internal/...`
      2. `go test -v -race ./cmd/tako/...`
    - Run e2e tests: `go test -v -tags=e2e --local ./...`
    - `go test -v -tags=e2e --remote ./...` # Remote E2E tests
    - `act --container-architecture linux/amd64 -P ubuntu-latest=catthehacker/ubuntu:act-latest` # CI simulation
    - Fix any issues and run again until no issues are found
  - Commit and push fixes, only after you have executed all the steps above. Do not push until all tests pass locally.
  - Repeat until all checks pass
  - Do not exit this step until the Pull Request checks have finished and succeeded

## 14. Completion

- Report issue resolution once all checks pass

## Key Requirements

- **Conventional Commits**: Follow the established commit message format
- **Comprehensive Testing**: Include clear instructions for testing the changes
- **Issue Reference**: Always reference the original issue in the PR description
- **CI Success**: All automated checks must pass before considering the work complete
- **Persistence**: Continue fixing issues until all CI checks succeed