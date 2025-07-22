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


### 9. Remaining Questions and Minor Gaps

This is a comprehensive and well-thought-out design. The following are some suggestions for improvement and questions that came up during the review.

### 9.1. Complexity and Phased Rollout

The proposed design is a significant leap in functionality, and with that comes complexity. While the implementation plan is broken down into milestones, it might be beneficial to consider an even more phased rollout to de-risk the implementation.

*   **Suggestion:** Could Milestone 1 be broken down further? For example, the first release could focus solely on the `on: exec` trigger and local, non-containerized execution. This would provide value to users quickly while the more complex features like containerization and `on: artifact_update` are being developed.

### 9.2. Security Model

The security model is a strong point of the design. Running steps in containers is a great way to isolate them and prevent them from interfering with the host system.

*   **Question:** How will secrets be managed? The design mentions that they will be passed as environment variables, but it doesn't specify how they will be defined in the `tako.yml` file. Will there be a separate `secrets` block?
*   **Suggestion:** It would be beneficial to add a section to the design that explicitly details the secret management strategy. This should include how secrets are defined, how they are passed to containers, and how they are scrubbed from logs.

### 9.3. Caching

The caching mechanism is well-defined and will be a great performance enhancement.

*   **Suggestion:** It would be useful to have a way to manually invalidate the cache for a specific step or workflow. This could be a CLI flag (e.g., `tako exec --no-cache`) or a command (e.g., `tako cache clean`).

### 9.4. Usability

The proposed CLI is very flexible, but it could be difficult to use for simple cases.

*   **Suggestion:** Consider adding some convenience commands or flags to make it easier to use. For example, a `tako run` command that is a simplified version of `tako exec` for running a single command across all repositories. This would be similar to the existing `run` command, but it would be integrated with the new workflow engine.

### 9.5. Migration

The `tako migrate` command is a great idea and will be essential for users upgrading from v0.1.0.

*   **Suggestion:** In addition to the `--dry-run` flag, it would be useful to have a `--validate` flag that checks the migrated configuration for errors without actually running any workflows. This would give users more confidence that the migration was successful.

### 9.6. Implementation and Testing

The implementation plan is very detailed, but it could be improved by adding more information about how the different milestones will be tested.

*   **Suggestion:** For each milestone, it would be beneficial to define the specific E2E tests that will be created. This will help to ensure that the implementation is correct and that there are no regressions. For example, for Milestone 2, an E2E test could be created that runs a workflow in a container and verifies that the output is correct.

### 9.7. Additional Suggestions 
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

### 9.8. Minor Implementation Details

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

### 9.9. Security Posture Review

**Strong security foundation** with:
- Container sandboxing with fixed non-root UID
- Secret environment variable isolation
- Network restriction capability
- Unprivileged execution model

**Minor considerations**:
- CEL expression evaluation should be sandboxed
- Large repository cache key computation could cause DoS
- Secret scrubbing should handle encoded values
