package system

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"sync"

	caspkg "github.com/zqk-os/zqk/pkg/storage/cas"

	"github.com/spf13/cobra"
)

// Tier-1 dual-CAS detection in system check output.
// print-time peek of every CAS blob; skip in
// --fast and memoize per process+root so JSON/YAML/table siblings do not rescan.

func inventoryCASDuplicateIDs(projectRoot string) caspkg.CASDuplicateIDInventory {
	return caspkg.InventoryCASDuplicateIDs(context.Background(), projectRoot)
}

var (
	casDupInvMemoMu sync.Mutex
	casDupInvMemo   = map[string]caspkg.CASDuplicateIDInventory{}
)

func inventoryCASDuplicateIDsCached(projectRoot string) caspkg.CASDuplicateIDInventory {
	casDupInvMemoMu.Lock()
	defer casDupInvMemoMu.Unlock()
	if inv, ok := casDupInvMemo[projectRoot]; ok {
		return inv
	}
	inv := inventoryCASDuplicateIDs(projectRoot)
	casDupInvMemo[projectRoot] = inv
	return inv
}

// ClearCASDuplicateIDInventoryCache clears the memoized inventory for a project root,
// forcing subsequent checks to rescan after CAS duplicate reconciliation.
func ClearCASDuplicateIDInventoryCache(projectRoot string) {
	casDupInvMemoMu.Lock()
	defer casDupInvMemoMu.Unlock()
	delete(casDupInvMemo, projectRoot)
}

// ClearAllCASDuplicateIDInventoryCaches clears all memoized inventories.
func ClearAllCASDuplicateIDInventoryCaches() {
	casDupInvMemoMu.Lock()
	defer casDupInvMemoMu.Unlock()
	casDupInvMemo = map[string]caspkg.CASDuplicateIDInventory{}
}

// inventoryCASDuplicateIDsForOutput is the print-path entry. Reduced-surface (--fast /
// --check-refs=false) skips the walk: validation already covered the objects it loaded,
// and a second full CAS peek is not an authoritative dual-blob verdict on that surface.
func inventoryCASDuplicateIDsForOutput(cmd *cobra.Command, projectRoot string) caspkg.CASDuplicateIDInventory {
	if checkFastModeEnabled(cmd) {
		return caspkg.CASDuplicateIDInventory{}
	}
	return inventoryCASDuplicateIDsCached(projectRoot)
}

// casDuplicateQuarantineCommand renders the remediation this check suggests.
//
// cleanup-duplicates takes an optional registered kind and rejects anything else, so a hit whose
// directory maps to no kind must be sent to the all-kinds form rather than handed a directory name.
// Naming the directory is what made the suggested fix fail with `unknown kind "scheduler_jobs"`: an
// unrunnable instruction is worse than a broader one, because the reader cannot tell whether the
// tool or the diagnosis is wrong.
func casDuplicateQuarantineCommand(kind string) string {
	if kind == "" {
		return "zqk system cleanup-duplicates --hash-duplicates"
	}
	return fmt.Sprintf("zqk system cleanup-duplicates %s --hash-duplicates", kind)
}

