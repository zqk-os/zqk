package cas

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/lanceman/zqk/pkg/execwrap"

	"gopkg.in/yaml.v3"

	"github.com/lanceman/zqk/pkg/objects"
	fileutil "github.com/lanceman/zqk/pkg/utils/fileutil"
)

// CASOrphanGuardResult captures the result of an orphan guard check.
type CASOrphanGuardResult struct {
	DeletedID   string   `json:"deleted_id"`
	DeletedPath string   `json:"deleted_path"`
	Referrers   []string `json:"referrers"`
}

// CASOrphanGuard enforces referential integrity for CAS objects during git commits.
// It checks that staged deletions under docs/process/ do not leave orphan references
// in surviving objects. This is the Go equivalent of scripts/check-process-delete-refs-staged.sh.
//
// TRACK: BLI-CAS-GIT-PACKAGE-ORPHAN-GUARD-001
type CASOrphanGuard struct {
	RepoRoot string
}

// NewCASOrphanGuard creates a new CASOrphanGuard rooted at repoRoot.
func NewCASOrphanGuard(repoRoot string) *CASOrphanGuard {
	return &CASOrphanGuard{RepoRoot: repoRoot}
}

// CheckStagedDeletions inspects the git staging area for docs/process/ YAML deletions
// and returns any that would leave orphan references. Returns nil if no orphans found.
// Fail-closed: any git error is returned as-is (no silent pass-through).
func (g *CASOrphanGuard) CheckStagedDeletions() ([]CASOrphanGuardResult, error) {
	cmd := execwrap.Command("git", "diff", "--cached", "--name-only", "--diff-filter=D")
	cmd.Dir = g.RepoRoot
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("cas orphan guard: git diff --cached failed: %w", err)
	}

	raw := strings.TrimSpace(string(out))
	if raw == "" {
		return nil, nil
	}

	var deletedDocs []string
	for _, f := range strings.Split(raw, "\n") {
		f = strings.TrimSpace(f)
		if strings.HasPrefix(f, "docs/process/") &&
			strings.HasSuffix(f, ".yaml") &&
			!strings.HasPrefix(f, "docs/process/_internal/") {
			deletedDocs = append(deletedDocs, f)
		}
	}

	if len(deletedDocs) == 0 {
		return nil, nil
	}

	var results []CASOrphanGuardResult
	for _, f := range deletedDocs {
		id, err := g.extractIDFromHEAD(f)
		if err != nil || id == "" {
			continue
		}

		// Check if the ID still exists in staging (CAS rename, not a real delete).
		if g.idStillInStaging(id) {
			continue
		}

		// Check for surviving references to this ID.
		referrers, err := g.findSurvivingReferrers(id)
		if err != nil {
			return nil, fmt.Errorf("cas orphan guard: ref scan failed for %s: %w", id, err)
		}

		if len(referrers) > 0 {
			results = append(results, CASOrphanGuardResult{
				DeletedID:   id,
				DeletedPath: f,
				Referrers:   referrers,
			})
		}
	}

	return results, nil
}

// extractIDFromHEAD reads the id: field from the HEAD version of a file.
func (g *CASOrphanGuard) extractIDFromHEAD(path string) (string, error) {
	cmd := execwrap.Command("git", "show", "HEAD:"+path)
	cmd.Dir = g.RepoRoot
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}

	var obj map[string]any
	if err := yaml.Unmarshal(out, &obj); err != nil {
		return "", err
	}

	id, _ := obj[objects.FieldKeyID].(string)
	return id, nil
}

// idStillInStaging reports whether the staged tree still *declares* the ID.
// If so, this is a CAS rename (delete old hash + add new hash), not a real removal.
//
// The declaration must be matched, not the bare id. Matching the id anywhere made this
// gate disable itself in precisely the case it exists to catch: a referrer's ref list
// mentions the id, the grep hits that referrer, the deletion is read as a rename, and the
// orphan commits. Commit 530ec90795 removed two criteria that two live milestones pointed
// at, and passed for this reason.
func (g *CASOrphanGuard) idStillInStaging(id string) bool {
	return len(g.stagedFilesDeclaring(id)) > 0
}

// stagedFilesDeclaring returns staged process files whose own id field is id.
func (g *CASOrphanGuard) stagedFilesDeclaring(id string) []string {
	return g.gitGrepStagedProcessFiles("-E", "^"+objects.FieldKeyID+": "+regexp.QuoteMeta(id)+"$")
}

