package object

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/fatih/color"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	"github.com/zqk-os/zqk/pkg/cliapp"
	"github.com/zqk-os/zqk/pkg/brand"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/storage"
	"github.com/zqk-os/zqk/pkg/utils/fileutil"
	"github.com/zqk-os/zqk/pkg/zqktime"
)

// RunCreateWithData runs the object create flow with pre-built objData (e.g. from quick commands).
// Does not load from --file/--data and does not cleanup any source file.
func RunCreateWithData(cmd *cobra.Command, kind string, objData map[string]any) error {
	proc, err := cli.NewProcessor(cmd)
	if err != nil {
		return cli.Guard(cmd).Err(err).Wrapf("failed to create processor: %w").Return()
	}
	kind, err = objects.ResolveAndValidateKindForProject(proc.ProjectRoot(), kind)
	if err != nil {
		return cli.Guard(cmd).Err(err).Return()
	}
	if err := guardManualStatusOnCreate(cmd, proc, kind, objData); err != nil {
		return cli.Guard(cmd).Err(err).Return()
	}
	if objData[objects.FieldKeyKind] == nil || objData[objects.FieldKeyKind] == emptyValue {
		objData[objects.FieldKeyKind] = kind
	}
	normalizeObjectData(objData, kind, proc)
	if err := ensureKindMatches(objData, kind, proc); err != nil {
		return cli.Guard(cmd).Err(err).Return()
	}
	handled, err := handleDryRun(cmd, objData, kind, proc)
	if err != nil {
		return cli.Guard(cmd).Err(err).Return()
	}
	if handled {
		return nil
	}
	relaxed, _ := cmd.Flags().GetBool("relaxed")
	if relaxed {
		setCacheCheckerForBatchCreation(proc)
	}
	objID, _ := objData[objects.FieldKeyID].(string)
	objKind, _ := objData[objects.FieldKeyKind].(string)
	force, _ := cmd.Flags().GetBool("force")
	promote, _ := cmd.Flags().GetBool("promote")
	casDirect, _ := cmd.Flags().GetBool("cas")
	// Sync create for interactive CLI latency (see create.go).
	opCtx := pkgctx.WithCacheUpdate(proc.OperationContext(), objID, objKind, "")
	opCtx = storage.WithCLIOperation(storage.WithSkipWriteBehind(opCtx))
	if promote || casDirect {
		opCtx = pkgctx.WithPromoteOnCreate(opCtx)
	}
	if err := proc.Storage().Create(opCtx, proc.SecurityContext(), objData); err != nil {
		if (err == storage.ErrObjectExists || strings.Contains(err.Error(), "already exists")) && force && objID != emptyValue {
			updateCtx := pkgctx.WithCacheUpdate(proc.OperationContext(), objID, objKind, "")
			if updateErr := proc.Storage().Update(updateCtx, proc.SecurityContext(), objID, objData); updateErr != nil {
				return cli.Guard(cmd).Err(updateErr).Wrapf("failed to update with --force: %w").Return()
			}
			logging.FluentEvent(proc.Logger()).Info("Updated existing object with --force").
				ObjectID(objID).
				Log()
		} else {
			return cli.Guard(cmd).Err(err).Wrapf("failed to create object: %w").Return()
		}
	}

	// Update object ID and kind from data (in case they were generated/normalized by storage)
	if id, ok := objData[objects.FieldKeyID].(string); ok && id != emptyValue {
		objID = id
	}
	if k, ok := objData[objects.FieldKeyKind].(string); ok && k != emptyValue {
	}

	return finalizeCLIObjectCreate(cmd, proc, objData, kind, objID, emptyValue)
}