// appendCASDuplicateIDCheckResults adds one Tier-1 registration issue per duplicated object id
// so summary + results_by_kind surface POL-CODE-004 dual blobs (cache-blind otherwise).
// It ensures finding rows are deduplicated by object ID (CRIT-1786695439226552000-f1a93ee6).
func appendCASDuplicateIDCheckResults(results []CheckResult, inv caspkg.CASDuplicateIDInventory) []CheckResult {
	if inv.DuplicateCount == 0 {
		return results
	}

	existingByID := make(map[string]int, len(results))
	for i, r := range results {
		if r.ObjectID != "" {
			existingByID[r.ObjectID] = i
		}
	}

	for _, hit := range inv.Hits {
		losers := make([]string, 0, len(hit.Paths))
		for _, p := range hit.Paths {
			if p != hit.KeeperPath {
				losers = append(losers, p)
			}
		}
		msg := fmt.Sprintf(
			"Duplicate CAS blob for object ID '%s' (POL-CODE-004): %d hash files. Keeper (newest mtime): %s. Losers: %s. Quarantine with: %s",
			hit.ObjectID,
			len(hit.Paths),
			filepath.Base(hit.KeeperPath),
			basenameList(losers),
			casDuplicateQuarantineCommand(hit.Kind),
		)
		iss := Issue{
			Tier:        1,
			Category:    "registration",
			Message:     msg,
			AutoFixable: true,
			FixCommand:  casDuplicateQuarantineCommand(hit.Kind),
		}

		if idx, found := existingByID[hit.ObjectID]; found {
			results[idx].Issues = DedupeCheckResultIssues(append(results[idx].Issues, iss))
		} else {
			results = append(results, CheckResult{
				ObjectID:   hit.ObjectID,
				ObjectKind: hit.Kind,
				FilePath:   hit.KeeperPath,
				Issues:     []Issue{iss},
			})
			existingByID[hit.ObjectID] = len(results) - 1
		}
	}
	if rem := inv.DuplicateCount - len(inv.Hits); rem > 0 {
		results = append(results, CheckResult{
			ObjectID:   "CAS-DUPLICATE-ID-SCAN",
			ObjectKind: "system",
			FilePath:   "",
			Issues: []Issue{{
				Tier:        1,
				Category:    "registration",
				Message:     fmt.Sprintf("%d additional object id(s) with dual CAS blobs omitted from sample. Run: zqk system cleanup-duplicates --hash-duplicates", rem),
				AutoFixable: true,
				FixCommand:  "zqk system cleanup-duplicates --hash-duplicates",
			}},
		})
	}
	return results
}

// DedupeCheckResultIssues ensures each CheckResult has unique issues (by message and category)
// so system check output rows never emit duplicate messages for the same object.
func DedupeCheckResultIssues(issues []Issue) []Issue {
	if len(issues) <= 1 {
		return issues
	}
	seen := make(map[string]bool, len(issues))
	var deduped []Issue
	for _, iss := range issues {
		key := fmt.Sprintf("%d:%s:%s", iss.Tier, iss.Category, iss.Message)
		if !seen[key] {
			seen[key] = true
			deduped = append(deduped, iss)
		}
	}
	return deduped
}

func writeCASDuplicateIDSummary(buf *strings.Builder, inv caspkg.CASDuplicateIDInventory) {
	if inv.DuplicateCount == 0 {
		return
	}
	buf.WriteString("=== CAS duplicate IDs (POL-CODE-004; Tier-1) ===\n")
	fmt.Fprintf(buf, "❌ %d object id(s) have multiple hash-named CAS blobs (object-id cache keeps one path — filesystem scan is authoritative).\n", inv.DuplicateCount)
	fmt.Fprintf(buf, "   Fix: zqk system cleanup-duplicates --hash-duplicates  (quarantines losers under .zqk/system-health/quarantine/hash-duplicates/)\n")
	maxShow := 8
	if len(inv.Hits) < maxShow {
		maxShow = len(inv.Hits)
	}
	for _, hit := range inv.Hits[:maxShow] {
		fmt.Fprintf(buf, "      - %s (%s): %d blobs\n", hit.ObjectID, hit.Kind, len(hit.Paths))
	}
	if inv.DuplicateCount > maxShow {
		fmt.Fprintf(buf, "      … and %d more\n", inv.DuplicateCount-maxShow)
	}
	buf.WriteString("\n")
}

func basenameList(paths []string) string {
	if len(paths) == 0 {
		return "(none)"
	}
	parts := make([]string, 0, len(paths))
	for _, p := range paths {
		parts = append(parts, filepath.Base(p))
	}
	return strings.Join(parts, ", ")
}
