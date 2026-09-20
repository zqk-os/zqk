package scheduler

import (
	"context"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/zqk-os/zqk/internal/cli"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
	"github.com/zqk-os/zqk/pkg/cli/bldr_cli_cmd_v1"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/paths"
	schedulerpkg "github.com/zqk-os/zqk/pkg/scheduler"
)

// NewEscalateCmd creates the `scheduler escalate` diagnostic and dispatch command.
func NewEscalateCmd() *cobra.Command {
	cmd := bldr_cli_cmd_v1.NewSchedulerEscalateCommandBuilder()
	cmd.SilenceUsage = true
	cmd.RunE = runEscalate
	return cmd
}

func maskWebhookURL(url string) string {
	if url == "" {
		return ""
	}
	parts := strings.Split(url, "/")
	if len(parts) > 2 {
		// Mask the last token (the secret key)
		parts[len(parts)-1] = "********"
		return strings.Join(parts, "/")
	}
	return "********"
}

func runEscalate(cmd *cobra.Command, _ []string) error {
	projectRoot := cli.ResolveProjectRoot(".")
	if projectRoot == "" {
		if ctx := cli.GetContext(cmd); ctx != nil && ctx.ProjectRoot != "" {
			projectRoot = ctx.ProjectRoot
		}
	}

	var flags clipkg.FlagBag
	dryRun := flags.Bool(cmd, "dry-run")
	testMode := flags.Bool(cmd, "test")
	severityStr := flags.String(cmd, "severity")
	title := flags.String(cmd, "title")
	issues := flags.StringSlice(cmd, "issue")
	suggestedActions := flags.StringSlice(cmd, "suggested-action")
	slackOnly := flags.Bool(cmd, "slack-only")
	inboxOnly := flags.Bool(cmd, "inbox-only")
	if err := flags.Err(); err != nil {
		return err
	}
	if len(issues) == 0 {
		issues = []string{"Diagnostic probe: testing escalation alert pipeline"}
	}
	if len(suggestedActions) == 0 {
		suggestedActions = []string{"Verify Slack channel receipt and human inbox entry"}
	}

	webhookURL := schedulerpkg.ResolveSlackWebhookURL(projectRoot)
	inboxDir := ""
	if projectRoot != "" {
		inboxDir = filepath.Join(projectRoot, paths.ProjectDataDir, paths.InboxSubdir, "human")
	}

	severity := schedulerpkg.EscalationSeverityWarning
	if strings.EqualFold(severityStr, "critical") {
		severity = schedulerpkg.EscalationSeverityCritical
	}

	notice := schedulerpkg.EscalationNotice{
		Title:            title,
		Severity:         severity,
		Timestamp:        time.Now().UTC().Format(time.RFC3339),
		Issues:           issues,
		SuggestedActions: suggestedActions,
		Context: map[string]any{
			"event_source": "zqk scheduler escalate",
			"project_root": projectRoot,
		},
	}

	report := map[string]any{
		"project_root":           projectRoot,
		"slack_webhook_detected": webhookURL != "",
		"slack_webhook_masked":   maskWebhookURL(webhookURL),
		"inbox_dir":              inboxDir,
		"inbox_available":        inboxDir != "",
		"dry_run":                dryRun,
		"alert_severity":         string(severity),
		"alert_title":            title,
	}

	if dryRun || !testMode {
		report["pipeline_state"] = "configured"
		if webhookURL == "" && inboxDir == "" {
			report["pipeline_state"] = "no_channels_available"
		}
		return cli.FormatOutput(cmd, report)
	}

	// Test dispatch mode
	dispatchedChannels := make(map[string]any)
	hasError := false

	ctx, cancel := context.WithTimeout(cmd.Context(), 15*time.Second)
	defer cancel()

	// Dispatch Slack webhook
	if !inboxOnly && webhookURL != "" {
		provider := &schedulerpkg.WebhookEscalationProvider{WebhookURL: webhookURL}
		start := time.Now()
		err := provider.Escalate(ctx, notice)
		duration := time.Since(start).Round(time.Millisecond)
		if err != nil {
			hasError = true
			dispatchedChannels["slack"] = map[string]any{
				"delivery_state": "failed",
				"error":          err.Error(),
				"duration_ms":    duration.Milliseconds(),
			}
		} else {
			dispatchedChannels["slack"] = map[string]any{
				"delivery_state": "success",
				"duration_ms":    duration.Milliseconds(),
			}
		}
	} else if !inboxOnly && webhookURL == "" {
		dispatchedChannels["slack"] = map[string]any{
			"delivery_state": "skipped",
			"skip_reason":    "no webhook URL configured",
		}
	}

	// Dispatch Inbox
	if !slackOnly && inboxDir != "" {
		provider := &schedulerpkg.InboxEscalationProvider{
			InboxDir:   inboxDir,
			MaxHistory: 10,
		}
		start := time.Now()
		err := provider.Escalate(ctx, notice)
		duration := time.Since(start).Round(time.Millisecond)
		if err != nil {
			hasError = true
			dispatchedChannels["inbox"] = map[string]any{
				"delivery_state": "failed",
				"error":          err.Error(),
				"duration_ms":    duration.Milliseconds(),
			}
		} else {
			dispatchedChannels["inbox"] = map[string]any{
				"delivery_state": "success",
				"latest_file":    filepath.Join(inboxDir, "LATEST_ESCALATION.json"),
				"duration_ms":    duration.Milliseconds(),
			}
		}
	}

	report["channels"] = dispatchedChannels
	if hasError {
		report["pipeline_state"] = "partial_failure"
		_ = cli.FormatOutput(cmd, report)
		return errfmt.Errorf("escalation dispatch encountered errors")
	}

	report["pipeline_state"] = "dispatched"
	return cli.FormatOutput(cmd, report)
}
