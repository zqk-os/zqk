package object

import (
	"bytes"
	stdcontext "context"
	"fmt"
	"maps"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/spf13/cobra"

	"github.com/zqk-os/zqk/internal/cli"
	"github.com/zqk-os/zqk/pkg/brand"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/goroutinelabels"
	"github.com/zqk-os/zqk/pkg/kernelcas"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/objectget"
	"github.com/zqk-os/zqk/pkg/objectidcache"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/storage"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
	"github.com/zqk-os/zqk/pkg/validation"
	"github.com/zqk-os/zqk/pkg/when"
)

// buildUpdatesFromFieldFlags builds updates map from --field flags using internal/cli FieldParser.
// Supports append operations (field+=value) when current object is available.
// BLI-854: Supports dotted field names (e.g. meta.tags) by expanding into nested maps.
// Optimized: only checks for append operations if currentObj is provided (avoids redundant checks).
func buildUpdatesFromFieldFlags(cmd *cobra.Command, proc *cli.Processor, currentObj map[string]any) (map[string]any, error) {
	fields, _ := cmd.Flags().GetStringArray("field")
	if len(fields) == 0 {
		return nil, nil
	}
	fp := cli.NewFieldParser(proc.Logger())
	var raw map[string]any
	var err error
	if currentObj != nil {
		hasAppend := false
		for _, fieldStr := range fields {
			if strings.Contains(fieldStr, "+=") {
				hasAppend = true
				break
			}
		}
		if hasAppend {
			raw, err = fp.ParseFieldFlagsWithAppend(fields, currentObj)
		} else {
			raw, err = fp.ParseFieldFlags(fields)
		}
	} else {
		raw, err = fp.ParseFieldFlags(fields)
	}
	if err != nil {
		return nil, err
	}
	return expandDottedFieldKeys(raw), nil
}

// expandDottedFieldKeys converts dotted keys (e.g. "meta.tags") into nested maps (BLI-854).
// Values from ParseFieldValue may be strings, numbers, slices, or maps; they are set as-is at the path.
func expandDottedFieldKeys(updates map[string]any) map[string]any {
	out := make(map[string]any)
	maps.Copy(out, updates)
	for key, value := range updates {
		if !strings.Contains(key, ".") {
			continue
		}
		delete(out, key)
		parts := strings.Split(key, ".")
		setNestedMap(out, parts, value)
	}
	return out
}

func setNestedMap(m map[string]any, path []string, value any) {
	if len(path) == 0 {
		return
	}
	if len(path) == 1 {
		m[path[0]] = value
		return
	}
	head, rest := path[0], path[1:]
	existing, ok := m[head]
	if !ok {
		existing = make(map[string]any)
		m[head] = existing
	}
	sub, ok := existing.(map[string]any)
	if !ok {
		sub = make(map[string]any)
		m[head] = sub
	}
	setNestedMap(sub, rest, value)
}

// validateFileHash validates the hash of a file if it appears to be a hash-based filename
// (content-addressable storage). Returns an error if the file has a hash mismatch.
// This enables early failure with clear error messages (BLI-914).
func validateFileHash(filePath string) error {
	filename := filepath.Base(filePath)

	// Check if filename looks like a hash-based filename (64 hex chars + .yaml = 69 chars)
	if len(filename) < 69 || !strings.HasSuffix(filename, ".yaml") {
		// Not a hash-based filename, skip validation
		return nil
	}

	// Extract hash from filename (remove .yaml extension)
	filenameHash := filename[:len(filename)-5]

	// Validate it's a valid hex string (64 hex characters)
	hexPattern := regexp.MustCompile(`^[0-9a-f]{64}$`)
	if !hexPattern.MatchString(filenameHash) {
		// Not a valid hash format, skip validation
		return nil
	}

	// Read file content
	fileData, err := fileutil.ReadFile(filePath)
	if err != nil {
		return errfmt.Newf("failed to read file").Wrap(err)
	}

	// Calculate actual content hash
	actualHash := storage.CalculateSHA256Hash(fileData)

	// Compare hash in filename with actual content hash
	if filenameHash != actualHash {
		return errfmt.Errorf("file provided via --file has hash mismatch: filename has %s, content has %s (file may be corrupted). Use 'zqk utility fix-hashes --force <file>' to fix", filenameHash, actualHash)
	}

	return nil
}

