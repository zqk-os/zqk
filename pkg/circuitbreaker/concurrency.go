// Package scheduler: package_concurrency_limiter limits how many run_wrapper jobs
// for the same go-test package path may run concurrently (separate go test processes).
// Limits come from scan output (.zqk/test-bundles/package_concurrency_limits.json) and
// scheduler_job metadata (max_concurrent_same_package), merged on ReloadJobs — not from
// hardcoded package paths.

package circuitbreaker

import (
	"context"
	"strings"
	"sync"
	"time"

	"github.com/lanceman/zqk/pkg/concurrency"
	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/errfmt"
	"github.com/lanceman/zqk/pkg/logging"
)

// defaultMaxConcurrentPerPackage is used when creating a semaphore for a path whose limit
// is missing or non-positive in the limits map (internal fallback).
const defaultMaxConcurrentPerPackage = 1

type pkgSem struct {
	limit   int
	inUse   int
	waiters []chan struct{}
}

// MapConcurrencyLimiter limits concurrent execution of run_wrapper jobs by package path.
// For example, pkg/storage defaults to 1 concurrent job unless ZQK_SCHEDULER_MAX_CONCURRENT_PKG_STORAGE overrides.
type MapConcurrencyLimiter struct {
	mu      sync.Mutex
	sems    map[string]*pkgSem // packagePath -> semaphore state
	limits  map[string]int     // packagePath -> max concurrent (default 1)
	maxWait time.Duration      // max time to wait for a slot before failing
	logger  logging.Logger
}

// NewConcurrencyLimiter creates an empty limiter; MergeLimits applies scan-driven caps.
// maxWait is how long to wait for a slot before returning an error (e.g. 30*time.Minute).
// NewConcurrencyLimiter creates a new package concurrency limiter
func NewConcurrencyLimiter(logger logging.Logger, maxWait time.Duration) ConcurrencyLimiter {
	limiter := &MapConcurrencyLimiter{
		sems:    make(map[string]*pkgSem),
		limits:  make(map[string]int),
		maxWait: maxWait,
		logger:  logger,
	}
	return limiter
}

// MergeLimits upserts per-package caps. Value <= 0 removes the limit when the semaphore is empty.
// If capacity changes and the semaphore has no in-flight tokens, the channel is recreated.
func (l *MapConcurrencyLimiter) MergeLimits(next map[string]int) {
	if l == nil || len(next) == 0 {
		return
	}
	_ = concurrency.RunInLockWithLogger(
		&l.mu, "SchedulerPackageLimiterGetSem", logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			for path, v := range next {
				pp := strings.TrimPrefix(strings.TrimSpace(path), "./")
				if pp == "" {
					continue
				}
				if v <= 0 {
					delete(l.limits, pp)
					if sem, ok := l.sems[pp]; ok {
						sem.limit = defaultMaxConcurrentPerPackage
						for sem.inUse < sem.limit && len(sem.waiters) > 0 {
							ch := sem.waiters[0]
							sem.waiters = sem.waiters[1:]
							sem.inUse++
							close(ch)
						}
					}
					continue
				}
				if v > 32 {
					v = 32
				}
				prev, had := l.limits[pp]
				l.limits[pp] = v
				if !had {
					// wait for getOrCreateSem
				} else if prev == v {
					continue
				}
				if sem, ok := l.sems[pp]; ok {
					sem.limit = v
					for sem.inUse < sem.limit && len(sem.waiters) > 0 {
						ch := sem.waiters[0]
						sem.waiters = sem.waiters[1:]
						sem.inUse++
						close(ch)
					}
				}
			}
			return nil
		},
	)
}

// getOrCreateSem returns the semaphore state for the package path, creating it if needed.
// Caller must hold l.mu.
func (l *MapConcurrencyLimiter) getOrCreateSem(packagePath string) *pkgSem {
	if l.sems[packagePath] == nil {
		cap := l.limits[packagePath]
		if cap <= 0 {
			cap = defaultMaxConcurrentPerPackage
		}
		l.sems[packagePath] = &pkgSem{limit: cap}
	}
	return l.sems[packagePath]
}

