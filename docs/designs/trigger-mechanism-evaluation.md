# Design Evaluation: Workflow Triggering Mechanisms

This document evaluates three different approaches for triggering workflows in a multi-repository environment within `tako`.

1.  **Model A: Downstream-Driven Triggers (`on: artifact_update`)**: A fully declarative, event-driven model. Downstream repositories subscribe to artifact updates and decide for themselves whether to trigger a workflow.
2.  **Model B: Imperative Triggers (`uses: tako/trigger-workflow@v1`)**: A fully imperative model. An upstream workflow explicitly calls and triggers a specific workflow in a specific downstream repository.
3.  **Model C: Directed-Discovery Triggers**: A hybrid model. An upstream workflow uses a "smart" trigger step that discovers which repositories to run against based on produced artifacts and the `dependents` list, and then calls a conventionally-named workflow in each.

---

## Model A: Downstream-Driven Triggers (`on: artifact_update`)

In this model, the connection between repositories is declared, but the action is initiated by the downstream consumer.

-   **Upstream (`go-lib/tako.yml`)**: Declares its artifacts and lists potential dependents. It has no knowledge of what workflows the downstream repositories will run.
    ```yaml
    artifacts:
      go-lib:
        path: ./go.mod
    dependents:
      - repo: my-org/app-one
        artifacts: [go-lib]
    ```
-   **Downstream (`app-one/tako.yml`)**: Listens for updates to artifacts it cares about and triggers its own workflow.
    ```yaml
    workflows:
      test-integration:
        on: artifact_update
        if: trigger.artifact.name == 'go-lib'
        steps:
          - run: ./scripts/run-tests.sh --version {{ .trigger.artifact.outputs.version }}
    ```

### Pros

-   **Loose Coupling**: The upstream repository is completely decoupled from the downstream implementation.
-   **High Scalability**: Adding a new consumer requires zero changes to the upstream repository.
-   **Owner Autonomy**: Downstream owners have full control over how they react to an update.

### Cons

-   **"Silent Failure" Risk**: If a downstream owner forgets to add an `on: artifact_update` workflow, their integration tests are silently skipped. The upstream has no way to enforce this critical step. This can be perceived as "brittle" because the contract is implicit.
-   **Indirect Control**: The upstream `release` workflow cannot force a specific behavior on its consumers.

---

## Model B: Imperative Triggers (`uses: tako/trigger-workflow@v1`)

In this model, the upstream workflow is responsible for explicitly triggering a specific workflow in a specific repository.

-   **Upstream (`go-lib/tako.yml`)**: The `release` workflow contains an explicit step to trigger a workflow in a single, hardcoded repository.
    ```yaml
    workflows:
      release:
        on: exec
        steps:
          - id: create_release
            run: ./scripts/release.sh
            produces:
              outputs:
                version: from_stdout
          - id: trigger_downstream_test
            uses: tako/trigger-workflow@v1
            with:
              repo: my-org/app-one # Explicit repo
              workflow: test-integration # Explicit workflow name
              inputs:
                version: {{ .steps.create_release.outputs.version }}
    ```

### Pros

-   **Maximum Control**: The upstream author has precise, explicit control over every action.

### Cons

-   **Tightest Coupling**: The upstream is hardcoded to the downstream repo name, workflow name, and input schema.
-   **Lowest Scalability**: The upstream `tako.yml` must be modified for every new consumer, creating a significant bottleneck.
-   **Prevents Graph Optimization**: The engine cannot see the full dependency graph until execution time.

---

## Model C: Directed-Discovery Triggers (Hybrid)

In this model, the upstream workflow directs downstream repositories to run a specific, conventionally-named workflow, but it discovers *which* repositories to trigger based on dependencies.

-   **Upstream (`go-lib/tako.yml`)**: It defines the artifacts it produces and what other repos depend on them. Its workflow then has a "smart" trigger step.
    ```yaml
    artifacts:
      go-lib:
        path: ./go.mod
    dependents:
      - repo: my-org/app-one
        artifacts: [go-lib]
      - repo: my-org/app-two
        artifacts: [go-lib]
    workflows:
      release:
        # 'on: exec' is implicit for all workflows in this model.
        steps:
          - id: build_lib
            run: ./scripts/build.sh
            produces:
              artifact: go-lib # This step produces the 'go-lib' artifact
              outputs:
                version: from_stdout
          - id: trigger_downstream_tests
            # This "smart" step triggers the 'test-integration' workflow
            # in all repos that depend on artifacts produced in this run.
            uses: tako/trigger-workflow@v1
            with:
              # It triggers a CONVENTION-BASED workflow name
              workflow: test-integration
              # Inputs are implicitly passed from the artifact context.
              # The trigger step would map 'version' from the artifact
              # to the 'version' input in the downstream workflow.
    ```
