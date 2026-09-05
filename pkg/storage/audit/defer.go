package audit

import "context"

// DeferEventsKey is the context value key for deferring audit writes during bulk ops.
type DeferEventsKey struct{}

// HasDeferEvents reports whether ctx was wrapped by WithDeferEvents.
func HasDeferEvents(ctx context.Context) bool {
	if ctx == nil {
		return false
	}
	v, ok := ctx.Value(DeferEventsKey{}).(bool)
	return ok && v
}

// WithDeferEvents marks ctx so event creation should enqueue instead of writing immediately.
func WithDeferEvents(ctx context.Context) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, DeferEventsKey{}, true)
}
