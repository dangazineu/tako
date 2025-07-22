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
        # This new key indicates the engine should not wait for completion.
        long_running: false
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
    # Timeout for waiting for all upstream artifacts to complete (default: 1h)
    aggregation_timeout: "2h"
    steps:
      - uses: tako/checkout@v1
      - uses: tako/update-dependency@v1
        with:
          # The trigger context is populated by the engine with the
          # outputs from the upstream 'produces' block.
          version: "{{ .trigger.artifact.outputs.version }}"

**Note on Template Complexity**: The `text/template` syntax provides a powerful and flexible way to parameterize workflows. While it may be more verbose for simple cases, it provides a consistent and well-documented syntax for all use cases. A simpler variable substitution syntax is not planned for the initial release to avoid introducing multiple ways to achieve the same result.

**Note on Template Functions**: To simplify common patterns, especially iteration, the template engine will be augmented with a set of custom functions. For example, iterating over trigger artifacts can be done directly in the template, making scripts cleaner and more readable. This approach was chosen over environment variable injection or dedicated iteration steps as it integrates seamlessly with the existing template syntax and offers the most flexibility.
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
    - **Artifact Aggregation and Triggering**: The engine's behavior for `on: artifact_update` workflows is governed by the `dependents` section of the upstream repository and the `if` condition in the downstream workflow.
      - **Trigger Evaluation**: When an artifact is produced by an upstream repository, the engine identifies all downstream repositories that declare a dependency on that artifact. For each of these downstream repositories, it evaluates all workflows with an `on: artifact_update` trigger.
      - **`if` Condition for Filtering**: The workflow's `if` condition is the primary mechanism for controlling execution. It is a CEL expression that receives the trigger context. For a single artifact update, the context is `.trigger.artifact`. This allows for fine-grained control. For example, a workflow can check the artifact name, version, or other outputs to decide if it should run.
        ```yaml
        # Example: Trigger only for a specific artifact and version range
        if: trigger.artifact.name == 'go-lib' && semver.satisfies(trigger.artifact.outputs.version, '^1.2.0')
        ```
        *(Note: This assumes `semver` functions will be added to the CEL environment.)*
      - **Aggregation for Multiple Updates**: If multiple artifacts that a single downstream repository depends on are updated within the same `tako exec` run, the engine aggregates these triggers. It will wait for all the corresponding upstream workflows to complete successfully. It will then trigger the downstream workflow only once.
        - **Trigger Context**: In an aggregation scenario, the `trigger` context will contain a list of artifacts, accessible via `.trigger.artifacts`. The workflow's `if` condition can then iterate over this list or check its contents.
          ```yaml
          # Example: Trigger only if both 'go-lib' and 'java-lib' are present
          if: "'go-lib' in [a.name for a in trigger.artifacts] && 'java-lib' in [a.name for a in trigger.artifacts]"
          ```
        - **Partial Updates**: The default behavior is to trigger only when *all* upstream dependencies that were part of the initial `tako exec` run have completed. The initial design does not support triggering on a subset of dependencies (an "any" or "N-of-M" strategy). This keeps the execution model predictable.
        - **Failure Policy**: If any of the upstream workflows that are part of the aggregation fail, the downstream workflow will not be triggered. The initial design does not support partial success or failure policies for aggregation.
        - **Timeout**: A configurable `aggregation_timeout` (default: `1h`) can be set on the downstream workflow to prevent indefinite waiting. If the timeout is reached before all required upstream workflows complete, the workflow will fail.

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
    - **Resource Limits**: Each workflow runs in a container. The `resources` block and corresponding CLI flags define hard limits for CPU and memory. If a container exceeds these limits, it will be terminated by the container runtime. For long-running steps, these resource limits continue to be enforced after the main `tako` process has exited.
    - **Resource Monitoring and Reporting**: To provide better visibility into resource usage, the engine will:
      - **Log Warnings**: Periodically monitor container resource usage and log a warning if it approaches the defined limits (e.g., >90% of memory or CPU). This helps diagnose terminations due to resource exhaustion.
      - **Post-Execution Reporting**: The `tako status <run-id>` command will include a summary of the peak resource usage for each step, allowing users to analyze and optimize their workflow's resource consumption.
    - **NOTE on Resource Exhaustion**: While resource limits are enforced, it is still possible for a long-running container to consume significant disk space in the workspace. The initial design does not include disk space quotas. Users should be mindful of this when designing workflows with long-running steps. Future versions may include configurable disk quotas and more advanced resource management features.
    - **Workspace**: The workspace (`~/.tako/workspaces/<run-id>/...`) is mounted into the container.
    - **Template Caching**: To optimize performance, templates are parsed once per workflow execution and the parsed representation is cached in-memory for the duration of the run. The initial design does not include hard limits on the template cache size, as the memory footprint is expected to be minimal for typical workflows. No hard limit will be imposed.

