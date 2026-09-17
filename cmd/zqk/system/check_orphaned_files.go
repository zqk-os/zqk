// Package system implements system diagnostic, check, and maintenance commands.
// TRACK: BLI-CEF-R17-SYSTEM-TRANCHE1-001, CRIT-CEF-R17-SYSTEM-TRANCHE1-001
package system

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	"github.com/lanceman/zqk/pkg/errfmt"
	"github.com/lanceman/zqk/pkg/kernelcas"
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/paths"
	"github.com/lanceman/zqk/pkg/storage"
	fileutil "github.com/lanceman/zqk/pkg/utils/fileutil"
)

func detectOrphanedFiles(cmd *cobra.Command, projectRoot string, results []CheckResult, autoFix bool) []CheckResult {
	if projectRoot == "" {
		return results
	}

	var isFullCheck bool
	if cmd != nil {
		args := cmd.Flags().Args()
		isFullCheck = len(args) == 0 || (len(args) == 1 && args[0] == "all")
	} else {
		isFullCheck = true
	}
	if !isFullCheck {
		return results
	}

	// Build map of tracked absolute file paths
	trackedPaths := make(map[string]bool)
	for _, r := range results {
		if r.FilePath != "" {
			trackedPaths[filepath.Clean(r.FilePath)] = true
		}
	}

	processDir := filepath.Join(projectRoot, paths.ProcessDir)
	var orphanedResults []CheckResult

	// Get all registered kinds and pre-filter existing directories
	kinds := objects.GetGlobalKindMapper().GetAllKinds()
	var kindsToCheck []string
	for _, kind := range kinds {
		// Skip high-volume, stream-backed, or automated kinds that do not participate in CAS index registry
		if kind == objects.KindAuditEvent || kind == objects.KindChangeJournalEntry || kind == objects.KindMcpSession || kind == objects.KindSchedulerJob || kind == "command_spec" || (len(kind) > 7 && kind[len(kind)-7:] == "_metric") {
			continue
		}

		dirName := objects.GetDirectoryFromKind(kind)
		if dirName == "" {
			continue
		}
		kindDir := filepath.Join(processDir, dirName)

		// Walk kind directory if it exists
		if _, err := fileutil.Stat(kindDir); err != nil {
			continue
		}
		kindsToCheck = append(kindsToCheck, kind)
	}

	if cmd != nil && len(kindsToCheck) > 0 {
		fmt.Fprintf(cmd.ErrOrStderr(), "Scanning CAS membrane for orphaned files across %d kinds...\n", len(kindsToCheck))
	}
	lastScanHeartbeat := time.Now()

	for idx, kind := range kindsToCheck {
		if cmd != nil && time.Since(lastScanHeartbeat) >= 1*time.Second {
			lastScanHeartbeat = time.Now()
			fmt.Fprintf(cmd.ErrOrStderr(), "Scanning CAS directories... (%d/%d kinds)\n", idx+1, len(kindsToCheck))
		}
		dirName := objects.GetDirectoryFromKind(kind)
		kindDir := filepath.Join(processDir, dirName)

		// Load CAS index mapping for this kind to prevent deleting valid CAS files
		var indexMappings map[string]string
		cas := storage.NewContentAddressableStorage(kindDir, kind)
		if cas != nil {
			idx := cas.GetIndex()
			if idx != nil {
				indexMappings = idx.Mappings
			}
		}

		diskHashCount := countCASHashYAMLFiles(kindDir)
		indexCount := len(indexMappings)
		// Incomplete/empty index + hash files on disk: do NOT treat unindexed files as
		// disposable orphans. That is how sole criteria/workstreams were wiped.
		// TRACK: BLI-1785723654802038000-b14064bc
		sparseIndex := casIndexSparseVersusDisk(indexCount, diskHashCount)
		if sparseIndex && diskHashCount > 0 {
			orphanedResults = append(orphanedResults, CheckResult{
				ObjectID:   kind,
				ObjectKind: kind,
				FilePath:   kindDir,
				Issues: []Issue{
					{
						Tier:     1,
						Category: "integrity",
						Message: fmt.Sprintf(
							"CAS index sparse for kind %s (index_mappings=%d disk_hash_files=%d): refusing destructive orphan cleanup; --auto-fix will reindex untracked hash files only",
							kind, indexCount, diskHashCount,
						),
						AutoFixable: false,
					},
				},
			})
		}

		// O(1) hash membership (value scan per file was O(index) and hid id-level drift).
		indexedHashes := make(map[string]struct{}, len(indexMappings))
		for _, h := range indexMappings {
			if h != "" {
				indexedHashes[h] = struct{}{}
			}
		}

		_ = filepath.Walk(kindDir, func(path string, info fileutil.FileInfo, err error) error {
			if err != nil {
				return nil
			}
			if info.IsDir() {
				name := info.Name()
				if name == "schemas" || name == "_internal" || strings.HasPrefix(name, ".") {
					return filepath.SkipDir
				}
				return nil
			}
			ext := strings.ToLower(filepath.Ext(path))
			if ext != ".yaml" && ext != ".yml" {
				return nil
			}
			filename := filepath.Base(path)
			if strings.HasPrefix(filename, ".") {
				return nil
			}

			cleanPath := filepath.Clean(path)
			if !trackedPaths[cleanPath] {
				// Check if this is a registered CAS file
				fileExt := filepath.Ext(filename)
				nameWithoutExt := strings.TrimSuffix(filename, fileExt)
				isCASHashName := hashFilePattern.MatchString(filename)
				if isCASHashName {
					if _, isRegistered := indexedHashes[nameWithoutExt]; isRegistered {
						return nil // Registered in CAS index, NOT orphaned!
					}
					// Index often lags updates: id still mapped to a deleted old hash while
					// the new hash file exists on disk. That is drift, not an orphan — Tier 1
					// "Orphaned file" made system check flap (0 ↔ hundreds) after bulk updates.
					// TRACK: BLI-1785723654802038000-b14064bc — remove when: CAS update always
					// durables the new hash before ACK and check discovery uses disk truth.
					if oid, idErr := extractObjectIDFromYAMLHead(cleanPath, idHeadBytes); idErr == nil && oid != "" {
						if indexedHash, ok := indexMappings[oid]; ok && indexedHash != nameWithoutExt {
							if autoFix {
								if recoveredID, recoverErr := reindexOrphanCASHashFile(kind, kindDir, cleanPath, nameWithoutExt, cas); recoverErr == nil && recoveredID != "" {
									orphanedResults = append(orphanedResults, CheckResult{
										ObjectID:   recoveredID,
										ObjectKind: kind,
										FilePath:   cleanPath,
										Issues:     nil,
										AutoFixed: []string{fmt.Sprintf(
											"reindexed CAS index drift: %s id=%s (was hash %s…)",
											filepath.Base(cleanPath), recoveredID, truncateHashPrefix(indexedHash, 8),
										)},
									})
									indexedHashes[nameWithoutExt] = struct{}{}
									indexMappings[oid] = nameWithoutExt
									return nil
								}
							}
							if sparseIndex {
								return nil // kind-level sparse warning already emitted
							}
							orphanedResults = append(orphanedResults, CheckResult{
								ObjectID:   oid,
								ObjectKind: kind,
								FilePath:   cleanPath,
								Issues: []Issue{
									{
										Tier:     4, // match integrity_check CAS index drift (not a blocking orphan)
										Category: "integrity",
										Message: fmt.Sprintf(
											"CAS index out of sync: disk file %s is current content for %s but index still maps to a different hash (recovery: --auto-fix reindexes)",
											filepath.Base(cleanPath), oid,
										),
										AutoFixable: true,
									},
								},
							})
							return nil
						}
					}
				}

				// Orphaned file found (not in check results paths and not in CAS index hash map).
				id := strings.TrimSuffix(filename, filepath.Ext(filename))

				if autoFix {
					// Prefer re-registering parseable CAS objects over deleting them.
					// Incomplete indexes falsely mark sole files as orphans; auto-delete wiped
					// criteria/workstreams during kernel repair (restore from git required).
					// TRACK: BLI-1785723654802038000-b14064bc — remove when: orphan auto-fix
					// always reindexes via a shared CAS reconcile API and never deletes sole files.
					if isCASHashName {
						if recoveredID, recoverErr := reindexOrphanCASHashFile(kind, kindDir, cleanPath, nameWithoutExt, cas); recoverErr == nil && recoveredID != "" {
							orphanedResults = append(orphanedResults, CheckResult{
								ObjectID:   recoveredID,
								ObjectKind: kind,
								FilePath:   cleanPath,
								Issues:     nil,
								AutoFixed:  []string{fmt.Sprintf("reindexed orphaned CAS file into index: %s (id=%s)", filepath.Base(cleanPath), recoveredID)},
							})
							return nil
						}
						// Refuse to delete a hash-shaped YAML we could not reindex — report for human/CLI repair.
						orphanedResults = append(orphanedResults, CheckResult{
							ObjectID:   id,
							ObjectKind: kind,
							FilePath:   cleanPath,
							Issues: []Issue{
								{
									Tier:        1,
									Category:    "integrity",
									Message:     "Orphaned CAS file on disk could not be reindexed (parse/register failed); not deleted to protect sole objects",
									AutoFixable: false,
								},
							},
						})
						return nil
					}
					// Sparse index: never delete traditional files either — discovery may be incomplete.
					if sparseIndex {
						orphanedResults = append(orphanedResults, CheckResult{
							ObjectID:   id,
							ObjectKind: kind,
							FilePath:   cleanPath,
							Issues: []Issue{
								{
									Tier:        2,
									Category:    "integrity",
									Message:     "Untracked process file while CAS index is sparse; not deleted",
									AutoFixable: false,
								},
							},
						})
						return nil
					}
					// Never fileutil.RemoveFile from orphan detection. IsCoreKernelKind is false when the
					// spec index is not loaded, so a "non-core" branch previously wiped real
					// process YAML. Reindex CAS hashes above; traditional files need CLI repair.
					// TRACK: BLI-1785723654802038000-b14064bc
					orphanedResults = append(orphanedResults, CheckResult{
						ObjectID:   id,
						ObjectKind: kind,
						FilePath:   cleanPath,
						Issues: []Issue{
							{
								Tier:        1,
								Category:    "integrity",
								Message:     "Untracked traditional process file; not deleted (use zqk object delete --unlink-references or kernel.cas_object_reconcile_index)",
								AutoFixable: false,
							},
						},
					})
				} else {
					msg := "Orphaned file on disk: file exists in " + paths.ProcessDir + " but is not registered in the database index (recovery: --auto-fix reindexes CAS hash files; does not delete them)"
					if sparseIndex && isCASHashName {
						// Kind-level sparse warning already emitted; keep per-file quieter as informational.
						return nil
					}
					orphanedResults = append(orphanedResults, CheckResult{
						ObjectID:   id,
						ObjectKind: kind,
						FilePath:   cleanPath,
						Issues: []Issue{
							{
								Tier:        1,
								Category:    "integrity",
								Message:     msg,
								AutoFixable: false,
							},
						},
					})
				}
			}
			return nil
		})
	}

	return append(results, orphanedResults...)
}

