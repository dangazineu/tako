# Implementation Plan: Java BOM E2E Test (Issue #147)

## Overview
Implement a fully automated E2E test demonstrating Java BOM release orchestration with multi-level fan-out and fan-in patterns. The test simulates a real-world scenario where updating a core library triggers cascading dependency updates culminating in an automated BOM release.

## Phase-Based Implementation Strategy

### Phase 1: Test Infrastructure Setup
**Goal**: Create test case definition and repository templates  
**Duration**: Foundation phase  
**Deliverables**: Test case structure, repository templates, mock scripts

#### Tasks:
1. **Add Test Case Definition** (`test/e2e/get_test_cases.go`)
   - Add `java-bom-fanout` test case entry
   - Configure environment definition with 4 repositories
   - Define test steps for triggering and verification

2. **Create Repository Templates** (`test/e2e/templates/java-bom-fanout/`)
   - `core-lib/`: Basic Java library with tako.yml for release workflow
   - `lib-a/`: Java library depending on core-lib with fan-out subscription
   - `lib-b/`: Java library depending on core-lib with fan-out subscription  
   - `java-bom/`: BOM project with fan-in coordination logic

3. **Create Mock Scripts** (within each repository template)
   - `mock-gh.sh`: Simulates GitHub CLI operations
   - `mock-semver.sh`: Simulates semantic versioning tool
   - Mock scripts create verification files for testing

#### Success Criteria:
- Test case loads without errors
- All repository templates are created
- Maven dependencies are correctly configured
- Mock scripts are executable and functional

### Phase 2: Core Workflow Implementation  
**Goal**: Implement tako.yml workflows for all repositories  
**Duration**: Core logic phase  
**Deliverables**: Working workflows with event routing

#### Tasks:
1. **Core-Lib Release Workflow**
   - Implement `release` workflow with fan-out step
   - Configure `core_library_released` event emission
   - Add mock Maven deployment step

2. **Library Update Workflows** (lib-a, lib-b)
   - Implement `propose-and-release-update` workflow
   - Add subscription to `core_library_released` event
   - Implement PR creation, waiting, merging, and release sequence
   - Add `library_released` event emission

3. **BOM Coordination Workflow** (java-bom)
   - Implement `aggregate-and-release-bom` workflow 
   - Add subscriptions to `library_released` events from lib-a and lib-b
   - Implement state file coordination logic
   - Add conditional BOM release workflow

#### Success Criteria:
- All workflows compile and validate successfully
- Event subscriptions are correctly configured
- Fan-out and fan-in logic is implemented
- Step output passing works correctly

### Phase 3: State Management and Coordination
**Goal**: Implement fan-in coordination using state files  
**Duration**: Coordination logic phase  
**Deliverables**: Working state management for BOM aggregation

#### Tasks:
1. **Atomic State File Management** 
   - Implement `tako.state.json` handling with write-then-rename strategy
   - Add state update logic: write to `tako.state.json.tmp`, then rename to `tako.state.json`
   - Implement readiness checking (all libraries present)
   - Add proper error handling for state file operations

2. **Fan-In Logic Implementation**
   - Add conditional execution based on state completeness
   - Implement BOM update from state file versions
   - Add final BOM release workflow invocation
   - Include cleanup logic with trap handlers for failure scenarios

3. **Mock External Tool Integration**
   - Integrate mock scripts with specific sub-command handling
   - Ensure PATH override works correctly in test environment
   - Verify mock outputs are captured for verification
   - Add mock script validation and consistency checks

#### Success Criteria:
- State file is correctly maintained across events using atomic operations
- BOM only triggers when both libraries are ready
- Mock tools produce expected verification artifacts
- Fan-in coordination works reliably under concurrent access
- Proper cleanup occurs even on workflow failures

### Phase 4: Verification and Testing
**Goal**: Implement comprehensive test verification  
**Duration**: Validation phase  
**Deliverables**: Complete E2E test with robust verification

#### Tasks:
1. **Execution Order Verification**
   - Add timestamped verification files at each step
   - Implement timestamp checking in test verification
   - Ensure proper execution sequence (core-lib → libs → BOM)

2. **Content Verification**
   - Add POM.xml content verification for final BOM
   - Verify correct versions are included in BOM
   - Add state file content verification

