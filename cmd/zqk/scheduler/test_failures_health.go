package scheduler

import (
	"errors"
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"github.com/zqk-os/zqk/internal/cli"
	"github.com/zqk-os/zqk/pkg/errfmt"
	schedpkg "github.com/zqk-os/zqk/pkg/scheduler"
	"github.com/spf13/cobra"
)

func runTestFailuresHealth(cliCtx *cli.Context, cmd *cobra.Command) error {
	projectRoot := cliCtx.ProjectRoot
	if projectRoot == emptyValue {
		projectRoot = cli.ResolveProjectRoot(".")
		if projectRoot == emptyValue {
			return errfmt.Errorf("project root not found")
		}
	}

	limit, _ := cmd.Flags().GetInt("limit")
	if limit <= 0 {
		limit = 500
	}

	path := schedpkg.TestBundlesHealthFilePath(projectRoot)
	recent, err := readTestBundleHealthTailForCLI(cmd, projectRoot, limit)
	if err != nil {
		if errors.Is(err, errTestBundleHealthFileHandled) {
			return nil
		}
		return err
	}

	var b strings.Builder
	fmt.Fprintf(&b, "Test bundle health (last %d line(s) from %s)\n\n", len(recent), filepath.Base(path))

	outcomeCount := map[string]int{}
	fingerprintLastOutcome := make(map[string]string)

	for _, m := range recent {
		o, _ := m[schedpkg.KeyTestOutcome].(string)
		if o != emptyValue {
			outcomeCount[o]++
		}
		if fp, ok := m[schedpkg.KeyBundleCommandFingerprint].(string); ok && fp != emptyValue && o != emptyValue {
			fingerprintLastOutcome[fp] = o
		}
	}

	b.WriteString("Outcome counts (in window):\n")
	keys := make([]string, 0, len(outcomeCount))
	for k := range outcomeCount {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	if len(keys) == 0 {
		b.WriteString("  (no parsed rows)\n")
	} else {
		for _, k := range keys {
			fmt.Fprintf(&b, "  %s: %d\n", k, outcomeCount[k])
		}
	}

	good := outcomeCount["pass"] + outcomeCount["ok"]
	bad := outcomeCount["test_fail"] + outcomeCount["fail"] + outcomeCount["timeout"]
	b.WriteString("\nSignal: ")
	if len(recent) == 0 {
		b.WriteString("no data.\n")
	} else if bad == 0 {
		b.WriteString("all recorded outcomes in this window are green (pass/ok).\n")
	} else if good >= bad {
		b.WriteString("mixed or recovering (failures present but some passes in window).\n")
	} else {
		b.WriteString("needs attention (failures/timeouts dominate this window).\n")
	}

	b.WriteString("\nPer-bundle fingerprint (latest outcome in window):\n")
	fps := make([]string, 0, len(fingerprintLastOutcome))
	for fp := range fingerprintLastOutcome {
		fps = append(fps, fp)
	}
	sort.Strings(fps)
	if len(fps) == 0 {
		b.WriteString("  (none)\n")
	} else {
		for _, fp := range fps {
			fmt.Fprintf(&b, "  %s → %s\n", fp, fingerprintLastOutcome[fp])
		}
	}

	b.WriteString("\nTip: failed bundle lines in events.jsonl include \"suggested_rerun_commands\" for copy-paste go test lines.\n")

	return cli.WriteOutput(cmd, []byte(b.String()))
}
