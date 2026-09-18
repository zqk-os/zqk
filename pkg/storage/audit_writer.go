package storage

import (
	"context"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/storage/audit"
)

// WriteEvent implements audit.Writer. Buffering, CAS, and metrics stay in
// CreateAuditEventWithBuilder so this package remains the persistence adapter.
func (f *FileObjectStorage) WriteEvent(ctx context.Context, projectRoot string, secCtx *pkgctx.SecurityContext, options *audit.EventOptions) error {
	if projectRoot == emptyValue && f != nil {
		projectRoot = f.GetProjectRoot()
	}
	return CreateAuditEventWithBuilder(ctx, projectRoot, secCtx, f, options)
}

var (
	_ audit.Writer     = (*FileObjectStorage)(nil)
	_ audit.EventStore = (*FileObjectStorage)(nil)
)
