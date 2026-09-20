package storage

import "context"

type unlinkRefsBeforeDeleteCtxKey struct{}

// WithUnlinkReferencesBeforeDelete marks delete so dependents are updated to drop references
// to the deleted ID (CASCADE_NULLIFY / set-null semantics) instead of failing or recursively deleting.
func WithUnlinkReferencesBeforeDelete(ctx context.Context) context.Context {
	return context.WithValue(ctx, unlinkRefsBeforeDeleteCtxKey{}, true)
}

// UnlinkReferencesBeforeDelete reports whether ctx requests reference stripping before delete.
func UnlinkReferencesBeforeDelete(ctx context.Context) bool {
	v, _ := ctx.Value(unlinkRefsBeforeDeleteCtxKey{}).(bool)
	return v
}
