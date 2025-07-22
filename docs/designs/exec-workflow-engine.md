# Design: The `tako exec` Workflow Engine v0.2.0

This document provides the complete technical design for the `tako exec` workflow engine. It is a **breaking change** from the v0.1.0 schema.

## 1. Core Concepts & Principles

-   **Workflows**: A named sequence of steps. Triggered manually (`on: exec`) or automatically (`on: artifact_update`).
-   **Artifacts**: The tangible, versionable outputs of a repository. They are the explicit link between upstream and downstream workflows.
-   **State**: A JSON object capturing step outputs, persisted locally for resumption.
-   **Security**: Workflows run in unprivileged containers; secrets are managed via environment variables and are never persisted.
-   **Clarity & Precision**: The schema and execution model are designed to be unambiguous and directly implementable.

## 2. `tako.yml` Schema Reference (`v0.2.0`)

### 2.1. Top-Level Structure

```yaml
version: 0.2.0
artifacts: { ... }
workflows: { ... }
dependents: { ... }
```

### 2.2. `artifacts`

Defines the outputs of a repository.

```yaml
artifacts:
  # The key 'tako-lib' is the artifact's unique name within this repo.
  tako-lib:
    # Path to the artifact's manifest, used for dependency analysis.
    path: ./go.mod
    # The ecosystem, used to select the correct tooling.
    ecosystem: go
```

### 2.3. `workflows`

Defines the executable processes.

```yaml
workflows:
  release:
    # This workflow is triggered manually by `tako exec release`.
    on: exec
    # Default container image for all steps unless overridden.
    image: "golang:1.21"
    # Defines inputs passed from the CLI.
    inputs:
      version-bump:
        description: "The type of version bump (major, minor, patch)."
        type: string # Supported types: string, boolean, number
        default: "patch"
        required: false
    # Defines resource limits for the main workflow container.
    resources:
      cpu_limit: "1.0" # 1 full CPU core
      mem_limit: "512Mi" # 512 Megabytes
    steps:
      - id: get_version
        run: ./scripts/get-version.sh --bump {{ .inputs.version-bump }}
        # This step's output is explicitly associated with the 'tako-lib' artifact.
        produces:
          artifact: tako-lib
          outputs:
            version: from_stdout

  downstream-test:
    # This workflow is triggered automatically by an update to an artifact
    # this repository depends on.
    on: artifact_update
    # A CEL expression to filter triggers. This workflow only runs if the
    # triggering artifact was 'tako-lib'.
    if: trigger.artifact.name == 'tako-lib'
    steps:
      - uses: tako/checkout@v1
      - uses: tako/update-dependency@v1
        with:
          # The trigger context is populated by the engine with the
          # outputs from the upstream 'produces' block.
          version: "{{ .trigger.artifact.outputs.version }}"
```

### 2.4. `dependents`

Explicitly declares dependencies on artifacts from other repositories.

```yaml
dependents:
  - repo: my-org/downstream-app
    # This repo depends on the 'tako-lib' artifact from this upstream repo.
    artifacts: [tako-lib]
```

## 3. Workflow Execution Model

1.  **Plan Generation**:
    - When `tako exec release` is run, the engine executes the `release` workflow.
    - The `get_version` step runs and, because of its `produces` block, the engine associates its output (`version`) with the `tako-lib` artifact.
    - The engine then traverses the dependency graph. It finds downstream repos that depend on `tako-lib` and have workflows with `on: artifact_update`.
    - For each potential downstream workflow, it evaluates the `if` condition using the **Common Expression Language (CEL)**. If the expression evaluates to true, the workflow is added to the execution plan.

2.  **Execution**:
    - Repositories are processed in parallel, limited by `--max-concurrent-repos` (default: 4).
    - Each workflow runs in a container with resource limits specified in the `resources` block or via CLI flags (`--cpu-limit`, `--mem-limit`), which override the schema.
    - The workspace (`~/.tako/workspaces/<run-id>/...`) is mounted into the container.

3.  **State & Resumption**:
    - State is saved to `~/.tako/state/<run-id>.json` after each step. The file is checksummed to detect corruption. If corrupt, the run fails.
    - To resume, a user runs `tako exec --resume <run-id>`.

## 4. Security

-   **Secret Scrubbing**: `tako` will maintain a list of secret names from the environment. It will perform a best-effort scrub of these exact string values from all logs.
-   **Container Security**: Containers run as a fixed, non-root UID (`1001`). `tako` will `chown` the workspace directory to this UID before starting the container.
-   **Network**: By default, containers have network access. It can be disabled per-step with a `network: none` key in the step definition.

## 5. Caching

-   **Cache Key**: A step's cache key is a SHA256 hash of:
    1.  The step's definition in `tako.yml`.
    2.  A hash of the file contents of the repository, filtered by a `cache_key_files` glob pattern in the step definition (defaults to `**/*`). The `.git` directory is always excluded.

## 6. Migration (`tako migrate`)

-   The `tako migrate` command will be provided to assist users in updating from `v0.1.0`. It will perform a best-effort conversion and add comments to areas that require manual intervention, such as defining `on: artifact_update` triggers. A `--dry-run` flag will be available.

## 7. Built-in Steps (`uses:`)

-   Built-in semantic steps (e.g., `tako/checkout@v1`) are versioned and bundled with the `tako` binary. A `tako steps list` command will be available to show available steps and their parameters.

## 8. Implementation Plan

The implementation will be broken down into the following issues, organized by milestones.

#### Milestone 1: Core Engine Scaffolding