-   **Downstream (`app-one/tako.yml`)**: It must provide a workflow with the conventionally-agreed-upon name.
    ```yaml
    workflows:
      test-integration: # Must match the name from the upstream 'with.workflow'
        # The 'on:' key is omitted; this workflow is executable by default.
        inputs:
          version:
            type: string
            required: true
        steps:
          - run: ./scripts/run-tests.sh --version {{ .inputs.version }}
    ```

**A Note on the `on:` Key:**
In this model, since all workflows are triggered directly (either by a user or the `tako/trigger-workflow@v1` step), they are all conceptually `on: exec`. Per our discussion, the `on:` key will be omitted from the schema for the initial implementation to reduce verbosity. All workflows will be considered executable by default. If future trigger types (e.g., `on: schedule`) are added, the `on:` key will be reintroduced at that time, with workflows lacking the key defaulting to `on: exec`.

### Pros

-   **Centralized Control**: The upstream workflow author can enforce that all dependents run a specific action (e.g., `test-integration`), preventing the "silent failure" scenario from Model A.
-   **Automated Dispatch**: The `dependents` list is used to automatically discover and loop over the correct repositories. The upstream author doesn't need to add a new step for each new consumer.
-   **Declarative Dependencies**: The `dependents` block still serves as a clear, machine-readable declaration of the repository graph, which the engine can use for planning and visualization.

### Cons

-   **Convention over Configuration**: This model's biggest drawback is that it imposes a strict naming convention on all consumers. Every downstream repository *must* have a workflow named `test-integration` that accepts a specific set of inputs. This creates a form of coupling at the convention level.
-   **Reduced Flexibility for Consumers**: A downstream owner can't easily rename their workflow or change its input schema without coordinating with all upstream callers. They lose autonomy.
-   **"Magic" Behavior**: The `trigger-workflow` step has a lot of implicit behavior. It automatically finds artifacts, looks up dependents, and maps inputs. This can be less transparent than the fully declarative `on: artifact_update` trigger.

## Comparative Analysis

| Criterion | Model A (Downstream-Driven) | Model B (Imperative) | Model C (Directed-Discovery) |
| :--- | :--- | :--- | :--- |
| **Coupling** | **Loose**. Upstream is unaware of downstream implementation. | **Very Tight**. Upstream knows repo, workflow, and inputs. | **Medium**. Coupled by workflow name convention. |
| **Scalability** | **High**. New consumers require no upstream changes. | **Very Low**. Upstream must be changed for every new consumer. | **High**. New consumers only need to be added to `dependents` list. |
| **Control** | **Decentralized**. Downstream has full control. | **Centralized**. Upstream has full, explicit control. | **Centralized**. Upstream directs the action to be taken. |
| **Brittleness** | Brittle to **inaction** (silent skips). | Brittle to **any change** (renaming, etc.). | Brittle to **convention breaks**. |
| **Winner** | | | |

## Recommendation

This analysis reveals a fundamental trade-off between **autonomy (Model A)** and **control (Model C)**.

-   **Model A (`on: artifact_update`)** is the most scalable and flexible. It treats repositories as independent services in an event-driven architecture. Its weakness is that the "contract" between services is implicit, and it cannot enforce behavior.

-   **Model C (Directed-Discovery)** provides a powerful way for an upstream "owner" to enforce a consistent process across all of its dependents. It solves the "silent failure" problem of Model A. Its weakness is that it imposes a rigid convention on all consumers, reducing their autonomy and creating coupling at the workflow-name level.

-   **Model B (Imperative)** is the least desirable, as it combines the tightest coupling with the lowest scalability.

**Conclusion:** The choice between Model A and Model C depends on the desired governance model for the ecosystem.

-   If the goal is to foster a loosely-coupled ecosystem where teams have high autonomy, **Model A** is superior.
-   If the goal is to have a centrally-managed process where an upstream repository can enforce standards and actions on its dependents, **Model C** is the better choice.

Given that `tako` is a tool for orchestration, providing the *option* for centralized control is a powerful feature. The "brittleness" of silent failures in Model A is a significant practical risk in many organizations.

