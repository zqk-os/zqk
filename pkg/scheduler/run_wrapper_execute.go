package scheduler

import (
	"context"
)

func (h *RunWrapperHandler) Execute(ctx context.Context, job *ScheduledJob) error {
	return h.executeRunWrapperCore(ctx, job)
}
