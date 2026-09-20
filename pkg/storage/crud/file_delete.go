package crud

import "context"

type contextKeySuppressHashReg struct{}

func WithSuppressHashRegistryUpdate(ctx context.Context) context.Context {
	return context.WithValue(ctx, contextKeySuppressHashReg{}, true)
}

func ShouldSuppressHashRegistryUpdate(ctx context.Context) bool {
	val := ctx.Value(contextKeySuppressHashReg{})
	if b, ok := val.(bool); ok {
		return b
	}
	return false
}
