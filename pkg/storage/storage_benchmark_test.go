package storage_test

import (
	"context"
	"testing"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/storage"
)

// TODO: Expand BenchmarkStorageRead beyond the NoopObjectStorage stub to establish real I/O baselines
// against FileObjectStorage and ContentAddressableStorage providers.
func BenchmarkStorageRead(b *testing.B) {
	provider := storage.NewNoopObjectStorage()
	ctx := context.Background()
	secCtx := pkgctx.NewSystemSecurityContext()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = provider.Read(ctx, secCtx, "BENCH-OBJ-1")
	}
}
