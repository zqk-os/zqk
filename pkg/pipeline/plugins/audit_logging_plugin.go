package plugins

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"time"

	"github.com/lanceman/zqk/pkg/objects"
)

// AuditLoggingPlugin securely logs all pipeline activities for compliance.
type AuditLoggingPlugin struct {
	logger io.Writer
}

// NewAuditLoggingPlugin creates a new AuditLoggingPlugin.
func NewAuditLoggingPlugin(logger io.Writer) *AuditLoggingPlugin {
	return &AuditLoggingPlugin{
		logger: logger,
	}
}

// Name returns the unique string identifier for the plugin.
func (p *AuditLoggingPlugin) Name() string {
	return "AuditLoggingPlugin"
}

// Execute logs the current payload for audit compliance.
func (p *AuditLoggingPlugin) Execute(ctx context.Context, payload map[string]any) (map[string]any, error) {
	if payload == nil {
		payload = make(map[string]any)
	}

	if p.logger != nil {
		logEntry := map[string]any{
			"timestamp":             time.Now().UTC().Format(time.RFC3339),
			"event":                 "pipeline_activity",
			objects.FieldKeyPayload: payload,
		}
		data, err := json.Marshal(logEntry)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal audit log entry: %w", err)
		}

		_, err = fmt.Fprintln(p.logger, string(data))
		if err != nil {
			return nil, fmt.Errorf("failed to write audit log entry: %w", err)
		}
	}

	payload["audit_logged"] = true

	return payload, nil
}

// Validate ensures the payload was successfully audited.
func (p *AuditLoggingPlugin) Validate(ctx context.Context, output map[string]any) error {
	if logged, ok := output["audit_logged"].(bool); !ok || !logged {
		return fmt.Errorf("validation failed: payload was not audit logged")
	}
	return nil
}
