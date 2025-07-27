# Manual Testing Guide for tako/fan-out@v1

This document provides a comprehensive manual testing guide for the `tako/fan-out@v1` step implementation, including test scripts, verification procedures, and expected results.

## Overview

⚠️ **IMPORTANT**: These tests validate only the **discovery and setup phases** of the `tako/fan-out@v1` implementation. The actual child workflow execution is currently mocked to avoid deadlock issues.

The testing covers:
- Repository discovery with matching subscriptions ✅
- Parameter validation and error handling ✅
- Basic integration with tako workflow system ✅
- **Child workflow execution**: Currently simulated only ❌

## Manual Testing Scripts

### Primary Testing Script: `simple_manual_test.sh`

A comprehensive testing script that validates the core functionality of the fan-out step:

```bash
./simple_manual_test.sh
```

**Test Coverage:**
1. **Binary Building** - Ensures tako and takotest CLIs build successfully
2. **Environment Setup** - Creates isolated test environment with multiple repositories
3. **Workflow Configuration** - Sets up publisher and subscriber repositories with appropriate tako.yml files
4. **Fan-out Execution** - Executes workflows containing fan-out steps
5. **Result Verification** - Confirms expected outputs and side effects
6. **Error Handling** - Tests parameter validation and error scenarios
7. **Configuration Variants** - Tests different fan-out parameter combinations

### Extended Testing Script: `manual_test_fanout.sh`

A more detailed testing script that includes additional test scenarios:

```bash
./manual_test_fanout.sh
```

**Additional Coverage:**
- Repository discovery mechanisms
- Subscription matching logic
- Template expansion in workflow steps
- Integration with existing tako workflow features
- Multi-step workflow compatibility

## Test Scenarios

### 1. Basic Fan-out Execution

**Purpose:** Verify that fan-out steps execute successfully within workflows.

**Setup:**
- Publisher repository with fan-out step
- Subscriber repository with matching subscription
- Simple event type configuration

**Test:**
```yaml
# Publisher workflow
- id: fanout-test
  uses: tako/fan-out@v1
  with:
    event_type: "test_event"
    wait_for_children: false
```

**Expected Result:** 
- Workflow executes without errors
- Fan-out step completes successfully
- No deadlocks or blocking issues

### 2. Parameter Validation

**Purpose:** Confirm parameter validation works correctly.

**Test Cases:**
- Missing required `event_type` parameter (should fail)
- Valid minimal parameters (should succeed)
- Valid complete parameters (should succeed)
- Invalid parameter types (should fail)

**Expected Results:**
- Clear error messages for invalid configurations
- Successful execution for valid configurations
- Proper type coercion where appropriate

### 3. Repository Discovery

**Purpose:** Verify the orchestrator can discover repositories with subscriptions.

**Setup:**
- Multiple repositories in cache directory structure
- Various subscription configurations
- Different artifact references and event types

**Expected Results:**
- All repositories with matching subscriptions are discovered
- Repository paths are correctly resolved
- Configuration files are loaded successfully

### 4. Error Handling

**Purpose:** Test robustness under various error conditions.

**Test Cases:**
- Invalid tako.yml syntax in subscriber repositories
- Missing repositories in cache
- Network timeouts (when implemented)
- Subscription evaluation errors

**Expected Results:**
- Graceful error handling
- Detailed error messages
- Continued execution despite individual failures

### 5. Integration Testing

**Purpose:** Verify compatibility with existing tako features.

**Test Cases:**
- Fan-out within multi-step workflows
- Template variable expansion
- Input parameter passing
- Output capture and usage

**Expected Results:**
- Seamless integration with other step types
- Proper context propagation
- No interference with existing functionality

## Verification Checklist

When running manual tests, verify the following:

### ✅ Execution Verification
- [ ] Fan-out steps execute without deadlocks
- [ ] Workflow completion times are reasonable
- [ ] No memory leaks or resource exhaustion
- [ ] Proper cleanup of temporary resources

### ✅ Functional Verification  
- [ ] Repository discovery works correctly
- [ ] Subscription matching functions as expected
- [ ] Event payloads are properly mapped to inputs
- [ ] Child workflow execution is triggered appropriately

### ✅ Error Handling Verification
- [ ] Invalid parameters trigger clear error messages
- [ ] Missing configurations are handled gracefully
- [ ] Partial failures don't block entire execution
- [ ] Error propagation follows expected patterns

### ✅ Integration Verification
- [ ] Fan-out integrates with existing workflow features
- [ ] Template expansion works correctly
- [ ] Input/output handling is consistent
- [ ] No conflicts with other step types

## Known Limitations

### Critical Implementation Gaps
1. **Child Workflow Execution**: The `ExecuteChildWorkflow` method is currently mocked to return simulated run IDs. **No actual child workflows are executed**, making this a proof of concept for discovery only.

2. **Event Payload Handling**: Event payloads are not generated or propagated to child workflows. Input mapping from events to workflow parameters is not tested.

3. **Synchronization**: The `wait_for_children` functionality cannot be tested without real child workflow execution.

4. **Cross-Repository Orchestration**: The core purpose of the fan-out step (orchestrating workflows across repositories) is not implemented or tested.

### Testing Limitations
1. **Network Operations**: Manual tests run in local mode only. Remote repository testing requires additional setup.

2. **Performance Testing**: Current tests focus on functionality rather than performance characteristics.

3. **Scale Testing**: Tests use small numbers of repositories. Large-scale testing should be performed separately.

## Test Results Summary

### Successful Test Cases ✅
- Basic fan-out execution
- Parameter validation (positive and negative cases)
- Error handling for invalid configurations
- Integration with multi-step workflows
- Template expansion in workflow steps
- Repository discovery mechanisms

### Expected Behaviors Confirmed ✅
- No deadlocks during execution
- Proper parameter validation
- Clear error messages for invalid inputs
- Successful integration with existing tako features
- Correct handling of various configuration options

## Running the Tests

### Prerequisites
- Go development environment
- Built tako and takotest binaries
- Sufficient disk space for temporary test environments

### Execution
```bash
# Run simplified test suite
./simple_manual_test.sh

# Run comprehensive test suite  
./manual_test_fanout.sh

# Build binaries if needed
go build -o tako ./cmd/tako
go build -o takotest ./cmd/takotest
```

### Test Environment
- Tests create isolated temporary directories
- Automatic cleanup on completion or failure
- No interference with existing tako configurations
- Safe to run multiple times

## Troubleshooting

### Common Issues

**1. Binary Build Failures**
- Ensure Go toolchain is properly installed
- Check for compilation errors in source code
- Verify all dependencies are available

**2. Test Environment Creation Failures**
- Check available disk space
- Verify write permissions in temporary directories
- Ensure takotest CLI is functioning correctly

**3. Fan-out Execution Errors**
- Review tako.yml syntax in test configurations
- Check parameter values and types
- Verify repository structure matches expectations

**4. False Test Failures**
- Confirm expected output patterns match actual implementation
- Check for timing-related issues in rapid execution
- Verify cleanup didn't interfere with verification steps

## Conclusion

The manual testing suite provides coverage of the **discovery and setup phases** of the `tako/fan-out@v1` step. The following components are working correctly:

- Repository discovery and subscription matching ✅
- Parameter validation and error handling ✅
- Basic integration with tako workflow parsing ✅

**⚠️ Critical Gap**: The core functionality of **executing workflows in other repositories is not implemented**. The current implementation is a proof of concept for the discovery phase only.

**Production Readiness**: This implementation is **NOT ready for production use** until child workflow execution is properly implemented and tested.