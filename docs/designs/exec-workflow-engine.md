# Design: The `tako exec` Workflow Engine v0.2.0

This document provides the complete technical design for the `tako exec` workflow engine. It is a **breaking change** from the v0.1.0 schema.

## 1. Core Concepts & Principles

-   **Workflows**: A named sequence of steps. Triggered manually (`on: exec`) or automatically (`on: artifact_update`).
-   **Artifacts**: The tangible, versionable outputs of a repository. They are the explicit link between upstream and downstream workflows.
-   **State**: A JSON object capturing step outputs, persisted locally for resumption.
-   **Security**: Workflows run in unprivileged containers; secrets are managed via environment variables and are never persisted.
-   **Clarity & Precision**: The schema and execution model are designed to be unambiguous and directly implementable. While alternative designs (e.g., event-driven, fully declarative) were considered, the imperative, step-based approach was chosen for its simplicity, predictability, and ease of debugging.

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
        validation:
          # Ensures the input is one of the specified values.
          enum: [major, minor, patch]
      tag:
        description: "A tag for the release."
        type: string
        validation:
          # Ensures the input matches the regex.
          regex: "^v[0-9]+\.[0-9]+\.[0-9]+$"
    # Defines resource limits for the main workflow container.
    resources:
      cpu_limit: "1.0" # 1 full CPU core
      mem_limit: "512Mi" # 512 Megabytes
    steps:
      - id: get_version
        if: .inputs.version-bump != "none"
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

**Note on Template Complexity**: The `text/template` syntax provides a powerful and flexible way to parameterize workflows. While it may be more verbose for simple cases, it provides a consistent and well-documented syntax for all use cases. A simpler variable substitution syntax is not planned for the initial release to avoid introducing multiple ways to achieve the same result.
```

### 2.4. `dependents`

Explicitly declares dependencies on artifacts from other repositories.

```yaml
dependents:
  - repo: my-org/downstream-app
    # This repo depends on the 'tako-lib' artifact from this upstream repo.
    artifacts: [tako-lib]
