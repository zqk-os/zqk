package bldr_routing_v1

import (
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/specbuilder/routing_builders"
)

// DefaultRulesBuilder builds the default_rules routing rules at version v1_0_0
// File: bldr_routing_v1/default_rules_builder.go - version is encoded in package/directory name
type DefaultRulesBuilder struct {
	*routing_builders.BaseRoutingRuleBuilder
}

// NewDefaultRulesBuilder creates a new builder for default_rules routing rules version v1_0_0
func NewDefaultRulesBuilder() *DefaultRulesBuilder {
	builder := &DefaultRulesBuilder{
		BaseRoutingRuleBuilder: routing_builders.NewBaseRoutingRuleBuilder("default_rules", "v1_0_0"),
	}

	// Add routing rules
	builder.addDefaultRulesRules()

	return builder
}

// addDefaultRulesRules adds the default_rules routing rules
func (b *DefaultRulesBuilder) addDefaultRulesRules() {

	b.AddRule(map[string]any{
		"actions": []any{
			map[string]any{
				"auth": map[string]any{
					"token":              "${ERROR_WEBHOOK_TOKEN}",
					objects.FieldKeyType: "bearer",
				},
				"method":   "POST",
				"protocol": "webhook",
				"retry": map[string]any{
					"backoff_ms":   1000,
					"max_attempts": 3,
				},
				objects.FieldKeyTarget: "${ERROR_WEBHOOK_URL}",
				"timeout_ms":           5000,
			},
		},
		objects.FieldKeyDescription: "Routes all failed job executions to error webhook endpoint",
		objects.FieldKeyEnabled:     true,
		"match": map[string]any{
			objects.FieldKeyEventType: "scheduler_job_failed",
			objects.FieldKeySeverity:  "high",
			objects.FieldKeySource:    "scheduler",
		},
		objects.FieldKeyName:     "route_job_failures",
		objects.FieldKeyPriority: 100,
	})
	b.AddRule(map[string]any{
		"actions": []any{
			map[string]any{
				"method":   "POST",
				"protocol": "webhook",
				"retry": map[string]any{
					"backoff_ms":   500,
					"max_attempts": 2,
				},
				objects.FieldKeyTarget: "${MONITORING_WEBHOOK_URL}",
				"timeout_ms":           3000,
			},
		},
		objects.FieldKeyDescription: "Routes successful job completions to monitoring webhook",
		objects.FieldKeyEnabled:     false,
		"match": map[string]any{
			objects.FieldKeyEventType: "scheduler_job_completed",
			objects.FieldKeySource:    "scheduler",
		},
		objects.FieldKeyName:     "route_job_completions",
		objects.FieldKeyPriority: 50,
	})
	b.AddRule(map[string]any{
		"actions": []any{
			map[string]any{
				"method":   "POST",
				"protocol": "webhook",
				"retry": map[string]any{
					"backoff_ms":   2000,
					"max_attempts": 5,
				},
				objects.FieldKeyTarget: "${ALERT_WEBHOOK_URL}",
				"timeout_ms":           5000,
			},
		},
		objects.FieldKeyDescription: "Routes high-severity events to alerting system",
		objects.FieldKeyEnabled:     false,
		"match": map[string]any{
			objects.FieldKeySeverity: "high",
			objects.FieldKeySource:   "scheduler",
		},
		objects.FieldKeyName:     "route_high_severity_alerts",
		objects.FieldKeyPriority: 200,
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
				"timeout_ms":           2000,
			},
		},
		objects.FieldKeyDescription: "Routes maintenance category jobs to local logging command",
		objects.FieldKeyEnabled:     false,
		"match": map[string]any{
			"job_category":         "maintenance",
			objects.FieldKeySource: "scheduler",
		},
		objects.FieldKeyName:     "route_maintenance_jobs_to_log",
		objects.FieldKeyPriority: 30,
	})
	b.AddRule(map[string]any{
		"actions": []any{
			map[string]any{
				"protocol":             "event",
				objects.FieldKeyTarget: "cache_monitoring",
				"transform": map[string]any{
					"add_fields": map[string]any{
						"cache_type": "${payload.job_type}",
						"monitored":  true,
					},
				},
			},
		},
		objects.FieldKeyDescription: "Routes cache prewarm job events to cache monitoring",
		objects.FieldKeyEnabled:     false,
		"match": map[string]any{
			objects.FieldKeyJobType: "cache_prewarm",
			objects.FieldKeySource:  "scheduler",
		},
		objects.FieldKeyName:     "route_cache_prewarm_events",
		objects.FieldKeyPriority: 40,
	})
}

func init() {
	routing_builders.RegisterBuilder(NewDefaultRulesBuilder())
}
