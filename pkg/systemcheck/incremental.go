package systemcheck

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/zqk-os/zqk/pkg/execwrap"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/storage"
	caspkg "github.com/zqk-os/zqk/pkg/storage/cas"
	"github.com/zqk-os/zqk/pkg/systemcheck/integrity"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
	"gopkg.in/yaml.v3"
)

// DirtyProcessFile represents a modified, added, or deleted file under .zqk/process/
type DirtyProcessFile struct {
	Path     string // relative to projectRoot, e.g. .zqk/process/backlog_items/<hash>.yaml
	FullPath string // absolute path on disk
	Kind     string // inferred object kind (e.g. backlog_item)
	ObjectID string // extracted object ID (e.g. BLI-xxx)
	Status   string // extracted status (e.g. planned, conceptual, etc.)
	Staged   bool   // true if staged in git index
	Deleted  bool   // true if file was deleted
	Content  []byte // file content (from disk or git index)
}

// IncrementalOptions configures incremental dirty-object validation
type IncrementalOptions struct {
	ProjectRoot string
	StagedOnly  bool
	Fast        bool
	Tier        int
}

// IncrementalCheckSummary summarizes an incremental check execution
type IncrementalCheckSummary struct {
	DirtyFilesCount   int           `json:"dirty_files_count"`
	DeletedFilesCount int           `json:"deleted_files_count"`
	MembraneClean     bool          `json:"membrane_clean"`
	Results           []CheckResult `json:"results"`
	TotalIssues       int           `json:"total_issues"`
	BlockingIssues    int           `json:"blocking_issues"`
	Duration          time.Duration `json:"duration"`
}

// IsProcessYAMLFile returns true if relPath is a process YAML object file
// (not in hidden/internal directories and with .yaml/.yml extension).
func IsProcessYAMLFile(path string) bool {
	path = filepath.ToSlash(path)
	if !strings.HasPrefix(path, paths.ProcessDir+"/") {
		return false
	}
	rel := strings.TrimPrefix(path, paths.ProcessDir+"/")
	if strings.HasPrefix(rel, ".") || strings.Contains(rel, "/.") {
		return false
	}
	return strings.HasSuffix(path, ".yaml") || strings.HasSuffix(path, ".yml")
}

// InferKindFromProcessPath extracts the kind name from a process file path.
// e.g. .zqk/process/backlog_items/abc.yaml -> backlog_item
func InferKindFromProcessPath(relPath string) string {
	parts := strings.Split(filepath.ToSlash(relPath), "/")
	if len(parts) >= 3 {
		dirName := parts[2]
		kind := objects.GetKindFromDirectory(dirName)
		if kind != "" {
			return kind
		}
		return dirName
	}
	return ""
}

