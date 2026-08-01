package scheduler

import (
	"strings"
	"sync"
)

// schedulerBucketCatchAll is the directory name for job IDs that match no [schedulerBucketRules] prefix.
const schedulerBucketCatchAll = "_other"

// schedulerBucketRule maps a stable job ID prefix to one filesystem bucket under .zqk/scheduler/state/.
//
// Order is significant: the first matching prefix wins. Keep more specific prefixes before broader ones
// (e.g. SCH-run-cmd-zqk-internal- before SCH-run-cmd-). When adding a bucket, append one row here and
// rely on tests in scheduler_state_bucket_test.go — do not duplicate names or prefixes elsewhere.
//
// Policy: workload-family bucketing for scan limits on top-level state/ entries; see
// .cursor/rules/filesystem-data-layout.mdc and docs/architecture/FILESYSTEM_DATA_LAYOUT.md.
type schedulerBucketRule struct {
	Name   string
	Prefix string
}

// schedulerBucketRules is the only definition of workload buckets (names + ID prefixes).
var schedulerBucketRules = []schedulerBucketRule{
	{Name: "bundle", Prefix: "SCH-run-bundle-"},
	{Name: "data-cell-stream", Prefix: "SCH-run-data-cell-stream-"},
	{Name: "pkg", Prefix: "SCH-run-pkg-"},
	{Name: "internal", Prefix: "SCH-run-cmd-zqk-internal-"},
	{Name: "ext", Prefix: "SCH-run-ext-"},
	{Name: "cmd", Prefix: "SCH-run-cmd-"},
}

var schedulerBucketKnownDirsOnce sync.Once

// schedulerBucketKnownDirs records valid top-level bucket directory names under state/ (rules + catch-all).
var schedulerBucketKnownDirs map[string]struct{}

func schedulerBucketKnownDirsMemo() map[string]struct{} {
	schedulerBucketKnownDirsOnce.Do(func() {
		m := make(map[string]struct{}, len(schedulerBucketRules)+1)
		for _, r := range schedulerBucketRules {
			m[r.Name] = struct{}{}
		}
		m[schedulerBucketCatchAll] = struct{}{}
		schedulerBucketKnownDirs = m
	})
	return schedulerBucketKnownDirs
}

func isSchedulerStateBucketDir(name string) bool {
	_, ok := schedulerBucketKnownDirsMemo()[name]
	return ok
}

func schedulerStateBucket(jobID string) string {
	id := strings.TrimSpace(jobID)
	for _, r := range schedulerBucketRules {
		if strings.HasPrefix(id, r.Prefix) {
			return r.Name
		}
	}
	return schedulerBucketCatchAll
}

// schedulerStateBucketREADMEBucketLegend returns a pipe-separated list of bucket names for _README.yaml
// so on-disk hints stay aligned with [schedulerBucketRules] without duplicating literals.
func schedulerStateBucketREADMEBucketLegend() string {
	var b strings.Builder
	for i, r := range schedulerBucketRules {
		if i > 0 {
			b.WriteString("|")
		}
		b.WriteString(r.Name)
	}
	b.WriteString("|")
	b.WriteString(schedulerBucketCatchAll)
	return b.String()
}
