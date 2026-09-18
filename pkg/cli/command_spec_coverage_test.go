package cli

import (
	"path/filepath"
	"testing"

	"github.com/spf13/cobra"

	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

func TestApplyCommandSpecCoverageBaseline(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		current   CommandSpecCoverage
		baseline  CommandSpecCoverageBaseline
		wantValid bool
		wantNew   int
		wantFixed int
	}{
		{
			name: "unchanged historical drift passes",
			current: CommandSpecCoverage{
				CommandsWithoutSpecs: []string{"system old"},
			},
			baseline: CommandSpecCoverageBaseline{
				CommandsWithoutSpecs: []string{"system old"},
			},
			wantValid: true,
		},
		{
			name: "new unspecced command fails closed",
			current: CommandSpecCoverage{
				CommandsWithoutSpecs: []string{"system new", "system old"},
			},
			baseline: CommandSpecCoverageBaseline{
				CommandsWithoutSpecs: []string{"system old"},
			},
			wantNew: 1,
		},
		{
			name:    "resolved historical drift passes",
			current: CommandSpecCoverage{},
			baseline: CommandSpecCoverageBaseline{
				CommandsWithoutSpecs: []string{"system old"},
			},
			wantValid: true,
			wantFixed: 1,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			got := ApplyCommandSpecCoverageBaseline(test.current, test.baseline, "baseline.json")
			if got.Valid != test.wantValid {
				t.Fatalf("Valid = %v, want %v", got.Valid, test.wantValid)
			}
			if len(got.NewCommandsWithoutSpecs) != test.wantNew {
				t.Errorf("new commands = %v, want count %d", got.NewCommandsWithoutSpecs, test.wantNew)
			}
			if len(got.ResolvedCommands) != test.wantFixed {
				t.Errorf("resolved commands = %v, want count %d", got.ResolvedCommands, test.wantFixed)
			}
		})
	}
}

func TestAnalyzeCommandSpecCoverageFindsUnspecifiedCommand(t *testing.T) {
	t.Parallel()

	specsDir := t.TempDir()
	systemDir := filepath.Join(specsDir, "system")
	if err := fileutil.Mkdir(systemDir, 0o755); err != nil {
		t.Fatal(err)
	}
	for path, contents := range map[string]string{
		filepath.Join(specsDir, "system_command.yaml"):   "name: system\nshort: system\ndescription: system\n",
		filepath.Join(systemDir, "covered_command.yaml"): "name: covered\nshort: covered\ndescription: covered\n",
	} {
		if err := fileutil.WriteFile(path, []byte(contents), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	root := &cobra.Command{Use: "zqk"}
	system := &cobra.Command{Use: "system"}
	system.AddCommand(&cobra.Command{Use: "covered"})
	system.AddCommand(&cobra.Command{Use: "missing"})
	root.AddCommand(system)

	coverage, err := AnalyzeCommandSpecCoverage(root, specsDir)
	if err != nil {
		t.Fatal(err)
	}
	if coverage.Parity {
		t.Fatal("expected parity failure")
	}
	if len(coverage.CommandsWithoutSpecs) != 1 || coverage.CommandsWithoutSpecs[0] != "system missing" {
		t.Fatalf("CommandsWithoutSpecs = %v", coverage.CommandsWithoutSpecs)
	}
}