4.  **State & Resumption**:
    - **State Persistence**: State is saved to `~/.tako/state/<run-id>.json` after each step completes successfully. This ensures that the state file always represents a consistent, completed step.
    - **Corruption Detection**: The state file is checksummed to detect corruption. If the state file is found to be corrupt during a resume operation, the run fails with a clear error.
    - **State Backups**: To improve resilience against corruption, before writing a new state file, the engine will create a backup of the previous state file (e.g., `<run-id>.json.bak`). If the primary state file is found to be corrupt, the engine will automatically attempt to fall back to the backup file.
    - **Resumption**: To resume, a user runs `tako exec --resume <run-id>`. The engine will load the state and continue execution from the next step.
    - **Idempotency**: It is the responsibility of the workflow author to design steps to be idempotent, especially in workflows that are expected to be resumed. The engine does not provide any guarantees about partially completed steps.
    - **Cross-Repository Consistency**: The engine does not provide transactional guarantees for state changes across multiple repositories. A failure in one repository's workflow does not automatically roll back changes in another.
    - **Future Enhancements**: For future releases, more advanced recovery mechanisms are being considered, such as deeper state validation against the workflow schema and tools for manual state inspection and repair.

5.  **Long-Running Steps**:
    - Steps can be marked as `long_running: true`. When the engine encounters such a step, it will start the step's container and then immediately persist the workflow state and exit, returning the `<run-id>` to the user.
    - The container will continue to run in the background. The user can check its status with `tako status <run-id>` and resume the workflow with `tako exec --resume <run-id>` once the long-running step has completed.
    - It is the responsibility of the workflow author to ensure that the long-running step will eventually complete and that there is a way to determine its completion. The `tako/poll@v1` built-in step is provided for this purpose, which can monitor for conditions like the completion of a container or the existence of a file.
    - **Output Capture**: To capture outputs from a long-running step, the step must have a `produces` block. The long-running process is responsible for writing its outputs to a JSON file at the well-known path `.tako/outputs.json` within the step's workspace. When the workflow is resumed and a subsequent polling step confirms completion, the engine will read this file to populate the outputs into the workflow state, making them available to downstream steps.
    - **Failure Detection**: The engine does not actively monitor long-running steps for crashes or system reboots. If a container crashes, it will simply exit. It is up to the workflow author to use the `tako/poll@v1` step with appropriate timeouts and checks to detect such failures. For example, a polling step can check the exit code of the long-running step's container.
    - **System Reboot Recovery**: When a system reboots while a long-running container is executing, the container will be lost. During workflow resumption, the engine will detect that the referenced container no longer exists and will restart the long-running step from the beginning. The engine accomplishes this by checking container existence before attempting to poll or resume from a long-running step. While this means some work may be repeated, it ensures consistent behavior and prevents the workflow from becoming permanently stuck.
    - **Orphaned Container Management**: To prevent resource leaks from long-running containers that become orphaned, the engine will implement automatic cleanup mechanisms:
      - **Container Labeling**: All `tako`-managed containers are labeled with metadata including the run ID and creation timestamp.
      - **Automatic Cleanup**: The `tako status` and `tako exec --resume` commands will automatically detect and clean up containers that have been running for more than 24 hours without an associated active workflow state.
      - **Manual Cleanup**: A `tako container clean --older-than <duration>` command will be provided to manually clean up orphaned containers based on age or other criteria.

### 3.1. Workspace Management

-   **Workspace Path**: Each workflow run is executed in an isolated workspace located at `~/.tako/workspaces/<run-id>`.
-   **Cleanup**: Workspaces are automatically cleaned up after a workflow completes successfully. For failed or persisted workflows, the workspace is retained to allow for debugging and resumption. A `tako workspace clean --older-than <duration>` command will be provided to clean up old workspaces.
-   **Storage Quotas**: The initial design does not include storage quotas for workspaces.

### 3.2. Error Handling

-   **Fail-Fast**: The engine follows a strict fail-fast policy. If any step in the workflow fails, the entire `tako exec` run will halt immediately. The initial design does not include configurable failure policies (e.g., `continue-on-error`), though this could be considered for a future release.

### 3.2.1. Failure and Compensation

While the engine follows a fail-fast policy for the main workflow, it provides a mechanism for running compensating actions upon failure.

-   **`on_failure` block**: A step can have an `on_failure` block that defines a sequence of steps to run if the primary `run` or `uses` block fails.

    ```yaml
    steps:
      - id: publish_package
        run: ./scripts/publish.sh
        on_failure:
          - id: rollback_publish
            run: ./scripts/rollback.sh --version {{ .steps.publish_package.outputs.version }}
    ```

-   **Execution Context**: The `on_failure` steps are executed in the same context as the failed step and have access to the same `.inputs`, `.steps`, and `.trigger` variables.
-   **Failure in Compensation**: If a step within the `on_failure` block fails, the entire workflow run halts immediately. There is no compensation for compensation.
-   **User Responsibility**: This mechanism provides a hook for rollbacks, but it is still the workflow author's responsibility to implement the compensation logic and ensure that compensating actions are idempotent. The engine does not provide transactional guarantees.

