package inbox

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/zqk-os/zqk/internal/cli"
	"github.com/zqk-os/zqk/pkg/cli/bldr_cli_cmd_v1"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/storage"
	"github.com/spf13/cobra"
)

// NewListCmd creates the 'inbox list' command that lists pending agent_instruction proposals.
func NewListCmd() *cobra.Command {
	var (
		statusFilter  string
		personaFilter string
	)

	cmd := bldr_cli_cmd_v1.NewInboxListCommandBuilder()
	cli.RequireSession(cmd, true)
	cli.BindAsyncProgress(cmd, func(cmd *cobra.Command, args []string) error {
		ctx := cli.GetContext(cmd)
		if ctx == nil {
			return errfmt.Errorf("failed to get context")
		}

		logger := logging.GetLoggerFromProfile(ctx.Profile)
		secCtx := pkgctx.NewSystemSecurityContext()
		storageCtx := ctx.GetStorageContext()

		store, err := cli.GetObjectStorageForCommand(cmd, ctx.ProjectRoot)
		if err != nil {
			return errfmt.Newf("failed to get storage").Wrap(err)
		}

		// Build filter for agent_instruction objects
		filters := make(map[string]any)
		if statusFilter != "" {
			filters[objects.FieldKeyStatus] = statusFilter
		}
		if personaFilter != "" {
			filters[objects.FieldKeyTargetPersona] = personaFilter
		}

		listFilter := storage.ListFilter{
			Kind:    "agent_instruction",
			Filters: filters,
			SortBy:  objects.FieldKeyCreatedAt,
			SortAsc: false, // newest first
		}

		result, err := store.List(cmd.Context(), secCtx, storageCtx, listFilter)
		if err != nil {
			return errfmt.Newf("failed to query inbox").Wrap(err)
		}

		items := result.Objects

		// Sort by created_at descending (newest first) as backup
		sort.Slice(items, func(i, j int) bool {
			iTime, _ := items[i][objects.FieldKeyCreatedAt].(string)
			jTime, _ := items[j][objects.FieldKeyCreatedAt].(string)
			return iTime > jTime
		})

		format := cli.GetFormat(cmd)
		if format == cli.FormatJSON {
			data, err := json.MarshalIndent(items, "", "  ")
			if err != nil {
				return errfmt.Newf("failed to marshal results").Wrap(err)
			}
			return cli.WriteOutput(cmd, data)
		}

		// Table format (default)
		if len(items) == 0 {
			return cli.WriteOutput(cmd, []byte("Inbox is empty. No agent instructions pending.\n"))
		}

		var sb strings.Builder
		sb.WriteString(fmt.Sprintf("%-40s %-12s %-12s %-40s %s\n",
			"ID", "STATUS", "PERSONA", "SESSION", "INSTRUCTION"))
		sb.WriteString(strings.Repeat("─", 120) + "\n")

		for _, item := range items {
			id, _ := item[objects.FieldKeyID].(string)
			status, _ := item[objects.FieldKeyStatus].(string)
			persona, _ := item[objects.FieldKeyTargetPersona].(string)
			sessionID, _ := item[objects.FieldKeySourceSession].(string)
			instruction, _ := item[objects.FieldKeyInstruction].(string)

			// Truncate long instructions for table display
			if len(instruction) > 40 {
				instruction = instruction[:37] + "..."
			}

			sb.WriteString(fmt.Sprintf("%-40s %-12s %-12s %-40s %s\n",
				id, status, persona, sessionID, instruction))
		}

		sb.WriteString(fmt.Sprintf("\n%d item(s)\n", len(items)))

		logging.Fluent(logger).Info(fmt.Sprintf("Listed %d inbox items", len(items))).Log()

		return cli.WriteOutput(cmd, []byte(sb.String()))
	})

	cmd.Flags().StringVar(&statusFilter, "status", "", "Filter by status (e.g. pending, approved, rejected)")
	cmd.Flags().StringVar(&personaFilter, "persona", "", "Filter by persona ID (e.g. tpm, neuron)")

	cli.AddCommonFlags(cmd)
	return cmd
}
