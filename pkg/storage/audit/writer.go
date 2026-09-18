package audit

import (
	"context"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
)

// EventStore is the persistence slice an audit writer needs. FileObjectStorage
// implements this via Create; the audit package does not import storage.
type EventStore interface {
	Create(ctx context.Context, secCtx *pkgctx.SecurityContext, obj map[string]any) error
}

// Writer persists one audit event. FileObjectStorage implements this; aggregation
// stays on Aggregator (a service, not the file store).
type Writer interface {
	WriteEvent(ctx context.Context, projectRoot string, secCtx *pkgctx.SecurityContext, options *EventOptions) error
}