// loadUpdatesFromFile loads updates from a file using internal/cli DataLoader.
// If the file appears to be a hash-based filename (content-addressable storage),
// validates the hash before processing (BLI-914).
func loadUpdatesFromFile(cmd *cobra.Command, proc *cli.Processor) (map[string]any, error) {
	filePath, _ := cmd.Flags().GetString("file")
	if filePath == emptyValue {
		return nil, nil
	}

	// Validate file hash if it's a hash-based filename (BLI-914)
	if err := validateFileHash(filePath); err != nil {
		logging.FluentEvent(proc.Logger()).Error("File hash validation failed", err).
			File(filePath).
			Log()
		return nil, errfmt.Newf("file hash validation failed").Wrap(err)
	}

	dl := cli.NewDataLoader(proc.Logger())
	data, _, err := dl.LoadFromFile(filePath)
	return data, err
}

// loadUpdatesFromData loads updates from inline data using internal/cli DataLoader.
func loadUpdatesFromData(cmd *cobra.Command, proc *cli.Processor) (map[string]any, error) {
	dataStr, _ := cmd.Flags().GetString("data")
	if dataStr == emptyValue {
		return nil, nil
	}
	dl := cli.NewDataLoader(proc.Logger())
	data, _, err := dl.LoadFromString(dataStr)
	return data, err
}

// loadUpdatesFromStdin loads updates from stdin using internal/cli DataLoader.
func loadUpdatesFromStdin(proc *cli.Processor) (map[string]any, error) {
	dl := cli.NewDataLoader(proc.Logger())
	data, _, err := dl.LoadFromStdin()
	return data, err
}

// stringSliceToAny converts []string to []any for storage/update compatibility.
func stringSliceToAny(s []string) []any {
	out := make([]any, len(s))
	for i, v := range s {
		out[i] = v
	}
	return out
}

// parseRefFlag parses a field=id string for reference operations.
func parseRefFlag(s string) (field, refVal string, err error) {
	for _, sep := range []string{"=", ":"} {
		if i := strings.Index(s, sep); i > 0 {
			field = strings.TrimSpace(s[:i])
			refVal = strings.TrimSpace(s[i+len(sep):])
			if field != emptyValue && refVal != emptyValue {
				return field, refVal, nil
			}
		}
	}
	return "", "", errfmt.Errorf("invalid format: use field=id (e.g. milestone_refs=MIL-001)")
}