### 3.3. Scalability

-   **Local Execution**: The initial design is focused on providing a powerful and flexible workflow engine for local and single-machine CI environments.
-   **Large-Scale Deployments**: The design does not explicitly address distributed execution or scaling to hundreds of concurrent workflows. These capabilities could be explored in a future release if there is sufficient demand.

#### 3.3.1. Performance and Scalability Considerations

While the engine is designed for local and single-machine CI environments, users should be aware of the following practical limits.

-   **Dependency Graph Size**: The performance of a topological sort on the dependency graph is negligible. However, the network I/O required to fetch and parse the `tako.yml` file for each repository can introduce a noticeable delay in workflows with a large number of repositories (e.g., >50). The engine will issue a warning if a dependency graph exceeds 50 repositories to alert the user to potential performance degradation. No hard limit will be imposed.
-   **Memory Usage**:
    -   **State Management**: The full workflow state is held in memory during execution. For workflows with a very large number of steps or steps that produce very large outputs, this can lead to significant memory consumption.
    -   **Template Caching**: The in-memory cache for parsed templates is not size-limited. While the memory footprint for each template is small, a workflow with thousands of unique `run` blocks could consume a considerable amount of memory.
-   **Disk Usage**:
    -   **Workspaces**: Each workflow run creates an isolated workspace. Concurrently running many workflows, especially those that generate large artifacts, can consume significant disk space. The `tako workspace clean` command is provided for manual cleanup.
    -   **State Files**: State files are typically small, but for workflows with extensive output capture, the `~/.tako/state/<run-id>.json` file can grow large. The engine does not currently implement streaming or partial state writes.
-   **Guideline**: As a general guideline, the v0.2.0 engine is optimized for workflows involving up to 50 repositories, with a few hundred steps in total, and where individual step outputs are in the order of kilobytes, not megabytes.

### 3.4. Run ID Generation

-   **Format**: The `<run-id>` is a UUIDv4 string.
-   **Collision Avoidance**: The use of UUIDv4 provides a high degree of confidence that each run will have a unique ID, preventing collisions between concurrent executions.

### 3.5. Error Message Quality

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

### 3.6. Container Image Management

-   **Image Pull Policy**: By default, `tako` will use the `pull-if-not-present` policy for container images. This can be overridden with an `image_pull_policy` key in the step definition (`always`, `never`, `if-not-present`).
-   **Private Registries**: Authentication with private container registries is handled by the underlying container runtime (Docker, Podman). Users should configure their registry credentials in the standard location for their chosen runtime (e.g., `~/.docker/config.json`).

### 3.7. Designing for Resilience

-   **Compensation**: The engine provides a direct mechanism for compensation via the `on_failure` block within a step (see `Error Handling`). For workflows that perform mutating operations across multiple repositories, this should be used to design resilient workflows. For example, if a downstream workflow fails after an upstream workflow created a pull request, a compensating action could be to close that pull request.
-   **Idempotency**: As mentioned in the `State & Resumption` section, designing steps to be idempotent is crucial for ensuring that they can be safely retried after a failure.

### 3.8. Container Runtime

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
-   **Long-Running Containers**: Long-running containers are subject to the same security restrictions as regular containers. It is the responsibility of the user to ensure that long-running containers are eventually stopped and that workspaces are cleaned up. The `tako workspace clean` command can be used for this purpose. Future versions may include a mechanism to automatically clean up orphaned containers that have been running for an excessive amount of time.

### 4.1. CEL Expression Security

-   **Sandboxing**: CEL expressions are evaluated in a sandboxed environment with a restricted set of functions. The sandbox will not have access to the filesystem, network, or environment variables.
-   **Resource Limits**: The execution of CEL expressions will be limited by a strict timeout (e.g., 100ms) and a memory limit (e.g., 64MB) to prevent denial-of-service attacks.
-   **Error Handling**: If a CEL expression fails to evaluate due to a syntax or runtime error, the workflow will fail with a descriptive error message.

### 4.1.1. Template Engine Security

In addition to sandboxing CEL expressions, the `text/template` engine used for `run` blocks has its own security considerations.

-   **Command Injection Risk**: The primary risk is command injection. If a template substitutes an output from a previous step or an artifact directly into a shell command, a malicious output could execute arbitrary code.
    -   **User Responsibility**: It is the workflow author's responsibility to treat all external inputs and step outputs as untrusted data.
    -   **Mitigation**: To mitigate this, the engine will provide built-in template functions for escaping shell arguments (e.g., `shell_quote`). Workflows should always use these functions when substituting external data into a command line.
      ```yaml
      # GOOD: Properly escaped
      run: ./scripts/process.sh --message {{ .inputs.message | shell_quote }}

      # BAD: Vulnerable to injection
      run: ./scripts/process.sh --message {{ .inputs.message }}
      ```
