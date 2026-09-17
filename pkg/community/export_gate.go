package community

import (
	"fmt"
	"io/fs"
	"path/filepath"
	"strings"

	"github.com/lanceman/zqk/pkg/utils/fileutil"
)

// Violation represents a prohibited path or content discovered in the community tree.
type Violation struct {
	Path    string `json:"path"`
	Rule    string `json:"rule"`
	Message string `json:"message"`
}

// ExportGateResult encapsulates the audit findings of the open-core export gate (BLI-SCRIPT-PROD-RELEASE-003).
type ExportGateResult struct {
	TargetDirectory string      `json:"target_directory"`
	FilesAudited    int         `json:"files_audited"`
	Violations      []Violation `json:"violations"`
	Passed          bool        `json:"passed"`
}

// DefaultProhibitedPatterns lists substrings and paths strictly forbidden from the open community distribution.
var DefaultProhibitedPatterns = []string{
	"scripts/zqk-internal",
	".gemini",
	".cursor",
	"brain",
	".env.local",
	"private_key",
	"id_rsa",
	"id_ed25519",
}

// DefaultProhibitedExtensions lists file extensions forbidden from public release packages.
var DefaultProhibitedExtensions = []string{
	".pem",
	".key",
	".csnap.bak",
}

// RunExportGate audits targetDir recursively against open-core distribution policies.
func RunExportGate(targetDir string, extraProhibited []string) (*ExportGateResult, error) {
	if targetDir == "" {
		targetDir = "."
	}

	absDir, err := filepath.Abs(targetDir)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve target directory: %w", err)
	}

	info, err := fileutil.Stat(absDir)
	if err != nil {
		return nil, fmt.Errorf("target directory does not exist: %w", err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("target path is not a directory: %s", absDir)
	}

	prohibited := append([]string{}, DefaultProhibitedPatterns...)
	prohibited = append(prohibited, extraProhibited...)

	result := &ExportGateResult{
		TargetDirectory: absDir,
		Violations:      make([]Violation, 0),
		Passed:          true,
	}

	err = filepath.WalkDir(absDir, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}

		rel, relErr := filepath.Rel(absDir, path)
		if relErr != nil {
			return relErr
		}

		if rel == "." {
			return nil
		}

		// Skip standard git metadata
		if d.IsDir() && (d.Name() == ".git" || d.Name() == "node_modules" || d.Name() == "vendor") {
			return filepath.SkipDir
		}

		// Check prohibited path patterns
		normRel := filepath.ToSlash(rel)
		for _, pat := range prohibited {
			if strings.Contains(normRel, pat) {
				result.Violations = append(result.Violations, Violation{
					Path:    normRel,
					Rule:    "POL-OPENCORE-PROHIBITED-PATH",
					Message: fmt.Sprintf("Prohibited pattern '%s' detected in community release payload", pat),
				})
				if d.IsDir() {
					return filepath.SkipDir
				}
				break
			}
		}

		if !d.IsDir() {
			result.FilesAudited++

			// Check prohibited extensions
			lower := strings.ToLower(d.Name())
			for _, ext := range DefaultProhibitedExtensions {
				if strings.HasSuffix(lower, ext) {
					result.Violations = append(result.Violations, Violation{
						Path:    normRel,
						Rule:    "POL-OPENCORE-PROHIBITED-EXTENSION",
						Message: fmt.Sprintf("Prohibited file extension '%s' detected", ext),
					})
					break
				}
			}
		}

		return nil
	})

	if err != nil {
		return nil, fmt.Errorf("error walking target tree: %w", err)
	}

	if len(result.Violations) > 0 {
		result.Passed = false
	}

	return result, nil
}
