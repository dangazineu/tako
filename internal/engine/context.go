package engine

import (
	"fmt"
	"strings"
	"time"
)

// ContextBuilder helps build template contexts for different execution scenarios.
type ContextBuilder struct {
	inputs      map[string]string
	stepOutputs map[string]map[string]string
	event       *EventContext
	trigger     *TriggerContext
}

// NewContextBuilder creates a new context builder.
func NewContextBuilder() *ContextBuilder {
	return &ContextBuilder{
		inputs:      make(map[string]string),
		stepOutputs: make(map[string]map[string]string),
	}
}

// WithInputs sets the workflow inputs.
func (cb *ContextBuilder) WithInputs(inputs map[string]string) *ContextBuilder {
	cb.inputs = inputs
	return cb
}

// WithStepOutputs sets the step outputs.
func (cb *ContextBuilder) WithStepOutputs(stepOutputs map[string]map[string]string) *ContextBuilder {
	cb.stepOutputs = stepOutputs
	return cb
}

// WithEvent sets the event context for subscription-triggered workflows.
func (cb *ContextBuilder) WithEvent(eventType, source string, payload map[string]interface{}) *ContextBuilder {
	cb.event = &EventContext{
		Type:      eventType,
		Payload:   payload,
		Source:    source,
		Timestamp: time.Now(),
	}
	return cb
}

// WithEventVersion sets the event schema version.
func (cb *ContextBuilder) WithEventVersion(version string) *ContextBuilder {
	if cb.event != nil {
		cb.event.Version = version
	}
	return cb
}

// WithLegacyTrigger sets the legacy trigger context for compatibility.
func (cb *ContextBuilder) WithLegacyTrigger(artifacts []ArtifactInfo) *ContextBuilder {
	cb.trigger = &TriggerContext{
		Artifacts: artifacts,
	}
	return cb
}

// Build creates the final template context.
func (cb *ContextBuilder) Build() *TemplateContext {
	return &TemplateContext{
		Inputs:  cb.inputs,
		Steps:   cb.stepOutputs,
		Event:   cb.event,
		Trigger: cb.trigger,
	}
}

// eventField extracts a field from the event payload.
func eventField(field string, event interface{}) interface{} {
	eventCtx, ok := event.(*EventContext)
	if !ok || eventCtx == nil || eventCtx.Payload == nil {
		return nil
	}

	return getNestedField(eventCtx.Payload, field)
}

// eventHasField checks if the event payload contains a specific field.
func eventHasField(field string, event interface{}) bool {
	eventCtx, ok := event.(*EventContext)
	if !ok || eventCtx == nil || eventCtx.Payload == nil {
		return false
	}

	return hasNestedField(eventCtx.Payload, field)
}

// eventFilter filters event payload based on criteria.
func eventFilter(criteria string, event interface{}) map[string]interface{} {
	eventCtx, ok := event.(*EventContext)
	if !ok || eventCtx == nil || eventCtx.Payload == nil {
		return map[string]interface{}{}
	}

	// Simple filtering based on key presence
	result := make(map[string]interface{})
	for key, value := range eventCtx.Payload {
		if strings.Contains(key, criteria) {
			result[key] = value
		}
	}

	return result
}

// getNestedField extracts a nested field from a map using dot notation.
func getNestedField(data map[string]interface{}, field string) interface{} {
	if field == "" {
		return data
	}

	parts := strings.Split(field, ".")
	current := data

	for i, part := range parts {
		if current == nil {
			return nil
		}

		value, exists := current[part]
		if !exists {
			return nil
		}

		// If this is the last part, return the value
		if i == len(parts)-1 {
			return value
		}

		// Otherwise, continue traversing
		if nextMap, ok := value.(map[string]interface{}); ok {
			current = nextMap
		} else {
			return nil
		}
	}

	return current
}

// hasNestedField checks if a nested field exists using dot notation.
func hasNestedField(data map[string]interface{}, field string) bool {
	if field == "" {
		return true
	}

	parts := strings.Split(field, ".")
	current := data

	for i, part := range parts {
		if current == nil {
			return false
		}

		value, exists := current[part]
		if !exists {
			return false
		}

		// If this is the last part, field exists
		if i == len(parts)-1 {
			return true
		}

		// Otherwise, continue traversing
		if nextMap, ok := value.(map[string]interface{}); ok {
			current = nextMap
		} else {
			return false
		}
	}

	return true
}