// buildRefUpdatesFromAddRemoveFlags builds updates for reference fields from --add-ref and --remove-ref (BLI-853).
// Validates that add-ref targets exist. Returns nil, nil if no add-ref/remove-ref flags.
func buildRefUpdatesFromAddRemoveFlags(cmd *cobra.Command, proc *cli.Processor, currentObj map[string]any, baseUpdates map[string]any) (map[string]any, error) {
	addRefs, _ := cmd.Flags().GetStringArray("add-ref")
	removeRefs, _ := cmd.Flags().GetStringArray("remove-ref")
	if len(addRefs) == 0 && len(removeRefs) == 0 {
		return nil, nil
	}
	if currentObj == nil {
		return nil, errfmt.Errorf("current object is required for --add-ref and --remove-ref (object not found or --force)")
	}

	refUpdates := make(map[string]any)

	refIDFromRef := func(refVal string) string {
		parsed := validation.ParseNamespace(refVal)
		if parsed != nil && parsed.ObjectID != emptyValue {
			return parsed.ObjectID
		}
		return refVal
	}

	getCurrentRefs := func(field string, overrides map[string]any) (isSlice bool, single string, slice []string) {
		v := overrides[field]
		if v == nil && baseUpdates != nil {
			v = baseUpdates[field]
		}
		if v == nil {
			v = currentObj[field]
		}
		if v == nil {
			return strings.HasSuffix(field, "_refs"), "", nil
		}
		switch v := v.(type) {
		case string:
			return false, v, nil
		case []any:
			out := make([]string, 0, len(v))
			for _, a := range v {
				if s, ok := a.(string); ok {
					out = append(out, s)
				}
			}
			return true, "", out
		}

		return strings.HasSuffix(field, "_refs"), "", nil
	}

	// Apply add-ref: validate target exists, then add (use refUpdates as overrides for chained adds)
	for _, s := range addRefs {
		field, refVal, err := parseRefFlag(s)
		if err != nil {
			return nil, err
		}
		if !strings.HasSuffix(field, "_ref") && !strings.HasSuffix(field, "_refs") {
			return nil, errfmt.Errorf("--add-ref field must be a reference field (*_ref or *_refs): %s", field)
		}
		objectID := refIDFromRef(refVal)
		relaxed := false
		if cmd.Flags().Lookup("relaxed") != nil {
			relaxed, _ = cmd.Flags().GetBool("relaxed")
		}
		if !relaxed {
			exists, err := proc.Storage().Exists(proc.OperationContext(), proc.SecurityContext(), objectID)
			if err != nil {
				return nil, errfmt.Errorf("failed to check if reference target exists %q: %w", objectID, err)
			}
			if !exists {
				return nil, errfmt.Errorf("reference target does not exist: %s (resolved id: %s)", refVal, objectID)
			}
		}
		isSlice, _, slice := getCurrentRefs(field, refUpdates)
		if isSlice {
			seen := make(map[string]bool)
			for _, r := range slice {
				seen[r] = true
			}
			if !seen[refVal] {
				slice = append(slice, refVal)
			}
			refUpdates[field] = stringSliceToAny(slice)
		} else {
			refUpdates[field] = refVal
		}
	}

	// Apply remove-ref (use refUpdates so removes see previous adds)
	for _, s := range removeRefs {
		field, refVal, err := parseRefFlag(s)
		if err != nil {
			return nil, err
		}
		if !strings.HasSuffix(field, "_ref") && !strings.HasSuffix(field, "_refs") {
			return nil, errfmt.Errorf("--remove-ref field must be a reference field (*_ref or *_refs): %s", field)
		}
		isSlice, single, slice := getCurrentRefs(field, refUpdates)
		if isSlice {
			newSlice := make([]string, 0, len(slice))
			refID := refIDFromRef(refVal)
			for _, r := range slice {
				if r != refVal && refIDFromRef(r) != refID {
					newSlice = append(newSlice, r)
				}
			}
			refUpdates[field] = stringSliceToAny(newSlice)
		} else {
			if single == refVal || refIDFromRef(single) == refIDFromRef(refVal) {
				refUpdates[field] = ""
			}
		}
	}

	return refUpdates, nil
}

// applyUnsetFieldFlags merges --unset-field into updates using storage.FieldUnset (removes keys on persist).
// Skips immutable/control field names.
func applyUnsetFieldFlags(cmd *cobra.Command, updates map[string]any) {
	unsetFields, _ := cmd.Flags().GetStringArray("unset-field")
	for _, fn := range unsetFields {
		fn = strings.TrimSpace(fn)
		if fn == emptyValue {
			continue
		}
		switch fn {
		case "id", "kind", "created_at", "created_by", "expected_updated_at":
			continue
		}
		updates[fn] = storage.FieldUnset
	}
}

// applyAutoStatusFlag sets updates[status] to the next lifecycle-valid status when --auto-status is set.
// It is a no-op when --auto-status is false or status is already explicitly provided.
func applyAutoStatusFlag(cmd *cobra.Command, currentObj map[string]any, updates map[string]any) error {
	autoStatus, _ := cmd.Flags().GetBool("auto-status")
	if !autoStatus {
		return nil
	}
	if currentObj == nil {
		return errfmt.Errorf("--auto-status requires an existing object")
	}
	if _, hasStatus := updates[objects.FieldKeyStatus]; hasStatus {
		return errfmt.Errorf("--auto-status cannot be combined with an explicit status update")
	}
	kind, _ := currentObj[objects.FieldKeyKind].(string)
	hasTrait, err := objects.KindHasTrait(kind, "auto_status_transitionable")
	if err != nil {
		return errfmt.Errorf("failed to evaluate auto-status trait for kind %q: %w", kind, err)
	}
	if !hasTrait {
		return errfmt.Errorf("--auto-status not supported for kind %q (missing auto_status_transitionable trait)", kind)
	}
	nextStatus, err := deriveNextLifecycleStatus(currentObj)
	if err != nil {
		return err
	}
	updates[objects.FieldKeyStatus] = nextStatus
	return nil
}

