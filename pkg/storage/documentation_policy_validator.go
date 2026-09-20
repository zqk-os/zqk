package storage

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/zqk-os/zqk/pkg/appledouble"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/paths"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

// DocumentationPolicyViolation represents a violation of POL-DOC-001
type DocumentationPolicyViolation struct {
	FilePath    string
	Violation   string
	Severity    string // "error", "warning"
	Message     string
	Recommended string // Recommended action
}

// ValidateDocumentationPolicy validates markdown files against POL-DOC-001
func ValidateDocumentationPolicy(projectRoot string) ([]DocumentationPolicyViolation, error) {
	var violations []DocumentationPolicyViolation

	// Allowed locations for documentation
	allowedPrefixes := []string{
		filepath.Join(projectRoot, paths.DocsDir),
		filepath.Join(projectRoot, paths.ProjectDataDir, paths.MCPDir, paths.MCPLogsDir), // Temporary logs (should be moved)
	}

	// Code directories that should only have README.md
	codeDirs := []string{
		filepath.Join(projectRoot, paths.PkgDir),
		filepath.Join(projectRoot, paths.CmdDir),
		filepath.Join(projectRoot, paths.InternalDir),
		filepath.Join(projectRoot, paths.ToolsDir),
		filepath.Join(projectRoot, paths.ScriptsDir),
	}

	// Exclusions (files/directories to skip)
	exclusions := []string{
		"node_modules",
		".git",
		"vendor",
		"testdata",
		paths.ProjectDataDir,
	}

	logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))

	// Walk through all markdown files
	err := filepath.Walk(projectRoot, func(path string, info fileutil.FileInfo, err error) error {
		if err != nil {
			return nil // Skip files we can't access
		}

		// Skip directories
		if info.IsDir() {
			return nil
		}

		if appledouble.SkipPathInTreeWalk(path) {
			return nil
		}

		// Only check markdown files
		if !strings.HasSuffix(path, paths.MarkdownExtension) && !strings.HasSuffix(path, paths.MarkdownAltExtension) {
			return nil
		}

		// Skip excluded paths
		relPath, err := filepath.Rel(projectRoot, path)
		if err != nil {
			return nil
		}

		for _, exclusion := range exclusions {
			if strings.Contains(relPath, exclusion) {
				return nil
			}
		}

		// Check if file is in allowed location
		inAllowedLocation := false
		for _, prefix := range allowedPrefixes {
			if strings.HasPrefix(path, prefix) {
				inAllowedLocation = true
				break
			}
		}

		// Check if file is in code directory
		inCodeDir := false
		for _, codeDir := range codeDirs {
			if strings.HasPrefix(path, codeDir) {
				inCodeDir = true
				break
			}
		}

		// Rule 1: Documentation must be in docs/ tree (except code READMEs)
		if !inAllowedLocation && !inCodeDir {
			violations = append(violations, DocumentationPolicyViolation{
				FilePath:    relPath,
				Violation:   ConstMiscDocumentationNotInDocsTree,
				Severity:    "error",
				Message:     fmt.Sprintf("Documentation file is not in docs/ tree: %s", relPath),
				Recommended: "Move to " + paths.ProcessArchitectureDir + "/ or appropriate docs/ subdirectory",
			})
			return nil
		}

		// Rule 2: Code directories should only have README.md (index files)
		if inCodeDir {
			filename := filepath.Base(path)
			if filename != "README.md" && filename != "readme.md" {
				violations = append(violations, DocumentationPolicyViolation{
					FilePath:    relPath,
					Violation:   ConstMiscDetailedDocsInCodeDirectory,
					Severity:    "warning",
					Message:     fmt.Sprintf(ConstMiscDetailedDocumentationInCodeDirectorySOnl, relPath),
					Recommended: ConstMiscMoveDetailedDocumentationTo + paths.ProcessArchitectureDir + "/ and keep only README.md in code directory",
				})
			} else {
				// Check if README is too detailed (heuristic: > 200 lines or contains detailed sections)
				if err := checkReadmeIsIndexOnly(path); err != nil {
					violations = append(violations, DocumentationPolicyViolation{
						FilePath:    relPath,
						Violation:   ConstMiscReadmeTooDetailed,
						Severity:    "warning",
						Message:     fmt.Sprintf(ConstMiscReadmeMdAppearsToContainDetailedDocument, err.Error()),
						Recommended: "Move detailed content to docs/ tree, keep README as index with links",
					})
				}
			}
		}

		// Rule 3: Check if markdown file is registered as doc_entry
		// This is checked separately via docman-sync, but we can warn about it here
		if inAllowedLocation && !strings.Contains(relPath, "_internal") {
			// Check if file should be registered (not in _internal or other system dirs)
			if !strings.Contains(relPath, "doc_entries") {
				// This is informational - actual registration check is done by docman-sync
				// We'll just note it for now
			}
		}

		return nil
	})

	if err != nil {
		StorageLog(logger).Warn(LogEventStorageDocumentationPolicyWalkDirFailed).WithError(err).Log()
	}

	return violations, nil
}