// GetDirtyProcessFiles inspects git staging area and working tree for dirty/staged files
// under .zqk/process/.
// If stagedOnly is true, only files in git index (staged) are inspected.
// If stagedOnly is false, unstaged working tree changes and untracked files are also included.
func GetDirtyProcessFiles(projectRoot string, stagedOnly bool) ([]DirtyProcessFile, []string, error) {
	if projectRoot == "" {
		projectRoot = "."
	}

	stagedMap := make(map[string]bool)
	stagedDeletedMap := make(map[string]bool)

	// 1. Git staged files: git diff --cached --name-only --diff-filter=ACMR .zqk/process/
	stagedCmd := execwrap.Command("git", "diff", "--cached", "--name-only", "--diff-filter=ACMR", paths.ProcessDir)
	stagedCmd.Dir = projectRoot
	if out, err := stagedCmd.Output(); err == nil {
		for _, line := range strings.Split(string(out), "\n") {
			line = strings.TrimSpace(line)
			if IsProcessYAMLFile(line) {
				stagedMap[line] = true
			}
		}
	}

	// 2. Git staged deletions: git diff --cached --name-only --diff-filter=D .zqk/process/
	stagedDelCmd := execwrap.Command("git", "diff", "--cached", "--name-only", "--diff-filter=D", paths.ProcessDir)
	stagedDelCmd.Dir = projectRoot
	if out, err := stagedDelCmd.Output(); err == nil {
		for _, line := range strings.Split(string(out), "\n") {
			line = strings.TrimSpace(line)
			if IsProcessYAMLFile(line) {
				stagedDeletedMap[line] = true
			}
		}
	}

	modifiedMap := make(map[string]bool)
	unstagedDeletedMap := make(map[string]bool)
	if !stagedOnly {
		// 3. Unstaged modified files
		modCmd := execwrap.Command("git", "diff", "--name-only", "--diff-filter=ACMR", paths.ProcessDir)
		modCmd.Dir = projectRoot
		if out, err := modCmd.Output(); err == nil {
			for _, line := range strings.Split(string(out), "\n") {
				line = strings.TrimSpace(line)
				if IsProcessYAMLFile(line) {
					modifiedMap[line] = true
				}
			}
		}

		// 4. Untracked files under .zqk/process/
		untrackedCmd := execwrap.Command("git", "ls-files", "--others", "--exclude-standard", paths.ProcessDir)
		untrackedCmd.Dir = projectRoot
		if out, err := untrackedCmd.Output(); err == nil {
			for _, line := range strings.Split(string(out), "\n") {
				line = strings.TrimSpace(line)
				if IsProcessYAMLFile(line) {
					modifiedMap[line] = true
				}
			}
		}

		// 5. Unstaged deleted files
		delCmd := execwrap.Command("git", "diff", "--name-only", "--diff-filter=D", paths.ProcessDir)
		delCmd.Dir = projectRoot
		if out, err := delCmd.Output(); err == nil {
			for _, line := range strings.Split(string(out), "\n") {
				line = strings.TrimSpace(line)
				if IsProcessYAMLFile(line) {
					unstagedDeletedMap[line] = true
				}
			}
		}
	}

	// Combine deleted paths
	deletedSet := make(map[string]bool)
	for p := range stagedDeletedMap {
		deletedSet[p] = true
	}
	for p := range unstagedDeletedMap {
		deletedSet[p] = true
	}
	var deletedPaths []string
	for p := range deletedSet {
		deletedPaths = append(deletedPaths, p)
	}

	// Combine active dirty paths
	allDirtyPaths := make(map[string]bool)
	for p := range stagedMap {
		allDirtyPaths[p] = true
	}
	for p := range modifiedMap {
		allDirtyPaths[p] = true
	}

	var dirtyFiles []DirtyProcessFile
	for relPath := range allDirtyPaths {
		if deletedSet[relPath] {
			continue
		}

		fullPath := filepath.Join(projectRoot, relPath)
		var content []byte
		var readErr error

		if fileutil.Exists(fullPath) {
			content, readErr = fileutil.ReadFile(fullPath)
		} else {
			// Read from git staged index
			showCmd := execwrap.Command("git", "show", ":"+relPath)
			showCmd.Dir = projectRoot
			content, readErr = showCmd.Output()
		}

		if readErr != nil {
			content = nil
		}

		kind := InferKindFromProcessPath(relPath)
		id := ""
		status := ""
		if len(content) > 0 {
			id = objects.ExtractProperty(content, objects.FieldKeyID)
			status = objects.ExtractProperty(content, objects.FieldKeyStatus)
		}

		dirtyFiles = append(dirtyFiles, DirtyProcessFile{
			Path:     relPath,
			FullPath: fullPath,
			Kind:     kind,
			ObjectID: id,
			Status:   status,
			Staged:   stagedMap[relPath],
			Deleted:  false,
			Content:  content,
		})
	}

	return dirtyFiles, deletedPaths, nil
}

