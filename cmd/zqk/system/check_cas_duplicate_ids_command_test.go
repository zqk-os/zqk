package system

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

// repoRootFromTest walks up from the test's working directory to the module root holding the process dir.
func repoRootFromTest(t *testing.T) string {
	t.Helper()
	dir, err := fileutil.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	for i := 0; i < 8; i++ {
		if st, err := fileutil.Stat(filepath.Join(dir, paths.ProcessDir)); err == nil && st.IsDir() {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	t.Skip("could not locate repo root containing process dir")
	return ""
}

// processKindDirs lists the process subdirectories the CAS duplicate inventory scans, using the
// same skip rule as InventoryCASDuplicateIDs (leading '_' or '.').
func processKindDirs(t *testing.T, root string) []string {
	t.Helper()
	entries, err := fileutil.ReadDir(filepath.Join(root, paths.ProcessDir))
	if err != nil {
		t.Fatalf("read process dir: %v", err)
	}
	var out []string
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		n := e.Name()
		if n == "" || n[0] == '_' || n[0] == '.' {
			continue
		}
		out = append(out, n)
	}
	if len(out) == 0 {
		t.Fatal("no scannable process kind directories found")
	}
	return out
}

// kindArgOf extracts the positional kind argument from a rendered cleanup-duplicates command, or ""
// when the command names no kind.
func kindArgOf(cmd string) string {
	fields := strings.Fields(cmd)
	for i, f := range fields {
		if f != "cleanup-duplicates" {
			continue
		}
		if i+1 < len(fields) && !strings.HasPrefix(fields[i+1], "-") {
			return fields[i+1]
		}
		return ""
	}
	return ""
}

// TestCASDuplicateQuarantineCommand_namesOnlyKindsTheCLIAccepts checks the remediation this diagnostic
// prints against the very function that judges it, objects.ResolveAndValidateKindForProject. That is
// the function whose rejection the operator saw:
//
//	unknown kind "scheduler_jobs": not in this project's object spec index
//
// The inventory scans .zqk/process by directory and used to report the directory as the kind, so the
// printed fix carried "scheduler_jobs" into an argument that only accepts "scheduler_job". A
// diagnostic whose suggested command cannot run is worse than silence: it tells the reader the tool
// is broken when the finding was real.
func TestCASDuplicateQuarantineCommand_namesOnlyKindsTheCLIAccepts(t *testing.T) {
	root := repoRootFromTest(t)

	for _, dir := range processKindDirs(t, root) {
		kind := objects.GetKindFromDirectory(dir)
		cmd := casDuplicateQuarantineCommand(kind)
		arg := kindArgOf(cmd)

		if kind == "" {
			if arg != "" {
				t.Errorf("dir %q maps to no registered kind, but the suggested command still names %q: %s",
					dir, arg, cmd)
			}
			continue
		}

		if arg != kind {
			t.Errorf("dir %q resolved to kind %q but the command names %q: %s", dir, kind, arg, cmd)
			continue
		}

		if _, err := objects.ResolveAndValidateKindForProject(root, arg); err != nil {
			t.Errorf("dir %q produced a command the CLI rejects: %s\n    %v", dir, cmd, err)
		}
	}
}

// TestCASDuplicateQuarantineCommand_omitsKindWhenUnresolvable pins the fallback directly, since the
// sweep above only exercises it if some directory happens to map to no kind.
func TestCASDuplicateQuarantineCommand_omitsKindWhenUnresolvable(t *testing.T) {
	cmd := casDuplicateQuarantineCommand("")
	if got := kindArgOf(cmd); got != "" {
		t.Errorf("expected no kind argument for an unresolvable kind, got %q in %s", got, cmd)
	}
	if !strings.Contains(cmd, "--hash-duplicates") {
		t.Errorf("fallback command lost the --hash-duplicates flag: %s", cmd)
	}
}

// TestCASDuplicateQuarantineCommand_rejectsDirectoryNamesForPluralKinds guards the specific shape of
// the regression: directory names that differ from their kind must never appear as the argument.
// backlog is included deliberately -- it maps to backlog_item, so singularizing a directory name
// would not have produced the right answer either.
func TestCASDuplicateQuarantineCommand_rejectsDirectoryNamesForPluralKinds(t *testing.T) {
	root := repoRootFromTest(t)

	for _, dir := range []string{"scheduler_jobs", "backlog_items"} {
		kind := objects.GetKindFromDirectory(dir)
		if kind == "" {
			t.Fatalf("directory %q no longer maps to a kind; update this guard", dir)
		}
		if kind == dir {
			t.Fatalf("directory %q now equals its kind, so it cannot guard the regression; pick another", dir)
		}
		cmd := casDuplicateQuarantineCommand(kind)
		// Compare the extracted argument, not a substring of the whole command: "backlog_item"
		// contains "backlog", so a containment check would fail on the correct output.
		arg := kindArgOf(cmd)
		if arg == dir {
			t.Errorf("command passed directory name %q instead of kind %q: %s", dir, kind, cmd)
		}
		if _, err := objects.ResolveAndValidateKindForProject(root, arg); err != nil {
			t.Errorf("kind %q for directory %q is not CLI-acceptable: %v", kind, dir, err)
		}
	}
}
