# Design: Tako Exec Workflow Engine

This document provides the complete technical design for the `tako exec` workflow engine, a general-purpose system for multi-repository automation with centralized orchestration and distributed subscription criteria.

## Glossary

- **Artifact**: A tangible, versionable output of a repository (e.g., a library, binary, or package) that serves as a dependency for other repositories
- **Event**: A structured message emitted by a workflow when an artifact is produced or updated, containing metadata and payload data
- **Subscription**: A repository's declaration of interest in specific events from other repositories, with filtering criteria that map events to local workflows
- **Fan-Out**: The process of triggering multiple workflows across repositories in parallel when a workflow emits events
- **Parent Repository**: A repository that orchestrates multi-repository operations by emitting events through `tako/fan-out@v1` steps
- **Child Repository**: A repository that participates in orchestration by defining subscriptions to events from other repositories
- **Execution Tree**: The complete hierarchy of workflow executions across repositories triggered by a single `tako exec` command

## 1. Core Concepts & Architecture

### 1.1. Design Principles

- **Workflows**: Named sequences of steps triggered manually (`on: exec`) or automatically through event subscriptions
- **Artifacts**: Tangible, versionable outputs of repositories that serve as explicit links between upstream and downstream workflows
- **Centralized Orchestration**: All multi-repository workflows are initiated from the parent repository perspective using `tako exec --repo=<parent>`
- **Distributed Configuration**: Parent repositories define artifacts/events they publish; child repositories define subscription criteria
- **Event-Driven Architecture**: Clean separation between artifact publishing and consumption logic using structured events
- **State Persistence**: JSON-based state management for resumable execution across system restarts
- **Security**: Workflows run in unprivileged containers with comprehensive security hardening

### 1.2. Design Evolution

This design represents an evolution from earlier approaches. Key design decisions include:

- **Event-Driven over Implicit Discovery**: Rather than having the engine automatically discover `on: artifact_update` triggers, workflows now use explicit `tako/fan-out@v1` steps with event subscriptions. This provides better visibility and control over multi-repository orchestration.
- **Subscriptions over Dependents**: Child repositories declare their own subscription criteria instead of parent repositories maintaining `dependents` blocks. This distributes configuration responsibility and reduces coupling.
- **Parent-Led Execution**: All execution flows through the parent repository (using `--repo` flag) rather than allowing distributed initiation. This ensures consistent state management and centralized control.

These choices prioritize clarity, predictability, and debugging ease over fully distributed or declarative alternatives.

### 1.3. Core Execution Model

The workflow engine implements a **centralized orchestration with distributed subscriptions** pattern. The `tako/fan-out@v1` step is the cornerstone of this architecture, enabling workflows in parent repositories to trigger workflows in child repositories through structured events.

```
┌─────────────────┐    Event     ┌─────────────────┐
│ Parent Repo     │  ─────────►  │ Child Repo      │
│ Workflow with   │ (fan-out     │ Subscription    │
│ tako/fan-out@v1 │  step)       │ → Workflow      │
└─────────────────┘              └─────────────────┘
```

**Key Features:**
- Global dependency tree management from parent repositories
- Explicit fan-out steps that trigger subscribed children with deep synchronization (DFS traversal)
- Persistence and resume capabilities for long-running workflows
- Fine-grained execution context isolation between concurrent workflow trees
- Hierarchical resource management (global → per-repository → per-step)

## 2. Configuration Schema

### 2.1. Top-Level Structure

```yaml
version: 0.1.0
artifacts: { ... }
workflows: { ... }
subscriptions: { ... }  # For child repositories
```

### 2.2. Artifacts

Artifacts define the versionable outputs of a repository and serve as the foundation for multi-repository dependency management.

```yaml
artifacts:
  go-lib:
    path: ./go.mod
    ecosystem: go
```

### 2.3. Workflows

The workflow configuration varies depending on whether the repository serves as a parent (orchestrator) or child (subscriber) in the execution model.

#### 2.3.1. Workflows in Parent Repositories

Parent repositories orchestrate multi-repository workflows using explicit fan-out steps. These workflows emit events that other repositories can subscribe to:

