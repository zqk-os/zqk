package system

import (
	stdcontext "context"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/zqk-os/zqk/internal/cli"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/migration/parser"
	"github.com/zqk-os/zqk/pkg/migration/scanner"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/storage"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
	"github.com/zqk-os/zqk/pkg/validation"
)

// CheckKindContext groups state for checking kind objects
type CheckKindContext struct {
	Ctx               *cli.Context
	StdCtx            stdcontext.Context // Standard context.Context for cancellation/timeouts
	Cmd               *cobra.Command
	Kind              string
	IDs               []string
	SpecLoader        *objects.SpecLoader
	LifecycleLoader   *objects.LifecycleLoader
	Validator         validation.Validator
	HashRegistryCache *HashRegistryCacheType
	ObjectIDCache     *ObjectIDCache
	ProjectRoot       string
	KindDir           string
	YAMLParser        *parser.YAMLParser
	Logger            logging.Logger
}

// initializeCheckKindContext sets up the check kind context
func initializeCheckKindContext(ctx *cli.Context, stdCtx stdcontext.Context, cmd *cobra.Command, kind string, ids []string, specLoader *objects.SpecLoader, lifecycleLoader *objects.LifecycleLoader, validator validation.Validator, hashRegistryCache *HashRegistryCacheType, objectIDCache *ObjectIDCache) (*CheckKindContext, error) {
	projectRoot := ctx.ProjectRoot
	projectRoot = ProjectRootOrResolve(projectRoot)

	kindDir := getKindDirectory(projectRoot, kind)
	if kindDir == emptyValue {
		return nil, errfmt.Errorf("unknown object kind: %s", kind)
	}

	// Use provided context or create system context if not provided
	if stdCtx == nil {
		stdCtx = pkgctx.NewSystemContext()
	}

	return &CheckKindContext{
		Ctx:               ctx,
		StdCtx:            stdCtx,
		Cmd:               cmd,
		Kind:              kind,
		IDs:               ids,
		SpecLoader:        specLoader,
		LifecycleLoader:   lifecycleLoader,
		Validator:         validator,
		HashRegistryCache: hashRegistryCache,
		ObjectIDCache:     objectIDCache,
		ProjectRoot:       projectRoot,
		KindDir:           kindDir,
		YAMLParser:        parser.NewYAMLParser(),
		Logger:            logging.GetLoggerFromProfile(ctx.Profile),
	}, nil
}

// scanKindFiles scans for YAML files in the kind directory
func scanKindFiles(checkCtx *CheckKindContext) ([]*scanner.YAMLFile, error) {
	yamlScanner := scanner.NewYAMLScanner(checkCtx.KindDir)
	files, err := yamlScanner.Scan()
	if err != nil {
		return nil, errfmt.Errorf("failed to scan %s: %w", checkCtx.KindDir, err)
	}
	return files, nil
}

// shouldProcessFileForCheck determines if a file should be processed based on ID filtering
func shouldProcessFileForCheck(file *scanner.YAMLFile, ids []string, logger logging.Logger) bool {
	if len(ids) == 0 {
		return true
	}

	for _, id := range ids {
		if file.ObjectID == id {
			logging.Fluent(logger).Info("Processing file for ID check").
				String("requested_id", id).
				String("file_object_id", file.ObjectID).
				String("file_path", file.Path).
				String("file_basename", filepath.Base(file.Path)).
				Log()
			return true
		}
	}
	return false
}

// readAndParseFile reads and parses a file
func readAndParseFile(checkCtx *CheckKindContext, file *scanner.YAMLFile) ([]byte, *parser.ParsedObject, error) {
	content, err := fileutil.ReadFile(file.Path)
	if err != nil {
		return nil, nil, errfmt.Newf("failed to read file").Wrap(err)
	}

	logDebugParseStart(checkCtx, file, content)

	obj, err := checkCtx.YAMLParser.ParseBytes(content)
	if err != nil {
		return nil, nil, errfmt.Newf("failed to parse YAML").Wrap(err)
	}

	logDebugParseResult(checkCtx, file, obj)

	return content, obj, nil
}

// logDebugParseStart logs debug info at parse start
func logDebugParseStart(checkCtx *CheckKindContext, file *scanner.YAMLFile, content []byte) {
	if file.ObjectID != "POL-DEBUG-001" {
		return
	}
	logging.Fluent(checkCtx.Logger).Info("=== POL-DEBUG-001 DEBUG: INITIAL PARSE IN CheckKindObjectsWithCache ===").
		String("filePath", file.Path).
		Int("content_len", len(content)).
		Log()
}

// logDebugParseResult logs debug info after parse
func logDebugParseResult(checkCtx *CheckKindContext, file *scanner.YAMLFile, obj *parser.ParsedObject) {
	if file.ObjectID != "POL-DEBUG-001" || obj == nil || obj.Properties == nil {
		return
	}
	bodyVal, bodyExists := obj.Properties[objects.FieldKeyBody]
	categoryVal, categoryExists := obj.Properties[objects.FieldKeyCategory]
	policyTypeVal, policyTypeExists := obj.Properties[objects.FieldKeyPolicyType]
	logging.Fluent(checkCtx.Logger).Info("Initial parse result").
		Int("properties_len", len(obj.Properties)).
		String("body_exists", fmt.Sprintf("%v", bodyExists)).
		String("body_type", fmt.Sprintf("%T", bodyVal)).
		String("category_exists", fmt.Sprintf("%v", categoryExists)).
		String("category_value", fmt.Sprintf("%v", categoryVal)).
		String("policy_type_exists", fmt.Sprintf("%v", policyTypeExists)).
		String("policy_type_value", fmt.Sprintf("%v", policyTypeVal)).
		Log()
}

