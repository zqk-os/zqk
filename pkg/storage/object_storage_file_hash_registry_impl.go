package storage

import (
	"context"
	"strings"

	"github.com/lanceman/zqk/pkg/concurrency"
	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/errfmt"
	"github.com/lanceman/zqk/pkg/logging"
	// hashRegistryCacheKey returns a cache key for (kind, dir). NUL is used as delimiter
	// so keys are unique (kind and dir cannot contain NUL).
)

func hashRegistryCacheKey(kind, dir string) string {
	return kind + "\x00" + dir
}

// newHashRegistry returns a hash registry for the given (kind, dir), reusing a cached
// instance per (kind, dir) to avoid spawning a save worker per create/update/delete/move.
// Registers with the global hash registry manager unless skipHashRegistryShutdownRegistration
// is set (test storage). Uses context.WithoutCancel(ctx) so the registry's save worker is
// not cancelled when the request context is cancelled.
// ctx: parent context from command entry point (should not be created here)
func (f *FileObjectStorage) newHashRegistry(ctx context.Context, kind, dir string) *HashRegistry {
	key := hashRegistryCacheKey(kind, dir)
	hr, err := f.hashRegistryCache.GetOrCreate(ctx, key, func(regCtx context.Context, cacheKey string) (*HashRegistry, error) {
		parts := strings.SplitN(cacheKey, "\x00", 2)
		if len(parts) != 2 {
			return nil, errfmt.Errorf(ConstStreamHashRegistryCacheKeyInvalidExpectedKind0dirQuote, cacheKey)
		}
		k, d := parts[0], parts[1]
		regCtx = context.WithoutCancel(regCtx)
		registry := NewHashRegistry(regCtx, k, d)
		registry.SetProjectRoot(f.projectRoot)
		registry.SetStorage(f)

		if f.skipHashRegistryShutdownRegistration {
			registry.SetSkipShutdownCoordinatorCheck(true)
			var _err_83389843 = concurrency.RunInLock(&f.unregisteredHashRegistriesMu, func() error {
				f.unregisteredHashRegistries = append(f.unregisteredHashRegistries, registry)
				return nil
			})
			if _err_83389843 != nil {
				logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).Error(ErrMsgSwallowedError, _err_83389843).Log()
			}
		} else {
			manager := GetGlobalHashRegistryManager()
			manager.RegisterRegistry(k, registry)
		}

		return registry, nil
	})
	if err != nil {
		return nil
	}
	return hr
}