// deriveNextLifecycleStatus returns the next progress-only lifecycle status.
func deriveNextLifecycleStatus(currentObj map[string]any) (string, error) {
	kind, _ := currentObj[objects.FieldKeyKind].(string)
	currentStatus, _ := currentObj[objects.FieldKeyStatus].(string)
	return objects.NextProgressLifecycleStatus(kind, currentStatus)
}

// buildUpdatesMap builds the complete updates map from all sources
// Uses shared utility but adds object-specific file hash validation
// currentObj is required for append operations (field+=value)
func buildUpdatesMap(cmd *cobra.Command, proc *cli.Processor, currentObj map[string]any) (map[string]any, error) {
	updates := make(map[string]any)

	// 1. Load from file (with hash validation), data, or stdin
	fileUpdates, err := loadUpdatesFromFile(cmd, proc)
	if err != nil {
		return nil, err
	}
	if fileUpdates != nil {
		maps.Copy(updates, fileUpdates)
	} else {
		dataUpdates, err := loadUpdatesFromData(cmd, proc)
		if err != nil {
			return nil, err
		}
		if dataUpdates != nil {
			maps.Copy(updates, dataUpdates)
		} else {
			// Try stdin only if no other update flags are set
			fields, _ := cmd.Flags().GetStringArray("field")
			unsetFields, _ := cmd.Flags().GetStringArray("unset-field")
			autoStatus, _ := cmd.Flags().GetBool("auto-status")
			addRefs, _ := cmd.Flags().GetStringArray("add-ref")
			removeRefs, _ := cmd.Flags().GetStringArray("remove-ref")
			if len(fields) == 0 && len(unsetFields) == 0 && !autoStatus && len(addRefs) == 0 && len(removeRefs) == 0 {
				stdinUpdates, err := loadUpdatesFromStdin(proc)
				if err != nil {
					return nil, err
				}
				maps.Copy(updates, stdinUpdates)
			}
		}
	}

	// 2. Reference add/remove (BLI-853) — apply next so it overrides file/data but --field can still override
	refUpdates, err := buildRefUpdatesFromAddRemoveFlags(cmd, proc, currentObj, updates)
	if err != nil {
		return nil, err
	}
	maps.Copy(updates, refUpdates)

	// 3. Build from --field flags (pass current object for append operations)
	fieldUpdates, err := buildUpdatesFromFieldFlags(cmd, proc, currentObj)
	if err != nil {
		return nil, err
	}
	// Deep-ish merge for fieldUpdates to handle nested maps (e.g. meta.tags)
	for k, v := range fieldUpdates {
		if vMap, ok := v.(map[string]any); ok {
			if existingMap, ok := updates[k].(map[string]any); ok {
				for subK, subV := range vMap {
					existingMap[subK] = subV
				}
				continue
			}
		}
		updates[k] = v
	}

	// 4. Apply auto-status
	if err := applyAutoStatusFlag(cmd, currentObj, updates); err != nil {
		return nil, err
	}

	// 5. Apply unset fields (removes keys)
	applyUnsetFieldFlags(cmd, updates)

	// Drop get-time hydration keys so CAS stays sealed.
	_ = objectget.StripReferenceResolverOverlayFields(updates)

	if len(updates) == 0 {
		return nil, errfmt.Errorf("no updates provided (use --file, --data, --field, --add-ref/--remove-ref, --unset-field, --auto-status, or pipe from stdin)")
	}

	return updates, nil
}

// addOptimisticLocking adds optimistic locking if provided
func addOptimisticLocking(cmd *cobra.Command, updates map[string]any) {
	expectedUpdatedAt, _ := cmd.Flags().GetString("expected-updated-at")
	if expectedUpdatedAt != emptyValue {
		updates["expected_updated_at"] = expectedUpdatedAt
	}
}

// handleUpdateDryRun handles dry-run mode for single object update using internal/cli DryRunHandler.
func handleUpdateDryRun(cmd *cobra.Command, id string, current map[string]any, updates map[string]any, proc *cli.Processor) (bool, error) {
	dr := cli.NewDryRunHandler(proc.Logger())
	handled, result, err := dr.HandleUpdateDryRunResult(cmd, id, current, updates)
	if err != nil {
		return handled, err
	}
	if handled && result != nil {
		return true, cli.FormatOutput(cmd, result)
	}
	return handled, nil
}

