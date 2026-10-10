package coordination

import (
	"context"
	"time"

	"github.com/zqk-os/zqk/pkg/goroutinelabels"
)

const coordinatorRouterTimeout = 5 * time.Second

// runCoordinatorRouterSync runs router work on the caller's goroutine. The timeout context is
// cancelled when this function returns (defer cancel at Emit scope is correct here).
func runCoordinatorRouterSync(emitCtx context.Context, work func(ctx context.Context)) {
	routerCtx, cancel := context.WithTimeout(emitCtx, coordinatorRouterTimeout)
	defer cancel()
	work(routerCtx)
}

// runCoordinatorRouterAsync schedules router work on a labeled goroutine via StartWithContext.
// DefaultOperationalRouter uses this for per-subscriber delivery as well as Coordinator.Emit async stages.
//
// Invariant: context.WithTimeout returns (routerCtx, cancel). If cancel() runs when the caller
// returns (e.g. defer cancel() in Emit), the async goroutine may observe an already-cancelled
// context before running—router.Emit never runs. For async stages, cancel must be deferred inside
// the callback passed to StartWithContext, not at Emit scope. This helper encodes that pairing.
func runCoordinatorRouterAsync(emitCtx context.Context, goroutineName, goroutinePurpose string, work func(ctx context.Context)) {
	var nilCoord *Coordinator
	nilCoord.runRouterAsync(emitCtx, goroutineName, goroutinePurpose, work)
}

// runRouterAsync schedules router work on a labeled goroutine tracked by c.wg.
func (c *Coordinator) runRouterAsync(emitCtx context.Context, goroutineName, goroutinePurpose string, work func(ctx context.Context)) {
	if c != nil {
		c.wg.Add(1)
	}
	routerCtx, cancel := context.WithTimeout(emitCtx, coordinatorRouterTimeout)
	bud := goroutinelabels.DefaultBudget()
	b := goroutinelabels.NewGoroutine(goroutineName, goroutinePurpose)
	if bud != nil {
		b = b.WithBudget(bud)
	}
	b.StartWithContext(routerCtx, func(ctx context.Context) error {
		if c != nil {
			defer c.wg.Done()
		}
		defer cancel()
		work(ctx)
		return nil
	})
}
