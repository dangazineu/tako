# Issue #147 REVISED: Complete Java BOM E2E Test with Directed+Event-Driven Orchestration

## Key Aspect About Tako
|   To properly orchestrate the release of a dependency graph, we need a hybrid system with both directed                                          |
│   orchestration control and event-driven flexibility. The directed orchestration comes in form of dependency declaration in a tako file, the     │
│   event-driven comes from mapping subscribers to workflows. When we trigger a workflow named "release" on a node of the graph (it could be any   │
│   name, it's just a string), it will run every step in that named workflow. If one of the steps emits an event, then only its subscribers react  │
│   to iit, launching new workflows at the children nodes that need to complete (DFS) before the parent workflow continues. Let's say the even     │
│   was "library_released" and the reaction of the child nodes was to update their dependency version. When they all finish doing that, the        │
│   parent resumes its workflow. The parent node's release step is only completed when all its subscribers reaction workflows (and any workflows   │
│   cascading from those events) complete. When all steps in the "release" workflow of a node complete, then the same workflow name is triggered   │
│   in its immediate children, and so on. Tako needs to resolve the entire graph ahead of time to account for fan-in situations, and prevent       │
│   workflow from running twice on the completion of the workflow for multiple parents of the same node. It also needs the ability to queue        │
│   events at a given node, to avoid concurrent runs of the same workflow on the same node, when reacting to the same event type from different    │
│   parents.

## Analysis Summary

After implementing Issue #147, several critical discrepancies were discovered between the original requirements, implementation plan, and actual implementation. Most significantly, **PR #117 accidentally deprecated the "dependents" field functionality** that is essential for Tako's hybrid directed+event-driven orchestration architecture. This revised issue addresses these conflicts and provides a clearer, more detailed specification that includes **restoring the lost dependents functionality**.

## CRITICAL: Missing Dependents Functionality - RESTORED ✅

**Root Cause:** PR #119 (not #117) removed the `dependents` field from `tako.yml` configurations, breaking Tako's intended hybrid directed+event-driven orchestration model.

**Missing Functionality - NOW RESTORED:**
- ✅ `dependents` field in `Config` struct for declaring downstream repositories  
- ✅ `Dependent` struct with `Repo`, `Artifacts`, and `Workflows` fields
- ✅ Validation functions for `validateRepoFormat` and `validateArtifacts`
- ⚠️ Directed workflow triggering (`release` workflow propagates to dependents) - STILL NEEDS IMPLEMENTATION
- ⚠️ Graph resolution for fan-in scenarios and duplicate prevention - STILL NEEDS IMPLEMENTATION  
- ⚠️ Event queuing to prevent concurrent workflow runs - STILL NEEDS IMPLEMENTATION

**Restoration Details:**
PR #119 (`feat(cmd): implement tako exec with event-driven support`) removed the dependents functionality. The restoration included:
1. Added `Dependents []Dependent` field back to the `Config` struct
2. Restored the `Dependent` struct definition with proper YAML tags
3. Added back the validation helper functions `validateRepoFormat` and `validateArtifacts`
4. Ensured all unit tests continue to pass

**Current Status:** The data structures and validation are restored, but the actual orchestration logic that uses dependents for workflow triggering still needs to be implemented. The current implementation only uses subscriptions for event-driven orchestration.

## Investigation Timeline and Findings

**PR Investigation Results:**
- **PR #117:** `feat: implement event-driven workflow engine (issue #99)` - Actually KEPT the dependents field with a comment "Legacy field, still required for graph functionality"
- **PR #119:** `feat(cmd): implement tako exec with event-driven support` - This PR actually REMOVED the dependents functionality entirely
- **Subsequent PRs:** Built orchestration features around subscriptions only, unaware that directed orchestration via dependents was intended

**Architecture Evolution:**
1. **Pre-PR #117:** No event system, relied on basic dependents model
2. **PR #117:** Added subscriptions while keeping dependents as "legacy" 
3. **PR #119:** Removed dependents entirely, assuming subscriptions replaced them completely
4. **Post-PR #119:** All orchestration became purely event-driven, losing directed capability

**Key Finding:** The removal was not intentional deprecation but rather a misunderstanding of the intended hybrid architecture. The original design called for both directed (dependents) AND event-driven (subscriptions) orchestration to work together.

## Key Questions Identified During Implementation

### 1. **ARCHITECTURE: Restore Hybrid Directed+Event-Driven Model**

**Current Problem:** Only event-driven subscriptions exist; no directed orchestration capability

