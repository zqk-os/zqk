package system

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/lanceman/zqk/internal/cli"
	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/migration/parser"
	"github.com/lanceman/zqk/pkg/storage"
	"github.com/spf13/cobra"

	"github.com/lanceman/zqk/pkg/objects"
)

// CheckObjectLegacyContext groups state for legacy checkObject
type CheckObjectLegacyContext struct {
	Ctx               *cli.Context
	Cmd               *cobra.Command
	Obj               *parser.ParsedObject
	FilePath          string
	Kind              string
	HashRegistryCache *HashRegistryCacheType
	ProjectRoot       string
}

// initializeCheckObjectLegacyContext sets up the legacy check object context
func initializeCheckObjectLegacyContext(ctx *cli.Context, cmd *cobra.Command, obj *parser.ParsedObject, filePath, kind string) *CheckObjectLegacyContext {
	tempCache := &HashRegistryCacheType{
		cache: make(map[string]storage.HashRegistryProvider),
	}

	projectRoot := ctx.ProjectRoot
	projectRoot = ProjectRootOrResolve(projectRoot)

	return &CheckObjectLegacyContext{
		Ctx:               ctx,
		Cmd:               cmd,
		Obj:               obj,
		FilePath:          filePath,
		Kind:              kind,
		HashRegistryCache: tempCache,
		ProjectRoot:       projectRoot,
	}
}

// normalizeKindForLegacyCheck normalizes the kind for legacy check
func normalizeKindForLegacyCheck(checkCtx *CheckObjectLegacyContext) string {
	kind := checkCtx.Kind
	if checkCtx.Obj.Kind != emptyValue {
		objKindDir := objects.GetDirectoryFromKind(checkCtx.Obj.Kind)
		expectedKindDir := objects.GetDirectoryFromKind(kind)
		if objKindDir != emptyValue && objKindDir == expectedKindDir {
			kind = checkCtx.Obj.Kind
		}
	}
	return kind
}

// performLegacyChecks performs all checks for legacy checkObject
func performLegacyChecks(checkCtx *CheckObjectLegacyContext) CheckResult {
	result := CheckResult{
		ObjectID:   checkCtx.Obj.ID,
		ObjectKind: checkCtx.Kind,
		FilePath:   checkCtx.FilePath,
		Issues:     []Issue{},
	}

	// 1. Registration validation
	result.Issues = append(result.Issues, checkRegistration(checkCtx.Obj, checkCtx.Kind)...)

	// 1.5. Directory location validation
	if err := storage.ValidateObjectFileLocation(checkCtx.FilePath, checkCtx.Kind); err != nil {
		result.Issues = append(result.Issues, Issue{
			Tier:     2,
			Category: "registration",
			Message:  err.Error(),
		})
	}

	// 2. Instance validation
	instanceIssues := checkInstanceValidation(checkCtx.Ctx, pkgctx.NewSystemContext(), checkCtx.Obj, checkCtx.Kind)
	result.Issues = append(result.Issues, instanceIssues...)

	// 3. Lifecycle validation
	result.Issues = append(result.Issues, checkLifecycle(checkCtx.Obj, checkCtx.Kind)...)

	// 4. Policy compliance
	result.Issues = append(result.Issues, checkPolicy(checkCtx.Obj, checkCtx.Kind)...)

	// 5. Reference integrity
	if shouldCheckReferencesLegacy(checkCtx.Cmd) {
		result.Issues = append(result.Issues, checkReferences(checkCtx.Ctx, checkCtx.Obj, checkCtx.Kind)...)
	}

	// 6. File integrity
	integrityIssues, autoFixed := performLegacyIntegrityCheck(checkCtx)
	result.Issues = append(result.Issues, integrityIssues...)
	if len(autoFixed) > 0 {
		result.AutoFixed = append(result.AutoFixed, autoFixed...)
	}

	// 7. Auto-fix
	if shouldAutoFixLegacy(checkCtx.Cmd) {
		loadedRegistry := getHashRegistryForFile(checkCtx.Cmd.Context(), checkCtx.FilePath, checkCtx.Kind, checkCtx.ProjectRoot, checkCtx.HashRegistryCache)
		autoFixedFromIssues := autoFixIssues(checkCtx.Ctx, checkCtx.Cmd, checkCtx.Obj, checkCtx.FilePath, checkCtx.Kind, result.Issues, loadedRegistry, nil, nil, nil)
		result.AutoFixed = append(result.AutoFixed, autoFixedFromIssues...)
	}

	return result
}

// shouldCheckReferencesLegacy determines if reference checking should be performed
func shouldCheckReferencesLegacy(cmd *cobra.Command) bool {
	if cmd == nil {
		return true
	}
	fastMode, _ := cmd.Flags().GetBool("fast")
	checkRefs, _ := cmd.Flags().GetBool("check-refs")
	return checkRefs && !fastMode
}

// shouldAutoFixLegacy determines if auto-fix should be performed
func shouldAutoFixLegacy(cmd *cobra.Command) bool {
	if cmd == nil {
		return false
	}
	autoFix, _ := cmd.Flags().GetBool("auto-fix")
	force, _ := cmd.Flags().GetBool("force")
	return autoFix || force
}

// performLegacyIntegrityCheck performs integrity check for legacy checkObject
func performLegacyIntegrityCheck(checkCtx *CheckObjectLegacyContext) ([]Issue, []string) {
	finalContent, err := os.ReadFile(checkCtx.FilePath)
	filename := filepath.Base(checkCtx.FilePath)
	isCASFile := len(filename) == 69 && strings.HasSuffix(filename, ".yaml") && isHexString(filename[:64])

	if err == nil {
		loadedRegistry := getHashRegistryForFile(checkCtx.Cmd.Context(), checkCtx.FilePath, checkCtx.Kind, checkCtx.ProjectRoot, checkCtx.HashRegistryCache)
		if isCASFile {
			return checkIntegrityCAS(checkCtx.Ctx, checkCtx.Obj, checkCtx.FilePath, checkCtx.Kind)
		}
		return checkIntegrityWithRegistryAndContent(checkCtx.Ctx, checkCtx.Obj, checkCtx.FilePath, checkCtx.Kind, finalContent, loadedRegistry, nil)
	}

	// Fallback if file read fails
	if isCASFile {
		return checkIntegrityCAS(checkCtx.Ctx, checkCtx.Obj, checkCtx.FilePath, checkCtx.Kind)
	}
	return checkIntegrity(checkCtx.Ctx, checkCtx.Cmd, checkCtx.Obj, checkCtx.FilePath, checkCtx.Kind)
}
