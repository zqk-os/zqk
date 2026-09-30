package validation

import (
	"time"

	"github.com/zqk-os/zqk/pkg/hostload"
	"github.com/zqk-os/zqk/pkg/logging"
)

const hostloadValidationYield = 50 * time.Millisecond

// holdHostAwareValidationSlot acquires a semaphore token. Extra slots are refused
// while *foreign* host CPU (AV, other tenants) is over budget. Held slots are never
// released because this check heated the machine — that was self-throttle (~23/s).
func (av *AsyncValidator) holdHostAwareValidationSlot(objectID string) (held, timedOut bool) {
	start := time.Now()
	deadline := start.Add(semaphoreFullWaitTimeout)
	for {
		if time.Now().After(deadline) {
			return false, true
		}
		inUse := len(av.validationSemaphore)
		capN := cap(av.validationSemaphore)
		if hostload.OverBudgetExcludingSelf(inUse+1, capN) {
			yield := time.NewTimer(hostloadValidationYield)
			select {
			case <-yield.C:
				continue
			case <-av.ctx.Done():
				yield.Stop()
				return false, false
			case <-av.shutdown:
				yield.Stop()
				return false, false
			}
		}
		remain := time.Until(deadline)
		timer := time.NewTimer(remain)
		select {
		case av.validationSemaphore <- struct{}{}:
			timer.Stop()
			if wait := time.Since(start); wait > 100*time.Millisecond {
				logging.Fluent(av.logger).Debug("Semaphore acquisition delayed").
					ObjectID(objectID).
					WaitTime(wait.String()).
					Log()
			}
			return true, false
		case <-timer.C:
			return false, true
		case <-av.ctx.Done():
			timer.Stop()
			return false, false
		case <-av.shutdown:
			timer.Stop()
			return false, false
		}
	}
}
