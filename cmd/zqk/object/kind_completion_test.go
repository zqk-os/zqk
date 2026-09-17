package object

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lanceman/zqk/pkg/paths"
	"github.com/lanceman/zqk/pkg/utils/fileutil"
	"github.com/lanceman/zqk/pkg/zqkenv"
	"github.com/spf13/cobra"

	"github.com/lanceman/zqk/pkg/objects"
)

// buildTestSpecIndex writes a minimal spec_index.json under the given projectRoot and
// wires the path alias cache so TryLoadSpecIndexForProjectRoot can find it via the
// standard prefix:process_internal/spec_index.json reference.
func buildTestSpecIndex(t *testing.T, projectRoot string) {
	t.Helper()

	index := &objects.SpecIndex{
		Kinds: map[string]objects.SpecKindSummary{
			pplanKindBacklogItem: {
				Kind: pplanKindBacklogItem,
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

	// Wire path alias cache so prefix:process_internal/spec_index.json resolves.
	aliases := paths.DefaultPathAliases()
	paths.ReplacePathCache(projectRoot, aliases)
}

func withProjectRootEnv(t *testing.T, projectRoot string, fn func()) {
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

func TestCompleteGroupBy_UsesSpecIndex(t *testing.T) {
	projectRoot := t.TempDir()
	buildTestSpecIndex(t, projectRoot)

	withProjectRootEnv(t, projectRoot, func() {
		// Minimal command with kind annotation so resolveCurrentKindForCompletion finds it.
		cmd := &cobra.Command{
			Use: "list",
			Annotations: map[string]string{
				objects.FieldKeyKind: pplanKindBacklogItem,
			},
		}

		// Simulate TAB after typing "sta" for --group-by.
		suggestions, _ := completeGroupBy(cmd, nil, "sta")
		if len(suggestions) == 0 {
			t.Fatalf("expected at least one suggestion, got 0")
		}

		found := false
		for _, s := range suggestions {
			parts := strings.SplitN(s, "\t", 2)
			if parts[0] == "status" {
				found = true
				if len(parts) == 2 && parts[1] == emptyValue {
					t.Errorf("expected non-empty description for status, got empty")
				}
			}
		}
		if !found {
			t.Errorf("expected 'status' in group-by suggestions, got %v", suggestions)
		}
	})
}

func TestCompleteSortBy_UsesSpecIndex(t *testing.T) {
	projectRoot := t.TempDir()
	buildTestSpecIndex(t, projectRoot)

	withProjectRootEnv(t, projectRoot, func() {
		cmd := &cobra.Command{
			Use: "list",
			Annotations: map[string]string{
				objects.FieldKeyKind: pplanKindBacklogItem,
			},
		}

		suggestions, _ := completeSortBy(cmd, nil, "sta")
		if len(suggestions) == 0 {
			t.Fatalf("expected at least one suggestion, got 0")
		}
		found := false
		for _, s := range suggestions {
			if strings.HasPrefix(s, "status") {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expected 'status' in sort-by suggestions, got %v", suggestions)
		}
	})
}

func TestCompleteListProjectFields_UsesSpecIndexAndCommaSegments(t *testing.T) {
	projectRoot := t.TempDir()
	buildTestSpecIndex(t, projectRoot)

	withProjectRootEnv(t, projectRoot, func() {
		cmd := &cobra.Command{
			Use: "list",
			Annotations: map[string]string{
				objects.FieldKeyKind: pplanKindBacklogItem,
			},
		}

		suggestions, _ := completeListProjectFields(cmd, nil, "tit")
		if len(suggestions) == 0 {
			t.Fatalf("expected suggestions for segment tit, got 0")
		}
		found := false
		for _, s := range suggestions {
			if strings.HasPrefix(s, "title") {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expected title in fields suggestions, got %v", suggestions)
		}

		suggestions2, _ := completeListProjectFields(cmd, nil, "status,tit")
		found2 := false
		for _, s := range suggestions2 {
			if strings.HasPrefix(s, "status,title") {
				found2 = true
				break
			}
		}
		if !found2 {
			t.Errorf("expected status,title prefix in comma completion, got %v", suggestions2)
		}
	})
}

func TestCompleteFilterField_UsesSpecIndexAndStripsOperator(t *testing.T) {
	projectRoot := t.TempDir()
	buildTestSpecIndex(t, projectRoot)

	withProjectRootEnv(t, projectRoot, func() {
		cmd := &cobra.Command{
			Use: "list",
			Annotations: map[string]string{
				objects.FieldKeyKind: pplanKindBacklogItem,
			},
		}

		// Simulate typing "status=" so we only complete field name.
		suggestions, _ := completeFilterField(cmd, nil, "status=")
		if len(suggestions) == 0 {
			t.Fatalf("expected at least one suggestion, got 0")
		}
		found := false
		for _, s := range suggestions {
			if strings.HasPrefix(s, "status") {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expected 'status' in filter field suggestions, got %v", suggestions)
		}
	})
}
