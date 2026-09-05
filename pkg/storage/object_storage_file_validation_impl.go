package storage

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	caspkg "github.com/lanceman/zqk/pkg/storage/cas"

	"github.com/lanceman/zqk/pkg/execwrap"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/errfmt"
	"github.com/lanceman/zqk/pkg/logging"
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/storage/crud"
	"github.com/lanceman/zqk/pkg/validation"
	"github.com/lanceman/zqk/pkg/zqkenv"
)

// ============================================================================
// Validation Functions
// ============================================================================

// validateObjectBeforeCreation validates object before writing
func (f *FileObjectStorage) validateObjectBeforeCreation(ctx context.Context, obj map[string]any, kind string, secCtx *pkgctx.SecurityContext) error {
	// Check for blocking issues before write operations (except for automated kinds)
	id, _ := obj[objects.FieldKeyID].(string)
	if err := f.checkForBlockingIssuesBeforeWrite(ctx, OpCreate, kind, id); err != nil {
		return err
	}

	// Ensure kind field is set correctly (preserve the kind passed in, not ontology)
	obj[objects.FieldKeyKind] = kind
	objects.CoerceMutationFields(kind, obj)

	// Ensure metadata
	f.ensureObjectMetadata(ctx, obj, secCtx, true)

	// Fail-closed identity validation: title must not be empty, whitespace-only, or contain newlines (CRIT-CEF-S18-IDENTITY-CREATE-CAS-001).
	if titleVal, ok := obj[objects.FieldKeyTitle]; ok {
		if titleStr, ok := titleVal.(string); ok {
			trimmed := strings.TrimSpace(titleStr)
			if trimmed == "" || strings.Contains(titleStr, "\n") {
				return errfmt.Errorf("invalid object title %q: title must be non-empty and single line", titleStr)
			}
		}
	}

	// Validate object (spec, lifecycle, references)
	validateStart := time.Now()
	if err := f.validateObject(ctx, obj, kind, ""); err != nil {
		// Non-draft-plane CAS objects must fail closed. Incomplete criteria park
		// off CAS (create succeeds); hash persist is refused by the membrane.
		// TRACK: BLI-KERNEL-CRIT-CATEGORY-MINT-001
		useDraftPlane := caspkg.UseObjectDraftPlane(kind, obj, pkgctx.GetPromoteOnCreate(ctx))
		if StreamStorageEnabledForKind(kind) {
			useDraftPlane = false
		}
		if !useDraftPlane || crud.IsPlanMembershipHardBlockError(err) {
			var _err_hard = f.trackPersistenceStep(ctx, secCtx, OpValidateBeforeWrite, PersistenceStepValidateObject, map[string]any{objects.FieldKeyKind: kind, objects.FieldKeyID: id}, nil, time.Since(validateStart), err)
			if _err_hard != nil {
				logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).Error(ErrMsgSwallowedError, _err_hard).Log()
			}
			return err
		}
		logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
		StorageLog(logger).Warn("object validation failed before creation, but proceeding with write to keep object outside the membrane until resolved").
			Kind(kind).
			ObjectID(id).
			WithError(err).
			Log()

		var _err_83719230 = f.trackPersistenceStep(ctx, secCtx, OpValidateBeforeWrite, PersistenceStepValidateObject, map[string]any{objects.FieldKeyKind: kind, objects.FieldKeyID: id}, nil, time.Since(validateStart), err)
		if _err_83719230 != nil {
			logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).Error(ErrMsgSwallowedError, _err_83719230).Log()
		}
		// Do not return errfmt.Newf(ConstStreamValidationFailed).Wrap(err) here!
	}
	validateDuration := time.Since(validateStart)
	if validateDuration > 100*time.Millisecond {
		var _err_83719579 = f.trackPersistenceStep(ctx, secCtx, OpValidateBeforeWrite, PersistenceStepValidateObject, map[string]any{objects.FieldKeyKind: kind, objects.FieldKeyID: id}, map[string]any{"duration_ms": float64(validateDuration.Nanoseconds()) / 1e6}, validateDuration, nil)
		if _err_83719579 != nil {
			logging.

				// checkForBlockingIssuesBeforeWrite checks for Tier 1 (blocking) issues before write operations
				// Uses the BlockingCheckContext pattern to trigger system-level checks
				// Now highly customizable via workflow profiles and per-kind/per-operation rules
				Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).Error(ErrMsgSwallowedError, _err_83719579).Log()
		}
	}

	return nil
}

