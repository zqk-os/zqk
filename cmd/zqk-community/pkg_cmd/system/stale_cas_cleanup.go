package system

import (
	"strings"

	"github.com/lanceman/zqk/pkg/datacell"
	"github.com/lanceman/zqk/pkg/logging"
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/storage"
)

// StaleCASCleanupResult holds the result of running Stale CAS cleanup for check results.
type StaleCASCleanupResult struct {
	KindsRun     []string // Kinds that had cleanup run
	FilesHandled int      // Total files quarantined or deleted
}

// RunStaleCASCleanupForResults scans check results for "Stale CAS version" issues,
// collects distinct kinds, and runs hash-duplicates cleanup for those kinds so that
// check --auto-fix resolves Stale CAS in one run (see INTEGRITY_RESOLUTION_PLAN.md).
// It also scans for missing CAS files / stale index entries / read errors on missing files
// and removes them from the corresponding CAS indexes.
// Returns the kinds that were cleaned and the number of files quarantined/deleted.
func RunStaleCASCleanupForResults(projectRoot string, results []CheckResult, logger logging.Logger) StaleCASCleanupResult {
	kindsSet := make(map[string]bool)
	for _, r := range results {
		for _, issue := range r.Issues {
			if strings.Contains(issue.Message, "Stale CAS version") && r.ObjectKind != emptyValue {
				kindsSet[r.ObjectKind] = true
			}
		}
	}

	// Also prune any explicitly missing CAS file index entries or failed-to-read missing files
	for _, r := range results {
		for _, issue := range r.Issues {
			isStaleIndex := strings.Contains(issue.Message, "stale index entry") || strings.Contains(issue.Message, "Missing CAS file")
			isMissingFile := strings.Contains(issue.Message, "no such file or directory") &&
				(strings.Contains(issue.Message, "failed to read file") || strings.Contains(issue.Message, "Missing CAS file"))

			if (isStaleIndex || isMissingFile) && r.ObjectKind != emptyValue && r.ObjectID != emptyValue {
				dirName := objects.GetDirectoryFromKind(r.ObjectKind)
				if dirName != emptyValue {
					kindDir := datacell.CellCASPrimaryDir(projectRoot, dirName)
					cas := storage.NewContentAddressableStorage(kindDir, r.ObjectKind)
					index := cas.GetIndex()
					if index != nil {
						// Remove the bad mapping (e.g. SCH-run-bundle-21 or e94fa7d8e94623a8b6f04c45f7fad34fe07f27965df4f8666a57d72c2b3228fd.yaml)
						if err := index.RemoveMapping(r.ObjectID); err == nil {
							logging.Fluent(logger).Info("Pruned stale or bad ID mapping from CAS index").
								String("object_id", r.ObjectID).
								String("kind", r.ObjectKind).
								Log()
						} else {
							logging.Fluent(logger).Warn("Failed to prune stale ID mapping from CAS index").
								String("object_id", r.ObjectID).
								String("kind", r.ObjectKind).
								WithError(err).
								Log()
						}
					}
				}
			}
		}
	}

	if len(kindsSet) == 0 {
		return StaleCASCleanupResult{KindsRun: nil, FilesHandled: 0}
	}
	kinds := make([]string, 0, len(kindsSet))
	for k := range kindsSet {
		kinds = append(kinds, k)
	}
	// System-generated kinds (bypass kinds): delete duplicates instead of quarantining so quarantine doesn't grow indefinitely.
	deleteForKinds := storage.GetGlobalBlockingCheckConfig().GetBypassKinds()
	total, errs := RunHashDuplicatesCleanupForKinds(projectRoot, kinds, false, false, logger, nil, deleteForKinds)
	if total > 0 {
		logging.Fluent(logger).Info("Stale CAS cleanup completed").
			String("kinds", strings.Join(kinds, ", ")).
			Int("files_quarantined", total).
			Log()
	}
	for _, e := range errs {
		if e != nil {
			logging.Fluent(logger).Warn("Stale CAS cleanup reported error").
				WithError(e).
				Log()
		}
	}
	return StaleCASCleanupResult{KindsRun: kinds, FilesHandled: total}
}
