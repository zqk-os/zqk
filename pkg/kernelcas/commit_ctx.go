package kernelcas

import "context"

type commitKey struct{}

// WithCommit marks ctx as inside pipeline COMMIT so storage may run the
// underlying mutate without re-entering Run*.
func WithCommit(ctx context.Context) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, commitKey{}, true)
}

// IsCommit reports whether ctx is inside a kernelcas COMMIT stage.
func IsCommit(ctx context.Context) bool {
	if ctx == nil {
		return false
	}
	v, _ := ctx.Value(commitKey{}).(bool)
	return v
}
