package cas

import (
	"context"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/lanceman/zqk/pkg/datacell"
	"github.com/lanceman/zqk/pkg/errfmt"
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/paths"
	"github.com/lanceman/zqk/pkg/storage/filecas"
	"github.com/lanceman/zqk/pkg/storage/systemcheck"
	fileutil "github.com/lanceman/zqk/pkg/utils/fileutil"
)

// TRACK: BLI-1786358681981576000-66f07f6c — fail-closed against dual CAS blobs (POL-CODE-004).
// ObjectIDCache / index keep one path per id, so per-object checkDuplicateIDs is blind to
// multiple hash-named YAML files that embed the same id. Filesystem scan is authoritative.

const (
	casDuplicateQuarantineSubdir = "hash-duplicates"
	// Soft cap so pathological trees cannot blow check JSON; DuplicateCount remains exact.
	casDuplicateInventoryCap = 200
)

// CASDuplicateIDHit is one object id that appears in more than one CAS hash file under a kind dir.
//
// Dir and Kind are deliberately separate. The scan is directory-driven, and a directory name is not
// a kind: .zqk/process/scheduler_jobs holds scheduler_job, and .zqk/process/backlog holds
// backlog_item, which no amount of singularizing would produce. Reporting the directory as the kind
// put an unusable argument into the remediation command this inventory suggests, so a copy-paste of
// the fix answered `unknown kind "scheduler_jobs"`. Kind is empty when the directory maps to no
// registered kind; callers must then avoid naming a kind rather than substituting the directory.
type CASDuplicateIDHit struct {
	// Kind is the registered object kind, or empty when the directory maps to none.
	Kind string `json:"kind"`
	// Dir is the .zqk/process subdirectory that was scanned.
	Dir        string   `json:"dir,omitempty"`
	ObjectID   string   `json:"object_id"`
	Paths      []string `json:"paths"`
	KeeperPath string   `json:"keeper_path,omitempty"` // newest mtime; others are quarantine candidates
}

// CASDuplicateIDInventory is a project-level rollup for system check (Tier-1).
type CASDuplicateIDInventory struct {
	// DuplicateCount is the number of object ids with >1 on-disk CAS blob.
	DuplicateCount int                 `json:"duplicate_count"`
	Hits           []CASDuplicateIDHit `json:"hits,omitempty"`
}

// DefaultCASDuplicateQuarantineDir returns .zqk/system-health/quarantine/hash-duplicates.
func DefaultCASDuplicateQuarantineDir(projectRoot string) string {
	if projectRoot == "" {
		return ""
	}
	return filepath.Join(projectRoot, paths.ProjectDataDir, paths.SystemHealthDir, paths.QuarantineDir, casDuplicateQuarantineSubdir)
}

// InventoryCASDuplicateIDs scans .zqk/process/<kind> CAS hash files for duplicate embedded ids.
// Read-only; does not quarantine. Missing process dir → empty inventory.
func InventoryCASDuplicateIDs(ctx context.Context, projectRoot string) CASDuplicateIDInventory {
	inv := CASDuplicateIDInventory{}
	if projectRoot == "" {
		return inv
	}
	if ctx == nil {
		ctx = context.Background() // Background: request-or-shutdown derived
	}
	processDir := datacell.ProcessPrimaryDir(projectRoot)
	entries, err := fileutil.ReadDir(processDir)
	if err != nil {
		return inv
	}
	var hits []CASDuplicateIDHit
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		name := e.Name()
		if name == "" || name[0] == '_' || name[0] == '.' {
			continue
		}
		kindDir := filepath.Join(processDir, name)
		res, scanErr := systemcheck.RunSystemCheckForHandCASAndDupIDs(ctx, kindDir, name)
		if scanErr != nil || res == nil || len(res.Duplicates) == 0 {
			continue
		}
		kind := objects.GetKindFromDirectory(name)
		for _, dup := range res.Duplicates {
			hit := CASDuplicateIDHit{
				Kind:       kind,
				Dir:        name,
				ObjectID:   dup.ObjectID,
				Paths:      append([]string(nil), dup.Paths...),
				KeeperPath: pickNewestCASPath(dup.Paths),
			}
			sort.Strings(hit.Paths)
			hits = append(hits, hit)
		}
	}
	inv.DuplicateCount = len(hits)
	if len(hits) == 0 {
		return inv
	}
	// Order by Dir, not Kind: Kind is empty for directories that map to no registered kind, so
	// sorting on it would collapse every such directory into one indistinguishable run.
	sort.Slice(hits, func(i, j int) bool {
		if hits[i].Dir != hits[j].Dir {
			return hits[i].Dir < hits[j].Dir
		}
		return hits[i].ObjectID < hits[j].ObjectID
	})
	if len(hits) > casDuplicateInventoryCap {
		inv.Hits = hits[:casDuplicateInventoryCap]
	} else {
		inv.Hits = hits
	}
	return inv
}