```yaml
workflows:
  release:
    on: exec
    inputs:
      version-bump:
        description: "The type of version bump (major, minor, patch)"
        type: string
        default: "patch"
        validation:
          enum: [major, minor, patch]
    resources:
      cpu_limit: "1.0"
      mem_limit: "512Mi"
    steps:
      - id: build_library
        run: ./scripts/build.sh --bump {{ .inputs.version-bump }}
        produces:
          artifact: go-lib
          outputs:
            version: from_stdout
          events:
            - type: library_built
              schema_version: "1.2.0"
              payload:
                version: "{{ .outputs.version }}"
                commit_sha: "{{ .env.GITHUB_SHA }}"
                breaking_changes: "{{ .outputs.breaking_changes }}"
      
      - id: fan_out_to_dependents
        uses: tako/fan-out@v1
        with:
          event_type: library_built
          wait_for_children: true
          timeout: "2h"
      
      - id: create_final_release
        run: ./scripts/publish-release.sh
```

#### 2.3.2. Workflows in Child Repositories

Child repositories define subscription criteria that map events to local workflows. Note that these are regular workflows that can also be invoked directly (e.g., `tako exec integration_test --inputs.version=1.2.3`). The subscription model replaces the earlier `dependents` configuration pattern:

```yaml
subscriptions:
  - artifact: my-org/go-lib:go-lib
    events: [library_built]
    schema_version: "(1.1.0...2.0.0]"
    filters:
      - semver.major(event.payload.version) > 0 || semver.minor(event.payload.version) > 0
      - has(event.payload.commit_sha)
    workflow: integration_test
    inputs:
      version: "{{ .event.payload.version }}"
      commit_sha: "{{ .event.payload.commit_sha }}"
      has_breaking_changes: "{{ .event.payload.breaking_changes == 'true' }}"

workflows:
  integration_test:
    inputs:
      version:
        type: string
        required: true
      has_breaking_changes:
        type: boolean
        default: false
    steps:
      - uses: tako/update-dependency@v1
        with:
          name: go-lib
          version: "{{ .inputs.version }}"
      - id: run_tests
        run: ./scripts/integration-test.sh
        if: "!.inputs.has_breaking_changes"
      - id: run_breaking_change_tests
        run: ./scripts/breaking-change-test.sh
        if: .inputs.has_breaking_changes
        long_running: true
```

### 2.4. Step Configuration

Steps support comprehensive configuration for execution control, output capture, and failure handling:

```yaml
steps:
  - id: example_step
    if: .inputs.version-bump != "none"  # CEL expression
    run: ./scripts/process.sh --arg {{ .inputs.value | shell_quote }}
    image: "golang:1.22"  # Override workflow-level image
    long_running: false
    network: default  # Enable network access
    capabilities: [CAP_NET_ADMIN]  # Optional capabilities
    cache_key_files: "src/**/*.go"  # Files to include in cache key
    produces:
      artifact: my-artifact
      outputs:
        result: from_stdout
        status: from_file:./output/status.json
    on_failure:
      - id: cleanup_failure
        run: ./scripts/cleanup.sh
```

### 2.5. Security Configuration

```yaml
workflows:
  release:
    secrets:
      - GITHUB_TOKEN
      - NPM_TOKEN
    steps:
      - id: publish
        run: ./scripts/publish.sh
        env:
          GH_TOKEN: GITHUB_TOKEN
          NPM_TOKEN: NPM_TOKEN
```

## 3. Execution Model

### 3.1. Workflow Initiation

All multi-repository workflows are initiated from the parent repository perspective:

```bash
# Start a new release workflow
tako exec release --repo=my-org/go-lib --inputs.version-bump=minor

# Resume a paused workflow
tako exec --resume exec-20240726-143022-a7b3c1d2 --repo=my-org/go-lib
```

### 3.2. Execution Flow

1. **Discovery Phase**: Tako analyzes the entire dependency tree using existing `graph.BuildGraph()`, starting from the specified repository
2. **Schema Validation**: Validates that all child subscription schema versions are compatible with parent event schemas
3. **Parent Execution**: Runs parent workflow steps until hitting fan-out steps
4. **Subscription Evaluation**: For each repository in the dependency tree, evaluates subscription filters against emitted events using CEL expressions
5. **Child Triggering**: Triggers workflows in repositories where subscription criteria are met, executing in parallel with configurable concurrency limits
6. **Deep Waiting**: Fan-out steps wait for ALL triggered workflows to complete, including any workflows they trigger (DFS traversal)
7. **Persistence**: State is persisted after every step completion, with automatic backup creation for corruption recovery
8. **Resumption**: Resume from parent perspective, continuing from where execution left off across all repositories

### 3.3. State Management

