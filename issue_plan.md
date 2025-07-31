# Issue #135 Implementation Plan

## Overview
Implement advanced subscription features including CEL caching, enhanced schema compatibility, and diamond dependency resolution with "first-wins" rule.

## Phase-by-Phase Implementation Plan

### Phase 1: CEL Expression Caching Infrastructure
**Goal**: Add in-memory CEL expression caching to SubscriptionEvaluator  
**Status**: ⏳ Pending  
**Health Check**: All existing tests pass + caching unit tests

#### Implementation Details
- Add `celCache` field to `SubscriptionEvaluator` struct using LRU cache
- Implement cache key generation based on CEL expression string
- Add cache hit/miss logic in `evaluateCELFilter` method
- Add cache statistics and optional debugging
- Ensure thread safety for concurrent access

#### Files to Modify
- `internal/engine/subscription.go` - Add caching to SubscriptionEvaluator
- `internal/engine/subscription_test.go` - Add unit tests for caching behavior

#### Testing Requirements
- Unit tests for cache hit/miss scenarios
- Performance benchmarks comparing cached vs uncached evaluation
- Memory usage tests to ensure reasonable cache size limits
- Thread safety tests for concurrent CEL evaluations

#### Success Criteria
- All existing subscription tests pass unchanged
- New caching tests demonstrate performance improvement
- Memory usage remains bounded under normal workloads

---

### Phase 2: Enhanced Schema Compatibility
**Goal**: Add support for compound version ranges  
**Status**: ⏳ Pending  
**Health Check**: All existing tests pass + extended range tests

#### Implementation Details
- Extend `evaluateVersionRange` function to support compound ranges
- Add parsing for ranges like ">=1.0.0 <2.0.0", ">=1.0.0 <=2.0.0"
- Improve error messages for invalid version specifications
- Maintain backward compatibility with existing single range formats

#### Files to Modify
- `internal/engine/subscription.go` - Enhance `evaluateVersionRange`
- `internal/engine/subscription_test.go` - Add compound range test cases

#### Testing Requirements
- Unit tests for all new compound range formats
- Edge case testing (boundary conditions, invalid ranges)
- Backward compatibility verification with existing configurations
- Error message clarity testing

#### Success Criteria
- All existing version range tests pass unchanged
- New compound ranges work correctly according to semver rules
- Clear, actionable error messages for invalid ranges

---

### Phase 3: Subscription Fingerprinting for Diamond Resolution
**Goal**: Enhance event fingerprinting to include subscription details  
**Status**: ⏳ Pending  
**Health Check**: All existing tests pass + fingerprinting tests

#### Implementation Details
- Modify `GenerateEventFingerprint` to accept optional subscription details
- Create `GenerateSubscriptionFingerprint` function for subscription-specific hashing
- Include subscription filters, inputs, workflow, and target repository in fingerprint
- Ensure deterministic ordering of subscription components in hash
- Maintain backward compatibility with existing event fingerprinting

#### Files to Modify
- `internal/engine/fanout_state.go` - Enhance fingerprinting functions
- `internal/engine/fanout_state_test.go` - Add subscription fingerprinting tests

#### Testing Requirements
- Unit tests for subscription fingerprint generation
- Consistency tests (same subscription = same fingerprint)
- Uniqueness tests (different subscriptions = different fingerprints)
- Backward compatibility with existing event fingerprints

#### Success Criteria
- Subscription fingerprints are deterministic and unique
- Existing event fingerprinting functionality unchanged
- Clear separation between event and subscription fingerprints

---

### Phase 4: Diamond Dependency Resolution in FanOutExecutor
**Goal**: Implement "first-wins" logic for duplicate subscriptions  
**Status**: ⏳ Pending  
**Health Check**: All existing tests pass + diamond resolution tests

#### Implementation Details
- Modify `triggerSubscribersWithState` to detect duplicate subscriptions
- Use subscription fingerprinting to identify identical subscriptions
- Implement first-wins ordering (likely based on repository discovery order)
- Add logging for skipped duplicate subscriptions
- Integrate with existing idempotency infrastructure
- Update result reporting to include skipped subscription counts

#### Files to Modify
- `internal/engine/fanout.go` - Enhance `triggerSubscribersWithState`
- `internal/engine/fanout_test.go` - Add diamond dependency test cases

#### Testing Requirements
- Unit tests for duplicate subscription detection
- Integration tests with multiple repositories subscribing to same event
- Test various diamond dependency scenarios (2, 3, 4+ duplicates)
- Verify first-wins ordering is consistent and predictable
- Test interaction with existing idempotency features

