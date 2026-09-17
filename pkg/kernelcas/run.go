package kernelcas

import (
	"context"

	"github.com/lanceman/zqk/pkg/logging"
)

// RunErase runs kernel.cas_object_erase (DECIDE erase policy → COMMIT).
func RunErase(ctx context.Context, logger logging.Logger, m *Mutation) error {
	if m != nil && m.Intent == "" {
		m.Intent = IntentEraseLogical
	}
	return runStages(ctx, logger, KindErase, m, decideErase)
}

// RunReconcileIndex runs kernel.cas_object_reconcile_index (reindex only).
func RunReconcileIndex(ctx context.Context, logger logging.Logger, m *Mutation) error {
	if m != nil && m.Intent == "" {
		m.Intent = IntentReconcileIndex
	}
	return runStages(ctx, logger, KindReconcileIndex, m, decideReconcile)
}

// RunCreate runs kernel.cas_object_create.
func RunCreate(ctx context.Context, logger logging.Logger, m *Mutation) error {
	if m != nil && m.Intent == "" {
		m.Intent = IntentCreate
	}
	return runStages(ctx, logger, KindCreate, m, decideAllowCasSync)
}

// RunUpdate runs kernel.cas_object_update.
func RunUpdate(ctx context.Context, logger logging.Logger, m *Mutation) error {
	if m != nil && m.Intent == "" {
		m.Intent = IntentUpdateFields
	}
	return runStages(ctx, logger, KindUpdate, m, decideAllowCasSync)
}

// RunTransition runs kernel.cas_object_transition.
func RunTransition(ctx context.Context, logger logging.Logger, m *Mutation) error {
	if m != nil && m.Intent == "" {
		m.Intent = IntentTransition
	}
	return runStages(ctx, logger, KindTransition, m, decideAllowCasSync)
}

// RunRestoreMerge runs kernel.cas_object_restore_merge.
func RunRestoreMerge(ctx context.Context, logger logging.Logger, m *Mutation) error {
	if m != nil && m.Intent == "" {
		m.Intent = IntentRestoreMerge
	}
	return runStages(ctx, logger, KindRestoreMerge, m, decideAllowCasSync)
}

// RunBlobGC runs kernel.cas_blob_gc.
func RunBlobGC(ctx context.Context, logger logging.Logger, m *Mutation) error {
	if m != nil && m.Intent == "" {
		m.Intent = IntentGcBlob
	}
	return runStages(ctx, logger, KindBlobGC, m, decideBlobGC)
}
