package system

import (
	"path/filepath"
	"strings"

	"github.com/zqk-os/zqk/pkg/datacell"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/storage"
	caspkg "github.com/zqk-os/zqk/pkg/storage/cas"
	"github.com/zqk-os/zqk/pkg/storage/filecas"
)

// StaleCASCleanupResult holds the result of running Stale CAS cleanup for check results.
type StaleCASCleanupResult struct {
	KindsRun     []string // Kinds that had cleanup run
	FilesHandled int      // Total files quarantined or deleted
}

// RunStaleCASCleanupForResults scans check results for "Stale CAS version" and duplicate CAS blob issues,
// collects distinct kinds, and runs hash-duplicates cleanup for those kinds so that
// check --auto-fix resolves Stale CAS and duplicate blobs in one run (see INTEGRITY_RESOLUTION_PLAN.md).
// It also scans for missing CAS files / stale index entries / read errors on missing files:
// attempting to re-link to an existing on-disk hash file via DiscoverCASFilePathByScanning, or removing
// the stale mapping if no on-disk file remains.
// Returns the kinds that were cleaned and the number of files quarantined/deleted.
func RunStaleCASCleanupForResults(projectRoot string, results []CheckResult, logger logging.Logger) StaleCASCleanupResult {
	kindsSet := make(map[string]bool)
	for _, r := range results {
		for _, issue := range r.Issues {
			isStaleOrDuplicate := strings.Contains(issue.Message, "Stale CAS version") ||
				strings.Contains(issue.Message, "Duplicate CAS blob") ||
				strings.Contains(issue.Message, "dual CAS blobs")
			if isStaleOrDuplicate && r.ObjectKind != emptyValue {
				kindsSet[r.ObjectKind] = true
			}
		}
	}

	// Also scan for any on-disk duplicate CAS blobs directly so auto-fix covers
	// dual blobs regardless of whether results already had appendCASDuplicateIDCheckResults run.
	if projectRoot != emptyValue {
		casDupInv := caspkg.InventoryCASDuplicateIDs(nil, projectRoot)
		for _, hit := range casDupInv.Hits {
			if hit.Kind != emptyValue {
				kindsSet[hit.Kind] = true
			} else if hit.Dir != emptyValue {
				if k := objects.GetKindFromDirectory(hit.Dir); k != emptyValue {
					kindsSet[k] = true
				}
			}
		}
	}

	// Also heal or prune any explicitly missing CAS file index entries or failed-to-read missing files
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
						// Attempt to re-link to any live on-disk hash file for this object ID
						discoveredPath, discoveredHash, scanErr := filecas.DiscoverCASFilePathByScanning(r.ObjectID, kindDir)
						if scanErr == nil && discoveredHash != "" && discoveredPath != "" {
							relDir, relErr := filepath.Rel(kindDir, filepath.Dir(discoveredPath))
							var bucketKey string
							if relErr == nil && relDir != "." && relDir != "" {
								bucketKey = relDir
							}
							var setErr error
							if bucketKey != "" {
								setErr = index.SetMapping(r.ObjectID, discoveredHash, bucketKey)
							} else {
								setErr = index.SetMapping(r.ObjectID, discoveredHash)
							}
							if setErr == nil {
								logging.Fluent(logger).Info("Re-linked stale CAS index mapping to discovered on-disk hash file").
									String("object_id", r.ObjectID).
									String("kind", r.ObjectKind).
									String("discovered_hash", discoveredHash).
									String("bucket_key", bucketKey).
									Log()
								continue
							}
						}

						// If no on-disk hash file exists, prune the dead mapping
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

	if projectRoot != emptyValue && total > 0 {
		ClearCASDuplicateIDInventoryCache(projectRoot)
	}

	// Reconcile and clear resolved duplicate issues from results so check output reflects auto-fix
	cleanedKinds := make(map[string]bool, len(kinds))
	for _, k := range kinds {
		cleanedKinds[k] = true
	}
	for i := range results {
		if cleanedKinds[results[i].ObjectKind] {
			var remainingIssues []Issue
			for _, iss := range results[i].Issues {
				if strings.Contains(iss.Message, "Duplicate CAS blob") ||
					strings.Contains(iss.Message, "dual CAS blobs") ||
					strings.Contains(iss.Message, "Stale CAS version") {
					results[i].AutoFixed = append(results[i].AutoFixed, "reconciled duplicate CAS blob and healed index")
				} else {
					remainingIssues = append(remainingIssues, iss)
				}
			}
			results[i].Issues = remainingIssues
		}
	}

	return StaleCASCleanupResult{KindsRun: kinds, FilesHandled: total}
}
