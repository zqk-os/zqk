package system

import (
	"context"
	"fmt"

	"github.com/zqk-os/zqk/internal/cli"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
	"github.com/zqk-os/zqk/pkg/cli/bldr_cli_cmd_v1"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/storage"
	"github.com/spf13/cobra"
)

// NewValidateCmd creates a new validate command
func NewValidateCmd() *cobra.Command {
	var (
		all   bool
		kind  string
		fix   bool
		quiet bool
	)

	helpBuilder := clipkg.DynamicHelpBuilder(
		"Validate object(s) against schema",
		"Validate object(s) against schema and check references.",
		"",
		"This command validates objects against their schema definitions and checks",
		"that all references are valid. It can validate:",
		"  - A specific object by ID",
		"  - All objects of a specific kind (--kind)",
		"  - All in-scope project objects (default when neither an ID nor --kind is given; same as --all)",
	).
		AddExample("Validate a specific object", "%s system validate BLI-657").
		AddExample("Validate all backlog items", "%s system validate --kind backlog_item").
		AddExample("Validate all in-scope objects (default)", "%s system validate").
		AddExample("Validate all (explicit)", "%s system validate --all").
		AddExample("Attempt to fix validation errors", "%s system validate --fix").
		ExcludeCommonFlags()

	validateCmd := clipkg.ApplyBuilder(bldr_cli_cmd_v1.NewSystemValidateCommandBuilder(), &cobra.Command{
		Use:  "validate [object-id]",
		Args: cobra.MaximumNArgs(1),
	})
	cli.BindAsyncProgress(validateCmd, func(cmd *cobra.Command, args []string) error {
		return runValidate(cmd, args, all, kind, fix, quiet)
	})

	// Apply help builder to command
	helpBuilder.ApplyToCommand(validateCmd)

	validateCmd.Flags().BoolVar(&all, "all", false, "Validate all objects")
	validateCmd.Flags().StringVar(&kind, "kind", "", "Validate objects of specific kind")
	validateCmd.Flags().BoolVar(&fix, "fix", false, "Attempt to fix validation errors")
	validateCmd.Flags().BoolVarP(&quiet, "quiet", "q", false, "Suppress non-essential output")
	cli.AddCommonFlags(validateCmd)

	return validateCmd
}

func runValidate(cmd *cobra.Command, args []string, all bool, kind string, fix, quiet bool) error {
	ctx := cli.GetContext(cmd)
	if ctx == nil {
		// No logger available yet, but root.go will log this
		return errfmt.Errorf("failed to get context")
	}
	// Use context profile for logging (respects MCP context)
	profile := profileOrDefault(ctx.Profile, systemProfileHuman) // Default fallback
	logger := logging.GetLoggerFromProfile(profile)

	projectRoot := ProjectRootOrResolveDot(ctx.ProjectRoot)
	if projectRoot == emptyValue {
		err := errfmt.Errorf("not a ZQK project (no project root found)")
		logging.Fluent(logger).Error("Project root validation failed", err).Log()
		return err
	}

	factory, err := storage.NewStorageFactory(cmd.Context(), projectRoot)
	if err != nil {
		logErr := errfmt.Newf("failed to initialize storage factory").Wrap(err)
		logging.Fluent(logger).Error("Storage initialization failed", logErr).Log()
		return logErr
	}
	storageProvider := factory.GetStorage()
	if storageProvider != nil {
		defer func() { _ = storageProvider.Shutdown(context.Background()) }() // Background: request-or-shutdown derived
	}

	format := cli.GetFormat(cmd)

	// With no object id and no --kind, default to the same scope as --all: the command name already
	// orients intent ("validate" under system = project validation), so operators need not type --all.
	if len(args) == 0 && kind == emptyValue && !all {
		all = true
	}

	// Determine what to validate
	if len(args) > 0 {
		objectID := args[0]
		valid, err := validateObject(cmd, storageProvider, objectID, fix, quiet, logger)
		if err != nil {
			return err
		}
		if format == cli.FormatJSON || format == cli.FormatYAML {
			out := map[string]any{"valid": valid, "object_id": objectID}
			return cli.FormatOutput(cmd, out)
		}
		return nil
	}
	if all {
		totalValid, totalInvalid, err := validateAll(cmd, storageProvider, fix, quiet, logger)
		if err != nil {
			return err
		}
		if format == cli.FormatJSON || format == cli.FormatYAML {
			out := map[string]any{"valid": totalInvalid == 0, "valid_count": totalValid, "invalid_count": totalInvalid}
			return cli.FormatOutput(cmd, out)
		}
		return nil
	}
	if kind != emptyValue {
		validCount, invalidCount, err := validateKind(cmd, storageProvider, kind, fix, quiet, logger)
		if err != nil {
			return err
		}
		if format == cli.FormatJSON || format == cli.FormatYAML {
			out := map[string]any{"valid": invalidCount == 0, objects.FieldKeyKind: kind, "valid_count": validCount, "invalid_count": invalidCount}
			return cli.FormatOutput(cmd, out)
		}
		return nil
	}
	err = errfmt.Errorf("must specify object ID, --all, or --kind")
	logging.Fluent(logger).Error("Validation arguments invalid", err).Log()
	return err
}