- **Execution Tree State**: Complete multi-repository execution state saved to `~/.tako/state/<run-id>.json`
- **Checksum Validation**: State files include checksums to detect corruption
- **Automatic Backups**: Previous state files backed up as `<run-id>.json.bak` for recovery
- **Resume Capability**: `tako exec --resume <run-id>` continues from last successful checkpoint
- **Smart Partial Resume**: Failed branches can be resumed while preserving successful work

### 3.4. Workspace Management

- **Isolated Workspaces**: Each workflow run executes in `~/.tako/workspaces/<run-id>/`
- **Repository Contexts**: Steps execute in their repository's root directory
- **Copy-on-Write Overlays**: Workspace isolation between concurrent executions
- **Automatic Cleanup**: Workspaces cleaned up after successful completion, retained for debugging on failure

### 3.5. Resource Management

Hierarchical resource limits prevent resource exhaustion across the execution tree. This three-tier approach was chosen for several reasons:
- **Prevents Resource Exhaustion**: Global limits protect the orchestrating machine from being overwhelmed by large dependency trees
- **Fair Resource Sharing**: Multiple concurrent workflow trees don't starve each other
- **Granular Control**: Different repository sizes and build complexity can have appropriate resource allocation

**Configuration Hierarchy**:
- **Global Limits**: Protect the orchestrating machine (default: 32 cores, 128GB RAM, 2TB disk)
- **Repository Quotas**: Fair resource sharing between repositories (default: 8 cores, 16GB RAM, 100GB disk)  
- **Step Limits**: Individual step resource constraints (default: 2 cores, 4GB RAM, 20GB disk)

### 3.6. Concurrency Control

- **Repository Parallelism**: Repositories processed in parallel, limited by `--max-concurrent-repos` (default: 4)
- **Fine-Grained Locking**: Repository-level locks with deadlock detection using dependency-aware lock ordering
- **Execution Context Isolation**: Multiple workflows can run simultaneously on non-overlapping repository sets

## 4. Event System

### 4.1. Event Structure

Events provide structured communication between parent and child repositories. When a parent workflow produces an artifact, it can emit events containing metadata and payload data:

```yaml
produces:
  events:
    - type: library_built
      schema_version: "1.2.0"
      payload:
        version: "{{ .outputs.version }}"
        commit_sha: "{{ .env.GITHUB_SHA }}"
        breaking_changes: "{{ .outputs.breaking_changes }}"
```

### 4.2. Event Delivery Semantics

**At-Least-Once Delivery**: The system implements at-least-once delivery semantics with robust idempotency handling in child repositories. This design choice was made for several reasons:
- **Network Resilience**: Critical for distributed multi-repository operations where network partitions and transient failures are expected
- **Resume Compatibility**: At-least-once delivery naturally supports Tako's resume functionality through idempotent event processing
- **Operational Reliability**: Provides highest reliability guarantees while maintaining reasonable implementation complexity

**Event Namespacing**: Events are automatically namespaced by source repository (e.g., `my-org/go-lib/library_built`) to prevent naming conflicts across repositories.

**Schema Evolution**: The system supports schema evolution through:
- **Additive-Only Changes**: Only allow adding optional fields to event schemas
- **Field Deprecation**: Old fields can be deprecated with warnings before removal
- **Multi-Version Support**: For rare cases requiring breaking changes, multiple schema versions are supported
- **Fail-Fast Validation**: Schema compatibility is checked during the discovery phase

### 4.3. Subscription Model

Child repositories define subscription criteria using the `repo:artifact` format. This syntax was chosen for several reasons:
- **Consistency**: Maintains Tako's existing `owner/repo:branch` format patterns
- **Clarity**: Unambiguous artifact resolution across repositories
- **Collision Avoidance**: Different repositories can have same artifact names without conflicts

### 4.4. Diamond Dependency Resolution

The system handles "diamond dependency" scenarios where multiple parents trigger the same child:

```
    Lib A (updated)
   ╱         ╲
Lib B       Lib C (both depend on A)
   ╲         ╱
    App D (depends on both B and C)
```

**Resolution Strategy**: Each matching event triggers the subscribed workflow independently (multiple sequential runs). The first matching subscription in configuration order triggers; others are logged but ignored.

```yaml
subscriptions:
  - artifact: my-org/go-lib:go-lib  # repository:artifact format
    events: [library_built]
    schema_version: "(1.1.0...2.0.0]"  # Optional version range
    filters:
      - semver.major(event.payload.version) > 0  # CEL expression
    workflow: integration_test
```

## 5. Container Security

### 5.1. Security Hardening

All steps execute in hardened containers with comprehensive security measures:

- **Non-Root Execution**: Containers run as fixed UID 1001
- **Read-Only Root Filesystem**: Container root filesystem mounted read-only
- **Dropped Capabilities**: All Linux capabilities dropped by default, optional selective enablement
- **Seccomp Profile**: Default seccomp profile restricts available syscalls
- **Network Isolation**: No network access by default, optional per-step enablement

### 5.2. Secret Management

Secrets are never interpolated into templates or persisted to state files:

```yaml
workflows:
  release:
    secrets:
      - GITHUB_TOKEN
    steps:
      - id: publish
        env:
          GH_TOKEN: GITHUB_TOKEN  # Secure environment variable injection
```

### 5.3. Template Security

- **Command Injection Prevention**: Built-in template functions for safe shell argument escaping
- **Sandboxed Evaluation**: CEL expressions evaluated in sandboxed environment with resource limits
- **Information Disclosure Protection**: Secret values scrubbed from all logs and debug output

## 6. Built-in Steps

### 6.1. Core Steps

**`tako/checkout@v1`**: Repository checkout operations
```yaml
- uses: tako/checkout@v1
  with:
    ref: main  # branch, tag, or commit SHA
```

**`tako/update-dependency@v1`**: Automatic dependency updates with ecosystem detection
```yaml
- uses: tako/update-dependency@v1
  with:
    name: go-lib
    version: "1.2.3"
    npm_registry: "https://custom-registry.com"  # Optional
    update_lock_files: true  # Optional
```

**`tako/create-pull-request@v1`**: Automated pull request creation
```yaml
- uses: tako/create-pull-request@v1
  with:
    title: "Update dependency to {{ .inputs.version }}"
    body: "Automated dependency update"
    base: main
    head: feature/update-deps
```

**`tako/fan-out@v1`**: Multi-repository workflow orchestration
```yaml
- uses: tako/fan-out@v1
  with:
    event_type: library_built
    wait_for_children: true
    timeout: "2h"
```

**`tako/poll@v1`**: Long-running step monitoring
```yaml
- uses: tako/poll@v1
  with:
    target: step
    step_id: long_running_build
    timeout: 60m
    success_on_exit_code: 0
```

### 6.2. Step Versioning

Built-in steps use semantic versioning (@v1, @v2) for backward compatibility and controlled evolution.

## 7. Caching System

### 7.1. Cache Strategy

Content-addressable caching improves performance through intelligent cache key generation:

- **Cache Key Generation**: SHA256 hash of step definition + repository content hash
- **Selective File Inclusion**: `cache_key_files` glob pattern limits files included in hash
- **Cache Storage**: Cached results stored at `~/.tako/cache`
- **Cache Management**: `tako cache clean` command for manual cleanup

### 7.2. Cache Invalidation

- **Manual Invalidation**: `--no-cache` flag bypasses cache for specific runs
- **Automatic Invalidation**: Cache keys automatically invalidated when inputs change
- **Repository Optimization**: Shallow cloning, sparse checkout, and incremental updates for large repositories

## 8. Long-Running Operations

### 8.1. Asynchronous Execution

Steps marked as `long_running: true` support asynchronous execution:

- **Container Persistence**: Long-running containers persist after main process exit
- **Output Capture**: Results captured via `.tako/outputs.json` file in step workspace
- **System Reboot Recovery**: Container restart detection and recovery
- **Orphaned Container Cleanup**: Automatic cleanup after 24 hours without active workflow

### 8.2. Monitoring and Resume

- **Status Monitoring**: `tako status <run-id>` shows real-time execution state
- **Container Lifecycle**: Containers labeled with run ID and creation timestamps
- **Resource Monitoring**: Periodic monitoring with warnings at 90% resource utilization
- **Graceful Resumption**: Resume operations detect completed long-running steps automatically

## 9. Error Handling and Recovery

### 9.1. Failure Policies

- **Fail-Fast**: Strict fail-fast policy with immediate workflow termination on step failure
- **Compensation**: `on_failure` blocks provide rollback and cleanup capabilities
- **Smart Partial Resume**: Resume only failed branches while preserving successful work
- **Automatic Retry**: Exponential backoff retry for transient failures (network issues, API rate limits)

### 9.2. Error Message Quality

Error messages follow a structured format for clarity and actionability:

```
Error: Failed to execute step 'get_version' in workflow 'release'.
Reason: Input validation failed for 'version-bump'.
Details: Expected one of [major, minor, patch], but got 'invalid'.
```

## 10. Debugging and Observability