// buildUpdateCacheContext builds the cache context for an update operation
func buildUpdateCacheContext(proc *cli.Processor, id string, updates map[string]any, objKind string) stdcontext.Context {
	// Check if ID is being changed
	if newID, ok := updates[objects.FieldKeyID].(string); ok && newID != id {
		// ID change - need to invalidate old and update new
		return pkgctx.WithCacheIDChange(proc.OperationContext(), id, newID, objKind, "")
	}
	// Normal update - just update cache
	return pkgctx.WithCacheUpdate(proc.OperationContext(), id, objKind, "")
}

// buildUpdateAllFilters builds filters from --filter flags
func buildUpdateAllFilters(cmd *cobra.Command, proc *cli.Processor) (map[string]any, error) {
	filters := make(map[string]any)
	filterStrs, _ := cmd.Flags().GetStringArray("filter")
	for _, filterStr := range filterStrs {
		fieldName, filterValue, err := ParseFilterString(filterStr)
		if err != nil {
			logging.FluentEvent(proc.Logger()).Error("Failed to parse filter", err).
				String("filter", filterStr).
				Log()
			return nil, errfmt.Errorf("failed to parse filter %s: %w", filterStr, err)
		}
		filters[fieldName] = filterValue
	}
	return filters, nil
}

// buildObjectUpdates builds object-specific updates including missing fields
func buildObjectUpdates(obj map[string]any, kind string, baseUpdates map[string]any, addMissingFields bool, proc *cli.Processor) map[string]any {
	objUpdates := make(map[string]any)
	maps.Copy(objUpdates, baseUpdates)

	if addMissingFields {
		missingFields := getMissingFields(obj, kind, proc)
		for k, v := range missingFields {
			if _, exists := objUpdates[k]; !exists {
				objUpdates[k] = v
			}
		}
	}

	return objUpdates
}

// getMissingFields identifies and adds missing fields based on object spec
// Currently focuses on namespace_id which is automatically derived
func getMissingFields(obj map[string]any, kind string, _ *cli.Processor) map[string]any {
	missing := make(map[string]any)

	// Check for namespace_id - this is the main field we want to backfill
	if _, hasNamespace := obj[objects.FieldKeyNamespaceID]; !hasNamespace {
		// This will be set by ensureObjectMetadata, but we can derive it here too
		id, _ := obj[objects.FieldKeyID].(string)
		if id != emptyValue {
			// Use the same logic as ensureObjectMetadata
			parsed := validation.ParseNamespace(id)
			when.When(func() bool { return parsed != nil && parsed.NamespaceID != emptyValue }).Then(func() {
				missing[objects.FieldKeyNamespaceID] = parsed.NamespaceID
			}).OrElse(func() {
				registry := validation.GetNamespaceRegistry()
				missing[objects.FieldKeyNamespaceID] = registry.GetNamespaceForKind(kind)
			}).Run()
		}
	}

	return missing
}

// countObjectsToUpdate counts how many objects will actually be updated
func countObjectsToUpdate(result *storage.QueryResult, kind string, baseUpdates map[string]any, addMissingFields bool, proc *cli.Processor) int {
	count := 0
	for _, obj := range result.Objects {
		objID, _ := obj[objects.FieldKeyID].(string)
		if objID == emptyValue {
			continue
		}
		objUpdates := buildObjectUpdates(obj, kind, baseUpdates, addMissingFields, proc)
		if len(objUpdates) > 0 {
			count++
		}
	}
	return count
}

// shouldUseBulkInvalidation determines if bulk cache invalidation should be used
func shouldUseBulkInvalidation(objectsToUpdate int, totalObjects int, dryRun bool) bool {
	const cacheInvalidationThreshold = 0.30 // 30% threshold
	if dryRun {
		return false
	}
	if totalObjects == 0 {
		return false
	}
	updateRatio := float64(objectsToUpdate) / float64(totalObjects)
	return updateRatio > cacheInvalidationThreshold
}

