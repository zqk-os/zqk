package context

import stdcontext "context"

// ValidationProgressFunc is called at validation stage boundaries (e.g. "spec", "lifecycle")
// so the CLI can emit progress. Stage and message are short labels for user/logs.
type ValidationProgressFunc func(stage, message string)

type validationProgressKey struct{}

// WithValidationProgress attaches a progress callback to the context.
// Used by the async CLI pattern to report validation stages (spec, lifecycle).
func WithValidationProgress(ctx stdcontext.Context, fn ValidationProgressFunc) stdcontext.Context {
	if ctx == nil || fn == nil {
		return ctx
	}
	return stdcontext.WithValue(ctx, validationProgressKey{}, fn)
}

// GetValidationProgress returns the progress callback if set; otherwise nil.
func GetValidationProgress(ctx stdcontext.Context) ValidationProgressFunc {
	if ctx == nil {
		return nil
	}
	if fn, ok := ctx.Value(validationProgressKey{}).(ValidationProgressFunc); ok {
		return fn
	}
	return nil
}
