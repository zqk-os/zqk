package system

import (
	"bytes"
	"fmt"

	"github.com/spf13/cobra"
	"github.com/zqk-os/zqk/pkg/cliapp"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
	"github.com/zqk-os/zqk/pkg/cli/bldr_cli_cmd_v1"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/storage"
)

// NewAuditBufferCmd creates a command to manage the audit event buffer
func NewAuditBufferCmd() *cobra.Command {
	helpBuilder := clipkg.DynamicHelpBuilder(
		"Manage audit event aggregation buffer",
		"Manage the in-memory audit event aggregation buffer.",
		"",
		"The audit event buffer aggregates low-severity, high-volume events (e.g., cache operations)",
		"into aggregated_summary events to reduce storage overhead while maintaining audit trail integrity.",
	).
		AddExample("View buffer statistics", "%s system audit-buffer stats").
		AddExample("Flush buffer immediately (write aggregated summaries)", "%s system audit-buffer flush").
		AddExample("Enable buffer", "%s system audit-buffer enable").
		AddExample("Disable buffer", "%s system audit-buffer disable").
		ExcludeCommonFlags()

	cmd := clipkg.ApplyBuilder(bldr_cli_cmd_v1.NewSystemAuditBufferCommandBuilder(

	// Apply help builder to command
	), &cobra.Command{
		Use: "audit-buffer",
	})

	helpBuilder.ApplyToCommand(cmd)

	cmd.AddCommand(NewAuditBufferStatsCmd())
	cmd.AddCommand(NewAuditBufferFlushCmd())
	cmd.AddCommand(NewAuditBufferEnableCmd())
	cmd.AddCommand(NewAuditBufferDisableCmd())

	return cmd
}

// NewAuditBufferStatsCmd creates a command to view buffer statistics
func NewAuditBufferStatsCmd() *cobra.Command {
	helpBuilder := clipkg.DynamicHelpBuilder(
		"View audit buffer statistics",
		"View current statistics about the audit event buffer, including buffered groups and event counts.",
	).
		ExcludeCommonFlags()

	cmd := clipkg.ApplyBuilder(bldr_cli_cmd_v1.NewSystemStatsCommandBuilder(), &cobra.Command{
		Use:  "stats",
		Args: cobra.NoArgs,
		RunE: runAuditBufferStats,
	})

	// Apply help builder to command
	helpBuilder.ApplyToCommand(cmd)

	cli.AddCommonFlags(cmd)
	return cmd
}

// NewAuditBufferFlushCmd creates a command to flush the buffer
func NewAuditBufferFlushCmd() *cobra.Command {
	helpBuilder := clipkg.DynamicHelpBuilder(
		"Flush audit buffer immediately",
		"Flush all buffered events as aggregated_summary audit events immediately.",
	).
		ExcludeCommonFlags()

	cmd := clipkg.ApplyBuilder(bldr_cli_cmd_v1.NewSystemFlushCommandBuilder(), &cobra.Command{
		Use:  "flush",
		Args: cobra.NoArgs,
		RunE: runAuditBufferFlush,
	})

	// Apply help builder to command
	helpBuilder.ApplyToCommand(cmd)

	cli.AddCommonFlags(cmd)
	return cmd
}

// NewAuditBufferEnableCmd creates a command to enable the buffer
func NewAuditBufferEnableCmd() *cobra.Command {
	helpBuilder := clipkg.DynamicHelpBuilder(
		"Enable audit event aggregation buffer",
		"Enable the audit event aggregation buffer. Events will be buffered and aggregated.",
	).
		ExcludeCommonFlags()

	cmd := clipkg.ApplyBuilder(bldr_cli_cmd_v1.NewSystemEnableCommandBuilder(), &cobra.Command{
		Use:  "enable",
		Args: cobra.NoArgs,
		RunE: runAuditBufferEnable,
	})

	// Apply help builder to command
	helpBuilder.ApplyToCommand(cmd)

	cli.AddCommonFlags(cmd)
	return cmd
}

