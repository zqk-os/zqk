package scheduler

import (
	"context"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/objects"
	storagepkg "github.com/lanceman/zqk/pkg/storage"
)

// ConvergenceStorageAdapter implements ConvergenceStorageProvider using ObjectStorageProvider
type ConvergenceStorageAdapter struct {
	storage storagepkg.ObjectStorageProvider
}

func (a *ConvergenceStorageAdapter) ListConvergenceSessions(ctx context.Context) ([]map[string]any, error) {
	secCtx := pkgctx.NewSystemSecurityContext()
	res, err := a.storage.List(ctx, secCtx, nil, storagepkg.ListFilter{
		Kind: objects.KindConvergenceSession,
	})
	if err != nil {
		return nil, err
	}
	return res.Objects, nil
}

func (a *ConvergenceStorageAdapter) UpdateSessionStatus(ctx context.Context, id, status string) error {
	secCtx := pkgctx.NewSystemSecurityContext()
	return a.storage.Update(ctx, secCtx, id, map[string]any{
		objects.FieldKeyStatus: status,
	})
}

func (a *ConvergenceStorageAdapter) CreatePriorityPlanForTimeout(ctx context.Context, sessionID string) error {
	secCtx := pkgctx.NewSystemSecurityContext()
	plan := map[string]any{
		objects.FieldKeyKind:        objects.KindPriorityPlan,
		objects.FieldKeyStatus:      objects.ObjectStatusGrooming,
		objects.FieldKeyCategory:    "Re-Alignment",
		objects.FieldKeyTitle:       "Drift-Control: Session " + sessionID + " Timeout",
		objects.FieldKeyDescription: "Convergence session exceeded 1 hour timeout. Automated re-alignment required.",
	}
	if err := a.storage.Create(ctx, secCtx, plan); err != nil {
		return err
	}
	if planID, _ := plan[objects.FieldKeyID].(string); planID != "" {
		return a.storage.Update(ctx, secCtx, planID, map[string]any{
			objects.FieldKeyStatus: objects.ObjectStatusActive,
		})
	}
	return nil
}