### 10.1. Debug Capabilities

- **Interactive Debug Mode**: `--debug` flag enables step-by-step execution with user confirmation
- **State Inspection**: `tako state inspect <run-id>` displays complete workflow state
- **Execution Visualization**: Dependency graph visualization for complex workflows
- **Template Resolution**: Debug mode shows resolved template variables before execution

### 10.2. Troubleshooting Multi-Repository Workflows

Debugging distributed, asynchronous workflows across multiple repositories requires specialized tooling:

**Inspecting Child Workflow State**:
```bash
# View execution tree status across all repositories
tako status exec-20240726-143022-a7b3c1d2

# Inspect specific repository's workflow state
tako status exec-20240726-143022-a7b3c1d2 --repo=my-org/app-one
```

**Event Payload Investigation**:
```bash
# View event payloads sent and received
tako state inspect exec-20240726-143022-a7b3c1d2 --show-events

# Debug subscription filter evaluation
tako exec --dry-run release --debug-subscriptions
```

**Manual Re-triggering**:
```bash
# Resume only failed workflows in child repositories without re-running parent
tako exec --resume exec-20240726-143022-a7b3c1d2 --failed-only

# Manually trigger specific workflow with custom event payload
tako exec integration_test --repo=my-org/app-one --simulate-event=library_built
```

**Circular Dependency Detection**:
The system detects circular dependencies in fan-out chains (e.g., Parent → Child → Parent) during the discovery phase and fails with a clear error listing the repositories in the cycle.

### 10.3. Status and Monitoring

```bash
# Real-time status monitoring
tako status exec-20240726-143022-a7b3c1d2

# Output shows execution tree across all repositories
├── my-org/go-lib [COMPLETED]
├── my-org/app-one [RUNNING] - step: run_tests (2m30s)
└── my-org/app-two [PENDING] - waiting for: app-one
```

### 10.4. Performance Optimization

- **Template Caching**: Parsed templates cached in-memory with LRU eviction (100MB limit)
- **Repository Optimization**: Shallow cloning and sparse checkout for large repositories
- **Lazy Evaluation**: Subscription filters evaluated only for repositories in dependency tree (O(dependency_tree_size) vs O(all_repositories))
- **Resource Monitoring**: Peak resource usage reporting for optimization

**Performance Design Decisions**:
- **Timestamp-Based Run IDs**: Use format `exec-YYYYMMDD-HHMMSS-<8-char-hash>` for human readability and natural chronological ordering
- **State Polling**: Parent polls children periodically (30s base, exponential backoff to 5m max) rather than push-based callbacks for network resilience
- **State File Limits**: Warn at 10MB, fail at 100MB to prevent resource exhaustion

## 11. CLI Interface

### 11.1. Primary Commands

```bash
# Execute workflows
tako exec <workflow-name> [flags]
tako exec --resume <run-id> [flags]

# Status and management
tako status <run-id>
tako steps list

# Cache and workspace management
tako cache clean [--older-than <duration>]
tako workspace clean [--older-than <duration>]

# Validation and testing
tako exec --dry-run <workflow-name>
tako lint  # Validate tako.yml syntax and semantics
```

### 11.2. Global Flags

| Flag | Description | Default |
|------|-------------|---------|
| `--max-concurrent-repos` | Maximum repositories to process in parallel | `4` |
| `--no-cache` | Invalidate cache and execute all steps | `false` |
| `--debug` | Enable interactive step-by-step execution | `false` |
| `--dry-run` | Show execution plan without changes | `false` |
| `--inputs.<name>=<value>` | Pass input variables to workflow | |

## 12. Authentication and Security

### 12.1. Credential Management

Hierarchical credential delegation with repository-scoped permissions:

- **OAuth Delegation**: Repository-scoped OAuth tokens for API access
- **Git Credential Manager Integration**: Leverages existing Git tooling
- **Enterprise Support**: RBAC integration and audit capabilities
- **Credential Isolation**: Execution-isolated credential contexts prevent abuse

### 12.2. Network and Execution Isolation

- **Network Partitions**: Fail-fast behavior during network issues with clear error messages
- **Execution Context Isolation**: Fine-grained locking prevents conflicting executions
- **Workspace Isolation**: Copy-on-write overlays ensure execution independence

## 13. Scalability and Performance

### 13.1. Performance Considerations