-   **Filesystem and Network Access**: The standard Go `text/template` engine does not provide functions to access the filesystem or network. Any custom functions added to the template environment will be carefully designed to not introduce such capabilities.
-   **Information Disclosure**:
    -   **Secret Scrubbing**: As mentioned, secret values are never available to the template engine and are scrubbed from logs.
    -   **Debug Mode**: The `--debug` mode will print resolved template variables. Users should be aware that this could expose non-secret sensitive data in logs if the workflow handles such data.

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

### 7.1. Future Extensibility for Custom Steps

While the initial release will not support a public plugin architecture for custom steps, the internal design of the built-in step system will be guided by the following principles to facilitate future extensibility.

-   **Step Interface**: All built-in steps will implement a common Go interface. This interface will define how a step is initialized with parameters, how it's executed, and how it returns outputs.

    ```go
    // A simplified example of the potential interface
    type Step interface {
        Init(params map[string]interface{}) error
        Run(ctx context.Context, workspace string) (map[string]string, error)
    }
    ```

-   **Self-Contained Logic**: Each built-in step will be a self-contained package, minimizing dependencies on the core engine. This will make it easier to eventually source steps from external plugins.
-   **Future Distribution Models**: Several models for distributing custom steps could be considered in the future:
    -   **Compiled Plugins**: Using Go's plugin package to load shared object files (`.so`). This is powerful but has limitations regarding version skew and platform compatibility.
    -   **Interpreted Scripts**: Allowing steps to be defined as scripts in a language like Lua or JavaScript, executed in an embedded interpreter. This is more portable but may be slower.
    -   **Remote Steps**: Fetching step definitions from a central registry, similar to GitHub Actions. This would require a well-defined, serializable format for step definitions.
-   **Security**: Any future plugin system will need a robust security model, likely extending the container and CEL sandboxing concepts to custom step code. Untrusted steps would need to run with even stricter limitations than built-in ones.

### 7.2. Convenience Commands

To maintain the ease of use for simple, one-off tasks, the existing `tako run` command will be retained as a simplified entrypoint to the workflow engine.

-   **`tako run <command>`**: This command is a convenience wrapper that dynamically constructs and executes a single-step workflow from the provided command. It is equivalent to creating a temporary `tako.yml` with a single `run` step and executing it with `tako exec`.
-   **`tako lint`**: This command will perform a semantic validation of the `tako.yml` file. It will check for common errors such as circular dependencies, unreachable steps, and invalid syntax in CEL expressions.

**Note on Script Migration**: A command to automatically import existing shell scripts into `tako.yml` (`tako import-script`) is not planned for the initial release but may be considered in the future. For now, users are encouraged to manually wrap their existing scripts in `run` steps.

### 7.3. Debugging and Introspection

To help users understand and debug complex workflows, the engine provides several tools.

-   **Debug Mode**: A `--debug` flag on `tako exec` enables step-by-step execution.
    -   **Interactive Mode**: In an interactive shell, the engine will pause before each step, print the step's definition and resolved template variables, and wait for user confirmation to proceed. This allows for close inspection of the execution flow.
    -   **Non-Interactive Mode**: In a non-interactive environment (e.g., CI), the engine will log the step information and continue without pausing.
-   **State Inspection**: A `tako state inspect <run-id>` command will be provided to print the persisted state of a workflow, which is useful for debugging. Secret values are never persisted to the state file and will not be displayed.
-   **Status Check**: A `tako status <run-id>` command will be provided to check the status of a running or completed workflow. For long-running steps, this command will show the status of the detached container.
-   **Workflow Replay**: The engine will support replaying a failed workflow from the point of failure. The `tako exec --resume <run-id>` command will be enhanced to detect a failed state and allow the user to retry the failed step. For more complex scenarios, a future `--replay-from <step-id>` flag could allow re-running a workflow from an arbitrary step.
-   **Future Enhancements**: While not planned for the initial release, the following features are being considered for improving the debugging experience in the future:
    -   **Execution Visualization**: A command to generate a DOT file or a terminal-based visualization of the dependency graph and execution plan.
    -   **Failure Analysis**: A tool that analyzes a failed run's state and logs to suggest common causes for the failure.

### 7.4. Testing Workflows

-   **Local Testing**: The `--dry-run` flag on `tako exec` is the primary tool for testing workflow definitions. It allows developers to see the execution plan without making any changes.
-   **Unit Testing Steps**: Individual steps that are defined as scripts or commands can be tested using standard shell scripting and testing techniques, outside of the `tako` engine.

### 7.5. CI/CD Integration

-   **Self-Contained System**: `tako` is designed to be a self-contained workflow engine. It can be run in any environment, including local machines and existing CI/CD systems like GitHub Actions or Jenkins.
-   **Triggering from CI/CD**: A common pattern is to have an existing CI/CD pipeline call `tako exec` to orchestrate a multi-repo workflow.
-   **Authentication**: Authentication with external systems (e.g., GitHub, Artifactory) is handled through the secrets management system.

