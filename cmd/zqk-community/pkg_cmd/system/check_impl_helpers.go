package system

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/datacell"
	"github.com/lanceman/zqk/pkg/errfmt"
	"github.com/lanceman/zqk/pkg/goroutinelabels"
	"github.com/lanceman/zqk/pkg/logging"
	"github.com/lanceman/zqk/pkg/migration/scanner"
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/storage"
	"github.com/lanceman/zqk/pkg/validation"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

// resolveBacklogItemPathByScan scans kindDir for a YAML file whose id field matches objectID.
// Used by tests when CAS index may not be flushed yet after Create. Returns the file path or error.
func resolveBacklogItemPathByScan(kindDir, objectID string) (string, error) {
	scnr := scanner.NewYAMLScanner(kindDir)
	files, err := scnr.Scan()
	if err != nil {
		return "", errfmt.Errorf("scan %s: %w", kindDir, err)
	}
	for _, f := range files {
		if f.ObjectID == objectID {
			return f.Path, nil
		}
	}
	return "", errfmt.Errorf("object %s not found in %s", objectID, kindDir)
}

func logInitialDebugState(checkCtx *CheckObjectContext) {
	logging.Fluent(checkCtx.Logger).Info("=== POLICY-DEBUG-001 DEBUG: BEFORE RE-PARSE IN checkObjectWithCacheAndContent ===").
		String("filePath", checkCtx.FilePath).
		Int("content_len", len(checkCtx.Content)).
		Int("obj_properties_len", len(checkCtx.Obj.Properties)).
		Log()
	if checkCtx.Obj.Properties != nil {
		bodyVal, bodyExists := checkCtx.Obj.Properties[objects.FieldKeyBody]
		categoryVal, categoryExists := checkCtx.Obj.Properties[objects.FieldKeyCategory]
		policyTypeVal, policyTypeExists := checkCtx.Obj.Properties[objects.FieldKeyPolicyType]
		logging.Fluent(checkCtx.Logger).Info("Initial obj.Properties state").
			String("body_exists", fmt.Sprintf("%v", bodyExists)).
			String("body_type", fmt.Sprintf("%T", bodyVal)).
			String("category_exists", fmt.Sprintf("%v", categoryExists)).
			String("category_value", fmt.Sprintf("%v", categoryVal)).
			String("policy_type_exists", fmt.Sprintf("%v", policyTypeExists)).
			String("policy_type_value", fmt.Sprintf("%v", policyTypeVal)).
			Log()
	}
}

// logFinalDebugState logs final debug state
func logFinalDebugState(checkCtx *CheckObjectContext, objMap map[string]any) {
	logging.Fluent(checkCtx.Logger).Info("=== POLICY-DEBUG-001 DEBUG: FINAL objMap BEFORE VALIDATION ===").
		Int("objMap_len", len(objMap)).
		Log()
	if len(objMap) > 0 {
		bodyVal, bodyExists := objMap[objects.FieldKeyBody]
		categoryVal, categoryExists := objMap[objects.FieldKeyCategory]
		policyTypeVal, policyTypeExists := objMap[objects.FieldKeyPolicyType]
		bodyStr := fmt.Sprintf("%v", bodyVal)
		if len(bodyStr) > 100 {
			bodyStr = bodyStr[:100] + "..."
		}
		logging.Fluent(checkCtx.Logger).Info("Final objMap state").
			String("body_exists", fmt.Sprintf("%v", bodyExists)).
			String("body_type", fmt.Sprintf("%T", bodyVal)).
			String("body_preview", bodyStr).
			String("category_exists", fmt.Sprintf("%v", categoryExists)).
			String("category_value", fmt.Sprintf("%v", categoryVal)).
			String("policy_type_exists", fmt.Sprintf("%v", policyTypeExists)).
			String("policy_type_value", fmt.Sprintf("%v", policyTypeVal)).
			Log()
	}
}
func discoverObjectKinds(processDir string) []string {
	var kinds []string

	// Use field registry to get all kinds (follows architecture pattern)
	// This works for both file and graph backends
	fieldRegistry := objects.GetGlobalFieldRegistry()
	if err := fieldRegistry.LoadFields(); err != nil {
		// If field registry fails, return empty (best effort)
		return kinds
	}

	allKinds, err := fieldRegistry.GetAllKinds()
	if err != nil {
		return kinds
	}
	kinds = allKinds

	internalKinds := map[string]bool{
		objects.KindBaseMetric: true, // internal; should never be instantiated (except built-in)
	}

	// Filter out internal kinds that should never be instantiated, and high-volume kinds.
	// Also filter out kinds that do not have a physical directory on disk, ensuring consistency
	// between cache-driven and storage-driven scans.
	filteredKinds := make([]string, 0, len(kinds))
	for _, kind := range kinds {
		if internalKinds[kind] || storage.IsHighVolumeKindForCache(kind) {
			continue
		}
		dirName := objects.GetDirectoryFromKind(kind)
		if dirName != "" {
			dirPath := filepath.Join(processDir, dirName)
			if _, err := os.Stat(dirPath); os.IsNotExist(err) {
				continue
			}
		}
		filteredKinds = append(filteredKinds, kind)
	}

	return filteredKinds
}