// determineActualKind determines the actual kind to use for validation.
// Kind-from-ID is source of truth: use ID-derived kind first, then object's declared kind, then discovery kind.
func determineActualKind(checkCtx *CheckKindContext, obj *parser.ParsedObject) string {
	if effectiveKind := inferKindFromID(obj.ID); effectiveKind != emptyValue {
		return effectiveKind
	}
	if obj.Kind != emptyValue {
		return obj.Kind
	}
	return checkCtx.Kind
}

// setupRegistryForFile sets up the hash registry for a file
func setupRegistryForFile(checkCtx *CheckKindContext, filePath string, actualKind string) storage.HashRegistryProvider {
	fileDir := filepath.Dir(filePath)
	isBucketed := fileDir != checkCtx.KindDir

	actualRegistry := getHashRegistryForFile(checkCtx.Cmd.Context(), filePath, actualKind, checkCtx.ProjectRoot, checkCtx.HashRegistryCache)

	if err := storage.ValidateRegistryLocation(fileDir, filePath, actualKind, isBucketed); err != nil {
		logging.Fluent(checkCtx.Logger).Warn("Registry location validation failed").
			Kind(actualKind).
			File(filePath).
			WithError(err).
			Log()
	}

	return actualRegistry
}

// createDeferredChecksPtr creates a pointer to deferred checks if in fix mode
func createDeferredChecksPtr(checkCtx *CheckKindContext) *[]deferredHashCheck {
	autoFix, _ := checkCtx.Cmd.Flags().GetBool("auto-fix")
	force, _ := checkCtx.Cmd.Flags().GetBool("force")
	if autoFix || force {
		deferredChecks := []deferredHashCheck{}
		return &deferredChecks
	}
	return nil
}

// logDebugObjectCheck logs debug info for specific object IDs
func logDebugObjectCheck(checkCtx *CheckKindContext, file *scanner.YAMLFile, obj *parser.ParsedObject) {
	if len(checkCtx.IDs) == 0 {
		return
	}
	if !isDebugValidationObject(checkCtx.IDs[0]) {
		return
	}
	logging.Fluent(checkCtx.Logger).Info("About to check object").
		String("requested_id", checkCtx.IDs[0]).
		String("file_object_id", file.ObjectID).
		String("file_path", file.Path).
		String("file_basename", filepath.Base(file.Path)).
		String("obj_id", obj.ID).
		Log()
}

// processFileForCheck processes a single file for checking
func processFileForCheck(checkCtx *CheckKindContext, file *scanner.YAMLFile) (CheckResult, *[]deferredHashCheck, error) {
	content, obj, err := readAndParseFile(checkCtx, file)
	if err != nil {
		return createErrorResult(file, checkCtx.Kind, err), nil, nil
	}

	actualKind := determineActualKind(checkCtx, obj)
	actualRegistry := setupRegistryForFile(checkCtx, file.Path, actualKind)
	deferredChecksPtr := createDeferredChecksPtr(checkCtx)

	logDebugObjectCheck(checkCtx, file, obj)

	var sharedStorage storage.ObjectStorageProvider
	if checkCtx.Cmd != nil && checkCtx.Cmd.Context() != nil {
		if p := cli.GetStorageProvider(checkCtx.Cmd.Context()); p != nil {
			if sp, ok := p.(storage.ObjectStorageProvider); ok {
				sharedStorage = sp
			}
		}
	}

	result := checkObjectWithCacheAndContent(
		checkCtx.Ctx,
		checkCtx.StdCtx,
		checkCtx.Cmd,
		obj,
		file.Path,
		actualKind,
		content,
		checkCtx.SpecLoader,
		checkCtx.LifecycleLoader,
		checkCtx.Validator,
		actualRegistry,
		checkCtx.ObjectIDCache,
		checkCtx.HashRegistryCache,
		deferredChecksPtr,
		sharedStorage,
	)

	if checkCtx.ObjectIDCache != nil {
		duplicateIssues := checkDuplicateIDs(checkCtx.Ctx, file.ObjectID, actualKind, file.Path, checkCtx.ObjectIDCache)
		result.Issues = append(result.Issues, duplicateIssues...)
	}

	return result, deferredChecksPtr, nil
}

// createErrorResult creates an error result for a file.
// For YAML parse errors, message includes "Failed to parse YAML" so tests and UI can detect parse failures.
func createErrorResult(file *scanner.YAMLFile, kind string, err error) CheckResult {
	msg := fmt.Sprintf("Failed to process file: %v", err)
	if err != nil && (strings.Contains(strings.ToLower(err.Error()), "parse") && strings.Contains(strings.ToLower(err.Error()), "yaml")) {
		msg = fmt.Sprintf("Failed to parse YAML: %v", err)
	}
	return CheckResult{
		ObjectID:   file.ObjectID,
		ObjectKind: kind,
		FilePath:   file.Path,
		Issues: []Issue{
			{
				Tier:     1,
				Category: "registration",
				Message:  msg,
			},
		},
	}
}