Therefore, the **Directed-Discovery model (Model C) is a strong candidate for the primary mechanism.** It offers a pragmatic balance of automation and control. The design should proceed with this hybrid model. We can re-evaluate adding Model A's `on: artifact_update` as a secondary, opt-in mechanism in the future if a more event-driven approach is also required.

---

## Additional Alternative Approaches

Beyond the three core models evaluated above, several additional approaches warrant consideration for downstream workflow invocation. These alternatives address specific limitations of the primary models and offer different trade-offs in terms of flexibility, control, and complexity.

### Model D: Policy-Based Triggers (`policies` + `on: artifact_update`)

This model extends Model A by adding a centralized policy layer that can enforce organizational standards while maintaining downstream autonomy.

-   **Upstream (`go-lib/tako.yml`)**: Declares its artifacts and policies that downstream repositories must adhere to.
    ```yaml
    artifacts:
      go-lib:
        path: ./go.mod
    dependents:
      - repo: my-org/app-one
        artifacts: [go-lib]
    policies:
      required_workflows:
        - name: test-integration
          inputs: [version]
          timeout: "30m"
        - name: security-scan
          inputs: [version]
          required: false
    ```
-   **Downstream (`app-one/tako.yml`)**: Must implement required workflows but has flexibility in implementation.
    ```yaml
    workflows:
      test-integration:
        on: artifact_update
        if: trigger.artifact.name == 'go-lib'
        inputs:
          version:
            type: string
            required: true
        steps:
          - run: ./scripts/run-tests.sh --version {{ .inputs.version }}
      security-scan:
        on: artifact_update
        if: trigger.artifact.name == 'go-lib'
        inputs:
          version:
            type: string
            required: true
        steps:
          - run: ./scripts/security-scan.sh --version {{ .inputs.version }}
    ```

#### Pros
-   **Enforced Standards**: The upstream can require specific workflows to exist, preventing silent failures.
-   **Implementation Flexibility**: Downstream teams retain control over how they implement required workflows.
-   **Gradual Adoption**: Policies can be marked as optional and made required over time.
-   **Audit Capability**: The engine can verify compliance with required policies.

#### Cons
-   **Increased Complexity**: Requires policy validation and compliance checking.
-   **Partial Coupling**: Creates coupling through policy names and input schemas.
-   **Enforcement Burden**: Requires tooling to monitor and enforce policy compliance.

### Model E: Manifest-Driven Triggers (`manifest.yml` + Discovery)

This model uses a separate manifest file to describe cross-repository dependencies and trigger behaviors, decoupling orchestration concerns from individual repository configurations.

-   **Central Manifest (`org-workflows/tako-manifest.yml`)**: Maintained separately from individual repositories.
    ```yaml
    version: 0.2.0
    repositories:
      - name: my-org/go-lib
        artifacts:
          go-lib:
            path: ./go.mod
        workflows:
          release:
            triggers:
              - repos: [my-org/app-one, my-org/app-two]
                workflow: test-integration
                inputs:
                  version: "{{ .artifacts.go-lib.outputs.version }}"
              - repos: [my-org/release-bom]
                workflow: update-bom
                depends_on: [my-org/app-one, my-org/app-two]
                inputs:
                  versions: "{{ .collect_outputs('version') }}"
    ```
-   **Individual Repositories**: Contain only their local workflow definitions without cross-repository concerns.
    ```yaml
    # app-one/tako.yml - No cross-repo dependencies declared
    workflows:
      test-integration:
        inputs:
          version:
            type: string
            required: true
        steps:
          - run: ./scripts/run-tests.sh --version {{ .inputs.version }}
    ```

#### Pros
-   **Centralized Orchestration**: All cross-repository logic lives in one place, making it easier to understand and maintain.
-   **Clean Separation**: Individual repositories focus only on their own workflows.
-   **Flexible Routing**: Complex trigger patterns can be expressed without modifying individual repositories.
-   **Version Control**: The manifest can be versioned independently and reviewed by platform teams.

#### Cons
-   **Central Bottleneck**: All cross-repository changes require manifest updates.
-   **Discovery Complexity**: Requires mechanism to discover and load the manifest.
-   **Synchronization**: Risk of manifest and repository definitions getting out of sync.
-   **Ownership Model**: Unclear who owns and maintains the central manifest.

### Model F: Subscription-Based Triggers (`subscriptions` + Events)