func validateObject(cmd *cobra.Command, storageProvider storage.ObjectStorageProvider, objectID string, _, quiet bool, logger logging.Logger) (valid bool, err error) {
	secCtx := pkgctx.NewSystemSecurityContext()
	obj, err := storageProvider.Read(cmd.Context(), secCtx, objectID)
	if err != nil {
		return false, errfmt.Errorf("failed to get object %s: %w", objectID, err)
	}

	if !quiet {
		logging.Fluent(logger).Info("Validating object").
			ObjectID(objectID).
			Log()
	}

	if obj == nil {
		return false, errfmt.Errorf("object %s not found", objectID)
	}
	if id, ok := obj[objects.FieldKeyID].(string); !ok || id == emptyValue {
		return false, errfmt.Errorf("object %s missing required field: id", objectID)
	}
	if objKind, ok := obj[objects.FieldKeyKind].(string); !ok || objKind == emptyValue {
		return false, errfmt.Errorf("object %s missing required field: kind", objectID)
	}

	if !quiet {
		logging.Fluent(logger).Info("Object is valid").
			ObjectID(objectID).
			Log()
	}
	return true, nil
}

func validateKind(cmd *cobra.Command, storageProvider storage.ObjectStorageProvider, kind string, _, quiet bool, logger logging.Logger) (validCount, invalidCount int, err error) {
	if !quiet {
		logging.Fluent(logger).Info("Validating all objects of kind").
			Kind(kind).
			Log()
	}

	secCtx := pkgctx.NewSystemSecurityContext()
	storageCtx := pkgctx.NewStorageContext()
	result, listErr := storageProvider.List(cmd.Context(), secCtx, storageCtx, storage.ListFilter{
		Kind: kind,
	})
	if listErr != nil {
		logging.Fluent(logger).Error("Object listing failed", listErr).
			Kind(kind).
			Log()
		return 0, 0, listErr
	}

	for _, obj := range result.Objects {
		objID, _ := obj[objects.FieldKeyID].(string)
		if objID == emptyValue {
			invalidCount++
			if !quiet {
				cmd.PrintErrf("  ⚠️  [%s] Object missing id field\n", kind)
				logging.Fluent(logger).Warn("Object missing id field").Log()
			}
			continue
		}
		if objKind, ok := obj[objects.FieldKeyKind].(string); !ok || objKind != kind {
			invalidCount++
			if !quiet {
				cmd.PrintErrf("  ⚠️  %s: kind mismatch (expected %s, got %s)\n", objID, kind, objKind)
				logging.Fluent(logger).Warn(fmt.Sprintf("%s: kind mismatch (expected %s, got %s)", objID, kind, objKind)).Log()
			}
			continue
		}
		validCount++
	}

	if !quiet {
		logging.Fluent(logger).Info("Validation summary").
			Kind(kind).
			Int("valid", validCount).
			Int("invalid", invalidCount).
			Log()
	}

	if invalidCount > 0 {
		return validCount, invalidCount, errfmt.Errorf("validation failed: %d invalid objects found", invalidCount)
	}
	return validCount, invalidCount, nil
}

func validateAll(cmd *cobra.Command, storageProvider storage.ObjectStorageProvider, fix, quiet bool, logger logging.Logger) (totalValid, totalInvalid int, err error) {
	if !quiet {
		logging.Fluent(logger).Info("Validating all objects").Log()
	}

	kinds := []string{
		objects.KindBacklogItem, objects.KindPolicy, objects.KindRequirement, objects.KindCriteria, objects.KindTestCase,
		objects.KindGoal, objects.KindMilestone, objects.KindWorkstream, objects.KindPriorityPlan,
		objects.KindDecision, objects.KindQuestion, objects.KindDocEntry,
	}

	for _, k := range kinds {
		validN, invalidN, listErr := validateKind(cmd, storageProvider, k, fix, quiet, logger)
		totalValid += validN
		totalInvalid += invalidN
		if listErr != nil {
			// Count kinds with issues (invalidN already added)
			_ = listErr
		}
	}

	if !quiet {
		logging.Fluent(logger).Info("Overall validation summary").
			Int("valid_objects", totalValid).
			Int("issues_found", totalInvalid).
			Log()
	}

	if totalInvalid > 0 {
		return totalValid, totalInvalid, errfmt.Errorf("validation completed with %d issues", totalInvalid)
	}
	if !quiet {
		logging.Fluent(logger).Info("All objects are valid").Log()
	}
	return totalValid, totalInvalid, nil
}