1.  **`feat(config): Implement v0.2.0 schema & migrate command`**: Update `internal/config` and create the `tako migrate` command.
2.  **`feat(cmd): Create 'tako exec' command`**: Add the `exec` command with support for typed inputs.
3.  **`feat(engine): Implement synchronous local runner`**: Create the single-repo execution loop.
4.  **`feat(engine): Implement step output passing`**: Implement `from_stdout` capture, the `produces` block, and `text/template` hydration.

#### Milestone 2: Graph-Aware and Containerized Execution

5.  **`feat(engine): Implement graph-aware execution & planning`**: Implement the artifact-based planning logic with CEL for `if` conditions and parallel execution.
6.  **`feat(engine): Introduce containerized step execution`**: Modify the runner to execute steps in a secure, isolated container with resource limits.
7.  **`feat(engine): Implement 'tako/checkout@v1' semantic step`**: Create the first built-in semantic step and the `tako steps list` command.

#### Milestone 3: Advanced Features & Use Cases

8.  **`feat(engine): Implement 'tako/update-dependency@v1' semantic step`**.
9.  **`feat(engine): Implement 'tako/create-pull-request@v1' semantic step`** with a default retry policy.
10. **`feat(engine): Implement step caching`** with content-addressable keys.
11. **`feat(engine): Implement asynchronous persistence and resume`**.
12. **`feat(exec): Implement --dry-run mode`**.

#### Milestone 4: Testing and Validation

13. **`test(e2e): Add E2E test for single-repo workflow`**.
14. **`test(e2e): Add E2E test for multi-repo fan-out/fan-in`**.

## 9. Final Review and Implementation Readiness Assessment

### 9.1. Major Improvements ✅

This version represents a **significant leap in precision and implementability**:

- **Artifact output association fixed**: The `produces` block (lines 64-67) elegantly solves the previous design flaw where outputs were associated with ALL artifacts
- **CEL expression engine specified**: Line 102 clearly states Common Expression Language for `if` conditions
- **Resource limits syntax defined**: Lines 58-59 provide concrete examples (`cpu_limit: "1.0"`, `mem_limit: "512Mi"`)
- **Input type system added**: Line 53 specifies supported types (`string`, `boolean`, `number`) 
- **Cache algorithm specified**: Line 121 specifies SHA256 hashing
- **Security details clarified**: Fixed UID 1001 (line 116), network restriction syntax (line 117)
- **Built-in step discovery**: `tako steps list` command (line 131)
- **Migration validation**: `--dry-run` flag for migration (line 127)

### 9.2. Remaining Questions and Minor Gaps

**❓ Input validation specifics**
- Line 53 mentions `string`, `boolean`, `number` types but no validation rules
- Are there plans for enum constraints, regex validation, range limits for numbers?
- How are type conversion errors handled (e.g., passing "abc" to a number input)?

**❓ CEL expression security and performance**
- CEL expressions in `if` conditions could be compute-intensive or have security implications
- Are there execution time limits? Sandboxing? Allowed function sets?
- What happens with CEL evaluation errors (syntax or runtime)?

**❓ Container resource enforcement**
- Resource limits are specified but enforcement mechanism unclear
- Are these hard limits (container killed) or soft limits (throttled)?
- What happens when resource limits are exceeded?

**❓ Workspace cleanup and storage management**
- Line 107 mentions workspace path but no cleanup policy
- Are workspaces automatically cleaned after successful runs?
- What about failed runs? Storage quotas?

**❓ Step caching edge cases**
- Cache key includes "file contents of the repository" (line 123) - performance implications for large repos?
- How are file modification times, permissions, and symlinks handled?
- Is there cache size management or eviction policy?

### 9.3. Minor Implementation Details

**🔧 Error handling granularity**
- Step failures halt execution, but no specification of partial recovery
- Should there be continue-on-error options for specific steps?
- How are temporary network failures vs. permanent errors distinguished?

**🔧 Template performance optimization**
- Template parsing per-step could be expensive for complex workflows
- Are parsed templates cached across steps or workflows?
- Memory usage implications for concurrent workflows?

**🔧 Secret scrubbing implementation**
- "Best-effort scrub of exact string values" (line 115) - what about encoded secrets?
- Base64, URL encoding, JSON embedded secrets handling?
- Performance impact of string scanning all logs?

### 9.4. Design Quality Assessment

**Precision Level: EXCELLENT** 🎯
- This version maintains high technical precision while being well-organized
- Critical specifications are present and unambiguous
- Examples are concrete and implementation-ready

**Implementation Readiness: 95%** 🚀
- All major architectural decisions are clearly specified
- Schema is complete with concrete examples
- Security model is well-defined
- Migration path is clear

**Remaining 5%** comprises minor implementation details that can be resolved during development:
- Input validation specifics
- CEL security sandbox configuration  
- Resource limit enforcement mechanism
- Workspace cleanup policies

### 9.5. Security Posture Review

**Strong security foundation** with:
- Container sandboxing with fixed non-root UID
- Secret environment variable isolation
- Network restriction capability
- Unprivileged execution model

**Minor considerations**:
- CEL expression evaluation should be sandboxed
- Large repository cache key computation could cause DoS
- Secret scrubbing should handle encoded values

### 9.6. Final Recommendation

This design document is **ready for implementation**. The major architectural concerns have been resolved, and the specification is sufficiently detailed for development teams to begin work.

The progression from earlier iterations shows **consistent improvement in precision** without losing clarity. The document successfully balances technical rigor with readability.

**Recommended next steps**:
1. Begin Milestone 1 implementation 
2. Create detailed issue templates based on implementation plan
3. Establish acceptance criteria for each milestone
4. Consider creating a technical RFC process for the minor gaps identified above

**Overall Assessment: APPROVED FOR IMPLEMENTATION** ✅