// finalizeCLIObjectCreate proves membrane visibility after storage.Create and either
// reports success or lands a repair draft with an honest notice.
func finalizeCLIObjectCreate(cmd *cobra.Command, proc *cli.Processor, objData map[string]any, kind, objID, sourceFilePath string) error {
	flushCtx, cancelFlush := storage.DurabilityFlushContext()
	defer cancelFlush()

	var readErr error
	preFlushOK := false
	if objID == emptyValue {
		readErr = errfmt.Errorf("create produced empty object id")
	} else {
		// Prove same-process readability first (before flush, which can thrash indexes).
		_, readErr = proc.Storage().Read(proc.OperationContext(), proc.SecurityContext(), objID)
		preFlushOK = readErr == nil
		// Draft plane is authoritative for preliminary mints — if the file is on disk, do not
		// repair_draft on a transient Read miss (semantic decorator / loader races).
		if !preFlushOK && proc.ProjectRoot() != emptyValue {
			draftPath := storage.ObjectDraftPlanePath(proc.ProjectRoot(), kind, objID)
			if _, statErr := fileutil.Stat(draftPath); statErr == nil {
				if _, retryErr := proc.Storage().Read(proc.OperationContext(), proc.SecurityContext(), objID); retryErr == nil {
					readErr = nil
					preFlushOK = true
				} else {
					// File exists: treat as visible for finalize (get-by-id dual-reads draft plane).
					readErr = nil
					preFlushOK = true
					logging.FluentEvent(proc.Logger()).Warn("Create finalize: draft-plane file present; Read missed once").
						ObjectID(objID).
						Kind(kind).
						WithError(retryErr).
						Log()
				}
			}
		}
	}

	// Draft-plane creates are id-keyed YAML — CAS index flush only thrash-contends with the
	// scheduler/test matrix and does not make drafts more gettable. Skip when already on disk.
	onDraftPlane := false
	if objID != emptyValue && proc.ProjectRoot() != emptyValue {
		draftPath := storage.ObjectDraftPlanePath(proc.ProjectRoot(), kind, objID)
		if _, err := fileutil.Stat(draftPath); err == nil {
			onDraftPlane = true
		}
	}

	var flushErr error
	if !onDraftPlane {
		t0 := time.Now()
		flushErr = storage.EnsureCLIObjectMutationVisibleForProvider(flushCtx, proc.Storage(), proc.ProjectRoot(), []string{kind})
		logSlowCLIObjectMutationFlush(proc.Logger(), "create", objID, []string{kind}, time.Since(t0), 0)
	}

	if objID != emptyValue && !preFlushOK {
		// Only require post-flush recovery when the object was not already gettable.
		for attempt := 0; attempt < 4; attempt++ {
			_, readErr = proc.Storage().Read(proc.OperationContext(), proc.SecurityContext(), objID)
			if readErr == nil {
				break
			}
			if attempt < 3 {
				time.Sleep(time.Duration(30*(attempt+1)) * time.Millisecond)
			}
		}
	} else if preFlushOK {
		// Flush lag must not turn a proven create into repair_draft.
		readErr = nil
	}
	if readErr != nil {
		return emitCreateRepairDraft(cmd, proc, objData, kind, objID, flushErr, readErr)
	}

	if flushErr != nil {
		// Gettable: index lag is real but not a ghost. Do not claim "durable" on flush failure alone.
		logging.FluentEvent(proc.Logger()).Warn("Persist flush after object create lagged; object is gettable").
			WithError(flushErr).
			Kind(kind).
			ObjectID(objID).
			Log()
		fmt.Fprintln(cmd.ErrOrStderr(), color.YellowString(paths.RewriteCanonicalCLIInvocations(fmt.Sprintf("Warning: Object '%s' is readable, but CAS index refresh lagged (%v). If another process cannot get it yet, run: zqk system sync-cas-index --file <hash.yaml>", objID, flushErr))))
	}

	proc.TriggerCacheFreshnessCheck("create", []string{kind})
	if sourceFilePath != emptyValue {
		_ = cli.ClearLastDraftPointerIfPath(proc.ProjectRoot(), sourceFilePath)
	}
	cleanupSourceFile(cmd, sourceFilePath, proc)

	format := cli.GetFormat(cmd)
	if format == cli.FormatJSON || format == cli.FormatYAML || format == cli.FormatJSONRPC || format == cli.FormatStream {
		return cli.FormatOutput(cmd, objData)
	}
	msg := formatCreateSuccessMessage(objData, kind, proc)
	return cli.WriteOutput(cmd, []byte(msg))
}

