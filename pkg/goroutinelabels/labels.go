package goroutinelabels

import (
	"context"
	"runtime/pprof"
)

const (
	labelKeyName    = "name"
	labelKeyPurpose = "purpose"
	emptyValue      = ""
)

// SetGoroutineLabel sets a label for the current goroutine that will appear in profiling
// This makes it easier to identify what work each goroutine is doing in profiler output
//
// The label will appear in CPU profiles and goroutine profiles, making it easier
// to identify which goroutines are doing what work in tools like pprof, sample, etc.
//
// Example:
//
//	go func() {
//	    SetGoroutineLabel("validation_worker_1", "processing validation queue")
//	    // ... work ...
//	}()
func SetGoroutineLabel(name, purpose string) {
	labels := pprof.Labels(labelKeyName, name)
	ctx := context.Background()
	ctx = pprof.WithLabels(ctx, labels)

	if purpose != emptyValue {
		// Add purpose label
		purposeLabels := pprof.Labels(labelKeyPurpose, purpose)
		ctx = pprof.WithLabels(ctx, purposeLabels)
	}

	pprof.SetGoroutineLabels(ctx)
}

// StartNamedGoroutine starts a goroutine with a meaningful name for profiling
// This is a convenience wrapper that sets the label and starts the goroutine
//
// Example:
//
//	StartNamedGoroutine("validation_worker_1", "processing validation queue", func() {
//	    // ... work ...
//	})
func StartNamedGoroutine(name, purpose string, fn func()) {
	go func() {
		SetGoroutineLabel(name, purpose)
		fn()
	}()
}

// DoWithLabels executes a function with goroutine labels set
// This is the recommended way to set labels as it ensures proper cleanup
//
// Example:
//
//	DoWithLabels("validation_worker_1", "processing validation queue", func() {
//	    // ... work ...
//	})
func DoWithLabels(name, purpose string, fn func()) {
	labels := pprof.Labels(labelKeyName, name)
	if purpose != emptyValue {
		// Create combined labels
		// Note: Using context.Background() here is acceptable (see SetGoroutineLabel for explanation)
		ctx := context.Background()
		ctx = pprof.WithLabels(ctx, labels)
		pprof.Do(ctx, pprof.Labels(labelKeyPurpose, purpose), func(ctx context.Context) {
			fn()
		})
	} else {
		// Note: Using context.Background() here is acceptable (see SetGoroutineLabel for explanation)
		ctx := context.Background()
		pprof.Do(ctx, labels, func(ctx context.Context) {
			fn()
		})
	}
}

// WithLabel wraps ctx with pprof labels for the given name and optional purpose/arguments.
// If an executable function (func() or func(context.Context)) is provided in extra,
// it executes the function under pprof.Do.
func WithLabel(ctx context.Context, name string, extra ...any) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	purpose := ""
	var runFn func()
	var runCtxFn func(context.Context)

	for _, arg := range extra {
		switch v := arg.(type) {
		case string:
			if purpose == "" {
				purpose = v
			}
		case func():
			runFn = v
		case func(context.Context):
			runCtxFn = v
		}
	}

	labels := pprof.Labels(labelKeyName, name)
	ctx = pprof.WithLabels(ctx, labels)
	if purpose != emptyValue {
		ctx = pprof.WithLabels(ctx, pprof.Labels(labelKeyPurpose, purpose))
	}

	if runFn != nil {
		pprof.Do(ctx, labels, func(context.Context) {
			runFn()
		})
	} else if runCtxFn != nil {
		pprof.Do(ctx, labels, runCtxFn)
	}

	return ctx
}