// performBulkCacheInvalidation performs bulk cache invalidation for a kind
func performBulkCacheInvalidation(kind string, proc *cli.Processor) {
	logging.FluentEvent(proc.Logger()).Info("Bulk update detected - invalidating cache for entire kind").
		Kind(kind).
		Log()

	// Invalidate the entire kind's cache
	objectidcache.InvalidateObjectIDCacheKind(kind)

	// Rebuild cache in background
	projectRoot := proc.ProjectRoot()
	if projectRoot != emptyValue {
		logger := proc.Logger()
		goroutinelabels.NewGoroutine("object_id_cache_rebuilder", fmt.Sprintf("rebuilding object ID cache for %s", kind)).
			StartSimple(func() {
				cache := objectidcache.GetGlobalObjectIDCache()
				err := cache.BuildCache(pkgctx.NewSystemContext(), projectRoot, false)
				when.When(func() bool { return err != nil }).Then(func() {
					logging.FluentEvent(logger).Warn("Failed to rebuild cache in background").
						Kind(kind).
						WithError(err).
						Log()
				}).OrElse(func() {
					logging.FluentEvent(logger).Debug("Cache rebuilt in background").
						Kind(kind).
						Log()
				}).Run()
			})
	}
}

// formatUpdateAllResults formats the results of a bulk update operation
func formatUpdateAllResults(kind string, successCount int, errorCount int, errors []string, dryRun bool) []byte {
	var buf bytes.Buffer
	fmt.Fprintf(&buf, "Update complete for kind %s:\n", kind)
	fmt.Fprintf(&buf, "  Success: %d\n", successCount)
	if errorCount > 0 {
		fmt.Fprintf(&buf, "  Errors: %d\n", errorCount)
		for _, errMsg := range errors {
			fmt.Fprintf(&buf, "    - %s\n", errMsg)
		}
	}
	if dryRun {
		buf.WriteString("  (Dry-run mode - no changes made)\n")
	}
	return buf.Bytes()
}

// guardManualStatusUpdate refuses --field status= unless promote/demote/park or an
// audited override. Multi-ID and --all must use this same door as single-ID update.
func guardManualStatusUpdate(cmd *cobra.Command, proc *cli.Processor, id, objKind string, updates map[string]any) error {
	statusVal, hasStatus := updates[objects.FieldKeyStatus]
	if !hasStatus {
		return nil
	}
	force, _ := cmd.Flags().GetBool("force")
	if objKind == objects.KindBacklogItem && statusVal == objects.ObjectStatusInProgress && proc != nil && !force {
		if pErr := enforceActivePlanMembership(proc.OperationContext(), proc.SecurityContext(), proc.Storage(), id); pErr != nil {
			return cli.Guard(cmd).Require(false, pErr.Error()).Return()
		}
		if pErr := enforcePriorityOrder(proc.OperationContext(), proc.SecurityContext(), proc.Storage(), id); pErr != nil {
			return cli.Guard(cmd).Require(false, pErr.Error()).Return()
		}
	}
	return refuseManualStatusUnlessOverride(cmd, proc, id, objKind)
}

func refuseManualStatusUnlessOverride(cmd *cobra.Command, proc *cli.Processor, id, objKind string) error {
	if cmd.Flags().Lookup("auto-status") != nil {
		if autoStatus, _ := cmd.Flags().GetBool("auto-status"); autoStatus {
			return nil
		}
	}
	override := false
	if cmd.Flags().Lookup("override") != nil {
		override, _ = cmd.Flags().GetBool("override")
	}
	if !override {
		exe := brand.ExecutableName()
		return cli.Guard(cmd).Require(false, fmt.Sprintf("manual status updates are restricted to preserve lifecycle integrity. Use '%s object promote|demote|park <id>' to move through the lifecycle state machine. Human interactive TTY snap-remedy (--override) is blocked for non-TTY/agent shells", exe)).Return()
	}
	reasonCode := emptyValue
	if cmd.Flags().Lookup("reason-code") != nil {
		reasonCode, _ = cmd.Flags().GetString("reason-code")
	}
	if reasonCode == emptyValue {
		return cli.Guard(cmd).Require(false, "--reason-code is required when using --override").Return()
	}
	if proc == nil {
		return nil
	}
	return clipkg.EnforceOverrideFriction(cmd, proc.OperationContext(), proc.SecurityContext(), proc.Storage(), id, objKind, reasonCode)
}

