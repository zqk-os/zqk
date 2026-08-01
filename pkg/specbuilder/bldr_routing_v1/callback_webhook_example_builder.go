package bldr_routing_v1

import (
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/specbuilder/routing_builders"
)

// CallbackWebhookExampleBuilder builds the callback_webhook_example routing rules at version v1_0_0
// File: bldr_routing_v1/callback_webhook_example_builder.go - version is encoded in package/directory name
type CallbackWebhookExampleBuilder struct {
	*routing_builders.BaseRoutingRuleBuilder
}

// NewCallbackWebhookExampleBuilder creates a new builder for callback_webhook_example routing rules version v1_0_0
func NewCallbackWebhookExampleBuilder() *CallbackWebhookExampleBuilder {
	builder := &CallbackWebhookExampleBuilder{
		BaseRoutingRuleBuilder: routing_builders.NewBaseRoutingRuleBuilder("callback_webhook_example", "v1_0_0"),
	}

	// Add routing rules
	builder.addCallbackWebhookExampleRules()

	return builder
}

// addCallbackWebhookExampleRules adds the callback_webhook_example routing rules
func (b *CallbackWebhookExampleBuilder) addCallbackWebhookExampleRules() {

	b.AddRule(map[string]any{
		"actions": []any{
			map[string]any{
				objects.FieldKeyEndpoint: "${CALLBACK_WEBHOOK_URL}",
				"protocol":               "webhook",
				"retry": map[string]any{
					"backoff":       "exponential",
					"initial_delay": "1s",
					"max_attempts":  3,
				},
				"timeout": "5s",
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
				objects.FieldKeyEndpoint: "${CALLBACK_WEBHOOK_URL}",
				"protocol":               "webhook",
				"retry": map[string]any{
					"backoff":       "exponential",
					"initial_delay": "1s",
					"max_attempts":  5,
				},
				"timeout": "5s",
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
				objects.FieldKeyEndpoint: "${CALLBACK_WEBHOOK_URL}",
				"protocol":               "webhook",
				"retry": map[string]any{
					"backoff":       "exponential",
					"initial_delay": "500ms",
					"max_attempts":  2,
				},
				"timeout": "5s",
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
	b.AddRule(map[string]any{
		"actions": []any{
			map[string]any{
				objects.FieldKeyEndpoint: "${CALLBACK_WEBHOOK_URL}",
				"protocol":               "webhook",
				"retry": map[string]any{
					"backoff":       "exponential",
					"initial_delay": "1s",
					"max_attempts":  3,
				},
				"timeout": "5s",
			},
		},
		objects.FieldKeyDescription: "Routes all callback types to a single webhook endpoint",
		objects.FieldKeyEnabled:     false,
		"match": map[string]any{
			objects.FieldKeyEventType: "scheduler_job_callback",
			objects.FieldKeySource:    "scheduler",
		},
		objects.FieldKeyName:     "route_all_callbacks_to_webhook",
		objects.FieldKeyPriority: 50,
	})
	b.AddRule(map[string]any{
		"actions": []any{
			map[string]any{
				objects.FieldKeyEndpoint: "/usr/local/bin/process-callback.sh",
				"protocol":               "command",
				"timeout":                "10s",
			},
		},
		objects.FieldKeyDescription: "Routes callbacks to a local command for processing",
		objects.FieldKeyEnabled:     false,
		"match": map[string]any{
			objects.FieldKeyEventType: "scheduler_job_callback_completion",
			"job_category":            "maintenance",
			objects.FieldKeySource:    "scheduler",
		},
		objects.FieldKeyName:     "route_callbacks_to_command",
		objects.FieldKeyPriority: 75,
	})
}

func init() {
	routing_builders.RegisterBuilder(NewCallbackWebhookExampleBuilder())
}
