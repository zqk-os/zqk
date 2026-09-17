package system

import (
	"context"
	"fmt"
	"time"

	"github.com/spf13/cobra"

	"github.com/lanceman/zqk/internal/cli"
	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/logging"
	"github.com/lanceman/zqk/pkg/migration/parser"
	"github.com/lanceman/zqk/pkg/storage"
	fileutil "github.com/lanceman/zqk/pkg/utils/fileutil"
	"github.com/lanceman/zqk/pkg/validation"
	"github.com/lanceman/zqk/pkg/when"

	"github.com/lanceman/zqk/pkg/objects"
)

// CheckObjectContext groups state for object checking
type CheckObjectContext struct {
	Ctx               *cli.Context
	StdCtx            context.Context // Standard context.Context for cancellation/timeouts
	Cmd               *cobra.Command
	Obj               *parser.ParsedObject
	FilePath          string
	Kind              string
	Content           []byte
	SpecLoader        *objects.SpecLoader
	LifecycleLoader   *objects.LifecycleLoader
	Validator         validation.Validator
	Registry          storage.HashRegistryProvider
	ObjectIDCache     *ObjectIDCache
	HashRegistryCache *HashRegistryCacheType
	DeferredChecks    *[]deferredHashCheck
	Logger            logging.Logger
	IsDebugObject     bool
	// StorageProvider is optional; when set (e.g. async check) ref validation reuses it instead of creating one per object.
	StorageProvider storage.ObjectStorageProvider
}

// initializeCheckObjectContext sets up the check object context.
// storageProvider is optional; when set (e.g. async system check) ref validation reuses it instead of creating one per object.
func initializeCheckObjectContext(ctx *cli.Context, stdCtx context.Context, cmd *cobra.Command, obj *parser.ParsedObject, filePath, kind string, content []byte, specLoader *objects.SpecLoader, lifecycleLoader *objects.LifecycleLoader, validator validation.Validator, registry storage.HashRegistryProvider, objectIDCache *ObjectIDCache, hashRegistryCache *HashRegistryCacheType, deferredChecks *[]deferredHashCheck, storageProvider storage.ObjectStorageProvider) *CheckObjectContext {
	logger := logging.GetLoggerFromProfile(ctx.Profile)
	isDebugObject := obj.ID == "POL-DEBUG-001"

	// Use provided context or create system context if not provided
	if stdCtx == nil {
		stdCtx = pkgctx.NewSystemContext()
	}

	return &CheckObjectContext{
		Ctx:               ctx,
		StdCtx:            stdCtx,
		Cmd:               cmd,
		Obj:               obj,
		FilePath:          filePath,
		Kind:              kind,
		Content:           content,
		SpecLoader:        specLoader,
		LifecycleLoader:   lifecycleLoader,
		Validator:         validator,
		Registry:          registry,
		ObjectIDCache:     objectIDCache,
		HashRegistryCache: hashRegistryCache,
		DeferredChecks:    deferredChecks,
		Logger:            logger,
		IsDebugObject:     isDebugObject,
		StorageProvider:   storageProvider,
	}
}

// normalizeKindForCheck normalizes the kind for checking.
// Kind-from-ID is source of truth: use ID-derived kind first, then object's declared kind, then discovery kind.
// This prevents wrong-kind validation (e.g. account in criteria dir validated as account, not criteria).
func normalizeKindForCheck(checkCtx *CheckObjectContext) string {
	if effectiveKind := inferKindFromID(checkCtx.Obj.ID); effectiveKind != emptyValue {
		return effectiveKind
	}
	if checkCtx.Obj.Kind != emptyValue {
		return checkCtx.Obj.Kind
	}
	return checkCtx.Kind
}

// readLatestContent reads the latest content from file or uses provided content
func readLatestContent(checkCtx *CheckObjectContext) ([]byte, error) {
	if len(checkCtx.Content) > 0 {
		return checkCtx.Content, nil
	}
	return fileutil.ReadFile(checkCtx.FilePath)
}

