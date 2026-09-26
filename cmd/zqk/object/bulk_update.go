package object

import (
	"fmt"
	"maps"
	"strings"
	"time"

	"github.com/fatih/color"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	"github.com/zqk-os/zqk/internal/cli"
	clitool "github.com/zqk-os/zqk/pkg/cli" // Re-add this import
	"github.com/zqk-os/zqk/pkg/cli/bldr_cli_cmd_v1"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/goroutinelabels"
	"github.com/zqk-os/zqk/pkg/logging" // Keep for GetCanonicalKind
	"github.com/zqk-os/zqk/pkg/objectget"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/process"
	"github.com/zqk-os/zqk/pkg/storage"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

// NewBulkUpdateCmd creates a new bulk update command (spec: .zqk/cli/specs/object/bulk/update_command.yaml).
//
// bulkUpdateFileEntry matches YAML for `object bulk update <kind> --file`: a list of
// { id, updates: { field: value } } entries (per-object field merges).
type bulkUpdateFileEntry struct {
	ID      string         `yaml:"id"`
	Updates map[string]any `yaml:"updates"`
	Unset   []string       `yaml:"unset,omitempty"`
}

func NewBulkUpdateCmd() *cobra.Command {
	cmd := bldr_cli_cmd_v1.NewObjectBulkUpdateCommandBuilder()
	cli.EnsureCmdAnnotations(cmd)
	cmd.Annotations[cli.AnnotationKeyObjectKindValidate] = cli.KindValidateBulkUpdateFileKind
	cli.BindAsyncProgress(cmd, runBulkUpdate)

	return cmd
}

func runBulkUpdate(cmd *cobra.Command, args []string) error {
	cliContext := cli.GetContext(cmd)
	logger := logging.GetLoggerFromContext(cmd.Context())
	secCtx := pkgctx.NewSystemSecurityContext()

	//nolint:errcheck // Flag reads use zero values on error
	filePath, _ := cmd.Flags().GetString("file")
	filterStrings, _ := cmd.Flags().GetStringArray("filter")
	setStrings, _ := cmd.Flags().GetStringArray("set")
	dryRun, _ := cmd.Flags().GetBool("dry-run")
	kindFlag, _ := cmd.Flags().GetString("kind")

	kind := strings.TrimSpace(kindFlag)
	if len(args) > 0 && strings.TrimSpace(args[0]) != emptyValue {
		kind = strings.TrimSpace(args[0])
	}

	if filePath != emptyValue {
		if len(filterStrings) > 0 || len(setStrings) > 0 {
			return errfmt.Errorf("--file cannot be combined with --filter or --set")
		}
		if kind == emptyValue {
			return errfmt.Errorf("kind is required when using --file (e.g. zqk-admin object bulk update <kind> --file <file>)")
		}
	}

	relaxed, _ := cmd.Flags().GetBool("relaxed")
	if relaxed {
		setCacheCheckerForBatchCreation(nil)
	}

	if filePath != emptyValue {
		return runBulkUpdateFromFile(cmd, cliContext, logger, secCtx, kind, filePath, dryRun)
	}

	// Filter/set path
	// Treat explicit empty kind argument as "not specified" (do not infer from filter)
	if len(args) > 0 && strings.TrimSpace(kind) == emptyValue {
		logging.FluentEvent(logger).Error("BulkUpdateCmd: Kind specified as empty string", errfmt.Errorf("kind must be specified as an argument or within a filter")).Log()
		return errfmt.Errorf("kind must be specified as an argument or within a filter")
	}
	logging.FluentEvent(logger).Debug("BulkUpdateCmd: Initial kind and args check").
		String("kind_arg", kind).
		String("args", fmt.Sprintf("%v", args)).
		Log()

	if kind == emptyValue && len(filterStrings) == 0 {
		logging.FluentEvent(logger).Error("BulkUpdateCmd: Kind not specified and no filters provided", errfmt.Errorf("kind must be specified as an argument or within a filter")).Log()
		return errfmt.Errorf("kind must be specified as an argument or within a filter")
	}

	// Resolve kind if only filter is provided
	if kind == emptyValue && len(filterStrings) > 0 {
		logging.FluentEvent(logger).Debug("BulkUpdateCmd: Attempting to infer kind from filter").
			String("filterStrings", fmt.Sprintf("%v", filterStrings)).
			Log()
		// Attempt to infer kind from filter, e.g., "id=BLI-001"
		for _, f := range filterStrings {
			if strings.HasPrefix(f, "id=") {
				id := strings.TrimPrefix(f, "id=")
				logging.FluentEvent(logger).Debug("BulkUpdateCmd: Inferring kind from ID").
					ObjectID(id).
					Log()
				inferredKind := objects.GetCanonicalKind(id)
				logging.FluentEvent(logger).Debug("BulkUpdateCmd: GetCanonicalKind returned").
					ObjectID(id).
					String("inferredKind", inferredKind).
					Log()
				// Only use inferred kind if it resolved to a canonical kind (not the raw ID)
				if inferredKind != emptyValue && inferredKind != id {
					kind = inferredKind
					break
				}
			}
		}
		if kind == emptyValue {
			logging.FluentEvent(logger).Error("BulkUpdateCmd: Could not infer kind from filter", errfmt.Errorf("could not infer kind from filter; please specify kind as an argument")).Log()
			return errfmt.Errorf("could not infer kind from filter; please specify kind as an argument")
		}
	}
	logging.FluentEvent(logger).Debug("BulkUpdateCmd: Resolved kind").
		Kind(kind).
		Log()

	// Parse filter strings into a map
	rawFilterMap := make(map[string]any)
	for _, f := range filterStrings {
		parts := strings.SplitN(f, "=", 2)
		if len(parts) != 2 {
			return errfmt.Errorf("invalid filter format: %s. Expected key=value", f)
		}
		rawFilterMap[parts[0]] = parts[1]
	}

	// Ensure kind is in the filter if provided
	if kind != emptyValue && rawFilterMap[objects.FieldKeyKind] == nil {
		rawFilterMap[objects.FieldKeyKind] = kind
	} else if kind == emptyValue && rawFilterMap[objects.FieldKeyKind] != nil {
		kind = rawFilterMap[objects.FieldKeyKind].(string)
	} else if kind != emptyValue && rawFilterMap[objects.FieldKeyKind] != nil && rawFilterMap[objects.FieldKeyKind].(string) != kind {
		logging.FluentEvent(logger).Error("BulkUpdateCmd: Kind conflict", errfmt.Errorf("kind specified in argument (%s) conflicts with kind in filter (%s)", kind, rawFilterMap[objects.FieldKeyKind].(string))).Log()
		return errfmt.Errorf("kind specified in argument (%s) conflicts with kind in filter (%s)", kind, rawFilterMap[objects.FieldKeyKind].(string))
	}

	if kind == emptyValue {
		logging.FluentEvent(logger).Error("BulkUpdateCmd: Final kind check failed", errfmt.Errorf("kind must be specified")).Log()
		return errfmt.Errorf("kind must be specified")
	}

	projectRoot := cli.ResolveProjectRoot(".")
	logging.FluentEvent(logger).Debug("BulkUpdateCmd: Resolved projectRoot for storage").
		ProjectRoot(projectRoot).
		Log()
	if projectRoot == emptyValue {
		logging.FluentEvent(logger).Error("BulkUpdateCmd: Project root not available", errfmt.Errorf("project root not available")).Log()
		return errfmt.Errorf("project root not available")
	}

	validatedKind, err := objects.ResolveAndValidateKindForProject(projectRoot, kind)
	if err != nil {
		return err
	}
	kind = validatedKind

	// Parse set strings into a map
	setMap := make(map[string]any)
	if len(setStrings) == 0 {
		logging.FluentEvent(logger).Error("BulkUpdateCmd: No set flags provided", errfmt.Errorf("at least one --set flag must be provided")).Log()
		return errfmt.Errorf("at least one --set flag must be provided")
	}
	for _, s := range setStrings {
		parts := strings.SplitN(s, "=", 2)
		if len(parts) != 2 {
			return errfmt.Errorf("invalid set format: %s. Expected key=value", s)
		}
		setMap[parts[0]] = parts[1]
	}
	if err := guardManualStatusUpdate(cmd, nil, "", kind, setMap); err != nil {
		return err
	}
	if err := guardManualRefFieldUpdates(cmd, nil, "", kind, setMap); err != nil {
		return err
	}
	if err := guardManualSystemProvenanceFields(cmd, nil, "", kind, setMap); err != nil {
		return err
	}

	storageProvider, err := cli.GetObjectStorageForCommand(cmd, projectRoot)
	if err != nil {
		logging.FluentEvent(logger).Error("BulkUpdateCmd: Failed to get storage provider", err).
			ProjectRoot(projectRoot).
			Log()
		return errfmt.Newf("failed to get storage provider").Wrap(err)
	}
	logging.FluentEvent(logger).Debug("BulkUpdateCmd: Successfully obtained storage provider").
		String("storageProviderType", fmt.Sprintf("%T", storageProvider)).
		Log()

	// List objects based on filter
	storageCtx := &pkgctx.StorageContext{}
	listFilter := storage.ListFilter{
		Kind:    kind,
		Filters: rawFilterMap,
	}
	logging.FluentEvent(logger).Debug("BulkUpdateCmd: Calling storageProvider.List").
		Kind(kind).
		String("filters", fmt.Sprintf("%v", rawFilterMap)).
		Log()
	queryResult, err := storageProvider.List(cmd.Context(), secCtx, storageCtx, listFilter)
	if err != nil {
		logging.FluentEvent(logger).Error("BulkUpdateCmd: Failed to list objects", err).
			Kind(kind).
			String("filters", fmt.Sprintf("%v", rawFilterMap)).
			Log()
		return errfmt.Newf("failed to list objects").Wrap(err)
	}
	logging.FluentEvent(logger).Debug("BulkUpdateCmd: storageProvider.List returned").
		Int("object_count", len(queryResult.Objects)).
		Log()

	if len(queryResult.Objects) == 0 {
		msg := color.YellowString("No objects found matching the filter to update.")
		logging.FluentEvent(logger).Warn(msg).Log()
		if err := cli.WriteOutput(cmd, append([]byte(msg), '\n')); err != nil {
			return err
		}
		return nil
	}

	updatedCount := 0
	var updatedObjectMaps []map[string]any
	var bulkErrors []clitool.BulkErrorInfo

	stopHeartbeat := make(chan struct{})
	defer close(stopHeartbeat)
	goroutinelabels.NewGoroutine("bulk_update_filter_heartbeat", "keep idle watchdog awake during bulk update").
		StartSimple(func() {
			ticker := time.NewTicker(2 * time.Second)
			defer ticker.Stop()
			for {
				select {
				case <-stopHeartbeat:
					return
				case <-ticker.C:
					process.TouchMeaningfulActivity()
				}
			}
		})

	for _, originalObjMap := range queryResult.Objects {
		process.TouchMeaningfulActivity()
		objectData := make(map[string]any)
		maps.Copy(objectData, originalObjMap)

		originalID, ok := objectData[objects.FieldKeyID].(string)
		if !ok {
			errMsg := fmt.Sprintf("object missing ID field or ID is not a string: %v", objectData)
			bulkErrors = append(bulkErrors, clitool.BulkErrorInfo{Message: errMsg})
			continue
		}
		originalKind, ok := objectData[objects.FieldKeyKind].(string)
		if !ok {
			errMsg := fmt.Sprintf("object %s missing kind field or kind is not a string: %v", originalID, objectData)
			bulkErrors = append(bulkErrors, clitool.BulkErrorInfo{ID: originalID, Message: errMsg})
			continue
		}

		pruneUnknownFields(objectData, kind)

		// Apply updates
		for field, value := range setMap {
			objectData[field] = value
		}

		_ = objectget.StripReferenceResolverOverlayFields(objectData)

		if dryRun {
			for k, v := range objectData {
				if storage.IsFieldUnset(v) {
					delete(objectData, k)
				}
			}
			logging.FluentEvent(logger).Info("Dry run: Would update object.").
				ObjectID(originalID).
				String("kind", originalKind).
				String("changes", fmt.Sprintf("%v", setMap)).
				Log()
			updatedObjectMaps = append(updatedObjectMaps, objectData)
			updatedCount++
		} else {
			err = storageProvider.Update(cmd.Context(), secCtx, originalID, objectData)
			if err != nil {
				errMsg := fmt.Sprintf("failed to update %s:%s: %v", originalKind, originalID, err)
				bulkErrors = append(bulkErrors, clitool.BulkErrorInfo{ID: originalID, Message: errMsg})
				continue
			}
			for k, v := range objectData {
				if storage.IsFieldUnset(v) {
					delete(objectData, k)
				}
			}
			updatedCount++
			updatedObjectMaps = append(updatedObjectMaps, objectData)
			logging.FluentEvent(logger).Info("Updated object.").
				ObjectID(originalID).
				String("kind", originalKind).
				String("changes", fmt.Sprintf("%v", setMap)).
				Log()
		}
	}

	// Write-behind: match object create/update/bulk create — flush so the next read or zqk process sees updates.
	if !dryRun && updatedCount > 0 && len(bulkErrors) == 0 {
		flushCtx, cancelFlush := storage.DurabilityFlushContext()
		defer cancelFlush()
		t0 := time.Now()
		if err := storage.EnsureCLIObjectMutationVisibleForProvider(flushCtx, storageProvider, projectRoot, []string{kind}); err != nil {
			logging.FluentEvent(logger).Warn("Persist flush after bulk update timed out, but objects are updated").
				WithError(err).
				Kind(kind).
				Log()
			// Emit warning but don't fail, the objects are safely on disk.
			fmt.Fprintln(cmd.ErrOrStderr(), color.YellowString("Warning: Bulk update completed, but index refresh is delayed."))
		}
		logSlowCLIObjectMutationFlush(logger, "bulk_update", "(bulk)", []string{kind}, time.Since(t0), 0)
	}

	if !dryRun {
		logging.FluentEvent(logger).Info(color.GreenString("Successfully updated %d of %d objects.", updatedCount, len(queryResult.Objects))).Log()
	} else {
		logging.FluentEvent(logger).Info(color.CyanString("Dry run complete. Would have updated %d of %d objects.", updatedCount, len(queryResult.Objects))).Log()
	}

	if len(bulkErrors) > 0 {
		logging.FluentEvent(logger).Error("Bulk update operation completed with errors.", errfmt.Errorf("bulk update operation failed with %d errors", len(bulkErrors))).
			String("total_errors", fmt.Sprintf("%d", len(bulkErrors))).
			String("colored_summary", color.RedString(fmt.Sprintf("Bulk update encountered %d errors.", len(bulkErrors)))).
			Log()
		for _, bErr := range bulkErrors {
			fmt.Fprintf(cmd.ErrOrStderr(), "Error for ID %s: %s\n", bErr.ID, bErr.Message)
			logging.FluentEvent(logger).Error("Individual error during bulk update.", errfmt.Errorf("error for ID %s: %s", bErr.ID, bErr.Message)).
				String("error_id", bErr.ID).
				String("error_message", bErr.Message).
				String("colored_error", color.RedString("- %s (ID: %s)", bErr.Message, bErr.ID)).
				Log()
		}
		return errfmt.Errorf("bulk update failed with %d errors", len(bulkErrors))
	}

	if len(updatedObjectMaps) > 0 {
		projectFields, ferr := clitool.FieldsFromCmd(cmd)
		if ferr != nil {
			return cli.Guard(cmd).Err(ferr).Return()
		}
		applyHybridProjectionToObjectMaps(updatedObjectMaps, projectFields)

		outputBytes, err := clitool.OutputBulkResult(
			len(queryResult.Objects),
			updatedCount,
			len(bulkErrors),
			updatedObjectMaps,
			bulkErrors,
			"update",
			string(cliContext.Format),
		)
		if err != nil {
			return errfmt.Newf("failed to format bulk output").Wrap(err)
		}
		if err := cli.WriteOutput(cmd, outputBytes); err != nil {
			return err
		}
	}

	return nil
}

func runBulkUpdateFromFile(cmd *cobra.Command, cliContext *cli.Context, logger *logging.EventLogger, secCtx *pkgctx.SecurityContext, kind, filePath string, dryRun bool) error {
	projectRoot := cli.ResolveProjectRoot(".")
	if projectRoot == emptyValue {
		return errfmt.Errorf("project root not available")
	}
	if k, ok := kindCanonicalFromPRERun(cmd); ok {
		kind = k
	} else {
		validatedKind, err := objects.ResolveAndValidateKindForProject(projectRoot, kind)
		if err != nil {
			return err
		}
		kind = validatedKind
	}

	data, err := fileutil.ReadFile(filePath)
	if err != nil {
		return errfmt.Newf("failed to read file").Wrap(err)
	}
	var entries []bulkUpdateFileEntry
	if err := yaml.Unmarshal(data, &entries); err != nil {
		return errfmt.Newf("failed to parse YAML").Wrap(err)
	}
	if len(entries) == 0 {
		msg := color.YellowString("No entries in file; nothing to update.")
		logging.FluentEvent(logger).Warn(msg).Log()
		if err := cli.WriteOutput(cmd, append([]byte(msg), '\n')); err != nil {
			return err
		}
		return nil
	}

	storageProvider, err := cli.GetObjectStorageForCommand(cmd, projectRoot)
	if err != nil {
		return errfmt.Newf("failed to get storage provider").Wrap(err)
	}

	var updatedObjectMaps []map[string]any
	var bulkErrors []clitool.BulkErrorInfo
	updatedCount := 0
	stopHeartbeat := make(chan struct{})
	defer close(stopHeartbeat)
	goroutinelabels.NewGoroutine("bulk_update_file_heartbeat", "keep idle watchdog awake during bulk update").
		StartSimple(func() {
			ticker := time.NewTicker(2 * time.Second)
			defer ticker.Stop()
			for {
				select {
				case <-stopHeartbeat:
					return
				case <-ticker.C:
					process.TouchMeaningfulActivity()
				}
			}
		})

	for _, entry := range entries {
		process.TouchMeaningfulActivity()
		id := strings.TrimSpace(entry.ID)
		if id == emptyValue {
			bulkErrors = append(bulkErrors, clitool.BulkErrorInfo{Message: "entry missing id"})
			continue
		}
		if len(entry.Updates) == 0 && len(entry.Unset) == 0 {
			bulkErrors = append(bulkErrors, clitool.BulkErrorInfo{ID: id, Message: "entry missing updates"})
			continue
		}
		if err := guardManualStatusUpdate(cmd, nil, id, kind, entry.Updates); err != nil {
			return err
		}
		if err := guardManualRefFieldUpdates(cmd, nil, id, kind, entry.Updates); err != nil {
			return err
		}
		if err := guardManualSystemProvenanceFields(cmd, nil, id, kind, entry.Updates); err != nil {
			return err
		}

		objectData, err := storageProvider.Read(cmd.Context(), secCtx, id)
		if err != nil {
			bulkErrors = append(bulkErrors, clitool.BulkErrorInfo{ID: id, Message: fmt.Sprintf("read failed: %v", err)})
			continue
		}
		objKind, _ := objectData[objects.FieldKeyKind].(string)
		if objKind != kind {
			bulkErrors = append(bulkErrors, clitool.BulkErrorInfo{ID: id, Message: fmt.Sprintf("object kind %q does not match %q", objKind, kind)})
			continue
		}

		for _, u := range entry.Unset {
			objectData[u] = storage.FieldUnset
		}
		for k, v := range entry.Updates {
			if v == nil || v == "<UNSET>" || storage.IsFieldUnset(v) {
				objectData[k] = storage.FieldUnset
			} else {
				objectData[k] = v
			}
		}

		pruneUnknownFields(objectData, kind)

		normalized, normErr := NormalizeObjectValues(objectData, kind)
		if normErr != nil {
			logging.FluentEvent(logger).Warn("NormalizeObjectValues warning").
				ObjectID(id).
				WithError(normErr).
				Log()
		} else {
			objectData = normalized
		}

		if dryRun {
			for k, v := range objectData {
				if storage.IsFieldUnset(v) {
					delete(objectData, k)
				}
			}
			logging.FluentEvent(logger).Info("Dry run: Would update object.").
				ObjectID(id).
				String("kind", kind).
				String("changes", fmt.Sprintf("%v", entry.Updates)).
				Log()
			updatedObjectMaps = append(updatedObjectMaps, objectData)
			updatedCount++
			continue
		}

		if err := storageProvider.Update(cmd.Context(), secCtx, id, objectData); err != nil {
			bulkErrors = append(bulkErrors, clitool.BulkErrorInfo{ID: id, Message: fmt.Sprintf("failed to update: %v", err)})
			continue
		}
		for k, v := range objectData {
			if storage.IsFieldUnset(v) {
				delete(objectData, k)
			}
		}
		updatedCount++
		updatedObjectMaps = append(updatedObjectMaps, objectData)
		logging.FluentEvent(logger).Info("Updated object.").
			ObjectID(id).
			String("kind", kind).
			Log()
	}

	// Write-behind: ensure CAS/durability before reporting success (same as filter/set path).
	if !dryRun && updatedCount > 0 && len(bulkErrors) == 0 {
		flushCtx, cancelFlush := storage.DurabilityFlushContext()
		defer cancelFlush()
		t0 := time.Now()
		if err := storage.EnsureCLIObjectMutationVisibleForProvider(flushCtx, storageProvider, projectRoot, []string{kind}); err != nil {
			logging.FluentEvent(logger).Warn("Persist flush after bulk update (file) timed out, but objects are updated").
				WithError(err).
				Kind(kind).
				Log()
			// Emit warning but don't fail, the objects are safely on disk.
			fmt.Fprintln(cmd.ErrOrStderr(), color.YellowString("Warning: Bulk update (file) completed, but index refresh is delayed."))
		}
		logSlowCLIObjectMutationFlush(logger, "bulk_update_file", "(bulk)", []string{kind}, time.Since(t0), 0)
	}

	if !dryRun {
		logging.FluentEvent(logger).Info(color.GreenString("Successfully updated %d of %d objects.", updatedCount, len(entries))).Log()
	} else {
		logging.FluentEvent(logger).Info(color.CyanString("Dry run complete. Would have updated %d of %d objects.", updatedCount, len(entries))).Log()
	}

	if len(bulkErrors) > 0 {
		logging.FluentEvent(logger).Error("Bulk update operation completed with errors.", errfmt.Errorf("bulk update operation failed with %d errors", len(bulkErrors))).Log()
		for _, bErr := range bulkErrors {
			fmt.Fprintf(cmd.ErrOrStderr(), "Error for ID %s: %s\n", bErr.ID, bErr.Message)
			logging.FluentEvent(logger).Error("Individual error during bulk update.", errfmt.Errorf("%s", bErr.Message)).
				String("error_id", bErr.ID).
				Log()
		}
		return errfmt.Errorf("bulk update failed with %d errors", len(bulkErrors))
	}

	if len(updatedObjectMaps) > 0 {
		projectFields, ferr := clitool.FieldsFromCmd(cmd)
		if ferr != nil {
			return cli.Guard(cmd).Err(ferr).Return()
		}
		applyHybridProjectionToObjectMaps(updatedObjectMaps, projectFields)

		outputBytes, err := clitool.OutputBulkResult(
			len(entries),
			updatedCount,
			len(bulkErrors),
			updatedObjectMaps,
			bulkErrors,
			"update",
			string(cliContext.Format),
		)
		if err != nil {
			return errfmt.Newf("failed to format bulk output").Wrap(err)
		}
		if err := cli.WriteOutput(cmd, outputBytes); err != nil {
			return err
		}
	}

	return nil
}

// pruneUnknownFields strips top-level fields not recognized by the kind specification to ensure
// strict-mode validation passes on objects created with legacy unvalidated attributes.
func pruneUnknownFields(obj map[string]any, kind string) {
	specLoader := objects.GetGlobalSpecLoader()
	if specLoader == nil {
		return
	}
	schemaVersion, _ := obj[objects.FieldKeySchemaVersion].(string)
	var spec *objects.Spec
	var err error
	if schemaVersion != "" {
		spec, err = specLoader.LoadSpecByVersion(kind, schemaVersion)
	}
	if spec == nil || err != nil {
		spec, err = specLoader.LoadSpecWithInheritance(kind + ".yaml")
	}
	if err != nil || spec == nil || spec.ResolvedFields == nil {
		return
	}
	for k, v := range obj {
		if k == objects.FieldKeyID || k == objects.FieldKeyKind {
			continue
		}
		if storage.IsFieldUnset(v) {
			continue
		}
		if _, ok := spec.ResolvedFields[k]; !ok {
			if !objects.IsCompositionFieldAllowed(kind, k, spec) {
				obj[k] = storage.FieldUnset
			}
		}
	}
}