This model implements a pub-sub pattern where downstream repositories subscribe to specific artifact events with filtering criteria.

-   **Upstream (`go-lib/tako.yml`)**: Simply declares its artifacts without knowledge of consumers.
    ```yaml
    artifacts:
      go-lib:
        path: ./go.mod
    workflows:
      release:
        steps:
          - id: build
            run: ./scripts/release.sh
            produces:
              artifact: go-lib
              outputs:
                version: from_stdout
              events:
                - type: artifact_published
                  payload:
                    version: "{{ .outputs.version }}"
                    commit_sha: "{{ .env.GITHUB_SHA }}"
    ```
-   **Downstream (`app-one/tako.yml`)**: Subscribes to specific events with complex filtering.
    ```yaml
    subscriptions:
      - artifact: my-org/go-lib:go-lib
        events: [artifact_published]
        filters:
          - semver.major(payload.version) >= 1
          - payload.commit_sha != ""
        workflow: test-integration
        inputs:
          version: "{{ .event.payload.version }}"
          commit_sha: "{{ .event.payload.commit_sha }}"
    workflows:
      test-integration:
        inputs:
          version:
            type: string
            required: true
          commit_sha:
            type: string
            required: true
        steps:
          - run: ./scripts/test.sh --version {{ .inputs.version }} --sha {{ .inputs.commit_sha }}
    ```

#### Pros
-   **Maximum Flexibility**: Complex filtering and routing logic using CEL expressions.
-   **Event-Driven Architecture**: Natural fit for asynchronous, loosely-coupled systems.
-   **Rich Context**: Events can carry arbitrary payload data beyond simple version numbers.
-   **Scalable**: Publishers don't need to know about subscribers.

#### Cons
-   **High Complexity**: Requires event system, filtering engine, and subscription management.
-   **Debugging Difficulty**: Event flows can be hard to trace and debug.
-   **Schema Evolution**: Event payload changes require careful versioning.
-   **Delivery Guarantees**: Need to handle event delivery failures and retries.

### Model G: Template-Based Triggers (`templates` + Code Generation)

This model uses template-driven code generation to create workflow definitions based on organizational patterns and repository metadata.

-   **Template Definition (`org-templates/dependency-update.yml.tmpl`)**: Organizational template for dependency update workflows.
    ```yaml
    # Template generates workflow based on repository metadata
    {{- $repo := .repository }}
    {{- $deps := .dependencies }}
    workflows:
      {{- range $deps }}
      test-after-{{ .name | kebab_case }}:
        on: artifact_update  
        if: trigger.artifact.name == '{{ .artifact_name }}'
        steps:
          - uses: tako/checkout@v1
          - uses: tako/update-dependency@v1
            with:
              name: {{ .name }}
              version: "{{ "{{ .trigger.artifact.outputs.version }}" }}"
          {{- if $repo.has_tests }}
          - run: {{ $repo.test_command }}
          {{- end }}
          {{- if $repo.has_security_scan }}
          - run: ./scripts/security-scan.sh
          {{- end }}
      {{- end }}
    ```
-   **Repository Metadata (`app-one/.tako/metadata.yml`)**: Describes repository capabilities and preferences.
    ```yaml
    repository:
      name: my-org/app-one
      has_tests: true
      has_security_scan: true
      test_command: "npm test"
    dependencies:
      - name: go-lib
        artifact_name: go-lib
        ecosystem: go
        update_strategy: auto
    ```

#### Pros
-   **Organization Standards**: Templates enforce consistent patterns across repositories.
-   **Customization**: Templates can adapt based on repository-specific metadata.
-   **DRY Principle**: Reduces duplication of common workflow patterns.
-   **Evolution**: Templates can be updated to roll out changes across all repositories.

#### Cons
-   **Template Complexity**: Complex template logic can be hard to understand and maintain.
-   **Generation Overhead**: Requires code generation step and tooling.
-   **Debugging**: Generated workflows may be hard to debug and understand.
-   **Metadata Maintenance**: Repository metadata must be kept accurate and up-to-date.

### Model H: Micro-Orchestration (`atomic` + Composition)

This model breaks workflows into atomic, composable units that can be dynamically orchestrated based on runtime conditions.

