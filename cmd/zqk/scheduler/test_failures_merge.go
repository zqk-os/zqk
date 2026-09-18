package scheduler

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/objects"
	schedpkg "github.com/zqk-os/zqk/pkg/scheduler"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

// mergeTestBundleEventFailures appends failures from .zqk/logs/scheduler/cvs/test-bundles/health.jsonl
func mergeTestBundleEventFailures(projectRoot string, cutoffTime time.Time, packageFilter string, failuresByPackage map[string][]string, totalFailures *int) error {
	path := schedpkg.TestBundlesHealthFilePath(projectRoot)
	data, err := fileutil.ReadFile(path)
	if err != nil {
		if fileutil.IsNotExist(err) {
			return nil
		}
		return errfmt.Newf("read test-bundle events").Wrap(err)
	}
	for line := range strings.SplitSeq(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == emptyValue {
			continue
		}
		var entry map[string]any
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			continue
		}
		if ts, ok := entry["timestamp"].(string); ok {
			if t, err := time.Parse(time.RFC3339, ts); err == nil {
				if t.Before(cutoffTime) {
					continue
				}
			}
		}
		eventType, _ := entry[objects.FieldKeyEventType].(string)
		if eventType != schedulerStateFailed && eventType != schedulerStateError {
			continue
		}
		testFailures, ok := entry["test_failures"].([]any)
		if !ok {
			continue
		}
		for _, failure := range testFailures {
			failureStr, ok := failure.(string)
			if !ok {
				continue
			}

			var pkg, testName string
			parts := strings.Split(failureStr, ".")
			if len(parts) >= 2 {
				pkg = strings.Join(parts[:len(parts)-1], ".")
				testName = parts[len(parts)-1]
			} else {
				testName = failureStr
				pkg = "./"
				if pkgField, _ := entry["package"].(string); pkgField != "" {
					pkg = pkgField
				} else if cmd, _ := entry[objects.FieldKeyCommand].(string); cmd != "" {
					if idx := strings.Index(cmd, "go test "); idx >= 0 {
						fields := strings.Fields(cmd[idx:])
						if len(fields) >= 3 {
							pkg = fields[2]
						}
					}
				}
			}

			if packageFilter != "" && !strings.Contains(pkg, packageFilter) {
				continue
			}
			pkg = strings.TrimPrefix(pkg, "github.com/zqk-os/zqk/")
			if !strings.HasPrefix(pkg, "./") {
				pkg = "./" + pkg
			}
			jobID, _ := entry["job_id"].(string)
			if jobID != "" {
				testName = fmt.Sprintf("%s (job: %s)", testName, jobID)
			}
			failuresByPackage[pkg] = append(failuresByPackage[pkg], testName)
			*totalFailures++
		}
	}
	return nil
}
