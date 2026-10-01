package internal

import (
	"sync"

	"github.com/zqk-os/zqk/pkg/cliapp"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/goroutinelabels"
	"github.com/zqk-os/zqk/pkg/storage"
)

func CountAllKindsParallel(proc *cli.Processor, storageProvider storage.ObjectStorageProvider, secCtx *pkgctx.SecurityContext, storageCtx *pkgctx.StorageContext, kinds []string, flags *CountFlags) (map[string]int, error) {
	counts := make(map[string]int)
	results := make([]int, len(kinds))
	var wg sync.WaitGroup
	errCh := make(chan error, len(kinds))

	for i, kind := range kinds {
		wg.Add(1)
		idx, k := i, kind
		goroutinelabels.NewGoroutine("refactor", "refactored").StartSimple(func() {
			defer wg.Done()
			count, err := countKind(proc, storageProvider, secCtx, storageCtx, k, flags, false)
			if err != nil {
				errCh <- err
				return
			}
			results[idx] = count
		})
	}

	wg.Wait()
	close(errCh)
	if len(errCh) > 0 {
		return nil, <-errCh
	}
	for i, k := range kinds {
		counts[k] = results[i]
	}
	return counts, nil
}