-   **Atomic Units (`go-lib/tako.yml`)**: Defines small, reusable workflow units.
    ```yaml
    atomic_units:
      build_library:
        inputs:
          version_bump:
            type: string
            default: patch
        steps:
          - run: ./scripts/build.sh --bump {{ .inputs.version_bump }}
            produces:
              artifact: go-lib
              outputs:
                version: from_stdout
      notify_downstream:
        inputs:
          version:
            type: string
            required: true
        steps:
          - uses: tako/trigger-units@v1
            with:
              repos: [my-org/app-one, my-org/app-two]
              units: [test_with_new_version]
              inputs:
                version: "{{ .inputs.version }}"
    compositions:
      release:
        sequence:
          - unit: build_library
            inputs:
              version_bump: "{{ .inputs.version_bump }}"
          - unit: notify_downstream
            inputs:
              version: "{{ .units.build_library.outputs.version }}"
    ```
-   **Downstream Units (`app-one/tako.yml`)**: Implements corresponding atomic units.
    ```yaml
    atomic_units:
      test_with_new_version:
        inputs:
          version:
            type: string
            required: true
        steps:
          - uses: tako/checkout@v1
          - uses: tako/update-dependency@v1
            with:
              version: "{{ .inputs.version }}"
          - run: ./scripts/test.sh
    ```

#### Pros
-   **Composability**: Small units can be combined in different ways for different use cases.
-   **Reusability**: Atomic units can be shared and reused across workflows.
-   **Dynamic Orchestration**: Compositions can be modified without changing atomic units.
-   **Testing**: Individual units are easier to test in isolation.

#### Cons
-   **Complexity**: Requires understanding of both atomic units and composition patterns.
-   **Indirection**: Multiple layers of abstraction can make workflows hard to follow.
-   **Coordination Overhead**: Managing dependencies between atomic units adds complexity.
-   **Versioning**: Atomic unit interfaces require careful versioning and compatibility management.

---

## Comprehensive Analysis & Recommendations

### Extended Comparative Matrix

| Criterion | Model A<br/>(Downstream-Driven) | Model B<br/>(Imperative) | Model C<br/>(Directed-Discovery) | Model D<br/>(Policy-Based) | Model E<br/>(Manifest-Driven) | Model F<br/>(Subscription-Based) | Model G<br/>(Template-Based) | Model H<br/>(Micro-Orchestration) |
|:---|:---|:---|:---|:---|:---|:---|:---|:---|
| **Coupling** | **Loose** | Very Tight | Medium | Medium | Medium-Loose | **Loose** | Medium | Medium |
| **Scalability** | **High** | Very Low | **High** | **High** | Medium | **High** | **High** | Medium |
| **Control** | Decentralized | **Centralized** | **Centralized** | Balanced | **Centralized** | Decentralized | **Centralized** | Balanced |
| **Complexity** | **Low** | **Low** | Medium | High | High | **Very High** | High | **Very High** |
| **Debugging** | **Easy** | **Easy** | Medium | Hard | Hard | **Very Hard** | Hard | **Very Hard** |
| **Standards Enforcement** | **Poor** | Good | **Excellent** | **Excellent** | **Excellent** | Poor | **Excellent** | Good |
| **Implementation Flexibility** | **Excellent** | Poor | Poor | **Excellent** | Good | **Excellent** | Good | Good |
| **Operational Overhead** | **Low** | **Low** | **Low** | High | Medium | High | Medium | High |
| **Evolution Support** | **Excellent** | Poor | Good | **Excellent** | Good | **Excellent** | **Excellent** | Good |

### Detailed Analysis by Use Case

#### Use Case 1: Small to Medium Organizations (< 50 repositories)

**Recommended Approach: Model C (Directed-Discovery) with Model A fallback**

For organizations with 10-50 repositories and moderate complexity, the hybrid approach provides the best balance:

- **Primary**: Model C for critical workflows where standards enforcement is essential
- **Fallback**: Model A for experimental or low-risk workflows where teams need maximum flexibility

**Rationale**:
- Model C prevents silent failures while maintaining reasonable complexity
- Convention-based workflow naming is manageable at this scale
- Teams can still customize implementation within established conventions
- Clear upgrade path to more sophisticated models as the organization grows

#### Use Case 2: Large Organizations (> 100 repositories)

**Recommended Approach: Model D (Policy-Based) with Model E (Manifest-Driven) for complex scenarios**

For large organizations with diverse teams and complex dependency graphs:

- **Primary**: Model D for enforcing organization-wide standards while preserving team autonomy
- **Complex Workflows**: Model E for sophisticated multi-repository orchestration patterns
- **Migration Path**: Start with Model C, evolve to Model D as organizational maturity increases

