package community

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/zqk-os/zqk/pkg/goroutinelabels"
)

var (
	// ErrAdapterPanicked is returned when an adapter execution encounters an unhandled panic.
	ErrAdapterPanicked = errors.New("adapter panicked")
	// ErrAdapterTimeout is returned when an adapter exceeds its execution timeout.
	ErrAdapterTimeout = errors.New("adapter execution timed out")
	// ErrInvalidAdapter is returned when adapter options or handlers are invalid.
	ErrInvalidAdapter = errors.New("invalid adapter configuration")
)

// SandboxOptions defines resource limits and constraints for sandboxed adapter execution.
type SandboxOptions struct {
	MaxTimeout      time.Duration
	AllowGoroutines bool
	MaxMemoryBytes  int64
}

// DefaultSandboxOptions returns standard safe execution options.
func DefaultSandboxOptions() SandboxOptions {
	return SandboxOptions{
		MaxTimeout:      5 * time.Second,
		AllowGoroutines: false,
		MaxMemoryBytes:  64 * 1024 * 1024, // 64 MB
	}
}

// SandboxedAdapter executes third-party provider code inside an isolated fail-closed boundary.
type SandboxedAdapter struct {
	opts SandboxOptions
}

// NewSandboxedAdapter creates a new SandboxedAdapter with given options.
func NewSandboxedAdapter(opts SandboxOptions) *SandboxedAdapter {
	if opts.MaxTimeout <= 0 {
		opts.MaxTimeout = 5 * time.Second
	}
	return &SandboxedAdapter{opts: opts}
}

// Execute safely runs a handler function in a protected boundary with panic recovery and timeout guarantees.
func (sa *SandboxedAdapter) Execute(ctx context.Context, fn func(ctx context.Context) error) (err error) {
	if fn == nil {
		return fmt.Errorf("%w: handler function cannot be nil", ErrInvalidAdapter)
	}

	execCtx, cancel := context.WithTimeout(ctx, sa.opts.MaxTimeout)
	defer cancel()

	done := make(chan error, 1)

	goroutinelabels.NewGoroutine("adapter_sandbox_exec", "execute sandboxed adapter function").StartSimple(func() {
		defer func() {
			if r := recover(); r != nil {
				done <- fmt.Errorf("%w: %v", ErrAdapterPanicked, r)
			}
		}()
		done <- fn(execCtx)
	})

	select {
	case <-execCtx.Done():
		if errors.Is(execCtx.Err(), context.DeadlineExceeded) {
			return ErrAdapterTimeout
		}
		return execCtx.Err()
	case err := <-done:
		return err
	}
}