// parseObjectContent parses object content with fallback logic
func parseObjectContent(checkCtx *CheckObjectContext, latestContent []byte) map[string]any {
	// Decoder reuse: If the content is identical to what we already parsed, reuse the properties!
	if checkCtx.Obj != nil && checkCtx.Obj.Properties != nil {
		if len(latestContent) == 0 || (len(latestContent) == len(checkCtx.Content)) {
			// Fast path: avoid double-parsing the YAML!
			return checkCtx.Obj.Properties
		}
	}

	yamlParser := parser.NewYAMLParser()

	if len(latestContent) > 0 {
		latestObj, parseErr := yamlParser.ParseBytes(latestContent)
		if parseErr == nil && latestObj.Properties != nil {
			logDebugParseSuccess(checkCtx, latestObj.Properties)
			return latestObj.Properties
		}
		// Parsing failed - try content parameter as fallback
		return getFallbackObjectMapWithContent(checkCtx, yamlParser)
	}

	// File read failed or no latestContent - try content parameter
	if len(checkCtx.Content) > 0 {
		contentObj, contentParseErr := yamlParser.ParseBytes(checkCtx.Content)
		if contentParseErr == nil && contentObj.Properties != nil {
			if checkCtx.IsDebugObject {
				logging.Fluent(checkCtx.Logger).Info("Using content parameter parse result (file read failed)").Log()
			}
			return contentObj.Properties
		}
	}

	return getFallbackObjectMap(checkCtx)
}

// getFallbackObjectMap gets fallback object map from original properties
func getFallbackObjectMap(checkCtx *CheckObjectContext) map[string]any {
	if checkCtx.Obj.Properties != nil {
		if checkCtx.IsDebugObject {
			logging.Fluent(checkCtx.Logger).Warn("Falling back to original obj.Properties (no content available)").Log()
		}
		return checkCtx.Obj.Properties
	}
	return make(map[string]any)
}

// getFallbackObjectMapWithContent tries content parameter as fallback
func getFallbackObjectMapWithContent(checkCtx *CheckObjectContext, yamlParser *parser.YAMLParser) map[string]any {
	if checkCtx.IsDebugObject {
		logging.Fluent(checkCtx.Logger).Warn("Latest parse failed, trying content parameter").Log()
	}

	if len(checkCtx.Content) > 0 {
		contentObj, contentParseErr := yamlParser.ParseBytes(checkCtx.Content)
		if contentParseErr == nil && contentObj.Properties != nil {
			if checkCtx.IsDebugObject {
				logging.Fluent(checkCtx.Logger).Info("Using content parameter parse result").Log()
			}
			return contentObj.Properties
		}
	}

	if checkCtx.Obj.Properties != nil {
		if checkCtx.IsDebugObject {
			logging.Fluent(checkCtx.Logger).Warn("Falling back to original obj.Properties (both parses failed)").Log()
		}
		return checkCtx.Obj.Properties
	}

	return make(map[string]any)
}

// logDebugParseSuccess logs debug info for successful parse
func logDebugParseSuccess(checkCtx *CheckObjectContext, objMap map[string]any) {
	if !checkCtx.IsDebugObject {
		return
	}

	bodyVal, bodyExists := objMap[objects.FieldKeyBody]
	categoryVal, categoryExists := objMap[objects.FieldKeyCategory]
	policyTypeVal, policyTypeExists := objMap[objects.FieldKeyPolicyType]
	logging.Fluent(checkCtx.Logger).Info("=== POL-DEBUG-001 DEBUG: AFTER RE-PARSE (latestContent) ===").
		Int("objMap_len", len(objMap)).
		String("body_exists", fmt.Sprintf("%v", bodyExists)).
		String("body_type", fmt.Sprintf("%T", bodyVal)).
		String("category_exists", fmt.Sprintf("%v", categoryExists)).
		String("category_value", fmt.Sprintf("%v", categoryVal)).
		String("policy_type_exists", fmt.Sprintf("%v", policyTypeExists)).
		String("policy_type_value", fmt.Sprintf("%v", policyTypeVal)).
		Log()
}