#### Success Criteria
- Duplicate subscriptions are correctly identified and deduplicated
- Only first subscription in discovery order executes
- Comprehensive logging and metrics for monitoring
- No regression in existing fan-out functionality

---

### Phase 5: Enhanced Orchestrator Logic
**Goal**: Add filtering and prioritization capabilities to Orchestrator  
**Status**: ⏳ Pending  
**Health Check**: All existing tests pass + orchestrator enhancement tests

#### Implementation Details
- Add subscription filtering methods to Orchestrator
- Implement priority-based sorting for subscription matches
- Add structured logging for orchestrator operations
- Integrate metrics collection for monitoring
- Add configuration options for orchestrator behavior
- Maintain backward compatibility with simple pass-through behavior

#### Files to Modify
- `internal/engine/orchestrator.go` - Add filtering and prioritization logic
- `internal/engine/orchestrator_test.go` - Add enhanced orchestrator tests

#### Testing Requirements
- Unit tests for subscription filtering logic
- Priority sorting tests with various subscription configurations
- Integration tests with FanOutExecutor
- Performance tests for large subscription sets
- Backward compatibility verification

#### Success Criteria
- Orchestrator provides valuable filtering and prioritization
- Performance remains acceptable for typical workloads
- Enhanced observability through logging and metrics
- No breaking changes to existing Orchestrator interface

---

### Phase 6: Integration Testing and Performance Optimization
**Goal**: End-to-end testing and performance tuning  
**Status**: ⏳ Pending  
**Health Check**: All tests pass + comprehensive integration tests

#### Implementation Details
- Create comprehensive integration test scenarios
- Test complete diamond dependency workflows end-to-end
- Performance testing with large subscription sets
- Memory usage optimization and monitoring
- Cache tuning based on real-world usage patterns
- Load testing with concurrent subscription processing

#### Files to Modify
- Create new integration test files
- Update existing test files with additional scenarios
- Add performance benchmarking utilities

#### Testing Requirements
- End-to-end diamond dependency resolution tests
- Performance benchmarks for CEL caching effectiveness
- Memory leak detection and resource cleanup verification
- Concurrent access testing for thread safety
- Large-scale subscription processing tests

#### Success Criteria
- All advanced features work correctly in realistic scenarios
- Performance improvements are measurable and significant
- No memory leaks or resource issues under load
- System remains stable under concurrent access

---

## Risk Mitigation

### Technical Risks
1. **CEL Caching Memory Usage**: Implement LRU eviction and size limits
2. **Thread Safety**: Use proper synchronization for concurrent access
3. **Performance Regression**: Maintain benchmarks and performance tests
4. **Backward Compatibility**: Comprehensive regression testing

### Integration Risks
1. **State Management Conflicts**: Careful integration with existing idempotency
2. **Discovery Order Dependencies**: Ensure deterministic first-wins behavior
3. **Configuration Validation**: Clear error messages for invalid configurations

## Success Metrics

### Functional Metrics
- ✅ CEL filters are evaluated correctly
- ✅ Schema versions are validated correctly  
- ✅ Diamond dependencies are resolved with first-wins rule
- ✅ All existing functionality remains unchanged

### Performance Metrics
- 🎯 CEL evaluation performance improvement (target: 50% faster for repeated expressions)
- 🎯 Memory usage remains bounded (target: <10MB for typical workloads)
- 🎯 No measurable performance regression in existing functionality

### Quality Metrics
- 🎯 Test coverage maintained at 76%+ overall
- 🎯 All linter checks pass
- 🎯 No functional regressions in existing features
- 🎯 Comprehensive integration test coverage for diamond scenarios

## Dependencies and Assumptions

### Dependencies
- Existing idempotency infrastructure from issue #134 ✅
- SubscriptionEvaluator and Orchestrator foundations ✅
- Complete configuration schema support ✅

### Assumptions
- Discovery order is deterministic and consistent
- CEL expressions in production are reasonable in complexity
- Memory usage for caching is acceptable for CLI workloads
- Diamond dependencies are relatively uncommon edge cases

## Implementation Notes

### Development Approach
- Each phase should leave the codebase in a healthy, compilable state
- Tests must pass after each phase completion
- Commit granularly with clear, focused changes
- Maintain backward compatibility throughout

### Code Quality Standards
- Follow existing code style and patterns
- Add comprehensive documentation for new public interfaces
- Use existing error handling and logging patterns
- Maintain thread safety where required

### Testing Strategy  
- Unit tests for all new logic (Phase 1-5)
- Integration tests for complete scenarios (Phase 6)
- Performance benchmarks for optimization validation
- Regression tests to prevent backsliding