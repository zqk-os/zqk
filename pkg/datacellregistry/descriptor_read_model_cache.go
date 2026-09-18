package datacellregistry

import (
	"path/filepath"
	"sync"

	"github.com/zqk-os/zqk/pkg/concurrency"
	"github.com/zqk-os/zqk/pkg/datacell"
	"github.com/zqk-os/zqk/pkg/errfmt"
)

// projectDescriptorCaches maps project root → per-process cache of [datacell.DescriptorReadModel].
// The CLI and long-lived daemons reuse entries; tests with distinct temp roots do not collide.
var projectDescriptorCaches sync.Map

type descriptorReadModelCache struct {
	mu   sync.Mutex
	root string
	snap *datacell.DescriptorReadModel
}

// normalizeProjectRootKey is the sync.Map key for descriptor caches so Abs-equivalent roots
// (e.g. "/proj" vs "/proj/.") dedupe. See registry_test.go TestInvalidateDescriptorReadModelCache_equivalentProjectRoots.
func normalizeProjectRootKey(root string) string {
	if root == "" {
		return ""
	}
	abs, err := filepath.Abs(root)
	if err != nil {
		return filepath.Clean(root)
	}
	return abs
}

func descriptorCacheForProject(root string) *descriptorReadModelCache {
	if root == "" {
		return &descriptorReadModelCache{root: root}
	}
	key := normalizeProjectRootKey(root)
	v, _ := projectDescriptorCaches.LoadOrStore(key, &descriptorReadModelCache{root: root})
	return v.(*descriptorReadModelCache)
}

// InvalidateDescriptorReadModelCache drops the in-process descriptor read model for projectRoot
// so the next [DescriptorReadModelForProject] reloads from disk. Call after materializing
// spec_index.json (e.g. generate-spec-index, update-specs refresh) so data-cell snapshots
// stay aligned with on-disk index revisions. Any string that [filepath.Abs] resolves like
// projectRoot clears the same entry (see [normalizeProjectRootKey]).
func InvalidateDescriptorReadModelCache(projectRoot string) {
	if projectRoot == "" {
		return
	}
	projectDescriptorCaches.Delete(normalizeProjectRootKey(projectRoot))
}

// DescriptorReadModelForProject returns a [datacell.DescriptorReadModel] for projectRoot.
// When currentSpecCacheRevision is 0, loads via [LoadDescriptorReadModel] without cross-call
// memoization (early init / tests). When non-zero, returns a cached snapshot while
// [datacell.ReadModelIsStale] is false against that revision; otherwise reloads under lock
// ([concurrency.RunInLock]).
func DescriptorReadModelForProject(projectRoot string, currentSpecCacheRevision uint64) (*datacell.DescriptorReadModel, error) {
	if projectRoot == "" {
		return nil, errfmt.Errorf("project root is required")
	}
	if currentSpecCacheRevision == 0 {
		return LoadDescriptorReadModel(projectRoot)
	}
	return descriptorCacheForProject(projectRoot).snapshot(currentSpecCacheRevision)
}

func (c *descriptorReadModelCache) snapshot(current uint64) (*datacell.DescriptorReadModel, error) {
	var out *datacell.DescriptorReadModel
	err := concurrency.RunInLock(&c.mu, func() error {
		if c.snap != nil && !datacell.ReadModelIsStale(c.snap, current) {
			out = c.snap
			return nil
		}
		snap, loadErr := LoadDescriptorReadModel(c.root)
		if loadErr != nil {
			return loadErr
		}
		c.snap = snap
		out = snap
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}
