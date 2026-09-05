package semantic

import (
	"context"
	"crypto/sha256"
	"fmt"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/errfmt"
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/storage"
	fileutil "github.com/lanceman/zqk/pkg/utils/fileutil"
	"github.com/lanceman/zqk/pkg/zqktime"
)

// SemanticReconciler identifies drift between external sources and internal specs.
type SemanticReconciler struct {
	store storage.ObjectStorageProvider
}

// NewSemanticReconciler creates a new SemanticReconciler.
func NewSemanticReconciler(store storage.ObjectStorageProvider) *SemanticReconciler {
	return &SemanticReconciler{
		store: store,
	}
}

// ReconcileResult represents the findings of a reconciliation run.
type ReconcileResult struct {
	ImportTrackingID string
	SourceFile       string
	Drifted          bool
	Error            error
}

// CheckDrift evaluates all import_tracking objects for source file changes.
func (r *SemanticReconciler) CheckDrift(ctx context.Context, secCtx *pkgctx.SecurityContext) ([]ReconcileResult, error) {
	storageCtx := pkgctx.NewStorageContext()

	// 1. List all import_tracking objects
	listResult, err := r.store.List(ctx, secCtx, storageCtx, storage.ListFilter{
		Kind: objects.KindImportTracking,
	})
	if err != nil {
		return nil, errfmt.Newf("failed to list import_tracking").Wrap(err)
	}

	results := make([]ReconcileResult, 0, len(listResult.Objects))

	for _, obj := range listResult.Objects {
		id, _ := obj[objects.FieldKeyID].(string)
		sourceFile, _ := obj[objects.FieldKeySourceFile].(string)
		storedHash, _ := obj[objects.FieldKeySourceHash].(string)
		storedMTime, _ := obj[objects.FieldKeySourceMtime].(string)

		if sourceFile == "" {
			continue
		}

		result := ReconcileResult{
			ImportTrackingID: id,
			SourceFile:       sourceFile,
		}

		// 2. MTime-First Validation
		info, err := fileutil.Stat(sourceFile)
		if err != nil {
			result.Error = errfmt.Newf("failed to stat source file").Wrap(err)
			results = append(results, result)
			continue
		}

		currentMTime := zqktime.FormatRFC3339UTC(info.ModTime())
		if storedMTime != "" && currentMTime == storedMTime {
			// File hasn't changed according to MTime, skip hashing
			results = append(results, result)
			continue
		}

		// 3. Calculate current hash (only if MTime differs or was missing)
		currentHash, err := calculateHash(sourceFile)
		if err != nil {
			result.Error = err
			results = append(results, result)
			continue
		}

		// 4. Compare Hash
		if currentHash != storedHash {
			result.Drifted = true
		}

		// 5. Update last_checked_at, source_hash, and source_mtime in storage
		updateData := map[string]any{
			objects.FieldKeyLastCheckedAt: zqktime.NowRFC3339UTC(),
			objects.FieldKeySourceHash:    currentHash,
			objects.FieldKeySourceMtime:   currentMTime,
		}

		if err := r.store.Update(ctx, secCtx, id, updateData); err != nil {
			result.Error = errfmt.Newf("failed to update import_tracking %s", id).Wrap(err)
		}

		results = append(results, result)
	}

	return results, nil
}

// ConvergenceResult represents the vitality of an active convergence session.
type ConvergenceResult struct {
	SessionID       string
	Title           string
	DeltaAssessment string
	TrendingAway    bool
}

// CheckConvergenceDrift identifies sessions that are trending away from their goals.
func (r *SemanticReconciler) CheckConvergenceDrift(ctx context.Context, secCtx *pkgctx.SecurityContext) ([]ConvergenceResult, error) {
	storageCtx := pkgctx.NewStorageContext()

	// List active convergence sessions
	listResult, err := r.store.List(ctx, secCtx, storageCtx, storage.ListFilter{
		Kind: objects.KindConvergenceSession,
		Filters: map[string]any{
			objects.FieldKeyStatus: "active",
		},
	})
	if err != nil {
		return nil, errfmt.Newf("failed to list convergence sessions").Wrap(err)
	}

	results := make([]ConvergenceResult, 0, len(listResult.Objects))
	for _, obj := range listResult.Objects {
		id, _ := obj[objects.FieldKeyID].(string)
		title, _ := obj[objects.FieldKeyTitle].(string)
		delta, _ := obj[objects.FieldKeyDeltaAssessment].(string)

		result := ConvergenceResult{
			SessionID:       id,
			Title:           title,
			DeltaAssessment: delta,
		}

		if delta == "trending_away" {
			result.TrendingAway = true
		}

		results = append(results, result)
	}

	return results, nil
}

func calculateHash(filePath string) (string, error) {
	data, err := fileutil.ReadFile(filePath)
	if err != nil {
		return "", err
	}
	hash := sha256.Sum256(data)
	return fmt.Sprintf("%x", hash), nil
}
