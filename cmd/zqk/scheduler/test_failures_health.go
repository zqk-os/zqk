package scheduler

import (
	"errors"
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"github.com/spf13/cobra"
	"github.com/zqk-os/zqk/pkg/cliapp"
	schedpkg "github.com/zqk-os/zqk/pkg/scheduler"
)

func runTestFailuresHealth(_ *cli.Context, cmd *cobra.Command) error {
	projectRoot, limit, err := resolveSchedulerRootAndLimit(cmd, 500)
	if err != nil {
		return err
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
		o := extractTestHealthMapString(m, schedpkg.KeyTestOutcome)
		if o != emptyValue {
			outcomeCount[o]++
		}
		fp := extractTestHealthMapString(m, schedpkg.KeyBundleCommandFingerprint)
		if fp != emptyValue && o != emptyValue {
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
	b.WriteString(resolveHealthSignalMessage(len(recent), good, bad))

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

func extractTestHealthMapString(m map[string]any, key string) string {
	if s, ok := m[key].(string); ok {
		return s
	}
	return ""
}

func resolveHealthSignalMessage(recentCount, good, bad int) string {
	switch {
	case recentCount == 0:
		return "no data.\n"
	case bad == 0:
		return "all recorded outcomes in this window are green (pass/ok).\n"
	case good >= bad:
		return "mixed or recovering (failures present but some passes in window).\n"
	default:
		return "needs attention (failures/timeouts dominate this window).\n"
	}
}

