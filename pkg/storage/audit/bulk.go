package audit

import (
	"context"

	pkgctx "github.com/lanceman/zqk/pkg/context"
)

// BulkStatusWriter archives (or otherwise status-patches) many objects.
// Implementations live in package storage so this package does not import BulkUpdateItem.
type BulkStatusWriter interface {
	UpdateStatus(ctx context.Context, secCtx *pkgctx.SecurityContext, ids []string, status string) (int, error)
}

// BulkDeleteOutcome is the audit-side view of a bulk delete (no BulkResult import).
type BulkDeleteOutcome struct {
	SuccessCount int
	FailureCount int
	FirstMessage string
	Optimized    bool
}

// AllFailed is true when every attempted delete failed.
func (o BulkDeleteOutcome) AllFailed() bool {
	return o.SuccessCount == 0 && o.FailureCount > 0
}

// WrapAllFailed is the generic BulkDelete fail-closed. The optimized file path
// returns SuccessCount without wrapping (historical behavior).
func (o BulkDeleteOutcome) WrapAllFailed() bool {
	return !o.Optimized && o.AllFailed()
}

// BulkDeleter deletes many objects. File-store implementations may use the
// optimized path; this package does not import FileObjectStorage.
type BulkDeleter interface {
	DeleteIDs(ctx context.Context, secCtx *pkgctx.SecurityContext, ids []string) (BulkDeleteOutcome, error)
}
