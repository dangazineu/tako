# Design Evaluation: Workflow Triggering Mechanisms

This document evaluates different approaches for triggering workflows in a multi-repository environment within `tako`.

## Model 0: Centralized Orchestration with Distributed Subscriptions (Recommended)

This model combines centralized orchestration control with distributed subscription criteria, providing the best of both worlds: 
global workflow coordination from the parent perspective while allowing children to define their own triggering logic.

### Core Principles

1. **Centralized Execution**: All multi-repository workflows are initiated from the parent repository using `tako exec --repo=parent` (or without the --repo flag, if running from a clone of the parent's repo)
2. **Distributed Configuration**: Parent defines artifacts/events it publishes; children define subscription criteria for when they should respond
3. **Global Dependency Tree**: Parent execution discovers and manages the entire dependency tree across all repositories
4. **Explicit Fan-out**: Parent workflows contain explicit fan-out steps that trigger subscribed children  
5. **Deep Synchronization**: Fan-out steps wait for all children AND their children to complete (DFS traversal)
6. **Persistence & Resume**: Long-running workflows persist state and can be resumed from parent perspective

### Configuration Model

**Parent Repository (`go-lib/tako.yml`)**: Defines artifacts, events, and orchestrates fan-out
```yaml
artifacts:
  go-lib:
    path: ./go.mod
    
workflows:
  release:
    inputs:
      version-bump:
        type: string
        default: "patch"
    steps:
      - id: build_library
        run: ./scripts/build.sh --bump {{ .inputs.version-bump }}
        produces:
          artifact: go-lib
          outputs:
            version: from_stdout
          events:
            - type: library_built
              schema_version: "1.2.0"  # Optional, defaults to "1.0.0"
              payload:
                version: "{{ .outputs.version }}"
                commit_sha: "{{ .env.GITHUB_SHA }}"
                breaking_changes: "{{ .outputs.breaking_changes }}"
                
      - id: fan_out_to_dependents
        uses: tako/fan-out@v1
        with:
          # Triggers all repositories with matching subscriptions in parallel
          # respecting global concurrency limits from ~/.tako config
          event_type: library_built
          # This step waits for ALL triggered workflows to complete,
          # including any child workflows they trigger (DFS)
          wait_for_children: true
          timeout: "2h"
          
      - id: create_final_release
        run: ./scripts/publish-release.sh
        # This only runs after ALL dependents (and their dependents) complete
```

**Child Repository (`app-one/tako.yml`)**: Defines subscription criteria and workflows
```yaml
subscriptions:
  - artifact: my-org/go-lib:go-lib
    events: [library_built]
    schema_version: "(1.1.0...2.0.0]"  # Optional, defaults to unbounded
    # CEL expressions to determine if this repo should be triggered
    filters:
      # Only trigger for non-patch releases
      - semver.major(event.payload.version) > 0 || semver.minor(event.payload.version) > 0
      # Only if we're on a supported branch  
      - has(event.payload.commit_sha)
    # Which workflow to trigger when criteria are met
    workflow: integration_test
    # Map event payload to workflow inputs
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
      commit_sha:
        type: string
        required: true
      has_breaking_changes:
        type: boolean
        default: false
    steps:
      - uses: tako/checkout@v1
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
        long_running: true  # This step might take 30+ minutes
        
      # This child can also fan out to its own children
      - id: notify_my_dependents
        uses: tako/fan-out@v1
        with:
          event_type: integration_complete
          wait_for_children: true
        produces:
          events:
            - type: integration_complete
              payload:
                upstream_version: "{{ .inputs.version }}"
                test_results: "{{ .steps.run_tests.outputs.results }}"
```

### Execution Model

**Initiation**: Always from parent repository perspective
```bash
# Start a new release workflow
tako exec release --repo=my-org/go-lib --inputs.version-bump=minor

# Resume a paused workflow (works regardless of where it's waiting)
tako exec --resume 123 --repo=my-org/go-lib
```

**Execution Flow**:
1. **Discovery Phase**: Tako analyzes the entire dependency tree using existing `graph.BuildGraph()`, starting from the specified repository (ignoring any subscriptions in the root repo)
2. **Schema Validation**: Validates that all child subscription schema versions are compatible with parent event schemas, exiting with error if incompatible
3. **Parent Execution**: Runs parent workflow steps until hitting `fan_out_to_dependents` 
4. **Subscription Evaluation**: For each repository in the dependency tree, evaluates their subscription filters against the emitted event (using namespaced event names like `my-org/go-lib/library_built`)
5. **Child Triggering**: Triggers workflows in all repositories where subscription criteria are met, executing in parallel respecting global concurrency limits
6. **Multi-Parent Handling**: Each matching event from each parent triggers the subscribed workflow independently (multiple runs if multiple parents emit matching events)
7. **Deep Waiting**: The fan-out step waits for ALL triggered workflows to complete, including any workflows they trigger (DFS traversal)
8. **Persistence**: If all steps actively running are `long_running`, the entire execution tree state is persisted and Tako exits
9. **Resumption**: Resume from parent perspective with `tako exec --resume <id> --repo=<parent>`, continuing from where execution left off across all repositories

**State Management**:
- Execution tree-level state persistence (entire multi-repo execution state saved)
- Resume commands always run against parent repo, regardless of where work is actually happening
- Tako tracks the complete execution tree and can resume any paused workflows
- Multiple subscriptions in same repo: first matching subscription triggers, others are logged but not executed

### Key Design Decisions

- **Repository Discovery**: Uses existing `graph.BuildGraph()` functionality, repositories cached under `$cacheDir/repos`
- **Event Namespacing**: Events automatically namespaced by source repo (e.g., `my-org/go-lib/library_built`)
- **Schema Versioning**: Optional schema versions (default "1.0.0"), optional version ranges in subscriptions (default unbounded)
- **Concurrency**: Fan-out executes children in parallel respecting global concurrency limits from `~/.tako` config
- **Event Payload**: No size limits, simple filter evaluation for all events
- **Multi-Parent Events**: Each matching event triggers workflow independently (multiple sequential runs)
- **Subscription Priority**: First matching subscription in config order triggers, others logged but ignored

### Monitoring & Observability

**Status Monitoring**:
```bash
# Quick snapshot of current execution state (reads from persisted state)
tako status exec-123

# Live execution with status updates (waits for completion or long-running persistence)
tako exec release --repo=my-org/go-lib

# Resume from where left off (advances any completed long-running steps)
tako exec --resume 123 --repo=my-org/go-lib
```

**Status Output Format**:
```
# Tree view showing current execution state across all repositories
├── my-org/go-lib [COMPLETED]
├── my-org/app-one [RUNNING] - step: run_tests (2m30s)
└── my-org/app-two [PENDING] - waiting for: app-one
```

### Pros

- **Centralized Control**: Parent maintains full visibility and control over entire workflow
- **Distributed Logic**: Children define their own subscription criteria without coupling to parent
- **Global Orchestration**: Single command manages complex multi-repository workflows
- **Deep Synchronization**: Fan-out steps naturally handle transitive dependencies
- **Resumable**: Long-running workflows can be paused and resumed from parent perspective
- **Flexible Filtering**: Rich subscription criteria using CEL expressions
- **Event-Driven**: Clean separation between artifact publishing and consumption logic
- **Leverages Existing Code**: Builds on current Tako dependency discovery and caching systems

### Cons

- **Complexity**: Requires sophisticated orchestration engine with global state management
- **Single Point of Control**: All execution must flow through parent repository
- **Resource Requirements**: Parent process must coordinate potentially hundreds of repositories
- **Debugging Challenges**: Complex execution trees may be difficult to debug
- **State Persistence**: Requires robust state management and recovery mechanisms

## Outstanding Questions

### My Questions

**Q25: Workflow ID Generation - How should execution IDs be generated and managed?**

When running `tako exec release --repo=my-org/go-lib`, how should the workflow ID (e.g., `exec-123`) be generated?
- **Option A**: Sequential numbering (exec-1, exec-2, exec-3)
- **Option B**: Timestamp-based (exec-20240726-143022)
- **Option C**: UUID-based (exec-a1b2c3d4-e5f6-7890)
- **Option D**: Content-addressable based on inputs and repo state

**Q26: Subscription Syntax - How should artifact references work in subscriptions?**

In the current examples, subscriptions reference artifacts as `artifact: my-org/go-lib:go-lib`. Should this syntax be:
- **Option A**: `repo:artifact` format as shown (e.g., `my-org/go-lib:go-lib`)
- **Option B**: Full artifact path (e.g., `my-org/go-lib/artifacts/go-lib`)
- **Option C**: Just artifact name with implicit parent resolution (e.g., `go-lib`)
- **Option D**: Something else?

### Gemini's Questions

**Q27: State Consistency - How should the system handle distributed state synchronization during execution?**

In the centralized orchestration model, the parent maintains global execution state while children execute workflows. How should state consistency be maintained when child workflows fail or succeed?
- **Option A**: Parent polls children periodically and updates master state
- **Option B**: Children push state updates to parent via callbacks/webhooks  
- **Option C**: Eventual consistency model with reconciliation at completion
- **Option D**: Pessimistic locking with distributed coordination

**Q28: Subscription Evaluation Timing - When should subscription filters be evaluated during execution?**

With CEL expressions and complex filters, subscription evaluation could be expensive. When should this evaluation occur?
- **Option A**: Pre-execution during dependency discovery phase (static evaluation)
- **Option B**: Just-in-time when each event is published (dynamic evaluation)
- **Option C**: Cached evaluation with invalidation on configuration changes
- **Option D**: Lazy evaluation only for repositories that could be triggered

**Q29: Event Schema Evolution - How should breaking changes to event schemas be handled?**

When a parent repository changes its event schema (adds/removes/modifies payload fields), how should backward compatibility be maintained?
- **Option A**: Strict semver for event schemas with breaking change detection
- **Option B**: Additive-only changes with field deprecation warnings
- **Option C**: Schema migration tools with automatic conversion
- **Option D**: Multiple schema versions supported simultaneously

**Q30: Execution Tree Depth Limits - Should there be limits on workflow execution tree depth?**

The design mentions DFS traversal for deep synchronization, but deeply nested dependencies could cause performance or resource issues.
- **Option A**: No limits, trust users to design reasonable dependency chains
- **Option B**: Configurable depth limit with warning at threshold
- **Option C**: Hard limit with graceful degradation (breadth-first instead of depth-first)
- **Option D**: Adaptive limits based on system resources and execution history

**Q31: Partial Failure Recovery - How should the system handle partial execution tree failures?**

When some children succeed and others fail in a fan-out scenario, how should resumption work?
- **Option A**: Resume only failed branches, skip successful ones
- **Option B**: Restart entire execution tree from last consistent checkpoint
- **Option C**: User choice between partial resume or full restart
- **Option D**: Automatic retry with exponential backoff for failed branches only

**Q32: Cross-Repository Resource Limits - How should resource consumption be managed across the entire execution tree?**

The design mentions global concurrency limits, but what about CPU, memory, and disk usage across all executing repositories?
- **Option A**: Repository-level limits only, no global coordination
- **Option B**: Global resource pool shared across all executing workflows
- **Option C**: Hierarchical limits (global -> per-repo -> per-step)
- **Option D**: Dynamic resource allocation based on execution priority

**Q33: Event Delivery Guarantees - What delivery semantics should events have in the distributed system?**

When the parent publishes events to children, what guarantees should be provided?
- **Option A**: At-least-once delivery with idempotency handling in children
- **Option B**: Exactly-once delivery with distributed transaction semantics
- **Option C**: At-most-once delivery with retry logic for failures
- **Option D**: Best-effort delivery with eventual consistency reconciliation

**Q34: Repository Authentication Propagation - How should authentication credentials be handled across repositories?**

When executing across multiple repositories that may require different credentials, how should authentication be managed?
- **Option A**: Parent credentials used for all repositories (single identity)
- **Option B**: Per-repository credential configuration in parent
- **Option C**: Children authenticate independently using local credentials
- **Option D**: Credential delegation with scoped permissions per repository

**Q35: Execution Context Isolation - How should execution contexts be isolated between concurrent workflow trees?**

When multiple `tako exec` commands run concurrently with overlapping repository sets, how should isolation be maintained?
- **Option A**: Lock entire repository set during execution (strict isolation)
- **Option B**: Fine-grained locking per repository with deadlock detection
- **Option C**: Copy-on-write workspaces with eventual merge
- **Option D**: Execution queuing with repository-level serialization

**Q36: Network Partition Handling - How should the system behave during network partitions between parent and children?**

In distributed environments, network partitions between parent and child repositories could occur.
- **Option A**: Fail fast on network issues, require manual intervention
- **Option B**: Continue execution with last known state, reconcile on reconnection
- **Option C**: Timeout-based failure detection with automatic retry
- **Option D**: Split-brain prevention with distributed consensus algorithms

**Q37: Multi-Parent Conflict Resolution - How should conflicting events from multiple parents be resolved?**

When a child repository subscribes to events from multiple parents that could trigger simultaneously, how should conflicts be handled?
- **Option A**: Execute workflows sequentially in parent precedence order
- **Option B**: Merge/aggregate conflicting events into single workflow execution
- **Option C**: Execute workflows in parallel with separate execution contexts
- **Option D**: User-defined conflict resolution strategies per subscription

**Q38: Workspace Sharing Strategy - How should workspace data be shared between parent and child executions?**

In the centralized model, workspaces are created per execution, but artifacts need to flow between repositories.
- **Option A**: Copy artifacts between separate workspaces for each repository
- **Option B**: Shared workspace mounted across all repositories in execution tree
- **Option C**: Artifact registry pattern with push/pull semantics
- **Option D**: Content-addressable storage with workspace references
