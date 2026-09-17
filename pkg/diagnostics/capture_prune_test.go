package diagnostics

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lanceman/zqk/pkg/utils/fileutil"
)

func TestPruneDiagnosticsCaptures_keepsNewestTenSets(t *testing.T) {
	dir := t.TempDir()
	prefix := "scheduler_daemon"

	suffixes := []string{"goroutines.pb.gz", "goroutines.txt", "heap.prof", "processes.txt", "threads.txt"}

	// Twelve lexicographically sortable timestamps (oldest → newest).
	for i := 1; i <= 12; i++ {
		ts := formatPruneTestTS(i)
		for _, suf := range suffixes {
			name := prefix + "_" + ts + "_" + suf
			p := filepath.Join(dir, name)
			if err := fileutil.WriteSecureFile(p, []byte("x")); err != nil {
				t.Fatal(err)
			}
		}
	}

	pruneDiagnosticsCaptures(dir, prefix)

	entries, err := fileutil.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 10*len(suffixes) {
		t.Fatalf("want %d files left, got %d", 10*len(suffixes), len(entries))
	}

	// Oldest two timestamps should be gone entirely.
	for i := 1; i <= 2; i++ {
		ts := formatPruneTestTS(i)
		for _, suf := range suffixes {
			p := filepath.Join(dir, prefix+"_"+ts+"_"+suf)
			if _, err := fileutil.Stat(p); err == nil {
				t.Fatalf("expected removal of old set %s: %s still present", ts, p)
			}
		}
	}

	for i := 3; i <= 12; i++ {
		ts := formatPruneTestTS(i)
		for _, suf := range suffixes {
			p := filepath.Join(dir, prefix+"_"+ts+"_"+suf)
			if _, err := fileutil.Stat(p); err != nil {
				t.Fatalf("expected to keep %s: %v", p, err)
			}
		}
	}
}

func TestPruneDiagnosticsCaptures_leavesUnknownFiles(t *testing.T) {
	dir := t.TempDir()
	prefix := "scheduler_daemon"

	legacy := filepath.Join(dir, "orphan-old-dump.txt")
	if err := fileutil.WriteSecureFile(legacy, []byte("legacy")); err != nil {
		t.Fatal(err)
	}

	ts := formatPruneTestTS(99)
	for _, suf := range []string{"goroutines.txt", "heap.prof", "processes.txt", "threads.txt", "goroutines.pb.gz"} {
		name := prefix + "_" + ts + "_" + suf
		if err := fileutil.WriteSecureFile(filepath.Join(dir, name), []byte("x")); err != nil {
			t.Fatal(err)
		}
	}

	pruneDiagnosticsCaptures(dir, prefix)

	if _, err := fileutil.Stat(legacy); err != nil {
		t.Fatalf("unknown file should remain: %v", err)
	}

	// Single complete set still present.
	want := prefix + "_" + ts + "_"
	for _, ent := range mustReadDirNames(t, dir) {
		if strings.HasPrefix(ent, prefix+"_") && !strings.Contains(ent, "orphan") {
			if !strings.HasPrefix(ent, want) {
				t.Fatalf("unexpected diag file left: %s", ent)
			}
		}
	}
}

func formatPruneTestTS(seq int) string {
	// Valid 20060102-150405 timestamps, lexicographically ordered for seq 1..31.
	return fmt.Sprintf("202001%02d-120000", seq)
}

func mustReadDirNames(t *testing.T, dir string) []string {
	t.Helper()
	ents, err := fileutil.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	out := make([]string, 0, len(ents))
	for _, e := range ents {
		out = append(out, e.Name())
	}
	return out
}
