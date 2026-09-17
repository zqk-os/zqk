package processhygiene_test

import (
	"path/filepath"
	"strings"
	"testing"

	fileutil "github.com/lanceman/zqk/pkg/utils/fileutil"
)

func TestPublicKindFitnessScorecardIntegrity(t *testing.T) {
	cwd, err := fileutil.Getwd()
	if err != nil {
		t.Fatalf("failed to get working directory: %v", err)
	}

	repoRoot := cwd
	for {
		if _, err := fileutil.Stat(filepath.Join(repoRoot, "go.mod")); err == nil {
			break
		}
		parent := filepath.Dir(repoRoot)
		if parent == repoRoot {
			t.Fatal("could not locate repo root containing go.mod")
		}
		repoRoot = parent
	}

	scorecardPath := filepath.Join(repoRoot, "docs", "architecture", "PUBLIC_KIND_FITNESS_SCORECARD.md")
	data, err := fileutil.ReadFile(scorecardPath)
	if err != nil {
		t.Fatalf("missing PUBLIC_KIND_FITNESS_SCORECARD.md: %v", err)
	}

	text := string(data)

	// Verify required sections and dispositions
	requiredElements := []string{
		"Public-Kind Fitness Scorecard",
		"KERNEL_OBJECT_KIND_EVALUATION_RUBRIC.md",
		"keep_enforce",
		"quarantine_fixtures",
		"remediate",
		"BLI-REDACTED", // Crevice sweep cross-link
		"L1_lifecycle",
		"L2_utilization",
		"L3_test_pollution",
		"L4_logic_teeth",
		"L5_required_fields",
		"L6_duplicative_ids",
		"L7_status_vocabulary",
	}

	for _, elem := range requiredElements {
		if !strings.Contains(text, elem) {
			t.Errorf("PUBLIC_KIND_FITNESS_SCORECARD.md missing required element: %q", elem)
		}
	}
}
