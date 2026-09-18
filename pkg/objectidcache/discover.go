package objectidcache

import (
	"context"
	"path/filepath"
	"time"

	"github.com/zqk-os/zqk/pkg/goroutinelabels"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/storage"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

func discoverObjectKinds(processDir string) []string {
	var kinds []string
	fieldRegistry := objects.GetGlobalFieldRegistry()
	if err := fieldRegistry.LoadFields(); err != nil {
		return kinds
	}
	allKinds, err := fieldRegistry.GetAllKinds()
	if err != nil {
		return kinds
	}
	kinds = allKinds

	internalKinds := map[string]bool{
		objects.KindBaseMetric: true,
	}

	filteredKinds := make([]string, 0, len(kinds))
	for _, kind := range kinds {
		if internalKinds[kind] || storage.IsHighVolumeKindForCache(kind) {
			continue
		}
		dirName := objects.GetDirectoryFromKind(kind)
		if dirName != "" {
			dirPath := filepath.Join(processDir, dirName)
			if _, err := fileutil.Stat(dirPath); fileutil.IsNotExist(err) {
				continue
			}
		}
		filteredKinds = append(filteredKinds, kind)
	}
	return filteredKinds
}

// discoverObjectKindsWithContext bounds field-registry discovery so BuildCache cannot hang indefinitely.
func discoverObjectKindsWithContext(ctx context.Context, processDir string) []string {
	const defaultDiscoverTimeout = 30 * time.Second
	timeout := defaultDiscoverTimeout
	if ctx != nil {
		if deadline, ok := ctx.Deadline(); ok {
			if d := time.Until(deadline); d > 0 && d < timeout {
				timeout = d
			}
		}
	}
	type result struct{ kinds []string }
	done := make(chan result, 1)
	goroutinelabels.NewGoroutine("objectidcache-discover", "discover object kinds").StartSimple(func() {
		done <- result{kinds: discoverObjectKinds(processDir)}
	})
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	if ctx != nil {
		select {
		case r := <-done:
			return r.kinds
		case <-timer.C:
			return nil
		case <-ctx.Done():
			return nil
		}
	}
	select {
	case r := <-done:
		return r.kinds
	case <-timer.C:
		return nil
	}
}
