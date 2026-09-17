package cli

import (
	"context"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/errfmt"
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/storage"
)

// SemanticStorageDecorator wraps an ObjectStorageProvider to enforce
// Persona-Aware Context Filtering (Vocabulary Scheme Graph Traversal).
type SemanticStorageDecorator struct {
	storage.ObjectStorageProvider
}

// UnderlyingObjectStorageProvider returns the wrapped provider for file/CAS unwrap
// (state-restore WriteObjectRaw). TRACK: BLI-REDACTED
func (d *SemanticStorageDecorator) UnderlyingObjectStorageProvider() storage.ObjectStorageProvider {
	if d == nil {
		return nil
	}
	return d.ObjectStorageProvider
}

// isKindAllowed checks if the given kind is semantically allowed for the active vocabulary schemes.
func (d *SemanticStorageDecorator) isKindAllowed(ctx context.Context, secCtx *pkgctx.SecurityContext, kind string) bool {
	if secCtx == nil || len(secCtx.ActiveVocabularySchemes) == 0 {
		return true // No semantic filtering applied
	}

	// Always allow core structural and orchestrator objects to prevent breaking CLI functionality
	if kind == objects.KindStrategicPlan || kind == objects.KindPriorityPlan || kind == objects.KindPersona || kind == objects.KindVocabularyScheme || kind == objects.KindRole || kind == objects.KindAccount {
		return true
	}

	systemSecCtx := pkgctx.NewSystemSecurityContext()
	for _, vocabRef := range secCtx.ActiveVocabularySchemes {
		vocabObj, err := d.ObjectStorageProvider.Read(ctx, systemSecCtx, vocabRef)
		if err == nil {
			allowedAny, ok := vocabObj[objects.FieldKeyAllowedKinds].([]any)
			if ok {
				for _, a := range allowedAny {
					if aStr, okStr := a.(string); okStr && aStr == kind {
						return true
					}
				}
			} else if allowedStrs, ok2 := vocabObj[objects.FieldKeyAllowedKinds].([]string); ok2 {
				for _, aStr := range allowedStrs {
					if aStr == kind {
						return true
					}
				}
			}
		}
	}
	return false
}

func (d *SemanticStorageDecorator) Read(ctx context.Context, secCtx *pkgctx.SecurityContext, id string) (map[string]any, error) {
	obj, err := d.ObjectStorageProvider.Read(ctx, secCtx, id)
	if err != nil {
		return nil, err
	}
	kind, _ := obj[objects.FieldKeyKind].(string)
	if !d.isKindAllowed(ctx, secCtx, kind) {
		return nil, errfmt.Errorf("semantic filtering: object %s (kind: %s) is out of scope for active persona", id, kind)
	}
	return obj, nil
}

func (d *SemanticStorageDecorator) List(ctx context.Context, secCtx *pkgctx.SecurityContext, storageCtx *pkgctx.StorageContext, filter storage.ListFilter) (*storage.QueryResult, error) {
	// If a specific kind is requested and it's not allowed, fast-fail
	if filter.Kind != "" && !d.isKindAllowed(ctx, secCtx, filter.Kind) {
		return &storage.QueryResult{Objects: []map[string]any{}}, nil
	}

	res, err := d.ObjectStorageProvider.List(ctx, secCtx, storageCtx, filter)
	if err != nil {
		return nil, err
	}

	// Filter out objects that violate the semantic scope
	if secCtx != nil && len(secCtx.ActiveVocabularySchemes) > 0 {
		var filtered []map[string]any
		for _, obj := range res.Objects {
			kind, _ := obj[objects.FieldKeyKind].(string)
			if d.isKindAllowed(ctx, secCtx, kind) {
				filtered = append(filtered, obj)
			}
		}
		res.Objects = filtered
	}

	return res, nil
}

func (d *SemanticStorageDecorator) Query(ctx context.Context, secCtx *pkgctx.SecurityContext, storageCtx *pkgctx.StorageContext, query storage.Query) (*storage.QueryResult, error) {
	res, err := d.ObjectStorageProvider.Query(ctx, secCtx, storageCtx, query)
	if err != nil {
		return nil, err
	}

	if secCtx != nil && len(secCtx.ActiveVocabularySchemes) > 0 {
		var filtered []map[string]any
		for _, obj := range res.Objects {
			kind, _ := obj[objects.FieldKeyKind].(string)
			if d.isKindAllowed(ctx, secCtx, kind) {
				filtered = append(filtered, obj)
			}
		}
		res.Objects = filtered
	}

	return res, nil
}