- **Dependency Graph Size**: Warning issued for dependency graphs exceeding 50 repositories
- **Memory Usage**: State file warnings at 10MB, failures at 100MB to prevent resource exhaustion
- **Template Cache**: LRU eviction when cache exceeds 100MB
- **Execution Tree Depth**: Configurable depth limit (default: 10 levels) with warnings at threshold

### 13.2. Optimization Features

- **Repository Caching**: Incremental updates rather than full re-cloning
- **Parallel Execution**: Repository-level parallelism with configurable concurrency
- **Smart Caching**: Content-addressable caching with selective file inclusion
- **Resource Pools**: Hierarchical resource management prevents resource exhaustion

## 14. Future Extensibility

### 14.1. Plugin Architecture

While not included in the initial release, the built-in step system is designed for future extensibility:

- **Common Step Interface**: All built-in steps implement a standardized Go interface
- **Self-Contained Logic**: Minimal dependencies on core engine for easier plugin development
- **Multiple Distribution Models**: Compiled plugins, interpreted scripts, or remote step registries

### 14.2. Advanced Features

Planned enhancements for future releases:

- **Workflow Composition**: Import statements for workflow libraries and reuse patterns
- **Distributed Execution**: Architecture preparation for distributed execution capabilities
- **Integration Webhooks**: Outbound webhook support for external system integration
- **Advanced Recovery**: Deeper state validation and manual state inspection tools

## Open Questions

The following areas require further exploration and decision-making:

1. **Plugin Security Model**: How should custom steps be sandboxed and secured?
2. **Workflow Composition Syntax**: What should the import statement format look like for workflow libraries?
3. **Distributed Architecture**: How should the transition from single-machine to distributed execution be architected?
4. **Integration Ecosystem**: Which external systems should have first-class integration support?
5. **Performance Scaling**: At what dependency graph sizes should distributed execution be recommended?
6. **Enterprise Features**: What additional audit, compliance, and governance features are needed?

## 15. Design Decisions and Rationale

This section documents key design decisions made during the evaluation process, including the rationale for chosen approaches and alternatives that were considered but rejected.

### 15.1. Workflow ID Generation

**Decision**: Timestamp-based with collision avoidance  
**Format**: `exec-YYYYMMDD-HHMMSS-<8-char-hash>` (e.g., `exec-20240726-143022-a7b3c1d2`)

**Rationale**: Timestamp-based IDs provide several critical benefits for Tako's distributed multi-repository orchestration:
- **Human readability**: Immediate temporal context aids debugging complex execution trees
- **Natural ordering**: Workflows sort chronologically for easier management and cleanup
- **Distribution-friendly**: No coordination required between Tako instances
- **Debugging support**: Time context correlates with commits, incidents, and logs
- **Collision resistance**: 8-character hash from hostname+PID+random+microseconds prevents conflicts
- **Resume experience**: Users can easily identify relevant workflows to resume

**Alternatives considered**: UUIDs were considered but lack human readability and temporal context.

### 15.2. Subscription Syntax

**Decision**: `repo:artifact` format  
**Example**: `my-org/go-lib:go-lib` (repository:artifact)

**Rationale**: This format was chosen for several key reasons:
- **Consistency**: Maintains Tako's existing `owner/repo:branch` format patterns
- **Clarity**: Unambiguous artifact resolution across repositories
- **Familiar**: Developers already understand `repo:branch` syntax from Git
- **Collision avoidance**: Different repositories can have same artifact names without conflicts
- **Tooling friendly**: Easy to parse and validate programmatically
- **Future-proof**: Can extend to `repo:branch:artifact` if needed

**Alternatives considered**: Hierarchical paths and simple names were considered but lacked clarity or collision resistance.

### 15.3. State Consistency Management

**Decision**: Parent polls children periodically with smart intervals  
**Implementation**: 30s base, exponential backoff to 5m max during network issues, 30m timeout threshold

**Rationale**: Polling was chosen over push-based callbacks for operational reliability:
- **Operational simplicity**: Consistent with Tako's existing long-running step polling patterns
- **Network resilience**: Continues working through partitions, no lost state updates
- **Fault tolerance**: Gradual degradation vs catastrophic failure on network issues
- **Infrastructure minimal**: No webhook endpoints or complex callback handling needed
- **Debuggable**: Clear polling logs provide audit trail of state changes
- **Consistent control**: Maintains centralized decision-making model

**Alternatives considered**: Push-based callbacks would provide real-time updates but introduce complexity and failure modes.

### 15.4. Subscription Evaluation Timing

**Decision**: Lazy evaluation only for repositories that could be triggered  
**Implementation**: Evaluate CEL expressions only for repositories in the discovered dependency tree when events are published

