package cli

import (
	"path/filepath"
	"testing"

	"github.com/spf13/cobra"

	"github.com/zqk-os/zqk/pkg/paths"
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
		{
			name: "specs without commands fails closed even if recorded in baseline",
			current: CommandSpecCoverage{
				SpecsWithoutCommands: []string{"mcp/phantom_command.yaml"},
			},
			baseline: CommandSpecCoverageBaseline{
				SpecsWithoutCommands: []string{"mcp/phantom_command.yaml"},
			},
			wantValid: false,
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
	if err := fileutil.Mkdir(systemDir, paths.DirPerm755); err != nil {
		t.Fatal(err)
	}
	for path, contents := range map[string]string{
		filepath.Join(specsDir, "system_command.yaml"):   "name: system\nshort: system\ndescription: system\n",
		filepath.Join(systemDir, "covered_command.yaml"): "name: covered\nshort: covered\ndescription: covered\n",
	} {
		if err := fileutil.WriteFile(path, []byte(contents), paths.FilePerm644); err != nil {
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

func TestWriteCommandSpecCoverageBaseline_RefusesOrphanedSpecs(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	baselinePath := filepath.Join(dir, "baseline.json")

	coverage := CommandSpecCoverage{
		CommandsWithoutSpecs: []string{"system missing"},
		SpecsWithoutCommands: []string{"mcp/orphan_command.yaml"},
	}

	err := WriteCommandSpecCoverageBaseline(baselinePath, coverage)
	if err == nil {
		t.Fatal("expected error when writing baseline with orphaned specs, got nil")
	}
}

func TestCommandSpecIDFromPath_MCPSvcNormalization(t *testing.T) {
	t.Parallel()

	cases := []struct {
		path   string
		wantID string
	}{
		{"mcp/svc/daemon_command.yaml", "mcp_daemon"},
		{"mcp/svc/serve_command.yaml", "mcp_serve"},
		{"mcp/svc/install_command.yaml", "mcp_install"},
		{"mcp/svc/proxy_command.yaml", "mcp_proxy"},
		{"mcp/svc/list_tools_command.yaml", "mcp_list_tools"},
		{"system/check_command.yaml", "system_check"},
	}

	for _, tc := range cases {
		got := commandSpecIDFromPath(tc.path)
		if got != tc.wantID {
			t.Errorf("commandSpecIDFromPath(%q) = %q, want %q", tc.path, got, tc.wantID)
		}
	}
}
