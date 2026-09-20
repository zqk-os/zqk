package scheduler

import (
	"context"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/zqk-os/zqk/internal/cli"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/paths"
	schedulerpkg "github.com/zqk-os/zqk/pkg/scheduler"
)

// NewEscalateCmd creates the `scheduler escalate` diagnostic and dispatch command.
// TRACK: PRI-SLACK-ALERT-DISPATCH-VERIFY-001 / BLI-1789634862108198000-26daaa5d
func NewEscalateCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "escalate",
		Short: "Diagnostic verification and testing of Slack webhook and inbox escalations",
		Long: "Verify configuration and dispatch diagnostic escalation alerts across Slack webhook " +
			"and human inbox notification channels.",
		SilenceUsage: true,
		RunE:         runEscalate,
	}

	cli.AddCommonFlags(cmd)
	cmd.Flags().Bool("test", false, "Dispatch a diagnostic test alert to active escalation channels")
	cmd.Flags().Bool("dry-run", false, "Inspect and validate escalation channels without sending alerts")
	cmd.Flags().String("severity", "warning", "Escalation severity (warning, critical)")
	cmd.Flags().String("title", "Diagnostic Escalation Test", "Alert title")
	cmd.Flags().StringSlice("issue", []string{"Diagnostic probe: testing escalation alert pipeline"}, "Issue descriptions to include")
	cmd.Flags().StringSlice("suggested-action", []string{"Verify Slack channel receipt and human inbox entry"}, "Suggested actions to include")
	cmd.Flags().Bool("slack-only", false, "Dispatch only to Slack webhook channel")
	cmd.Flags().Bool("inbox-only", false, "Dispatch only to human inbox channel")

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

	dryRun, _ := cmd.Flags().GetBool("dry-run")
	testMode, _ := cmd.Flags().GetBool("test")
	severityStr, _ := cmd.Flags().GetString("severity")
	title, _ := cmd.Flags().GetString("title")
	issues, _ := cmd.Flags().GetStringSlice("issue")
	suggestedActions, _ := cmd.Flags().GetStringSlice("suggested-action")
	slackOnly, _ := cmd.Flags().GetBool("slack-only")
	inboxOnly, _ := cmd.Flags().GetBool("inbox-only")

	webhookURL := schedulerpkg.ResolveSlackWebhookURL(projectRoot)
	inboxDir := ""
	if projectRoot != "" {
		inboxDir = filepath.Join(projectRoot, paths.ProjectDataDir, "inbox", "human")
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