// logValidationPhaseStart logs at the start of a phase so timeouts can be attributed to the running phase.
// For audit_event/doc_entry, when --verbose is set, uses Warn for likely-slow phases.
// Without --verbose, only Debug-level logging is used to avoid flooding output.
func logValidationPhaseStart(checkCtx *CheckObjectContext, phase string) {
	if checkCtx.Kind != objects.KindAuditEvent && checkCtx.Kind != objects.KindDocEntry {
		return
	}
	verbose := false
	if checkCtx.Cmd != nil {
		verbose, _ = checkCtx.Cmd.Flags().GetBool("verbose") //nolint:errcheck
	}
	sysLogger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
	when.When(func() bool {
		return (phase == "instance_validation" || phase == "reference_integrity" || phase == "integrity") && verbose
	}).Then(func() {
		logging.Fluent(sysLogger).Warn("validation_phase_start").
			String("object_id", checkCtx.Obj.ID).
			String("phase", phase).
			Log()
	}).OrElse(func() {
		logging.Fluent(sysLogger).Debug("validation_phase_start").
			String("object_id", checkCtx.Obj.ID).
			String("phase", phase).
			Log()
	}).Run()
}

// logValidationPhaseDuration logs phase timing for audit_event/doc_entry (debug) or when phase exceeds slowThreshold (warn).
// Used to trace where validation time is spent when timeouts occur (doc_entry under load; audit_event historically).
func logValidationPhaseDuration(checkCtx *CheckObjectContext, phase string, elapsed time.Duration) {
	const slowThreshold = 2 * time.Second
	if checkCtx.Kind == objects.KindAuditEvent || checkCtx.Kind == objects.KindDocEntry {
		logging.Fluent(checkCtx.Logger).Debug("validation_phase").
			String("object_id", checkCtx.Obj.ID).
			String("phase", phase).
			String("duration", elapsed.String()).
			Log()
	}
	if elapsed >= slowThreshold {
		logging.Fluent(checkCtx.Logger).Warn("validation_phase_slow").
			String("object_id", checkCtx.Obj.ID).
			Kind(checkCtx.Kind).
			String("phase", phase).
			String("duration", elapsed.String()).
			Log()
	}
}