## 8. Testing Scenarios

This section outlines several testing scenarios to validate the capabilities of the workflow engine and explore the flow of information between steps and repositories.

### 8.1. Scenario 1: Fan-Out/Fan-In Release

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
        run: |
          #!/bin/bash
          {{ range .trigger.artifacts }}
          ./scripts/update-bom.sh --name {{ .name }} --version {{ .outputs.version }}
          {{ end }}
```

### 8.2. Scenario 2: Asynchronous Workflow with Resume

This scenario tests the ability to persist the state of a long-running workflow and resume it later.

**Execution Flow**:

1.  A user runs `tako exec process-data` in a repository with a long-running data processing job.
2.  The `prepare-data` step runs successfully.
3.  The `run-simulation` step begins. Because it is marked as `long_running`, the `tako` engine persists the workflow state to `~/.tako/state/<run-id>.json` and exits, returning the `<run-id>` to the user.
4.  The user can now close their terminal. The simulation continues to run in its container.
5.  Later, the user checks the status of the simulation. Once it is complete, they resume the workflow with `tako exec --resume <run-id>`.

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
      - id: check-simulation
        # This step polls for the result of the long-running step.
        uses: tako/poll@v1
        with:
          target: step
          step_id: run-simulation
          timeout: 60m
          success_on_exit_code: 0
      - id: publish-results
        run: ./scripts/publish.sh --dataset {{ .steps.prepare-data.outputs.dataset_id }}
```

The `tako/poll@v1` built-in step is documented in Appendix B.


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

### `tako/poll@v1`

Polls for a specific condition to be met, typically used to check the status of a long-running step.

**Parameters**:

-   `target` (string, required): The target to poll. Supported values: `file`, `step`.
-   `path` (string, optional): The path to the file to check. Required if `target` is `file`.
-   `step_id` (string, optional): The `id` of the long-running step to check. Required if `target` is `step`.
-   `timeout` (duration, required): The maximum time to wait for the condition to be met.
-   `interval` (duration, optional): The interval at which to poll. Defaults to `10s`. Future versions may include support for exponential backoff.
-   `content_pattern` (string, optional): If `target` is `file`, this regex pattern must match the file's content for the poll to succeed.
-   `success_on_exit_code` (int, optional): If `target` is `step`, the poll succeeds if the container for the specified step has exited with this code. Defaults to `0`.

**Security Note**: The `tako/poll@v1` step executes within the step's container and is subject to the same security restrictions, including filesystem and network isolation. It can only access resources that are available to the container.

**Sanitization**: All error messages originating from template parsing or execution will be sanitized to prevent the leaking of sensitive information or internal system details.

## 9. Implementation Plan

The implementation will be broken down into the following GitHub issues, organized by milestones.

### Milestone 1: MVP - Local, Synchronous Execution

This milestone focuses on delivering the core, single-repository `on: exec` functionality without containerization. This provides immediate value and a solid foundation for more advanced features.

### Issue 1: `feat(config): Implement v0.2.0 schema & migrate command`

**Description:**
Update the configuration system to support the new v0.2.0 schema with workflows, step definitions, and input validation. Implement the `tako migrate` command to convert v0.1.0 configurations to v0.2.0.

**Implementation Details:**
- Update `internal/config/config.go` to support the new workflow schema
- Extend `Workflow` struct to include `On`, `If`, `AggregationTimeout`, `Inputs`, `Resources`, `Steps` fields
- Implement step schema with `ID`, `If`, `Run`, `Uses`, `With`, `Produces`, `LongRunning` fields
- Add input validation types (string, boolean, number) with enum and regex validation
- Create `cmd/tako/internal/migrate.go` command
- Support `--dry-run` and `--validate` flags for migration command
- Implement schema version validation to reject unsupported versions
- Update existing tests to use v0.2.0 schema
- Add comprehensive migration test coverage

**Acceptance Criteria:**
- [ ] v0.2.0 schema fully supported in config loading
- [ ] `tako migrate` converts v0.1.0 to v0.2.0 with comments for manual steps
- [ ] `tako migrate --dry-run` shows proposed changes without modification
- [ ] `tako migrate --validate` checks migrated config for errors
- [ ] Backward compatibility: v0.2.0+ schemas load correctly
- [ ] Breaking change: v0.1.0 schemas are rejected with clear error message
- [ ] All existing tests pass with updated configuration

**Files to Modify:**
- `internal/config/config.go`
- `cmd/tako/internal/migrate.go` (new)
- Test files throughout the codebase
- Example `tako.yml` files in tests

---

### Issue 2: `feat(cmd): Create 'tako exec' command`

**Description:**
Implement the `tako exec` command to execute workflows defined in `tako.yml`. Support workflow input validation, type conversion, and basic command execution without containerization.

