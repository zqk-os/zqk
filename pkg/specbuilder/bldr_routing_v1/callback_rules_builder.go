package bldr_routing_v1

import (
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/specbuilder/routing_builders"
)

// CallbackRulesBuilder builds the callback_rules routing rules at version v1_0_0
// File: bldr_routing_v1/callback_rules_builder.go - version is encoded in package/directory name
type CallbackRulesBuilder struct {
	*routing_builders.BaseRoutingRuleBuilder
}

// NewCallbackRulesBuilder creates a new builder for callback_rules routing rules version v1_0_0
func NewCallbackRulesBuilder() *CallbackRulesBuilder {
	builder := &CallbackRulesBuilder{
		BaseRoutingRuleBuilder: routing_builders.NewBaseRoutingRuleBuilder("callback_rules", "v1_0_0"),
	}

	// Add routing rules
	builder.addCallbackRulesRules()

	return builder
}

// addCallbackRulesRules adds the callback_rules routing rules
func (b *CallbackRulesBuilder) addCallbackRulesRules() {

	b.AddRule(map[string]any{
		"actions": []any{
			map[string]any{
				objects.FieldKeyEndpoint: "http://localhost:9090/webhook",
				"method":                 "POST",
				"protocol":               "webhook",
				"retry": map[string]any{
					"backoff_ms":   1000,
					"max_attempts": 3,
				},
				"timeout_ms": 5000,
			},
		},
		objects.FieldKeyDescription: "Routes job completion callbacks to webhook endpoint",
		objects.FieldKeyEnabled:     true,
		"match": map[string]any{
			objects.FieldKeyEventType: "scheduler_job_callback_completion",
			objects.FieldKeySource:    "scheduler",
		},
		objects.FieldKeyName:     "route_completion_callbacks_to_webhook",
		objects.FieldKeyPriority: 100,
	})
	b.AddRule(map[string]any{
		"actions": []any{
			map[string]any{
				objects.FieldKeyEndpoint: "http://localhost:9090/webhook",
				"method":                 "POST",
				"protocol":               "webhook",
				"retry": map[string]any{
					"backoff_ms":   1000,
					"max_attempts": 5,
				},
				"timeout_ms": 5000,
			},
		},
		objects.FieldKeyDescription: "Routes job error callbacks to webhook endpoint",
		objects.FieldKeyEnabled:     true,
		"match": map[string]any{
			objects.FieldKeyEventType: "scheduler_job_callback_error",
			objects.FieldKeySource:    "scheduler",
		},
		objects.FieldKeyName:     "route_error_callbacks_to_webhook",
		objects.FieldKeyPriority: 100,
	})
	b.AddRule(map[string]any{
		"actions": []any{
			map[string]any{
				objects.FieldKeyEndpoint: "http://localhost:9090/webhook",
				"method":                 "POST",
				"protocol":               "webhook",
				"retry": map[string]any{
					"backoff_ms":   500,
					"max_attempts": 2,
				},
				"timeout_ms": 3000,
			},
		},
		objects.FieldKeyDescription: "Routes job status callbacks to webhook endpoint",
		objects.FieldKeyEnabled:     true,
		"match": map[string]any{
			objects.FieldKeyEventType: "scheduler_job_callback_status",
			objects.FieldKeySource:    "scheduler",
		},
		objects.FieldKeyName:     "route_status_callbacks_to_webhook",
		objects.FieldKeyPriority: 100,
	})
}

func init() {
	routing_builders.RegisterBuilder(NewCallbackRulesBuilder())
}