3. **Test Robustness**
   - Add error scenario testing (partial failures, one library fails)
   - Implement idempotency verification: trigger core-lib release twice, verify downstream workflows run only once
   - Add timeout and cleanup verification with trap handlers
   - Test graceful failure: BOM not released if library workflows fail

4. **Integration Testing**
   - Test with both local and remote modes
   - Verify test runs reliably in CI environment
   - Performance and resource usage validation
   - Add negative path testing (verify BOM doesn't release on partial failures)

#### Success Criteria:
- Test passes consistently in local environment
- All verification steps validate correct orchestration
- Test handles error scenarios gracefully with proper cleanup
- Idempotency is verified through duplicate event testing
- Negative path scenarios are validated (graceful failure)
- Performance is acceptable for CI execution

### Phase 5: Documentation and Refinement
**Goal**: Complete documentation and final refinements  
**Duration**: Polish phase  
**Deliverables**: Production-ready test with documentation

#### Tasks:
1. **Code Documentation**
   - Add comprehensive comments to template files
   - Document mock script behavior and verification
   - Add README for test scenario explanation

2. **Test Coverage Analysis**
   - Update `issue_coverage.md` with final coverage numbers
   - Use targeted coverage analysis: profile specific code paths exercised by E2E test
   - Document new coverage areas: multi-repo fan-in coordination, cascading Maven updates, idempotency of subscriptions
   - Verify no regressions in existing functionality
   - Document critical paths covered: fan-out state manager, event subscription router, nested workflow execution

3. **Final Verification**
   - Run complete test suite to ensure no regressions
   - Verify new test integrates seamlessly with existing tests
   - Performance and resource impact assessment

#### Success Criteria:
- All documentation is complete and accurate
- Test coverage is maintained or improved
- No regressions in existing functionality
- Test is ready for production use

## Key Architecture Decisions

### Event Flow Design
```
core-lib (release) 
    ↓ core_library_released event
    ├─→ lib-a (propose-and-release-update) → library_released event
    └─→ lib-b (propose-and-release-update) → library_released event
              ↓ both events
         java-bom (aggregate-and-release-bom)
```

### State Management Strategy
- Use `tako.state.json` in java-bom workspace for coordination
- JSON format: `{"lib-a": "version", "lib-b": "version"}`
- Conditional execution based on state completeness

### Mock Strategy
- PATH-based script override for external tools
- Scripts create verification files for testing
- Maintain actual Maven workflow for build authenticity

### Verification Approach
- Multi-layered verification (execution order, content, state)
- Timestamped artifacts for sequence validation
- Content-based validation of final BOM

## Risk Mitigation

### Potential Issues:
1. **Race Conditions**: Fan-in coordination timing issues
2. **State Corruption**: Concurrent state file access
3. **Mock Reliability**: External tool simulation accuracy
4. **Test Flakiness**: Non-deterministic behavior in complex orchestration
5. **Configuration Drift**: Inconsistencies between tako.yml files across repositories
6. **Diamond Dependencies**: Complex dependency scenarios in future extensions

### Mitigation Strategies:
1. Use atomic file operations (write-then-rename) for state management
2. Implement proper cleanup with trap handlers for failure scenarios
3. Use specific sub-command mocking rather than generic tool mocking
4. Multiple test runs during development to catch flakiness
5. Create consistency checks or shared templates for common workflow patterns
6. Explicitly test idempotency by triggering duplicate events

## Success Metrics

### Functional Requirements:
- [ ] Test demonstrates full automation with zero human intervention
- [ ] Core library update triggers downstream library updates
- [ ] BOM only releases after all dependencies are ready
- [ ] All external tool interactions are properly mocked
- [ ] Verification confirms correct execution order and content
- [ ] Idempotency verified: duplicate events don't cause duplicate workflows
- [ ] Graceful failure: BOM not released if library workflows fail

### Quality Requirements:
- [ ] Test runs reliably in CI environment
- [ ] No regressions in existing functionality
- [ ] Performance impact is acceptable
- [ ] Code coverage is maintained
- [ ] Documentation is comprehensive

### Technical Requirements:
- [ ] Leverages existing fan-out and event systems
- [ ] Uses established E2E testing patterns
- [ ] Integrates seamlessly with existing test suite
- [ ] Mock strategy is maintainable and extensible