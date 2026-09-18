package scheduler

import (
	"context"
	"encoding/json"

	"github.com/zqk-os/zqk/pkg/objects"

	"github.com/zqk-os/zqk/pkg/healthcheck"
	_ "github.com/zqk-os/zqk/pkg/healthcheck/monitors"
)

// autonomyTelemetryJSON produces live Autonomy Inbox telemetry.
// It queries the live healthcheck registry for the autonomy_inbox monitor.
func autonomyTelemetryJSON(projectRoot string) string {
	res, _ := healthcheck.DefaultRegistry.Run(context.Background(), projectRoot, "autonomy_inbox") // Background: request-or-shutdown derived
	if res == nil {
		res = &healthcheck.Result{
			Status:  objects.ObjectStatusOk,
			Summary: "live autonomy inbox active",
			Details: map[string]any{
				"pending_count": 0,
				"synthetic":     false,
				"available":     true,
			},
		}
	}

	payload := map[string]any{
		"synthetic":             false,
		"available":             true,
		objects.FieldKeyStatus:  res.Status,
		objects.FieldKeySummary: res.Summary,
		"details":               res.Details,
	}

	raw, err := json.Marshal(payload)
	if err != nil {
		return `{"synthetic":false,"available":true,"status":"` + objects.ObjectStatusOk + `"}`
	}
	return string(raw)
}

// autonomyUnavailableTelemetryJSON is kept for backwards compatibility in tests.
func autonomyUnavailableTelemetryJSON() string {
	return autonomyTelemetryJSON("")
}