// QuarantineCASDuplicateLosers keeps the newest-mtime CAS blob per duplicated id and moves
// older siblings into quarantineDir/<kind>/. dryRun only reports what would move (quarantined count).
func QuarantineCASDuplicateLosers(ctx context.Context, kind, kindDir, quarantineDir string, dryRun bool) (quarantined int, errs []error) {
	if kindDir == "" || kind == "" || quarantineDir == "" {
		return 0, []error{errfmt.Errorf("kind, kindDir, and quarantineDir are required")}
	}
	if ctx == nil {
		ctx = context.Background() // Background: request-or-shutdown derived
	}
	res, err := systemcheck.RunSystemCheckForHandCASAndDupIDs(ctx, kindDir, kind)
	if err != nil {
		return 0, []error{err}
	}
	if res == nil || len(res.Duplicates) == 0 {
		return 0, nil
	}
	for _, dup := range res.Duplicates {
		keep := pickNewestCASPath(dup.Paths)
		if keep == "" {
			continue
		}
		for _, p := range dup.Paths {
			if p == keep {
				continue
			}
			if _, stErr := fileutil.Stat(p); stErr != nil {
				continue
			}
			if dryRun {
				quarantined++
				continue
			}
			if qErr := moveCASFileToQuarantine(p, quarantineDir, kind); qErr != nil {
				errs = append(errs, qErr)
				continue
			}
			quarantined++
		}
		if !dryRun {
			keepBase := filepath.Base(keep)
			keepHash := strings.TrimSuffix(keepBase, filepath.Ext(keepBase))
			if len(keepHash) == 64 {
				cas := filecas.NewContentAddressableStorage(kindDir, kind)
				if idx := cas.GetIndex(); idx != nil {
					relDir, relErr := filepath.Rel(kindDir, filepath.Dir(keep))
					var bucketKey string
					if relErr == nil && relDir != "." && relDir != "" {
						bucketKey = relDir
					}
					if bucketKey != "" {
						_ = idx.SetMapping(dup.ObjectID, keepHash, bucketKey)
					} else {
						_ = idx.SetMapping(dup.ObjectID, keepHash)
					}
				}
			}
		}
	}
	return quarantined, errs
}

func pickNewestCASPath(paths []string) string {
	keep := ""
	var keepMTime time.Time
	for _, p := range paths {
		fi, err := fileutil.Stat(p)
		if err != nil {
			continue
		}
		if keep == "" || fi.ModTime().After(keepMTime) {
			keep = p
			keepMTime = fi.ModTime()
		}
	}
	return keep
}

func moveCASFileToQuarantine(src, quarantineRoot, kind string) error {
	destDir := filepath.Join(quarantineRoot, kind)
	if err := fileutil.MkdirAll(destDir, paths.DirPerm755); err != nil {
		return errfmt.Errorf("mkdir quarantine %s: %w", destDir, err)
	}
	base := filepath.Base(src)
	dest := filepath.Join(destDir, base)
	if _, err := fileutil.Stat(dest); err == nil {
		dest = filepath.Join(destDir, time.Now().UTC().Format("20060102T150405Z")+"_"+base)
	}
	if err := fileutil.Rename(src, dest); err != nil {
		return errfmt.Errorf("quarantine %s → %s: %w", src, dest, err)
	}
	return nil
}

// QuarantineCASHashFile moves one CAS hash YAML into quarantineRoot/<kind>/.
func QuarantineCASHashFile(src, quarantineRoot, kind string) error {
	return moveCASFileToQuarantine(src, quarantineRoot, kind)
}

// CASDuplicateIDInventoryPtr returns nil when there are no duplicate ids (omit from JSON).
func CASDuplicateIDInventoryPtr(inv CASDuplicateIDInventory) *CASDuplicateIDInventory {
	if inv.DuplicateCount == 0 {
		return nil
	}
	out := inv
	return &out
}