// ValidateDocumentationPolicyForFile validates a single file against POL-DOC-001
// This is optimized for use in system check where we're already iterating files
func ValidateDocumentationPolicyForFile(projectRoot, filePath string) ([]DocumentationPolicyViolation, error) {
	var violations []DocumentationPolicyViolation

	// Only check markdown files
	if !strings.HasSuffix(filePath, ".md") && !strings.HasSuffix(filePath, ".markdown") {
		return violations, nil
	}

	relPath, err := filepath.Rel(projectRoot, filePath)
	if err != nil {
		return violations, nil
	}

	// Exclusions
	exclusions := []string{
		"node_modules",
		".git",
		"vendor",
		"testdata",
	}

	for _, exclusion := range exclusions {
		if strings.Contains(relPath, exclusion) {
			return violations, nil
		}
	}

	// Allowed locations
	allowedPrefixes := []string{
		paths.DocsDir,
	}

	// Code directories that should only have README.md
	codeDirs := []string{
		paths.PkgDir,
		paths.CmdDir,
		paths.InternalDir,
		paths.ToolsDir,
		paths.ScriptsDir,
	}

	// Check if file is in allowed location
	inAllowedLocation := false
	for _, prefix := range allowedPrefixes {
		if strings.HasPrefix(relPath, prefix+"/") || strings.HasPrefix(relPath, prefix+"\\") {
			inAllowedLocation = true
			break
		}
	}

	// Check if file is in code directory
	inCodeDir := false
	codeDirPath := ""
	for _, codeDir := range codeDirs {
		if strings.HasPrefix(relPath, codeDir+"/") || strings.HasPrefix(relPath, codeDir+"\\") {
			inCodeDir = true
			codeDirPath = codeDir
			break
		}
	}

	// Rule 1: Documentation must be in docs/ tree (except code READMEs)
	if !inAllowedLocation && !inCodeDir {
		violations = append(violations, DocumentationPolicyViolation{
			FilePath:    relPath,
			Violation:   ConstMiscDocumentationNotInDocsTree,
			Severity:    "error",
			Message:     fmt.Sprintf("Documentation file is not in docs/ tree: %s", relPath),
			Recommended: "Move to " + paths.ProcessArchitectureDir + "/ or appropriate docs/ subdirectory",
		})
		return violations, nil
	}

	// Rule 2: Code directories should only have README.md (index files)
	if inCodeDir {
		filename := filepath.Base(filePath)
		if filename != "README.md" && filename != "readme.md" {
			violations = append(violations, DocumentationPolicyViolation{
				FilePath:    relPath,
				Violation:   ConstMiscDetailedDocsInCodeDirectory,
				Severity:    "warning",
				Message:     fmt.Sprintf(ConstMiscDetailedDocumentationInCodeDirectorySSOn, codeDirPath, relPath),
				Recommended: ConstMiscMoveDetailedDocumentationTo + paths.ProcessArchitectureDir + "/ and keep only README.md in code directory",
			})
		} else {
			// Check if README is too detailed
			if err := checkReadmeIsIndexOnly(filePath); err != nil {
				violations = append(violations, DocumentationPolicyViolation{
					FilePath:    relPath,
					Violation:   ConstMiscReadmeTooDetailed,
					Severity:    "warning",
					Message:     fmt.Sprintf(ConstMiscReadmeMdAppearsToContainDetailedDocument, err.Error()),
					Recommended: "Move detailed content to docs/ tree, keep README as index with links",
				})
			}
		}
	}

	return violations, nil
}

