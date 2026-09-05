package objectrecord

import "context"

// Recorder records object CRUD against a command-execution (or similar) trace.
// Defined outside pkg/cli so pkg/storage can record without importing CLI.
type Recorder interface {
	RecordObjectCreated(objectID string)
	RecordObjectUpdated(objectID string)
	RecordObjectDeleted(objectID string)
}

type ctxKey struct{}

// WithRecorder stores r on ctx. A nil recorder is a no-op store.
func WithRecorder(ctx context.Context, r Recorder) context.Context {
	if ctx == nil || r == nil {
		return ctx
	}
	return context.WithValue(ctx, ctxKey{}, r)
}

// FromContext returns the recorder stored by WithRecorder, or nil.
func FromContext(ctx context.Context) Recorder {
	if ctx == nil {
		return nil
	}
	r, _ := ctx.Value(ctxKey{}).(Recorder)
	return r
}
