# Issue #147: test(e2e): Add fully automated scenario for Java BOM release with fan-in

**Status:** OPEN  
**Created:** July 31, 2025  
**Updated:** July 31, 2025  
**Author:** Dan Gazineu (@dangazineu)  
**Milestone:** Milestone 3: MVP - Local, Synchronous Execution  

---

## Summary

This issue proposes the creation of a new, complex, end-to-end test that demonstrates a fully automated, real-world scenario: the orchestrated release of a Java Bill of Materials (BOM) following an update to a core library. This test will serve as a benchmark for `tako`'s advanced orchestration capabilities, including fan-out, fan-in aggregation, and automated PR management, with zero human intervention required unless an error occurs.

## Scenario: Fully Automated BOM Release for a Java Library Ecosystem

**The Goal:** When a new version of a foundational `core-lib` is released, `tako` will orchestrate a cascade of automated actions: creating dependency-update PRs in downstream libraries, waiting for their CI checks to pass, merging them, triggering their releases, and finally, once all dependent libraries are released, creating and releasing an updated BOM that includes all the new versions.

---

## **The Repositories Involved**

1.  **`core-lib` (The Initial Publisher):** The foundational Java library.
2.  **`lib-a` (Consumer & Publisher):** A library that depends on `core-lib`.
3.  **`lib-b` (Consumer & Publisher):** Another library that depends on `core-lib`.
4.  **`java-bom` (The Aggregator / Final Publisher):** Manages and releases the final BOM.

---

## **The Fully Automated Orchestration Flow**

1.  **Phase 1: Initial Fan-Out (PR Creation)**
    *   `core-lib` is released and emits a `core_library_released` event.
    *   `lib-a` and `lib-b` subscribe to this event and trigger their `propose-and-release-update` workflows in parallel.

2.  **Phase 2: The "Propose, Wait, Merge, Release" Cycle (in `lib-a` and `lib-b`)**
    *   Each library's workflow autonomously creates a PR for the dependency update.
    *   It then waits for the PR's CI checks to complete successfully.
    *   Once checks pass, it automatically merges the PR.
    *   After the merge, it triggers its own `release` workflow, which publishes the new version and emits a `library_released` event.

3.  **Phase 3: The Automated Fan-In and BOM Release**
    *   The `java-bom` repository subscribes to the `library_released` events from `lib-a` and `lib-b`.
    *   Its workflow uses a state file to track the arrival of these events.
    *   Once all required release events are collected, it autonomously creates, waits for, merges, and releases a final PR for the updated BOM.

---

## **The `tako.yml` Configurations**

*   **`core-lib/tako.yml`**:
    ```yaml
    version: v1
    workflows:
      release:
        inputs: { version: { type: string, required: true } }
        steps:
          - id: publish-jar
            run: "mvn deploy"
          - id: fan-out-core-release
            uses: tako/fan-out@v1
            with:
              event_type: "core_library_released"
              payload: { version: "{{ .Inputs.version }}" }
    ```
*   **`lib-a/tako.yml`** (and `lib-b` is similar):
    ```yaml
    version: v1
    subscriptions:
      - artifact: "acme-corp/core-lib:main"
        events: ["core_library_released"]
        workflow: "propose-and-release-update"
        inputs: { core_version: "{{ .event.payload.version }}" }
    workflows:
      propose-and-release-update:
        inputs: { core_version: { type: string } }
        steps:
          - id: create-pr
            run: |
              # Creates branch, updates pom, runs tests, creates PR, outputs PR number
              PR_URL=$(gh pr create --title "chore: Update core-lib" --body "Automated update.")
              echo $PR_URL | awk -F'/' '{print $NF}'
            produces: { outputs: { pr_number: from_stdout } }
          - id: wait-and-merge
            run: |
              gh pr checks {{ .steps.create-pr.outputs.pr_number }} --watch
              gh pr merge {{ .steps.create-pr.outputs.pr_number }} --squash --delete-branch
          - id: trigger-release
            run: "tako exec release --inputs.version=$(semver -i patch $(git describe --tags --abbrev=0))"
      release:
        inputs: { version: { type: string, required: true } }
        steps:
          - run: "mvn deploy"
          - uses: tako/fan-out@v1
            with:
              event_type: "library_released"
              payload: { library_name: "lib-a", new_version: "{{ .Inputs.version }}" }
    ```
*   **`java-bom/tako.yml`**:
    ```yaml
    version: v1
    subscriptions:
      - artifact: "acme-corp/lib-a:main"
        events: ["library_released"]
        workflow: "aggregate-and-release-bom"
      - artifact: "acme-corp/lib-b:main"
        events: ["library_released"]
        workflow: "aggregate-and-release-bom"
    workflows:
      aggregate-and-release-bom:
        steps:
          - id: update-state
            run: "jq '. * {\"{{ .event.payload.library_name }}\": \"{{ .event.payload.new_version }}\"}' release-state.json > state.tmp && mv state.tmp release-state.json"
          - id: check-and-trigger
            run: |
              if jq -e 'has("lib-a") and has("lib-b")' release-state.json; then
                tako exec create-bom-pr
              fi
      create-bom-pr:
        steps:
          - id: create-pr
            run: |
              # Creates branch, updates BOM from state file, creates PR, outputs PR number
              PR_URL=$(gh pr create --title "chore: Update BOM" --body "Automated BOM update.")
              echo $PR_URL | awk -F'/' '{print $NF}'
            produces: { outputs: { pr_number: from_stdout } }
          - id: wait-and-merge
            run: |
              gh pr checks {{ .steps.create-pr.outputs.pr_number }} --watch
              gh pr merge {{ .steps.create-pr.outputs.pr_number }} --squash --delete-branch
          - id: trigger-release
            run: "tako exec release-bom --inputs.version=$(semver -i patch $(git describe --tags --abbrev=0))"
      release-bom:
        inputs: { version: { type: string, required: true } }
        steps:
          - run: "mvn deploy"
    ```

---

## **Test Strategy & Acceptance Criteria**

-   **E2E Test Implementation:** This scenario must be implemented as an automated E2E test using `takotest`.
-   **Mocking:** All external interactions (`mvn deploy`, `gh pr`, `semver`) must be replaced with mock scripts that create verifiable file-based side effects. For example, `gh pr merge` will be a script that creates a file like `merged_pr_123.txt`.
-   **Verification:** The test must programmatically assert the existence of these files at each stage to verify that the entire flow executed correctly and in the right order.
-   **Features Under Test:**
    -   Multi-level fan-out.
    -   Workflow chaining (`tako exec` from within a workflow).
    -   Stateful fan-in logic using a workspace file.
    -   Capturing step outputs (`produces`) and using them in subsequent steps.
    -   Execution of complex shell scripts involving external tools (`gh`, `jq`).
-   The test must pass reliably in the CI pipeline.