// emitCreateRepairDraft preserves the payload under .zqk/drafts and refuses ghost success.
func emitCreateRepairDraft(cmd *cobra.Command, proc *cli.Processor, objData map[string]any, kind, objID string, flushErr, readErr error) error {
	draftPath, draftErr := writeCreateRepairDraft(proc.ProjectRoot(), kind, objID, objData)
	reason := "object get failed after create"
	if flushErr != nil {
		reason = fmt.Sprintf("%s; flush: %v", reason, flushErr)
	}
	reason = fmt.Sprintf("%s; read: %v", reason, readErr)
	if draftErr != nil {
		reason = fmt.Sprintf("%s; draft write failed: %v", reason, draftErr)
	}

	logging.FluentEvent(proc.Logger()).Warn("Create did not prove membrane visibility; repair draft written").
		WithError(readErr).
		Kind(kind).
		ObjectID(objID).
		String("draft_path", draftPath).
		String("reason", reason).
		Log()

	envelope := map[string]any{
		"create_status":        "repair_draft",
		"membrane_visible":     false,
		objects.FieldKeyReason: reason,
		"draft_path":           draftPath,
		objects.FieldKeyID:     objID,
		objects.FieldKeyKind:   kind,
		"object":               objData,
		"repair_hint":          paths.RewriteCanonicalCLIInvocations(fmt.Sprintf("Edit draft if needed, then: zqk object create %s  (uses last-draft). If a CAS hash file exists: zqk system sync-cas-index --file <path>", kind)),
	}
	format := cli.GetFormat(cmd)
	if format == cli.FormatJSON || format == cli.FormatYAML || format == cli.FormatJSONRPC || format == cli.FormatStream {
		_ = cli.FormatOutput(cmd, envelope)
	} else {
		fmt.Fprintln(cmd.ErrOrStderr(), color.YellowString(fmt.Sprintf(
			"Create did not prove membrane visibility for %s. Payload preserved at %s (%s).",
			objID, draftPath, reason,
		)))
	}

	if draftPath != emptyValue {
		return cli.Guard(cmd).Require(false, fmt.Sprintf(
			"create not membrane-visible for %s; repair draft at %s", objID, draftPath,
		)).Return()
	}
	return cli.Guard(cmd).Err(readErr).Wrapf("create not membrane-visible and draft write failed: %w").Return()
}

func writeCreateRepairDraft(projectRoot, kind, objID string, objData map[string]any) (string, error) {
	if projectRoot == emptyValue {
		projectRoot = "."
	}
	dir := filepath.Join(projectRoot, paths.ProjectDataDir, paths.DraftsDir)
	if err := fileutil.EnsureDir(dir); err != nil {
		return "", errfmt.Newf("create repair drafts dir").Wrap(err)
	}
	raw, err := yaml.Marshal(objData)
	if err != nil {
		return "", errfmt.Newf("marshal create repair draft").Wrap(err)
	}
	safeID := strings.Map(func(r rune) rune {
		if r == '/' || r == '\\' || r == ':' {
			return '-'
		}
		return r
	}, objID)
	if safeID == emptyValue {
		safeID = "unknown-id"
	}
	ts := zqktime.NowLayoutUTC(zqktime.LayoutLogRotateStamp)
	out := filepath.Join(dir, fmt.Sprintf("create-repair-%s-%s-%s.yaml", kind, safeID, ts))
	header := paths.RewriteCanonicalCLIInvocations(fmt.Sprintf("# Create repair draft — not yet in the membrane\n# Retry: zqk object create %s\n# Or: zqk system sync-cas-index --file <cas-hash.yaml> if the file exists\n\n", kind))
	if err := fileutil.WriteSecureFile(out, append([]byte(header), raw...)); err != nil {
		return "", errfmt.Newf("write create repair draft").Wrap(err)
	}
	absOut, err := filepath.Abs(out)
	if err == nil {
		_ = cli.WriteLastDraftPointer(projectRoot, cli.LastDraftScopeObject, kind, absOut)
		return absOut, nil
	}
	return out, nil
}

// loadObjectData loads object data from file, inline data, last-draft pointer, or stdin using internal/cli DataLoader.
func loadObjectData(cmd *cobra.Command, proc *cli.Processor, kind string) (objData map[string]any, filePath string, err error) {
	dl := cli.NewDataLoader(proc.Logger())
	hint := &cli.LastDraftHint{
		Scope: cli.LastDraftScopeObject,
		Kind:  kind,
	}
	return dl.LoadData(cmd, hint)
}