**Rationale**: This approach balances performance with complexity:
- **Performance efficiency**: Only evaluates filters for repositories that could actually be triggered
- **Memory efficiency**: Avoids pre-computing evaluations for irrelevant repositories
- **Complexity balance**: Simpler than cache invalidation logic while still being efficient
- **Architecture fit**: Leverages existing `graph.BuildGraph()` discovery phase naturally
- **Scalability**: O(dependency_tree_size) instead of O(all_repositories)

**Alternatives considered**: Cached evaluation provides better performance but adds complexity; eager evaluation is simpler but inefficient.

### 15.5. Event Schema Evolution

**Decision**: Additive-only changes with field deprecation warnings, with multi-version fallback  
**Primary strategy**: Only allow adding optional fields, deprecate old fields with warnings

**Rationale**: This approach balances operational simplicity with evolution needs:
- **Operational simplicity**: Additive-only changes avoid complex deployment coordination across repositories
- **Gradual rollouts**: New fields added first, consumers updated incrementally on their timeline
- **Graceful degradation**: CEL `has()` function handles missing fields safely
- **Clear migration path**: Deprecation warnings guide users toward new fields before removal
- **Breaking change fallback**: Multi-version support for rare cases requiring true breaking changes
- **Fail-fast validation**: Schema compatibility checked during discovery phase

**Alternatives considered**: Multi-version support was considered primary but adds operational complexity.

### 15.6. Execution Tree Depth Limits

**Decision**: Configurable depth limit with warning at threshold  
**Configuration**: Default limit of 10 levels, warning at 7 levels, configurable per-workflow and globally

**Rationale**: This approach balances flexibility with safety:
- **Balances flexibility and safety**: Allows legitimate deep chains while preventing runaway executions
- **Fits Tako's design philosophy**: Consistent with existing configurable limits (concurrency, timeouts)
- **Operational practicality**: Most real dependency chains rarely exceed 7-10 levels deep
- **Early warning system**: Threshold provides feedback to encourage better architectural practices
- **Implementation simplicity**: Easy to add to existing `buildGraphRecursive()` function
- **Enterprise readiness**: Helps with compliance, audit trails, and resource planning

**Alternatives considered**: No limits (risk of runaway execution) and fixed limits (inflexible for different use cases).

### 15.7. Partial Failure Recovery

**Decision**: Smart Partial Resume - Resume only failed branches while preserving successful work  
**Implementation**: Automatic retry for transient failures with exponential backoff, partial resume when retry limits reached

**Rationale**: This combines efficiency with intelligent automation:
- **Resource efficiency**: Preserves expensive multi-repository work that succeeded, critical for build/test/deployment operations
- **Operational simplicity**: Builds on existing `--resume` command pattern, no complex user decisions required
- **Intelligent automation**: Automatic retry with exponential backoff for transient failures (network issues, API rate limits)
- **Persistent failure handling**: Falls back to partial resume when automatic retry reaches limits
- **Architecture consistency**: Extends existing execution tree state management and centralized control model
- **Debugging support**: Failed branches retain workspace state, clear logging of success/failure status per branch

**Alternatives considered**: Full restart (wastes work) and manual intervention only (requires user expertise).

### 15.8. Cross-Repository Resource Limits

**Decision**: Hierarchical limits (global → per-repo → per-step)  
**Implementation**: Three-tier resource management with global pools, per-repository quotas, and per-step limits

**Rationale**: This multi-tier approach provides comprehensive resource control:
- **Consistency with existing design**: Aligns perfectly with Tako's configurable limits philosophy
- **Prevents resource exhaustion**: Global limits protect the orchestrating machine from being overwhelmed
- **Fair resource sharing**: Multiple concurrent workflow trees don't starve each other of resources
- **Granular control**: Repository-level quotas allow tuning for different repository sizes and build complexity
- **Step-level safety**: Individual step limits prevent runaway processes within repositories
- **Operational flexibility**: Different execution contexts (CI vs local development) can have different resource profiles

**Alternatives considered**: Global-only limits (not granular enough) and per-step-only limits (no global protection).

### 15.9. Event Delivery Guarantees

**Decision**: At-least-once delivery with idempotency handling in children  
**Implementation**: Robust idempotency handling in child repository event processors