// ValidateContext performs validation on a template context.
func ValidateContext(context *TemplateContext) error {
	if context == nil {
		return fmt.Errorf("template context cannot be nil")
	}

	// Validate inputs
	if context.Inputs == nil {
		context.Inputs = make(map[string]string)
	}

	// Validate step outputs
	if context.Steps == nil {
		context.Steps = make(map[string]map[string]string)
	}

	// Validate event context if present
	if context.Event != nil {
		if err := validateEventContext(context.Event); err != nil {
			return fmt.Errorf("invalid event context: %v", err)
		}
	}

	// Validate trigger context if present
	if context.Trigger != nil {
		if err := validateTriggerContext(context.Trigger); err != nil {
			return fmt.Errorf("invalid trigger context: %v", err)
		}
	}

	return nil
}

// validateEventContext validates an event context.
func validateEventContext(event *EventContext) error {
	if event.Type == "" {
		return fmt.Errorf("event type cannot be empty")
	}

	if event.Source == "" {
		return fmt.Errorf("event source cannot be empty")
	}

	if event.Payload == nil {
		event.Payload = make(map[string]interface{})
	}

	if event.Timestamp.IsZero() {
		return fmt.Errorf("event timestamp cannot be zero")
	}

	return nil
}

// validateTriggerContext validates a trigger context.
func validateTriggerContext(trigger *TriggerContext) error {
	if trigger.Artifacts == nil {
		trigger.Artifacts = []ArtifactInfo{}
		return nil
	}

	for i, artifact := range trigger.Artifacts {
		if artifact.Name == "" {
			return fmt.Errorf("artifact %d: name cannot be empty", i)
		}
		if artifact.Version == "" {
			return fmt.Errorf("artifact %d: version cannot be empty", i)
		}
	}

	return nil
}

// MergeContexts merges multiple template contexts, with later contexts taking precedence.
func MergeContexts(contexts ...*TemplateContext) *TemplateContext {
	if len(contexts) == 0 {
		return &TemplateContext{
			Inputs: make(map[string]string),
			Steps:  make(map[string]map[string]string),
		}
	}

	result := &TemplateContext{
		Inputs: make(map[string]string),
		Steps:  make(map[string]map[string]string),
	}

	for _, ctx := range contexts {
		if ctx == nil {
			continue
		}

		// Merge inputs
		for k, v := range ctx.Inputs {
			result.Inputs[k] = v
		}

		// Merge step outputs
		for stepID, outputs := range ctx.Steps {
			if result.Steps[stepID] == nil {
				result.Steps[stepID] = make(map[string]string)
			}
			for k, v := range outputs {
				result.Steps[stepID][k] = v
			}
		}

		// Event and Trigger contexts are not merged, last one wins
		if ctx.Event != nil {
			result.Event = ctx.Event
		}
		if ctx.Trigger != nil {
			result.Trigger = ctx.Trigger
		}
	}

	return result
}

// CloneContext creates a deep copy of a template context.
func CloneContext(ctx *TemplateContext) *TemplateContext {
	if ctx == nil {
		return nil
	}

	result := &TemplateContext{
		Inputs: make(map[string]string),
		Steps:  make(map[string]map[string]string),
	}

	// Copy inputs
	for k, v := range ctx.Inputs {
		result.Inputs[k] = v
	}

	// Copy step outputs
	for stepID, outputs := range ctx.Steps {
		result.Steps[stepID] = make(map[string]string)
		for k, v := range outputs {
			result.Steps[stepID][k] = v
		}
	}

	// Clone event context
	if ctx.Event != nil {
		result.Event = &EventContext{
			Type:      ctx.Event.Type,
			Source:    ctx.Event.Source,
			Timestamp: ctx.Event.Timestamp,
			Version:   ctx.Event.Version,
			Payload:   clonePayload(ctx.Event.Payload),
		}
	}

	// Clone trigger context
	if ctx.Trigger != nil {
		result.Trigger = &TriggerContext{
			Artifacts: make([]ArtifactInfo, len(ctx.Trigger.Artifacts)),
		}
		copy(result.Trigger.Artifacts, ctx.Trigger.Artifacts)
	}

	return result
}

// clonePayload creates a deep copy of event payload.
func clonePayload(payload map[string]interface{}) map[string]interface{} {
	if payload == nil {
		return nil
	}

	result := make(map[string]interface{})
	for k, v := range payload {
		result[k] = cloneValue(v)
	}
	return result
}

// cloneValue creates a deep copy of an interface{} value.
func cloneValue(v interface{}) interface{} {
	if v == nil {
		return nil
	}

	switch val := v.(type) {
	case map[string]interface{}:
		result := make(map[string]interface{})
		for k, v := range val {
			result[k] = cloneValue(v)
		}
		return result
	case []interface{}:
		result := make([]interface{}, len(val))
		for i, v := range val {
			result[i] = cloneValue(v)
		}
		return result
	default:
		// For primitive types, return as-is (they're copied by value)
		return v
	}
}