// normalizeObjectData normalizes object values to ensure correct types.
// Uses GetFieldsForKindIfLoaded only so we do not trigger LoadFields() on the create hot path (PRE_CHANGE_CHECKLIST §3).
func normalizeObjectData(objData map[string]any, kind string, proc *cli.Processor) {
	fieldRegistry := objects.GetGlobalFieldRegistry()
	kindFields, ok := fieldRegistry.GetFieldsForKindIfLoaded(kind)
	if !ok {
		return
	}
	if err := normalizeObjectValues(objData, kindFields); err != nil {
		logging.FluentEvent(proc.Logger()).Warn("Failed to normalize object values").
			WithError(err).
			Log()
	}
}

// handleKindSynonym handles special case for kind_synonym objects
func handleKindSynonym(objData map[string]any) {
	// For kind_synonym, the YAML's "kind" field is the target kind (spec field "kind")
	// Storage needs obj[kind] to be kind_synonym (object's own kind)
	// If target_kind is not set, use the "kind" field value as target_kind
	if _, hasTargetKind := objData[objects.FieldKeyTargetKind]; !hasTargetKind {
		if kindValue, ok := objData[objects.FieldKeyKind].(string); ok && kindValue != objects.KindSynonym {
			objData[objects.FieldKeyTargetKind] = kindValue
		}
	}
	objData[objects.FieldKeyKind] = objects.KindSynonym // Set object's own kind for storage
}

// ensureKindMatches ensures the object's kind matches the command's kind
func ensureKindMatches(objData map[string]any, kind string, proc *cli.Processor) error {
	if kind == objects.KindSynonym {
		handleKindSynonym(objData)
		return nil
	}

	return clipkg.EnsureKindMatches(objData, kind, proc.Logger())
}

// handleDryRun handles dry-run mode using internal/cli DryRunHandler and formats through output formatters.
func handleDryRun(cmd *cobra.Command, objData map[string]any, kind string, proc *cli.Processor) (bool, error) {
	dr := cli.NewDryRunHandler(proc.Logger())
	handled, result, err := dr.HandleCreateDryRunResult(cmd, objData, kind, "object")
	return processDryRunResult(cmd, handled, result, err)
}

// cleanupSourceFile removes the source file after successful creation.
// Uses shared utility from pkg/cli
func cleanupSourceFile(cmd *cobra.Command, filePath string, proc *cli.Processor) {
	clipkg.CleanupSourceFile(cmd, filePath, proc.Logger())
}

// formatCreateSuccessMessage formats the success message for object creation
// Uses shared utility from pkg/cli
func formatCreateSuccessMessage(objData map[string]any, kind string, proc *cli.Processor) string {
	return clipkg.FormatCreateSuccessMessage(objData, kind, "Object", proc.Logger())
}

// guardManualStatusOnCreate refuses user-supplied status on create unless audited break-glass --override.
func guardManualStatusOnCreate(cmd *cobra.Command, proc *cli.Processor, kind string, objData map[string]any) error {
	statusVal, hasStatus := objData[objects.FieldKeyStatus]
	if !hasStatus || statusVal == nil {
		return nil
	}
	statusStr, ok := statusVal.(string)
	if !ok || strings.TrimSpace(statusStr) == "" {
		return nil
	}

	if cmd.Flags().Lookup("promote") != nil {
		if promote, _ := cmd.Flags().GetBool("promote"); promote {
			return nil
		}
	}

	if loader := objects.GetGlobalLifecycleLoader(); loader != nil {
		if origin, err := loader.GetOriginStatus(kind); err == nil && origin != "" && statusStr == origin {
			return nil
		}
	}

	override := false
	if cmd.Flags().Lookup("override") != nil {
		override, _ = cmd.Flags().GetBool("override")
	}
	if !override {
		exe := brand.ExecutableName()
		return cli.Guard(cmd).Require(false, fmt.Sprintf("manual status assignment on create is prohibited to preserve lifecycle integrity. Objects start at lifecycle origin on the draft plane, or use '%s object create <kind> --promote' to advance to the initial shovel-ready status. Human interactive TTY snap-remedy (--override) is blocked for non-TTY/agent shells", exe)).Return()
	}
	objID, _ := objData[objects.FieldKeyID].(string)
	if objID == "" {
		objID = "create-" + kind
	}
	return enforceOverrideFrictionFromFlags(cmd, proc, objID, kind, "")
}
