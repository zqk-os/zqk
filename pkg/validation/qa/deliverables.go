package qa

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/utils/fileutil"
)

var (
	// ErrMissingDeliverables is returned when a deliverable-bearing object has no artifacts declared.
	ErrMissingDeliverables = errors.New(ReasonMissingArtifacts)
)

// ExtractArtifactList parses an arbitrary value (string, []string, []any) into a cleaned slice of strings.
func ExtractArtifactList(value any) []string {
	var paths []string
	switch items := value.(type) {
	case []any:
		for _, item := range items {
			if s, ok := item.(string); ok && strings.TrimSpace(s) != "" {
				paths = append(paths, strings.TrimSpace(s))
			}
		}
	case []string:
		for _, s := range items {
			if strings.TrimSpace(s) != "" {
				paths = append(paths, strings.TrimSpace(s))
			}
		}
	case string:
		if strings.TrimSpace(items) != "" {
			paths = append(paths, strings.TrimSpace(items))
		}
	}
	return paths
}

// extractArtifactPaths is maintained for package-internal backward compatibility.
func extractArtifactPaths(value any) []string {
	return ExtractArtifactList(value)
}

// ExtractObjectArtifacts extracts all declared artifact paths for an object,
// falling back to code_location if artifacts is empty.
func ExtractObjectArtifacts(obj map[string]any) []string {
	if obj == nil {
		return nil
	}
	paths := ExtractArtifactList(obj[objects.FieldKeyArtifacts])
	if len(paths) == 0 {
		paths = ExtractArtifactList(obj[objects.FieldKeyCodeLocation])
	}
	return paths
}

// IsDeliverableBearingKind reports whether the given object kind is required to produce deliverable code/artifacts.
func IsDeliverableBearingKind(kind string) bool {
	return kind == objects.KindBacklogItem || kind == objects.KindAgentTask
}

// ValidateArtifactFiles verifies that all artifact paths exist on disk, are regular files, and are readable.
// If projectRoot is non-empty, relative paths are resolved against it.
func ValidateArtifactFiles(paths []string, projectRoot string) error {
	for _, p := range paths {
		targetPath := p
		if !filepath.IsAbs(targetPath) && projectRoot != "" {
			targetPath = filepath.Join(projectRoot, targetPath)
		}
		info, err := fileutil.Stat(targetPath)
		if err != nil || info.IsDir() {
			return fmt.Errorf("artifact file does not exist or cannot be read: %s", p)
		}
		f, openErr := fileutil.Open(targetPath)
		if openErr != nil {
			return fmt.Errorf("artifact file cannot be read: %s", p)
		}
		_ = f.Close()
	}
	return nil
}

// ValidateDeliverableArtifacts verifies that all artifacts declared on the object exist on disk and are readable.
// For deliverable-bearing kinds (backlog_item, agent_task), at least one artifact is required.
// If projectRoot is non-empty, relative artifact paths are resolved against projectRoot.
func ValidateDeliverableArtifacts(obj map[string]any, projectRoot string) ([]string, error) {
	if obj == nil {
		return nil, nil
	}
	paths := ExtractObjectArtifacts(obj)
	kind, _ := obj[objects.FieldKeyKind].(string)
	id, _ := obj[objects.FieldKeyID].(string)

	if len(paths) == 0 && IsDeliverableBearingKind(kind) {
		if id != "" {
			return nil, fmt.Errorf("missing required deliverable artifacts for %s", id)
		}
		return nil, ErrMissingDeliverables
	}

	if err := ValidateArtifactFiles(paths, projectRoot); err != nil {
		return paths, err
	}

	return paths, nil
}
