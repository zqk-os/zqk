package filecas

import (
	"maps"
	"path/filepath"
	"regexp"

	"github.com/zqk-os/zqk/pkg/utils/fileutil"
)

// casDateBucketNameRe matches chrono bucket dirs (YYYY-MM or YYYY-MM-DD).
var casDateBucketNameRe = regexp.MustCompile(`^\d{4}-\d{2}(-\d{2})?$`)

// mergeCASIndexMaps merges a durable on-disk mapping snapshot with a same-process
// in-memory view. Disk wins when its hash file exists and memory's does not (stale
// process cache). Memory wins when its hash file exists and disk's does not (index
// lag after update / sync-cas-index). When both or neither exist, prefer disk so
// short-lived CLI caches cannot clobber a healed index; callers must re-apply the
// in-flight batch/explicit mapping afterward.
//
// TRACK: BLI-1785723654802038000-b14064bc — remove when: CAS index writers never
// overlay a full stale process map onto a fresher disk snapshot.
func MergeCASIndexMaps(kindDir string, disk, memory map[string]string) map[string]string {
	out := make(map[string]string, len(disk)+len(memory))
	if disk != nil {
		maps.Copy(out, disk)
	}
	for id, memHash := range memory {
		if id == "" || memHash == "" {
			continue
		}
		diskHash, onDisk := out[id]
		if !onDisk || diskHash == "" {
			out[id] = memHash
			continue
		}
		if diskHash == memHash {
			continue
		}
		out[id] = pickCASIndexHash(kindDir, diskHash, memHash)
	}
	return out
}

func pickCASIndexHash(kindDir, diskHash, memHash string) string {
	memExists := CasHashYAMLExists(kindDir, memHash)
	diskExists := CasHashYAMLExists(kindDir, diskHash)
	switch {
	case memExists && !diskExists:
		return memHash
	case diskExists && !memExists:
		return diskHash
	case memExists && diskExists:
		// Both blobs present (rare mid-update). Prefer disk so other processes'
		// sync-cas-index / heal is not reverted; in-flight batch re-apply restores
		// this process's authoritative updates.
		return diskHash
	default:
		// Neither file on disk — keep disk index value (may be pending delete).
		return diskHash
	}
}

func CasHashYAMLExists(kindDir, hash string) bool {
	if kindDir == "" || hash == "" {
		return false
	}
	name := hash + ".yaml"
	if _, err := fileutil.Stat(filepath.Join(kindDir, name)); err == nil {
		return true
	}
	// Chrono-bucketed kinds (scheduler_job, qa_success, …) store under YYYY-MM/.
	entries, err := fileutil.ReadDir(kindDir)
	if err != nil {
		return false
	}
	for _, entry := range entries {
		if !entry.IsDir() || !casDateBucketNameRe.MatchString(entry.Name()) {
			continue
		}
		if _, err := fileutil.Stat(filepath.Join(kindDir, entry.Name(), name)); err == nil {
			return true
		}
	}
	return false
}
