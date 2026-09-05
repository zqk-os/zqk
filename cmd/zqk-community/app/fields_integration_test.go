package app_test

import (
	"github.com/lanceman/zqk/pkg/datacell"
	fileutil "github.com/lanceman/zqk/pkg/utils/fileutil"

	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/lanceman/zqk/cmd/zqk-community/app"
	"github.com/lanceman/zqk/cmd/zqk/object"
	testkit "github.com/lanceman/zqk/pkg/testkit"
	"github.com/lanceman/zqk/pkg/zqkenv"

	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/paths"
)

func init() {
	// So the first GetGlobalFieldRegistry() (e.g. when ensureCommandsRegistered runs) finds specs
	// and adds dynamic kind subcommands, set ZQK_TEST_ROOT to project root when unset.
	if os.Getenv(zqkenv.TestRoot()) != app.EmptyValue {
		return
	}
	dir, err := fileutil.Getwd()
	if err != nil {
		return
	}
	for {
		specsDir := filepath.Join(dir, paths.ProcessInternalObjectSpecsDir)
		if info, err := fileutil.Stat(specsDir); err == nil && info.IsDir() {
			_ = os.Setenv(zqkenv.TestRoot(), dir)
			return
		}
		if _, err := fileutil.Stat(filepath.Join(dir, "go.mod")); err == nil {
			if _, err := fileutil.Stat(datacell.ProcessPrimaryDir(dir)); err == nil {
				_ = os.Setenv(zqkenv.TestRoot(), dir)
				return
			}
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
}

// setupFieldsIntegrationWithSpecs sets up an isolated test root with specs and registers teardown via testkit.
func setupFieldsIntegrationWithSpecs(t *testing.T, scenarioName string) (testRoot, projectRoot string) {
	t.Helper()
	testRoot, projectRoot = setupIntegrationTestWithSpecsApp(t, scenarioName)
	t.Cleanup(func() {
		_ = testkit.RunStandardTeardown(testkit.TeardownOptions{
			ProjectRoot:                       testRoot,
			StripProcessArtifacts:             true,
			DrainGlobalListingIndexQueueFirst: true,
			AggressiveTempProjectCleanup:      true,
			WALTimeout:                        20 * time.Second,
			ShutdownTimeout:                   20 * time.Second,
		})
	})
	return testRoot, projectRoot
}

// isObjectOrInternalHelp returns true if out looks like object or internal command help
// (wrong routing: kind-specific "object <kind> fields" / "internal <kind> fields" showed parent help).
func isObjectOrInternalHelp(out string) bool {
	objectHelp := "Object operations for managing all object kinds"
	internalHelp := "Manage internal and built-in objects with privileged access"
	return strings.Contains(out, objectHelp) || strings.Contains(out, internalHelp)
}

// TestFields_AAAListKinds_Integration runs first (file order) so ensureCommandsRegistered()
// runs with ZQK_TEST_ROOT set and dynamic kind subcommands are added for later tests.
func TestFields_AAAListKinds_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	_, projectRoot := setupFieldsIntegrationWithSpecs(t, "fields-list-kinds-integration")
	_ = projectRoot

	// Ensure registry can load from test root (may have been created earlier with empty path)
	reg := objects.GetGlobalFieldRegistry()
	if reg.TryReloadFromFindSpecsDir() {
		_ = reg.LoadFields()
	}
	if err := reg.LoadFields(); err != nil {
		t.Fatalf("registry LoadFields after setup: %v", err)
	}
	kinds, err := reg.GetAllKinds()
	if err != nil {
		t.Fatalf("registry GetAllKinds: %v", err)
	}
	var hasBacklog bool
	for _, k := range kinds {
		if k == objects.KindBacklogItem {
			hasBacklog = true
			break
		}
	}
	if !hasBacklog {
		t.Fatalf("registry has no %s after LoadFields; kinds=%v", objects.KindBacklogItem, kinds)
	}

	// Ensure registration runs with ZQK_TEST_ROOT set (already done by SetupIntegrationTestWithSpecs)
	root := app.NewRootCommand()
	// Add kind subcommands from registry so "object <kind> fields" works when registration ran before registry was loaded
	objCmd, _, _ := root.Find([]string{"object"})
	if objCmd != nil && len(kinds) > 0 {
		object.RegisterKindCommandsForKinds(objCmd, kinds)
	}

	root.SetArgs([]string{"object", "fields", "--list-kinds"})

	out := captureStdoutApp(t, func() {
		if err := root.Execute(); err != nil {
			t.Fatalf("object fields --list-kinds: %v", err)
		}
	})

	if !strings.Contains(out, objects.KindBacklogItem) && !strings.Contains(out, "Kinds:") {
		t.Errorf("expected kinds or Kinds in output, got: %s", out[:min(200, len(out))])
	}
	// Sanity check: kind subcommands must be present for later tests
	rootCmd := app.NewRootCommand()
	objCmd, _, _ = rootCmd.Find([]string{"object"})
	if objCmd != nil {
		var hasBacklog bool
		for _, c := range objCmd.Commands() {
			if c.Name() == objects.KindBacklogItem {
				hasBacklog = true
				break
			}
		}
		if !hasBacklog {
			t.Errorf("after list-kinds: object command missing %s subcommand (RegisterDynamicKindCommands did not add kinds)", objects.KindBacklogItem)
		}
	}
}

// TestFields_ObjectBacklogItem_Integration tests "object backlog_item fields".
// Runs in same process after TestFields_AAAListKinds_Integration so kind subcommands are registered.
func TestFields_ObjectBacklogItem_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	_, _ = setupFieldsIntegrationWithSpecs(t, "fields-object-integration")

	root := app.NewRootCommand()
	root.SetArgs([]string{"object", objects.KindBacklogItem, "fields"})

	out := captureStdoutApp(t, func() {
		if err := root.Execute(); err != nil {
			t.Fatalf("object backlog_item fields: %v", err)
		}
	})

	if isObjectOrInternalHelp(out) {
		objCmd, _, _ := root.Find([]string{"object"})
		children := []string{}
		if objCmd != nil {
			for _, c := range objCmd.Commands() {
				children = append(children, c.Name())
			}
		}
		t.Fatalf("expected fields output, got object help; object children: %v", children)
	}
	if !strings.Contains(out, "Common Fields") && !strings.Contains(out, "Fields") {
		t.Errorf("expected Common Fields or Fields in output, got: %s", out[:min(200, len(out))])
	}
	if !strings.Contains(out, "Specialized Fields") && !strings.Contains(out, objects.KindBacklogItem) {
		t.Errorf("expected Specialized Fields or kind name, got: %s", out[:min(200, len(out))])
	}
}

// TestFields_ObjectListKinds_JSON_Integration tests "object fields --list-kinds --format json".
func TestFields_ObjectListKinds_JSON_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	_, _ = setupFieldsIntegrationWithSpecs(t, "fields-list-kinds-json-integration")

	root := app.NewRootCommand()
	root.SetArgs([]string{"object", "fields", "--list-kinds", "--format", "json"})

	out := captureStdoutApp(t, func() {
		if err := root.Execute(); err != nil {
			t.Fatalf("object fields --list-kinds --format json: %v", err)
		}
	})

	var m map[string]any
	if err := json.Unmarshal([]byte(out), &m); err != nil {
		t.Fatalf("output should be JSON: %v\noutput: %s", err, out[:min(300, len(out))])
	}
	if _, ok := m["kinds"]; !ok {
		t.Errorf("expected 'kinds' key in JSON, got keys: %v", keys(m))
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func keys(m map[string]any) []string {
	k := make([]string, 0, len(m))
	for s := range m {
		k = append(k, s)
	}
	return k
}