**Required Architecture:** 
1. **Directed Orchestration**: Dependencies declared via `dependents` field
2. **Event-Driven Reactions**: Subscriptions for cross-cutting concerns (like dependency updates)
3. **Hybrid Flow**: Workflows trigger dependents AND emit events that subscribers react to
4. **Synchronization**: Parent workflows wait for ALL subscriber reactions AND dependent completions

**Implementation Status:**
- ✅ Restore `dependents` field to `Config` struct - COMPLETED
- ⚠️ Implement directed workflow triggering to dependents - STILL NEEDED
- ⚠️ Add graph resolution to handle fan-in and prevent duplicate runs - STILL NEEDED
- ⚠️ Add event queuing for concurrent event management - STILL NEEDED
- ⚠️ Ensure parent workflows wait for both dependents AND subscribers before completing - STILL NEEDED

### 2. **Event Flow: Direct Triggering vs Event Subscriptions**

**Question:** Should the test exercise the event subscription mechanism or bypass it?

**Original Issue:** Emphasized testing "fan-out, fan-in aggregation" through event subscriptions
**Current Implementation:** Orchestrator directly triggers workflows instead of using event subscriptions

**Conflict:** The current E2E test doesn't actually test the event-driven subscriptions defined in the repository configurations. The `subscriptions` blocks in `lib-a`, `lib-b`, and `java-bom` are not exercised.

**Resolution Needed:** Choose one approach:
- **Option A:** Pure event-driven test that triggers `core-lib` and waits for cascade completion
- **Option B:** Orchestrator that triggers `core-lib` AND waits for event-driven cascade to complete
- **Option C:** Two separate tests - one for pure choreography, one for orchestrated control

A: We need all these tests implemented separately, as indicated here. But when they are all passing, we need the e2e real world scenario described in issue 147 implemented as a single test case.

### 3. **Repository Naming and Structure** 

**Question:** What should the repository naming convention be?

**Original Issue:** `acme-corp/core-lib`, `acme-corp/lib-a`, etc.
**Current Implementation:** `tako-test/java-bom-fanout-java-bom-fanout-core-lib`, etc.

**Impact:** The mangled naming makes configuration more complex and differs from real-world scenarios.

**Resolution Needed:** Standardize on a clear naming convention that balances test isolation with readability.
A: Tests should always use "tako-test" as the test owner. `takotest` CLI should always use "tako-test" as the default owner, but accept it to be overridden. Main code should never hardcode the owner name. Repository names for tests, should be derived from the name of the test case and test environment to avoid conflicting with other test repos. For this specific test, the names used look good. Please record these instructions in the README file so we remember it.

### 4. **State Management and Fan-In Logic**

**Question:** How should the fan-in coordination work in practice?

**Original Issue:** Used `release-state.json` with `jq` commands
**Current Implementation:** Uses `tako.state.json` with `jq` (✅ correctly implemented)
**But:** The orchestrator simulation bypasses testing this logic

**Resolution Needed:** Ensure the E2E test actually exercises the stateful fan-in logic in `java-bom`, not just verifies simulated artifacts.

A: All these workflows will be executed by a single instance of `tako`. Fan-in is a multithreading problem that should be resolved in the tako codebase without relying on any additional artifacts.  Please elaborate on how you plan to resolve that.

### 5. **PR Lifecycle Testing**

**Question:** What level of PR automation should be tested?

**Original Issue:** Full automation including "wait for CI checks" and "merge"
**Current Implementation:** Correctly implements full PR lifecycle with mocks
**But:** Only tested if orchestrator actually triggers the workflows (currently simulated)

**Resolution Needed:** Ensure the full PR lifecycle is actually executed and tested, not just simulated.

A: When running in local mode, PR creation and merging are mocked. When running e2e test case in remote mode, PR creation should actually create a PR. PR merging should be done by the test case, not the workflow. From tako's perspective, it will open the PR and exit the long running step. Then it will be run again, verify that the PR has been merged, and proceed to the next step or workflow completion.

### 6. **Verification Strategy**

**Question:** What constitutes successful completion of the release train?

**Current Issue:** Test verifies orchestrator-created simulation artifacts
**Should Verify:** Actual artifacts created by each repository in the chain

**Specific Verification Needed:**
- Real `published_core-lib_X.Y.Z.txt` from core-lib release
- Real `published_lib-a_X.Y.Z.txt` from lib-a release  
- Real `published_lib-b_X.Y.Z.txt` from lib-b release
- Real `published_java-bom_X.Y.Z.txt` from java-bom release
- Real `tako.state.json` showing both libraries released
- Evidence of actual PR creation, CI simulation, and merge

