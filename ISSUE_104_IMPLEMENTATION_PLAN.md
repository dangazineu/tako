# Issue #104 Implementation Plan: Containerized Execution with Security Hardening

## Overview
Issue #104 focuses on implementing secure containerized step execution with comprehensive security hardening and resource management. This builds on the existing execution engine to add container support.

## Key Components to Implement

### 1. Container Runtime Management
- **File**: `internal/engine/container.go`
- **Features**:
  - Auto-detect Docker/Podman runtime using generic OCI client abstraction
  - Container lifecycle management (create, run, cleanup) with proper logging
  - Image pull with private registry support and robust error handling
  - Security hardening (non-root user, read-only filesystem, capabilities)
  - Container logging capture and streaming to Tako output
  - Environment variable injection and working directory management
  - Volume mount restrictions (read-only by default, path whitelisting)

### 2. Resource Management System
- **File**: `internal/engine/resources.go` 
- **Features**:
  - Three-tier hierarchical limits (global → repository → step)
  - Resource monitoring with 90% utilization warnings (surfaced via logs and CLI output)
  - CPU and memory constraint enforcement with configurable breach actions
  - Resource quota management (per-workflow and per-repository scoped)
  - Over-subscription handling and queueing strategies
  - Performance-optimized monitoring with configurable frequency

### 3. Security Hardening Extensions
- **File**: `internal/engine/security.go` (extend existing)
- **Features**:
  - Container security profiles with restrictive defaults
  - Network isolation controls (no network access by default)
  - Seccomp profile enforcement with default restrictive profile
  - Fine-grained capability management with explicit opt-in
  - Secrets management for private registries (env vars, credential helpers)
  - Comprehensive audit logging for all container operations
  - Image trust and integrity verification recommendations

### 4. Configuration Integration
- Update workflow execution to support container steps with entrypoint/command overrides
- Integrate resource limits into step execution with early validation
- Add container-specific configuration validation (images, resources, security)
- Environment variable and working directory configuration support
- Image caching strategy and performance optimization

## Implementation Strategy

### Phase 1: Core Container Infrastructure 
1. **Container Runtime Detection & Basic Operations**
   - Implement container runtime auto-detection
   - Basic container create/run/cleanup operations
   - Image management and pull logic

2. **Security Hardening Foundation**
   - Non-root user execution (UID 1001)
   - Read-only root filesystem
   - Basic capability dropping

### Phase 2: Resource Management
1. **Resource Limit Framework**
   - Parse and validate resource configurations
   - Implement three-tier hierarchical limits
   - Resource monitoring and alerting

2. **Integration with Execution Engine**
   - Extend existing runner to support container steps
   - Resource enforcement during execution
   - Container cleanup integration

### Phase 3: Advanced Security & Polish
1. **Advanced Security Features**
   - Network isolation controls
   - Seccomp profile enforcement
   - Fine-grained capability management

2. **Private Registry Support**
   - Authentication handling
   - Image pull policies
   - Registry configuration

## Testing Strategy

### Unit Tests
- Container runtime detection logic
- Resource parsing and validation
- Security hardening functions  
- Container lifecycle operations
- Configuration validation (malformed tako.yml, invalid images)
- Error handling and edge cases

### Integration Tests
- Container execution with security measures
- Resource limit enforcement and breach actions
- Multi-container cleanup and interrupted workflows
- Registry authentication and private registry support
- Network failure scenarios and image pull failures
- Container exit code capture and propagation
- Long-running container management

### E2E Tests
- Full workflow with containerized steps
- Resource exhaustion and over-subscription scenarios
- Security validation tests (container breakout attempts)
- Cross-platform compatibility (Docker/Podman)
- Container logging and output streaming
- Environment variable injection and working directory tests
- Volume mount security and path whitelisting

## Key Technical Considerations

1. **Backward Compatibility**: Ensure existing shell-based steps continue working
2. **Cross-Platform Support**: Work with both Docker and Podman runtimes using OCI abstraction
3. **Resource Safety**: Prevent container resource leaks on the host system
4. **Security-First**: All containers run with minimal privileges by default, network isolated
5. **Performance**: Efficient container reuse, image caching, and cleanup
6. **Error Handling**: Use idiomatic Go error handling with context.Context for cancellation
7. **Observability**: Structured logging and comprehensive audit trails
8. **Configuration**: Early validation and clear error messages for misconfigurations

## Expected Configuration Example
```yaml
workflows:
  build:
    resources:
      cpu_limit: "2.0" 
      mem_limit: "4Gi"
    steps:
      - id: compile
        image: "golang:1.22"
        resources:
          cpu_limit: "1.0"
          mem_limit: "2Gi" 
        run: go build ./...
```

## Deliverables
1. ✅ **Container execution engine** with security hardening
2. ✅ **Resource management system** with hierarchical limits  
3. ✅ **Comprehensive test coverage** for all container features
4. ✅ **Documentation** for container configuration and security
5. ✅ **Backward compatibility** with existing shell-based workflows

## Implementation Phases

### Phase 1: Core Container Infrastructure
- [ ] Container configuration validation and early error detection
- [ ] Container runtime detection (Docker/Podman) with OCI abstraction
- [ ] Basic container operations (create, run, cleanup) with proper logging
- [ ] Image management and pull logic with robust error handling
- [ ] Container logging capture and streaming to Tako output
- [ ] Environment variable injection and working directory management
- [ ] Non-root user execution (UID 1001)
- [ ] Read-only root filesystem with volume mount restrictions
- [ ] Basic capability dropping with secure defaults
- [ ] **COMMIT**: Core container infrastructure with security foundation

### Phase 2: Resource Management
- [ ] Resource configuration parsing and validation with breach action configs
- [ ] Three-tier hierarchical resource limits (global → repository → step)
- [ ] Resource monitoring with 90% warnings (logs and CLI output)
- [ ] Over-subscription handling and queueing strategies
- [ ] Integration with existing execution engine using context.Context
- [ ] Container resource enforcement with configurable actions
- [ ] Performance-optimized monitoring with configurable frequency
- [ ] **COMMIT**: Comprehensive resource management system

### Phase 3: Advanced Security & Polish
- [ ] Network isolation controls (no network access by default)
- [ ] Seccomp profile enforcement with restrictive default profile
- [ ] Fine-grained capability management with explicit opt-in
- [ ] Private registry authentication using secure credential management
- [ ] Image pull policies and caching strategy
- [ ] Image trust and integrity verification features
- [ ] Comprehensive audit logging for all container operations
- [ ] Entrypoint/command override support
- [ ] **COMMIT**: Advanced security and registry support

### Testing & Finalization
- [ ] Comprehensive unit test coverage
- [ ] Integration tests for container execution
- [ ] E2E tests for containerized workflows
- [ ] Documentation updates
- [ ] **COMMIT**: Complete testing and documentation

This implementation will provide Tako with secure, resource-managed container execution capabilities while maintaining the existing workflow execution patterns. The security-first approach ensures containers run with minimal privileges and proper isolation.