// checkReadmeIsIndexOnly checks if a README is appropriately brief (index only)
func checkReadmeIsIndexOnly(filePath string) error {
	data, err := fileutil.ReadFile(filePath)
	if err != nil {
		return nil // Can't read, skip check
	}

	content := string(data)
	lines := strings.Split(content, "\n")

	// Heuristic: If README is > 200 lines, it's probably too detailed
	if len(lines) > 200 {
		return errfmt.Errorf(ConstMiscReadmeIsDLinesShouldBeBriefIndex200Lines, len(lines))
	}

	// Check for detailed documentation indicators
	detailedIndicators := []string{
		ConstMiscArchitecture,
		"## Design",
		ConstMiscImplementation,
		"## Detailed",
		"## Process",
		"## Workflow",
	}

	hasDetailedContent := false
	for _, indicator := range detailedIndicators {
		if strings.Contains(content, indicator) {
			hasDetailedContent = true
			break
		}
	}

	// Check if it has links to docs/ tree (good sign it's an index)
	hasDocLinks := strings.Contains(content, paths.ProcessDir+"/") || strings.Contains(content, paths.OnboardingDir+"/")

	if hasDetailedContent && !hasDocLinks {
		return errfmt.Errorf("README contains detailed sections but no links to docs/ tree")
	}

	return nil
}

// FindUnregisteredDocumentation finds markdown files that should be registered as doc_entry objects
// This requires checking against existing doc_entry objects
func FindUnregisteredDocumentation(projectRoot string, existingDocEntries map[string]bool) ([]string, error) {
	var unregistered []string

	allowedPrefixes := []string{
		filepath.Join(projectRoot, paths.ProcessArchitectureDir),
		filepath.Join(projectRoot, paths.ProcessPoliciesDir),
		filepath.Join(projectRoot, paths.OnboardingDir),
		filepath.Join(projectRoot, paths.ProcessPlanningDir),
	}

	exclusions := []string{
		"_internal",
		"doc_entries",
		"node_modules",
		".git",
	}

	err := filepath.Walk(projectRoot, func(path string, info fileutil.FileInfo, err error) error {
		if err != nil {
			return nil
		}

		if info.IsDir() {
			return nil
		}

		if appledouble.SkipPathInTreeWalk(path) {
			return nil
		}

		if !strings.HasSuffix(path, ".md") && !strings.HasSuffix(path, ".markdown") {
			return nil
		}

		relPath, err := filepath.Rel(projectRoot, path)
		if err != nil {
			return nil
		}

		// Skip excluded paths
		for _, exclusion := range exclusions {
			if strings.Contains(relPath, exclusion) {
				return nil
			}
		}

		// Check if in allowed location
		inAllowedLocation := false
		for _, prefix := range allowedPrefixes {
			if strings.HasPrefix(path, prefix) {
				inAllowedLocation = true
				break
			}
		}

		if !inAllowedLocation {
			return nil
		}

		// Check if registered
		if !existingDocEntries[relPath] {
			unregistered = append(unregistered, relPath)
		}

		return nil
	})

	return unregistered, err
}
