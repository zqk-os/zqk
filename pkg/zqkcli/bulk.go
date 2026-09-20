package internal

import (
	"bytes"
	"fmt"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	"github.com/zqk-os/zqk/internal/cli"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/logging"
	objkeys "github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/storage"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

// NewInternalBulkCmd creates a new bulk command with subcommands for internal objects
func NewInternalBulkCmd() *cobra.Command {
	helpBuilder := clipkg.DynamicHelpBuilder(
		"Bulk operations for multiple internal objects (admin only)",
		"Perform bulk operations on multiple internal or built-in objects at once.",
		"",
		"Bulk operations are atomic - either all operations succeed or all fail (transaction-based).",
		"For operations that can partially succeed (like bulk-get), errors are reported per-object.",
		"",
		"Available subcommands:",
		"  - create: Create multiple internal objects from a file",
		"  - update: Update multiple internal objects from a file",
		"  - get: Get multiple internal objects by ID",
		"  - delete: Delete multiple internal objects by ID",
	).
		AddExample("Create multiple internal objects", "%s internal bulk create kind_synonym --file synonyms.yaml").
		AddExample("Update multiple built-in objects", "%s internal bulk update --file updates.yaml").
		AddExample("Get multiple internal objects", "%s internal bulk get --ids KSYN-001,KSYN-002").
		AddExample("Delete multiple internal objects", "%s internal bulk delete --ids KSYN-001,KSYN-002 --cascade")

	bulkCmd := &cobra.Command{
		Use: "bulk",
	}

	// Apply help builder to command
	helpBuilder.ApplyToCommand(bulkCmd)

	bulkCmd.AddCommand(NewInternalBulkCreateCmd())
	bulkCmd.AddCommand(NewInternalBulkUpdateCmd())
	bulkCmd.AddCommand(NewInternalBulkGetCmd())
	bulkCmd.AddCommand(NewInternalBulkDeleteCmd())

	return bulkCmd
}

// NewInternalBulkCreateCmd creates a bulk create command for internal objects
func NewInternalBulkCreateCmd() *cobra.Command {
	helpBuilder := clipkg.DynamicHelpBuilder(
		"Create multiple internal objects from a file (admin only)",
		"Create multiple internal objects of the specified kind from a YAML file.",
		"",
		"The file should contain a YAML array of objects:",
		"  - kind: kind_synonym",
		"    kind: backlog_item",
		"    synonym: bli",
		"    ...",
		"  - kind: kind_synonym",
		"    kind: priority_plan",
		"    synonym: pplan",
		"    ...",
	).
		AddExample("Create multiple kind synonyms from file", "%s internal bulk create kind_synonym --file synonyms.yaml").
		AddExample("Dry-run to see what would be created", "%s internal bulk create kind_synonym --file synonyms.yaml --dry-run").
		ExcludeCommonFlags()

	cmd := &cobra.Command{
		Use:  "create <kind> --file <file>",
		Args: cobra.ExactArgs(1),
		RunE: runInternalBulkCreate,
	}

	// Apply help builder to command
	helpBuilder.ApplyToCommand(cmd)

	cli.AddCommonFlags(cmd)
	cmd.Flags().String("file", "", "Path to YAML file containing array of objects (required)")
	cmd.Flags().Bool("dry-run", false, "Show what would be created without actually creating")

	ensureCmdAnnotations(cmd)
	cmd.Annotations[AnnotationKindValidate] = KindValidatePositional0

	return cmd
}

func runInternalBulkCreate(cmd *cobra.Command, args []string) error {
	// Create processor (handles context, storage, security, logging)
	proc, err := cli.NewProcessor(cmd)
	if err != nil {
		return errfmt.Newf("failed to create processor").Wrap(err)
	}

	kind, ok := kindCanonicalFromInternalPRERun(cmd)
	if !ok {
		var rerr error
		kind, rerr = objkeys.ResolveAndValidateKindForProject(proc.ProjectRoot(), args[0])
		if rerr != nil {
			return rerr
		}
	}

	filePath, err := cmd.Flags().GetString("file")
	if err != nil || filePath == emptyValue {
		return errfmt.Errorf("--file is required")
	}

	// Read file
	data, err := fileutil.ReadFile(filePath)
	if err != nil {
		logging.FluentEvent(proc.Logger()).Error("Failed to read file", err).
			File(filePath).
			Log()
		return errfmt.Newf("failed to read file").Wrap(err)
	}

	// Parse YAML array
	var objects []map[string]any
	if err := yaml.Unmarshal(data, &objects); err != nil {
		logging.FluentEvent(proc.Logger()).Error("Failed to parse YAML", err).Log()
		return errfmt.Newf("failed to parse YAML").Wrap(err)
	}

	// Ensure all objects have the correct kind and source_type
	for i, obj := range objects {
		if obj[objkeys.FieldKeyKind] == nil {
			obj[objkeys.FieldKeyKind] = kind
		} else if obj[objkeys.FieldKeyKind] != kind {
			return errfmt.Errorf("object at index %d has kind %v, expected %s", i, obj[objkeys.FieldKeyKind], kind)
		}
		// Ensure source_type is set to internal for internal objects
		if obj[objkeys.FieldKeySourceType] == nil {
			obj[objkeys.FieldKeySourceType] = "internal"
		}
	}

	dryRun, err := cmd.Flags().GetBool("dry-run")
	if err != nil {
		dryRun = false
	}
	if dryRun {
		logging.FluentEvent(proc.Logger()).Info("Dry-run mode: showing what would be created").
			Int("count", len(objects)).
			Log()
		var buf bytes.Buffer
		fmt.Fprintf(&buf, "Would create %d internal objects:\n", len(objects))
		for i, obj := range objects {
			fmt.Fprintf(&buf, "\nObject %d:\n", i+1)
			output, err := yaml.Marshal(obj)
			if err != nil {
				fmt.Fprintf(&buf, "Error marshaling object: %v\n", err)
				continue
			}
			buf.Write(output)
		}
		return cli.WriteOutput(cmd, buf.Bytes())
	}

	result, err := proc.Storage().BulkCreate(proc.OperationContext(), proc.SecurityContext(), objects)
	if err != nil {
		logging.FluentEvent(proc.Logger()).Error("Bulk create failed", err).Log()
		return errfmt.Newf("bulk create failed").Wrap(err)
	}

	// Write-behind: match object bulk create — flush so a follow-up zqk process sees CAS.
	if result != nil && result.SuccessCount > 0 {
		flushCtx, cancelFlush := storage.DurabilityFlushContext()
		defer cancelFlush()
		if err := storage.EnsureCLIObjectMutationVisibleForProvider(flushCtx, proc.Storage(), proc.ProjectRoot(), []string{kind}); err != nil {
			logging.FluentEvent(proc.Logger()).Error("Persist flush after internal bulk create failed", err).
				Kind(kind).
				Log()
			return errfmt.Newf("internal bulk create reported success but data is not yet readable (write-behind flush)").Wrap(err)
		}
		proc.TriggerCacheFreshnessCheck("internal_bulk_create", []string{kind})
	}

	// Output results
	format := string(proc.Format())
	outputBulkResult(cmd, result, format, "create")

	return nil
}

// NewInternalBulkUpdateCmd creates a bulk update command for internal objects
func NewInternalBulkUpdateCmd() *cobra.Command {
	helpBuilder := clipkg.DynamicHelpBuilder(
		"Update multiple internal objects from a file (admin only)",
		"Update multiple internal or built-in objects from a YAML file.",
		"",
		"The file should contain a YAML array of update items:",
		"  - id: KSYN-001",
		"    updates:",
		"      priority: 10",
		"  - id: COMP-TYPE-001",
		"    updates:",
		"      title: \"Updated Title\"",
	).
		AddExample("Update multiple internal objects from file", "%s internal bulk update --file updates.yaml").
		AddExample("Dry-run to see what would be updated", "%s internal bulk update --file updates.yaml --dry-run").
		ExcludeCommonFlags()

	cmd := &cobra.Command{
		Use:  "update --file <file>",
		Args: cobra.NoArgs,
		RunE: runInternalBulkUpdate,
	}

	// Apply help builder to command
	helpBuilder.ApplyToCommand(cmd)

	cli.AddCommonFlags(cmd)
	cmd.Flags().String("file", "", "Path to YAML file containing array of updates (required)")
	cmd.Flags().Bool("dry-run", false, "Show what would be updated without actually updating")

	return cmd
}

func runInternalBulkUpdate(cmd *cobra.Command, args []string) error {
	// Create processor (handles context, storage, security, logging)
	proc, err := cli.NewProcessor(cmd)
	if err != nil {
		return errfmt.Newf("failed to create processor").Wrap(err)
	}

	filePath, err := cmd.Flags().GetString("file")
	if err != nil || filePath == emptyValue {
		return errfmt.Errorf("--file is required")
	}

	// Read file
	data, err := fileutil.ReadFile(filePath)
	if err != nil {
		logging.FluentEvent(proc.Logger()).Error("Failed to read file", err).
			File(filePath).
			Log()
		return errfmt.Newf("failed to read file").Wrap(err)
	}

	// Parse YAML array
	var updateItems []map[string]any
	if err := yaml.Unmarshal(data, &updateItems); err != nil {
		logging.FluentEvent(proc.Logger()).Error("Failed to parse YAML", err).Log()
		return errfmt.Newf("failed to parse YAML").Wrap(err)
	}

	// Convert to BulkUpdateItem format
	updates := make([]storage.BulkUpdateItem, 0, len(updateItems))
	for i, item := range updateItems {
		id, ok := item[objkeys.FieldKeyID].(string)
		if !ok || id == emptyValue {
			return errfmt.Errorf("update item at index %d missing 'id' field", i)
		}

		updatesMap, ok := item["updates"].(map[string]any)
		if !ok {
			return errfmt.Errorf("update item at index %d missing 'updates' field or invalid format", i)
		}

		updates = append(updates, storage.BulkUpdateItem{
			ID:      id,
			Updates: updatesMap,
		})
	}

	dryRun, err := cmd.Flags().GetBool("dry-run")
	if err != nil {
		dryRun = false
	}
	if dryRun {
		logging.FluentEvent(proc.Logger()).Info("Dry-run mode: showing what would be updated").
			Int("count", len(updates)).
			Log()
		var buf bytes.Buffer
		fmt.Fprintf(&buf, "Would update %d internal objects:\n", len(updates))
		for _, update := range updates {
			fmt.Fprintf(&buf, "\nObject %s:\n", update.ID)
			output, err := yaml.Marshal(update.Updates)
			if err != nil {
				fmt.Fprintf(&buf, "Error marshaling update: %v\n", err)
				continue
			}
			buf.Write(output)
		}
		return cli.WriteOutput(cmd, buf.Bytes())
	}

	result, err := proc.Storage().BulkUpdate(proc.OperationContext(), proc.SecurityContext(), updates)
	if err != nil {
		logging.FluentEvent(proc.Logger()).Error("Bulk update failed", err).Log()
		return errfmt.Newf("bulk update failed").Wrap(err)
	}

	// Write-behind: BulkUpdate result rows are id-only; infer kinds from IDs for CAS flush.
	if result != nil && result.SuccessCount > 0 {
		ids := make([]string, 0, len(result.Results))
		for _, m := range result.Results {
			if id, ok := m[objkeys.FieldKeyID].(string); ok && id != emptyValue {
				ids = append(ids, id)
			}
		}
		kinds := uniqueKindsFromObjectIDs(ids)
		flushCtx, cancelFlush := storage.DurabilityFlushContext()
		defer cancelFlush()
		if err := storage.EnsureCLIObjectMutationVisibleForProvider(flushCtx, proc.Storage(), proc.ProjectRoot(), kinds); err != nil {
			logging.FluentEvent(proc.Logger()).Error("Persist flush after internal bulk update failed", err).Log()
			return errfmt.Newf("internal bulk update reported success but data is not yet readable (write-behind flush)").Wrap(err)
		}
		proc.TriggerCacheFreshnessCheck("internal_bulk_update", kinds)
	}

	// Output results
	format := string(proc.Format())
	outputBulkResult(cmd, result, format, "update")

	return nil
}

// NewInternalBulkGetCmd creates a bulk get command for internal objects
func NewInternalBulkGetCmd() *cobra.Command {
	helpBuilder := clipkg.DynamicHelpBuilder(
		"Get multiple internal objects by ID (admin only)",
		"Get multiple internal or built-in objects by their IDs.",
		"",
		"You can provide IDs either:",
		"  - Via --ids flag (comma-separated): --ids KSYN-001,KSYN-002,KSYN-003",
		"  - Via --file flag (YAML array of IDs): --file ids.yaml",
	).
		AddExample("Get multiple internal objects by ID", "%s internal bulk get --ids KSYN-001,KSYN-002,KSYN-003").
		AddExample("Get from file", "%s internal bulk get --file ids.yaml").
		AddExample("Output as JSON", "%s internal bulk get --ids KSYN-001,KSYN-002 --format json").
		ExcludeCommonFlags()

	cmd := &cobra.Command{
		Use:  "get --ids <id1,id2,...> | --file <file>",
		Args: cobra.NoArgs,
		RunE: runInternalBulkGet,
	}

	// Apply help builder to command
	helpBuilder.ApplyToCommand(cmd)

	cli.AddCommonFlags(cmd)
	cmd.Flags().String("ids", "", "Comma-separated list of object IDs")
	cmd.Flags().String("file", "", "Path to YAML file containing array of IDs")

	return cmd
}

func runInternalBulkGet(cmd *cobra.Command, args []string) error {
	// Create processor (handles context, storage, security, logging)
	proc, err := cli.NewProcessor(cmd)
	if err != nil {
		return errfmt.Newf("failed to create processor").Wrap(err)
	}

	// Get IDs from --ids or --file using shared utility
	ids, err := clipkg.LoadIDsFromFlags(cmd, proc.Logger())
	if err != nil {
		return err
	}

	result, err := proc.Storage().BulkGet(proc.OperationContext(), proc.SecurityContext(), ids)
	if err != nil {
		logging.FluentEvent(proc.Logger()).Error("Bulk get failed", err).Log()
		return errfmt.Newf("bulk get failed").Wrap(err)
	}

	// Output results
	format := string(proc.Format())
	outputBulkResult(cmd, result, format, "get")

	return nil
}

// NewInternalBulkDeleteCmd creates a bulk delete command for internal objects
func NewInternalBulkDeleteCmd() *cobra.Command {
	helpBuilder := clipkg.DynamicHelpBuilder(
		"Delete multiple internal objects by ID (admin only)",
		"Delete multiple internal or built-in objects by their IDs.",
		"",
		"You can provide IDs either:",
		"  - Via --ids flag (comma-separated): --ids KSYN-001,KSYN-002,KSYN-003",
		"  - Via --file flag (YAML array of IDs): --file ids.yaml",
		"",
		"By default, deletion will fail if any object has dependents.",
		"Use --unlink-references to strip each ID from dependents' reference fields before deleting.",
		"Use --cascade to delete objects and all their dependents recursively (do not combine with --unlink-references).",
		"",
		"WARNING: Deleting built-in objects may break system functionality.",
	).
		AddExample("Delete multiple internal objects", "%s internal bulk delete --ids KSYN-001,KSYN-002,KSYN-003").
		AddExample("Unlink references then delete multiple", "%s internal bulk delete --ids KSYN-001,KSYN-002 --unlink-references").
		AddExample("Delete with cascade", "%s internal bulk delete --ids KSYN-001,KSYN-002 --cascade").
		AddExample("Delete from file", "%s internal bulk delete --file ids.yaml --cascade").
		AddExample("Dry-run to see what would be deleted", "%s internal bulk delete --ids KSYN-001,KSYN-002 --cascade --dry-run").
		ExcludeCommonFlags()

	cmd := &cobra.Command{
		Use:  "delete --ids <id1,id2,...> | --file <file> [--unlink-references] [--cascade]",
		Args: cobra.NoArgs,
		RunE: runInternalBulkDelete,
	}

	// Apply help builder to command
	helpBuilder.ApplyToCommand(cmd)

	cli.AddCommonFlags(cmd)
	cmd.Flags().String("ids", "", "Comma-separated list of object IDs")
	cmd.Flags().String("file", "", "Path to YAML file containing array of IDs")
	cmd.Flags().Bool("cascade", false, "Delete objects and all objects that reference them")
	cmd.Flags().Bool("unlink-references", false, "Strip this ID from dependents' reference fields, then delete (does not delete dependent objects)")
	cmd.Flags().Bool("dry-run", false, "Show what would be deleted without actually deleting")

	return cmd
}

func runInternalBulkDelete(cmd *cobra.Command, args []string) error {
	// Create processor (handles context, storage, security, logging)
	proc, err := cli.NewProcessor(cmd)
	if err != nil {
		return errfmt.Newf("failed to create processor").Wrap(err)
	}

	// Parse flags
	flags, err := parseBulkDeleteFlags(cmd, proc)
	if err != nil {
		return err
	}

	// Handle dry-run mode
	if flags.DryRun {
		return handleDryRunDelete(cmd, proc, flags.IDs, flags.Cascade, flags.UnlinkReferences)
	}

	// Execute bulk delete
	result, err := executeBulkDelete(proc, flags.IDs, flags.Cascade, flags.UnlinkReferences)
	if err != nil {
		return err
	}

	// Write-behind: match object bulk delete — flush so deletes are visible to the next zqk process.
	if result != nil && result.SuccessCount > 0 {
		ids := make([]string, 0, len(result.Results))
		for _, m := range result.Results {
			if id, ok := m[objkeys.FieldKeyID].(string); ok && id != emptyValue {
				ids = append(ids, id)
			}
		}
		kinds := uniqueKindsFromObjectIDs(ids)
		flushCtx, cancelFlush := storage.DurabilityFlushContext()
		defer cancelFlush()
		if err := storage.EnsureCLIObjectMutationVisibleForProvider(flushCtx, proc.Storage(), proc.ProjectRoot(), kinds); err != nil {
			logging.FluentEvent(proc.Logger()).Error("Persist flush after internal bulk delete failed", err).Log()
			return errfmt.Newf("internal bulk delete reported success but data is not yet readable (write-behind flush)").Wrap(err)
		}
		proc.TriggerCacheFreshnessCheck("internal_bulk_delete", kinds)
	}

	// Output results
	format := string(proc.Format())
	outputBulkResult(cmd, result, format, "delete")

	return nil
}

// outputBulkResult outputs bulk operation results using shared utility
func outputBulkResult(cmd *cobra.Command, result *storage.BulkResult, format, operation string) {
	// Convert storage.BulkResult errors to clipkg.BulkErrorInfo
	errors := make([]clipkg.BulkErrorInfo, len(result.Errors))
	for i, err := range result.Errors {
		errors[i] = clipkg.BulkErrorInfo{
			ID:      err.ID,
			Index:   err.Index,
			Message: err.Message,
		}
	}

	// Build structured data for format handlers
	outputData := clipkg.BuildBulkResultData(
		result.TotalCount,
		result.SuccessCount,
		result.FailureCount,
		result.Results,
		errors,
		operation,
	)

	// Use FormatOutput for consistent formatting (respects --format flag)
	if err := cli.FormatOutput(cmd, outputData); err != nil {
		// Fallback to legacy output if FormatOutput fails
		output, outputErr := clipkg.OutputBulkResult(
			result.TotalCount,
			result.SuccessCount,
			result.FailureCount,
			result.Results,
			errors,
			operation,
			format,
		)
		if outputErr != nil {
			// Last resort fallback
			fallback := fmt.Sprintf("Bulk %s operation completed with errors (failed to format output: %v)\n", operation, outputErr)
			//nolint:errcheck // Output errors are non-critical
			_ = cli.WriteOutput(cmd, []byte(fallback))
			return
		}
		//nolint:errcheck // Output errors are non-critical
		_ = cli.WriteOutput(cmd, output)
	}
}
