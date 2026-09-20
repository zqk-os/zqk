package keystore

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	"github.com/zqk-os/zqk/internal/cli"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
	"github.com/zqk-os/zqk/pkg/cli/bldr_cli_cmd_v1"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/storage"
)

// NewListCmd creates a new keystore list command
func NewListCmd() *cobra.Command {
	helpBuilder := clipkg.DynamicHelpBuilder(
		"List keystore entries",
		"List keystore entries for the current user.",
		"",
		"Access control:",
		"  - Users can only see their own entries",
		"  - Admins can see all entries",
		"  - Sensitive fields (credential_hash, salt) are never shown",
	).
		AddExample("List your keystore entries", "%s keystore list").
		AddExample("List in JSON format", "%s keystore list --format json").
		AddExample("List with filtering", "%s keystore list --filter key_type=api_key").
		ExcludeCommonFlags()

	listCmd := clipkg.ApplyBuilder(bldr_cli_cmd_v1.NewKeystoreListCommandBuilder(

	// Apply help builder to command
	), &cobra.Command{
		Use:  "list [flags]",
		RunE: runList,
	})

	helpBuilder.ApplyToCommand(listCmd)

	// Add common flags
	cli.AddCommonFlags(listCmd)

	// Add list-specific flags
	listCmd.Flags().StringArray("filter", []string{}, "Filter by field (format: field=value, can be used multiple times)")
	listCmd.Flags().String("sort-by", "", "Field to sort by (e.g., title, created_at)")
	listCmd.Flags().Bool("sort-asc", true, "Sort ascending (default: true)")
	listCmd.Flags().Int("offset", 0, "Pagination offset")
	listCmd.Flags().Int("limit", 0, "Pagination limit (0 = no limit)")

	return listCmd
}

func runList(cmd *cobra.Command, args []string) error {
	return cli.WithProcessor(func(cmd *cobra.Command, args []string, proc *cli.Processor) error {
		var err error
		_ = err

		proc.Logger().LogDebug("Listing keystore entries")

		// Get security context (for access control)
		secCtx := proc.SecurityContext()
		storageCtx := proc.StorageContext()

		// Build filters
		filters := make(map[string]any)
		filterFlags, _ := cmd.Flags().GetStringArray("filter") //nolint:errcheck // Flag getters don't fail in cobra
		for _, filterStr := range filterFlags {
			parts := strings.SplitN(filterStr, "=", 2)
			if len(parts) == 2 {
				filters[parts[0]] = parts[1]
			}
		}

		// Get sort options
		sortBy, _ := cmd.Flags().GetString("sort-by") //nolint:errcheck // Flag getters don't fail in cobra
		sortAsc, _ := cmd.Flags().GetBool("sort-asc") //nolint:errcheck // Flag getters don't fail in cobra
		offset, _ := cmd.Flags().GetInt("offset")     //nolint:errcheck // Flag getters don't fail in cobra
		limit, _ := cmd.Flags().GetInt("limit")       //nolint:errcheck // Flag getters don't fail in cobra

		// Build list filter
		listFilter := storage.ListFilter{
			Kind:    objects.KindKeystoreEntry,
			Filters: filters,
			SortBy:  sortBy,
			SortAsc: sortAsc,
			Offset:  offset,
			Limit:   limit,
		}

		// List entries
		storageProvider := proc.Storage()
		result, err := storageProvider.List(proc.OperationContext(), secCtx, storageCtx, listFilter)
		if err != nil {
			proc.Logger().LogError("Failed to list keystore entries", err)
			return errfmt.Newf("failed to list keystore entries").Wrap(err)
		}

		if len(result.Objects) == 0 {
			switch proc.Format() {
			case cli.FormatJSON, cli.FormatJSONL, cli.FormatYAML:
				return cli.FormatOutput(cmd, result.Objects)
			default:
				return cli.WriteOutput(cmd, []byte("No keystore entries found.\n"))
			}
		}

		switch proc.Format() {
		case cli.FormatJSON, cli.FormatJSONL, cli.FormatYAML:
			return cli.FormatOutput(cmd, result.Objects)
		default:
			// Table format (default)
			var buf strings.Builder
			fmt.Fprintf(&buf, "Found %d keystore entr%s:\n\n", len(result.Objects), pluralize(len(result.Objects)))

			for i, obj := range result.Objects {
				id, _ := obj[objects.FieldKeyID].(string)
				title, _ := obj[objects.FieldKeyTitle].(string)
				keyType, _ := obj[objects.FieldKeyKeyType].(string)
				accountID, _ := obj[objects.FieldKeyAccountID].(string)
				revoked, _ := obj[objects.FieldKeyRevoked].(bool)
				expiresAt, _ := obj[objects.FieldKeyExpiresAt].(string)
				lastUsedAt, _ := obj[objects.FieldKeyLastUsedAt].(string)

				fmt.Fprintf(&buf, "%d. %s\n", i+1, id)
				if title != emptyValue {
					fmt.Fprintf(&buf, "   Title: %s\n", title)
				}
				fmt.Fprintf(&buf, "   Type: %s\n", keyType)
				fmt.Fprintf(&buf, "   Account: %s\n", accountID)
				if revoked {
					buf.WriteString("   Status: REVOKED\n")
				} else {
					buf.WriteString("   Status: Active\n")
				}
				if expiresAt != emptyValue {
					fmt.Fprintf(&buf, "   Expires: %s\n", expiresAt)
				}
				if lastUsedAt != emptyValue {
					fmt.Fprintf(&buf, "   Last Used: %s\n", lastUsedAt)
				}
				buf.WriteString("\n")
			}

			// Add warning about sensitive fields
			buf.WriteString("⚠️  Sensitive fields (credential_hash, salt) are never displayed\n")
			outputData := []byte(buf.String())
			return cli.WriteOutput(cmd, outputData)
		}
	})(cmd, args)
}

func pluralize(count int) string {
	if count == 1 {
		return "y"
	}
	return "ies"
}