// VerifyMembraneInvariants checks that mutator pipelines and validation rules
// enforce kernel CAS membrane invariants. This completes in sub-second time.
func VerifyMembraneInvariants(projectRoot string) ([]Issue, error) {
	var issues []Issue

	// 1. Membrane coverage gate (kernelcas Run* references)
	if ok, gaps := integrity.MembraneCoverageGate(projectRoot); !ok {
		for _, gap := range gaps {
			issues = append(issues, Issue{
				Tier:     1,
				Category: "integrity",
				Message:  fmt.Sprintf("CAS Membrane coverage gap: %s does not enforce kernelcas invariants", gap),
			})
		}
	}

	// 2. Legacy custom rules gate (no Go customRuleValidators bodies)
	if ok, note := integrity.LegacyCustomRulesGate(projectRoot); !ok {
		issues = append(issues, Issue{
			Tier:     1,
			Category: "integrity",
			Message:  fmt.Sprintf("CAS Membrane violation: %s", note),
		})
	}

	return issues, nil
}

// VerifyDirtyCASMembrane validates CAS content-addressable hash invariants,
// preliminary status prohibitions, and staged deletions for dirty files.
func VerifyDirtyCASMembrane(projectRoot string, dirtyFiles []DirtyProcessFile, deletedPaths []string) []CheckResult {
	var results []CheckResult

	// 1. Check staged deletions with CASOrphanGuard
	if len(deletedPaths) > 0 {
		guard := caspkg.NewCASOrphanGuard(projectRoot)
		if orphanResults, err := guard.CheckStagedDeletions(); err == nil {
			for _, orphan := range orphanResults {
				results = append(results, CheckResult{
					ObjectID:   orphan.DeletedID,
					ObjectKind: "orphan_reference",
					FilePath:   orphan.DeletedPath,
					Issues: []Issue{
						{
							Tier:     1,
							Category: "integrity",
							Message:  fmt.Sprintf("Deleted object %s is still referenced by surviving objects: %s", orphan.DeletedID, strings.Join(orphan.Referrers, ", ")),
						},
					},
				})
			}
		}
	}

	// 2. Check dirty files for CAS hash match and preliminary status
	for _, df := range dirtyFiles {
		var issues []Issue

		if len(df.Content) == 0 {
			issues = append(issues, Issue{
				Tier:     1,
				Category: "integrity",
				Message:  fmt.Sprintf("File is empty or cannot be read: %s", df.Path),
			})
			results = append(results, CheckResult{
				ObjectID:   df.ObjectID,
				ObjectKind: df.Kind,
				FilePath:   df.Path,
				Status:     df.Status,
				Issues:     issues,
			})
			continue
		}

		// Valid YAML syntax
		var dummy map[string]any
		if err := yaml.Unmarshal(df.Content, &dummy); err != nil {
			issues = append(issues, Issue{
				Tier:     1,
				Category: "integrity",
				Message:  fmt.Sprintf("Malformed YAML in %s: %v", df.Path, err),
			})
		}

		// Preliminary status invariant: conceptual is prohibited in .zqk/process/
		if strings.EqualFold(df.Status, "conceptual") {
			issues = append(issues, Issue{
				Tier:     1,
				Category: "integrity",
				Message:  fmt.Sprintf("CAS Membrane Violation: Preliminary status (conceptual) detected in %s. Objects with preliminary status must remain on the draft plane (.zqk/object_drafts/)", df.Path),
			})
		}

		// CAS hash invariant: if filename is 64-char sha256 hash, verify sha256(content) == filename
		base := strings.TrimSuffix(filepath.Base(df.Path), filepath.Ext(df.Path))
		if len(base) == 64 && isHex(base) {
			actualHash := storage.CalculateSHA256Hash(df.Content)
			if !strings.EqualFold(base, actualHash) {
				issues = append(issues, Issue{
					Tier:     1,
					Category: "integrity",
					Message:  fmt.Sprintf("CAS Membrane Violation: Hash mismatch in %s (expected %s, actual %s)", df.Path, base, actualHash),
				})
			}
		}

		if len(issues) > 0 {
			results = append(results, CheckResult{
				ObjectID:   df.ObjectID,
				ObjectKind: df.Kind,
				FilePath:   df.Path,
				Status:     df.Status,
				Issues:     issues,
			})
		}
	}

	return results
}

func isHex(s string) bool {
	for _, c := range s {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')) {
			return false
		}
	}
	return true
}
