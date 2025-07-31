# Issue #145 Implementation Plan - Protobuf API Evolution E2E Test

## Overview
Create a comprehensive E2E test that verifies tako's fan-out orchestration capabilities for a real-world Protobuf API evolution scenario. The test will validate selective triggering, CEL expression evaluation, and proper isolation of non-affected services.

## Phase 1: Create Repository Templates (Foundation)

### 1.1 Create Directory Structure
**Duration**: 15 minutes  
**Health Check**: Directory structure exists and follows existing patterns

Create the template directory structure:
```
test/e2e/templates/protobuf-api-evolution/
├── api-definitions/
│   ├── tako.yml
│   └── proto/
│       └── user.proto (placeholder)
├── go-user-service/
│   ├── tako.yml
│   └── scripts/
│       └── deploy.sh
├── nodejs-billing-service/
│   ├── tako.yml
│   └── scripts/
│       └── deploy.sh
└── go-legacy-service/
    ├── tako.yml (minimal/empty)
    └── README.md
```

### 1.2 Implement api-definitions Repository
**Duration**: 30 minutes  
**Health Check**: Publisher workflow can emit events with proper payload structure

Create `api-definitions/tako.yml`:
```yaml
version: v1
workflows:
  release-api:
    inputs:
      version: { type: string, required: true }
      changed_services: { type: string, required: true }
    steps:
      - id: tag-release
        run: "git tag {{ .Inputs.version }} && echo 'tagged_{{ .Inputs.version }}' > pushed_tag_{{ .Inputs.version }}"
      - id: fan-out-event
        uses: tako/fan-out@v1
        with:
          event_type: "api_published"
          schema_version: "1.0.0"
          payload:
            git_tag: "{{ .Inputs.version }}"
            services_affected: "{{ .Inputs.changed_services }}"
events:
  produces:
    - type: "api_published"
      schema_version: "1.0.0"
```

Add placeholder proto file for realism.

### 1.3 Implement Consumer Services
**Duration**: 45 minutes  
**Health Check**: Consumer repositories have correct subscription filters and mock deployment scripts

Create `go-user-service/tako.yml`:
```yaml
version: v1
subscriptions:
  - artifact: "acme-corp/api-definitions:main"
    events: ["api_published"]
    schema_version: "^1.0.0"
    filters:
      - "'user-service' in event.payload.services_affected.split(',').map(s, s.trim().lower())"
    workflow: "update-and-deploy"
    inputs:
      api_version: "{{ .event.payload.git_tag }}"
      repo_name: "go-user-service"
workflows:
  update-and-deploy:
    inputs:
      api_version: { type: string }
      repo_name: { type: string }
    steps:
      - run: "echo 'Updating Go service to API {{ .Inputs.api_version }}'"
      - run: "echo '{{ .Inputs.api_version }}' > {{ .Inputs.repo_name }}_deployed_with_api_{{ .Inputs.api_version }}"
      - run: "./scripts/deploy.sh {{ .Inputs.api_version }}"
```

Create `nodejs-billing-service/tako.yml` (similar structure with billing-service filter).

Create `go-legacy-service/tako.yml` (minimal - no subscriptions).

### 1.4 Create Mock Deployment Scripts
**Duration**: 15 minutes  
**Health Check**: Mock scripts create verifiable side effects

Create deployment scripts that output verifiable files:
```bash
#!/bin/bash
# deploy.sh for each service
mkdir -p $(dirname $0)
echo "Deployed with API version: $1" > "deployment_log_$1.txt"
echo "$1" > "last_deployed_version.txt"
# Append to log for idempotency testing
echo "$(date '+%Y-%m-%d %H:%M:%S'): Deployed version $1" >> "deployment_history.log"
```

## Phase 2: Add Test Environment Configuration (Integration)

### 2.1 Add Environment Definition
**Duration**: 20 minutes  
**Health Check**: New environment appears in test environment map

