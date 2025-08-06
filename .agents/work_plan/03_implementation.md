# Implementation Phase

This phase executes the planned development work with continuous testing and quality assurance.

## 7. Phase-by-Phase Development

- For each phase in the plan:
  - Implement the planned changes
  - Repeat the following steps till no issues are found:
    - Format the code: `go fmt ./...`
    - Run all tests (add `-v` flag to understand failures, if any)
      - Run linters: `go test .`
      - Run unit tests:
        1. `go test -race ./internal/...`
        2. `go test -race ./cmd/tako/...`
      - Run e2e tests: `go test -tags=e2e --local ./...`
      - Fix any failing tests
  - Verify test coverage:
    - After you've run the linters, `coverage.out` will be updated, compare the current coverage with the baseline you recorded
    - Overall coverage cannot drop more than 1%
    - Individual function coverage cannot drop more than 10%
    - Add tests if needed to maintain coverage
  - Update dependencies if needed: `go mod tidy`
  - If current overall coverage went up from baseline, update `issue_coverage.md` with current coverage numbers
  - Mark phase complete in `issue_plan.md`
  - Commit phase completion

## 8. Integration Testing

- Once all implementation phases are completed, run comprehensive test suite:
  - Format the code: `go fmt ./...`
  - Run all tests (add `-v` flag to understand failures, if any)
    - Run linters: `go test .`
    - Run unit tests:
      - `go test -race ./internal/...`
      - `go test -race ./cmd/tako/...`
    - Run e2e tests: 
      - `go test -tags=e2e --local ./...` # Local E2E tests
      - `go test -tags=e2e --remote ./...` # Remote E2E tests (require GH API Token)
      - `act --container-architecture linux/amd64 -P ubuntu-latest=catthehacker/ubuntu:act-latest` # CI simulation
- Fix any issues and run again until no issues are found
- Commit fixes

## 9. Implementation Review

- Ask Gemini to review implementation, providing:
  - All background documentation
  - Implementation plan
  - Relevant tests and code
  - Design documents
- If actionable feedback aligns with project goals:
  - Add new phases to plan
  - Return to step 7 for additional work
- Commit any changes

## Key Requirements

- **Quality Gates**: Each phase must pass all tests before proceeding
- **Test Coverage**: Maintain coverage standards throughout development
- **Incremental Progress**: Each phase leaves the codebase in a healthy state
- **External Review**: Use Gemini feedback to improve implementation quality