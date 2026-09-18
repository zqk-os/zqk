package scheduler

import (
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/zqk-os/zqk/pkg/zqkenv"
	"github.com/spf13/cobra"
)

func TestSchedulerConvergenceDelegatesRegistered(t *testing.T) {
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatalf("failed to get caller path")
	}
	projectRoot := filepath.Dir(filepath.Dir(filepath.Dir(filepath.Dir(filename))))
	t.Setenv(zqkenv.ProjectRoot().Name(), projectRoot)

	cmd := NewSchedulerConvergenceCmd()
	names := collectUseWords(cmd.Commands())
	want := []string{"promotion-readiness", "record-overseer-run"}
	for _, w := range want {
		if !containsString(names, w) {
			t.Fatalf("missing subcommand %q; have %v", w, names)
		}
	}
}

func collectUseWords(cmds []*cobra.Command) []string {
	var out []string
	for _, c := range cmds {
		u := strings.Fields(c.Use)
		if len(u) > 0 {
			out = append(out, u[0])
		}
	}
	return out
}

func containsString(slice []string, s string) bool {
	for _, x := range slice {
		if x == s {
			return true
		}
	}
	return false
}