Add to `test/e2e/environments.go`:
```go
"protobuf-api-evolution": {
    Name: "protobuf-api-evolution",
    Repositories: []RepositoryDef{
        {
            Name:   "api-definitions",
            Branch: "main",
            Files: []FileDef{
                {Path: "tako.yml", Template: "protobuf-api-evolution/api-definitions/tako.yml"},
                {Path: "proto/user.proto", Template: "protobuf-api-evolution/api-definitions/proto/user.proto"},
            },
        },
        {
            Name:   "go-user-service",
            Branch: "main",
            Files: []FileDef{
                {Path: "tako.yml", Template: "protobuf-api-evolution/go-user-service/tako.yml"},
                {Path: "scripts/deploy.sh", Template: "protobuf-api-evolution/go-user-service/scripts/deploy.sh"},
            },
        },
        {
            Name:   "nodejs-billing-service",
            Branch: "main",
            Files: []FileDef{
                {Path: "tako.yml", Template: "protobuf-api-evolution/nodejs-billing-service/tako.yml"},
                {Path: "scripts/deploy.sh", Template: "protobuf-api-evolution/nodejs-billing-service/scripts/deploy.sh"},
            },
        },
        {
            Name:   "go-legacy-service",
            Branch: "main",
            Files: []FileDef{
                {Path: "tako.yml", Template: "protobuf-api-evolution/go-legacy-service/tako.yml"},
                {Path: "README.md", Template: "protobuf-api-evolution/go-legacy-service/README.md"},
            },
        },
    },
},
```

## Phase 3: Implement Core Test Case (Testing Logic)

### 3.1 Add Basic Test Case Structure
**Duration**: 30 minutes  
**Health Check**: Test case appears in test list and can be discovered

Add to `test/e2e/get_test_cases.go`:
```go
{
    Name:        "protobuf-api-evolution",
    Environment: "protobuf-api-evolution",
    ReadOnly:    false,
    Test: []Step{
        // Will be populated in next phase
    },
    Verify: Verification{
        // Will be populated in next phase
    },
},
```

### 3.2 Implement Multi-Scenario Test Steps
**Duration**: 60 minutes  
**Health Check**: All test scenarios execute without errors

Add comprehensive test steps covering:

**Scenario 1: Single Service Trigger**
```go
{
    Name:    "trigger user-service only",
    Command: "tako",
    Args:    []string{"exec", "release-api", "--inputs.version=v1.0.0", "--inputs.changed_services=user-service"},
    AssertOutputContains: []string{
        "Executing workflow 'release-api'",
        "Success: true",
    },
},
```

**Scenario 2: Multiple Services Trigger**
```go
{
    Name:    "trigger both user-service and billing-service",
    Command: "tako",
    Args:    []string{"exec", "release-api", "--inputs.version=v1.1.0", "--inputs.changed_services=user-service,billing-service"},
    AssertOutputContains: []string{
        "Executing workflow 'release-api'",
        "Success: true",
    },
},
```

**Scenario 3: Edge Cases**
```go
{
    Name:    "trigger with whitespace and malformed list",
    Command: "tako",
    Args:    []string{"exec", "release-api", "--inputs.version=v1.2.0", "--inputs.changed_services= user-service , "},
    AssertOutputContains: []string{
        "Executing workflow 'release-api'",
        "Success: true",
    },
},
{
    Name:    "trigger with case-insensitive service names",
    Command: "tako",
    Args:    []string{"exec", "release-api", "--inputs.version=v1.2.1", "--inputs.changed_services=User-Service,BILLING-SERVICE"},
    AssertOutputContains: []string{
        "Executing workflow 'release-api'",
        "Success: true",
    },
},
{
    Name:    "trigger with duplicate services in list",
    Command: "tako",
    Args:    []string{"exec", "release-api", "--inputs.version=v1.2.2", "--inputs.changed_services=user-service,user-service,billing-service"},
    AssertOutputContains: []string{
        "Executing workflow 'release-api'",
        "Success: true",
    },
},
```