func (f *FileObjectStorage) checkForBlockingIssuesBeforeWrite(ctx context.Context, operationKind, kind, id string) error {
	// Check if this kind should bypass blocking checks (from external config)
	config := GetGlobalBlockingCheckConfig()
	if config.ShouldBypassBlockingCheck(kind) {
		return nil
	}

	// Check if system-level blocking is required for this kind/operation
	if !config.ShouldRequireSystemLevelCheck(kind, operationKind) {
		return nil // Skip system-level check if not required
	}

	// Create blocking check context
	blockingCtx := pkgctx.NewBlockingCheckContext(f.projectRoot, operationKind, kind, id)
	blockingCtx = blockingCtx.WithContext(ctx)

	// Process the context through the listener pipeline
	// This will trigger the registered blocking check handler
	resultChan := pkgctx.ProcessContext(blockingCtx)
	result := <-resultChan

	// Check if processing failed
	if result.FinalError != nil {
		// If check fails, log warning but don't block (best effort)
		logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
		StorageLog(logger).Warn(LogEventStorageObjectValidationBlockingCheckFailedWarn).
			Kind(kind).
			WithError(result.FinalError).
			Log()
		return nil // Don't block on check failures
	}

	// Check result from the handler
	if checkResult, ok := result.FinalResult.(*pkgctx.BlockingCheckResult); ok {
		if checkResult.HasBlockingIssues && len(checkResult.Issues) > 0 {
			// Format blocking issues for error message
			var issueMessages []string
			for i, issue := range checkResult.Issues {
				if i < 5 { // Limit to first 5 issues
					issueMessages = append(issueMessages, fmt.Sprintf("%s (%s): %s", issue.ObjectID, issue.ObjectKind, issue.Message))
				}
			}
			if len(checkResult.Issues) > 5 {
				issueMessages = append(issueMessages, fmt.Sprintf(ConstStreamEllipsisAndIntMoreBlockingIssues, len(checkResult.Issues)-5))
			}

			return errfmt.Errorf(
				ConstStreamBlockingIssuesDetectedResolveCriticalViolations+
					ConstStreamRunZqkSystemCheckToSeeAllIssuesN+
					"  Issues: %s",
				strings.Join(issueMessages, "; "))
		}
	}

	return nil
}