**Implementation Details:**
- Create `cmd/tako/internal/exec.go` command
- Implement workflow input parsing from `--inputs.<name>=<value>` CLI flags
- Add input validation against schema (type checking, enum validation, regex validation)
- Implement type conversion (string to boolean/number)
- Create basic workflow execution loop for single repository
- Support workflow-level `if` conditions using CEL evaluation
- Add step-level `if` condition support
- Integrate CEL expression evaluation with security sandboxing
- Implement template parsing and caching for performance
- Add comprehensive error messages following the design standard
- Support `--debug` flag for step-by-step execution

**Acceptance Criteria:**
- [ ] `tako exec <workflow-name>` executes workflows from current directory
- [ ] `--inputs.<name>=<value>` flags properly parsed and validated
- [ ] Type conversions work correctly (string "true" → boolean true)
- [ ] Input validation errors are clear and actionable
- [ ] Workflow-level and step-level `if` conditions work with CEL
- [ ] Template variables are properly substituted in step commands
- [ ] `--debug` mode provides interactive step-by-step execution
- [ ] Error messages follow the design standard (context, reason, details)

**Files to Modify:**
- `cmd/tako/internal/exec.go` (new)
- `internal/engine/` package (new)
- Integration with existing `internal/config` and `internal/errors`

---

### Issue 3: `feat(engine): Implement synchronous local runner`

**Description:**
Create the core execution engine that runs workflow steps sequentially on the host system. This provides the foundation for step execution, output capture, and state management.

**Implementation Details:**
- Create `internal/engine/runner.go` for step execution
- Implement synchronous step execution on host (no containers initially)
- Add workspace management at `~/.tako/workspaces/<run-id>/`
- Implement UUIDv4 run ID generation
- Create basic state persistence to `~/.tako/state/<run-id>.json`
- Add state file checksumming for corruption detection
- Support step execution with proper directory context switching
- Implement basic resource limit monitoring (process-level)
- Add workspace cleanup after successful completion
- Create foundation for resumable execution (state loading)

**Acceptance Criteria:**
- [ ] Steps execute sequentially in workspace directory
- [ ] Unique UUIDv4 run IDs generated for each execution
- [ ] Workspace created at `~/.tako/workspaces/<run-id>/`
- [ ] State persisted to `~/.tako/state/<run-id>.json` after each step
- [ ] State files include checksums and detect corruption
- [ ] Failed workflows retain workspace for debugging
- [ ] Successful workflows clean up workspace automatically
- [ ] Step execution respects working directory context

**Files to Modify:**
- `internal/engine/runner.go` (new)
- `internal/engine/state.go` (new)
- `internal/engine/workspace.go` (new)

---

### Issue 4: `feat(engine): Implement step output passing`

**Description:**
Implement the `produces` block functionality to capture step outputs and make them available to subsequent steps via template variables.

**Implementation Details:**
- Extend step execution to capture stdout when `produces.outputs.from_stdout` is specified
- Implement artifact association via `produces.artifact` field
- Add template context with `.inputs`, `.steps.<id>.outputs`, and `.trigger` variables
- Create output state management and persistence
- Support template variable resolution in step commands
- Add custom template functions for iteration (e.g., `range .trigger.artifacts`)
- Implement template caching for performance optimization
- Add validation for output references in templates
- Support step output chaining (step A → step B → step C)

**Acceptance Criteria:**
- [ ] `produces.outputs` captures stdout and stores in workflow state
- [ ] Template variables `.steps.<id>.outputs.<name>` work in subsequent steps
- [ ] Artifact association via `produces.artifact` field functions correctly
- [ ] Custom template functions for iteration are available
- [ ] Template parsing errors provide clear error messages
- [ ] Output state is properly persisted between steps
- [ ] Template performance is acceptable with caching

**Files to Modify:**
- `internal/engine/runner.go`
- `internal/engine/template.go` (new)
- `internal/engine/outputs.go` (new)

---

### Issue 5: `test(e2e): Add E2E test for single-repo workflow`

**Description:**
Create comprehensive end-to-end tests for single-repository workflow execution to validate the MVP functionality.

**Implementation Details:**
- Create test workflow scenarios using the existing E2E test framework
- Test basic workflow execution with inputs and outputs
- Verify step output passing and template variable resolution
- Test input validation (type conversion, enum validation, regex validation)
- Add error case testing (invalid inputs, template errors, step failures)
- Test workspace and state management
- Verify cleanup behavior for successful and failed workflows
- Test debug mode functionality
- Create test scenarios matching the design examples

**Acceptance Criteria:**
- [ ] E2E tests cover basic workflow execution
- [ ] Input validation test cases (success and failure scenarios)
- [ ] Step output passing verified through multiple steps
- [ ] Template variable resolution tested with various contexts
- [ ] Error handling and error message quality verified
- [ ] Workspace and state management tested
- [ ] Debug mode functionality validated
- [ ] Tests run reliably in CI environment

