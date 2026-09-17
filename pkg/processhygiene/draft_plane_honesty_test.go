package processhygiene

import (
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/lanceman/zqk/pkg/paths"
	fileutil "github.com/lanceman/zqk/pkg/utils/fileutil"
)

// TestDraftPlaneHonestyZeroDraftOnCAS enforces that no objects on CAS (paths.ProcessDir)
// have status=draft (BLI-1786689721908382000-6402a858 / CRIT-1786695439226651000-a8288b0d).
// Draft objects belong exclusively on the draft plane (.zqk/object_drafts/).
func TestDraftPlaneHonestyZeroDraftOnCAS(t *testing.T) {
	repoRoot := findRepoRoot(t)
	processDir := filepath.Join(repoRoot, paths.ProcessDir)

	entries, err := fileutil.ReadDir(processDir)
	if err != nil {
		t.Fatalf("ReadDir %s: %v", processDir, err)
	}

	draftObjects := make([]string, 0)

	for _, entry := range entries {
		if !entry.IsDir() || strings.HasPrefix(entry.Name(), ".") || entry.Name() == "_internal" {
			continue
		}

		kindDir := filepath.Join(processDir, entry.Name())
		files, err := fileutil.ReadDir(kindDir)
		if err != nil {
			continue
		}

		for _, f := range files {
			if !strings.HasSuffix(f.Name(), ".yaml") || strings.HasPrefix(f.Name(), ".") {
				continue
			}

			filePath := filepath.Join(kindDir, f.Name())
			data, err := fileutil.ReadFile(filePath)
			if err != nil {
				continue
			}

			var obj struct {
				ID     string `yaml:"id"`
				Status string `yaml:"status"`
			}
			if err := yaml.Unmarshal(data, &obj); err != nil {
				continue
			}

			if strings.EqualFold(strings.TrimSpace(obj.Status), "draft") {
				draftObjects = append(draftObjects, filePath)
			}
		}
	}

	if len(draftObjects) > 0 {
		t.Errorf("Found %d objects with status=draft on CAS (violates draft plane honesty): %v", len(draftObjects), draftObjects)
	}
}

// TestDraftPlaneHonestyRubricDocumentation verifies that KERNEL_OBJECT_KIND_EVALUATION_RUBRIC.md
// explicitly documents draft plane honesty under Lens 2 & Lens 7 (CRIT-1786695439226667000-82701ce2).
func TestDraftPlaneHonestyRubricDocumentation(t *testing.T) {
	repoRoot := findRepoRoot(t)
	rubricPath := filepath.Join(repoRoot, "docs", "architecture", "KERNEL_OBJECT_KIND_EVALUATION_RUBRIC.md")
	data, err := fileutil.ReadFile(rubricPath)
	if err != nil {
		t.Fatalf("ReadFile %s: %v", rubricPath, err)
	}

	text := string(data)
	if !strings.Contains(text, "Draft plane honesty") && !strings.Contains(text, "draft plane honesty") {
		t.Errorf("KERNEL_OBJECT_KIND_EVALUATION_RUBRIC.md does not document draft plane honesty")
	}
	if !strings.Contains(text, "status=draft") {
		t.Errorf("KERNEL_OBJECT_KIND_EVALUATION_RUBRIC.md does not explicitly mention status=draft invariant")
	}
}