// findSurvivingReferrers finds staged objects that still mention the given ID.
// Files that declare the id are excluded, so an object is never reported as its own referrer.
func (g *CASOrphanGuard) findSurvivingReferrers(id string) ([]string, error) {
	declaring := make(map[string]bool)
	for _, f := range g.stagedFilesDeclaring(id) {
		declaring[f] = true
	}

	var referrers []string
	for _, f := range g.gitGrepStagedProcessFiles("-F", id) {
		if !declaring[f] {
			referrers = append(referrers, f)
		}
	}
	return referrers, nil
}

// gitGrepStagedProcessFiles lists staged docs/process YAML matching the pattern.
// A non-zero exit means no match, which is not an error for this gate.
func (g *CASOrphanGuard) gitGrepStagedProcessFiles(patternFlag, pattern string) []string {
	cmd := execwrap.Command("git", "grep", "--cached", "-l", patternFlag, pattern, "--", "docs/process/**/*.yaml")
	cmd.Dir = g.RepoRoot
	out, err := cmd.Output()
	if err != nil {
		return nil
	}

	raw := strings.TrimSpace(string(out))
	if raw == "" {
		return nil
	}
	return strings.Split(raw, "\n")
}

// CheckPackageIntegrity verifies that a docs/process directory tree has no orphan
// references at the filesystem level (offline, no git required). Useful for
// packaging and snapshot validation.
func (g *CASOrphanGuard) CheckPackageIntegrity() ([]CASOrphanGuardResult, error) {
	processDir := filepath.Join(g.RepoRoot, "docs", "process")
	if _, err := fileutil.Stat(processDir); err != nil {
		return nil, fmt.Errorf("cas orphan guard: process dir not found: %w", err)
	}

	// Collect all object IDs and their reference targets.
	type objEntry struct {
		id   string
		path string
		refs []string
	}

	var objectEntries []objEntry
	allIDs := make(map[string]bool)

	err := filepath.Walk(processDir, func(path string, info fileutil.FileInfo, err error) error {
		if err != nil || info.IsDir() || !strings.HasSuffix(path, ".yaml") {
			return err
		}
		// Skip _internal config/schema files.
		rel, _ := filepath.Rel(processDir, path)
		if strings.HasPrefix(rel, "_internal") {
			return nil
		}

		data, readErr := fileutil.ReadFile(path)
		if readErr != nil {
			return nil // Skip unreadable files, don't fail.
		}

		var obj map[string]any
		if yamlErr := yaml.Unmarshal(data, &obj); yamlErr != nil {
			return nil // Skip unparseable YAML.
		}

		id, _ := obj[objects.FieldKeyID].(string)
		if id == "" {
			return nil
		}

		allIDs[id] = true
		objectEntries = append(objectEntries, objEntry{
			id:   id,
			path: path,
			refs: extractRefFields(obj),
		})
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("cas orphan guard: walk failed: %w", err)
	}

	// Find orphan references: referenced IDs that don't exist.
	var results []CASOrphanGuardResult
	for _, obj := range objectEntries {
		for _, refID := range obj.refs {
			if !allIDs[refID] {
				results = append(results, CASOrphanGuardResult{
					DeletedID:   refID,
					DeletedPath: "(missing)",
					Referrers:   []string{obj.path},
				})
			}
		}
	}

	return results, nil
}

// extractRefFields collects all ID-like reference values from a YAML object's
// reference fields (*_ref, *_refs, parent_*, goal_refs, etc.).
func extractRefFields(obj map[string]any) []string {
	var refs []string
	for key, val := range obj {
		isRef := strings.HasSuffix(key, "_ref") ||
			strings.HasSuffix(key, "_refs") ||
			strings.HasPrefix(key, "parent_") ||
			key == "goal_refs" ||
			key == "requirement_refs" ||
			key == "decision_refs" ||
			key == "priority_plan_ref"
		if !isRef {
			continue
		}

		switch v := val.(type) {
		case string:
			if v != "" {
				refs = append(refs, v)
			}
		case []any:
			for _, item := range v {
				if s, ok := item.(string); ok && s != "" {
					refs = append(refs, s)
				}
			}
		}
	}
	return refs
}