```

### 2.5. Schema Evolution

-   **Versioning**: The `version` field in `tako.yml` is mandatory and will be used to manage schema changes. The schema will follow semantic versioning.
-   **Extensibility**: The initial design does not include support for organization-specific extensions to the schema. Future versions may include a mechanism for this if there is sufficient demand.

## 3. Workflow Execution Model

1.  **Plan Generation**:
    - The engine first builds a dependency graph of all repositories defined in the `dependents` sections of the `tako.yml` files.
    - It performs a topological sort on the graph to determine the execution order. This process naturally detects circular dependencies; if a cycle is found, the execution fails with an error listing the repositories in the cycle.
    - When `tako exec release` is run, the engine executes the `release` workflow in the root repository.
    - The `get_version` step runs and, because of its `produces` block, the engine associates its output (`version`) with the `tako-lib` artifact.
    - The engine then traverses the dependency graph. It finds downstream repos that depend on `tako-lib` and have workflows with `on: artifact_update`.
    - For each potential downstream workflow, it evaluates the `if` condition using the **Common Expression Language (CEL)**. If the expression evaluates to true, the workflow is added to the execution plan. The traversal has no fixed depth limit but is protected from infinite loops by the initial cycle detection.
    - **Artifact Aggregation**: If a downstream repository depends on multiple artifacts that are updated within the same `tako exec` run, the engine will wait for all the corresponding upstream workflows to complete successfully. It will then trigger the downstream workflow only once.
      - **Trigger Context**: The `trigger` context will contain a list of artifacts, accessible via `.trigger.artifacts`. Each element in the list will have the same structure as a single artifact trigger (e.g., `.trigger.artifacts[0].name`, `.trigger.artifacts[0].outputs.version`).
      - **Failure Policy**: If any of the upstream workflows fail, the downstream workflow will not be triggered. The initial design does not support partial success or failure policies for aggregation.
      - **Timeout**: A configurable `aggregation_timeout` (default: `1h`) can be set on the downstream workflow to prevent indefinite waiting. If the timeout is reached before all upstream workflows complete, the workflow will fail.

2.  **Input Validation**:
    - Before execution, the engine validates all workflow inputs against the `validation` rules defined in the schema.
    - Type conversions are attempted (e.g., string "true" to boolean `true`). If a conversion fails or a validation rule is not met, the workflow fails with a descriptive error message.

3.  **Execution**:
    - **Repository Parallelism**: Repositories are processed in parallel, limited by `--max-concurrent-repos` (default: 4).
    - **Step Execution**: Within a single repository's workflow, steps are executed sequentially. Each step can have an optional `if` condition (a CEL expression). The step is skipped if the condition evaluates to false.
    
      - **`if` Condition Context**: The CEL expression in a step's `if` condition has access to the following contexts:
        -   `.inputs`: The workflow's input parameters.
        -   `.steps`: The outputs of all previously completed steps in the same workflow (e.g., `.steps.previous-step.outputs.version`).
        -   `.trigger`: For workflows with `on: artifact_update`, the trigger context containing information about the upstream artifact(s).
    - **Resource Limits**: Each workflow runs in a container. The `resources` block and corresponding CLI flags define hard limits for CPU and memory. If a container exceeds these limits, it will be terminated by the container runtime.
    - **Workspace**: The workspace (`~/.tako/workspaces/<run-id>/...`) is mounted into the container.
    - **Template Caching**: To optimize performance, templates are parsed once per workflow execution and the parsed representation is cached in-memory for the duration of the run. The initial design does not include hard limits on the template cache size, as the memory footprint is expected to be minimal for typical workflows. No hard limit will be imposed.

4.  **State & Resumption**:
    - State is saved to `~/.tako/state/<run-id>.json` after each step. The file is checksummed to detect corruption. If the state file is found to be corrupt, the run fails. While there is no automatic recovery in the initial version, state file versioning and incremental backups are being considered for future releases to improve resilience.
    - To resume, a user runs `tako exec --resume <run-id>`.
    - **Idempotency**: It is the responsibility of the workflow author to design steps to be idempotent, especially in workflows that are expected to be resumed. The engine does not provide any guarantees about partially completed steps.
    - **Cross-Repository Consistency**: The engine does not provide transactional guarantees for state changes across multiple repositories. A failure in one repository's workflow does not automatically roll back changes in another.

### 3.1. Workspace Management

-   **Workspace Path**: Each workflow run is executed in an isolated workspace located at `~/.tako/workspaces/<run-id>`.
-   **Cleanup**: Workspaces are automatically cleaned up after a workflow completes successfully. For failed or persisted workflows, the workspace is retained to allow for debugging and resumption. A `tako workspace clean --older-than <duration>` command will be provided to clean up old workspaces.
-   **Storage Quotas**: The initial design does not include storage quotas for workspaces.

### 3.2. Error Handling

-   **Fail-Fast**: The engine follows a strict fail-fast policy. If any step in the workflow fails, the entire `tako exec` run will halt immediately. The initial design does not include configurable failure policies (e.g., `continue-on-error`), though this could be considered for a future release.

### 3.3. Scalability

-   **Local Execution**: The initial design is focused on providing a powerful and flexible workflow engine for local and single-machine CI environments.
-   **Large-Scale Deployments**: The design does not explicitly address distributed execution or scaling to hundreds of concurrent workflows. These capabilities could be explored in a future release if there is sufficient demand.

#### 3.3.1. Performance Considerations

-   **Graph Parsing**: The performance of a topological sort on the dependency graph is negligible. However, the I/O required to fetch and parse the `tako.yml` file for each repository can introduce a noticeable delay in workflows with a large number of repositories (e.g., >50).
-   **Warning**: The engine will issue a warning if a dependency graph exceeds 50 repositories to alert the user to potential performance degradation. No hard limit will be imposed.

### 3.4. Run ID Generation

-   **Format**: The `<run-id>` is a UUIDv4 string.
-   **Collision Avoidance**: The use of UUIDv4 provides a high degree of confidence that each run will have a unique ID, preventing collisions between concurrent executions.

### 3.8. Error Message Quality

-   **Standard**: Error messages should be clear, concise, and actionable. They should provide context, explain the error, and suggest a solution.
-   **Example of a Good Error Message**:
    ```
    Error: Failed to execute step 'get_version' in workflow 'release'.
    Reason: Input validation failed for 'version-bump'.
    Details: Expected one of [major, minor, patch], but got 'invalid'.
    ```
-   **Example of a Bad Error Message**:
    ```
    Error: Step failed.
    ```

### 3.7. Container Image Management

-   **Image Pull Policy**: By default, `tako` will use the `pull-if-not-present` policy for container images. This can be overridden with an `image_pull_policy` key in the step definition (`always`, `never`, `if-not-present`).
-   **Private Registries**: Authentication with private container registries is handled by the underlying container runtime (Docker, Podman). Users should configure their registry credentials in the standard location for their chosen runtime (e.g., `~/.docker/config.json`).

### 3.6. Designing for Resilience

-   **Compensation Patterns**: Given the lack of transactional guarantees, workflows that perform mutating operations across multiple repositories should be designed with resilience in mind. This can be achieved by using compensation patterns, where a failure in one part of the workflow triggers a compensating action to revert changes in another part.
-   **Idempotency**: As mentioned in the `State & Resumption` section, designing steps to be idempotent is crucial for ensuring that they can be safely retried after a failure.

### 3.5. Container Runtime

-   **Supported Runtimes**: The engine will support both Docker and Podman as container runtimes. It will detect the available runtime by looking for the respective executables in the system's `PATH`.
-   **Fallback Behavior**: If neither Docker nor Podman is available, and a workflow requires containerized execution, the workflow will fail with a clear error message. For workflows that do not specify an `image`, steps will be run directly on the host.

## 4. Security

-   **Secret Scrubbing**: `tako` will maintain a list of secret names from the environment. It will perform a best-effort scrub of the exact string values of these secrets from all logs. This is a best-effort approach and may not catch secrets that have been encoded (e.g., Base64) or transformed. The performance impact of scanning logs is expected to be minimal for typical use cases.
-   **Container Security**: Containers are executed with a set of hardening measures to reduce the risk of container escape and privilege escalation:
    -   **Non-Root User**: Containers run as a fixed, non-root UID (`1001`). `tako` will `chown` the workspace directory to this UID before starting the container.
    -   **Read-Only Root Filesystem**: The container's root filesystem will be mounted as read-only.
    -   **Dropped Capabilities**: By default, all Linux capabilities are dropped. A `capabilities` block can be added to a step to request specific capabilities.
    -   **Seccomp Profile**: A default seccomp profile will be applied to restrict the available syscalls.
    -   **Future Enhancements**: Future versions may include support for AppArmor and SELinux profiles for additional hardening.
-   **Network**: By default, containers do not have network access. It can be enabled per-step with a `network: default` key in the step definition.

### 4.1. CEL Expression Security

-   **Sandboxing**: CEL expressions are evaluated in a sandboxed environment with a restricted set of functions. The sandbox will not have access to the filesystem, network, or environment variables.
-   **Resource Limits**: The execution of CEL expressions will be limited by a strict timeout (e.g., 100ms) and a memory limit (e.g., 64MB) to prevent denial-of-service attacks.
-   **Error Handling**: If a CEL expression fails to evaluate due to a syntax or runtime error, the workflow will fail with a descriptive error message.

### 4.2. Secrets Management

To enhance security and align with best practices, the `tako.yml` file will not store secrets directly. Instead, it will define which secrets are required by a workflow. These secrets must be provided as environment variables to the `tako exec` process.

**Important**: Secret values are **never** interpolated directly into the `tako.yml` file or any logs. The templating engine does not have access to secret values.

```yaml
workflows:
  release:
    on: exec
    # Defines the secrets required by this workflow.
    secrets:
      - GITHUB_TOKEN
      - NPM_TOKEN
    steps:
      - id: publish
        run: ./scripts/publish.sh
        # The engine will make the secrets available as environment
        # variables inside the container. The script can then use them.
        env:
          GH_TOKEN: GITHUB_TOKEN
          NPM_TOKEN: NPM_TOKEN