// guardManualRefFieldUpdates refuses direct mutation of *_ref and *_refs fields via --field,
// requiring first-class 'object ref add/remove' commands or audited break-glass --override.
func guardManualRefFieldUpdates(cmd *cobra.Command, proc *cli.Processor, id, objKind string, updates map[string]any) error {
	var refFields []string
	for k := range updates {
		if strings.HasSuffix(k, "_ref") || strings.HasSuffix(k, "_refs") {
			refFields = append(refFields, k)
		}
	}
	if len(refFields) == 0 {
		return nil
	}

	// Legacy flags --add-ref and --remove-ref explicitly manipulate ref fields
	allowedRefFields := make(map[string]bool)
	if cmd != nil && cmd.Flags() != nil {
		if cmd.Flags().Lookup("add-ref") != nil {
			addRefs, _ := cmd.Flags().GetStringArray("add-ref")
			for _, s := range addRefs {
				if f, _, err := parseRefFlag(s); err == nil {
					allowedRefFields[f] = true
				}
			}
		}
		if cmd.Flags().Lookup("remove-ref") != nil {
			removeRefs, _ := cmd.Flags().GetStringArray("remove-ref")
			for _, s := range removeRefs {
				if f, _, err := parseRefFlag(s); err == nil {
					allowedRefFields[f] = true
				}
			}
		}
		if cmd.Flags().Lookup("unset-field") != nil {
			unsetFields, _ := cmd.Flags().GetStringArray("unset-field")
			for _, f := range unsetFields {
				allowedRefFields[f] = true
			}
		}
	}

	var violatingFields []string
	for _, f := range refFields {
		if !allowedRefFields[f] {
			violatingFields = append(violatingFields, f)
		}
	}
	if len(violatingFields) == 0 {
		return nil
	}

	override := false
	if cmd != nil && cmd.Flags() != nil && cmd.Flags().Lookup("override") != nil {
		override, _ = cmd.Flags().GetBool("override")
	}
	if !override {
		exe := brand.ExecutableName()
		return cli.Guard(cmd).Require(false, fmt.Sprintf("⚡️ AGENT POISON PILL: Direct mutation of reference field(s) %v is prohibited to preserve graph integrity. Use '%s object ref add <id> <target_id>' or '%s object ref remove <id> <target_id>' instead. Human override requires --override --reason-code=<reason>.", violatingFields, exe, exe)).Return()
	}
	reasonCode := emptyValue
	if cmd != nil && cmd.Flags() != nil && cmd.Flags().Lookup("reason-code") != nil {
		reasonCode, _ = cmd.Flags().GetString("reason-code")
	}
	if reasonCode == emptyValue {
		return cli.Guard(cmd).Require(false, "--reason-code is required when using --override on reference fields").Return()
	}
	if proc == nil {
		return nil
	}
	return clipkg.EnforceOverrideFriction(cmd, proc.OperationContext(), proc.SecurityContext(), proc.Storage(), id, objKind, reasonCode)
}

// withUpdateBreakGlass arms DECIDE break_glass for --force/--override. Critical kinds
// require --reason-code.
func withUpdateBreakGlass(cmd *cobra.Command, ctx stdcontext.Context, objKind string) (stdcontext.Context, error) {
	force, _ := cmd.Flags().GetBool("force")
	override, _ := cmd.Flags().GetBool("override")
	if !force && !override {
		return ctx, nil
	}
	reasonCode, _ := cmd.Flags().GetString("reason-code")
	if reasonCode == emptyValue {
		reasonCode = "cli object update --force/--override"
	}
	if kernelcas.IsCriticalKind(objKind) {
		rc, _ := cmd.Flags().GetString("reason-code")
		if rc == emptyValue {
			return ctx, errfmt.Errorf("--reason-code is required when using --force/--override on critical kinds (Kernel Mutation Pipeline break_glass)")
		}
		reasonCode = rc
	}
	opCtx := pkgctx.WithLifecycleBreakGlass(ctx, reasonCode)
	if storage.IsCoreKernelKind(objKind) {
		coreCtx, coreErr := withCoreDeleteReasonFromFlags(cmd, opCtx)
		if coreErr != nil {
			return ctx, coreErr
		}
		opCtx = coreCtx
	}
	return opCtx, nil
}
