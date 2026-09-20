package matrix

import (
	"fmt"
	"path/filepath"
	"testing"

	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/testkit"
	"github.com/zqk-os/zqk/pkg/utils/fileutil"
)

// isolatedMatrixProject seeds a temp project (clears PROJECT_ROOT) and writes the fixture registry.
// Matrix CLI resolves the registry from ResolveProjectRoot; TEST_ROOT alone loses to PROJECT_ROOT.
func isolatedMatrixProject(t *testing.T, csvBody string) string {
	t.Helper()
	proj := testkit.PrepareIsolatedTempProject(t, &testkit.IsolatedTempProjectOptions{
		SkipFileStorage: true,
		Kind:            "matrix",
	})
	writeMinimalMatrixProject(t, proj.Root, csvBody)
	return proj.Root
}

// writeMinimalMatrixProject writes docs/quality/matrix_registry.yaml, profile.yaml, and m.csv under projectRoot.
// csvBody must include a header line starting with file_path,fully_vetted (minimal gate matrix for CLI tests).
func writeMinimalMatrixProject(t *testing.T, projectRoot, csvBody string) {
	t.Helper()
	dq := filepath.Join(projectRoot, paths.DocsQualityDir)
	if err := fileutil.EnsureDir(dq); err != nil {
		t.Fatal(err)
	}
	registry := fmt.Sprintf(`schema_version: 1
default_name: t
matrices:
  t:
    csv: %s/m.csv
    profile: %s/profile.yaml
`, paths.DocsQualityDir, paths.DocsQualityDir)
	if err := fileutil.WriteSecureFile(filepath.Join(dq, "matrix_registry.yaml"), []byte(registry)); err != nil {
		t.Fatal(err)
	}
	if err := fileutil.WriteSecureFile(filepath.Join(dq, "m.csv"), []byte(csvBody)); err != nil {
		t.Fatal(err)
	}
	prof := `completion:
  gate_columns:
    - fully_vetted
  done_values:
    - yes
    - pending
`
	if err := fileutil.WriteSecureFile(filepath.Join(dq, "profile.yaml"), []byte(prof)); err != nil {
		t.Fatal(err)
	}
}