// validateObject validates an object against its spec, lifecycle, and references
// CRITICAL: This is called before ALL write operations (Create, Update, Move, Migration)
// Validation errors are immediately registered and prevent persistence of invalid objects
func (f *FileObjectStorage) validateObject(ctx context.Context, obj map[string]any, kind, currentState string) error {
	// Early detection: Check for minimal required fields before full validation
	// This catches partial/incomplete data immediately
	if err := crud.DetectPartialData(obj, kind); err != nil {
		// Register validation error immediately for partial/incomplete data
		f.registerValidationError(ctx, obj, kind, "partial_data", err.Error())
		return errfmt.Newf(ConstStreamPartialOrIncompleteDataDetected).Wrap(err)
	}

	// Un-spoofable Agent Task Security Gate
	// Work-done terminals (implemented) require a real commit. Archive is an abandon
	// hop — leftover remint debris has no commit_hash. TRACK: BLI-COMMS-ORCH-DRAFT-PLANE-DIRTY-001
	if kind == objects.KindAgentTask {
		status, _ := obj[objects.FieldKeyStatus].(string)
		if crud.AgentTaskWorkDoneRequiresCommit(status) {
			commitHash, _ := obj[objects.FieldKeyCommitHash].(string)
			if commitHash == "" {
				return errfmt.Errorf("Security Gate: agent_task cannot transition to terminal status without a valid commit_hash")
			}

			// Verify commit hash exists in git
			cmd := execwrap.CommandContext(ctx, "git", "cat-file", "-t", commitHash)
			cmd.Dir = f.projectRoot
			out, err := cmd.Output()
			if err != nil || strings.TrimSpace(string(out)) != "commit" {
				return errfmt.Errorf("Security Gate: commit_hash '%s' is not a valid commit in the repository", commitHash)
			}

			// Verify all task_steps are closed (step vocabulary ≠ agent_task lifecycle).
			if steps, ok := obj[objects.FieldKeyTaskSteps].([]any); ok {
				for i, stepAny := range steps {
					if stepMap, ok := stepAny.(map[string]any); ok {
						stepStatus, _ := stepMap[objects.FieldKeyStatus].(string)
						if !objects.TaskStepIsClosed(stepStatus) {
							return errfmt.Errorf("Security Gate: agent_task cannot transition to terminal status because task_step %d is '%s'", i+1, stepStatus)
						}
					}
				}
			}
		}
	}

	// 1. Spec validation using GoValidator
	// BLI-621 / KMP: --force skips lifecycle only when DECIDE break_glass reason is present for critical kinds.
	// TRACK: REDACTED
	if pkgctx.IsLifecycleBreakGlass(ctx) && IsCoreKernelKind(kind) {
		if !pkgctx.GetAllowCoreObjectDelete(ctx) && os.Getenv(zqkenv.TestRoot()) == "" {
			return errfmt.Errorf("break_glass requires --reason-code for critical kind %s (Kernel Mutation Pipeline DECIDE)", kind)
		}
	}
	// Break-glass and trusted shockwave writes still validate lifecycle. The
	// validator skips auto-only *edges* under those overrides; skipping here
	// dropped criteria/complete holds and allowed false-complete BLIs.
	// TRACK: REDACTED
	options := &validation.ValidationOptions{
		CurrentState:          currentState,
		ValidateLifecycle:     true,
		ValidateSemanticTypes: true,
		// Git mutation evidence must resolve commits in THIS storage tree.
		// Leaving ProjectRoot empty made FindNearestProjectRoot(".") use the
		// caller's cwd (the real repo under `go test`), so isolated fixtures
		// with a real product commit still failed the complete gate.
		// TRACK: REDACTED
		ProjectRoot: f.GetProjectRoot(),
	}
	options.ObjectLookup = func(id string) (map[string]any, error) {
		secCtx := pkgctx.NewSystemSecurityContext()
		return f.Read(ctx, secCtx, id)
	}
	options.ObjectStatusLookup = func(id string) (string, error) {
		obj, err := options.ObjectLookup(id)
		if err != nil {
			return "", err
		}
		status, _ := obj[objects.FieldKeyStatus].(string)
		return status, nil
	}
	options.DependentsLookup = func(id string) []string {
		return DependentsForID(ctx, f, id)
	}
	if fn := pkgctx.GetValidationProgress(ctx); fn != nil {
		options.ProgressCallback = func(stage, message string) { fn(stage, message) }
	}

	// Inject hook for shockwave cascade deletion of dependent objects
	options.OnValidationFailure = func(failObj map[string]any, failKind string, errors []validation.ValidationError) {
		// Removed asynchronous cascade delete of the object itself to prevent
		// race conditions during tests and accidental deletion of user data.
	}

	result, err := f.validator.Validate(ctx, obj, kind, options)
	if err != nil {
		// Register validation error immediately
		f.registerValidationError(ctx, obj, kind, ConstStreamValidationFailure, err.Error())
		return errfmt.Newf(ConstStreamSpecValidationFailed).Wrap(err)
	}

	if !result.IsValid {
		// Check tier configuration to determine which errors should block saves
		// Use the blocking check config which now includes validation tier blocking
		blockingConfig := GetGlobalBlockingCheckConfig()
		blockingErrors := blockingConfig.GetBlockingValidationErrors(result.Errors, kind, "")
		nonBlockingErrors := getNonBlockingValidationErrors(result.Errors, blockingConfig, kind, "")

		// Register all validation errors immediately (both blocking and non-blocking)
		// Log all errors for debugging (especially in test scenarios)
		for _, validationErr := range result.Errors {
			errorMsg := fmt.Sprintf("%s: %s", validationErr.Field, validationErr.Message)
			// Register each validation error immediately
			f.registerValidationError(ctx, obj, kind, validationErr.Rule, errorMsg)
			// Also log to help with debugging (especially in tests)
			logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
			StorageLog(logger).Debug(LogEventStorageObjectValidationErrorDetailDebug).
				String("field", validationErr.Field).
				String(ConstStreamValidationRule, validationErr.Rule).
				String("message", validationErr.Message).
				Kind(kind).
				ObjectID(crud.GetObjectID(obj)).
				Log()
		}

		// Check per-kind rules for allowing non-blocking errors
		allowNonBlocking := false
		if kindRules, ok := blockingConfig.PerKindRules[kind]; ok {
			allowNonBlocking = kindRules.AllowNonBlockingErrors
		}

		// If there are blocking errors, prevent save (UNLESS we are creating)
		if len(blockingErrors) > 0 {
			// Per user directive: don't fail 'create' unless vital info missing (which is checked in detectPartialData).
			// Validation should happen during promotion, so we downgrade blocking errors to non-blocking during creation.
			// Invalid create statuses are coerced to lifecycle origin in ensureCreateLifecycleStatus before validate.
			//
			// Exception — sealed priority_plan membership: create must NOT soft-accept a priority_plan_ref onto an
			// execution-facing plan (or draft-plane status onto one). Otherwise agents create exploring+active-plan,
			// then promote with the same ref and bypass new_link_only. TRACK: REDACTED
			if currentState == "" {
				var hard []validation.ValidationError
				var soft []validation.ValidationError
				for _, err := range blockingErrors {
					if crud.IsPlanMembershipHardBlockOnCreate(err) {
						hard = append(hard, err)
					} else {
						soft = append(soft, err)
					}
				}
				if len(hard) > 0 {
					var blockingMessages []string
					for _, err := range hard {
						tier := blockingConfig.GetTierForRule(err.Rule)
						blockingMessages = append(blockingMessages, fmt.Sprintf(ConstStreamStrTierIntStr, err.Field, tier, err.Message))
					}
					return errfmt.Errorf(ConstStreamValidationErrorsBlockSaveStr, strings.Join(blockingMessages, "; "))
				}
				nonBlockingErrors = append(nonBlockingErrors, soft...)
			} else {
				var blockingMessages []string
				for _, err := range blockingErrors {
					tier := blockingConfig.GetTierForRule(err.Rule)
					blockingMessages = append(blockingMessages, fmt.Sprintf(ConstStreamStrTierIntStr, err.Field, tier, err.Message))
				}
				return errfmt.Errorf(ConstStreamValidationErrorsBlockSaveStr, strings.Join(blockingMessages, "; "))
			}
		}

		// If there are only non-blocking errors, log them but allow save (unless per-kind rules override)
		if len(nonBlockingErrors) > 0 && allowNonBlocking {
			var nonBlockingMessages []string
			for _, err := range nonBlockingErrors {
				tier := blockingConfig.GetTierForRule(err.Rule)
				nonBlockingMessages = append(nonBlockingMessages, fmt.Sprintf(ConstStreamStrTierIntStr, err.Field, tier, err.Message))
			}
			logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
			StorageLog(logger).Warn(LogEventStorageObjectValidationNonBlockingWorkflowWarn).
				ObjectID(crud.GetObjectID(obj)).
				Kind(kind).
				String("errors", strings.Join(nonBlockingMessages, "; ")).
				Log()
		}

		// Allow save to proceed (only non-blocking errors)
		return nil
	}

	// 2. Reference validation
	refValidateStart := time.Now()
	status, _ := obj[objects.FieldKeyStatus].(string)
	if status != "error" {
		if err := f.validateReferences(obj, kind); err != nil {
			var _err_83726556 = f.trackPersistenceStep(ctx, nil, OpValidateObject, PersistenceStepValidateReferences, map[string]any{objects.FieldKeyKind: kind, objects.FieldKeyID: crud.GetObjectID(obj)}, nil, time.Since(refValidateStart), err)
			if _err_83726556 !=
				// Register reference validation error immediately
				nil {
				logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).Error(ErrMsgSwallowedError, _err_83726556).Log()
			}

			f.registerValidationError(ctx, obj, kind, "reference", err.Error())

			// Check if reference validation errors should block saves
			blockingConfig := GetGlobalBlockingCheckConfig()
			tier := blockingConfig.GetTierForRule("reference")

			// Auto-detect batch creation mode:
			// 1. Cache checker is set (explicit batch mode via --cascade or scenario builder)
			// 2. Referenced objects exist in cache but not on disk (being created in same batch)
			// Lock-free read using atomic.Value
			var cacheCheckerValue func(string) (string, bool)
			if val := cacheChecker.Load(); val != nil {
				cacheCheckerValue = val.(func(string) (string, bool))
			}
			isBatchMode := cacheCheckerValue != nil
			logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
			StorageLog(logger).Info(LogEventStorageObjectValidationBatchModeInfo).
				ObjectID(crud.GetObjectID(obj)).
				Kind(kind).
				Bool("is_batch_mode", isBatchMode).
				Bool(ConstStreamCacheCheckerSet, cacheCheckerValue != nil).
				String(ConstStreamCacheCheckerAddr, fmt.Sprintf("%p", cacheCheckerValue)).
				Log()
			if !isBatchMode {
				// Check if referenced objects are in cache but not yet on disk
				// This indicates they're being created in the same batch
				// Use the captured cacheCheckerValue for consistency
				isBatchMode = f.detectBatchCreationFromCache(ctx, obj, kind)
			}

			// During batch creation, reference validation is non-blocking
			// Objects are created in hierarchical order, so references will resolve as the batch completes
			if isBatchMode {
				// Batch creation mode - log but allow save (references will resolve in hierarchical order)
				StorageLog(logger).Warn(LogEventStorageObjectValidationBatchRefWarn).
					ObjectID(crud.GetObjectID(obj)).
					Kind(kind).
					WithError(err).
					Log()
				return nil
			}

			// Normal mode - check tier configuration
			if blockingConfig.IsBlockingTier(tier, kind, "") {
				return errfmt.Errorf(ConstStreamReferenceValidationFailedTierIntBlocksSaveErr, tier, err)
			}

			// Non-blocking reference error - log but allow save
			StorageLog(logger).Warn(LogEventStorageObjectValidationNonBlockingRefWarn).
				ObjectID(crud.GetObjectID(obj)).
				Kind(kind).
				WithError(err).
				Log()
			return nil
		}
	}
	refValidateDuration := time.Since(refValidateStart)
	if refValidateDuration > 100*time.Millisecond {
		var _err_83729117 = f.trackPersistenceStep(ctx, nil, OpValidateObject, PersistenceStepValidateReferences, map[string]any{objects.FieldKeyKind: kind, objects.FieldKeyID: crud.GetObjectID(obj)}, map[string]any{"duration_ms": float64(refValidateDuration.Nanoseconds()) / 1e6}, refValidateDuration, nil)
		if _err_83729117 != nil {
			logging.

				// detectBatchCreationFromCache checks if referenced objects exist in cache but not on disk
				// This indicates they're being created in the same batch, so we should allow the reference
				// Uses cacheChecker if available, otherwise returns false (can't access cache without circular dependency)
				Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).Error(ErrMsgSwallowedError, _err_83729117).Log()
		}
	}

	return nil
}
