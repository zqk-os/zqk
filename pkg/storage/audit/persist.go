package audit

import (
	"context"
	"errors"
	"strings"

	pkgctx "github.com/lanceman/zqk/pkg/context"
)

// PersistResult is the outcome of PersistWithIDRetry.
type PersistResult struct {
	Instance      map[string]any
	Err           error
	AlreadyExists bool
	Retried       bool
}

// IsAlreadyExists reports whether err is a duplicate-create (sentinel or needle).
func IsAlreadyExists(err, sentinel error, needle string) bool {
	if err == nil {
		return false
	}
	if sentinel != nil && errors.Is(err, sentinel) {
		return true
	}
	return needle != "" && strings.Contains(err.Error(), needle)
}

// PersistWithIDRetry creates instance via store. On duplicate, retryBuild may
// supply a new map (new ID). Persistence stays on EventStore; ID allocation
// stays with the caller.
func PersistWithIDRetry(
	ctx context.Context,
	store EventStore,
	secCtx *pkgctx.SecurityContext,
	instance map[string]any,
	isAlreadyExists func(error) bool,
	retryBuild func() (map[string]any, error),
) PersistResult {
	out := PersistResult{Instance: instance}
	if store == nil || instance == nil {
		return out
	}
	err := store.Create(ctx, secCtx, instance)
	if err == nil {
		return out
	}
	out.Err = err
	if isAlreadyExists == nil || !isAlreadyExists(err) {
		return out
	}
	out.AlreadyExists = true
	if retryBuild == nil {
		return out
	}
	retryInstance, retryErr := retryBuild()
	if retryErr != nil || retryInstance == nil {
		return out
	}
	out.Retried = true
	out.Instance = retryInstance
	err2 := store.Create(ctx, secCtx, retryInstance)
	if err2 == nil {
		out.Err = nil
		out.AlreadyExists = false
		return out
	}
	out.Err = err2
	out.AlreadyExists = isAlreadyExists(err2)
	return out
}