// Acquire blocks until a slot is available for the package path or ctx is done / maxWait expires.
// Returns nil on success. Call Release(packagePath) when the job finishes.
// Uses RunInLockWithLogger for the critical section (lock ordering: see LOCK_ORDERING.md).
func (l *MapConcurrencyLimiter) Acquire(ctx context.Context, packagePath string) error {
	if packagePath == "" {
		return nil
	}
	var waitCh chan struct{}
	err := concurrency.RunInLockWithLogger(
		&l.mu, "SchedulerPackageLimiterGetSem", logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			sem := l.getOrCreateSem(packagePath)
			if sem.inUse < sem.limit {
				sem.inUse++
			} else {
				waitCh = make(chan struct{})
				sem.waiters = append(sem.waiters, waitCh)
			}
			return nil
		},
	)
	if err != nil {
		return err
	}
	if waitCh == nil {
		return nil
	}

	acquireDeadline := time.Now().Add(l.maxWait)
	if deadline, ok := ctx.Deadline(); ok && deadline.Before(acquireDeadline) {
		acquireDeadline = deadline
	}
	acquireCtx, cancel := context.WithDeadline(ctx, acquireDeadline)
	defer cancel()

	select {
	case <-waitCh:
		return nil
	case <-acquireCtx.Done():
		_ = concurrency.RunInLockWithLogger(
			&l.mu, "SchedulerPackageLimiterCancel", logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
			func() error {
				sem := l.sems[packagePath]
				if sem == nil {
					return nil
				}
				for i, w := range sem.waiters {
					if w == waitCh {
						sem.waiters = append(sem.waiters[:i], sem.waiters[i+1:]...)
						return nil
					}
				}
				// If not in waiters, it was already granted. We must release it.
				sem.inUse--
				for sem.inUse < sem.limit && len(sem.waiters) > 0 {
					ch := sem.waiters[0]
					sem.waiters = sem.waiters[1:]
					sem.inUse++
					close(ch)
				}
				return nil
			},
		)
		return errfmt.Errorf("timed out waiting for package concurrency slot for %q (max wait %v): %w", packagePath, l.maxWait, acquireCtx.Err())
	}
}

// Release releases the slot for the package path. Must be called once per successful Acquire.
// Uses RunInLockWithLogger for the critical section (lock ordering: see LOCK_ORDERING.md).
func (l *MapConcurrencyLimiter) Release(packagePath string) {
	if packagePath == "" {
		return
	}
	_ = concurrency.RunInLockWithLogger(
		&l.mu, "SchedulerPackageLimiterReleaseRead", logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			sem := l.sems[packagePath]
			if sem == nil {
				return nil
			}
			sem.inUse--
			for sem.inUse < sem.limit && len(sem.waiters) > 0 {
				ch := sem.waiters[0]
				sem.waiters = sem.waiters[1:]
				sem.inUse++
				close(ch)
			}
			return nil
		},
	)
}

// ExtractPackagePathFromRunWrapperCommand returns the Go package path from a run_wrapper job's
// command and args (e.g. "go test ./pkg/storage -run ..." -> "pkg/storage").
// Returns empty string if not a "go test" command or path not found.
// command is often "/bin/sh" with commandArgs ["-c", "go test ./pkg/storage -run '^...' ..."].
func ExtractPackagePathFromRunWrapperCommand(command string, commandArgs []string) string {
	for _, arg := range commandArgs {
		if strings.HasPrefix(arg, "./") && arg != "./..." && !strings.HasPrefix(arg, "./.") {
			return strings.TrimPrefix(arg, "./")
		}
		// Shell -c: single string like "go test ./pkg/storage -run '^...'"
		if strings.Contains(arg, "go test") {
			for _, part := range strings.Fields(arg) {
				part = strings.Trim(part, "'\"")
				if strings.HasPrefix(part, "./pkg/") || strings.HasPrefix(part, "./cmd/") {
					return strings.TrimPrefix(part, "./")
				}
			}
		}
	}
	return ""
}

// Snapshot reports the configured concurrency cap and how many slots are currently held
// for packagePath (semaphore send tokens in the buffered channel). Use when Acquire fails to
// tell “all slots taken” (inUse == limit) from limits-only state.
// SetLimit sets the concurrency limit for a specific package path
func (l *MapConcurrencyLimiter) SetLimit(packagePath string, limit int) {
	l.MergeLimits(map[string]int{packagePath: limit})
}

func (l *MapConcurrencyLimiter) Snapshot(packagePath string) (limit int, inUse int) {
	if l == nil || packagePath == "" {
		return 0, 0
	}
	_ = concurrency.RunInLockWithLogger(
		&l.mu, "SchedulerPackageLimiterSnapshot", logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			if sem, ok := l.sems[packagePath]; ok && sem != nil {
				inUse = sem.inUse
				limit = sem.limit
				return nil
			}
			if n, ok := l.limits[packagePath]; ok && n > 0 {
				limit = n
			}
			return nil
		},
	)
	return limit, inUse
}

// ShouldLimit returns true if this package path has a concurrency limit.
func (l *MapConcurrencyLimiter) ShouldLimit(packagePath string) bool {
	var result bool
	_ = concurrency.RunInLockWithLogger(
		&l.mu, "SchedulerPackageLimiterShouldLimit", logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			n, ok := l.limits[packagePath]
			result = ok && n > 0
			return nil
		},
	)
	return result
}