**Files to Modify:**
- `internal/e2e/exec_test.go` (new)
- Test fixture files for workflow scenarios

---

### Milestone 2: Containerization and Graph-Aware Execution

This milestone introduces the core security and isolation features, and expands execution to multiple repositories.

### Issue 6: `feat(engine): Introduce containerized step execution`

**Description:**
Modify the execution engine to run steps in secure, isolated containers with proper resource limits and security hardening.

**Implementation Details:**
- Add container runtime detection (Docker/Podman)
- Implement container execution with security hardening:
  - Non-root user (UID 1001)
  - Read-only root filesystem
  - Dropped capabilities (with optional `capabilities` block)
  - Default seccomp profile
  - No network access by default (with optional `network: default`)
- Add workspace mounting into containers
- Implement resource limits (CPU, memory) enforcement
- Add container image pull policy support
- Create container lifecycle management
- Support both containerized and host execution modes
- Add container cleanup after step completion
- Implement proper error handling for container failures

**Acceptance Criteria:**
- [ ] Steps execute in isolated containers by default
- [ ] Security hardening measures are properly applied
- [ ] Resource limits are enforced and prevent resource exhaustion
- [ ] Workspace is properly mounted and accessible to containers
- [ ] Container images are pulled according to configured policy
- [ ] Containers are cleaned up after step completion
- [ ] Host execution mode still works for non-containerized steps
- [ ] Clear error messages for container runtime issues

**Files to Modify:**
- `internal/engine/container.go` (new)
- `internal/engine/runner.go` (extend)
- `internal/engine/security.go` (new)

---

### Issue 7: `feat(engine): Implement graph-aware execution & planning`

**Description:**
Extend the execution engine to support multi-repository workflows with artifact-based dependency management and parallel execution.

**Implementation Details:**
- Extend existing graph building logic for artifact dependencies
- Implement artifact aggregation logic with configurable timeouts
- Add support for `on: artifact_update` workflows
- Create trigger context with `.trigger.artifact` and `.trigger.artifacts`
- Implement parallel repository execution with `--max-concurrent-repos`
- Add CEL evaluation for workflow and step-level `if` conditions
- Support artifact-based workflow triggering across repositories
- Implement failure policy for aggregation scenarios
- Add comprehensive logging for multi-repo execution flow
- Integrate with existing repository caching and checkout logic

**Acceptance Criteria:**
- [ ] Multi-repository workflows execute in topological order
- [ ] Artifact updates trigger downstream workflows correctly
- [ ] Artifact aggregation waits for all upstream workflows
- [ ] Parallel repository execution respects concurrency limits
- [ ] Trigger context provides correct artifact information
- [ ] CEL conditions work with trigger, inputs, and steps contexts
- [ ] Aggregation timeouts prevent indefinite waiting
- [ ] Clear error messages for dependency resolution issues

**Files to Modify:**
- `internal/engine/planner.go` (new)
- `internal/engine/artifacts.go` (new)
- `internal/graph/graph.go` (extend)
- `cmd/tako/internal/exec.go` (extend)

---

### Issue 8: `feat(engine): Implement 'tako/checkout@v1' semantic step`

**Description:**
Create the first built-in semantic step and establish the framework for additional built-in steps. Implement the `tako steps list` command.

**Implementation Details:**
- Create `internal/steps/` package for built-in steps
- Implement `tako/checkout@v1` step with `ref` parameter
- Add step registration and discovery mechanism
- Create `cmd/tako/internal/steps.go` for `tako steps list` command
- Integrate built-in steps with the execution engine
- Add proper parameter validation for built-in steps
- Support versioning for built-in steps (@v1, @v2, etc.)
- Add comprehensive documentation for built-in step parameters
- Create testing framework for built-in steps

**Acceptance Criteria:**
- [ ] `uses: tako/checkout@v1` works in workflow steps
- [ ] `ref` parameter properly checks out specified branch/tag/commit
- [ ] `tako steps list` displays available built-in steps and parameters
- [ ] Step versioning works correctly (@v1 syntax)
- [ ] Parameter validation provides clear error messages
- [ ] Built-in step framework supports future extensions
- [ ] Integration tests verify checkout functionality

**Files to Modify:**
- `internal/steps/checkout.go` (new)
- `internal/steps/registry.go` (new)
- `cmd/tako/internal/steps.go` (new)
- `internal/engine/runner.go` (extend)

---

### Issue 9: `test(e2e): Add E2E test for multi-repo fan-out/fan-in`

**Description:**
Create comprehensive end-to-end tests for multi-repository workflow execution to validate graph-aware execution and artifact aggregation.

**Implementation Details:**
- Implement the fan-out/fan-in test scenario from the design document
- Create test repositories with proper dependency relationships
- Test artifact production and consumption across repositories
- Verify parallel execution and aggregation behavior
- Test artifact aggregation timeouts and failure policies
- Add comprehensive logging verification
- Test trigger context population with multiple artifacts
- Verify proper cleanup of multi-repo test environments

