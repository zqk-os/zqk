package diagnostics

import (
	"path/filepath"
	"regexp"
	"sort"
	"time"

	fileutil "github.com/lanceman/zqk/pkg/utils/fileutil"
)

// diagnosticsRetentionMaxCaptureSets is how many complete timestamped capture groups to retain
// under outputDir (each capture adds ~5 files). Older groups are removed best-effort after a
// successful CaptureDiagnostics so flat directories under .zqk/scheduler/diagnostics stay scannable.
const diagnosticsRetentionMaxCaptureSets = 10

var diagnosticsKnownSuffixes = map[string]struct{}{
	"goroutines.pb.gz": {},
	"goroutines.txt":   {},
	"heap.prof":        {},
	"processes.txt":    {},
	"threads.txt":      {},
}

// pruneDiagnosticsCaptures removes oldest capture groups for this prefix, keeping the newest
// diagnosticsRetentionMaxCaptureSets runs. Files that do not match the expected naming pattern are left
// untouched. Errors during removal are ignored (best-effort cleanup).
func pruneDiagnosticsCaptures(outputDir, prefix string) {
	keep := diagnosticsRetentionMaxCaptureSets
	if keep <= 0 {
		return
	}

	entries, err := fileutil.ReadDir(outputDir)
	if err != nil {
		return
	}

	re := regexp.MustCompile(`^` + regexp.QuoteMeta(prefix) + `_(\d{8}-\d{6})_(.+)$`)

	// ts -> full paths for files belonging to that capture
	byTS := make(map[string][]string)

	for _, ent := range entries {
		if ent.IsDir() {
			continue
		}
		name := ent.Name()
		m := re.FindStringSubmatch(name)
		if len(m) != 3 {
			continue
		}
		ts := m[1]
		suf := m[2]
		if _, ok := diagnosticsKnownSuffixes[suf]; !ok {
			continue
		}
		if _, err := time.ParseInLocation("20060102-150405", ts, time.Local); err != nil {
			continue
		}
		full := filepath.Join(outputDir, name)
		byTS[ts] = append(byTS[ts], full)
	}

	if len(byTS) <= keep {
		return
	}

	keys := make([]string, 0, len(byTS))
	for ts := range byTS {
		keys = append(keys, ts)
	}
	sort.Strings(keys)

	// Oldest first; drop until only `keep` timestamps remain.
	toRemove := keys[:len(keys)-keep]
	for _, ts := range toRemove {
		for _, p := range byTS[ts] {
			_ = fileutil.Remove(p)
		}
	}
}