**Scenario 4: No Matching Services**
```go
{
    Name:    "trigger with no matching services",
    Command: "tako",
    Args:    []string{"exec", "release-api", "--inputs.version=v1.3.0", "--inputs.changed_services=inventory-service"},
    AssertOutputContains: []string{
        "Executing workflow 'release-api'",
        "Success: true",
    },
},
```

### 3.3 Implement Comprehensive Verification
**Duration**: 45 minutes  
**Health Check**: Verification correctly validates positive and negative cases

Add verification steps for each scenario:
```go
Verify: Verification{
    Files: []VerifyFileExists{
        // Positive cases
        {
            FileName:        "go_user_service_deployed_with_api_v1.0.0",
            ShouldExist:     true,
            ExpectedContent: "v1.0.0",
        },
        {
            FileName:        "nodejs_billing_service_deployed_with_api_v1.1.0",
            ShouldExist:     true,
            ExpectedContent: "v1.1.0",
        },
        // Negative cases
        {
            FileName:    "go_user_service_deployed_with_api_v1.3.0",
            ShouldExist: false,
        },
        // Legacy service should never have files
        {
            FileName:    "go_legacy_service_deployed_with_api_v1.0.0",
            ShouldExist: false,
        },
    },
},
```

## Phase 4: Advanced Test Features (Enhancement)

### 4.1 Add Idempotency Testing
**Duration**: 30 minutes  
**Health Check**: Same event executed twice produces same result without duplication

Add test step to verify idempotency:
```go
{
    Name:    "verify idempotency - run same event twice",
    Command: "tako",
    Args:    []string{"exec", "release-api", "--inputs.version=v1.0.0", "--inputs.changed_services=user-service"},
    AssertOutputContains: []string{
        "Success: true",
    },
},
```

Add verification that deployment happens only once by checking deployment_history.log contains only one entry for v1.0.0.

### 4.2 Add Error Handling Tests
**Duration**: 30 minutes  
**Health Check**: Invalid configurations are handled gracefully

Add test scenarios for:
- Invalid tako.yml in consumer repository
- CEL expression evaluation errors
- Network/permission issues (simulated)

### 4.3 Enhanced Logging and Debugging
**Duration**: 20 minutes  
**Health Check**: Test output provides clear debugging information

Add debug output verification:
- Log inspection for CEL filter evaluation results
- Verification of skipped workflows with reasons
- Timeline of fan-out execution

## Phase 5: Integration and Polish (Completion)

### 5.1 Template Placeholder Resolution
**Duration**: 20 minutes  
**Health Check**: All template placeholders are correctly resolved

Ensure all `{{.Owner}}` and repository references work correctly in test templates.

### 5.2 Test Isolation and Cleanup
**Duration**: 15 minutes  
**Health Check**: Test runs cleanly multiple times without interference

Verify that:
- Mock files are properly cleaned up
- No state leaks between test runs
- Concurrent test execution works

### 5.3 Documentation and Comments
**Duration**: 15 minutes  
**Health Check**: Code is well-documented and maintainable

Add comprehensive comments explaining:
- Complex CEL expressions
- Test scenario rationale
- Verification logic

## Success Criteria

At the end of each phase, the following must be true:
1. **All unit tests pass**: `go test -v ./...`
2. **All local E2E tests pass**: `go test -v -tags=e2e --local ./...`
3. **Code formatting is clean**: `go fmt ./...`
4. **No linter errors**: Covered by unit tests

## Risk Mitigation

**Phase 1-2 Risks**: Template syntax errors, incorrect file paths
- **Mitigation**: Test template resolution in isolation first

**Phase 3 Risks**: Complex CEL expressions, timing issues
- **Mitigation**: Add CEL unit tests, use deterministic verification

**Phase 4-5 Risks**: Test flakiness, edge case coverage gaps
- **Mitigation**: Comprehensive verification matrix, multiple test runs

## Rollback Plan

If any phase fails:
1. Revert to previous working commit
2. Analyze failure in isolation
3. Fix root cause before proceeding
4. Re-run full test suite to ensure stability

Each phase leaves the codebase in a healthy, compilable state.