**Acceptance Criteria:**
- [ ] Fan-out/fan-in scenario executes correctly
- [ ] Artifact dependencies trigger downstream workflows
- [ ] Parallel execution respects repository concurrency limits
- [ ] Artifact aggregation waits for all upstream completions
- [ ] Trigger context contains correct artifact information
- [ ] Test reliably reproduces the design scenario
- [ ] Performance is acceptable for typical multi-repo workflows

**Files to Modify:**
- `internal/e2e/multi_repo_test.go` (new)
- Test fixture repositories and configurations

---

### Milestone 3: Advanced Features & Use Cases

### Issue 10: `feat(engine): Implement 'tako/update-dependency@v1' semantic step`

**Description:**
Implement the update-dependency built-in step that automatically detects package managers and updates dependencies.

**Implementation Details:**
- Add package manager detection (go.mod, package.json, pom.xml, etc.)
- Implement dependency update logic for each supported ecosystem
- Add proper error handling for update failures
- Support `name` and `version` parameters
- Add validation for dependency names and version formats
- Integrate with existing ecosystem tooling
- Create comprehensive test coverage for different package managers

**Acceptance Criteria:**
- [ ] `uses: tako/update-dependency@v1` works with `name` and `version` parameters
- [ ] Automatic package manager detection functions correctly
- [ ] Dependency updates work for supported ecosystems (Go, Node.js, Maven)
- [ ] Clear error messages for unsupported package managers or invalid versions
- [ ] Step integrates properly with workflow execution

---

### Issue 11: `feat(engine): Implement 'tako/create-pull-request@v1' semantic step`

**Description:**
Implement the create-pull-request built-in step with retry logic for integration with code hosting platforms.

**Implementation Details:**
- Add support for creating pull requests via GitHub API
- Implement retry policy with exponential backoff
- Add authentication via secrets management system
- Support required parameters: title, body, base, head
- Add proper error handling and rate limit management
- Create integration tests with GitHub API

**Acceptance Criteria:**
- [ ] Pull request creation works with GitHub
- [ ] Retry policy handles transient failures
- [ ] Authentication works via secrets management
- [ ] All required parameters are properly validated

---

### Issue 12: `feat(engine): Implement step caching`

**Description:**
Implement content-addressable step caching to improve workflow execution performance.

**Implementation Details:**
- Create cache key generation based on step definition and repository content hash
- Implement cache storage at `~/.tako/cache`
- Add `cache_key_files` glob pattern support
- Support `--no-cache` flag for cache invalidation
- Add `tako cache clean` command
- Implement cache hit/miss logic and validation

**Acceptance Criteria:**
- [ ] Step results are cached and reused when appropriate
- [ ] Cache keys are generated correctly from step definition and file content
- [ ] `--no-cache` flag bypasses cache as expected
- [ ] Cache management commands work correctly

---

### Issue 13: `feat(engine): Implement asynchronous persistence and resume`

**Description:**
Implement long-running step support with container persistence and workflow resumption.

**Implementation Details:**
- Add `long_running: true` step support
- Implement container persistence after main process exit
- Create output capture via `.tako/outputs.json` file
- Add container lifecycle management and recovery
- Implement orphaned container cleanup (24-hour policy)
- Support system reboot recovery with container restart
- Add container labeling with run ID and timestamps

**Acceptance Criteria:**
- [ ] Long-running steps persist containers after main process exit
- [ ] Output capture works correctly via `.tako/outputs.json`
- [ ] Container recovery handles system reboots appropriately
- [ ] Orphaned container cleanup prevents resource leaks
- [ ] Workflow resumption works correctly with long-running steps

---

### Issue 14: `feat(exec): Implement --dry-run mode`

**Description:**
Implement dry-run functionality to show execution plans without making changes.

**Implementation Details:**
- Add execution plan generation and display
- Show workflow and step execution order
- Display template variable resolution without execution
- Add dependency graph visualization
- Support dry-run for both single and multi-repo workflows

**Acceptance Criteria:**
- [ ] `--dry-run` flag shows execution plan without changes
- [ ] Template variables are resolved and displayed
- [ ] Dependency execution order is clearly shown
- [ ] No side effects occur during dry-run mode

---

### Issue 15: `feat(cmd): Create 'tako status' command`

**Description:**
Implement the status command to check workflow execution status and long-running container status.

**Implementation Details:**
- Create `cmd/tako/internal/status.go` command
- Display workflow execution status from state files
- Show long-running container status and resource usage
- Add automatic cleanup of orphaned containers during status check
- Support status display for completed, failed, and in-progress workflows
- Add container labeling verification and cleanup

**Acceptance Criteria:**
- [ ] `tako status <run-id>` shows comprehensive workflow status
- [ ] Long-running container status is displayed correctly
- [ ] Orphaned containers are detected and cleaned up automatically
- [ ] Status information is clear and actionable for users