A: Yes, we need to verify all expected artifacts. `tako.state.json` should not exist, it is a hack and I expect your solution does not rely on it and eliminates it from the codebase. Please elaborate on how you plan on doing that.

## Recommended Implementation Approach

### Phase 1: Fix Orchestrator Implementation
- Remove simulation steps from `orchestrator/tako.yml`
- Implement real polling mechanism to wait for `java-bom` completion
- Add timeout and error handling for failed workflows
- Ensure orchestrator verifies actual artifacts, not simulated ones

### Phase 2: Test Event-Driven Flow
- Create test that exercises pure event subscriptions (without orchestrator)
- Verify that `core-lib` release actually triggers `lib-a` and `lib-b` via events
- Verify that `lib-a` and `lib-b` releases actually trigger `java-bom` via events

### Phase 3: Enhanced Verification
- Update verification script to check artifacts from all repositories
- Implement pattern matching for dynamic version numbers
- Add verification of state files and mock PR artifacts
- Ensure test fails properly when orchestration is incomplete

### Phase 4: Thoroughly review current tests to ensure they match the expected usage patterns
- Find and resolve duplications
- Find and resolve missing or incorrect assertions

## Success Criteria

### Must Test Successfully:
1. **Event-Driven Fan-Out:** `core-lib` release → automatic `lib-a` and `lib-b` triggering
2. **PR Automation:** Full create → wait → merge cycle for each repository
3. **Stateful Fan-In:** `java-bom` waits for both libraries before proceeding  
4. **State Management:** `tako.state.json` correctly tracks library releases
5. **Version Propagation:** Versions flow correctly through the entire chain
6. **Error Handling:** Failed CI or timeouts are handled gracefully

### Must Verify Existence Of:
- Real published artifacts from each repository
- State files showing proper coordination
- Mock PR lifecycle evidence
- Timing logs showing proper sequencing

## Testing Strategy

The E2E test should support both modes:
1. **Direct Mode:** Test pure event choreography by triggering `core-lib` directly
2. **Orchestrated Mode:** Test centralized control by triggering the orchestrator

Both modes should verify the same end state but exercise different control patterns.

## Key Requirements Clarifications

### Repository Structure
```
├── core-lib/               # Initial publisher
├── lib-a/                  # Consumer & publisher  
├── lib-b/                  # Consumer & publisher
├── java-bom/               # Final aggregator
└── orchestrator/           # Optional centralized control
```

### Event Flow
```
core-lib release → library_released event → [lib-a, lib-b] → library_released events → java-bom → BOM release
```

### Orchestrator Flow  
```
orchestrator → core-lib → [monitors for completion] → verification
```

### Artifacts Verified
- Published JAR simulation files from each repository
- State files showing coordination 
- Mock GitHub PR evidence files
- Version consistency across all components
- Timing and sequencing logs

This revised specification addresses the implementation challenges discovered and provides clear guidance for completing a robust, comprehensive E2E test of Tako's advanced orchestration capabilities.

---

## Q&A: Remaining Questions for Implementation

### Q1: Hybrid Architecture Implementation Priority
Given that we've restored the `dependents` data structures, should we:
- **Option A:** First implement the directed orchestration logic (using dependents) and then enhance the existing event-driven system?
- **Option B:** Fix the current event-driven test to work properly first, then add directed orchestration as a separate test?
- **Option C:** Implement both simultaneously as the true "hybrid" model from the start?

**Context:** The current Java BOM E2E test is broken because it simulates instead of actually orchestrating. We could fix it to use pure event-driven orchestration OR implement the missing directed orchestration.