**Rationale**:
- Policy-based approach scales to large numbers of repositories
- Maintains team autonomy while ensuring compliance
- Manifest-driven approach handles complex, organization-specific workflow patterns
- Gradual adoption path reduces organizational risk

#### Use Case 3: Microservices Architecture

**Recommended Approach: Model F (Subscription-Based) with Model H (Micro-Orchestration) for complex workflows**

For organizations with event-driven architectures and sophisticated microservices patterns:

- **Event Integration**: Model F naturally aligns with existing pub-sub infrastructure
- **Complex Orchestration**: Model H for workflows that need dynamic composition
- **Infrastructure Investment**: Requires significant tooling and operational investment

**Rationale**:
- Aligns with existing microservices patterns and event-driven architecture
- Provides maximum flexibility for complex routing and filtering
- Enables sophisticated workflow composition patterns
- Requires mature DevOps/platform engineering capabilities

#### Use Case 4: Open Source Projects

**Recommended Approach: Model A (Downstream-Driven) with Model G (Template-Based) for consistency**

For open source ecosystems where contributor autonomy is paramount:

- **Primary**: Model A for maximum contributor freedom and low barriers to entry
- **Consistency**: Model G for providing recommended patterns without enforcement
- **Community Standards**: Templates maintained by core maintainers, adopted voluntarily

**Rationale**:
- Preserves the autonomy that open source contributors expect
- Templates provide consistency without mandatory enforcement
- Low operational overhead for maintainer teams
- Easy for new contributors to understand and adopt

### Strategic Implementation Recommendations

#### Phase 1: Foundation (Months 1-3)
**Implement Model C (Directed-Discovery)**
- Provides immediate value with reasonable complexity
- Establishes core workflow engine capabilities
- Creates foundation for more advanced models
- Validates core assumptions with real usage

#### Phase 2: Enhancement (Months 4-6)
**Add Model A (Downstream-Driven) as optional mode**
- Provide `on: artifact_update` as alternative to directed triggers
- Allow mixed usage patterns within same organization
- Gather feedback on which approach works better for different teams
- Refine tooling based on production usage

#### Phase 3: Scale & Sophistication (Months 7-12)
**Based on adoption feedback, implement one advanced model**
- **If enforcement is key concern**: Implement Model D (Policy-Based)
- **If complexity management is key**: Implement Model E (Manifest-Driven)
- **If event-driven architecture is mature**: Implement Model F (Subscription-Based)

### Implementation Complexity Assessment

#### Low Complexity (Suitable for MVP)
- **Model A**: Requires only event system and CEL evaluation
- **Model B**: Simple imperative calling, but limited scalability
- **Model C**: Moderate complexity, good MVP candidate

#### Medium Complexity (Second iteration)
- **Model D**: Requires policy validation and compliance checking
- **Model E**: Needs manifest discovery and synchronization
- **Model G**: Requires template engine and code generation

#### High Complexity (Future iterations)
- **Model F**: Full event/subscription system with delivery guarantees
- **Model H**: Complex orchestration engine with atomic unit management

### Risk Assessment & Mitigation

#### Model C Risks (Primary Recommendation)
**Risk**: Convention lock-in reduces future flexibility
**Mitigation**: Design conventions to be extensible, provide escape hatches to Model A

**Risk**: Debugging "magic" behavior in directed discovery
**Mitigation**: Excellent observability, clear logging, dry-run mode

#### Model A Risks (Fallback Recommendation)
**Risk**: Silent failures due to missing downstream workflows
**Mitigation**: Validation tooling, organizational monitoring, clear documentation

**Risk**: Inconsistent implementation across teams
**Mitigation**: Templates, best practices documentation, periodic audits

### Final Recommendation

**For Tako v0.2.0 Implementation:**

1. **Primary**: Implement Model C (Directed-Discovery) as designed
2. **Secondary**: Plan Model A (Downstream-Driven) for v0.3.0
3. **Future**: Evaluate advanced models (D, E, F) based on user feedback and organizational needs

This approach provides:
- **Immediate value** with reasonable implementation complexity
- **Clear upgrade path** to more sophisticated approaches
- **Validation opportunity** for core assumptions before adding complexity
- **Flexibility** to adapt based on real-world usage patterns

The key insight is that different organizations will need different approaches, and Tako should evolve to support multiple models rather than forcing a single approach on all users.
