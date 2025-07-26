# Implementation Plan: Tako Exec Workflow Engine

This document outlines the detailed implementation plan for the Tako exec workflow engine, broken down into GitHub issues organized by milestones. The plan has been updated to reflect the event-driven architecture with `tako/fan-out@v1` steps and subscription-based orchestration.

## Overview

The implementation is structured into three major milestones:

1. **Milestone 3: MVP - Local, Synchronous Execution** (GitHub Milestone 9): Core single-repository functionality
2. **Milestone 4: Event-Driven Multi-Repository Orchestration** (GitHub Milestone 10): Fan-out, subscriptions, and containerization
3. **Milestone 5: Advanced Features & Production Readiness** (GitHub Milestone 11): Caching, long-running steps, and observability

Each issue is linked to the parent epic (#21) and design issue (#98). The plan assumes no existing clients need migration, allowing implementation without breaking change considerations.

**Total Issues**: 17 issues across 3 milestones, including the addition of Issue 14 (`tako lint` command) based on feedback.

## Assessment of Existing Issues

**Current Issues Linked to #21:**
- **Issue #93** - `feat(config): Add workflow schema to tako.yml` → **SUPERSEDED** by Issue 1 (comprehensive workflow schema)
- **Issue #94** - `feat(cmd): Add exec command for multi-step workflows` → **SUPERSEDED** by Issue 2 (includes event-driven architecture)
- **Issue #95** - `feat(exec): Implement workflow execution logic` → **SUPERSEDED** by Issues 3-4 (adds state management, templates, events)
- **Issue #96** - `feat(exec): Add --dry-run support to exec command` → **SUPERSEDED** by Issue 14 (comprehensive dry-run with event simulation)
- **Issue #97** - `test(exec): Add E2E tests for multi-step workflows` → **SUPERSEDED** by Issues 5, 9, 16 (comprehensive testing suite)

**Recommendation**: Close issues #93-#97 as superseded by the comprehensive event-driven design.

## Milestone 3: MVP - Local, Synchronous Execution (GitHub Milestone 9)

This milestone delivers core single-repository workflow functionality, providing immediate value and a foundation for multi-repository features.

### Issue 1: `feat(config): Implement event-driven workflow schema` → **#99**

**Related Issues:**
- Parent Epic: #21 - Execute multi-step workflows
- Design: #98 - Event-driven workflow engine design
- Supersedes: #93 - feat(config): Add workflow schema to tako.yml

**Description:**
Implement the complete workflow schema supporting event-driven architecture with subscriptions, artifacts, and event emission. This establishes the foundation for both single-repository and multi-repository orchestration.

**Key Schema Changes from Original Plan:**
- Replace `dependents` blocks with `subscriptions` configuration in child repositories
- Add `events` emission capability in `produces` blocks
- Add `subscriptions` top-level section for event-driven workflows
- Update to support `--repo` flag for parent-led execution

**Implementation Details:**
- Update `internal/config/config.go` for complete event-driven schema
- Add `Subscriptions` struct with artifact references, event filters, and CEL expressions
- Implement `EventProduction` in `produces` blocks with schema versioning
- Support `repo:artifact` format for subscription references
- Add comprehensive validation for subscription criteria and event schemas
- Ensure backward compatibility with existing non-workflow `tako.yml` files

**Schema Reference:**
```yaml
version: 0.1.0
artifacts:
  go-lib:
    path: ./go.mod
    ecosystem: go

# Parent repository workflows
workflows:
  release:
    inputs:
      version-bump:
        type: string
        validation:
          enum: [major, minor, patch]
    steps:
      - id: build
        run: ./scripts/build.sh --bump {{ .inputs.version-bump }}
        produces:
          artifact: go-lib
          outputs:
            version: from_stdout
          events:
            - type: library_built
              schema_version: "1.0.0"
              payload:
                version: "{{ .outputs.version }}"
                commit_sha: "{{ .env.GITHUB_SHA }}"
      - uses: tako/fan-out@v1
        with:
          event_type: library_built
          wait_for_children: true

# Child repository subscriptions
subscriptions:
  - artifact: my-org/go-lib:go-lib
    events: [library_built]
    schema_version: "^1.0.0"
    filters:
      - semver.major(event.payload.version) > 0
    workflow: update_integration
    inputs:
      upstream_version: "{{ .event.payload.version }}"
```

**Acceptance Criteria:**
- [ ] Complete event-driven schema supported in configuration loading
- [ ] Subscription parsing with `repo:artifact` format validation
- [ ] Event schema versioning and compatibility validation
- [ ] CEL filter expression parsing and validation
- [ ] Parent-led execution with `--repo` flag support
- [ ] Backward compatibility with existing tako.yml files maintained

**Files to Modify:**
- `internal/config/config.go` (major updates)
- `internal/config/subscription.go` (new)
- `internal/config/events.go` (new)
- Test files throughout codebase

---

### Issue 2: `feat(cmd): Implement 'tako exec' with event-driven support` → **#100**

**Related Issues:**
- Parent Epic: #21 - Execute multi-step workflows
- Design: #98 - Event-driven workflow engine design
- Supersedes: #94 - feat(cmd): Add exec command
- Depends On: #99 (event-driven schema support)

**Description:**
Implement the `tako exec` command with full support for event-driven multi-repository orchestration, including parent-led execution and event simulation for debugging.

**Key Updates from Original Plan:**
- Add `--repo` flag for parent-led multi-repository execution
- Support event simulation with `--simulate-event` for debugging
- Add `--debug-subscriptions` for subscription filter evaluation
- Implement timestamp-based run ID generation (`exec-YYYYMMDD-HHMMSS-<hash>`)

**CLI Interface:**
```bash
# Parent-led multi-repository execution
tako exec release --repo=my-org/go-lib --inputs.version-bump=minor

# Event simulation for debugging
tako exec integration_test --repo=my-org/app-one --simulate-event=library_built

# Subscription debugging
tako exec --dry-run release --debug-subscriptions

# Resume workflows from parent perspective
tako exec --resume exec-20240726-143022-a7b3c1d2 --repo=my-org/go-lib
```

**Implementation Details:**
- Extend `cmd/tako/internal/exec.go` with repository resolution
- Implement parent repository detection and validation
- Add event simulation capability for testing child workflows
- Support subscription filter debugging and evaluation
- Integrate timestamp-based run ID generation
- Add comprehensive parent-child workflow coordination

**Acceptance Criteria:**
- [ ] `--repo` flag enables parent-led multi-repository execution
- [ ] Event simulation works for debugging child repository workflows
- [ ] Subscription filter debugging shows evaluation results
- [ ] Timestamp-based run IDs generated (exec-YYYYMMDD-HHMMSS-hash format)
- [ ] Resume operations work from parent repository perspective
- [ ] Error messages provide clear guidance for multi-repository scenarios

**Files to Modify:**
- `cmd/tako/internal/exec.go` (major updates)
- `internal/engine/` package (new)
- Integration with repository discovery and caching

---

### Issue 3: `feat(engine): Implement core execution engine with state management` → **#101**

**Related Issues:**
- Parent Epic: #21 - Execute multi-step workflows
- Design: #98 - Event-driven workflow engine design
- Depends On: #100 (exec command)

**Description:**
Create the core execution engine supporting both single-repository and multi-repository workflows with comprehensive state management and workspace isolation.

**Key Updates from Original Plan:**
- Timestamp-based run ID generation for human readability
- Enhanced state management for multi-repository execution trees
- Copy-on-write workspace isolation for concurrent executions
- Fine-grained locking with deadlock detection

**State Management Enhancements:**
- Execution tree state persistence across all repositories
- Smart partial resume capability for failed branches
- State polling with exponential backoff (30s → 5m)
- Lock acquisition state persistence for resume operations

**Implementation Details:**
- Create `internal/engine/runner.go` with execution tree support
- Implement hierarchical state management for multi-repository workflows
- Add workspace management with copy-on-write overlays
- Implement timestamp-based run ID generation with collision avoidance
- Add fine-grained repository locking with dependency-aware ordering
- Support both single-repository and multi-repository execution modes

**Acceptance Criteria:**
- [ ] Execution tree state persisted across all affected repositories
- [ ] Timestamp-based run IDs generated with human-readable format
- [ ] Copy-on-write workspace isolation prevents execution conflicts
- [ ] Smart partial resume works for failed branches only
- [ ] Repository-level locking prevents concurrent access conflicts
- [ ] State polling implements exponential backoff strategy

**Files to Modify:**
- `internal/engine/runner.go` (new, comprehensive)
- `internal/engine/state.go` (new, with execution tree support)
- `internal/engine/workspace.go` (new, with isolation)
- `internal/engine/locks.go` (new, fine-grained locking)

---

### Issue 4: `feat(engine): Implement template engine with event context` → **#102**

**Related Issues:**
- Parent Epic: #21 - Execute multi-step workflows
- Design: #98 - Event-driven workflow engine design
- Depends On: #101 (execution engine)

**Description:**
Implement the template engine with comprehensive support for event-driven workflows, including event context, security functions, and performance optimizations.

**Key Updates from Original Plan:**
- Add `.event` template context for subscription-triggered workflows
- Enhanced security with `shell_quote` and other escaping functions
- Template caching with LRU eviction (100MB limit)
- Custom functions for event payload processing

**Template Context:**
```yaml
# Available contexts in templates:
# .inputs - workflow input parameters
# .steps.<id>.outputs - outputs from previous steps
# .event.payload - event payload for subscription-triggered workflows
# .trigger.artifacts - (legacy, for compatibility)

steps:
  - run: ./scripts/process.sh --version {{ .event.payload.version | shell_quote }}
  - run: |
      {{ range .event.payload.dependencies }}
      ./scripts/update.sh --dep {{ .name }} --version {{ .version }}
      {{ end }}
```

**Implementation Details:**
- Create `internal/engine/template.go` with comprehensive context support
- Implement security functions: `shell_quote`, `json_escape`, `url_encode`
- Add event-specific template functions for payload processing
- Implement template caching with LRU eviction policy
- Add validation for template variable references and security

**Acceptance Criteria:**
- [ ] Event context (`.event.payload`) available in subscription-triggered workflows
- [ ] Security functions prevent command injection attacks
- [ ] Template caching improves performance with LRU eviction
- [ ] Custom functions support event payload iteration and processing
- [ ] Template validation provides clear error messages for invalid references

**Files to Modify:**
- `internal/engine/template.go` (new, comprehensive)
- `internal/engine/security.go` (new, for template security)
- `internal/engine/context.go` (new, for template context management)

---

### Issue 5: `test(e2e): Implement single-repository workflow testing` → **#103**

**Related Issues:**
- Parent Epic: #21 - Execute multi-step workflows
- Design: #98 - Event-driven workflow engine design
- Depends On: #99-#102 (core functionality)

**Description:**
Create comprehensive E2E tests for single-repository workflows that validate the foundation for multi-repository orchestration.

**Test Coverage Enhancements:**
- Event emission and payload validation
- Template context resolution with all supported contexts
- State management with timestamp-based run IDs
- Error scenarios with improved error message validation

**Acceptance Criteria:**
- [ ] Single-repository workflows execute correctly with event emission
- [ ] Template context resolution tested for all supported contexts
- [ ] State management works with timestamp-based run IDs
- [ ] Error handling provides clear, actionable error messages
- [ ] Performance is acceptable for typical single-repository scenarios

**Files to Modify:**
- `internal/e2e/single_repo_test.go` (new)
- Test fixture files for various workflow scenarios

---

## Milestone 4: Event-Driven Multi-Repository Orchestration (GitHub Milestone 10)

This milestone introduces the core event-driven multi-repository functionality with containerization and advanced orchestration features.

### Issue 6: `feat(engine): Implement containerized execution with security hardening` → **#104**

**Related Issues:**
- Parent Epic: #21 - Execute multi-step workflows
- Design: #98 - Event-driven workflow engine design
- Depends On: #101 (execution engine)

**Description:**
Implement secure containerized step execution with comprehensive security hardening and resource management.

**Security Hardening Features:**
- Non-root execution (UID 1001)
- Read-only root filesystem
- Dropped capabilities with selective enablement
- Network isolation with optional access
- Seccomp profile enforcement

**Implementation Details:**
- Container runtime detection (Docker/Podman)
- Hierarchical resource limits (global → repository → step)
- Container lifecycle management with cleanup
- Image pull policies and private registry support
- Resource monitoring with 90% utilization warnings

**Acceptance Criteria:**
- [ ] Containers execute with comprehensive security hardening
- [ ] Hierarchical resource limits prevent resource exhaustion
- [ ] Container cleanup prevents resource leaks
- [ ] Resource monitoring provides utilization warnings
- [ ] Private registry authentication works correctly

**Files to Modify:**
- `internal/engine/container.go` (new)
- `internal/engine/security.go` (extend)
- `internal/engine/resources.go` (new)

---

### Issue 7: `feat(engine): Implement 'tako/fan-out@v1' semantic step` → **#105**

**Related Issues:**
- Parent Epic: #21 - Execute multi-step workflows
- Design: #98 - Event-driven workflow engine design
- Depends On: #101, #102 (execution engine and templates)

**Description:**
Implement the `tako/fan-out@v1` built-in step that enables event-driven multi-repository orchestration through explicit fan-out operations.

**Key Functionality:**
- Event emission with schema versioning
- Deep synchronization with DFS traversal
- Timeout handling for aggregation scenarios
- Parallel execution with concurrency limits

**Fan-Out Step Parameters:**
```yaml
- uses: tako/fan-out@v1
  with:
    event_type: library_built           # Required: event type to emit
    wait_for_children: true             # Optional: wait for all triggered workflows
    timeout: "2h"                       # Optional: timeout for waiting
    concurrency_limit: 4                # Optional: max concurrent child executions
```

**Implementation Details:**
- Create `internal/steps/fanout.go` with comprehensive event emission
- Implement repository discovery and subscription evaluation
- Add DFS traversal for deep synchronization
- Support timeout handling and partial failure scenarios
- Integrate with existing repository caching and checkout logic

**Acceptance Criteria:**
- [ ] Events emitted with correct schema versioning and payload
- [ ] Repository discovery finds all subscribed repositories
- [ ] Subscription filter evaluation works with CEL expressions
- [ ] Deep synchronization waits for complete execution tree
- [ ] Timeout handling prevents indefinite waiting

**Files to Modify:**
- `internal/steps/fanout.go` (new, central to new architecture)
- `internal/engine/discovery.go` (new)
- `internal/engine/subscription.go` (new)

---

### Issue 8: `feat(engine): Implement subscription-based workflow triggering` → **#106**

**Related Issues:**
- Parent Epic: #21 - Execute multi-step workflows
- Design: #98 - Event-driven workflow engine design
- Depends On: #105 (fan-out implementation)

**Description:**
Implement the subscription-based workflow triggering system that evaluates event filters and maps events to workflows in child repositories.

**Key Features:**
- Lazy evaluation for repositories in dependency tree only
- At-least-once delivery with idempotency handling
- Diamond dependency resolution (first-subscription-wins)
- Schema compatibility validation

**Subscription Processing:**
```yaml
subscriptions:
  - artifact: my-org/go-lib:go-lib
    events: [library_built]
    schema_version: "^1.0.0"
    filters:
      - semver.major(event.payload.version) > 0
      - has(event.payload.commit_sha)
    workflow: update_integration
    inputs:
      version: "{{ .event.payload.version }}"
```

**Implementation Details:**
- Create `internal/engine/subscriptions.go` for event processing
- Implement CEL filter evaluation with performance optimizations
- Add schema compatibility validation between producers and consumers
- Support idempotency checking for at-least-once delivery
- Handle diamond dependency scenarios with clear conflict resolution

**Acceptance Criteria:**
- [ ] Subscription filters evaluated correctly with CEL expressions
- [ ] Schema compatibility validated between event producers and consumers
- [ ] Idempotency prevents duplicate workflow executions
- [ ] Diamond dependencies resolved with first-subscription-wins policy
- [ ] Performance acceptable for typical dependency trees

**Files to Modify:**
- `internal/engine/subscriptions.go` (new)
- `internal/engine/schema.go` (new)
- `internal/engine/idempotency.go` (new)

---

### Issue 9: `feat(steps): Implement semantic steps framework` → **#107**

**Related Issues:**
- Parent Epic: #21 - Execute multi-step workflows
- Design: #98 - Event-driven workflow engine design
- Depends On: Issue 6 (containerized execution)

**Description:**
Create the semantic steps framework and implement core built-in steps for workflow automation.

**Built-in Steps to Implement:**
- `tako/checkout@v1` - Repository checkout operations
- `tako/update-dependency@v1` - Cross-ecosystem dependency updates
- `tako/create-pull-request@v1` - Automated pull request creation
- `tako/poll@v1` - Long-running step monitoring

**Framework Features:**
- Step versioning with semantic version support (@v1, @v2)
- Parameter validation and documentation
- Integration with template engine and security model
- Extensible registration system for future steps

**Implementation Details:**
- Create `internal/steps/` package with registration framework
- Implement each built-in step with comprehensive parameter validation
- Add `tako steps list` command for step discovery
- Integrate steps with execution engine and container runtime
- Create testing framework for built-in steps

**Acceptance Criteria:**
- [ ] All built-in steps work correctly with parameter validation
- [ ] Step versioning supports semantic version syntax
- [ ] `tako steps list` shows available steps and parameters
- [ ] Integration tests validate step functionality
- [ ] Framework supports easy addition of new steps

**Files to Modify:**
- `internal/steps/registry.go` (new)
- `internal/steps/checkout.go` (new)
- `internal/steps/update_dependency.go` (new)
- `internal/steps/create_pr.go` (new)
- `internal/steps/poll.go` (new)
- `cmd/tako/internal/steps.go` (new)

---

### Issue 10: `test(e2e): Implement multi-repository event-driven testing` → **#108**

**Related Issues:**
- Parent Epic: #21 - Execute multi-step workflows
- Design: #98 - Event-driven workflow engine design
- Depends On: #104-#107 (complete event-driven implementation)

**Description:**
Create comprehensive E2E tests for multi-repository event-driven workflows, validating the complete orchestration system.

**Test Scenarios:**
- Fan-out/fan-in with event-driven architecture
- Subscription filter evaluation and schema compatibility
- Diamond dependency resolution
- Partial failure recovery and smart resume
- Long-running workflows with container persistence

**Key Test: Event-Driven Fan-Out/Fan-In**
```yaml
# Parent (go-lib) emits library_built event
# Children (app-one, app-two) subscribe and emit service_updated events  
# Aggregator (release-bom) subscribes to both service_updated events
# Validates complete event flow and dependency coordination
```

**Acceptance Criteria:**
- [ ] Event-driven fan-out/fan-in scenario executes correctly
- [ ] Subscription filters evaluated properly with real repositories
- [ ] Schema compatibility validation works across repository boundaries
- [ ] Diamond dependencies resolved without conflicts
- [ ] Smart partial resume works for complex execution trees

**Files to Modify:**
- `internal/e2e/event_driven_test.go` (new)
- `internal/e2e/subscription_test.go` (new)
- Test repository fixtures with realistic configurations

---

## Milestone 5: Advanced Features & Production Readiness (GitHub Milestone 11)

This milestone adds production-ready features including performance optimizations, observability, and advanced workflow capabilities.

### Issue 11: `feat(engine): Implement step caching system` → **#109**

**Related Issues:**
- Parent Epic: #21 - Execute multi-step workflows
- Design: #98 - Event-driven workflow engine design
- Depends On: #101 (execution engine)

**Description:**
Implement content-addressable step caching with performance optimizations for multi-repository workflows.

**Caching Features:**
- Content-addressable cache keys based on step definition and repository state
- `cache_key_files` glob pattern support for selective file inclusion
- Cache storage at `~/.tako/cache` with management commands
- Integration with repository optimization (shallow cloning, sparse checkout)

**Implementation Details:**
- Create cache key generation with SHA256 hashing
- Implement cache storage and retrieval with validation
- Add `tako cache clean` command with age-based cleanup
- Support `--no-cache` flag for cache invalidation
- Optimize for large repositories with selective file hashing

**Acceptance Criteria:**
- [ ] Step results cached and reused appropriately
- [ ] Cache keys generated correctly from step definition and file content
- [ ] Cache management commands work for cleanup and maintenance
- [ ] Performance improvement measurable for repeated executions
- [ ] Cache works correctly with multi-repository scenarios

**Files to Modify:**
- `internal/engine/cache.go` (new)
- `internal/engine/hashing.go` (new)
- `cmd/tako/internal/cache.go` (new)

---

### Issue 12: `feat(engine): Implement long-running steps with container persistence` → **#110**

**Related Issues:**
- Parent Epic: #21 - Execute multi-step workflows
- Design: #98 - Event-driven workflow engine design
- Depends On: Issues 6, 9 (containerization and polling step)

**Description:**
Implement long-running step support with container persistence and advanced lifecycle management.

**Long-Running Features:**
- Container persistence after main process exit
- Output capture via `.tako/outputs.json` file
- Orphaned container cleanup with 24-hour policy
- System reboot recovery with container restart detection
- Container labeling with run ID and metadata

**Implementation Details:**
- Add `long_running: true` step support in execution engine
- Implement container persistence with restart policies
- Create output capture mechanism for long-running processes
- Add automatic cleanup for orphaned containers
- Support workflow resumption with long-running step detection

**Acceptance Criteria:**
- [ ] Long-running containers persist after main process exit
- [ ] Output capture works correctly via standardized JSON file
- [ ] Orphaned container cleanup prevents resource leaks
- [ ] System reboot recovery handled gracefully
- [ ] Workflow resumption works with long-running steps

**Files to Modify:**
- `internal/engine/longrunning.go` (new)
- `internal/engine/container.go` (extend)
- `internal/engine/cleanup.go` (new)

---

### Issue 13: `feat(cmd): Implement comprehensive status and observability` → **#111**

**Related Issues:**
- Parent Epic: #21 - Execute multi-step workflows
- Design: #98 - Event-driven workflow engine design
- Depends On: Issue 12 (long-running steps)

**Description:**
Implement comprehensive status reporting and observability for multi-repository workflows.

**Status Features:**
- Execution tree visualization across all repositories
- Long-running container status and resource usage
- Event flow tracking and subscription evaluation history
- Performance metrics and bottleneck identification

**Status Commands:**
```bash
# Comprehensive workflow status
tako status exec-20240726-143022-a7b3c1d2

# Repository-specific status
tako status exec-20240726-143022-a7b3c1d2 --repo=my-org/app-one

# Event flow debugging
tako status exec-20240726-143022-a7b3c1d2 --show-events
```

**Implementation Details:**
- Create `cmd/tako/internal/status.go` with tree visualization
- Implement event flow tracking and history
- Add resource usage monitoring and reporting
- Support debugging commands for subscription evaluation
- Integrate with container runtime for long-running step monitoring

**Acceptance Criteria:**
- [ ] Status shows execution tree across all repositories
- [ ] Long-running container status displayed correctly
- [ ] Event flow tracking provides debugging information
- [ ] Resource usage monitoring identifies bottlenecks
- [ ] Status information actionable for operators and developers

**Files to Modify:**
- `cmd/tako/internal/status.go` (new)
- `internal/engine/monitoring.go` (new)
- `internal/engine/visualization.go` (new)

---

### Issue 14: `feat(cmd): Implement tako lint command for configuration validation` → **#112**

**Related Issues:**
- Parent Epic: #21 - Execute multi-step workflows
- Design: #98 - Event-driven workflow engine design
- Depends On: #99 (configuration schema)

**Description:**
Implement `tako lint` command for comprehensive validation of `tako.yml` configuration files, including syntax, semantics, and best practices.

**Key Features:**
- Schema validation for workflows, subscriptions, and artifacts
- CEL expression syntax validation
- Subscription criteria validation and warnings
- Template syntax validation
- Security best practices validation

**Implementation Details:**
- Create `cmd/tako/internal/lint.go` with validation framework
- Implement comprehensive configuration file validation
- Add warnings for potential issues and anti-patterns
- Support `--strict` mode for enhanced validation
- Integrate with existing configuration loading logic

**Acceptance Criteria:**
- [ ] Complete syntax validation for all tako.yml sections
- [ ] CEL expression validation with clear error messages
- [ ] Subscription criteria validation and compatibility checks
- [ ] Template syntax validation without execution
- [ ] Security best practices validation and warnings

**Files to Modify:**
- `cmd/tako/internal/lint.go` (new)
- `internal/config/validation.go` (new)
- Integration with existing configuration loading

---

### Issue 15: `feat(exec): Implement comprehensive dry-run mode` → **#113**

**Related Issues:**
- Parent Epic: #21 - Execute multi-step workflows
- Design: #98 - Event-driven workflow engine design
- Depends On: Issues 7-8 (event-driven implementation)

**Description:**
Implement comprehensive dry-run functionality for multi-repository event-driven workflows.

**Dry-Run Features:**
- Execution plan visualization across repository boundaries
- Event simulation with subscription evaluation
- Template variable resolution without execution
- Resource requirement analysis and validation
- Dependency graph visualization

**Implementation Details:**
- Add execution plan generation for multi-repository scenarios
- Implement event simulation for debugging subscription filters
- Support template resolution display without side effects
- Create dependency graph visualization
- Add resource requirement analysis and validation

**Acceptance Criteria:**
- [ ] Dry-run shows execution plan across all affected repositories
- [ ] Event simulation evaluates subscription filters correctly
- [ ] Template variables resolved and displayed without execution
- [ ] Dependency graph visualization aids debugging
- [ ] No side effects occur during dry-run mode

**Files to Modify:**
- `internal/engine/dryrun.go` (new)
- `internal/engine/simulation.go` (new)
- `cmd/tako/internal/exec.go` (extend)

---

### Issue 16: `feat(engine): Implement cancellation and cleanup` → **#114**

**Related Issues:**
- Parent Epic: #21 - Execute multi-step workflows
- Design: #98 - Event-driven workflow engine design
- Depends On: #110-#112 (long-running steps and status)

**Description:**
Implement workflow cancellation and comprehensive cleanup across multi-repository execution trees.

**Cancellation Features:**
- `tako cancel <run-id>` command for explicit cancellation
- Graceful termination of in-flight steps across repositories
- `on_cancellation` blocks for cleanup actions
- Partial cancellation (some repositories complete, others cancelled)

**Implementation Details:**
- Create `cmd/tako/internal/cancel.go` command
- Implement graceful step termination across execution tree
- Add cancellation state propagation to all affected repositories
- Support cleanup actions through `on_cancellation` blocks
- Integrate with container lifecycle management for proper cleanup

**Acceptance Criteria:**
- [ ] Cancellation works across complete execution tree
- [ ] In-flight steps terminated gracefully without data corruption
- [ ] Cleanup actions executed across all affected repositories
- [ ] Partial cancellation supported for complex scenarios
- [ ] Status commands show cancellation state clearly

**Files to Modify:**
- `cmd/tako/internal/cancel.go` (new)
- `internal/engine/cancellation.go` (new)
- `internal/engine/cleanup.go` (extend)

---

### Issue 17: `test(e2e): Implement comprehensive production testing` → **#115**

**Related Issues:**
- Parent Epic: #21 - Execute multi-step workflows
- Design: #98 - Event-driven workflow engine design
- Depends On: Issues 11-16 (all advanced features)

**Description:**
Create comprehensive production-readiness testing including performance, scalability, and chaos testing.

**Test Categories:**
- Performance testing with 50+ repository scenarios
- Scalability testing for execution tree depth
- Chaos testing for network partitions and failures
- Security testing for malicious configurations
- Long-running workflow testing with persistence

**Advanced Test Scenarios:**
- Thundering herd on resume (20+ child repositories)
- State migration testing across Tako versions
- Mutation testing for critical code paths
- Property-based testing for template security
- Consumer-driven contract testing for events

**Acceptance Criteria:**
- [ ] Performance tests validate scalability up to 50 repositories
- [ ] Chaos tests validate resilience under failure conditions
- [ ] Security tests prevent malicious configuration attacks
- [ ] Advanced testing strategies identify potential issues
- [ ] Test suite provides confidence for production deployment

**Files to Modify:**
- `internal/e2e/performance_test.go` (new)
- `internal/e2e/chaos_test.go` (new)
- `internal/e2e/security_test.go` (new)
- `internal/e2e/advanced_test.go` (new)

---

## Implementation Guidelines

### Dependency Management

**Critical Path Dependencies:**
- Issues 1-4 form the core foundation and must be completed sequentially
- Issue 7 (fan-out) is the cornerstone of the event-driven architecture
- Issue 8 (subscriptions) depends heavily on Issue 7
- Issues 12-13 (long-running and status) are interdependent

**Parallel Development Opportunities:**
- Issues 6 (containerization) and 9 (semantic steps) can be developed in parallel after Issue 3
- Testing issues (#103, #108, #115) can be developed alongside their corresponding feature issues
- Issues #109 (caching) and #113 (dry-run) are largely independent after core functionality

### Quality Assurance

**Code Quality Standards:**
- Minimum 85% code coverage for new code
- All E2E scenarios must pass before milestone completion
- Performance regression limits: <10% degradation
- Security tests must achieve 100% pass rate

**Testing Strategy Integration:**
- Each issue includes comprehensive unit tests
- Integration tests validate cross-component functionality
- E2E tests validate complete scenarios
- Performance tests validate scalability requirements

### Release Strategy

**Milestone Releases:**
- Milestone 3: Single-repository workflow engine (immediate value)
- Milestone 4: Multi-repository event-driven orchestration (core differentiator)  
- Milestone 5: Production-ready feature complete system

**Feature Flags:**
- Event-driven features can be flag-gated during development
- Container execution can fall back to host execution
- Advanced features can be progressively enabled

This implementation plan provides a comprehensive roadmap for building a robust, event-driven multi-repository workflow orchestration system that scales from simple single-repository automation to complex enterprise-grade multi-repository coordination.

The plan incorporates feedback from comprehensive review and validation, ensuring all aspects of the original design have been captured and evolved appropriately for the event-driven architecture.