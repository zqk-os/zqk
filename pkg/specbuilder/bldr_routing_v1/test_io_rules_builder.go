package bldr_routing_v1

import (
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/specbuilder/routing_builders"
)

// TestIoRulesBuilder builds the test_io_rules routing rules at version v1_0_0
// File: bldr_routing_v1/test_io_rules_builder.go - version is encoded in package/directory name
type TestIoRulesBuilder struct {
	*routing_builders.BaseRoutingRuleBuilder
}

// NewTestIoRulesBuilder creates a new builder for test_io_rules routing rules version v1_0_0
func NewTestIoRulesBuilder() *TestIoRulesBuilder {
	builder := &TestIoRulesBuilder{
		BaseRoutingRuleBuilder: routing_builders.NewBaseRoutingRuleBuilder("test_io_rules", "v1_0_0"),
	}

	// Add routing rules
	builder.addTestIoRulesRules()

	return builder
}

// addTestIoRulesRules adds the test_io_rules routing rules
func (b *TestIoRulesBuilder) addTestIoRulesRules() {

	b.AddRule(map[string]any{
		"actions": []any{
			map[string]any{
				"auth": map[string]any{
					"token":              "${TEST_IO_WEBHOOK_TOKEN}",
					objects.FieldKeyType: "bearer",
				},
				"method":   "POST",
				"protocol": "webhook",
				"retry": map[string]any{
					"backoff_ms":   500,
					"max_attempts": 2,
				},
				objects.FieldKeyTarget: "${TEST_IO_WEBHOOK_URL}",
				"timeout_ms":           3000,
			},
		},
		objects.FieldKeyDescription: "Routes all test I/O messages to monitoring webhook for verification",
		objects.FieldKeyEnabled:     true,
		"match": map[string]any{
			objects.FieldKeyEventType: "test_io_message",
			objects.FieldKeyJobType:   "test_io",
			objects.FieldKeySource:    "scheduler",
		},
		objects.FieldKeyName:     "route_test_io_messages",
		objects.FieldKeyPriority: 100,
	})
	b.AddRule(map[string]any{
		"actions": []any{
			map[string]any{
				"method":   "POST",
				"protocol": "webhook",
				"retry": map[string]any{
					"backoff_ms":   200,
					"max_attempts": 1,
				},
				objects.FieldKeyTarget: "${LOGGING_WEBHOOK_URL}",
				"timeout_ms":           2000,
			},
		},
		objects.FieldKeyDescription: "Routes log level test messages to logging endpoint",
		objects.FieldKeyEnabled:     true,
		"match": map[string]any{
			objects.FieldKeyEventType: "test_io_log_level",
			objects.FieldKeySource:    "scheduler",
		},
		objects.FieldKeyName:     "route_test_io_log_levels",
		objects.FieldKeyPriority: 90,
	})
	b.AddRule(map[string]any{
		"actions": []any{
			map[string]any{
				"method":   "POST",
				"protocol": "webhook",
				"retry": map[string]any{
					"backoff_ms":   1000,
					"max_attempts": 3,
				},
				objects.FieldKeyTarget: "${ALERT_WEBHOOK_URL}",
				"timeout_ms":           5000,
			},
		},
		objects.FieldKeyDescription: "Routes test I/O alert messages to alerting endpoint",
		objects.FieldKeyEnabled:     false,
		"match": map[string]any{
			objects.FieldKeyEventType: "test_io_alert",
			objects.FieldKeySeverity:  "high",
			objects.FieldKeySource:    "scheduler",
		},
		objects.FieldKeyName:     "route_test_io_alerts",
		objects.FieldKeyPriority: 150,
	})
	b.AddRule(map[string]any{
		"actions": []any{
			map[string]any{
				"method":               "POST",
				"protocol":             "webhook",
				objects.FieldKeyTarget: "${STATUS_WEBHOOK_URL}",
				"timeout_ms":           2000,
			},
		},
		objects.FieldKeyDescription: "Routes test I/O status messages to status monitoring",
		objects.FieldKeyEnabled:     false,
		"match": map[string]any{
			objects.FieldKeyEventType: "test_io_status",
			objects.FieldKeySource:    "scheduler",
		},
		objects.FieldKeyName:     "route_test_io_status",
		objects.FieldKeyPriority: 80,
	})
	b.AddRule(map[string]any{
		"actions": []any{
			map[string]any{
				"args": []any{
					"--level",
					"info",
					"--format",
					"json",
				},
				"protocol":             "command",
				objects.FieldKeyTarget: "logger",
				"timeout_ms":           1000,
			},
		},
		objects.FieldKeyDescription: "Routes test I/O messages to local logger command (for development)",
		objects.FieldKeyEnabled:     false,
		"match": map[string]any{
			objects.FieldKeyJobType: "test_io",
			objects.FieldKeySource:  "scheduler",
		},
		objects.FieldKeyName:     "route_test_io_to_logger",
		objects.FieldKeyPriority: 50,
	})
}

func init() {
	routing_builders.RegisterBuilder(NewTestIoRulesBuilder())
}