// NewAuditBufferDisableCmd creates a command to disable the buffer
func NewAuditBufferDisableCmd() *cobra.Command {
	helpBuilder := clipkg.DynamicHelpBuilder(
		"Disable audit event aggregation buffer",
		"Disable the audit event aggregation buffer. Events will be written immediately.",
	).
		ExcludeCommonFlags()

	cmd := clipkg.ApplyBuilder(bldr_cli_cmd_v1.NewSystemDisableCommandBuilder(), &cobra.Command{
		Use:  "disable",
		Args: cobra.NoArgs,
		RunE: runAuditBufferDisable,
	})

	// Apply help builder to command
	helpBuilder.ApplyToCommand(cmd)

	cli.AddCommonFlags(cmd)
	return cmd
}

func runAuditBufferStats(cmd *cobra.Command, args []string) error {
	buffer := storage.GetGlobalAuditEventBuffer()
	stats := buffer.GetBufferStats()

	switch cli.GetFormat(cmd) {
	case cli.FormatJSON, cli.FormatJSONL, cli.FormatYAML:
		return cli.FormatOutput(cmd, stats)
	case cli.FormatTable:
		var buf bytes.Buffer
		buf.WriteString("\nAudit Event Buffer Statistics\n")
		buf.WriteString("=============================\n\n")
		fmt.Fprintf(&buf, "Enabled: %v\n", stats[objects.FieldKeyEnabled])
		fmt.Fprintf(&buf, "Group Count: %v\n", stats["group_count"])
		fmt.Fprintf(&buf, "Total Events: %v\n", stats["total_events"])
		fmt.Fprintf(&buf, "Window Size: %v\n", stats["window_size"])
		fmt.Fprintf(&buf, "Threshold: %v\n\n", stats["threshold"])

		if groups, ok := stats["groups"].([]any); ok && len(groups) > 0 {
			buf.WriteString("Buffered Groups:\n")
			for _, group := range groups {
				g, ok := group.(map[string]any)
				if !ok {
					continue
				}
				fmt.Fprintf(&buf, "  - Key: %v\n", g["key"])
				fmt.Fprintf(&buf, "    Event Type: %v\n", g[objects.FieldKeyEventType])
				fmt.Fprintf(&buf, "    Target Kind: %v\n", g[objects.FieldKeyTargetKind])
				fmt.Fprintf(&buf, "    Count: %v\n", g["count"])
				fmt.Fprintf(&buf, "    First Seen: %v\n", g[objects.FieldKeyFirstSeen])
				fmt.Fprintf(&buf, "    Last Seen: %v\n\n", g[objects.FieldKeyLastSeen])
			}
		} else {
			buf.WriteString("No buffered groups.\n\n")
		}
		return cli.WriteOutput(cmd, buf.Bytes())
	default:
		return errfmt.Errorf("unsupported format: %s", cli.GetFormat(cmd))
	}
}

func runAuditBufferFlush(cmd *cobra.Command, args []string) error {
	buffer := storage.GetGlobalAuditEventBuffer()
	if err := buffer.Flush(); err != nil {
		return errfmt.Newf("failed to flush buffer").Wrap(err)
	}

	msg := "✅ Buffer flushed successfully.\n"
	return cli.WriteOutput(cmd, []byte(msg))
}

func runAuditBufferEnable(cmd *cobra.Command, args []string) error {
	buffer := storage.GetGlobalAuditEventBuffer()
	buffer.SetEnabled(true)
	msg := "✅ Audit event aggregation buffer enabled.\n"
	return cli.WriteOutput(cmd, []byte(msg))
}

func runAuditBufferDisable(cmd *cobra.Command, args []string) error {
	// Flush buffer before disabling
	buffer := storage.GetGlobalAuditEventBuffer()
	if err := buffer.Flush(); err != nil {
		return errfmt.Newf("failed to flush buffer before disabling").Wrap(err)
	}
	buffer.SetEnabled(false)
	msg := "✅ Audit event aggregation buffer disabled (buffer flushed).\n"
	return cli.WriteOutput(cmd, []byte(msg))
}