// countCASHashYAMLFiles counts 64-hex hash-named YAML files under kindDir (non-recursive buckets included via Walk).
func countCASHashYAMLFiles(kindDir string) int {
	count := 0
	_ = filepath.Walk(kindDir, func(path string, info fileutil.FileInfo, err error) error {
		if err != nil || info == nil || info.IsDir() {
			return nil
		}
		if hashFilePattern.MatchString(info.Name()) {
			count++
		}
		return nil
	})
	return count
}

// casIndexSparseVersusDisk reports when the on-disk CAS population clearly exceeds the index,
// which historically caused mass false-orphan deletes.
func casIndexSparseVersusDisk(indexMappings, diskHashFiles int) bool {
	if diskHashFiles <= 0 {
		return false
	}
	if indexMappings == 0 {
		return true
	}
	// Index far behind disk (e.g. 20 mapped, 87 on disk after partial cache warm).
	if diskHashFiles >= indexMappings*2 && diskHashFiles-indexMappings >= 5 {
		return true
	}
	return false
}

func truncateHashPrefix(hash string, n int) string {
	if n <= 0 || hash == "" {
		return ""
	}
	if len(hash) <= n {
		return hash
	}
	return hash[:n]
}

// reindexOrphanCASHashFile registers a hash-named YAML into the kind CAS index when it parses
// as an object of that kind. Returns the object id on success.
// Runs under kernel.cas_object_reconcile_index (reindex only; never sole-delete).
// TRACK: BLI-1785784865905766000-dded0895
func reindexOrphanCASHashFile(kind, kindDir, filePath, hash string, cas *storage.ContentAddressableStorage) (string, error) {
	var recoveredID string
	err := kernelcas.RunReconcileIndex(context.Background(), nil, &kernelcas.Mutation{ // Background: request-or-shutdown derived
		Kind:   kind,
		ID:     hash,
		Intent: kernelcas.IntentReconcileIndex,
		CommitFn: func(_ context.Context) error {
			data, err := fileutil.ReadFile(filePath)
			if err != nil {
				return err
			}
			var obj map[string]any
			if err := yaml.Unmarshal(data, &obj); err != nil {
				return err
			}
			oid, _ := obj[objects.FieldKeyID].(string)
			okind, _ := obj[objects.FieldKeyKind].(string)
			if oid == "" || (okind != "" && okind != kind) {
				return errfmt.Errorf("orphan CAS file missing id or kind mismatch")
			}
			localCAS := cas
			if localCAS == nil {
				localCAS = storage.NewContentAddressableStorage(kindDir, kind)
			}
			idx := localCAS.GetIndex()
			if idx == nil {
				return errfmt.Errorf("CAS index unavailable for kind %s", kind)
			}
			relDir, relErr := filepath.Rel(kindDir, filepath.Dir(filePath))
			var bucketKey string
			if relErr == nil && relDir != "." && relDir != "" {
				bucketKey = relDir
			}
			var setErr error
			if bucketKey != "" {
				setErr = idx.SetMapping(oid, hash, bucketKey)
			} else {
				setErr = idx.SetMapping(oid, hash)
			}
			if setErr != nil {
				return setErr
			}
			recoveredID = oid
			return nil
		},
	})
	return recoveredID, err
}
