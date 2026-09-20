package internal

import (
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/spf13/cobra"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/utils/fileutil"
	"github.com/zqk-os/zqk/pkg/zqkenv"

	"github.com/zqk-os/zqk/pkg/objects"
)

// buildInternalTestSpecIndex writes a minimal spec_index.json under the given projectRoot and
// wires the path alias cache so TryLoadSpecIndexForProjectRoot can find it via the
// standard prefix:process_internal/spec_index.json reference.
func buildInternalTestSpecIndex(t *testing.T, projectRoot string) {
	t.Helper()

	index := &objects.SpecIndex{
		Kinds: map[string]objects.SpecKindSummary{
			"object_spec": {
				Kind: "object_spec",
				Fields: []objects.SpecFieldSummary{
					{
						Name:       "status",
						Groupable:  true,
						Filterable: true,
						Sortable:   true,
						Traits:     []string{"groupable", "filterable", "sortable"},
					},
					{
						Name:       "title",
						Filterable: true,
						Traits:     []string{"filterable"},
					},
				},
			},
		},
	}

	specDir := filepath.Join(projectRoot, paths.ProcessInternalDir)
	if err := fileutil.EnsureDir(specDir); err != nil {
		t.Fatalf("failed to create spec index dir: %v", err)
	}

	indexPath := filepath.Join(specDir, "spec_index.json")
	data, err := json.Marshal(index)
	if err != nil {
		t.Fatalf("failed to marshal spec index: %v", err)
	}
	if err := fileutil.WriteSecureFile(indexPath, data); err != nil {
		t.Fatalf("failed to write spec index: %v", err)
	}

	aliases := paths.DefaultPathAliases()
	paths.ReplacePathCache(projectRoot, aliases)
}

func withInternalProjectRootEnv(t *testing.T, projectRoot string, fn func()) {
	t.Helper()
	prev := zqkenv.ProjectRoot().Get()
	if err := zqkenv.ProjectRoot().Set(projectRoot); err != nil {
		t.Fatalf("failed to set ZQK_PROJECT_ROOT: %v", err)
	}
	defer func() {
		_ = zqkenv.ProjectRoot().Set(prev)
	}()
	fn()
}

func TestInternalCompleteGroupBy_UsesSpecIndex(t *testing.T) {
	projectRoot := t.TempDir()
	buildInternalTestSpecIndex(t, projectRoot)

	withInternalProjectRootEnv(t, projectRoot, func() {
		cmd := &cobra.Command{
			Use: "list",
			Annotations: map[string]string{
				objects.FieldKeyKind: "object_spec",
			},
		}

		suggestions, _ := internalCompleteGroupBy(cmd, nil, "sta")
		_ = suggestions // Smoke test: ensure call path does not panic.
	})
}

func TestInternalCompleteFilterField_StripsOperator(t *testing.T) {
	projectRoot := t.TempDir()
	buildInternalTestSpecIndex(t, projectRoot)

	withInternalProjectRootEnv(t, projectRoot, func() {
		cmd := &cobra.Command{
			Use: "list",
			Annotations: map[string]string{
				objects.FieldKeyKind: "object_spec",
			},
		}

		suggestions, _ := internalCompleteFilterField(cmd, nil, "status=")
		_ = suggestions // Smoke test: ensure call path does not panic.
	})
}