**Rationale**: This provides the highest reliability guarantees:
- **Consistency with Tako's design philosophy**: Aligns perfectly with the "Persist Every Step" approach
- **Network resilience**: Critical for distributed multi-repository operations where network partitions are expected
- **Resume compatibility**: At-least-once delivery naturally supports Tako's resume functionality
- **Operational reliability**: Provides highest reliability guarantees while maintaining reasonable implementation complexity
- **State persistence integration**: Leverages existing state tracking mechanisms for workflow resume capabilities
- **Debugging support**: Event redelivery provides clear audit trails for troubleshooting

**Alternatives considered**: Exactly-once (complex to implement correctly) and at-most-once (risks data loss).

### 15.10. Execution Context Isolation

**Decision**: Fine-grained locking per repository with deadlock detection  
**Implementation**: Hierarchical fine-grained locking with dependency-aware lock ordering and workspace isolation

**Rationale**: This maximizes concurrency while ensuring safety:
- **Performance optimization**: Multiple workflows can run simultaneously on non-overlapping repository sets
- **Data consistency**: Repository-level locks with read/write semantics ensure workspace isolation
- **Deadlock prevention**: Dependency-aware lock ordering using topological sort with timeout-based acquisition
- **Tako integration**: Builds naturally on existing graph building, repository caching, and state persistence
- **Workspace isolation**: Extends existing repository caching structure with execution-specific overlays
- **Resume compatibility**: Lock acquisition state persisted with execution tree for Smart Partial Resume operations

**Alternatives considered**: Global locking (poor concurrency) and no locking (data races).

### 15.11. Why Event-Driven Over Implicit Discovery

**Decision**: Explicit `tako/fan-out@v1` steps with event subscriptions instead of automatic `on: artifact_update` triggers

**Rationale**: Explicit fan-out provides better operational characteristics:
- **Visibility**: Clear indication in parent workflows of when multi-repository orchestration occurs
- **Control**: Parents can decide when and how to trigger dependent repositories
- **Debugging**: Easier to trace execution flow and understand why certain repositories were triggered
- **Flexibility**: Parents can customize event payloads and trigger conditions
- **Testability**: Fan-out steps can be tested in isolation with dry-run mode

**Alternatives rejected**: Implicit discovery based on artifact dependencies was simpler but lacked visibility and control.

### 15.12. Why Subscriptions Over Dependents

**Decision**: Child repositories declare subscription criteria instead of parents maintaining `dependents` blocks

**Rationale**: Distributed configuration responsibility reduces coupling:
- **Reduced coupling**: Children decide their own trigger criteria without parent coordination
- **Autonomy**: Child repositories can evolve their subscription logic independently
- **Clarity**: Subscription criteria are co-located with the workflows they trigger
- **Maintainability**: Parents don't need to maintain knowledge of all downstream consumers
- **Scalability**: Distribution scales better than centralized dependency management

**Alternatives rejected**: Parent-maintained dependents blocks create tight coupling and maintenance burden.

## 16. Implementation Plan

### 16.1. Milestone Dependencies

The implementation follows a structured approach with clear prerequisite relationships:

**Milestone 1: MVP - Local, Synchronous Execution**
- Issue 1: Workflow schema support (foundation)
- Issue 2: `tako exec` command → depends on Issue 1
- Issue 3: Synchronous local runner → depends on Issue 2  
- Issue 4: Step output passing → depends on Issue 3
- Issue 5: Single-repo E2E tests → depends on Issues 1-4

**Milestone 2: Containerization and Graph-Aware Execution**
- Issue 6: Containerized execution → depends on Issue 3
- Issue 7: Graph-aware execution → depends on Issues 3-4
- Issue 8: Built-in steps framework → depends on Issue 3
- Issue 9: Multi-repo E2E tests → depends on Issues 6-8

**Milestone 3: Advanced Features**
- Issue 13: Long-running steps → depends on Issues 3, 6
- Issue 15: Status command → depends on Issue 13
- Issues 10-12, 14: Additional features → depends on earlier milestones

### 16.2. Assessment of Existing Issues

**Current Issues #93-#97**: These existing issues are superseded by the comprehensive design and should be closed in favor of the new implementation plan, which provides significantly more detail and covers the full workflow engine scope.

### 16.3. Backward Compatibility Strategy

- **Schema Versioning**: The engine implements versioned built-in steps (e.g., `tako/checkout@v1`, `tako/checkout@v2`)
- **Deprecation Process**: Clear deprecation warnings for step versions before removal
- **Additive Schema Changes**: Schema extensions follow additive patterns to maintain compatibility

This design provides a comprehensive foundation for the Tako workflow engine while maintaining flexibility for future enhancements and ecosystem growth.