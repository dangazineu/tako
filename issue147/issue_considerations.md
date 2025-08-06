# Design Evolution for Issue #147: Automated Java BOM Release

This document captures the detailed discussion and evolution of the real-world testing scenario for a fully automated Java Bill of Materials (BOM) release. It serves as a record of the design process, including initial ideas, feedback, and the final, refined proposal.

## The Goal

The core objective was to define a complex, real-world scenario that could rigorously test and demonstrate `tako`'s advanced fan-out and fan-in orchestration capabilities in a multi-repository environment.

---

### 1. Initial Proposal: Direct Release Cascade with Stateful Fan-In

My first proposal focused on a direct, event-driven cascade using a stateful aggregator for the fan-in.

**The Flow:**
1.  **Fan-Out:** `core-lib` releases and emits a `core_library_released` event.
2.  **Parallel Releases:** `lib-a` and `lib-b` are triggered, and their workflows immediately build and release a new version of themselves. Upon completion, they each emit a `library_released` event.
3.  **Stateful Fan-In:** The `java-bom` repository subscribes to these `library_released` events. Its workflow would use a state file (e.g., `release-state.json`) to track which libraries had reported in.
4.  **Final Release:** Once the state file showed that both `lib-a` and `lib-b` had released, a final step in the `java-bom` workflow would trigger to generate and publish the new BOM.

---

### 2. User Feedback 1: The Need for a Safer, Non-Releasing Workflow

You provided critical feedback on the initial proposal, pointing out a significant flaw in its real-world applicability:

> "I don't think that is the proper flow. lib-a and lib-b depend on core-lib, so they should update their own pom files when they are notified of the core-lib new version... We want to avoid bumping dependencies AND releasing in a single step."

**Key Insights from this Feedback:**
*   **Safety:** Automatically releasing a new version of a library just because a dependency was updated is unsafe. It bypasses crucial steps like code review and dedicated testing.
*   **Separation of Concerns:** The act of updating a dependency should be separate from the act of releasing a new version.

---

### 3. Refined Proposal: A PR-Based Workflow with Manual Intervention

Based on your feedback, I proposed a much safer, more realistic workflow that introduced a human-in-the-loop for the critical release steps.

**The Flow:**
1.  **Fan-Out PR Creation:** `core-lib`'s release triggers workflows in `lib-a` and `lib-b` that **only create Pull Requests** to update the dependency.
2.  **Human Review:** Developers would manually review, approve, and merge these PRs.
3.  **Manual Releases:** After merging, developers would manually trigger separate `release` workflows in `lib-a` and `lib-b`.
4.  **Manual BOM Update:** Finally, a developer would manually trigger a workflow in `java-bom` to create a PR for the BOM update.

---

### 4. User Feedback 2: The Goal of Full Automation

You refined the vision further, clarifying that the goal was to achieve a fully autonomous "lights-out" process, removing the manual steps but keeping the safety of the PR-based approach.

> "I don't want any human intervention throughout the process, unless an error happens... I want the tako process to include the logic to merge the Pull Requests, once they pass their checks, and I want it to have a step to trigger the release of lib-a and lib-c after the PRs are merged."

**Key Insights from this Feedback:**
*   **Full Automation is the Goal:** The ideal workflow should be end-to-end automated.
*   **CI Checks as the Gate:** The "human review" step can be replaced by an automated "wait for CI checks to pass" step.
*   **Orchestrate the Full Lifecycle:** `tako` should be responsible for the entire lifecycle: proposing the PR, monitoring it, merging it, and then triggering the next step in the chain (the release).

---

### 5. Final Proposal: The Fully Autonomous Orchestration

This led to the final, comprehensive scenario that was captured in issue #147. It combines the safety of a PR-based workflow with the power of full automation.

**The High-Level Flow:**
1.  **Fan-Out PR Creation:** `core-lib`'s release triggers workflows in `lib-a` and `lib-b`.
2.  **Autonomous PR Lifecycle:** Each library's workflow performs a complete, automated sequence:
    a.  Creates a PR for the dependency update.
    b.  Waits for the PR's CI checks to pass successfully.
    c.  Automatically merges the PR.
    d.  Triggers its own `release` workflow.
    e.  The `release` workflow publishes the new version and emits a `library_released` event.
3.  **Automated Stateful Fan-In:** The `java-bom` repository uses the stateful aggregator pattern to listen for the `library_released` events.
4.  **Autonomous BOM Release:** Once all required events are collected, the `java-bom` repository kicks off its own autonomous "create PR -> wait for checks -> merge -> release" cycle.

#### **Detailed Step-by-Step Workflow Implementation**

This section elaborates on *how* the autonomous lifecycle is broken down into distinct, chained steps within a `tako.yml` file, using `lib-a` as the primary example.

The `propose-and-release-update` workflow is the core of this automation and is composed of three separate steps:

**Step 1: `create-pr`**
*   **Responsibility:** To propose the dependency update as a Pull Request.
*   **Actions:**
    1.  Creates a new branch (e.g., `chore/update-core-lib-v1.2.0`).
    2.  Updates the `pom.xml` with the new dependency version.
    3.  Runs local verification (`mvn clean install`).
    4.  Commits the change and uses the `gh` CLI to create a PR.
*   **Key `tako` Feature:** Uses `produces: { outputs: { pr_number: from_stdout } }` to capture the new PR's number and pass it to the next step.

**Step 2: `wait-and-merge`**
*   **Responsibility:** To act as the automated gatekeeper, replacing manual review.
*   **Actions:**
    1.  Receives the PR number from the previous step's output (`{{ .steps.create-pr.outputs.pr_number }}`).
    2.  Executes a blocking command like `gh pr checks {{...}} --watch`, which polls until the PR's CI checks complete.
    3.  If checks are successful, it runs `gh pr merge {{...}} --squash` to merge the PR.
*   **Key `tako` Feature:** `tako`'s execution engine will wait for the blocking `gh` command to finish, effectively pausing the workflow until the CI gate is passed.

**Step 3: `trigger-release`**
*   **Responsibility:** To initiate the release of the library *after* the update has been safely merged.
*   **Actions:**
    1.  Calculates the next semantic version for `lib-a`.
    2.  Executes `tako exec release --inputs.version=...`, which chains to the separate, dedicated `release` workflow within the same `tako.yml`.
*   **Key `tako` Feature:** Demonstrates workflow chaining, allowing for the separation of the PR lifecycle logic from the release logic.

This explicit, multi-step process is what makes the automation robust and observable. Each step has a single, clear responsibility, and the entire process is orchestrated by `tako`.
