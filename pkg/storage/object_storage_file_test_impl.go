package storage

import (
	"sync"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
)

var testIdentityCacheHandlerOnce sync.Once

// EnsureTestIdentityCacheHandler installs a succeeding no-op so hermetic tests ACK
// without the CLI wiring. Tests that need fail-closed nil-handler set the handler
// to nil after construction.
func EnsureTestIdentityCacheHandler() {
	ensureTestIdentityCacheHandler()
}

func ensureTestIdentityCacheHandler() {
	testIdentityCacheHandlerOnce.Do(func() {
		if cacheOperationHandler == nil {
			SetCacheOperationHandler(func(*pkgctx.CacheContext) error { return nil })
		}
	})
}
