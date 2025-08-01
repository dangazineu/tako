# Issue #147 Background Analysis

## Issue Summary
Create a fully automated E2E test demonstrating Java BOM release orchestration with fan-out and fan-in patterns. The test simulates a real-world scenario where updating a core library triggers cascading dependency updates across multiple libraries, culminating in an automated BOM release.

## Repository Architecture
The test involves 4 repositories in a specific dependency pattern:
- **core-lib**: Foundational Java library (triggers the cascade)
- **lib-a**: Depends on core-lib, publishes library releases
- **lib-b**: Depends on core-lib, publishes library releases  
- **java-bom**: Aggregates all libraries into a Bill of Materials

## Orchestration Flow
1. **Phase 1**: core-lib release triggers lib-a and lib-b updates (fan-out)
2. **Phase 2**: Each library autonomously creates PRs, waits for CI, merges, and releases
3. **Phase 3**: java-bom collects release events and creates final BOM (fan-in)

## Research Findings

### Existing E2E Test Architecture
The codebase has a robust E2E testing framework:
- **Test Definition**: Tests defined in `test/e2e/get_test_cases.go` with structured `TestCase` format
- **Environment Setup**: Template-based repository creation with dependency management
- **Verification**: File-based verification patterns for asserting execution order
- **Existing Java Pattern**: `java-binary-incompatibility` test provides Maven integration patterns

### Fan-Out System Capabilities
The existing fan-out implementation (`internal/engine/fanout.go`) provides:
- **Multi-level Fan-out**: Supports cascading event propagation
- **State Management**: Comprehensive tracking of child workflow states
- **Idempotency**: Event fingerprinting prevents duplicate executions
- **Advanced Features**: Circuit breakers, retry logic, timeouts, concurrency limits
- **Diamond Dependencies**: First-wins rule for duplicate subscriptions

### Event and Subscription System
- **CEL Filtering**: Advanced filtering capabilities for event routing
- **Template Variables**: Rich context available (`{{ .event.payload.field }}`)
- **Subscription Patterns**: Artifact-based subscription with branch specificity
- **State Tracking**: Fan-out state manager handles complex orchestration flows

### Workflow Execution and Chaining
- **Workflow Isolation**: Child workflows run in isolated environments
- **Context Propagation**: Event context and inputs flow through the system
- **Resource Management**: Automatic cleanup and resource limits
- **Step Output Passing**: `produces.outputs` mechanism for data flow between steps

### Mock Strategy Patterns
Existing tests use sophisticated mocking:
- **File-based Verification**: Create timestamped files to verify execution order
- **External Tool Mocking**: Mock `git`, `mvn`, and other external commands via scripts
- **PATH Manipulation**: Test scripts can override system commands
- **State File Tracking**: Use files to maintain state across workflow executions

### Maven Integration Patterns
The `java-binary-incompatibility` test demonstrates:
- **Repository Isolation**: Uses `MAVEN_REPO_DIR` for test isolation
- **POM Templates**: Standard Maven project structures
- **Build Integration**: `mvn clean install` with custom repository paths
- **Dependency Management**: Multi-repository dependency chains

## Key Technical Requirements

### Event Types and Payloads
- `core_library_released`: Payload includes version information
- `library_released`: Payload includes library name and new version
- Events must include sufficient context for template resolution

### Workflow Features to Test
- **Fan-out Step**: `uses: tako/fan-out@v1` with event distribution
- **Workflow Chaining**: `tako exec` commands from within workflows
- **Step Output Capture**: `produces.outputs` for PR numbers and versions
- **Conditional Logic**: State file checking before triggering actions
- **External Tool Integration**: Mock GitHub CLI, semantic versioning tools

### Mock Implementation Strategy
- **GitHub CLI Mock**: Scripts that simulate `gh pr create`, `gh pr checks`, `gh pr merge`
- **Semantic Versioning Mock**: Script that simulates version increments
- **Maven Mock**: Use actual Maven with isolated repositories
- **Verification Files**: Timestamped files to assert execution order and completeness

### State Management Requirements
- **BOM State File**: Track which libraries have reported new versions
- **Conditional Execution**: Only trigger BOM creation when all dependencies ready
- **Fan-in Coordination**: Use existing state management system for aggregation
- **Idempotency**: Ensure repeated events don't cause duplicate work

## Integration Points

### Test Environment Structure
Must create 4 repository templates with:
- Maven POM files with appropriate dependencies
- Tako configuration files with workflows and subscriptions
- Mock scripts for external tool simulation
- Verification scripts for state checking

### Fan-Out Configuration
- **Event Propagation**: core-lib → lib-a/lib-b (parallel)
- **Fan-In Aggregation**: lib-a + lib-b → java-bom (synchronized)
- **State Coordination**: Use existing fan-out state manager
- **Timeout Handling**: Appropriate timeouts for async operations

### Verification Strategy
- **Execution Order**: Verify core-lib triggers before library updates
- **State Consistency**: Ensure all intermediate states are captured
- **Final Validation**: Confirm BOM creation only after all dependencies
- **Error Scenarios**: Test partial failures and recovery

## Architecture Compatibility

### Existing System Strengths
- Robust fan-out system with state management
- Template engine with rich context support
- Comprehensive E2E testing framework
- Maven integration patterns already established

### Areas Requiring Extension
- New event types for Java ecosystem
- State file coordination patterns for fan-in
- Mock script integration for GitHub operations
- Complex multi-repository coordination testing

## Implementation Feasibility
Based on research findings, the issue is **fully feasible** with existing architecture:
- All required fan-out capabilities exist
- Event system supports required complexity
- Testing framework can accommodate the scenario
- Existing Java patterns provide foundation
- Mock strategies are well-established

The implementation will primarily involve:
1. Creating template repositories with appropriate tako.yml configurations
2. Building mock scripts for external tool simulation
3. Implementing file-based verification for complex orchestration
4. Leveraging existing fan-out and state management systems

**Confidence Level**: High - All architectural components are in place