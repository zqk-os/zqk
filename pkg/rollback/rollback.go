package rollback

import (
	"context"
	"time"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/errfmt"
	"github.com/lanceman/zqk/pkg/logging"
	"github.com/lanceman/zqk/pkg/storage"
)

// Capture records a rollback point with the given scope and states. If config.CaptureEnabled is false, returns without error and empty ID.
// getStates is called to obtain the object states to snapshot (e.g. read from storage for status-relevant graph).
// Returns the rollback point ID. After append, Retain is not called automatically — caller should call Retain periodically or after Capture.
func Capture(projectRoot, scopeType, scopeID string, getStates func() ([]ObjectState, error)) (pointID string, err error) {
	cfg := DefaultConfig()
	if !cfg.CaptureEnabled {
		return "", nil
	}
	states, err := getStates()
	if err != nil {
		return "", errfmt.Newf("rollback capture: get states").Wrap(err)
	}
	if len(states) == 0 {
		return "", nil
	}
	store, err := NewStore(projectRoot)
	if err != nil {
		return "", err
	}
	p := &RollbackPoint{
		ID:           "rb-" + time.Now().UTC().Format("20060102-150405.000000000"),
		Timestamp:    time.Now().UTC(),
		ScopeType:    scopeType,
		ScopeID:      scopeID,
		ObjectStates: states,
	}
	if err := store.Append(p); err != nil {
		return "", err
	}
	return p.ID, nil
}

// List returns metadata for rollback points, most recent last. Optionally limit to last n and/or within duration.
func List(projectRoot string, lastN int, withinDuration time.Duration) ([]Meta, error) {
	store, err := NewStore(projectRoot)
	if err != nil {
		return nil, err
	}
	metas, err := store.List()
	if err != nil {
		return nil, err
	}
	if withinDuration > 0 {
		cutoff := time.Now().UTC().Add(-withinDuration)
		var filtered []Meta
		for _, m := range metas {
			if !m.Timestamp.Before(cutoff) {
				filtered = append(filtered, m)
			}
		}
		metas = filtered
	}
	if lastN > 0 && len(metas) > lastN {
		metas = metas[len(metas)-lastN:]
	}
	return metas, nil
}

// Get returns a rollback point by ID.
func Get(projectRoot, pointID string) (*RollbackPoint, error) {
	store, err := NewStore(projectRoot)
	if err != nil {
		return nil, err
	}
	return store.Get(pointID)
}

// Apply reapplies the object states from the rollback point to storage (quick rollback path).
func Apply(ctx context.Context, projectRoot, pointID string, provider storage.ObjectStorageProvider) error {
	if ctx == nil {
		ctx = pkgctx.NewSystemContext()
	}
	p, err := Get(projectRoot, pointID)
	if err != nil || p == nil {
		return errfmt.Errorf("rollback point not found: %s", pointID)
	}
	secCtx := pkgctx.NewSystemSecurityContext()
	logger := logging.NewEventLogger(ctx)
	for _, obj := range p.ObjectStates {
		if err := provider.Update(ctx, secCtx, obj.ID, obj.State); err != nil {
			logger.LogDebug("rollback apply: update failed",
				logging.KindField(obj.Kind),
				logging.IDField(obj.ID),
				logging.Error(err))
			return errfmt.Errorf("rollback apply %s/%s: %w", obj.Kind, obj.ID, err)
		}
	}
	return nil
}

// ApplyReconstruct is the beyond-threshold rollback path: for each ref, reconstruct state at
// targetTimestamp from the change journal and apply. Use when the rollback point was pruned.
// refs must be the status-relevant graph for the scope (e.g. from lifecycle.RecomputeRefsFromScope).
func ApplyReconstruct(
	ctx context.Context,
	projectRoot string,
	targetTimestamp time.Time,
	refs []ObjectRef,
	provider storage.ObjectStorageProvider,
	logger logging.Logger,
) error {
	if ctx == nil {
		ctx = pkgctx.NewSystemContext()
	}
	if logger == nil {
		logger = logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
	}
	secCtx := pkgctx.NewSystemSecurityContext()
	for _, ref := range refs {
		if ref.Kind == emptyValue || ref.ID == emptyValue {
			continue
		}
		current, err := provider.Read(ctx, secCtx, ref.ID)
		if err != nil {
			return errfmt.Errorf("rollback reconstruct read %s/%s: %w", ref.Kind, ref.ID, err)
		}
		if current == nil {
			return errfmt.Errorf("rollback reconstruct: object %s not found", ref.ID)
		}
		reconstructed, err := storage.ReconstructStateAtTimestamp(ctx, provider, current, ref.ID, ref.Kind, targetTimestamp, logger)
		if err != nil {
			return errfmt.Errorf("rollback reconstruct %s/%s: %w", ref.Kind, ref.ID, err)
		}
		if err := provider.Update(ctx, secCtx, ref.ID, reconstructed); err != nil {
			return errfmt.Errorf("rollback reconstruct apply %s/%s: %w", ref.Kind, ref.ID, err)
		}
	}
	return nil
}

// Retain trims the rollback store to the configured keep count and duration.
func Retain(projectRoot string) error {
	cfg := DefaultConfig()
	store, err := NewStore(projectRoot)
	if err != nil {
		return err
	}
	return store.Retain(cfg.RetainCount, cfg.RetainDuration)
}
