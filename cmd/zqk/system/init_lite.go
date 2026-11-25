package system

import (
	"fmt"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/zqk-os/zqk/pkg/cli/bldr_cli_cmd_v1"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/search"
	"github.com/zqk-os/zqk/pkg/utils/fileutil"
)

// runLiteInit executes the zero-bloat Progressive Disclosure onboarding path ("Land with zgrep, Expand to Kernel").
// It creates minimal .zqk/{cache,logs,state} directories, generates multi-host IDE rules, and builds the
// persistent trigram index without seeding the full .zqk/process CAS ontology.
func runLiteInit(cmd *cobra.Command, projectRoot string, force bool) error {
	if projectRoot == "" {
		cwd, err := fileutil.Getwd()
		if err != nil {
			return fmt.Errorf("failed resolving working directory: %w", err)
		}
		projectRoot = cwd
	}
	if err := paths.ValidateProjectRootNesting(projectRoot); err != nil {
		return fmt.Errorf("cannot initialize lite workspace: %w", err)
	}

	zqkDir := filepath.Join(projectRoot, paths.ProjectDataDir)
	liteDirs := []string{
		filepath.Join(zqkDir, paths.CacheDir),
		filepath.Join(zqkDir, paths.LogsDir),
		filepath.Join(zqkDir, paths.StateDir),
	}
	if err := createDirectoryWithSubdirs(zqkDir, liteDirs); err != nil {
		return fmt.Errorf("failed creating lite workspace directories: %w", err)
	}

	res, err := bldr_cli_cmd_v1.GenerateRules(projectRoot, bldr_cli_cmd_v1.RulesGenerateOptions{
		Hosts: []string{"all"},
		Force: force,
	})
	if err != nil {
		return fmt.Errorf("failed generating multi-host IDE rules: %w", err)
	}

	engine := search.NewEngine(projectRoot)
	if idxErr := engine.BuildTrigramIndex(search.SearchOptions{UseIndex: true}); idxErr != nil {
		return fmt.Errorf("failed building initial trigram index: %w", idxErr)
	}

	out := cmd.OutOrStdout()
	fmt.Fprintf(out, "✓ Initialized ZQK Lite (zgrep-first) workspace in %s\n", projectRoot)
	fmt.Fprintf(out, "  • Zero-bloat footprint: created %s/{cache,logs,state} (no .zqk/process CAS objects seeded)\n", paths.ProjectDataDir)
	fmt.Fprintf(out, "  • Trigram search index warmed and cached for sub-15ms queries\n")
	for _, file := range res.Written {
		fmt.Fprintf(out, "  • Wrote IDE rule: %s\n", file)
	}
	for _, file := range res.Skipped {
		fmt.Fprintf(out, "  • Kept existing IDE rule: %s (use --force to overwrite)\n", file)
	}
	fmt.Fprintf(out, "\nNext Steps (60-Second Quickstart):\n")
	fmt.Fprintf(out, "  1. Structural polyglot search:  %s\n", paths.CLIUsage("grep --ast --kind struct"))
	fmt.Fprintf(out, "  2. Token-budgeted agent JSON:   %s\n", paths.CLIUsage("grep 'error' --max-tokens 2000 -f json"))
	fmt.Fprintf(out, "  3. Pilot ROI compliance report: %s\n", paths.CLIUsage("grep --roi"))
	fmt.Fprintf(out, "  4. Expand to full Knowledge Kernel anytime: %s\n", paths.CLIUsage("init --merge"))
	return nil
}