// DiscoverObjectKindsWithContext runs discoverObjectKinds with optional context timeout so callers
// (e.g. BuildCache, async check) never block indefinitely. Uses existing loader/shared state
// (FieldRegistry) but bounds the call. Returns nil on timeout or context cancel so callers can fall back.
func DiscoverObjectKindsWithContext(ctx context.Context, processDir string) []string {
	const defaultDiscoverTimeout = 30 * time.Second
	timeout := defaultDiscoverTimeout
	if ctx != nil {
		if deadline, ok := ctx.Deadline(); ok {
			if d := time.Until(deadline); d > 0 && d < timeout {
				timeout = d
			}
		}
	}
	type result struct{ kinds []string }
	done := make(chan result, 1)
	goroutinelabels.NewGoroutine("refactor", "refactored").StartSimple(func() {
		func() {
			done <- result{kinds: discoverObjectKinds(processDir)}
		}()
	})
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	if ctx != nil {
		select {
		case r := <-done:
			return r.kinds
		case <-timer.C:
			return nil
		case <-ctx.Done():
			return nil
		}
	}
	select {
	case r := <-done:
		return r.kinds
	case <-timer.C:
		return nil
	}
}

func directoryToKind(dirName string) string {
	return objects.GetKindFromDirectory(dirName)
}
func getKindDirectory(projectRoot, kind string) string {
	dirName := objects.GetDirectoryFromKind(kind)
	if dirName == emptyValue {
		return ""
	}

	return datacell.CellCASPrimaryDir(projectRoot, dirName)
}
func inferKindFromID(id string) string {
	// Use configurable validator
	idValidator := validation.GetIDValidator()
	//nolint:errcheck // Load if not already loaded - error is non-critical
	_ = idValidator.LoadPatterns() // Load if not already loaded
	return idValidator.InferKindFromID(id)
}
func findObjectByID(projectRoot, id string) (filePath, kind string) {
	// Prefer storage path resolution (uses spec bucketing and CAS index).
	if kind := inferKindFromID(id); kind != emptyValue {
		ctx := pkgctx.NewSystemContext()
		factory, err := storage.NewStorageFactory(ctx, projectRoot)
		if err == nil {
			sp := factory.GetStorage()
			if f, ok := sp.(*storage.FileObjectStorage); ok {
				if path, err := f.GetFilePathForObject(id, kind); err == nil {
					return path, kind
				}
			}
		}
	}
	// Fallback: scan all object directories (slower, no strategy).
	processDir := datacell.ProcessPrimaryDir(projectRoot)
	kinds := discoverObjectKinds(processDir)
	for _, kind := range kinds {
		kindDir := getKindDirectory(projectRoot, kind)
		if kindDir == emptyValue {
			continue
		}
		scnr := scanner.NewYAMLScanner(kindDir)
		files, err := scnr.Scan()
		if err != nil {
			continue
		}
		for _, file := range files {
			if file.ObjectID == id {
				return file.Path, kind
			}
		}
	}
	return "", ""
}
func validateIDFormat(id, kind string) bool {
	idValidator := validation.GetIDValidator()
	//nolint:errcheck // Load if not already loaded - error is non-critical
	_ = idValidator.LoadPatterns() // Load if not already loaded
	valid, err := idValidator.ValidateID(id, kind)
	if err != nil {
		return false
	}
	return valid
}
func filterResultsByTier(results []CheckResult, tier int) []CheckResult {
	var filtered []CheckResult
	for _, result := range results {
		// Check if this result has any issues of the specified tier
		hasTierIssue := false
		for _, issue := range result.Issues {
			if issue.Tier == tier {
				hasTierIssue = true
				break
			}
		}
		if hasTierIssue {
			// Include the result, but filter issues to only show the specified tier
			filteredResult := result
			filteredResult.Issues = []Issue{}
			for _, issue := range result.Issues {
				if issue.Tier == tier {
					filteredResult.Issues = append(filteredResult.Issues, issue)
				}
			}
			filtered = append(filtered, filteredResult)
		}
	}
	return filtered
}
func prepareOutputData(results []CheckResult, bufferCount int, bufferSummary map[string]int) any {
	// Calculate summary statistics
	summary := struct {
		TotalObjects          int            `json:"total_objects"`
		PublicObjects         int            `json:"public_objects"`
		InternalObjects       int            `json:"internal_objects"`
		TotalIssues           int            `json:"total_issues"`
		BlockingIssues        int            `json:"blocking_issues"`
		PublicBlocking        int            `json:"public_blocking"`
		InternalBlocking      int            `json:"internal_blocking"`
		Warnings              int            `json:"warnings"`
		PublicWarnings        int            `json:"public_warnings"`
		InternalWarnings      int            `json:"internal_warnings"`
		Informational         int            `json:"informational"`
		PublicInformational   int            `json:"public_informational"`
		InternalInformational int            `json:"internal_informational"`
		Recommendations       int            `json:"recommendations"`
		AutoFixed             int            `json:"auto_fixed"`
		AuditEventsBuffered   int            `json:"audit_events_buffered,omitempty"`
		AuditEventsSummary    map[string]int `json:"audit_events_summary,omitempty"`
	}{}

	summary.TotalObjects = len(results)
	for _, result := range results {
		isInternal := isInternalKind(result.ObjectKind)
		if isInternal {
			summary.InternalObjects++
		} else {
			summary.PublicObjects++
		}

		summary.AutoFixed += len(result.AutoFixed)
		for _, issue := range result.Issues {
			summary.TotalIssues++
			switch issue.Tier {
			case 1:
				summary.BlockingIssues++
				if isInternal {
					summary.InternalBlocking++
				} else {
					summary.PublicBlocking++
				}
			case 2:
				summary.Warnings++
				if isInternal {
					summary.InternalWarnings++
				} else {
					summary.PublicWarnings++
				}
			case 3:
				summary.Informational++
				if isInternal {
					summary.InternalInformational++
				} else {
					summary.PublicInformational++
				}
			case 4:
				summary.Recommendations++
			}
		}
	}

	if bufferCount > 0 {
		summary.AuditEventsBuffered = bufferCount
		summary.AuditEventsSummary = bufferSummary
	}

	return struct {
		Summary struct {
			TotalObjects          int            `json:"total_objects"`
			PublicObjects         int            `json:"public_objects"`
			InternalObjects       int            `json:"internal_objects"`
			TotalIssues           int            `json:"total_issues"`
			BlockingIssues        int            `json:"blocking_issues"`
			PublicBlocking        int            `json:"public_blocking"`
			InternalBlocking      int            `json:"internal_blocking"`
			Warnings              int            `json:"warnings"`
			PublicWarnings        int            `json:"public_warnings"`
			InternalWarnings      int            `json:"internal_warnings"`
			Informational         int            `json:"informational"`
			PublicInformational   int            `json:"public_informational"`
			InternalInformational int            `json:"internal_informational"`
			Recommendations       int            `json:"recommendations"`
			AutoFixed             int            `json:"auto_fixed"`
			AuditEventsBuffered   int            `json:"audit_events_buffered,omitempty"`
			AuditEventsSummary    map[string]int `json:"audit_events_summary,omitempty"`
		} `json:"summary"`
		Results []CheckResult `json:"results"`
		Message string        `json:"message,omitempty"`
	}{
		Summary: summary,
		Results: results,
		Message: func() string {
			if bufferCount > 0 {
				return fmt.Sprintf("Note: %d audit event(s) are being buffered and will be written/flushed after this operation completes. Run the command again to see final audit event counts.", bufferCount)
			}
			return ""
		}(),
	}
}
func wrapText(text string, width int, indent string) string {
	if width <= 0 {
		width = 80 // Default to 80 characters
	}

	// Try to detect terminal width if available
	if width == 80 {
		if w, _, err := term.GetSize(0); err == nil && w > 0 {
			width = w
		}
	}

	// Account for indent in width calculation
	effectiveWidth := width - len(indent)
	if effectiveWidth < 20 {
		effectiveWidth = 20 // Minimum width
	}

	words := strings.Fields(text)
	if len(words) == 0 {
		return ""
	}

	var lines []string
	currentLine := words[0]

	for _, word := range words[1:] {
		// Check if adding this word would exceed the width
		if len(currentLine)+1+len(word) > effectiveWidth {
			lines = append(lines, indent+currentLine)
			currentLine = word
		} else {
			currentLine += " " + word
		}
	}

	// Add the last line
	if currentLine != emptyValue {
		lines = append(lines, indent+currentLine)
	}

	return strings.Join(lines, "\n")
}
func isHashMismatchFixMode(cmd *cobra.Command) bool {
	//nolint:errcheck // Flag get - error indicates flag not set, default used
	autoFix, _ := cmd.Flags().GetBool("auto-fix")
	//nolint:errcheck // Flag get - error indicates flag not set, default used
	force, _ := cmd.Flags().GetBool("force")
	return autoFix || force
}
func isInternalKind(kind string) bool {
	// Check known internal kinds first (fast path)
	knownInternalKinds := map[string]bool{
		objects.KindAuditEvent:             true,
		objects.KindChangeJournalEntry:     true,
		objects.KindLifecycle:              true,
		objects.KindObjectSpec:             true,
		objects.KindTemplate:               true,
		objects.KindIntegrityManifest:      true,
		objects.KindSynonym:                true,
		objects.KindBaseMetric:             true,
		objects.KindAuditAggregationMetric: true,
		objects.KindCommandMetric:          true,
	}
	if knownInternalKinds[kind] {
		return true
	}

	// Load spec to check visibility (use global loader to share cache)
	specLoader := objects.GetGlobalSpecLoader()
	specFile := kind + ".yaml"
	spec, err := specLoader.LoadSpecWithInheritance(specFile)
	if err != nil {
		// If spec can't be loaded, assume it's not internal (safer default)
		return false
	}

	// Check visibility from spec - only the spec's own visibility matters
	// Visibility is NOT inherited from parent specs
	if spec.Visibility == "internal" {
		return true
	}

	return false
}