A: Add an implementation phase where you exclude the java-bom test (and any other tests added after PR #141), and focus on fixing the orchestration logic. Most e2e tests, for example, should work with orchestration and not rely on subscriptions at all. After that phase is done, and all tests pass, then add back the subscription-based tests, and fix them. And then continue with the plan to complete the issue 147.

### Q2: Event Flow Testing Approach
For the event subscription testing (Question #2 in the doc):
- Should the test verify that the actual event subscriptions in `lib-a/tako.yml`, `lib-b/tako.yml`, and `java-bom/tako.yml` are working correctly?
- Or is it acceptable to have the orchestrator bypass the event system and directly trigger workflows?

**Context:** Currently the orchestrator calls `tako exec` directly instead of emitting events that trigger subscriptions. The subscriptions defined in the repository configs are never actually tested.

A: Answered above. 

### Q3: Orchestrator vs Pure Event-Driven Testing
Given your note about creating "two separate tests - one for pure choreography, one for orchestrated control":
- Should we keep the current orchestrator-based test and CREATE a second pure event-driven test?
- Or should we REPLACE the orchestrator test with a pure event-driven one?
- If keeping both, what should each test focus on specifically?
A: Create a new one and keep the one we have. Work with gemini to outline the best test plan.

### Q4: Dependents Configuration Usage
Now that we've restored the `dependents` field:
- Should the Java BOM E2E test repositories include `dependents` sections in their `tako.yml` files? A: yes
- How should `dependents` and `subscriptions` coexist in the same repository config? A: Dependents are the repositories that depend on a repo. Subscriptions are the subscriptions that dependents have on events emitted by their parents. It's a doubly linked list, where `dependents` is the edge in one direction, and `subscriptions` is the edge in the other direction. 
- Should repositories have BOTH dependents declarations AND subscription blocks? A: Yes, if they have parents, they can have subscriptions to them, and if they have children, they must declare them as dependents.

**Example:** Should `core-lib/tako.yml` have:
A: Please elaborate this again based on the info above.

### Q5: Implementation Order and Scope
For this specific Issue #147:
- Should we implement the full hybrid directed+event-driven orchestration system?
- Or should we focus on making the E2E test work with the current event-driven system?
- Is the goal to test Tako's orchestration capabilities, or to implement missing orchestration features?

### Q6: README Documentation Requirements
You mentioned recording the repository naming instructions in the README. Should I also document:
- The hybrid architecture design (dependents + subscriptions)?
- The difference between directed orchestration and event-driven orchestration?
- How to configure repositories to use both models?
A: Yes to all of these. In fact, you should completelly rewrite the README file based on the final state of this repo after we complete this task.

---

## REVISED IMPLEMENTATION PLAN

Based on your answers, here's the comprehensive approach:

### Phase 0: Infrastructure Fixes (PRIORITY)
**Exclude java-bom test and focus on core orchestration logic**
- Temporarily disable `java-bom-fanout` E2E test 
- Implement missing directed orchestration logic using `dependents` field
- Ensure existing E2E tests work with orchestration (not subscriptions)
- **Key Goal:** Make Tako actually orchestrate workflows across dependency graphs

### Phase 1: Hybrid Architecture Implementation
**Implement both directed orchestration AND event-driven subscriptions**
- Directed orchestration: When `tako exec release` runs on a node, it triggers the same workflow on immediate children
- Event-driven reactions: When a workflow step emits an event, subscribers launch reaction workflows
- Parent workflows wait for BOTH dependent completion AND subscriber reaction completion
- Implement DFS graph resolution with fan-in handling and duplicate prevention
- Implement event queuing to prevent concurrent workflow runs on same node

### Phase 2: Fix Subscription-Based Tests  
**Re-enable and fix java-bom test with proper orchestration**
- Remove simulation steps, implement real orchestration
- Eliminate `tako.state.json` hack - handle fan-in through multithreading in Tako core
- Verify actual artifacts from each repository, not simulated ones

### Phase 3: Comprehensive Test Coverage
**Multiple test scenarios as you specified:**
- **Test A:** Pure event-driven choreography (trigger core-lib, wait for cascade)  
- **Test B:** Orchestrator with event-driven cascade (orchestrator triggers core-lib AND waits)
- **Test C:** Real-world Issue #147 scenario (comprehensive Java BOM release train)

### Phase 4: PR Lifecycle Handling
**Implement proper PR workflow handling**
- Local mode: Mock PR creation/merging
- Remote mode: Actually create PRs, test merges them (not workflows)  
- Workflows open PRs then exit long-running steps
- When resumed, workflows verify PR merged and continue

### Phase 5: Documentation
**Complete README rewrite covering:**
- Hybrid architecture (dependents + subscriptions as doubly-linked list)
- Repository naming conventions for tests
- Configuration examples for both models
- How directed orchestration and event-driven orchestration work together

## ADDITIONAL CLARIFYING QUESTIONS

### Q1: Fan-In Resolution Strategy
You mentioned "fan-in is a multithreading problem that should be resolved in the tako codebase." Currently, the java-bom repository uses `jq` commands to check if both lib-a and lib-b have released before proceeding.

**How should fan-in work instead?**
- Should Tako's orchestration engine track which dependencies have completed and automatically trigger java-bom when both lib-a and lib-b finish?
- Should there be a built-in fan-in mechanism that repositories can declare (e.g., "wait for N dependencies before proceeding")?
- Or should repositories still handle their own coordination logic, but without external state files?

A: The current implementation is completely wrong, and it leaves fan-in responsibility to be implemented inside the workflow step. Tako is a tool to be used by customers. They expect Tako to perform the complex operations like fan-out and fan-in. The workflow itself, to be written by customers, should only focus on business logic, not on workflow mechanics. Tako is where the logic to match events with subscribers should live, as well as the logic to determine which steps are ready to be executed, which need to wait, when to fan out, and when to wait for a fan-in. The whole orchestration logic should live in the tako codebase, not in the customer's workflow. 

### Q2: Dependents vs Subscriptions Configuration  
You described them as a "doubly linked list" where dependents point forward and subscriptions point backward. 

**For the Java BOM example, should the configuration be:**
```yaml
# core-lib/tako.yml
dependents:
  - repo: "tako-test/lib-a:main"
  - repo: "tako-test/lib-b:main"

# lib-a/tako.yml  
subscriptions:
  - artifact: "tako-test/core-lib:main"
    events: ["library_released"]
    workflow: "propose-and-release-update"
dependents:
  - repo: "tako-test/java-bom:main" 

# java-bom/tako.yml
subscriptions:
  - artifact: "tako-test/lib-a:main"
    events: ["library_released"] 
    workflow: "update-bom"
  - artifact: "tako-test/lib-b:main"
    events: ["library_released"]
    workflow: "update-bom"
```

A: something like that. I removed the `workflow` field from dependents, as that makes no sense. A dependent is a dependent of the repo, not of a single workflow. Every workflow executed at a repo is going to be triggered at every dependent. 

### Q3: Event vs Workflow Triggering
From your "Key Aspect About Tako" description: when a workflow completes, it triggers the same workflow name in immediate children. When a workflow step emits an event, only subscribers react.

**For the Java BOM scenario:**
- Should `core-lib` release workflow trigger `propose-and-release-update` workflow in lib-a/lib-b via dependents? A: yes, but we should name it `release`
- Should the `core-lib` workflow ALSO emit `library_released` event for subscribers? A: yes, the event can have the same name, the values in the event will be different.
- Or should lib-a/lib-b only be triggered via subscriptions to events, not direct workflow triggering? A: No, the `release` workflow on core-lib first executes a step where the artifact is built and released (you need to implement that). The completion of this step will emit the event `library_released`, which immediately matches subscriptions on `lib-a` and `lib-b`. Each of these repos will trigger their own `update_dependency` workflows, as a reaction to the `library_released` event. When their workflow is completed, then we return to the `core-lib` and let it continue its workflow. It may or may not have new steps. Once all steps on the `core-lib` `release` workflow are executed, then its immediate dependents `release` workflow will start, in parallel. 

### Q4: Graph Resolution Implementation
You mentioned Tako needs to "resolve the entire graph ahead of time." 

**Where should this graph resolution happen?**
- Should it be part of the `tako exec` command when it starts?
- Should there be a separate graph analysis phase before any workflow execution?
- How should Tako discover the full dependency graph (by scanning all repositories' dependents declarations)?

A: `tako graph` up till PR #141 was merged had the logic to resolve the entire dependency graph based on `dependents` field. We should restore that functionality and rely on it.  

### Q5: Event Queuing Implementation 
For preventing concurrent workflow runs when reacting to events from different parents:

**How should the queuing work?**
- Should Tako maintain a global queue per repository+workflow combination? A: Tako is a CLI, the entire state of an workflow execution is in tako's memory. It should just have a data structure to represent that.
- Should events be queued until the current workflow completes, then process queued events? A: Yes, each event instance should trigger one workflow at a time. If an event of a given time is already being processed, the new events should wait. An event finishes being processed when its workflow finishes (or if it doesn't match the filters). 
- How should Tako handle the case where multiple parents complete simultaneously? A: FIFO

---

## FINAL IMPLEMENTATION PLAN

Based on your comprehensive answers, here's the technical architecture:

### Core Orchestration Architecture

**Tako handles ALL orchestration complexity - customers write only business logic**

1. **Graph Resolution**: Restore `tako graph` functionality (pre-PR #141) to resolve dependency graphs
2. **Fan-in/Fan-out**: Tako manages all coordination logic, not workflow steps 
3. **Event Queuing**: FIFO queue per repository+event type, in Tako's memory
4. **Hybrid Execution Flow**:
   ```
   tako exec release core-lib → 
   ├─ Execute core-lib release steps
   │  ├─ Build/release step emits library_released event  
   │  ├─ Event triggers update_dependency workflows in lib-a & lib-b
   │  └─ Wait for subscriber reactions to complete
   ├─ Core-lib release workflow completes
   └─ Trigger release workflow in immediate dependents (lib-a & lib-b) in parallel
   ```

### Updated Configuration Schema

```yaml
# core-lib/tako.yml
dependents:
  - repo: "tako-test/lib-a:main"
  - repo: "tako-test/lib-b:main"
workflows:
  release:
    steps: [build-and-release-step, ...]

# lib-a/tako.yml  
subscriptions:
  - artifact: "tako-test/core-lib:main"
    events: ["library_released"]
    workflow: "update_dependency"
dependents:
  - repo: "tako-test/java-bom:main"
workflows:
  update_dependency: [steps...]
  release: [steps...]

# java-bom/tako.yml
subscriptions:
  - artifact: "tako-test/lib-a:main"
    events: ["library_released"] 
    workflow: "update_dependency"
  - artifact: "tako-test/lib-b:main"
    events: ["library_released"]
    workflow: "update_dependency"  
workflows:
  update_dependency: [steps...]
  release: [steps...]
```

### Implementation Phases (Revised)

**Phase 0: Restore Graph Resolution**
- Restore `tako graph` functionality from pre-PR #141
- Ensure it can resolve full dependency graphs from `dependents` declarations

**Phase 1: Core Orchestration Engine**  
- Implement hybrid execution: workflow completion triggers dependents + event emission triggers subscribers
- Add event queuing (FIFO per repo+event type) in Tako's memory
- Implement fan-in coordination (Tako waits for all subscriber reactions before continuing)

**Phase 2: Remove Customer Orchestration Logic**
- Remove `jq` fan-in logic from java-bom workflows  
- Customers write only business logic, Tako handles orchestration

**Phase 3: Test Implementation**
- Re-enable java-bom test with proper orchestration
- Verify Tako handles all coordination without external state files

## FINAL TECHNICAL QUESTIONS

### Q1: Event Emission Implementation
When a workflow step "emits an event," how should this work technically?
- Should steps use a special `produces: events: [...] ` YAML syntax?
- Should there be a built-in step type like `uses: tako/emit-event@v1`?
- Or should Tako automatically emit events based on step completion and configuration?

**Example:** How should the core-lib build step emit `library_released`?

A: Please look at the code, I believe this is already implemented. Please show me your proposal based on what you find in the code. 

### Q2: Graph Resolution Restoration
You mentioned restoring `tako graph` from pre-PR #141. Looking at the current codebase:
- Should I revert the graph resolution logic back to that specific commit?
- Or should I rebuild the graph resolution using the current codebase structure but with the old logic?
- How should the graph resolution integrate with the new event-driven subscriptions?

A: you should rebuild the graph resolution using the current codebase structure but with the old logic, then adapt it to the new requirements as needed.

### Q3: Workflow Name Consistency  
You mentioned lib-a/lib-b should have `release` workflows instead of `propose-and-release-update`:
- Should all repositories have the same workflow names (e.g., `release`, `update_dependency`)? A: This is just a string from tako's perspective. I am being specific about the name because I want the test case to be expressive.
- How should Tako handle cases where a dependent doesn't have the triggered workflow name? A: Fail.
- Should there be a naming convention or validation for workflow names? A: No.

### Q4: Event-Driven vs Directed Triggering Priority
In the hybrid model, when both mechanisms could apply:
- If core-lib completes `release` workflow AND emits `library_released` event, which happens first? A: Events are emitted at the end of steps. Workflows are completed when all steps are executed, and their events have finished being consumed. So events come first, then workflows.
- Should subscriber reactions (update_dependency) complete BEFORE dependent workflows (release) start? A: Yes, for the `release_artifact` step to fully complete, its event needs to be processed by its children. Only then the `release` workflow of the parent will end. 
- Or should they run in parallel? A: No.

### Q5: Implementation Starting Point
To begin implementation:
- Should I start by temporarily disabling the java-bom-fanout E2E test as planned? A: yes
- Which existing E2E tests should I verify work with directed orchestration first? A: Please look iinto the code, ask gemini for help.
- Should I focus on the orchestration engine first, or graph resolution first? A: sounds good.

### Q6: Memory State Management
Since Tako keeps workflow execution state in memory:
- How should Tako handle workflow resumption if the process is interrupted? 
- Should there be any persistence for long-running orchestrations?
- How should multiple `tako exec` instances coordinate if run simultaneously?

A: I believe this is already covered in the docs/designs folder. Please read the content there and let me know if you have additional questions about this or anything else.

---

## CRITICAL DESIGN CONFLICT DISCOVERED ⚠️

**MAJOR INCONSISTENCY FOUND**: The existing design document `docs/designs/exec-workflow/design.md` explicitly **REJECTS the dependents approach** that you want me to implement!

### Design Document vs Your Requirements

**Design Document Says:**
- **"Subscriptions over Dependents"** - Line 32: "Child repositories declare their own subscription criteria instead of parent repositories maintaining `dependents` blocks"
- **Line 808**: "Alternatives rejected: Parent-maintained dependents blocks create tight coupling and maintenance burden"
- **Line 128**: "The subscription model replaces the earlier `dependents` configuration pattern"

**Your Requirements Say:**
- "Tako needs to resolve the entire graph ahead of time" with dependents field
- Use dependents for directed orchestration + subscriptions for event-driven reactions
- "It's a doubly linked list, where `dependents` is the edge in one direction, and `subscriptions` is the edge in the other direction"

### Core Architectural Conflict

**Design Document Architecture:**
```
Parent Workflow → tako/fan-out@v1 step → Events → Child Subscriptions → Child Workflows
```

**Your Intended Architecture:**  
```
Parent Workflow → Dependents triggering → Child Workflows
                → Event emissions    → Subscriptions → Reaction Workflows
```

### Questions to Resolve This Conflict

1. **Should I implement YOUR hybrid dependents+subscriptions architecture?**
   - This means the existing design document becomes incorrect/outdated
   - Current implementation using `tako/fan-out@v1` would be replaced/supplemented

2. **Or should I follow the existing design document?**
   - This means your "dependents" requirements can't be implemented as specified
   - Would need to achieve directed orchestration through subscriptions only

3. **Is the design document outdated?**
   - Was it written before you clarified the hybrid architecture requirements?
   - Should I treat your specifications as the authoritative source?

## REVISED IMPLEMENTATION PLAN

**I need clarification on this fundamental conflict before proceeding.** The plan below assumes I should implement YOUR hybrid architecture (dependents + subscriptions), making the current design document outdated:

### What the Design Document Actually Implements

**Current Architecture (from design doc):**
- **Only subscriptions** - no dependents field <- this should be corrected to use both `dependents` and `subscriptions`
- **tako/fan-out@v1 steps** trigger child workflows through events <- this is outdated and unnecessary given the current plan
- **"Parent-Led Execution"** but only via event emission <- our current plan is superior
- **State persistence** to `~/.tako/state/<run-id>.json` files <- we should keep this. The state file is for tako to exit and resume execution.
- **Discovery via graph.BuildGraph()** but only for subscription evaluation <- this needs to use the `dependents` field instead of subscribers, since by definition a root node will have no subscriptions, only dependents.

**Key Design Quotes:**
- Line 223: "Tako analyzes the entire dependency tree using existing `graph.BuildGraph()`"
- Line 225-228: "Subscription Evaluation" → "Child Triggering" → "Deep Waiting" for fan-out steps
- Line 115-120: Parent workflows use `tako/fan-out@v1` with `wait_for_children: true`

### What You Want Me To Implement

**Your Hybrid Architecture:**
- **Dependents field** for directed orchestration (workflow completion triggers same workflow in children)
- **Subscriptions** for event-driven reactions (step events trigger reaction workflows)
- **Both mechanisms working together** with proper synchronization
- **Graph resolution ahead of time** for fan-in handling
- **FIFO event queuing** in memory for concurrent event management

### The Fundamental Question

**Which architecture should I implement?**

**Option 1: Your Hybrid Architecture (Ignore Design Doc)**
- Implement dependents + subscriptions as you specified
- Current `tako/fan-out@v1` approach becomes incorrect
- Design document needs complete rewrite
- Much more complex orchestration logic needed

**Option 2: Design Document Architecture (Modify Your Requirements)**  
- Use only subscriptions with `tako/fan-out@v1` steps
- Achieve "directed orchestration" through structured events
- Design document remains accurate
- Leverage existing architecture and state management

**Option 3: Evolution of Design Document**
- Keep existing subscriptions + `tako/fan-out@v1` 
- Add dependents as supplementary directed triggering mechanism
- Both approaches coexist
- Design document gets updated to reflect hybrid model

A: Option 1

---

## FINAL IMPLEMENTATION PLAN - HYBRID ARCHITECTURE

Based on your clarification to implement **Option 1** and update the design document, here's the complete plan:

### Architecture Overview

**Hybrid Directed + Event-Driven Orchestration:**
- **Dependents** provide directed orchestration (workflow completion triggers same workflow in children)
- **Subscriptions** provide event-driven reactions (step events trigger reaction workflows)
- **Graph resolution** uses dependents field to build dependency tree (not subscriptions)
- **State persistence** kept as designed (`~/.tako/state/<run-id>.json`) for resume capabilities
- **Event structure** from design doc is good and should be preserved

### Design Document Updates Needed

**What to Keep from Current Design:**
- Event data structure and payload format ✅
- State persistence and resume capabilities ✅  
- CEL expressions for subscription filters ✅
- Schema versioning and validation ✅
- Container security model ✅
- Resource management and concurrency control ✅

**What to Update in Design:**
- Replace subscriptions-only with hybrid dependents+subscriptions model
- Update graph resolution to use dependents field instead of subscriptions
- Remove tako/fan-out@v1 as primary orchestration mechanism
- Update execution flow to show dependents triggering + event reactions
- Revise "Subscriptions over Dependents" rationale section

### Implementation Phases

**Phase 0: Prepare Foundation**
```bash
# Temporarily disable problematic test
git mv test/e2e/templates/java-bom-fanout test/e2e/templates/java-bom-fanout.disabled
```

**Phase 1: Graph Resolution with Dependents**  
- Update `internal/graph/graph.go` to use dependents field for tree building
- Ensure BuildGraph() traverses dependents (not subscriptions) for root discovery
- Root nodes have dependents but no subscriptions (as you noted)

**Phase 2: Hybrid Orchestration Engine**
- Implement execution flow: workflow steps → event reactions → wait → workflow completion → dependent triggering
- Add FIFO event queuing per repo+event type in Tako's memory
- Coordinate fan-in without customer jq logic
- Remove need for tako/fan-out@v1 steps

**Phase 3: Java BOM Test Configuration**
```yaml
# core-lib/tako.yml  
dependents:
  - repo: "tako-test/lib-a:main"
  - repo: "tako-test/lib-b:main"
workflows:
  release: [steps that emit library_released events]

# lib-a/tako.yml + lib-b/tako.yml
subscriptions:
  - artifact: "tako-test/core-lib:main"
    events: ["library_released"]
    workflow: "update_dependency"
dependents:
  - repo: "tako-test/java-bom:main"
workflows:
  update_dependency: [react to core-lib changes]
  release: [triggered by dependents after core-lib completes]

# java-bom/tako.yml
subscriptions:
  - artifact: "tako-test/lib-a:main"
    events: ["library_released"] 
    workflow: "update_dependency"
  - artifact: "tako-test/lib-b:main"
    events: ["library_released"]
    workflow: "update_dependency"
workflows:
  update_dependency: [react to library changes]
  release: [triggered by dependents after lib-a/lib-b complete]
```

**Phase 4: Remove Legacy Orchestration**
- Remove tako.state.json usage and jq fan-in logic
- Eliminate orchestrator simulation steps
- All coordination handled in Tako core

**Phase 5: Test Coverage & Design Doc Update**
- Re-enable java-bom-fanout test with hybrid orchestration
- Update design document with hybrid architecture
- Verify all E2E test types work correctly

### Immediate Next Steps

**Question 1: Graph Resolution Implementation**
The current `internal/graph/graph.go` needs to be updated to build dependency trees from dependents field. Should I:
- Start by examining the current BuildGraph() function and see how it works with subscriptions?
- Look at pre-PR #141 logic to understand how it worked with dependents originally?
- What's the entry point for graph resolution in the execution flow?

**Question 2: Orchestration Engine Integration** 
Where should the new hybrid orchestration logic be implemented?
- Should it extend the existing fanout engine in `internal/engine/fanout.go`?
- Or create a new orchestration module separate from the current event-focused fanout?
- How does it integrate with the `tako exec` command flow?

**Question 3: State Management for Hybrid Model**
The design doc's state persistence covers event-driven flows. For the hybrid model:
- How should dependents triggering be tracked in the state file?
- Should the FIFO event queue state be persisted for resume?
- Any changes needed to the execution tree state format?

**Question 4: Test Strategy**
For validating the implementation:
- Should I start with `java-binary-incompatibility` (pure dependents)?
- Move to `fan-out-test` (pure subscriptions)?  
- Then tackle `java-bom-fanout` (hybrid)?
- How should I verify the existing subscription tests still work?

---

**Ready to begin implementation of the hybrid architecture with these clarifications!**