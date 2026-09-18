package internal

import (
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"github.com/zqk-os/zqk/pkg/cli"
	"github.com/zqk-os/zqk/pkg/objects"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

// TestInternalCommandSurfaceMatchesYAMLSpecs asserts that each fixed internal command's
// registered local flags match the declarative contract under .zqk/cli/specs/internal/.
// See docs/architecture/COMMAND_SPEC_COVERAGE.md — specs are the registration record.
func specIDExists(specsDir, id string) bool {
	found := false
	_ = filepath.Walk(specsDir, func(path string, info fileutil.FileInfo, err error) error {
		if err != nil || info.IsDir() || filepath.Ext(path) != ".yaml" {
			return nil
		}
		content, err := fileutil.ReadFile(path)
		if err == nil && strings.Contains(string(content), "id: "+id) {
			found = true
			return filepath.SkipAll
		}
		return nil
	})
	return found
}

func findSpecByID(t *testing.T, specsDir, id string) *cli.CommandSpec {
	t.Helper()
	var foundPath string
	_ = filepath.Walk(specsDir, func(path string, info fileutil.FileInfo, err error) error {
		if err != nil || info.IsDir() || filepath.Ext(path) != ".yaml" {
			return nil
		}
		content, err := fileutil.ReadFile(path)
		if err == nil && strings.Contains(string(content), "id: "+id) {
			foundPath = path
			return filepath.SkipDir
		}
		return nil
	})
	if foundPath == "" {
		t.Fatalf("could not find spec with id %s in %s", id, specsDir)
	}
	return mustLoadSpec(t, foundPath)
}

func TestInternalCommandSurfaceMatchesYAMLSpecs(t *testing.T) {
	t.Parallel()
	root := findProjectRootForDNATest(t)
	specDir := filepath.Join(root, ".zqk/cli/specs")
	// Historical CSPEC-internal-* DNA may be absent until internal file DNA is fully migrated.
	if !specIDExists(specDir, "CSPEC-internal-bulk_command") {
		t.Skip(".zqk/cli/specs missing CSPEC-internal-* DNA (internal command DNA not in this checkout)")
	}

	bulkSpec := findSpecByID(t, specDir, "CSPEC-internal-bulk_command")
	bulkDeleteSpec, ok := cli.BulkSubcommandSpec(bulkSpec, "delete")
	if !ok || bulkDeleteSpec == nil {
		t.Fatal("internal bulk_command.yaml must define subcommand delete with inline spec")
	}

	fieldsRoot := findSpecByID(t, specDir, "CSPEC-internal-root_fields_command")
	fieldsKind := findSpecByID(t, specDir, "CSPEC-internal-kind_fields_command")

	cases := []struct {
		label string
		spec  *cli.CommandSpec
		cmd   func() *cobra.Command
	}{
		{"list", findSpecByID(t, specDir, "CSPEC-internal-list_command"), NewInternalListCmd},
		{"get", findSpecByID(t, specDir, "CSPEC-internal-get_command"), NewInternalGetCmd},
		{"create", findSpecByID(t, specDir, "CSPEC-internal-create_command"), NewInternalCreateCmd},
		{"update", findSpecByID(t, specDir, "CSPEC-internal-update_command"), NewInternalUpdateCmd},
		{"delete", findSpecByID(t, specDir, "CSPEC-internal-delete_command"), NewInternalDeleteCmd},
		{"count", findSpecByID(t, specDir, "CSPEC-internal-count_command"), NewInternalCountCmd},
		{"process", findSpecByID(t, specDir, "CSPEC-internal-process_command"), NewInternalProcessCmd},
		{"bulk_delete", bulkDeleteSpec, NewInternalBulkDeleteCmd},
		{"fields_root", fieldsRoot, NewInternalRootFieldsCmd},
		{"fields_kind", fieldsKind, newInternalKindFieldsCmd},
	}

	for _, tc := range cases {
		t.Run(tc.label, func(t *testing.T) {
			t.Parallel()

			rawWant := cli.ExpectedLocalFlagNamesFromCommandSpec(tc.spec)
			var want []string
			commonFlags := map[string]bool{
				objects.FieldKeyFormat: true, "output": true, "quiet": true, "verbose": true, "timeout": true, "columns": true,
			}
			for _, w := range rawWant {
				if !commonFlags[w] {
					want = append(want, w)
				}
			}

			got := sortedLocalFlagNames(tc.cmd())
			if !slices.Equal(want, got) {
				t.Errorf("local flags must match spec DNA\nwant: %v\ngot:  %v", want, got)
			}
		})
	}
}

func mustLoadSpec(t *testing.T, path string) *cli.CommandSpec {
	t.Helper()
	s, err := cli.LoadCommandSpecYAML(path)
	if err != nil {
		t.Fatalf("load spec %s: %v", path, err)
	}
	return s
}

func sortedLocalFlagNames(cmd *cobra.Command) []string {
	var names []string
	commonFlags := map[string]bool{
		objects.FieldKeyFormat: true, "output": true, "quiet": true, "verbose": true, "timeout": true, "columns": true,
	}
	cmd.Flags().VisitAll(func(f *pflag.Flag) {
		if !commonFlags[f.Name] {
			names = append(names, f.Name)
		}
	})
	sort.Strings(names)
	return names
}

func findProjectRootForDNATest(t *testing.T) string {
	t.Helper()
	wd, err := fileutil.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	dir := wd
	for {
		if _, err := fileutil.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatalf("go.mod not found from %s", wd)
		}
		dir = parent
	}
}
