package vet

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
	"gopkg.in/yaml.v3"
)

// CheckReferenceIntegrity inspects backlog items in process directories and verifies that
// each non-draft, non-preliminary item declares at least one goal_ref or requirement_ref
// to uphold the Hierarchical Chain of Integrity. Any unanchored item yields a SeverityError finding.
func CheckReferenceIntegrity(root string, cfg *GatesConfig) ([]Finding, error) {
	var findings []Finding

	searchDirs := []string{
		filepath.Join(root, ".zqk", "process", "backlog_items"),
		filepath.Join(root, ".zqk", "process", "backlog_item"),
		filepath.Join(root, ".zqk", "datacell", "backlog_items"),
	}

	for _, dir := range searchDirs {
		if fi, err := os.Stat(dir); err != nil || !fi.IsDir() {
			continue
		}

		if walkErr := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() || !strings.HasSuffix(d.Name(), ".yaml") {
				return nil
			}

			content, readErr := fileutil.ReadFile(path)
			if readErr != nil {
				return nil
			}

			var raw map[string]any
			if unmarshalErr := yaml.Unmarshal(content, &raw); unmarshalErr != nil || raw == nil {
				return nil
			}

			kind, _ := raw["kind"].(string)
			if kind != "backlog_item" {
				return nil
			}

			id, _ := raw["id"].(string)
			status, _ := raw["status"].(string)
			statusLower := strings.ToLower(strings.TrimSpace(status))

			// Preliminary draft statuses (draft, preliminary, originated) or terminal states (archived)
			// do not fail closed here as they represent draft-plane objects.
			if statusLower == "draft" || statusLower == "preliminary" || statusLower == "originated" || statusLower == "archived" {
				return nil
			}

			hasRef := false
			if goalRefs, ok := raw["goal_refs"].([]any); ok && len(goalRefs) > 0 {
				hasRef = true
			}
			if !hasRef {
				if reqRefs, ok := raw["requirement_refs"].([]any); ok && len(reqRefs) > 0 {
					hasRef = true
				}
			}

			if !hasRef {
				rel, relErr := filepath.Rel(root, path)
				if relErr != nil {
					rel = path
				}
				findings = append(findings, Finding{
					CheckID:  "reference/missing-goal-req-ref",
					Suite:    "hygiene",
					File:     rel,
					Message:  fmt.Sprintf("backlog_item %s (status %s) must declare at least one goal_ref or requirement_ref (Hierarchical Chain of Integrity)", id, status),
					Severity: SeverityError,
				})
			}
			return nil
		}); walkErr != nil {
			return nil, walkErr
		}
	}

	return findings, nil
}