```

-   **Declaration**: The `secrets` block in a workflow lists the names of the environment variables that the workflow's steps require.
-   **Injection**: The `env` block within a step maps the name of an environment variable inside the container (e.g., `GH_TOKEN`) to the name of a secret declared in the `secrets` block (e.g., `GITHUB_TOKEN`). The `tako` engine is responsible for securely passing the secret's value from its own environment into the container as the specified environment variable.
-   **Scrubbing**: As mentioned in the `Security` section, the names of these secrets will be used to scrub their values from logs.
-   **Error Handling**: If a required secret is not present in the environment when `tako exec` is run, the execution will fail before any steps are run.

**Note on Syntax**: The distinction between the `env` mapping for secrets and the `{{ . }}` interpolation for other values is a deliberate security measure. This ensures that secret values are never processed by the templating engine, preventing accidental disclosure in logs or debug output.

-   **Debug Mode**: A `--debug` flag on `tako exec` will enable step-by-step execution, pausing before each step and waiting for user confirmation to proceed. Secret values will be redacted from any debug output.
-   **State Inspection**: A `tako state inspect <run-id>` command will be provided to print the persisted state of a workflow, which is useful for debugging. Secret values are never persisted to the state file.

## 5. Caching

-   **Cache Key**: A step's cache key is a SHA256 hash of:
    1.  The step's definition in `tako.yml`.
    2.  A hash of the file contents of the repository. To mitigate performance issues in large repositories, the `cache_key_files` glob pattern in the step definition can be used to limit the set of files included in the hash (defaults to `**/*`). The hash is based on file content only; modification times, permissions, and symlinks are ignored. The `.git` directory and Git LFS files are always excluded.
-   **Cache Invalidation**: The cache for a workflow run can be manually invalidated using the `--no-cache` flag on the `tako exec` command. Additionally, the entire cache can be cleared using the `tako cache clean` command.
-   **Cache Management**: The initial design does not include cache size management or eviction policies. The cache is stored at `~/.tako/cache`.

## 6. Migration (`tako migrate`)

-   The `tako migrate` command will be provided to assist users in updating from `v0.1.0`. It will perform a best-effort conversion and add comments to areas that require manual intervention, such as defining `on: artifact_update` triggers. A `--dry-run` flag will be available to show the proposed changes without writing them to disk. A `--validate` flag will also be available to check the migrated configuration for schema errors without running any workflows.

    **Example Output of `tako migrate --dry-run`**:

    ```diff
    --- a/tako.yml
    +++ b/tako.yml
    @@ -1,5 +1,15 @@
    -version: 0.1.0
    -command:
    -  release:
    -    run: ./scripts/release.sh
    +version: 0.2.0
    +workflows:
    +  release:
    +    on: exec
    +    steps:
    +      - id: release
    +        run: ./scripts/release.sh
    +        # TODO: Define the artifacts produced by this step.
    +        # produces:
    +        #   artifact: <artifact-name>
    +        #   outputs:
    +        #     version: from_stdout
    ```

### 6.1. Schema Versioning and Compatibility

-   **Breaking Change**: The `v0.2.0` schema is a breaking change. The `tako` binary at this version will only support `v0.2.0` and later schemas.
-   **Transition Period**: For projects that need to support both `v0.1.0` and `v0.2.0` schemas during a transition, it is recommended to use different versions of the `tako` binary.
-   **Rollback**: If critical issues are discovered in `v0.2.0`, the recommended rollback strategy is to revert to a previous version of the `tako` binary and the `tako.yml` configuration.

## 7. Built-in Steps (`uses:`)

-   Built-in semantic steps (e.g., `tako/checkout@v1`) are versioned and bundled with the `tako` binary. A `tako steps list` command will be available to show available steps and their parameters.
-   **Custom Steps**: The initial design does not include a plugin architecture for creating and distributing custom steps. This could be considered for a future release.

### 7.1. Convenience Commands

To maintain the ease of use for simple, one-off tasks, the existing `tako run` command will be retained as a simplified entrypoint to the workflow engine.

-   **`tako run <command>`**: This command is a convenience wrapper that dynamically constructs and executes a single-step workflow from the provided command. It is equivalent to creating a temporary `tako.yml` with a single `run` step and executing it with `tako exec`.
-   **`tako lint`**: This command will perform a semantic validation of the `tako.yml` file. It will check for common errors such as circular dependencies, unreachable steps, and invalid syntax in CEL expressions.

**Note on Script Migration**: A command to automatically import existing shell scripts into `tako.yml` (`tako import-script`) is not planned for the initial release but may be considered in the future. For now, users are encouraged to manually wrap their existing scripts in `run` steps.

### 7.2. Debugging and Introspection

-   **Debug Mode**: A `--debug` flag on `tako exec` will enable step-by-step execution.
    -   **Interactive Mode**: In an interactive shell, the engine will pause before each step and wait for user confirmation to proceed.
    -   **Non-Interactive Mode**: In a non-interactive environment (e.g., CI), the engine will log the step information and continue without pausing.
-   **State Inspection**: A `tako state inspect <run-id>` command will be provided to print the persisted state of a workflow, which is useful for debugging.

### 7.3. Testing Workflows

-   **Local Testing**: The `--dry-run` flag on `tako exec` is the primary tool for testing workflow definitions. It allows developers to see the execution plan without making any changes.
-   **Unit Testing Steps**: Individual steps that are defined as scripts or commands can be tested using standard shell scripting and testing techniques, outside of the `tako` engine.

### 7.4. CI/CD Integration

-   **Self-Contained System**: `tako` is designed to be a self-contained workflow engine. It can be run in any environment, including local machines and existing CI/CD systems like GitHub Actions or Jenkins.
-   **Triggering from CI/CD**: A common pattern is to have an existing CI/CD pipeline call `tako exec` to orchestrate a multi-repo workflow.
-   **Authentication**: Authentication with external systems (e.g., GitHub, Artifactory) is handled through the secrets management system.

## 8. Implementation Plan

The implementation will be broken down into the following issues, organized by milestones.

#### Milestone 1: MVP - Local, Synchronous Execution

This milestone focuses on delivering the core, single-repository `on: exec` functionality without containerization. This provides immediate value and a solid foundation for more advanced features.

1.  **`feat(config): Implement v0.2.0 schema & migrate command`**: Update `internal/config` and create the `tako migrate` command.
2.  **`feat(cmd): Create 'tako exec' command`**: Add the `exec` command with support for typed inputs.
3.  **`feat(engine): Implement synchronous local runner`**: Create the single-repo execution loop that runs commands directly on the host.
4.  **`feat(engine): Implement step output passing`**: Implement `from_stdout` capture, the `produces` block, and `text/template` hydration.
5.  **`test(e2e): Add E2E test for single-repo workflow`**.

#### Milestone 2: Containerization and Graph-Aware Execution

This milestone introduces the core security and isolation features, and expands execution to multiple repositories.

6.  **`feat(engine): Introduce containerized step execution`**: Modify the runner to execute steps in a secure, isolated container with resource limits.
7.  **`feat(engine): Implement graph-aware execution & planning`**: Implement the artifact-based planning logic with CEL for `if` conditions and parallel execution.
8.  **`feat(engine): Implement 'tako/checkout@v1' semantic step`**: Create the first built-in semantic step and the `tako steps list` command.
9.  **`test(e2e): Add E2E test for multi-repo fan-out/fan-in`**.

#### Milestone 3: Advanced Features & Use Cases

10. **`feat(engine): Implement 'tako/update-dependency@v1' semantic step`**.
11. **`feat(engine): Implement 'tako/create-pull-request@v1' semantic step`** with a default retry policy.
12. **`feat(engine): Implement step caching`** with content-addressable keys.
13. **`feat(engine): Implement asynchronous persistence and resume`**.
14. **`feat(exec): Implement --dry-run mode`**.


## 9. Testing Scenarios

This section outlines several testing scenarios to validate the capabilities of the workflow engine and explore the flow of information between steps and repositories.

### 9.1. Scenario 1: Fan-Out/Fan-In Release

This scenario tests the core graph-aware execution model, where a change in a central library fans out to its dependents, which are then aggregated in a final "bill of materials" repository.

-   **Repo A (`go-lib`)**: The core library.
-   **Repo B (`app-one`)**: A downstream consumer of `go-lib`.
-   **Repo C (`app-two`)**: Another downstream consumer of `go-lib`.
-   **Repo D (`release-bom`)**: A repository that tracks the released versions of `app-one` and `app-two`.

**Execution Flow**:

1.  A user runs `tako exec release --inputs.version-bump=minor` in `go-lib`.
2.  The `release` workflow in `go-lib` runs, builds the library, and `produces` the new version (e.g., `v1.2.0`) for the `go-lib` artifact.
3.  The engine detects that `app-one` and `app-two` depend on `go-lib` and have workflows with `on: artifact_update`.
4.  The engine triggers the `update-downstream` workflow in `app-one` and `app-two` in parallel. The `trigger` context contains the new version from `go-lib`.
5.  Both `app-one` and `app-two` update their `go.mod` file, run tests, and `produce` their own new versions for their respective artifacts (`app-one-artifact` and `app-two-artifact`).
6.  The engine then detects that `release-bom` depends on both of these artifacts. It waits for both `app-one` and `app-two` to complete their workflows.
7.  Finally, the engine triggers the `update-bom` workflow in `release-bom`, which receives the new versions of both apps in its `trigger` context and updates a central `versions.json` file.

**Configuration**:

**Repo A: `go-lib/tako.yml`**
```yaml
version: 0.2.0
artifacts:
  go-lib:
    path: ./go.mod
    ecosystem: go
dependents:
  - repo: my-org/app-one
    artifacts: [go-lib]
  - repo: my-org/app-two
    artifacts: [go-lib]
workflows:
  release:
    on: exec
    inputs:
      version-bump:
        type: string
        default: "patch"
    steps:
      - id: build
        run: ./scripts/get-version.sh --bump {{ .inputs.version-bump }}
        produces:
          artifact: go-lib
          outputs:
            version: from_stdout
```

**Repo B: `app-one/tako.yml`**
```yaml
version: 0.2.0
artifacts:
  app-one-artifact:
    path: ./pom.xml
    ecosystem: maven
dependents:
  - repo: my-org/release-bom
    artifacts: [app-one-artifact]
workflows:
  update-downstream:
    on: artifact_update
    if: trigger.artifact.name == 'go-lib'
    steps:
      - uses: tako/checkout@v1
      - uses: tako/update-dependency@v1
        with:
          version: "{{ .trigger.artifact.outputs.version }}"
      - id: build
        run: ./scripts/get-version.sh # Assumes this script calculates the next version
        produces:
          artifact: app-one-artifact
          outputs:
            version: from_stdout
```

**Repo D: `release-bom/tako.yml`**
```yaml
version: 0.2.0
workflows:
  update-bom:
    on: artifact_update
    steps:
      - uses: tako/checkout@v1
      - id: update_json
        # This script iterates through all triggering artifacts.
        run: |
          #!/bin/bash
          for i in $(seq 0 $(({{ len .trigger.artifacts }} - 1))); do
            NAME=$(echo '{{ index .trigger.artifacts "'$i'" "name" }}')
            VERSION=$(echo '{{ index .trigger.artifacts "'$i'" "outputs" "version" }}')
            ./scripts/update-bom.sh --name $NAME --version $VERSION
          done
```

### 9.2. Scenario 2: Asynchronous Workflow with Resume

This scenario tests the ability to persist the state of a long-running workflow and resume it later.

**Execution Flow**:

1.  A user runs `tako exec process-data` in a repository with a long-running data processing job.
2.  The `prepare-data` step runs successfully.
3.  The `run-simulation` step begins. Because it is marked as `long_running`, the `tako` engine persists the workflow state to `~/.tako/state/<run-id>.json` and exits, returning the `<run-id>` to the user.
4.  The user can now close their terminal. The simulation continues to run in its container.
5.  Later, the user checks the status of the simulation. Once it is complete, they resume the workflow with `tako exec --resume <run-id>`.

<!-- EFFICIENCY CONCERN: How does the user "check the status of the simulation"? There's no documented command for this. Should there be a `tako status <run-id>` command? -->

<!-- RESOURCE MANAGEMENT QUESTION: Long-running containers could consume system resources indefinitely. Are there resource limits that persist after the main process exits? What happens if the system runs out of disk space or memory? -->
6.  The engine loads the state, sees that the `run-simulation` step was the last one running, and proceeds to the next step, `publish-results`.

**Configuration**:

```yaml
version: 0.2.0
workflows:
  process-data:
    on: exec
    steps:
      - id: prepare-data
        run: ./scripts/prepare.sh
        produces:
          outputs:
            dataset_id: from_stdout
      - id: run-simulation
        # This new key indicates the engine should not wait for completion.
        long_running: true
        run: ./scripts/simulation.sh --dataset {{ .steps.prepare-data.outputs.dataset_id }}
        
        <!-- PRECISION LOSS: The `long_running` feature is introduced here without any documentation in the main schema section (2.3) or execution model (section 3). This is a significant new feature that affects container lifecycle, state management, and resource usage. -->
        
        <!-- SECURITY CONCERN: Long-running containers that persist after the main tako process exits could be a security risk. How are these containers secured? What prevents them from running indefinitely? Are there automatic cleanup mechanisms? -->
        
        <!-- FUNCTIONALITY QUESTION: How does the engine track the status of long-running steps? What happens if the container crashes or the system reboots? Is there a mechanism to detect if the long-running step has actually completed successfully? -->
      - id: check-simulation
        # This step polls for the result of the long-running step.
        # The `tako/poll` built-in step would handle the logic of checking
        # for a specific output (e.g., a file, a status in a database).
        uses: tako/poll@v1
        with:
          target: file
          path: ./results/{{ .steps.prepare-data.outputs.dataset_id }}.json
          timeout: 60m
      - id: publish-results
        run: ./scripts/publish.sh --dataset {{ .steps.prepare-data.outputs.dataset_id }}
```

We will need to add a new built-in step `tako/poll@v1` to support this functionality.

<!-- PRECISION LOSS: The `tako/poll@v1` built-in step is used in the example above but not documented in Appendix B: Built-in Steps. This reduces implementation precision. -->

<!-- SECURITY CONCERN: A polling mechanism that can check files, databases, or external systems could be a security risk if not properly sandboxed. How does polling interact with the container's network isolation and filesystem restrictions? -->

<!-- EFFICIENCY CONCERN: Continuous polling for 60 minutes could be resource-intensive. Should there be exponential backoff or configurable polling intervals? -->


## Appendix A: CLI Reference

| Flag                     | Description                                                                                             | Default |
| ------------------------ | ------------------------------------------------------------------------------------------------------- | ------- |
| `--max-concurrent-repos` | The maximum number of repositories to process in parallel.                                              | `4`       |
| `--no-cache`             | Invalidate the cache for this run and execute all steps.                                                | `false`   |
| `--debug`                | Enable debug mode, which provides step-by-step execution and additional logging.                        | `false`   |
| `--resume <run-id>`      | Resume a previously persisted workflow run.                                                             |         |
| `--dry-run`              | Print the execution plan without making any changes.                                                    | `false`   |
| `--inputs.<name>=<value>`| Pass an input variable to the workflow.                                                                 |         |

## Appendix B: Built-in Steps

### `tako/checkout@v1`

Checks out the source code of the repository.

**Parameters**:

-   `ref` (string): The branch, tag, or commit SHA to checkout. Defaults to the current branch.

### `tako/update-dependency@v1`

Updates a dependency in a repository. The step will automatically detect the package manager and update the dependency.

**Parameters**:

-   `name` (string, required): The name of the dependency to update.
-   `version` (string, required): The new version of the dependency.

### `tako/create-pull-request@v1`

Creates a pull request on the code hosting platform.

**Parameters**:

-   `title` (string, required): The title of the pull request.
-   `body` (string, required): The body of the pull request.
-   `base` (string, required): The base branch for the pull request.
-   `head` (string, required): The head branch for the pull request.

**Sanitization**: All error messages originating from template parsing or execution will be sanitized to prevent the leaking of sensitive information or internal system details.