// performAllChecks performs all validation checks on the object
func performAllChecks(checkCtx *CheckObjectContext, objMap map[string]any) CheckResult {
	// Report the object's actual kind from file content when present for accurate violation output
	// (e.g. SHM-* in metrics dir reported as scheduler_health_metric, not directory-derived kind)
	reportKind := checkCtx.Kind
	if checkCtx.Obj.Kind != emptyValue {
		reportKind = checkCtx.Obj.Kind
	}
	result := CheckResult{
		ObjectID:   checkCtx.Obj.ID,
		ObjectKind: reportKind,
		FilePath:   checkCtx.FilePath,
		Status:     checkCtx.Obj.Status,
		Issues:     []Issue{},
	}

	// 2. Registration validation
	logValidationPhaseStart(checkCtx, "registration")
	t0 := time.Now()
	result.Issues = append(result.Issues, checkRegistration(checkCtx.Obj, checkCtx.Kind)...)
	logValidationPhaseDuration(checkCtx, "registration", time.Since(t0))

	// 3. Lifecycle validation
	logValidationPhaseStart(checkCtx, objects.KindLifecycle)
	t0 = time.Now()
	result.Issues = append(result.Issues, checkLifecycleWithLoader(checkCtx.Obj, checkCtx.Kind, checkCtx.LifecycleLoader)...)
	logValidationPhaseDuration(checkCtx, objects.KindLifecycle, time.Since(t0))

	// 4. Instance validation
	logValidationPhaseStart(checkCtx, "instance_validation")
	t0 = time.Now()
	instanceIssues := checkInstanceValidationWithValidatorAndData(checkCtx.Ctx, checkCtx.StdCtx, checkCtx.Obj, checkCtx.Kind, checkCtx.Validator, objMap, checkCtx.StorageProvider)
	result.Issues = append(result.Issues, instanceIssues...)
	logValidationPhaseDuration(checkCtx, "instance_validation", time.Since(t0))

	// 5. Policy compliance
	logValidationPhaseStart(checkCtx, objects.KindPolicy)
	t0 = time.Now()
	result.Issues = append(result.Issues, checkPolicy(checkCtx.Obj, checkCtx.Kind)...)
	logValidationPhaseDuration(checkCtx, objects.KindPolicy, time.Since(t0))

	// 5. Reference integrity
	logValidationPhaseStart(checkCtx, "reference_integrity")
	t0 = time.Now()
	if shouldCheckReferences(checkCtx.Cmd) && checkCtx.ObjectIDCache != nil {
		// Reuse checkCtx.StorageProvider when set (async check); else checkReferencesWithCache gets one per call
		result.Issues = append(result.Issues, checkReferencesWithCache(checkCtx.Ctx, checkCtx.Obj, checkCtx.Kind, checkCtx.ObjectIDCache, checkCtx.StorageProvider)...)
	}
	logValidationPhaseDuration(checkCtx, "reference_integrity", time.Since(t0))

	// 6. File integrity
	logValidationPhaseStart(checkCtx, "integrity")
	t0 = time.Now()
	finalContent, err := fileutil.ReadFile(checkCtx.FilePath)
	if err == nil {
		checkCtx.Content = finalContent
	}
	integrityIssues, autoFixedFromCheck := checkIntegrityWithRegistryAndContent(checkCtx.Ctx, checkCtx.Obj, checkCtx.FilePath, checkCtx.Kind, checkCtx.Content, checkCtx.Registry, checkCtx.StorageProvider)
	result.Issues = append(result.Issues, integrityIssues...)
	if len(autoFixedFromCheck) > 0 {
		result.AutoFixed = append(result.AutoFixed, autoFixedFromCheck...)
	}
	logValidationPhaseDuration(checkCtx, "integrity", time.Since(t0))

	// 7. Auto-fix
	logValidationPhaseStart(checkCtx, "auto_fix")
	t0 = time.Now()
	if shouldAutoFix(checkCtx.Cmd) {
		// For individual object checks, always run synchronous auto-fix
		// This ensures fixes are applied immediately and can be verified
		// NOTE: In async mode, this runs in validation goroutines, so fixes are applied during validation
		autoFixedFromIssues := autoFixIssues(checkCtx.Ctx, checkCtx.Cmd, checkCtx.Obj, checkCtx.FilePath, checkCtx.Kind, result.Issues, checkCtx.Registry, checkCtx.HashRegistryCache, checkCtx.ObjectIDCache, checkCtx.StorageProvider)
		result.AutoFixed = append(result.AutoFixed, autoFixedFromIssues...)

		// Emit auto-fix event via coordinator (especially important in async mode)
		if len(autoFixedFromIssues) > 0 {
			projectRoot := ProjectRootOrResolve(checkCtx.Ctx.ProjectRoot)
			// Reuse checkCtx.StorageProvider when set (async path)
			operationID := fmt.Sprintf("auto_fix_%s_%d", checkCtx.Obj.ID, time.Now().UnixNano())
			emitAutoFixAppliedViaCoordinator(
				checkCtx.StdCtx, projectRoot, checkCtx.StorageProvider, operationID,
				checkCtx.Obj.ID, checkCtx.Kind, autoFixedFromIssues, checkCtx.Ctx.Profile,
			)
		}

		if len(autoFixedFromIssues) > 0 && checkCtx.DeferredChecks != nil {
			handleDeferredHashCheckForObject(checkCtx)
		}
	}
	logValidationPhaseDuration(checkCtx, "auto_fix", time.Since(t0))

	return result
}

// shouldCheckReferences determines if reference checking should be performed
func shouldCheckReferences(cmd *cobra.Command) bool {
	if cmd == nil {
		return true // Default to checking refs when no command (e.g. background validation)
	}
	fastMode, _ := cmd.Flags().GetBool("fast")
	checkRefs, _ := cmd.Flags().GetBool("check-refs")
	return checkRefs && !fastMode
}

// shouldAutoFix determines if auto-fix should be performed
func shouldAutoFix(cmd *cobra.Command) bool {
	if cmd == nil {
		return false // Never auto-fix when no command (e.g. background validation)
	}
	autoFix, _ := cmd.Flags().GetBool("auto-fix")
	force, _ := cmd.Flags().GetBool("force")

	return autoFix || force
}

// handleDeferredHashCheckForObject handles deferred hash checks after auto-fix for a single object
func handleDeferredHashCheckForObject(checkCtx *CheckObjectContext) {
	updatedContent, err := fileutil.ReadFile(checkCtx.FilePath)
	if err == nil {
		checkCtx.Content = updatedContent
	}
	*checkCtx.DeferredChecks = append(*checkCtx.DeferredChecks, deferredHashCheck{
		obj:      checkCtx.Obj,
		filePath: checkCtx.FilePath,
		kind:     checkCtx.Kind,
		content:  checkCtx.Content,
		registry: checkCtx.Registry,
	})
}
